package lifecycle

import (
	"context"
	"testing"
	"time"

	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/lifecycle"
	"cognida/internal/model/objectstore"
	"github.com/stretchr/testify/require"
)

type timeoutStageRepository struct {
	d.Repository
	stage  d.StageRun
	events []d.Event
}

func (r *timeoutStageRepository) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return fn(r)
}
func (r *timeoutStageRepository) FindStage(context.Context, int64, string, bool) (*d.StageRun, error) {
	copy := r.stage
	return &copy, nil
}
func (r *timeoutStageRepository) UpdateStage(_ context.Context, st *d.StageRun) error {
	r.stage = *st
	return nil
}
func (r *timeoutStageRepository) AppendEvent(_ context.Context, e *d.Event) error {
	r.events = append(r.events, *e)
	return nil
}

type timeoutQueue struct{ job.Queue }

func (*timeoutQueue) Enqueue(context.Context, job.Job) error { return nil }

type timeoutStore struct{ objectstore.Store }

func TestStageTimeoutPersistsFailureAndExhaustsRetryBudget(t *testing.T) {
	for _, lateSuccess := range []bool{false, true} {
		t.Run(map[bool]string{false: "adapter_error", true: "late_success"}[lateSuccess], func(t *testing.T) {
			repo := &timeoutStageRepository{stage: d.StageRun{ID: "stage-timeout", TenantID: 1, UserID: 2, AgentID: "data-access", Status: d.StatusQueued, MaxAttempts: 2}}
			calls := 0
			s, err := NewService(repo, timeoutStore{}, &timeoutQueue{}, Config{StageTimeout: 20 * time.Millisecond, Adapters: map[string]d.Adapter{"data-access": adapterFunc(func(ctx context.Context, _ d.StageRequest) (d.Output, error) {
				calls++
				_, hasDeadline := ctx.Deadline()
				require.True(t, hasDeadline)
				<-ctx.Done()
				if lateSuccess {
					return d.Output{Gate: d.GateResult{Status: d.GatePassed}}, nil
				}
				return d.Output{}, ctx.Err()
			})}})
			require.NoError(t, err)
			j, err := stageJob(repo.stage)
			require.NoError(t, err)
			require.ErrorIs(t, s.handle(context.Background(), j), context.DeadlineExceeded)
			require.Equal(t, d.StatusQueued, repo.stage.Status)
			require.NoError(t, s.handle(context.Background(), j))
			require.Equal(t, d.StatusFailed, repo.stage.Status)
			require.Contains(t, repo.stage.Error, "stage execution exceeded")
			require.Nil(t, repo.stage.Output)
			require.NoError(t, s.handle(context.Background(), j))
			require.Equal(t, 2, calls, "terminal redelivery must not invoke the adapter")
		})
	}
}

func TestStageParentCancellationDoesNotCommitTimeout(t *testing.T) {
	repo := &timeoutStageRepository{stage: d.StageRun{ID: "stage-cancel", TenantID: 1, UserID: 2, AgentID: "data-access", Status: d.StatusQueued, MaxAttempts: 2}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := NewService(repo, timeoutStore{}, &timeoutQueue{}, Config{Adapters: map[string]d.Adapter{"data-access": adapterFunc(func(ctx context.Context, _ d.StageRequest) (d.Output, error) {
		cancel()
		<-ctx.Done()
		return d.Output{}, ctx.Err()
	})}})
	require.NoError(t, err)
	require.Equal(t, 5*time.Minute, s.config.StageTimeout)
	j, err := stageJob(repo.stage)
	require.NoError(t, err)
	require.ErrorIs(t, s.handle(ctx, j), context.Canceled)
	require.Equal(t, d.StatusRunning, repo.stage.Status, "lease loss/shutdown must leave ownership to recovery")
	require.Empty(t, repo.stage.Error)
}
