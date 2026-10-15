//go:build integration

package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	d "cognida/internal/model/taskcontrol"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func tcRuntimeStart(t *testing.T, r *TaskControlRepository, scope d.Scope, nodes, attempts int) (*d.Task, *d.TaskLease) {
	t.Helper()
	ctx := context.Background()
	budget := tcRuntimeBudget{MaxNodes: nodes, MaxConcurrency: nodes, MaxAttempts: nodes * attempts, MaxCalls: nodes * attempts, MaxCostMicros: int64(nodes * attempts * 100), TimeoutSeconds: 300}
	raw, err := json.Marshal(budget)
	require.NoError(t, err)
	task, err := r.CreateTask(ctx, scope, d.CreateTaskInput{Goal: "synthetic runtime validation", InputRefs: []byte(`[]`), Budget: raw, IdempotencyKey: uuid.NewString()})
	require.NoError(t, err)
	plan := tcRuntimePlan{Budget: budget}
	for i := 0; i < nodes; i++ {
		plan.Nodes = append(plan.Nodes, tcRuntimeNode{ID: fmt.Sprintf("node%d", i), MaxAttempts: attempts, MaxCallsPerAttempt: 1, MaxCostMicrosPerAttempt: 100})
	}
	raw, err = json.Marshal(plan)
	require.NoError(t, err)
	_, err = r.AppendPlan(ctx, scope, task.ID, task.Version, raw)
	require.NoError(t, err)
	task = tcRefresh(t, r, scope, task.ID)
	task, err = r.BeginExecution(ctx, scope, task.ID, task.Version, d.StartExecutionInput{PlanVersion: 1, IdempotencyKey: "execute", TimeoutSeconds: 300})
	require.NoError(t, err)
	lease, err := r.ClaimTask(ctx, scope, task.ID, "test-worker", time.Minute)
	require.NoError(t, err)
	return task, lease
}
func tcRuntimeEnqueue(t *testing.T, r *TaskControlRepository, s d.Scope, taskID, node string, attempt int, f d.ExecutionFence) *d.Child {
	t.Helper()
	task := tcRefresh(t, r, s, taskID)
	child, err := r.EnqueueChild(context.Background(), s, taskID, task.Version, d.DispatchInput{PlanVersion: 1, NodeID: node, Attempt: attempt, Domain: "lifecycle", Payload: []byte(`{"synthetic":true}`), Fence: f})
	require.NoError(t, err)
	return child
}
func tcRuntimeBind(t *testing.T, r *TaskControlRepository, s d.Scope, child *d.Child, f d.ExecutionFence) *d.Child {
	t.Helper()
	event := tcEvent(t, r, s, child.ID)
	_, err := r.ClaimOutboxWithLease(context.Background(), s, event.ID, event.Version, f)
	require.NoError(t, err)
	child, err = r.GetChild(context.Background(), s, child.ID)
	require.NoError(t, err)
	child, err = r.BindChild(context.Background(), s, child.ID, child.Version, child.Attempt, d.Binding{DomainID: uuid.NewString(), Receipt: []byte(`{"accepted":true}`), Fence: f})
	require.NoError(t, err)
	return child
}
func tcRuntimeObserve(t *testing.T, r *TaskControlRepository, s d.Scope, child *d.Child, status string, f d.ExecutionFence) *d.Child {
	t.Helper()
	refs := json.RawMessage(`[]`)
	if status == d.DomainSucceeded {
		refs = json.RawMessage(fmt.Sprintf(`[{"asset_id":%q,"version":1}]`, child.ID))
	}
	out, err := r.ObserveChild(context.Background(), s, child.ID, child.Version, child.Attempt, d.ChildObservation{DomainID: child.DomainID, DomainStatus: status, OutputRefs: refs, ObservedAt: time.Now().UTC(), Fence: f})
	require.NoError(t, err)
	return out
}
func TestTaskRuntimeIntegrationStartLeaseAndIsolation(t *testing.T) {
	r, _, s := taskControlIT(t)
	ctx := context.Background()
	task, lease := tcRuntimeStart(t, r, s, 1, 2)
	require.Equal(t, d.StatusRunning, task.Status)
	require.Equal(t, int64(1), task.ExecutionPlanVersion)
	require.NotNil(t, task.StartedAt)
	require.NotNil(t, task.DeadlineAt)
	replay, err := r.BeginExecution(ctx, s, task.ID, 1, d.StartExecutionInput{PlanVersion: 1, IdempotencyKey: "execute", TimeoutSeconds: 300})
	require.NoError(t, err)
	require.Equal(t, task.ID, replay.ID)
	_, err = r.BeginExecution(ctx, s, task.ID, task.Version, d.StartExecutionInput{PlanVersion: 1, IdempotencyKey: "execute", TimeoutSeconds: 299})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.AppendPlan(ctx, s, task.ID, task.Version, []byte(`{}`))
	require.ErrorIs(t, err, d.ErrConflict)
	renewed, err := r.RenewTaskLease(ctx, s, task.ID, lease.Fence(), time.Minute)
	require.NoError(t, err)
	require.Equal(t, lease.Epoch, renewed.Epoch)
	require.Equal(t, task.Version, tcRefresh(t, r, s, task.ID).Version)
	for _, foreign := range []d.Scope{{TenantID: s.TenantID + 1, UserID: s.UserID}, {TenantID: s.TenantID, UserID: s.UserID + 1}} {
		_, err = r.ClaimTask(ctx, foreign, task.ID, "foreign", time.Second)
		require.ErrorIs(t, err, d.ErrNotFound)
		_, err = r.RenewTaskLease(ctx, foreign, task.ID, lease.Fence(), time.Second)
		require.ErrorIs(t, err, d.ErrNotFound)
		_, err = r.ListOutbox(ctx, foreign, d.OutboxFilter{TaskID: task.ID})
		require.ErrorIs(t, err, d.ErrNotFound)
	}
	require.NoError(t, r.ReleaseTaskLease(ctx, s, task.ID, lease.Fence(), time.Time{}))
	var wg sync.WaitGroup
	wins := make(chan *d.TaskLease, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, e := r.ClaimTask(ctx, s, task.ID, fmt.Sprintf("worker%d", i), time.Minute)
			if e == nil {
				wins <- l
			} else {
				errs <- e
			}
		}(i)
	}
	wg.Wait()
	close(wins)
	close(errs)
	require.Len(t, wins, 1)
	for err := range errs {
		require.ErrorIs(t, err, d.ErrConflict)
	}
	winner := <-wins
	require.Greater(t, winner.Epoch, lease.Epoch)
	_, err = r.EnqueueChild(ctx, s, task.ID, task.Version, d.DispatchInput{PlanVersion: 1, NodeID: "node0", Attempt: 1, Domain: "lifecycle", Payload: []byte(`{}`), Fence: lease.Fence()})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.FinalizeTask(ctx, s, task.ID, task.Version, d.TaskUpdate{Status: d.StatusFailed, Fence: lease.Fence()})
	require.ErrorIs(t, err, d.ErrConflict)
	require.NoError(t, r.ReleaseTaskLease(ctx, s, task.ID, winner.Fence(), time.Time{}))
	page, err := r.ListRunnable(ctx, d.ListFilter{Limit: 100})
	require.NoError(t, err)
	found := false
	for _, item := range page.Items {
		if item.ID == task.ID {
			found = true
			require.Equal(t, s, item.Scope)
		}
	}
	require.True(t, found)
	lease, err = r.ClaimTask(ctx, s, task.ID, "last-worker", time.Minute)
	require.NoError(t, err)
	task = tcRefresh(t, r, s, task.ID)
	task, err = r.RequestCancel(ctx, s, task.ID, task.Version)
	require.NoError(t, err)
	require.Equal(t, d.StatusCancelRequested, task.Status)
	require.Equal(t, "user_cancelled", task.Reason)
	task, err = r.FinalizeTask(ctx, s, task.ID, task.Version, d.TaskUpdate{Status: d.StatusCancelled, Reason: task.Reason, Fence: lease.Fence()})
	require.NoError(t, err)
	require.NotNil(t, task.FinishedAt)
	_, err = r.ClaimTask(ctx, s, task.ID, "resurrection", time.Minute)
	require.ErrorIs(t, err, d.ErrConflict)
}
func TestTaskRuntimeIntegrationObservationRetryAndTruth(t *testing.T) {
	r, _, s := taskControlIT(t)
	ctx := context.Background()
	task, lease := tcRuntimeStart(t, r, s, 1, 2)
	f := lease.Fence()
	child := tcRuntimeEnqueue(t, r, s, task.ID, "node0", 1, f)
	require.Equal(t, 1, child.CallsReserved)
	require.Equal(t, int64(100), child.CostMicrosReserved)
	child = tcRuntimeBind(t, r, s, child, f)
	current := tcRefresh(t, r, s, task.ID)
	_, err := r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusCompleted, Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.EnqueueChild(ctx, s, task.ID, current.Version, d.DispatchInput{PlanVersion: 1, NodeID: "node0", Attempt: 2, Domain: "lifecycle", Payload: []byte(`{}`), Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	child = tcRuntimeObserve(t, r, s, child, d.DomainFailed, f)
	_, err = r.ObserveChild(ctx, s, child.ID, child.Version, 1, d.ChildObservation{DomainID: child.DomainID, DomainStatus: d.DomainRunning, ObservedAt: time.Now().Add(time.Second), Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	child2 := tcRuntimeEnqueue(t, r, s, task.ID, "node0", 2, f)
	child2 = tcRuntimeBind(t, r, s, child2, f)
	child2 = tcRuntimeObserve(t, r, s, child2, d.DomainSucceeded, f)
	current = tcRefresh(t, r, s, task.ID)
	_, err = r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusCompleted, ResultRefs: []byte(`[{"asset_id":"fabricated","version":1}]`), Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	complete, err := r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusCompleted, ResultRefs: child2.OutputRefs, Fence: f})
	require.NoError(t, err)
	require.JSONEq(t, string(child2.OutputRefs), string(complete.ResultRefs))
	_, err = r.ObserveChild(ctx, s, child2.ID, child2.Version, 2, d.ChildObservation{DomainID: child2.DomainID, DomainStatus: d.DomainFailed, ObservedAt: time.Now().Add(time.Second), Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
}
func TestTaskRuntimeIntegrationStopAndLateReceipt(t *testing.T) {
	r, _, s := taskControlIT(t)
	ctx := context.Background()
	task, lease := tcRuntimeStart(t, r, s, 2, 1)
	f := lease.Fence()
	first := tcRuntimeEnqueue(t, r, s, task.ID, "node0", 1, f)
	pending := tcRuntimeEnqueue(t, r, s, task.ID, "node1", 1, f)
	event := tcEvent(t, r, s, first.ID)
	event, err := r.ClaimOutboxWithLease(ctx, s, event.ID, event.Version, f)
	require.NoError(t, err)
	current := tcRefresh(t, r, s, task.ID)
	stopping, err := r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusCancelRequested, Reason: "budget_timeout", Fence: f})
	require.NoError(t, err)
	require.Equal(t, "budget_timeout", stopping.Reason)
	pending, err = r.GetChild(ctx, s, pending.ID)
	require.NoError(t, err)
	require.Equal(t, d.ChildCancelled, pending.Status)
	_, err = r.FinalizeTask(ctx, s, task.ID, stopping.Version, d.TaskUpdate{Status: d.StatusTimedOut, Reason: stopping.Reason, Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	event, err = r.MarkDispatchUnknownWithLease(ctx, s, event.ID, event.Version, "transport_timeout", f)
	require.NoError(t, err)
	require.Equal(t, "budget_timeout", tcRefresh(t, r, s, task.ID).Reason)
	first, err = r.GetChild(ctx, s, first.ID)
	require.NoError(t, err)
	first, err = r.BindChild(ctx, s, first.ID, first.Version, 1, d.Binding{DomainID: "real-domain-job", Receipt: []byte(`{"committed":true}`), Fence: f})
	require.NoError(t, err)
	first = tcRuntimeObserve(t, r, s, first, d.DomainSucceeded, f)
	current = tcRefresh(t, r, s, task.ID)
	_, err = r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusTimedOut, Reason: current.Reason, Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	done, err := r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusPartiallyCompleted, Reason: current.Reason, ResultRefs: first.OutputRefs, Fence: f})
	require.NoError(t, err)
	require.Equal(t, "budget_timeout", done.Reason)
	require.JSONEq(t, `{"committed":true}`, string(first.Receipt))
}

// This helper is run in a separate OS process. It deliberately exits after an
// outbox claim, without releasing its lease or recording a dispatch response.
func TestTaskRuntimeProcessCrashWorker(t *testing.T) {
	id := os.Getenv("COGNIDA_TASKRUNTIME_CRASH_TASK")
	if id == "" {
		t.Skip("subprocess helper")
	}
	tenant, err := strconv.ParseInt(os.Getenv("COGNIDA_TASKRUNTIME_CRASH_TENANT"), 10, 64)
	require.NoError(t, err)
	cfg, err := driver.ParseDSN(os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN"))
	if err != nil {
		t.Fatal("invalid isolated review DSN")
	}
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "crash subprocess requires isolated review database")
	db, err := gorm.Open(gormmysql.Open(os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	r := NewTaskControlRepository(db)
	s := d.Scope{TenantID: tenant, UserID: 921}
	ctx := context.Background()
	lease, err := r.ClaimTask(ctx, s, id, "crashed-process", 150*time.Millisecond)
	require.NoError(t, err)
	page, err := r.ListOutbox(ctx, s, d.OutboxFilter{TaskID: id})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	_, err = r.ClaimOutboxWithLease(ctx, s, page.Items[0].ID, page.Items[0].Version, lease.Fence())
	require.NoError(t, err)
}
func TestTaskRuntimeIntegrationProcessRestartReconcilesWithoutReplay(t *testing.T) {
	r, _, s := taskControlIT(t)
	ctx := context.Background()
	task, lease := tcRuntimeStart(t, r, s, 1, 1)
	child := tcRuntimeEnqueue(t, r, s, task.ID, "node0", 1, lease.Fence())
	require.NoError(t, r.ReleaseTaskLease(ctx, s, task.ID, lease.Fence(), time.Time{}))
	cmd := exec.Command(os.Args[0], "-test.run=^TestTaskRuntimeProcessCrashWorker$", "-test.count=1")
	cmd.Env = append(os.Environ(), "COGNIDA_TASKRUNTIME_CRASH_TASK="+task.ID, fmt.Sprintf("COGNIDA_TASKRUNTIME_CRASH_TENANT=%d", s.TenantID))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	time.Sleep(180 * time.Millisecond)
	// A fresh database connection represents a restarted controller process.
	fresh, err := gorm.Open(gormmysql.Open(os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := fresh.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	r = NewTaskControlRepository(fresh)
	renewed, err := r.ClaimTask(ctx, s, task.ID, "restart-worker", time.Minute)
	require.NoError(t, err)
	require.Greater(t, renewed.Epoch, lease.Epoch)
	event := tcEvent(t, r, s, child.ID)
	require.Equal(t, d.OutboxDispatching, event.Status)
	_, err = r.ClaimOutboxWithLease(ctx, s, event.ID, event.Version, renewed.Fence())
	require.ErrorIs(t, err, d.ErrConflict)
	event, err = r.MarkDispatchUnknownWithLease(ctx, s, event.ID, event.Version, "restart_unknown", renewed.Fence())
	require.NoError(t, err)
	current := tcRefresh(t, r, s, task.ID)
	_, err = r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusRunning, Fence: renewed.Fence()})
	require.ErrorIs(t, err, d.ErrConflict)
	child, err = r.GetChild(ctx, s, child.ID)
	require.NoError(t, err)
	child, err = r.BindChild(ctx, s, child.ID, child.Version, 1, d.Binding{DomainID: "looked-up-authoritative-job", Receipt: []byte(`{"lookup":true}`), Fence: renewed.Fence()})
	require.NoError(t, err)
	current = tcRefresh(t, r, s, task.ID)
	resumed, err := r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusRunning, Fence: renewed.Fence()})
	require.NoError(t, err)
	require.Equal(t, task.ExecutionPlanVersion, resumed.ExecutionPlanVersion)
	require.True(t, task.DeadlineAt.Equal(*resumed.DeadlineAt))
	child = tcRuntimeObserve(t, r, s, child, d.DomainSucceeded, renewed.Fence())
	current = tcRefresh(t, r, s, task.ID)
	_, err = r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusCompleted, ResultRefs: child.OutputRefs, Fence: renewed.Fence()})
	require.NoError(t, err)
}

