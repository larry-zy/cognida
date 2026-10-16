//go:build integration

package governedops_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/governedops"
	"cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type queue struct {
	job.Queue
	mu   sync.Mutex
	jobs map[string]job.Job
}

func (q *queue) Enqueue(_ context.Context, j job.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[j.ID] = j
	return nil
}
func (q *queue) get(id string) job.Job { q.mu.Lock(); defer q.mu.Unlock(); return q.jobs[id] }

type fixture struct {
	db     *gorm.DB
	p      l.Principal
	s      *svc.Service
	repo   d.Repository
	assets *lifecycle.Service
	q      *queue
	router *gin.Engine
	revoke atomic.Bool
	ref    l.RefID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL required")
	}
	config, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(config.DBName, "cognida_review_"))
	require.NotContains(t, config.DBName, "e2e")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 618}, q: &queue{jobs: map[string]job.Job{}}}
	f.repo = repo.NewGovernedOperationRepository(db)
	t.Cleanup(func() {
		var ids []string
		require.NoError(t, db.Table("governed_operation_jobs").Where("tenant_id=?", f.p.TenantID).Pluck("id", &ids).Error)
		for _, id := range ids {
			sum := sha256.Sum256([]byte(id))
			name := "cognida_op_" + hex.EncodeToString(sum[:])[:32]
			require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+name+"`").Error)
		}
		for _, table := range []string{"governed_operation_versions", "governed_operation_jobs", "governed_operation_targets", "agent_operation_audit", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	auth := func(_ context.Context, p l.Principal, _ string) error {
		if p != f.p || f.revoke.Load() {
			return l.ErrForbidden
		}
		return nil
	}
	f.assets, e = lifecycle.NewService(repo.NewLifecycleRepository(db), store, f.q, lifecycle.Config{Authorize: auth, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		return f.s.ValidateGovernedOrigin(ctx, p, ref)
	}})
	require.NoError(t, e)
	f.s, e = svc.New(f.repo, f.assets, f.q, auth)
	require.NoError(t, e)
	input := l.Input{Kind: "table", Columns: []string{"code", "amount"}, ColumnTypes: map[string]string{"code": "VARCHAR", "amount": "DECIMAL(38,18)"}, Rows: [][]any{{"00123", "9007199254740993.123456789012345678"}, {"ABC", "2.00"}}}
	asset, e := f.assets.Import(context.Background(), f.p, input, "exact operations input")
	require.NoError(t, e)
	f.ref = asset.RefID
	f.router = gin.New()
	f.router.Use(func(c *gin.Context) {
		if c.GetHeader("X-No-Identity") != "1" {
			c.Set("tenant_id", f.p.TenantID)
			id := f.p.UserID
			if c.GetHeader("X-Other") == "1" {
				id++
			}
			c.Set("user_id", id)
		}
		c.Next()
	})
	handler.NewGovernedOperationHandler(f.s).Register(f.router.Group("/api/v1"))
	return f
}
func (f *fixture) call(method, path string, body any, key, identity string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	if s, ok := body.(string); ok {
		raw = []byte(s)
	}
	r := httptest.NewRequest(method, "/api/v1/operations"+path, strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	if identity != "" {
		r.Header.Set(identity, "1")
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}
func decode(t *testing.T, w *httptest.ResponseRecorder) *d.Job {
	t.Helper()
	require.Equal(t, 200, w.Code, w.Body.String())
	var envelope struct {
		Data d.Job `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	return &envelope.Data
}
func (f *fixture) finish(t *testing.T, j *d.Job) *d.Job {
	t.Helper()
	confirmed := decode(t, f.call("POST", "/jobs/"+j.ID+"/confirm", d.Confirmation{ExpectedVersion: j.Version, PlanHash: j.Plan.Hash, Action: j.Plan.Request.Action}, "", ""))
	require.Equal(t, "queued", confirmed.Status)
	require.NoError(t, f.s.Handle(context.Background(), f.q.get(j.ID)))
	done := decode(t, f.call("GET", "/jobs/"+j.ID, nil, "", ""))
	require.Equal(t, "completed", done.Status)
	require.NotNil(t, done.OutputRef)
	return done
}
func TestOperationIntegrationExactSQLConfirmationAndRollback(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	req := d.Request{Action: "etl", Target: "agent_etl_sales", InputRef: &f.ref}
	planned := decode(t, f.call("POST", "/plans", req, "etl-key", ""))
	require.Equal(t, "planned", planned.Status)
	target, e := f.repo.Target(ctx, f.p, req.Target)
	require.NoError(t, e)
	require.Zero(t, target.Version)
	require.Equal(t, 401, f.call("GET", "/jobs/"+planned.ID, nil, "", "X-No-Identity").Code)
	require.Equal(t, 403, f.call("GET", "/jobs/"+planned.ID, nil, "", "X-Other").Code)
	require.Equal(t, 400, f.call("POST", "/plans", `{"action":"etl","unknown":true}`, "bad", "").Code)
	require.Equal(t, 409, f.call("POST", "/jobs/"+planned.ID+"/confirm", d.Confirmation{ExpectedVersion: planned.Version, PlanHash: strings.Repeat("0", 64), Action: "etl"}, "", "").Code)
	first := f.finish(t, planned)
	require.EqualValues(t, 1, first.CommittedVersion)
	v, e := f.s.Version(ctx, f.p, req.Target, 1)
	require.NoError(t, e)
	require.Equal(t, "9007199254740993.123456789012345678", v.Table.Rows[0][1].Text)
	_, raw, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, *first.OutputRef)
	require.NoError(t, e)
	require.Contains(t, string(raw), `"00123"`)
	require.Contains(t, string(raw), `"9007199254740993.123456789012345678"`)
	update, e := f.s.PrepareAgentSQL(ctx, f.p, "UPDATE agent_etl_sales SET amount=3.00 WHERE code='00123'", "update-key")
	require.NoError(t, e)
	second := f.finish(t, update)
	require.EqualValues(t, 2, second.CommittedVersion)
	v, e = f.s.Version(ctx, f.p, req.Target, 2)
	require.NoError(t, e)
	require.Equal(t, "3.00", v.Table.Rows[0][1].Text)
	replay, e := f.s.PrepareAgentSQL(ctx, f.p, "UPDATE agent_etl_sales SET amount=3.00 WHERE code='00123'", "update-key")
	require.NoError(t, e)
	require.Equal(t, second.ID, replay.ID)
	require.Equal(t, "completed", replay.Status)
	_, e = f.s.PrepareAgentSQL(ctx, f.p, "UPDATE agent_etl_sales SET amount=4 WHERE code='00123'", "update-key")
	require.ErrorIs(t, e, d.ErrConflict)
	rollback := decode(t, f.call("POST", "/plans", d.Request{Action: "rollback", Target: req.Target, ExpectedVersion: 2, RollbackVersion: 1}, "rollback-key", ""))
	third := f.finish(t, rollback)
	require.EqualValues(t, 3, third.CommittedVersion)
	active, e := f.s.Version(ctx, f.p, req.Target, 0)
	require.NoError(t, e)
	require.Equal(t, "9007199254740993.123456789012345678", active.Table.Rows[0][1].Text)
	v, e = f.s.Version(ctx, f.p, req.Target, 2)
	require.NoError(t, e)
	require.Equal(t, "3.00", v.Table.Rows[0][1].Text)
	again := decode(t, f.call("POST", "/plans", req, "etl-key", ""))
	require.Equal(t, first.ID, again.ID)
	var commits int64
	require.NoError(t, f.db.Table("agent_operation_audit").Where("tenant_id=? AND operation_type='governed_commit'", f.p.TenantID).Count(&commits).Error)
	require.EqualValues(t, 3, commits)
	f.revoke.Store(true)
	require.Equal(t, 403, f.call("GET", "/jobs/"+third.ID, nil, "", "").Code)
	cancel := decode(t, f.call("POST", "/jobs/"+third.ID+"/cancel", map[string]any{}, "", ""))
	require.Equal(t, "completed", cancel.Status)
	require.Nil(t, cancel.OutputRef)
	require.Empty(t, cancel.Plan.SourceRefs)
}
func TestOperationIntegrationCancelCommitFenceAndRecovery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		name := "test" + strings.ReplaceAll(uuid.NewString(), "-", "")
		j, e := f.s.Prepare(ctx, f.p, d.Request{Action: "etl", Target: name, InputRef: &f.ref}, uuid.NewString())
		require.NoError(t, e)
		j, e = f.s.Confirm(ctx, f.p, j.ID, d.Confirmation{ExpectedVersion: j.Version, PlanHash: j.Plan.Hash, Action: "etl"})
		require.NoError(t, e)
		claimed, e := f.repo.Claim(ctx, f.p, j.ID)
		require.NoError(t, e)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = f.repo.Commit(ctx, f.p, j.ID, claimed.Epoch) }()
		go func() { defer wg.Done(); _, _ = f.s.Cancel(ctx, f.p, j.ID) }()
		wg.Wait()
		actual, e := f.repo.Get(ctx, f.p, j.ID)
		require.NoError(t, e)
		target, e := f.repo.Target(ctx, f.p, name)
		require.NoError(t, e)
		if actual.Status == "completed" {
			require.EqualValues(t, 1, target.Version)
			same, e := f.repo.Commit(ctx, f.p, j.ID, claimed.Epoch)
			require.NoError(t, e)
			require.Equal(t, actual.CommittedVersionID, same.CommittedVersionID)
		} else {
			require.Equal(t, "cancelled", actual.Status)
			require.Zero(t, target.Version)
			_, e = f.repo.Commit(ctx, f.p, j.ID, claimed.Epoch)
			require.ErrorIs(t, e, d.ErrConflict)
		}
	}
	_, e := f.s.PendingJobs(ctx, 0)
	require.ErrorIs(t, e, d.ErrInvalid)
}
