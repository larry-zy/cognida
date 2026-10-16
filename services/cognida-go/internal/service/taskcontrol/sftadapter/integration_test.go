//go:build integration

package sftadapter_test

import (
	"bytes"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	sa "cognida/internal/service/taskcontrol/sftadapter"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Real MySQL/local object storage/HTTP services; identity, queue delivery and
// cleaning compute are explicit fixtures. No persistent worker or model runs.
type queue struct {
	job.Queue
	mu   sync.Mutex
	jobs map[string]job.Job
	fail atomic.Bool
}

func (q *queue) Enqueue(_ context.Context, j job.Job) error {
	if q.fail.Load() {
		return errors.New("queue unavailable")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[j.ID] = j
	return nil
}
func (q *queue) get(id string) job.Job { q.mu.Lock(); defer q.mu.Unlock(); return q.jobs[id] }

type compute struct{}

func (compute) Call(_ context.Context, op string, in l.Input, _ map[string]any, _ string) (lifecycle.ComputeResult, error) {
	if op == "clean" {
		return lifecycle.ComputeResult{SchemaVersion: "1", Method: "rules", Result: map[string]any{"data": in}}, nil
	}
	return lifecycle.ComputeResult{SchemaVersion: "1", Method: "rules", Result: map[string]any{"overall_score": 100}}, nil
}

type fixture struct {
	db       *gorm.DB
	s        *svc.Service
	assets   *lifecycle.Service
	adapter  *sa.Adapter
	q        *queue
	p        l.Principal
	source   l.DataAssetRef
	standard string
	auth     lifecycle.Authorizer
	revoked  atomic.Bool
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_DOMAIN_TEST_DSN not set")
	}
	cfg, err := sqlmysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 813}, q: &queue{jobs: map[string]job.Job{}}}
	t.Cleanup(func() {
		for _, table := range []string{"governance_records", "lifecycle_events", "lifecycle_stages", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	store, err := objects.NewLocal(t.TempDir())
	require.NoError(t, err)
	f.auth = func(ctx context.Context, p l.Principal, _ string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if f.revoked.Load() || p.TenantID != f.p.TenantID || p.UserID != f.p.UserID {
			return l.ErrForbidden
		}
		return nil
	}
	f.assets, err = lifecycle.NewService(repo.NewLifecycleRepository(db), store, f.q, lifecycle.Config{Authorize: f.auth, Compute: compute{}, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		return f.s.ValidateGovernedOrigin(ctx, p, ref)
	}})
	require.NoError(t, err)
	f.recover(t)
	ctx := context.Background()
	raw, err := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"q", "a", "split"}, Rows: [][]any{{"Why E42?", "Use the documented reset.", "train"}, {"Serial number?", "00123", "test"}}}, "synthetic technical support", uuid.NewString())
	require.NoError(t, err)
	clean, err := f.assets.Invoke(ctx, f.p, "data-cleaning", []l.RefID{raw.RefID}, nil, uuid.NewString(), "fixture")
	require.NoError(t, err)
	require.NoError(t, f.assets.Handle(ctx, f.q.get("lifecycle:"+clean.ID)))
	clean, err = f.assets.Repository().FindStage(ctx, f.p.TenantID, clean.ID, false)
	require.NoError(t, err)
	prepared, err := f.assets.PrepareAsset(ctx, f.p, clean.Output.Refs[0].RefID, l.PreparationOptions{}, uuid.NewString(), "fixture")
	require.NoError(t, err)
	require.NoError(t, f.assets.Handle(ctx, f.q.get("lifecycle:"+prepared.ID)))
	prepared, err = f.assets.Repository().FindStage(ctx, f.p.TenantID, prepared.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, prepared.Status)
	f.source = prepared.Output.Refs[0]
	std, err := f.s.CreateLabelStandard(ctx, f.p, d.LabelStandardInput{Name: "Support QA", TaskType: "qa", Guidelines: "Use only explicit existing reference answers."}, uuid.NewString())
	require.NoError(t, err)
	version, err := f.s.PublishLabelStandard(ctx, f.p, std.ID, std.Version, uuid.NewString())
	require.NoError(t, err)
	f.standard = version.ID
	return f
}
func (f *fixture) recover(t *testing.T) {
	t.Helper()
	f.s = svc.NewService(repo.NewGovernanceRepository(f.db), f.assets, compute{}, f.q, f.auth)
	var e error
	f.adapter, e = sa.New(f.s, f.assets)
	require.NoError(t, e)
}
func (f *fixture) command() tc.ExecutionCommand {
	ref := f.source.RefID
	c := tc.ExecutionCommand{Scope: m.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}, TaskID: uuid.NewString(), PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(2 * time.Minute), InputRefs: []l.RefID{ref}, Node: tc.Node{ID: "sft", Capability: sa.CapabilityID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{FromNode: "prepare", Kind: l.CleanedAsset}}, DependsOn: []string{"prepare"}, OutputKinds: []l.Kind{l.Result}, Tools: []string{sa.CapabilityID}, Permissions: []string{"read"}, MaxAttempts: 1, MaxCallsPerAttempt: 3, TimeoutSeconds: 120, Parameters: &tc.NodeParameters{SFT: &tc.SFTParameters{Name: "Troubleshooting SFT", StandardVersionID: f.standard, Format: "qa", Columns: d.SFTColumns{Question: "q", ReferenceAnswer: "a", Split: "split"}}}}}
	seal(&c)
	return c
}
func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
}
func (f *fixture) state(t *testing.T, id string) (*d.Record, d.GovernedExecution) {
	t.Helper()
	r, e := f.s.Repo.Get(context.Background(), f.p.TenantID, d.GovernedKind, id)
	require.NoError(t, e)
	var s d.GovernedExecution
	require.NoError(t, json.Unmarshal(r.Payload, &s))
	return r, s
}
func (f *fixture) count(t *testing.T, kind string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind=?", f.p.TenantID, kind).Count(&n).Error)
	return n
}
func (f *fixture) http(t *testing.T, user int64, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1")
	g.Use(func(c *gin.Context) { c.Set("tenant_id", f.p.TenantID); c.Set("user_id", user); c.Next() })
	handler.NewSFTHandler(f.s).Register(g)
	handler.NewGovernanceHandler(f.s).Register(g)
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	out := httptest.NewRecorder()
	r.ServeHTTP(out, req)
	return out
}

