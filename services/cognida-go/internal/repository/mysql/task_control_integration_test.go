//go:build integration

package mysql

import (
	"context"
	"errors"
	"os"
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

// Only an explicitly provisioned, migrated review database is permitted. Tests
// never migrate schemas and cleanup is limited to their random tenant IDs.
func taskControlIT(t *testing.T) (*TaskControlRepository, *gorm.DB, d.Scope) {
	t.Helper()
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	require.NotEmpty(t, dsn, "COGNIDA_TASKCONTROL_TEST_DSN must name an isolated review database")
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid task-control test DSN")
	}
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated cognida_review_* database required")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var version int64
	var dirty bool
	require.NoError(t, sqlDB.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty))
	require.GreaterOrEqual(t, version, int64(14), "apply versioned migrations first")
	require.False(t, dirty)
	scope := d.Scope{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 921}
	t.Cleanup(func() {
		for _, table := range []string{"task_control_outbox", "task_control_children", "task_control_plans", "task_control_leases", "task_control_tasks"} {
			require.NoError(t, db.Table(table).Where("tenant_id IN ?", []int64{scope.TenantID, scope.TenantID + 1}).Delete(map[string]any{}).Error)
		}
	})
	return NewTaskControlRepository(db), db, scope
}
func tcInput(key string) d.CreateTaskInput {
	return d.CreateTaskInput{Goal: "真实数据库任务基础验证", InputRefs: []byte(`[]`), Budget: []byte(`{"max_cost_micros":9007199254740993}`), IdempotencyKey: key, RequestID: "test-request"}
}
func tcReady(t *testing.T, repo *TaskControlRepository, scope d.Scope) *d.Task {
	t.Helper()
	ctx := context.Background()
	task, err := repo.CreateTask(ctx, scope, tcInput(uuid.NewString()))
	require.NoError(t, err)
	_, err = repo.AppendPlan(ctx, scope, task.ID, task.Version, []byte(`{"nodes":[{"id":"clean"}]}`))
	require.NoError(t, err)
	task, err = repo.GetTask(ctx, scope, task.ID)
	require.NoError(t, err)
	task, err = repo.UpdateTaskStatus(ctx, scope, task.ID, task.Version, d.TaskUpdate{Status: d.StatusReady})
	require.NoError(t, err)
	return task
}
func tcEvent(t *testing.T, repo *TaskControlRepository, scope d.Scope, childID string) *d.Outbox {
	t.Helper()
	page, err := repo.ListOutbox(context.Background(), scope, d.OutboxFilter{})
	require.NoError(t, err)
	for i := range page.Items {
		if page.Items[i].ChildID == childID {
			return &page.Items[i]
		}
	}
	t.Fatal("child outbox missing")
	return nil
}
func tcRefresh(t *testing.T, repo *TaskControlRepository, scope d.Scope, id string) *d.Task {
	t.Helper()
	task, err := repo.GetTask(context.Background(), scope, id)
	require.NoError(t, err)
	return task
}
func tcChildInput() d.DispatchInput {
	return d.DispatchInput{PlanVersion: 1, NodeID: "clean", Attempt: 1, Domain: "lifecycle", Payload: []byte(`{"input_ref":"fixed-source:1"}`)}
}

