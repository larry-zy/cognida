package collection

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "cognida/internal/model/collection"
)

// ScheduleDispatcher bridges collection's durable due-task store to its
// runner. Generic timer lifecycle is provided by infrastructure/job.Scheduler.
type ScheduleDispatcher struct {
	repository dueTaskClaimer
	runner     domain.Runner
	clock      domain.Clock
	batchSize  int
}

type dueTaskClaimer interface {
	ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]*domain.Task, error)
}

func NewScheduleDispatcher(repository dueTaskClaimer, runner domain.Runner, clock domain.Clock, batchSize int) *ScheduleDispatcher {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &ScheduleDispatcher{repository: repository, runner: runner, clock: clock, batchSize: batchSize}
}

// Tick claims due collection tasks and submits every claim independently.
// Dispatch retry is deliberately owned by the configured durable queue runner.
func (s *ScheduleDispatcher) Tick(ctx context.Context) error {
	var failures []error
	if recovery, ok := s.runner.(interface {
		RecoverPendingRuns(context.Context, int) error
	}); ok {
		if err := recovery.RecoverPendingRuns(ctx, s.batchSize); err != nil {
			failures = append(failures, err)
		}
	}
	tasks, err := s.repository.ClaimDueTasks(ctx, s.clock.Now().UTC(), s.batchSize)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.runner.Start(ctx, task.TenantID, task.ID); err != nil {
			failures = append(failures, fmt.Errorf("dispatch collection %s: %w", task.ID, err))
		}
	}
	return errors.Join(failures...)
}