type hookedAssets struct {
	svc.AssetService
	after func(l.DataAssetRef) error
}

func (a hookedAssets) ImportDerived(ctx context.Context, p l.Principal, k l.Kind, producer string, parents []l.RefID, raw json.RawMessage, policy l.PolicySnapshot) (l.DataAssetRef, error) {
	ref, e := a.AssetService.ImportDerived(ctx, p, k, producer, parents, raw, policy)
	if e == nil && a.after != nil {
		e = a.after(ref)
	}
	return ref, e
}

func TestSFTGovernedMySQLHTTPRecoveryAndHumanBoundary(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := f.command()
	require.NoError(t, f.adapter.Validate(ctx, c.Scope, c.Node))
	f.q.fail.Store(true)
	h, e := f.adapter.Submit(ctx, c)
	require.Error(t, e)
	require.Nil(t, h) // Lost delivery is reconciled through the durable lookup.
	require.Zero(t, f.count(t, d.SFTDatasetKind))
	f.q.fail.Store(false)
	f.recover(t)
	lookup, e := f.adapter.Lookup(ctx, c)
	require.NoError(t, e)
	require.NotNil(t, lookup)
	h = lookup
	created, _ := f.state(t, h.DomainID)
	databaseNow, e := f.s.Repo.(d.GovernedCommitClock).GovernedNow(ctx)
	require.NoError(t, e)
	require.False(t, created.CreatedAt.After(databaseNow))
	require.Less(t, databaseNow.Sub(created.CreatedAt), 5*time.Second)
	queued := f.http(t, f.p.UserID, "GET", "/governance-executions/"+h.DomainID, nil)
	require.Equal(t, 200, queued.Code, queued.Body.String())
	require.Contains(t, queued.Body.String(), `"domain_id":""`)
	jobs, e := f.s.PendingJobs(ctx, 100)
	require.NoError(t, e)
	var work job.Job
	for _, j := range jobs {
		if j.ID == h.DomainID {
			work = j
		}
	}
	require.NotEmpty(t, work.ID)
	// Simulate a crash after the immutable manifest is persisted but before the
	// candidate/receipt transaction. The pre-commit manifest must remain unreadable.
	var uncommitted l.DataAssetRef
	f.s.Assets = hookedAssets{AssetService: f.assets, after: func(ref l.DataAssetRef) error {
		uncommitted = ref
		return errors.New("fixture process stopped before commit")
	}}
	require.Error(t, f.s.Handle(ctx, work))
	require.Zero(t, f.count(t, d.SFTDatasetKind))
	_, _, e = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, uncommitted.RefID)
	require.ErrorIs(t, e, l.ErrForbidden)
	f.recover(t)
	// Two recovered deliveries share the same transaction fence and exact IDs.
	completed := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { completed <- f.s.Handle(ctx, work) }()
	}
	require.NoError(t, <-completed)
	require.NoError(t, <-completed)
	require.NoError(t, f.s.Handle(ctx, work))
	obs, e := f.adapter.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, obs.Status)
	require.Equal(t, "sft_execution", obs.DomainKind)
	require.Len(t, obs.OutputRefs, 1)
	require.Equal(t, uncommitted.RefID, obs.OutputRefs[0])
	require.EqualValues(t, 1, f.count(t, d.SFTDatasetKind))
	r, st := f.state(t, h.DomainID)
	require.Equal(t, "completed", r.Status)
	require.Equal(t, d.SFTDatasetKind, st.DomainKind)
	_, body, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, obs.OutputRefs[0])
	require.NoError(t, e)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(body, &manifest))
	require.Equal(t, st.DomainID, manifest["dataset_id"])
	require.Equal(t, true, manifest["review_required"])
	require.Equal(t, false, manifest["dataset_published"])
	response := f.http(t, f.p.UserID, "GET", "/governance-executions/"+h.DomainID, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), st.DomainID)
	response = f.http(t, f.p.UserID, "GET", "/sft-datasets/"+st.DomainID+"/rows", nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "00123")
	response = f.http(t, f.p.UserID, "POST", "/sft-datasets/"+st.DomainID+"/publish", map[string]any{"expected_version": 1})
	require.Equal(t, 409, response.Code)
	require.Zero(t, f.count(t, d.SFTDatasetVersionKind))
	response = f.http(t, f.p.UserID+1, "GET", "/sft-datasets/"+st.DomainID, nil)
	require.Equal(t, 403, response.Code)
	late, e := f.adapter.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, late.Status)
	require.Equal(t, obs.OutputRefs, late.OutputRefs)
	f.revoked.Store(true)
	_, e = f.adapter.Inspect(ctx, c, *h)
	require.ErrorIs(t, e, m.ErrForbidden)
	lookup, e = f.adapter.Lookup(ctx, c)
	require.NoError(t, e)
	require.Equal(t, h, lookup)
	late, e = f.adapter.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, late.Status)
	require.Empty(t, late.OutputRefs)
	response = f.http(t, f.p.UserID, "GET", "/sft-datasets/"+st.DomainID+"/rows", nil)
	require.Equal(t, 403, response.Code)
}