func TestTaskControlIntegrationImmutablePlanIdempotencyAndIsolation(t *testing.T) {
	repo, _, scope := taskControlIT(t)
	ctx := context.Background()
	input := tcInput("Case-Sensitive-Key")
	task, err := repo.CreateTask(ctx, scope, input)
	require.NoError(t, err)
	require.Equal(t, d.StatusDraft, task.Status)
	require.Equal(t, int64(1), task.Version)
	require.Contains(t, string(task.Budget), "9007199254740993")
	input.RequestID = "retry-request"
	same, err := repo.CreateTask(ctx, scope, input)
	require.NoError(t, err)
	require.Equal(t, task.ID, same.ID)
	input.Goal = "changed"
	_, err = repo.CreateTask(ctx, scope, input)
	require.ErrorIs(t, err, d.ErrConflict)
	input = tcInput("case-sensitive-key")
	other, err := repo.CreateTask(ctx, scope, input)
	require.NoError(t, err)
	require.NotEqual(t, task.ID, other.ID)
	plan1, err := repo.AppendPlan(ctx, scope, task.ID, 1, []byte(`{"v":1,"amount":9007199254740993}`))
	require.NoError(t, err)
	_, err = repo.AppendPlan(ctx, scope, task.ID, 1, []byte(`{"v":2}`))
	require.ErrorIs(t, err, d.ErrConflict)
	plan2, err := repo.AppendPlan(ctx, scope, task.ID, 2, []byte(`{"v":2}`))
	require.NoError(t, err)
	require.Equal(t, int64(2), plan2.Version)
	stored, err := repo.GetPlan(ctx, scope, task.ID, 1)
	require.NoError(t, err)
	require.Equal(t, plan1.ContentHash, stored.ContentHash)
	require.Contains(t, string(stored.Snapshot), "9007199254740993")
	for _, foreign := range []d.Scope{{TenantID: scope.TenantID + 1, UserID: scope.UserID}, {TenantID: scope.TenantID, UserID: scope.UserID + 1}} {
		_, err = repo.GetTask(ctx, foreign, task.ID)
		require.ErrorIs(t, err, d.ErrNotFound)
		_, err = repo.GetPlan(ctx, foreign, task.ID, 1)
		require.ErrorIs(t, err, d.ErrNotFound)
		_, err = repo.AppendPlan(ctx, foreign, task.ID, 3, []byte(`{}`))
		require.ErrorIs(t, err, d.ErrNotFound)
		_, err = repo.RequestCancel(ctx, foreign, task.ID, 3)
		require.ErrorIs(t, err, d.ErrNotFound)
		page, err := repo.ListTasks(ctx, foreign, d.ListFilter{})
		require.NoError(t, err)
		require.Empty(t, page.Items)
		foreignTask, err := repo.CreateTask(ctx, foreign, tcInput("Case-Sensitive-Key"))
		require.NoError(t, err)
		require.NotEqual(t, task.ID, foreignTask.ID)
	}
	page, err := repo.ListTasks(ctx, scope, d.ListFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.NotEmpty(t, page.NextCursor)
	next, err := repo.ListTasks(ctx, scope, d.ListFilter{Limit: 1, Cursor: page.NextCursor})
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	require.NotEqual(t, page.Items[0].ID, next.Items[0].ID)
	require.Empty(t, next.NextCursor)
	task, err = repo.RequestCancel(ctx, scope, task.ID, 3)
	require.NoError(t, err)
	require.Equal(t, d.StatusCancelled, task.Status)
	_, err = repo.AppendPlan(ctx, scope, task.ID, task.Version, []byte(`{}`))
	require.ErrorIs(t, err, d.ErrConflict)
	replay, err := repo.CreateTask(ctx, scope, tcInput("Case-Sensitive-Key"))
	require.NoError(t, err)
	require.Equal(t, d.StatusCancelled, replay.Status)
}

func TestTaskControlIntegrationConcurrentCreateCASAndAtomicOutbox(t *testing.T) {
	repo, db, scope := taskControlIT(t)
	ctx := context.Background()
	const workers = 8
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := repo.CreateTask(ctx, scope, tcInput("concurrent"))
			errs <- err
			if err == nil {
				ids <- task.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		require.Equal(t, first, id)
	}
	wins := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.AppendPlan(ctx, scope, first, 1, []byte(`{"nodes":[]}`))
			wins <- err
		}()
	}
	wg.Wait()
	close(wins)
	succeeded, conflicted := 0, 0
	for err := range wins {
		if err == nil {
			succeeded++
		} else if errors.Is(err, d.ErrConflict) {
			conflicted++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, conflicted)
	task := tcReady(t, repo, scope)
	missingNode := tcChildInput()
	missingNode.NodeID = "not-in-plan"
	_, err := repo.EnqueueChild(ctx, scope, task.ID, task.Version, missingNode)
	require.ErrorIs(t, err, d.ErrInvalid)
	// Inject an outbox insert failure after child insertion to verify transaction rollback.
	callback := "task-control-outbox-failure-" + uuid.NewString()
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "task_control_outbox" {
			tx.AddError(errors.New("injected outbox storage failure"))
		}
	}))
	_, err = repo.EnqueueChild(ctx, scope, task.ID, task.Version, tcChildInput())
	require.ErrorContains(t, err, "injected")
	require.NoError(t, db.Callback().Create().Remove(callback))
	children, err := repo.ListChildren(ctx, scope, task.ID)
	require.NoError(t, err)
	require.Empty(t, children)
	require.Equal(t, task.Version, tcRefresh(t, repo, scope, task.ID).Version)
	child, err := repo.EnqueueChild(ctx, scope, task.ID, task.Version, tcChildInput())
	require.NoError(t, err)
	replay, err := repo.EnqueueChild(ctx, scope, task.ID, task.Version, tcChildInput())
	require.NoError(t, err)
	require.Equal(t, child.ID, replay.ID)
	changed := tcChildInput()
	changed.Payload = []byte(`{"input_ref":"different:1"}`)
	_, err = repo.EnqueueChild(ctx, scope, task.ID, task.Version, changed)
	require.ErrorIs(t, err, d.ErrConflict)
	event := tcEvent(t, repo, scope, child.ID)
	require.Equal(t, child.OperationKey, event.OperationKey)
	page, err := repo.ListOutbox(ctx, scope, d.OutboxFilter{Status: d.OutboxPending})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	task = tcRefresh(t, repo, scope, task.ID)
	_, err = repo.AppendPlan(ctx, scope, task.ID, task.Version, []byte(`{"nodes":[]}`))
	require.ErrorIs(t, err, d.ErrConflict)
}

