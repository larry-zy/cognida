//go:build integration

package lifecycleadapter

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	repository "cognida/internal/repository/mysql"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These are real database/object/queue integration tests. Compute deliberately
// uses a deterministic fixture; real Python/API acceptance is a separate gate.
func integrationFixture(t *testing.T) (*lifecycle.Service, *gorm.DB, *job.RedisQueue, *objects.LocalStore, d.Scope, l.DataAssetRef) {
	t.Helper()
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated COGNIDA_TASKCONTROL_TEST_DSN required")
	}
	parsed, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(parsed.DBName, "cognida_review_") || strings.HasSuffix(parsed.DBName, "_test"), "only an explicitly isolated review/test database is allowed")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	scope := d.Scope{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 791}
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", scope.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	addr := os.Getenv("COGNIDA_TASKCONTROL_TEST_REDIS_ADDR")
	require.NotEmpty(t, addr, "explicit isolated Redis address required")
	client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("COGNIDA_TASKCONTROL_TEST_REDIS_PASSWORD")})
	require.NoError(t, client.Ping(context.Background()).Err())
	prefix := "cognida:governed-adapter-test:" + uuid.NewString()
	t.Cleanup(func() {
		var cursor uint64
		for {
			keys, next, e := client.Scan(context.Background(), cursor, prefix+":*", 100).Result()
			require.NoError(t, e)
			if len(keys) > 0 {
				require.NoError(t, client.Del(context.Background(), keys...).Err())
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		require.NoError(t, client.Close())
	})
	q := job.NewRedisQueueWithPrefix(client, prefix)
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	service, e := lifecycle.NewService(repository.NewLifecycleRepository(db), store, q, lifecycle.Config{Compute: adapterCompute{}, Authorize: func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	ref, e := service.ImportWithKey(context.Background(), principal(scope), l.Input{Kind: "text", Text: "durable controlled upload"}, "fixture", "fixture-"+uuid.NewString())
	require.NoError(t, e)
	return service, db, q, store, scope, ref
}
func integrationCommand(a *Adapter, scope d.Scope, ref l.DataAssetRef) tc.ExecutionCommand {
	cmd := tc.ExecutionCommand{Scope: scope, TaskID: uuid.NewString(), PlanVersion: 1, Attempt: 1, InputRefs: []l.RefID{ref.RefID}, Deadline: time.Now().Add(time.Minute), RequestID: "governed-integration"}
	cmd.Node = tc.Node{ID: "quality", Capability: a.capability.ID, CapabilityVersion: a.capability.Version, Tools: a.capability.Tools, Permissions: a.capability.Permissions, Inputs: []tc.InputBinding{{Asset: &ref.RefID, Kind: ref.Kind}}, OutputKinds: []l.Kind{l.QualityReport}, MaxAttempts: 2, MaxCallsPerAttempt: 2, TimeoutSeconds: 30}
	bindCommand(&cmd)
	return cmd
}
func drainOwnQueue(t *testing.T, s *lifecycle.Service, q *job.RedisQueue) {
	t.Helper()
	for i := 0; i < 20; i++ {
		j, e := q.TryClaim(context.Background())
		require.NoError(t, e)
		if j == nil {
			return
		}
		require.NoError(t, s.Handle(context.Background(), *j))
		require.NoError(t, q.Ack(context.Background(), *j))
	}
	t.Fatal("unexpected unbounded queue")
}

func TestGovernedAdapterMySQLRedisIdempotencyRecoveryAndRevocation(t *testing.T) {
	service, db, q, persistedStore, scope, ref := integrationFixture(t)
	ctx := context.Background()
	a, e := New(service, "lifecycle.data-quality")
	require.NoError(t, e)
	cmd := integrationCommand(a, scope, ref)
	type result struct {
		h *tc.ExecutionHandle
		e error
	}
	results := make(chan result, 8)
	for i := 0; i < 8; i++ {
		go func() { h, e := a.Submit(ctx, cmd); results <- result{h, e} }()
	}
	var first *tc.ExecutionHandle
	for i := 0; i < 8; i++ {
		got := <-results
		require.NoError(t, got.e)
		if first == nil {
			first = got.h
		}
		require.Equal(t, first, got.h)
	}
	var count int64
	require.NoError(t, db.Table("lifecycle_stages").Where("tenant_id=?", scope.TenantID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	size, e := q.ReadySize(ctx)
	require.NoError(t, e)
	require.EqualValues(t, 1, size)
	// Construct a fresh adapter and repository-backed service boundary; no local
	// idempotency map is carried over from Submit.
	freshService, e := lifecycle.NewService(repository.NewLifecycleRepository(db), persistedStore, q, lifecycle.Config{Compute: adapterCompute{}, Authorize: func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	recovered, e := New(freshService, "lifecycle.data-quality")
	require.NoError(t, e)
	found, e := recovered.Lookup(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, first, found)
	drainOwnQueue(t, freshService, q)
	ob, e := recovered.Inspect(ctx, cmd, *found)
	require.NoError(t, e)
	require.Equal(t, d.DomainSucceeded, ob.Status)
	require.Equal(t, 2, ob.CallsReserved)
	require.NotEmpty(t, ob.OutputRefs)
	visible, e := recovered.Cancel(ctx, cmd, *found)
	require.NoError(t, e)
	require.Equal(t, d.DomainSucceeded, visible.Status)
	require.Equal(t, ob.OutputRefs, visible.OutputRefs)
	for _, id := range ob.OutputRefs {
		asset, body, e := service.Resolve(ctx, scope.TenantID, scope.UserID, id)
		require.NoError(t, e)
		require.NotEmpty(t, body)
		require.Equal(t, scope.TenantID, asset.TenantID)
	}
	changed := cmd
	changed.Node.TimeoutSeconds++
	bindCommand(&changed)
	_, e = a.Submit(ctx, changed)
	require.ErrorIs(t, e, d.ErrConflict)
	wrong := cmd
	wrong.Scope.UserID++
	bindCommand(&wrong)
	_, e = a.Inspect(ctx, wrong, *first)
	require.ErrorIs(t, e, d.ErrConflict)
	wrong = cmd
	wrong.Scope.TenantID++
	bindCommand(&wrong)
	_, e = a.Submit(ctx, wrong)
	require.ErrorIs(t, e, d.ErrForbidden)
	require.NoError(t, db.Table("lifecycle_assets").Where("tenant_id=? AND asset_id=? AND version=?", scope.TenantID, ref.AssetID, ref.Version).Update("body", gorm.Expr("JSON_SET(body, '$.status', 'disabled')")).Error)
	_, e = a.Inspect(ctx, cmd, *first)
	require.ErrorIs(t, e, d.ErrForbidden)
	found, e = a.Lookup(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, first, found)
	cancelled, e := a.Cancel(ctx, cmd, *first)
	require.NoError(t, e)
	require.Equal(t, d.DomainSucceeded, cancelled.Status)
	require.Empty(t, cancelled.OutputRefs)
}

type createBarrier struct {
	l.Repository
	arrived *sync.WaitGroup
}

func (r createBarrier) CreateStage(ctx context.Context, st *l.StageRun) error {
	r.arrived.Done()
	r.arrived.Wait()
	return r.Repository.CreateStage(ctx, st)
}

func TestGovernedAdapterMySQLRedisUnknownCancellationFencesLateSubmit(t *testing.T) {
	service, db, q, _, scope, ref := integrationFixture(t)
	ctx := context.Background()
	// Only record creation is synchronized. Both clients first observe absence;
	// the actual MySQL unique constraint decides which concurrent insert wins.
	barrier := &sync.WaitGroup{}
	barrier.Add(2)
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	// Reuse an upload made in this store so all validation reads real bytes.
	concurrent, e := lifecycle.NewService(createBarrier{Repository: service.Repository(), arrived: barrier}, store, q, lifecycle.Config{Compute: adapterCompute{}, Authorize: func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	ref, e = concurrent.ImportWithKey(ctx, principal(scope), l.Input{Kind: "text", Text: "cancel race"}, "race", "race-"+uuid.NewString())
	require.NoError(t, e)
	a, e := New(concurrent, "lifecycle.data-quality")
	require.NoError(t, e)
	cmd := integrationCommand(a, scope, ref)
	errs := make(chan error, 2)
	go func() { _, e := a.Submit(ctx, cmd); errs <- e }()
	go func() { _, e := a.CancelSubmission(ctx, cmd); errs <- e }()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	h, e := a.Lookup(ctx, cmd)
	require.NoError(t, e)
	require.NotNil(t, h)
	cancelled, e := a.Cancel(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainCancelled, cancelled.Status)
	late, e := a.Submit(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, h, late)
	drainOwnQueue(t, concurrent, q)
	var records []struct{ Body json.RawMessage }
	require.NoError(t, db.Table("lifecycle_stages").Select("body").Where("tenant_id=?", scope.TenantID).Find(&records).Error)
	require.Len(t, records, 1)
	var st l.StageRun
	require.NoError(t, json.Unmarshal(records[0].Body, &st))
	require.Equal(t, l.StatusCancelled, st.Status)
	require.Zero(t, st.Attempt)
	// No source access is needed to persist a cancellation tombstone even after
	// an unknown submit's absolute deadline has elapsed.
	cmd.TaskID = uuid.NewString()
	cmd.Deadline = time.Now().Add(-time.Minute)
	bindCommand(&cmd)
	plain, e := New(service, "lifecycle.data-quality")
	require.NoError(t, e)
	h, e = plain.CancelSubmission(ctx, cmd)
	require.NoError(t, e)
	require.NotNil(t, h)
	ob, e := plain.Cancel(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainCancelled, ob.Status)
	require.Empty(t, ob.OutputRefs)
}