func (f *fixture) prepared(t *testing.T, input l.Input) l.DataAssetRef {
	t.Helper()
	ctx := context.Background()
	raw, err := f.assets.ImportWithKey(ctx, f.p, input, "SFT negative fixture", uuid.NewString())
	require.NoError(t, err)
	clean, err := f.assets.Invoke(ctx, f.p, "data-cleaning", []l.RefID{raw.RefID}, nil, uuid.NewString(), "fixture")
	require.NoError(t, err)
	require.NoError(t, f.assets.Handle(ctx, f.q.get("lifecycle:"+clean.ID)))
	clean, err = f.assets.Repository().FindStage(ctx, f.p.TenantID, clean.ID, false)
	require.NoError(t, err)
	stage, err := f.assets.PrepareAsset(ctx, f.p, clean.Output.Refs[0].RefID, l.PreparationOptions{}, uuid.NewString(), "fixture")
	require.NoError(t, err)
	require.NoError(t, f.assets.Handle(ctx, f.q.get("lifecycle:"+stage.ID)))
	stage, err = f.assets.Repository().FindStage(ctx, f.p.TenantID, stage.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, stage.Status)
	return stage.Output.Refs[0]
}

func TestSFTGovernedMySQLRejectsUnpreparedAndBlankAnswerWithoutPartialDraft(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// Synthetic support case: the third answer is only whitespace. No row is
	// silently filtered and the two good rows do not become a partial dataset.
	input := l.Input{Kind: "table", Columns: []string{"q", "a", "split"}, Rows: [][]any{{"Documented E42 reset?", "Follow the manual.", "train"}, {"Device code?", "00123", "test"}, {"Undocumented failure?", " \t ", "train"}}}
	bad := f.prepared(t, input)
	c := f.command()
	c.InputRefs[0] = bad.RefID
	seal(&c)
	_, err := f.adapter.Submit(ctx, c)
	require.ErrorIs(t, err, m.ErrInvalid)
	require.Zero(t, f.count(t, d.SFTDatasetKind))
	require.Zero(t, f.count(t, d.GovernedKind))
	// An otherwise valid raw table cannot forge prepared status just by declaring
	// cleaned_asset in the node's input binding.
	input.Rows = input.Rows[:2]
	raw, err := f.assets.ImportWithKey(ctx, f.p, input, "unprepared fixture", uuid.NewString())
	require.NoError(t, err)
	c = f.command()
	c.InputRefs[0] = raw.RefID
	seal(&c)
	_, err = f.adapter.Submit(ctx, c)
	require.ErrorIs(t, err, m.ErrInvalid)
	require.Zero(t, f.count(t, d.GovernedKind))
	// Explicit standard ownership is checked even with a dynamic prepared input.
	c = f.command()
	c.Scope.UserID++
	seal(&c)
	_, err = f.adapter.Submit(ctx, c)
	require.Error(t, err)
	require.Zero(t, f.count(t, d.SFTDatasetKind))
}