func TestTaskControlIntegrationCancelBeforeDispatchAndScopeFences(t *testing.T) {
	repo, _, scope := taskControlIT(t)
	ctx := context.Background()
	task := tcReady(t, repo, scope)
	child, err := repo.EnqueueChild(ctx, scope, task.ID, task.Version, tcChildInput())
	require.NoError(t, err)
	event := tcEvent(t, repo, scope, child.ID)
	foreign := d.Scope{TenantID: scope.TenantID, UserID: scope.UserID + 1}
	_, err = repo.GetChild(ctx, foreign, child.ID)
	require.ErrorIs(t, err, d.ErrNotFound)
	_, err = repo.ListChildren(ctx, foreign, task.ID)
	require.ErrorIs(t, err, d.ErrNotFound)
	_, err = repo.EnqueueChild(ctx, foreign, task.ID, 1, tcChildInput())
	require.ErrorIs(t, err, d.ErrNotFound)
	_, err = repo.ClaimOutbox(ctx, foreign, event.ID, event.Version)
	require.ErrorIs(t, err, d.ErrNotFound)
	_, err = repo.MarkDispatchUnknown(ctx, foreign, event.ID, event.Version, "lost")
	require.ErrorIs(t, err, d.ErrNotFound)
	_, err = repo.BindChild(ctx, foreign, child.ID, child.Version, 1, d.Binding{DomainID: "real-id", Receipt: []byte(`{}`)})
	require.ErrorIs(t, err, d.ErrNotFound)
	_, err = repo.UpdateTaskStatus(ctx, foreign, task.ID, 1, d.TaskUpdate{Status: d.StatusReady})
	require.ErrorIs(t, err, d.ErrNotFound)
	page, err := repo.ListOutbox(ctx, foreign, d.OutboxFilter{})
	require.NoError(t, err)
	require.Empty(t, page.Items)
	task = tcRefresh(t, repo, scope, task.ID)
	task, err = repo.RequestCancel(ctx, scope, task.ID, task.Version)
	require.NoError(t, err)
	require.Equal(t, d.StatusCancelled, task.Status)
	event = tcEvent(t, repo, scope, child.ID)
	require.Equal(t, d.OutboxCancelled, event.Status)
	_, err = repo.ClaimOutbox(ctx, scope, event.ID, event.Version)
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = repo.BindChild(ctx, scope, child.ID, child.Version, 1, d.Binding{DomainID: "not-submitted", Receipt: []byte(`{}`)})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = repo.UpdateTaskStatus(ctx, scope, task.ID, task.Version, d.TaskUpdate{Status: d.StatusCompleted})
	require.ErrorIs(t, err, d.ErrConflict)
	again, err := repo.RequestCancel(ctx, scope, task.ID, 1)
	require.NoError(t, err)
	require.Equal(t, task.Version, again.Version)
}

