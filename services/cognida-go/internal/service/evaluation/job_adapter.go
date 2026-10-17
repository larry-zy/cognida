package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	domeval "cognida/internal/model/evaluation"
	jobinfra "cognida/internal/model/job"
)

const EvaluationRunJobType = "evaluation.run"

type RunJobPayload struct {
	TaskID string `json:"task_id"`
}

// JobEnqueuer is the evaluation-facing port adapter. The service only knows
// task IDs, while durable delivery remains infrastructure-owned.
type JobEnqueuer struct{ Queue jobinfra.Queue }

func (e JobEnqueuer) Enqueue(ctx context.Context, taskID string) error {
	if e.Queue == nil {
		return fmt.Errorf("evaluation job queue is not configured")
	}
	j, err := jobinfra.New(taskID, EvaluationRunJobType, RunJobPayload{TaskID: taskID})
	if err != nil {
		return err
	}
	return e.Queue.Enqueue(ctx, j)
}

func decodeRunJob(j jobinfra.Job) (RunJobPayload, error) {
	if j.Type != EvaluationRunJobType {
		return RunJobPayload{}, fmt.Errorf("unexpected evaluation job type %q", j.Type)
	}
	var payload RunJobPayload
	if err := json.Unmarshal(j.Payload, &payload); err != nil {
		return payload, fmt.Errorf("decode evaluation job: %w", err)
	}
	if payload.TaskID == "" {
		return payload, fmt.Errorf("invalid evaluation job payload")
	}
	return payload, nil
}

// RunJobHandler keeps evaluation execution and its domain status transitions
// in the evaluation service while the generic dispatcher owns delivery.
type RunJobHandler struct{ Worker *EvaluationWorker }

func (h RunJobHandler) Handle(ctx context.Context, j jobinfra.Job) error {
	payload, err := decodeRunJob(j)
	if err != nil {
		return err
	}
	if h.Worker == nil {
		return fmt.Errorf("evaluation worker is not configured")
	}
	h.Worker.executeTaskWithContext(ctx, payload.TaskID)
	return nil // executeTask persists domain failures to retain existing API semantics.
}
func (h RunJobHandler) Exhausted(ctx context.Context, j jobinfra.Job, cause error) error {
	payload, err := decodeRunJob(j)
	if err != nil {
		return err
	}
	if h.Worker != nil {
		h.Worker.handleTaskError(ctx, payload.TaskID, cause, j.Attempt)
	}
	return nil
}

// RunRecoverySource exposes only durable pending evaluation work to the
// reusable dispatcher. Fresh running tasks retain their lease and are not
// duplicated during multi-process recovery.
type RunRecoverySource struct {
	Tasks domeval.EvaluationTaskRepository
}

func (s RunRecoverySource) PendingJobs(ctx context.Context, limit int) ([]jobinfra.Job, error) {
	if s.Tasks == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	jobs := make([]jobinfra.Job, 0, limit)
	for _, status := range []domeval.TaskStatus{domeval.TaskStatusPending, domeval.TaskStatusRunning} {
		tasks, _, err := s.Tasks.List(ctx, &domeval.TaskFilter{Status: &status, Page: 1, PageSize: limit})
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if task == nil || task.Type == domeval.OfflineType || (task.Status == domeval.TaskStatusRunning && time.Since(task.UpdatedAt) < MaxTaskTimeout) {
				continue
			}
			if task.Status == domeval.TaskStatusRunning {
				_ = s.Tasks.UpdateStatus(ctx, task.ID, domeval.TaskStatusPending)
			}
			j, err := jobinfra.New(task.ID, EvaluationRunJobType, RunJobPayload{TaskID: task.ID})
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, j)
			if len(jobs) >= limit {
				return jobs, nil
			}
		}
	}
	return jobs, nil
}
