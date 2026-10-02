package collection

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "cognida/internal/model/collection"
)

type schedulerClock struct{ now time.Time }

func (c schedulerClock) Now() time.Time { return c.now }

type dueTasksStub struct {
	tasks []*domain.Task
	now   time.Time
	limit int
	err   error
}

func (s *dueTasksStub) ClaimDueTasks(_ context.Context, now time.Time, limit int) ([]*domain.Task, error) {
	s.now, s.limit = now, limit
	return s.tasks, s.err
}

type runnerStub struct {
	started []string
	errFor  map[string]error
}

func (r *runnerStub) Start(_ context.Context, tenantID int64, taskID string) error {
	r.started = append(r.started, taskID)
	return r.errFor[taskID]
}
func (*runnerStub) Cancel(context.Context, int64, string) error { return nil }

func TestScheduleDispatcherClaimsDueTasksAndDispatchesEveryTask(t *testing.T) {
	now := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	repo := &dueTasksStub{tasks: []*domain.Task{{ID: "a", TenantID: 1}, {ID: "b", TenantID: 2}}}
	runner := &runnerStub{errFor: map[string]error{"a": errors.New("queue unavailable")}}
	dispatcher := NewScheduleDispatcher(repo, runner, schedulerClock{now: now}, 12)
	if err := dispatcher.Tick(context.Background()); !errors.Is(err, runner.errFor["a"]) {
		t.Fatalf("dispatch failure must be returned: %v", err)
	}
	if !repo.now.Equal(now) || repo.limit != 12 {
		t.Fatalf("claim arguments = %s, %d", repo.now, repo.limit)
	}
	if len(runner.started) != 2 || runner.started[0] != "a" || runner.started[1] != "b" {
		t.Fatalf("started = %#v", runner.started)
	}
}

func TestScheduleDispatcherReturnsClaimError(t *testing.T) {
	want := errors.New("database unavailable")
	dispatcher := NewScheduleDispatcher(&dueTasksStub{err: want}, &runnerStub{}, schedulerClock{}, 1)
	if err := dispatcher.Tick(context.Background()); !errors.Is(err, want) {
		t.Fatalf("tick error = %v, want %v", err, want)
	}
}

type recoveringRunner struct {
	runnerStub
	recoveries int
}

func (r *recoveringRunner) RecoverPendingRuns(context.Context, int) error { r.recoveries++; return nil }
func TestScheduleTickPerformsRecoveryEveryTime(t *testing.T) {
	runner := &recoveringRunner{}
	dispatcher := NewScheduleDispatcher(&dueTasksStub{}, runner, schedulerClock{}, 10)
	if err := dispatcher.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runner.recoveries != 2 {
		t.Fatalf("recoveries=%d", runner.recoveries)
	}
}