func TestTaskControlIntegrationUnknownCommitCancellationAndLateReceipt(t *testing.T) {
	repo, _, scope := taskControlIT(t)
	ctx := context.Background()
	task := tcReady(t, repo, scope)
	child, err := repo.EnqueueChild(ctx, scope, task.ID, task.Version, tcChildInput())
	require.NoError(t, err)
	event := tcEvent(t, repo, scope, child.ID)
	event, err = repo.ClaimOutbox(ctx, scope, event.ID, event.Version)
	require.NoError(t, err)
	require.Equal(t, d.OutboxDispatching, event.Status)
	_, err = repo.ClaimOutbox(ctx, scope, event.ID, event.Version)
	require.ErrorIs(t, err, d.ErrConflict)
	event, err = repo.MarkDispatchUnknown(ctx, scope, event.ID, event.Version, "submit response lost")
	require.NoError(t, err)
	task = tcRefresh(t, repo, scope, task.ID)
	require.Equal(t, d.StatusReconcileRequired, task.Status)
	_, err = repo.UpdateTaskStatus(ctx, scope, task.ID, task.Version, d.TaskUpdate{Status: d.StatusCompleted})
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = repo.ClaimOutbox(ctx, scope, event.ID, event.Version)
	require.ErrorIs(t, err, d.ErrConflict)
	task, err = repo.RequestCancel(ctx, scope, task.ID, task.Version)
	require.NoError(t, err)
	require.Equal(t, d.StatusCancelRequested, task.Status)
	input := tcChildInput()
	input.Attempt = 2
	_, err = repo.EnqueueChild(ctx, scope, task.ID, task.Version, input)
	require.ErrorIs(t, err, d.ErrConflict)
	child, err = repo.GetChild(ctx, scope, child.ID)
	require.NoError(t, err)
	binding := d.Binding{DomainID: "existing-domain-run", Receipt: []byte(`{"committed":true,"receipt_id":"published-existing"}`)}
	_, err = repo.BindChild(ctx, scope, child.ID, child.Version, 2, binding)
	require.ErrorIs(t, err, d.ErrConflict)
	_, err = repo.BindChild(ctx, scope, child.ID, 1, 1, binding)
	require.ErrorIs(t, err, d.ErrConflict)
	bound, err := repo.BindChild(ctx, scope, child.ID, child.Version, 1, binding)
	require.NoError(t, err)
	require.Equal(t, d.ChildBound, bound.Status)
	retry, err := repo.BindChild(ctx, scope, child.ID, 1, 1, binding)
	require.NoError(t, err)
	require.Equal(t, bound.Version, retry.Version)
	binding.DomainID = "different-run"
	_, err = repo.BindChild(ctx, scope, child.ID, bound.Version, 1, binding)
	require.ErrorIs(t, err, d.ErrConflict)
	binding.DomainID = bound.DomainID
	binding.Receipt = []byte(`{"committed":false}`)
	_, err = repo.BindChild(ctx, scope, child.ID, bound.Version, 1, binding)
	require.ErrorIs(t, err, d.ErrConflict)
	task = tcRefresh(t, repo, scope, task.ID)
	require.Equal(t, d.StatusCancelRequested, task.Status)
	_, err = repo.UpdateTaskStatus(ctx, scope, task.ID, task.Version, d.TaskUpdate{Status: d.StatusCompleted})
	require.ErrorIs(t, err, d.ErrConflict)
	event = tcEvent(t, repo, scope, child.ID)
	require.Equal(t, d.OutboxDelivered, event.Status)
	persisted, err := repo.GetChild(ctx, scope, child.ID)
	require.NoError(t, err)
	require.Contains(t, string(persisted.Receipt), "published-existing")
	// Reopening a repository models process restart: persisted dispatch is delivered,
	// not reset to pending, and the parent cannot silently resume after cancellation.
	recovered := NewTaskControlRepository(repo.db)
	page, err := recovered.ListOutbox(ctx, scope, d.OutboxFilter{Status: d.OutboxPending})
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.Equal(t, d.StatusCancelRequested, tcRefresh(t, recovered, scope, task.ID).Status)
}