func TestSFTGovernedMySQLCancellationFenceAndFailure(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	t.Run("cancel before submit", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.CancelSubmission(ctx, c)
		require.NoError(t, e)
		again, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		require.Equal(t, h, again)
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainCancelled, obs.Status)
		require.Zero(t, f.count(t, d.SFTDatasetKind))
	})
	t.Run("full payload conflict", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		c.Node.Parameters.SFT.Name = "Changed"
		seal(&c)
		_, e = f.adapter.Submit(ctx, c)
		require.ErrorIs(t, e, m.ErrConflict)
		r, _ := f.state(t, h.DomainID)
		require.Equal(t, "queued", r.Status)
	})
	t.Run("cancel after manifest before commit", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		wrong := *h
		wrong.DomainID = uuid.NewString()
		_, e = f.adapter.Cancel(ctx, c, wrong)
		require.ErrorIs(t, e, m.ErrConflict)
		r, _ := f.state(t, h.DomainID)
		require.Equal(t, "queued", r.Status)
		entered, release := make(chan l.DataAssetRef, 1), make(chan struct{})
		f.s.Assets = hookedAssets{AssetService: f.assets, after: func(ref l.DataAssetRef) error { entered <- ref; <-release; return nil }}
		done := make(chan error, 1)
		go func() { done <- f.s.Handle(ctx, f.q.get(h.DomainID)) }()
		var ref l.DataAssetRef
		select {
		case ref = <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not reach manifest")
		}
		obs, e := f.adapter.Cancel(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainCancelled, obs.Status)
		close(release)
		require.NoError(t, <-done)
		require.Zero(t, f.count(t, d.SFTDatasetKind))
		_, _, e = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, ref.RefID)
		require.ErrorIs(t, e, l.ErrForbidden)
		f.recover(t)
	})
	t.Run("revoked before worker", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		f.revoked.Store(true)
		require.NoError(t, f.s.Handle(ctx, f.q.get(h.DomainID)))
		r, st := f.state(t, h.DomainID)
		require.Equal(t, "failed", r.Status)
		require.Empty(t, st.OutputRefs)
		require.Zero(t, f.count(t, d.SFTDatasetKind))
		f.revoked.Store(false)
	})
	t.Run("expired database deadline", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		f.expireDeadline(t, h.DomainID)
		require.NoError(t, f.s.Handle(ctx, f.q.get(h.DomainID)))
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainTimedOut, obs.Status)
		require.Empty(t, obs.OutputRefs)
	})
	t.Run("deadline elapses after manifest before commit", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		f.s.Assets = hookedAssets{AssetService: f.assets, after: func(_ l.DataAssetRef) error {
			f.expireDeadline(t, h.DomainID)
			return nil
		}}
		require.NoError(t, f.s.Handle(ctx, f.q.get(h.DomainID)))
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainTimedOut, obs.Status)
		require.Empty(t, obs.OutputRefs)
		require.Zero(t, f.count(t, d.SFTDatasetKind))
		f.recover(t)
	})
}