func TestTaskRuntimeIntegrationDeadlineReservationAndTaskFilter(t *testing.T) {
	r, db, s := taskControlIT(t)
	ctx := context.Background()
	task, lease := tcRuntimeStart(t, r, s, 2, 2)
	f := lease.Fence()
	child := tcRuntimeEnqueue(t, r, s, task.ID, "node0", 1, f)
	other, otherLease := tcRuntimeStart(t, r, s, 1, 1)
	tcRuntimeEnqueue(t, r, s, other.ID, "node0", 1, otherLease.Fence())
	page, err := r.ListOutbox(ctx, s, d.OutboxFilter{TaskID: task.ID})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, child.ID, page.Items[0].ChildID)
	// Runtime mutations cannot fall back to the old, unfenced API.
	_, err = r.ClaimOutbox(ctx, s, page.Items[0].ID, page.Items[0].Version)
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.UpdateTaskStatus(ctx, s, task.ID, tcRefresh(t, r, s, task.ID).Version, d.TaskUpdate{Status: d.StatusFailed})
	require.ErrorIs(t, err, d.ErrConflict)
	// Simulate expiry of this task only. The worker must persist a stop and settle
	// pending dispatches instead of accepting another child or claiming an event.
	require.NoError(t, tcScope(db.Model(&taskControlTaskRow{}), s).Where("id = ?", task.ID).Update("deadline_at", time.Now().UTC().Add(-time.Second)).Error)
	current := tcRefresh(t, r, s, task.ID)
	_, err = r.EnqueueChild(ctx, s, task.ID, current.Version, d.DispatchInput{PlanVersion: 1, NodeID: "node1", Attempt: 1, Domain: "lifecycle", Payload: []byte(`{}`), Fence: f})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.ClaimOutboxWithLease(ctx, s, page.Items[0].ID, page.Items[0].Version, f)
	require.ErrorIs(t, err, d.ErrConflict)
	stopping, err := r.FinalizeTask(ctx, s, task.ID, current.Version, d.TaskUpdate{Status: d.StatusCancelRequested, Reason: "budget_timeout", Fence: f})
	require.NoError(t, err)
	stopped, err := r.FinalizeTask(ctx, s, task.ID, stopping.Version, d.TaskUpdate{Status: d.StatusTimedOut, Reason: stopping.Reason, Fence: f})
	require.NoError(t, err)
	require.Equal(t, "budget_timeout", stopped.Reason)
	event := tcEvent(t, r, s, child.ID)
	require.Equal(t, d.OutboxCancelled, event.Status)
}

