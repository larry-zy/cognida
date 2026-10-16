//go:build integration

package collectionadapter_test

import (
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	col "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	tcmodel "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/collection"
	"cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	ca "cognida/internal/service/taskcontrol/collectionadapter"
	ga "cognida/internal/service/taskcontrol/governanceadapter"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These tests exercise real MySQL, local immutable objects, domain workers and
// HTTP authorization. Fetch/compute and delivery are explicit deterministic
// fixtures, not a claim of live HTTPS/Python/Redis end-to-end acceptance.
type deliveryQueue struct {
	job.Queue
	mu      sync.Mutex
	entries map[string]job.Job
	fail    atomic.Bool
}

func (q *deliveryQueue) Enqueue(_ context.Context, j job.Job) error {
	if q.fail.Load() {
		return errors.New("queue unavailable")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries[j.ID] = j
	return nil
}
func (q *deliveryQueue) get(id string) job.Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.entries[id]
}

type fixtureFetch struct {
	calls atomic.Int32
	fn    func(context.Context, col.FetchRequest) (*col.FetchResponse, error)
}

func (f *fixtureFetch) Fetch(ctx context.Context, r col.FetchRequest) (*col.FetchResponse, error) {
	f.calls.Add(1)
	if f.fn != nil {
		return f.fn(ctx, r)
	}
	return &col.FetchResponse{CanonicalURL: r.URL, StatusCode: 200, ContentType: "text/plain", Body: []byte("sales evidence 2026")}, nil
}

type fixtureCompute struct {
	calls atomic.Int32
	fn    func(context.Context) (lifecycle.ComputeResult, error)
}

func (f *fixtureCompute) Call(ctx context.Context, _ string, _ l.Input, _ map[string]any, _ string) (lifecycle.ComputeResult, error) {
	f.calls.Add(1)
	if f.fn != nil {
		return f.fn(ctx)
	}
	return lifecycle.ComputeResult{SchemaVersion: "1", Method: "rules", Result: map[string]any{"tags": map[string]any{"value": []string{"sales"}, "evidence": []string{"sales"}}, "category": map[string]any{"value": "sales"}}}, nil
}

type fixture struct {
	db      *gorm.DB
	p       l.Principal
	q       *deliveryQueue
	fetch   *fixtureFetch
	compute *fixtureCompute
	cs      *collection.Service
	gs      *governance.Service
	assets  *lifecycle.Service
	ca      *ca.Adapter
	tag     *ga.Adapter
	class   *ga.Adapter
	revoked atomic.Bool
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_TASKCONTROL_TEST_DSN not set")
	}
	cfg, e := sqlmysql.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	var migration struct {
		Version uint
		Dirty   bool
	}
	require.NoError(t, db.Table("schema_migrations").First(&migration).Error)
	require.GreaterOrEqual(t, migration.Version, uint(25))
	require.False(t, migration.Dirty)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 713}, q: &deliveryQueue{entries: map[string]job.Job{}}, fetch: &fixtureFetch{}, compute: &fixtureCompute{}}
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_assets", "governance_records", "collection_tasks"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	auth := func(ctx context.Context, p l.Principal, _ string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if f.revoked.Load() || p != f.p {
			return l.ErrForbidden
		}
		return nil
	}
	origin := func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		if strings.HasPrefix(ref.ProducerID, "collection-governed:") {
			return f.cs.ValidateGovernedOrigin(ctx, p, ref)
		}
		if strings.HasPrefix(ref.ProducerID, "governance-governed:") {
			return f.gs.ValidateGovernedOrigin(ctx, p, ref)
		}
		return l.ErrForbidden
	}
	f.assets, e = lifecycle.NewService(repo.NewLifecycleRepository(db), store, f.q, lifecycle.Config{Authorize: auth, ValidateGovernedOrigin: origin})
	require.NoError(t, e)
	f.cs = collection.NewService(repo.NewCollectionRepository(db), f.fetch, nil, collection.TextNormalizer{})
	f.cs.SetArtifactStore(store)
	f.cs.SetRunSubmitter(collection.JobSubmitter{Queue: f.q})
	f.ca, e = ca.New(f.cs, f.assets)
	require.NoError(t, e)
	f.gs = governance.NewService(repo.NewGovernanceRepository(db), f.assets, f.compute, f.q, auth)
	f.tag, e = ga.New(f.gs, f.assets, "governance.tagging")
	require.NoError(t, e)
	f.class, e = ga.New(f.gs, f.assets, "governance.classification")
	require.NoError(t, e)
	return f
}
func (f *fixture) input(t *testing.T, input l.Input) l.DataAssetRef {
	t.Helper()
	ref, e := f.assets.ImportWithKey(context.Background(), f.p, input, "synthetic domain fixture", uuid.NewString())
	require.NoError(t, e)
	return ref
}
func command(t *testing.T, f *fixture, a interface{ Capability() tc.Capability }, ref *l.DataAssetRef) tc.ExecutionCommand {
	t.Helper()
	cap := a.Capability()
	c := tc.ExecutionCommand{Scope: tcmodel.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}, TaskID: uuid.NewString(), PlanVersion: 1, Attempt: 1, Deadline: time.Now().UTC().Add(3 * time.Minute), InputRefs: []l.RefID{}, Node: tc.Node{ID: "node", Capability: cap.ID, CapabilityVersion: cap.Version, Inputs: []tc.InputBinding{}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: cap.Execution.BaseCalls, TimeoutSeconds: 120}}
	if ref == nil {
		c.Node.Parameters = &tc.NodeParameters{Collection: &tc.CollectionParameters{URL: "https://example.com/synthetic", MaxResponseBytes: 2048}}
	} else {
		c.Node.Parameters = &tc.NodeParameters{Governance: &tc.GovernanceParameters{Labels: []string{"sales", "legal"}}}
		c.InputRefs = []l.RefID{ref.RefID}
		c.Node.Inputs = []tc.InputBinding{{Asset: &ref.RefID, Kind: ref.Kind}}
		c.Node.MaxCallsPerAttempt++
	}
	seal(&c)
	return c
}
func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = tcmodel.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = tcmodel.CanonicalJSON(raw, '{', tcmodel.MaxSnapshotBytes)
	c.PayloadHash = tcmodel.Digest(raw)
}
func (f *fixture) runCollection(t *testing.T, h *tc.ExecutionHandle) {
	t.Helper()
	require.NoError(t, f.cs.ExecuteRun(context.Background(), collection.RunJobPayload{TenantID: f.p.TenantID, TaskID: h.DomainID, RunID: h.DomainID}))
}
func (f *fixture) runGovernance(t *testing.T, h *tc.ExecutionHandle) {
	t.Helper()
	require.NoError(t, f.gs.Handle(context.Background(), f.q.get(h.DomainID)))
}
func (f *fixture) http(t *testing.T, user int64, path, method string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", f.p.TenantID); c.Set("user_id", user); c.Next() })
	h := handler.NewCollectionHandler(f.cs, repo.NewCollectionRepository(f.db), nil)
	router.GET("/collection", h.List)
	router.GET("/collection/:id", h.Get)
	router.GET("/collection-runs/:id", h.GetRun)
	router.POST("/collection/:id/start", h.Start)
	router.POST("/collection/:id/cancel", h.Cancel)
	handler.NewGovernanceHandler(f.gs).Register(router.Group("/governance"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}
func TestGovernedCollectionIntegrationDurableSnapshotAndHTTP(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := command(t, f, f.ca, nil)
	f.q.fail.Store(true)
	_, e := f.ca.Submit(ctx, c)
	require.Error(t, e)
	h, e := f.ca.Lookup(ctx, c)
	require.NoError(t, e)
	require.NotNil(t, h, "SQL committed even though delivery receipt was lost")
	f.q.fail.Store(false)
	again, e := f.ca.Submit(ctx, c)
	require.NoError(t, e)
	require.Equal(t, h, again)
	changed := c
	changed.Node.Parameters = &tc.NodeParameters{Collection: &tc.CollectionParameters{URL: "https://example.com/other", MaxResponseBytes: 2048}}
	seal(&changed)
	_, e = f.ca.Submit(ctx, changed)
	require.ErrorIs(t, e, tcmodel.ErrConflict)
	f.runCollection(t, h)
	observed, e := f.ca.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "succeeded", observed.Status)
	require.Len(t, observed.OutputRefs, 2)
	for _, id := range observed.OutputRefs {
		ref, raw, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, id)
		require.NoError(t, e)
		if ref.Kind == l.RawAsset {
			var input l.Input
			require.NoError(t, json.Unmarshal(raw, &input))
			require.Equal(t, "sales evidence 2026", input.Text)
		} else {
			var manifest map[string]any
			require.NoError(t, json.Unmarshal(raw, &manifest))
			require.Equal(t, false, manifest["knowledge_published"])
			require.Equal(t, h.DomainID, manifest["run_id"])
		}
		_, _, e = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID+1, id)
		require.Error(t, e)
	}
	// Delivery replay must neither fetch again nor create another domain attempt.
	f.runCollection(t, h)
	require.EqualValues(t, 1, f.fetch.calls.Load())
	var tasks, runs, docs int64
	require.NoError(t, f.db.Table("collection_tasks").Where("tenant_id=?", f.p.TenantID).Count(&tasks).Error)
	require.NoError(t, f.db.Table("collection_runs").Where("tenant_id=?", f.p.TenantID).Count(&runs).Error)
	require.NoError(t, f.db.Table("knowledge").Where("tenant_id=?", f.p.TenantID).Count(&docs).Error)
	require.EqualValues(t, 1, tasks)
	require.EqualValues(t, 1, runs)
	require.Zero(t, docs)
	require.Equal(t, 200, f.http(t, f.p.UserID, "/collection/"+h.DomainID, "GET").Code)
	require.Equal(t, 403, f.http(t, f.p.UserID+1, "/collection/"+h.DomainID, "GET").Code)
	require.NotContains(t, f.http(t, f.p.UserID+1, "/collection", "GET").Body.String(), h.DomainID)
	require.Equal(t, 409, f.http(t, f.p.UserID, "/collection/"+h.DomainID+"/start", "POST").Code)
	require.Equal(t, 409, f.http(t, f.p.UserID, "/collection/"+h.DomainID+"/cancel", "POST").Code)
	stopped, e := f.ca.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "succeeded", stopped.Status)
	require.Equal(t, observed.OutputRefs, stopped.OutputRefs)
	f.revoked.Store(true)
	_, e = f.ca.Inspect(ctx, c, *h)
	require.ErrorIs(t, e, tcmodel.ErrForbidden)
	stopped, e = f.ca.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "succeeded", stopped.Status)
	require.Empty(t, stopped.OutputRefs)
}
func TestGovernedCollectionIntegrationCancelAndConcurrentSubmit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := command(t, f, f.ca, nil)
	h, e := f.ca.CancelSubmission(ctx, c)
	require.NoError(t, e)
	again, e := f.ca.Submit(ctx, c)
	require.NoError(t, e)
	require.Equal(t, h, again)
	f.runCollection(t, h)
	out, e := f.ca.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "cancelled", out.Status)
	require.Zero(t, f.fetch.calls.Load())
	c = command(t, f, f.ca, nil)
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, e := f.ca.Submit(ctx, c)
			errs <- e
			if h != nil {
				ids <- h.DomainID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	id := ""
	for got := range ids {
		if id == "" {
			id = got
		}
		require.Equal(t, id, got)
	}
	h, e = f.ca.Lookup(ctx, c)
	require.NoError(t, e)
	started, release := make(chan struct{}), make(chan struct{})
	f.fetch.fn = func(ctx context.Context, r col.FetchRequest) (*col.FetchResponse, error) {
		close(started)
		<-release
		return &col.FetchResponse{CanonicalURL: r.URL, StatusCode: 200, ContentType: "text/plain", Body: []byte("late content")}, nil
	}
	done := make(chan error, 1)
	go func() {
		done <- f.cs.ExecuteRun(ctx, collection.RunJobPayload{TenantID: f.p.TenantID, TaskID: h.DomainID, RunID: h.DomainID})
	}()
	<-started
	out, e = f.ca.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "cancelled", out.Status)
	close(release)
	require.NoError(t, <-done)
	out, e = f.ca.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Empty(t, out.OutputRefs)
	var assets int64
	require.NoError(t, f.db.Table("lifecycle_assets").Where("tenant_id=?", f.p.TenantID).Count(&assets).Error)
	require.Zero(t, assets)
}