func (f *fixture) expireDeadline(t *testing.T, id string) {
	t.Helper()
	_, state := f.state(t, id)
	now, err := f.s.Repo.(d.GovernedCommitClock).GovernedNow(context.Background())
	require.NoError(t, err)
	expired := now.Add(-time.Second)
	state.ExecutionDeadline = &expired
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND id=?", f.p.TenantID, id).Update("payload", raw).Error)
}

func TestSFTGovernedMySQLTimezoneStableDeadline(t *testing.T) {
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_DOMAIN_TEST_DSN not set")
	}
	cfg, err := sqlmysql.ParseDSN(dsn)
	require.NoError(t, err)
	cfg.Loc, err = time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	t.Setenv("COGNIDA_DOMAIN_TEST_DSN", cfg.FormatDSN())
	f := setup(t)
	ctx := context.Background()
	before, err := f.s.Repo.(d.GovernedCommitClock).GovernedNow(ctx)
	require.NoError(t, err)
	c := f.command()
	c.Node.TimeoutSeconds = 30
	seal(&c)
	h, err := f.adapter.Submit(ctx, c)
	require.NoError(t, err)
	_, state := f.state(t, h.DomainID)
	require.NotNil(t, state.ExecutionDeadline)
	require.True(t, state.ExecutionDeadline.Before(before.Add(35*time.Second)))
	require.False(t, state.ExecutionDeadline.Before(before.Add(30*time.Second)))
	f.recover(t)
	require.NoError(t, f.s.Handle(ctx, f.q.get(h.DomainID)))
	obs, err := f.adapter.Inspect(ctx, c, *h)
	require.NoError(t, err)
	require.Equal(t, m.DomainSucceeded, obs.Status)
	_, after := f.state(t, h.DomainID)
	require.Equal(t, state.ExecutionDeadline, after.ExecutionDeadline)
}
