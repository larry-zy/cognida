package collection

import (
	"context"
	"encoding/json"
	"fmt"

	jobinfra "cognida/internal/infrastructure/job"
	domain "cognida/internal/model/collection"
)

const CollectionRunJobType = "collection.run"

type RunJobPayload struct {
	RunID    string `json:"run_id"`
	TaskID   string `json:"task_id"`
	TenantID int64  `json:"tenant_id"`
}

// RunningRunReader is implemented by the collection persistence adapter. This
// local port prevents the reusable dispatcher from learning collection schema.
type RunningRunReader interface {
	ListRunningRuns(context.Context, int) ([]*domain.Run, error)
}

// JobSubmitter keeps durable delivery outside the collection state machine.
type JobSubmitter struct{ Queue jobinfra.Queue }

func (s JobSubmitter) Submit(ctx context.Context, run *domain.Run) error {
	if s.Queue == nil {
		return fmt.Errorf("collection job queue is not configured")
	}
	j, err := NewRunJob(run)
	if err != nil {
		return err
	}
	return s.Queue.Enqueue(ctx, j)
}

type RunRecoverySource struct{ Runs RunningRunReader }

func (s RunRecoverySource) PendingJobs(ctx context.Context, limit int) ([]jobinfra.Job, error) {
	runs, err := s.Runs.ListRunningRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	jobs := make([]jobinfra.Job, 0, len(runs))
	for _, run := range runs {
		if run == nil || run.Status != domain.RunStatusRunning {
			continue
		}
		j, err := NewRunJob(run)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
func NewRunJob(run *domain.Run) (jobinfra.Job, error) {
	if run == nil {
		return jobinfra.Job{}, fmt.Errorf("collection run is required")
	}
	return jobinfra.New(run.ID, CollectionRunJobType, RunJobPayload{RunID: run.ID, TaskID: run.TaskID, TenantID: run.TenantID})
}
func DecodeRunJob(job jobinfra.Job) (RunJobPayload, error) {
	if job.Type != CollectionRunJobType {
		return RunJobPayload{}, fmt.Errorf("unexpected collection job type %q", job.Type)
	}
	var payload RunJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return RunJobPayload{}, fmt.Errorf("decode collection run job: %w", err)
	}
	if payload.RunID == "" || payload.TaskID == "" || payload.TenantID <= 0 {
		return RunJobPayload{}, fmt.Errorf("invalid collection run job payload")
	}
	return payload, nil
}

// RunJobHandler adapts collection execution to the generic dispatcher. The
// service owns status transitions, so infrastructure never assumes a domain's
// definition of completion or failure.
type RunJobHandler struct {
	Execute     func(context.Context, RunJobPayload) error
	OnExhausted func(context.Context, RunJobPayload, error) error
}

func (h RunJobHandler) Handle(ctx context.Context, job jobinfra.Job) error {
	payload, err := DecodeRunJob(job)
	if err != nil {
		return err
	}
	if h.Execute == nil {
		return fmt.Errorf("collection run executor is not configured")
	}
	return h.Execute(ctx, payload)
}
func (h RunJobHandler) Exhausted(ctx context.Context, job jobinfra.Job, cause error) error {
	payload, err := DecodeRunJob(job)
	if err != nil {
		return err
	}
	if h.OnExhausted == nil {
		return nil
	}
	return h.OnExhausted(ctx, payload, cause)
}
