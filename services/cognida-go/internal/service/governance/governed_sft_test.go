package governance

import (
	d "cognida/internal/model/governance"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type sftClock struct {
	d.Repository
	now time.Time
	err error
}

func (s sftClock) GovernedNow(context.Context) (time.Time, error) { return s.now, s.err }

func TestSFTCommitFenceUsesDatabaseClockAndActualTerminal(t *testing.T) {
	now := time.Now().UTC()
	op := &d.Record{Meta: d.Meta{CreatedAt: now.Add(-time.Second)}, Status: "running"}
	st := &d.GovernedExecution{Invocation: d.GovernedInvocation{Deadline: now.Add(time.Minute), TimeoutSeconds: 60}}
	deadline := now.Add(59 * time.Second)
	st.ExecutionDeadline = &deadline
	require.NoError(t, sftCommitAllowed(context.Background(), op, st, sftClock{now: now}))
	for _, status := range []string{"completed", "cancelled", "failed", "cancel_requested"} {
		op.Status = status
		require.ErrorIs(t, sftCommitAllowed(context.Background(), op, st, sftClock{now: now}), context.Canceled)
	}
	op.Status = "queued"
	require.ErrorIs(t, sftCommitAllowed(context.Background(), op, st, sftClock{now: now.Add(time.Minute)}), context.DeadlineExceeded)
	st.Invocation.TimeoutSeconds = 120
	st.Invocation.Deadline = now
	st.ExecutionDeadline = &now
	require.ErrorIs(t, sftCommitAllowed(context.Background(), op, st, sftClock{now: now}), context.DeadlineExceeded)
	st.Invocation.Deadline = now.Add(time.Minute)
	st.ExecutionDeadline = &deadline
	failure := errors.New("database unavailable")
	require.ErrorIs(t, sftCommitAllowed(context.Background(), op, st, sftClock{now: now, err: failure}), failure)
	require.ErrorIs(t, sftCommitAllowed(context.Background(), op, st, &unitRepo{}), d.ErrInvalid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, sftCommitAllowed(ctx, op, st, sftClock{now: now}), context.Canceled)
}