func TestCollectionLegacyEntryRealHTTPAlwaysCandidates(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	task := &col.Task{ID: uuid.NewString(), TenantID: f.p.TenantID, UserID: f.p.UserID, KnowledgeBaseID: "target-kb", IdempotencyKey: uuid.NewString(), Limits: col.Limits{MaxSources: 1, MaxResponseBytes: 2048}, Status: col.TaskStatusDraft, CreatedAt: now, UpdatedAt: now}
	_, e := f.cs.Create(ctx, task, []*col.Source{{ID: uuid.NewString(), TaskID: task.ID, URL: "https://example.com/source", CreatedAt: now}})
	require.NoError(t, e)
	// Even a pre-migration task mode may only create candidate runs now.
	require.NoError(t, f.db.Table("collection_tasks").Where("id=? AND tenant_id=?", task.ID, f.p.TenantID).Update("mode", "knowledge").Error)
	for attempt := 0; attempt < 2; attempt++ {
		require.Equal(t, 200, f.http(t, f.p.UserID, "/collection/"+task.ID+"/start", "POST").Code)
		run, e := repo.NewCollectionRepository(f.db).FindLatestRun(ctx, f.p.TenantID, task.ID)
		require.NoError(t, e)
		require.True(t, run.CandidateOnly)
		require.NoError(t, f.cs.ExecuteRun(ctx, collection.RunJobPayload{TenantID: f.p.TenantID, TaskID: task.ID, RunID: run.ID}))
		items, e := repo.NewCollectionRepository(f.db).ListItems(ctx, f.p.TenantID, run.ID)
		require.NoError(t, e)
		require.Len(t, items, 1)
		require.Equal(t, col.ItemStatusCandidate, items[0].Status)
		require.Empty(t, items[0].KnowledgeID)
		require.NotEmpty(t, items[0].NormalizedObjectKey)
		response := f.http(t, f.p.UserID, "/collection/"+task.ID, "GET")
		require.Equal(t, 200, response.Code)
		require.Contains(t, response.Body.String(), `"candidates":1`)
		exact := f.http(t, f.p.UserID, "/collection-runs/"+run.ID, "GET")
		require.Equal(t, 200, exact.Code)
		require.Contains(t, exact.Body.String(), run.ID)
		require.Equal(t, 403, f.http(t, f.p.UserID+1, "/collection-runs/"+run.ID, "GET").Code)
	}
	var count int64
	require.NoError(t, f.db.Table("knowledge").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.Zero(t, count, "collection never bypasses publication approval")
	require.Equal(t, 403, f.http(t, f.p.UserID+1, "/collection/"+task.ID+"/start", "POST").Code)
}

func TestGovernedCollectionInspectRechecksRevocationBeforeOutput(t *testing.T) {
	f := setup(t)
	c := command(t, f, f.ca, nil)
	h, e := f.ca.Submit(context.Background(), c)
	require.NoError(t, e)
	f.revoked.Store(true)
	_, e = f.ca.Inspect(context.Background(), c, *h)
	require.ErrorIs(t, e, tcmodel.ErrForbidden)
	observed, e := f.ca.Cancel(context.Background(), c, *h)
	require.NoError(t, e)
	require.Equal(t, tcmodel.DomainCancelled, observed.Status)
	require.Empty(t, observed.OutputRefs)
}
func TestGovernedCollectionDatabaseDeadlineRejectsLateCompletion(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := tcmodel.Digest([]byte(uuid.NewString()))
	in := col.GovernedInvocation{OperationKey: key, PayloadHash: tcmodel.Digest([]byte("fixture")), Profile: col.SnapshotProfile, URL: "https://example.com/fixture", MaxResponseBytes: 2048, Deadline: time.Now().UTC().Add(-time.Second), TimeoutSeconds: 120, CallsReserved: 3}
	repository := repo.NewCollectionRepository(f.db)
	run, e := repository.SubmitGoverned(ctx, f.p, in, false)
	require.NoError(t, e)
	done, e := repository.FinishGoverned(ctx, f.p, run.ID, col.RunStatusCompleted, []l.RefID{{AssetID: uuid.NewString(), Version: 1}}, "")
	require.NoError(t, e)
	require.Equal(t, col.RunStatusFailed, done.Status)
	require.Equal(t, "deadline_exceeded", done.ErrorMessage)
	require.Empty(t, done.OutputRefs)
}