func TestTaskRuntimeIntegrationExpiredFenceAndDriverTimezone(t *testing.T) {
	_, _, s := taskControlIT(t)
	ctx := context.Background()
	cfg, err := driver.ParseDSN(os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN"))
	require.NoError(t, err)
	cfg.Loc, err = time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	r := NewTaskControlRepository(db)
	task, lease := tcRuntimeStart(t, r, s, 1, 1)
	require.WithinDuration(t, time.Now().Add(300*time.Second), *task.DeadlineAt, 3*time.Second)
	child := tcRuntimeEnqueue(t, r, s, task.ID, "node0", 1, lease.Fence())
	child = tcRuntimeBind(t, r, s, child, lease.Fence())
	require.NoError(t, r.ReleaseTaskLease(ctx, s, task.ID, lease.Fence(), time.Time{}))
	lease, err = r.ClaimTask(ctx, s, task.ID, "short-worker", 100*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(130 * time.Millisecond)
	_, err = r.RenewTaskLease(ctx, s, task.ID, lease.Fence(), time.Minute)
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.ObserveChild(ctx, s, child.ID, child.Version, 1, d.ChildObservation{DomainID: child.DomainID, DomainStatus: d.DomainSucceeded, ObservedAt: time.Now(), Fence: lease.Fence()})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = r.BindChild(ctx, s, child.ID, child.Version, 1, d.Binding{DomainID: child.DomainID, Receipt: child.Receipt, Fence: lease.Fence()})
	require.ErrorIs(t, err, d.ErrConflict)
	newLease, err := r.ClaimTask(ctx, s, task.ID, "new-worker", time.Minute)
	require.NoError(t, err)
	child = tcRuntimeObserve(t, r, s, child, d.DomainSucceeded, newLease.Fence())
	require.Equal(t, d.DomainSucceeded, child.DomainStatus)
}
