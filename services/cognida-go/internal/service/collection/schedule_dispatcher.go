package collection

import (
	"context"
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
	tasks, err := s.repository.ClaimDueTasks(ctx, s.clock.Now().UTC(), s.batchSize)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = s.runner.Start(ctx, task.TenantID, task.ID)
	}
	return nil
}
