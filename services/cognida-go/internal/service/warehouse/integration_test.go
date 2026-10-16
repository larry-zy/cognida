//go:build integration

package warehouse_test

import (
	"bytes"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	ev "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	lin "cognida/internal/model/lineage"
	m "cognida/internal/model/taskcontrol"
	d "cognida/internal/model/warehouse"
	repo "cognida/internal/repository/mysql"
	govsvc "cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"cognida/internal/service/taskcontrol/governanceadapter"
	"cognida/internal/service/taskcontrol/warehouseadapter"
	svc "cognida/internal/service/warehouse"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type unusedQueue struct{ job.Queue }

func (unusedQueue) Enqueue(context.Context, job.Job) error { return errors.New("unexpected enqueue") }

type capturedLineage struct{ fail atomic.Bool }

func (g *capturedLineage) Upsert(_ context.Context, _ int64, _ string, graph lin.Graph) error {
	if g.fail.Load() {
		return errors.New("lineage unavailable")
	}
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		return errors.New("missing actual lineage")
	}
	return nil
}
func (g *capturedLineage) Query(context.Context, int64, string, string, int) (*lin.Graph, error) {
	return nil, nil
}

type fixture struct {
	s      *svc.Service
	gov    *govsvc.Service
	assets *lifecycle.Service
	r      d.Repository
	db     *gorm.DB
	p      l.Principal
	revoke atomic.Bool
	graph  *capturedLineage
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_DOMAIN_TEST_DSN required")
	}
	cfg, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, e := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 9123}, graph: &capturedLineage{}}
	f.r = repo.NewWarehouseRepository(db)
	sql, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() {
		var ids []string
		require.NoError(t, db.Table("warehouse_models").Where("tenant_id=?", f.p.TenantID).Pluck("id", &ids).Error)
		if len(ids) > 0 {
			require.NoError(t, db.Table("warehouse_model_versions").Where("model_id IN ?", ids).Delete(map[string]any{}).Error)
		}
		for _, table := range []string{"feature_materializations", "feature_values", "warehouse_events", "warehouse_runs", "warehouse_models", "governance_records", "evaluation_dataset_versions", "evaluation_dataset_heads", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	auth := func(_ context.Context, p l.Principal, _ string) error {
		if p != f.p {
			return l.ErrForbidden
		}
		return nil
	}
	f.assets, e = lifecycle.NewService(repo.NewLifecycleRepository(db), objects, unusedQueue{}, lifecycle.Config{Authorize: func(ctx context.Context, p l.Principal, perm string) error {
		if f.revoke.Load() {
			return l.ErrForbidden
		}
		return auth(ctx, p, perm)
	}, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		if strings.HasPrefix(ref.ProducerID, "feature-materialization:") {
			return f.gov.ValidateFeatureMaterializationOrigin(ctx, p, ref)
		}
		return f.s.ValidateGovernedOrigin(ctx, p, ref)
	}})
	require.NoError(t, e)
	f.s = svc.NewService(f.r, f.assets, nil, f.graph, auth)
	f.gov = govsvc.NewService(repo.NewGovernanceRepository(db), f.assets, nil, unusedQueue{}, auth)
	return f
}
func (f *fixture) source(t *testing.T, rows [][]any) l.DataAssetRef {
	t.Helper()
	ref, e := f.assets.ImportWithKey(context.Background(), f.p, l.Input{Kind: "table", Columns: []string{"id", "region", "amount", "event"}, ColumnTypes: map[string]string{"amount": "DECIMAL"}, Rows: rows}, "warehouse integration snapshot", uuid.NewString())
	require.NoError(t, e)
	return ref
}
func definition(ref string) d.Definition {
	return d.Definition{Name: "按区域营收", Subject: "销售", Grain: "每区域一行", GrainKeys: []string{"region"}, FactNode: "orders", FactKeys: []string{"id"}, EventField: "event", UpdatePolicy: "upsert", DeletePolicy: "unsupported", LatePolicy: "reject_incremental_use_backfill", OutputNode: "regional", Nodes: []d.Node{{ID: "orders", Layer: "raw", Kind: "source", SourceRef: ref}, {ID: "regional", Layer: "summary", Kind: "aggregate", Inputs: []string{"orders"}, GroupBy: []string{"region"}, Metrics: []d.Metric{{Name: "revenue", Field: "amount", Aggregate: "sum", Caliber: "按订单原始金额精确相加"}}}}}
}
func (f *fixture) http(t *testing.T, p l.Principal, path string, body any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID) })
	handler.NewWarehouseHandler(f.s).Register(r.Group("/api"))
	raw, e := json.Marshal(body)
	require.NoError(t, e)
	req := httptest.NewRequest("POST", "/api/warehouse"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}
func TestWarehouseMySQLHTTPApprovalIncrementalAndHistoricalSnapshot(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, [][]any{{"a", "north", "9007199254740993.123456789012345678", "2026-01-01T00:00:00Z"}, {"b", "north", "0.1", "2026-01-02T00:00:00Z"}})
	def := definition(source.RefID.String())
	code, body := f.http(t, f.p, "/preview", map[string]any{"definition": def, "limit": 20})
	require.Equal(t, 200, code, body)
	require.Equal(t, "9007199254740993.223456789012345678", body["data"].(map[string]any)["exact_rows"].([]any)[0].([]any)[1])
	m, e := f.s.SaveModel(ctx, f.p, "", 0, def, "model")
	require.NoError(t, e)
	run, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: m.ID, ModelVersion: 1, Mode: "full"}, "full", nil)
	require.NoError(t, e)
	require.NoError(t, f.s.Process(ctx, f.p, run.ID))
	run, e = f.s.GetRun(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "awaiting_approval", run.Status)
	require.True(t, run.LineageRegistered)
	head, e := f.s.GetModel(ctx, f.p, m.ID, 0)
	require.NoError(t, e)
	require.Empty(t, head.ActiveRef)
	code, body = f.http(t, f.p, "/runs/"+run.ID+"/approve", map[string]any{"expected_version": run.Version})
	require.Equal(t, 200, code, body)
	run, e = f.s.GetRun(ctx, f.p, run.ID)
	require.NoError(t, e)
	original := run.PublishedRef
	originalID, e := l.ParseRefID(original)
	require.NoError(t, e)
	_, before, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, originalID)
	require.NoError(t, e)
	incSource := f.source(t, [][]any{{"b", "north", "0.2", "2026-01-03T00:00:00Z"}})
	inc, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: m.ID, ModelVersion: 1, Mode: "incremental", FactSourceRef: incSource.RefID.String()}, "inc", nil)
	require.NoError(t, e)
	require.NoError(t, f.s.Process(ctx, f.p, inc.ID))
	inc, e = f.s.GetRun(ctx, f.p, inc.ID)
	require.NoError(t, e)
	inc, e = f.s.Approve(ctx, f.p, inc.ID, inc.Version)
	require.NoError(t, e)
	require.True(t, inc.Watermark.After(*run.Watermark))
	_, after, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, originalID)
	require.NoError(t, e)
	require.JSONEq(t, string(before), string(after))
	id, e := l.ParseRefID(inc.PublishedRef)
	require.NoError(t, e)
	_, raw, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, id)
	require.NoError(t, e)
	require.Contains(t, string(raw), "9007199254740993.323456789012345678")
	backSource := f.source(t, [][]any{{"b", "north", "0.05", "2026-01-01T00:00:00Z"}})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	back, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: m.ID, ModelVersion: 1, Mode: "backfill", FactSourceRef: backSource.RefID.String(), EventFrom: &from, EventTo: &to}, "back", nil)
	require.NoError(t, e)
	require.NoError(t, f.s.Process(ctx, f.p, back.ID))
	back, e = f.s.GetRun(ctx, f.p, back.ID)
	require.NoError(t, e)
	require.True(t, back.Watermark.Equal(*inc.Watermark))
	head, e = f.s.GetModel(ctx, f.p, m.ID, 0)
	require.NoError(t, e)
	require.Equal(t, inc.PublishedRef, head.ActiveRef)
	// A newer definition invalidates approval of the previously generated candidate.
	def.Name = "new model version"
	_, e = f.s.SaveModel(ctx, f.p, m.ID, 1, def, "model-v2")
	require.NoError(t, e)
	_, e = f.s.Approve(ctx, f.p, back.ID, back.Version)
	require.ErrorIs(t, e, d.ErrConflict)
	head, e = f.s.GetModel(ctx, f.p, m.ID, 0)
	require.NoError(t, e)
	require.Equal(t, inc.PublishedRef, head.ActiveRef)
}
func TestWarehouseMySQLResumeFailureAndCancellationFences(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, [][]any{{"a", "north", "1.25", "2026-01-01T00:00:00Z"}, {"b", "south", "2.5", "2026-01-02T00:00:00Z"}})
	def := definition(source.RefID.String())
	m, e := f.s.SaveModel(ctx, f.p, "", 0, def, "model")
	require.NoError(t, e)
	run, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: m.ID, ModelVersion: 1, Mode: "full"}, "resume", nil)
	require.NoError(t, e)
	claim, e := f.r.Claim(ctx, f.p, run.ID)
	require.NoError(t, e)
	preview, e := f.s.Preview(ctx, f.p, def, 20)
	require.NoError(t, e)
	raw, e := json.Marshal(preview.Output)
	require.NoError(t, e)
	part, e := f.assets.ImportDerived(ctx, f.p, l.Result, "warehouse-candidate:"+run.ID+":part-0", []l.RefID{source.RefID}, raw, l.PolicySnapshot{Scopes: []string{"read"}, Version: m.Digest})
	require.NoError(t, e)
	hash := sha256.Sum256(raw)
	require.NoError(t, f.r.Update(ctx, claim, func(v *d.Run) error {
		v.Partitions = append(v.Partitions, d.Partition{Index: 0, Count: 2, Digest: hex.EncodeToString(hash[:]), OutputRef: part.RefID.String()})
		return nil
	}))
	require.NoError(t, f.db.Table("warehouse_runs").Where("id=?", run.ID).Update("lease_until", time.Now().Add(-time.Minute)).Error)
	require.NoError(t, f.s.Process(ctx, f.p, run.ID))
	run, e = f.s.GetRun(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Len(t, run.Partitions, 1)
	require.Equal(t, part.RefID.String(), run.Partitions[0].OutputRef)
	run, e = f.s.Approve(ctx, f.p, run.ID, run.Version)
	require.NoError(t, e)
	failed, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: m.ID, ModelVersion: 1, Mode: "full"}, "failure", nil)
	require.NoError(t, e)
	f.graph.fail.Store(true)
	require.Error(t, f.s.Process(ctx, f.p, failed.ID))
	head, e := f.s.GetModel(ctx, f.p, m.ID, 0)
	require.NoError(t, e)
	require.Equal(t, run.PublishedRef, head.ActiveRef)
	failed, e = f.s.GetRun(ctx, f.p, failed.ID)
	require.NoError(t, e)
	require.Equal(t, "failed", failed.Status)
	queued, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: m.ID, ModelVersion: 1, Mode: "full"}, "cancel", nil)
	require.NoError(t, e)
	f.revoke.Store(true)
	_, e = f.s.GetRun(ctx, f.p, queued.ID)
	require.Error(t, e)
	stopped, e := f.s.Cancel(ctx, f.p, queued.ID, queued.Version)
	require.NoError(t, e)
	require.Equal(t, "cancelled", stopped.Status)
	require.Empty(t, stopped.SourceDigests)
	foreign := f.p
	foreign.UserID++
	_, e = f.r.GetRun(ctx, foreign, queued.ID)
	require.ErrorIs(t, e, d.ErrNotFound)
}

func TestWarehouseMySQLAdapterReplayCancelAndSourceFences(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, [][]any{{"a", "north", json.Number("9007199254740993.123456789012345678"), "2026-01-01T00:00:00Z"}})
	def := definition(source.RefID.String())
	model, e := f.s.SaveModel(ctx, f.p, "", 0, def, "model")
	require.NoError(t, e)
	preview, e := f.s.Preview(ctx, f.p, def, 20)
	require.NoError(t, e)
	require.Equal(t, "9007199254740993.123456789012345678", preview.ExactRows[0][1])
	adapter, e := warehouseadapter.New(f.s)
	require.NoError(t, e)
	cap := adapter.Capability()
	command := func(task string) tc.ExecutionCommand {
		c := tc.ExecutionCommand{Scope: m.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}, TaskID: task, PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), RequestID: "warehouse-adapter-test", InputRefs: []l.RefID{source.RefID}, Node: tc.Node{ID: "warehouse", Capability: cap.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &source.RefID, Kind: source.Kind}}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 103, TimeoutSeconds: 120, Parameters: &tc.NodeParameters{Warehouse: &tc.WarehouseParameters{ModelID: model.ID, Version: 1, Mode: "full"}}}}
		raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
		c.OperationKey = m.Digest(raw)
		raw, _ = json.Marshal(c)
		raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
		c.PayloadHash = m.Digest(raw)
		return c
	}
	c := command("completed")
	require.NoError(t, adapter.Validate(ctx, c.Scope, c.Node))
	handle, e := adapter.Submit(ctx, c)
	require.NoError(t, e)
	duplicate, e := adapter.Submit(ctx, c)
	require.NoError(t, e)
	require.Equal(t, handle, duplicate)
	require.NoError(t, f.s.Process(ctx, f.p, handle.DomainID))
	observation, e := adapter.Inspect(ctx, c, *handle)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, observation.Status)
	require.Len(t, observation.OutputRefs, 1)
	head, e := f.s.GetModel(ctx, f.p, model.ID, 0)
	require.NoError(t, e)
	require.Empty(t, head.ActiveRef, "task completion never activates candidate")
	queued := command("queued")
	qh, e := adapter.Submit(ctx, queued)
	require.NoError(t, e)
	bad := *qh
	bad.DomainID = handle.DomainID
	_, e = adapter.Cancel(ctx, queued, bad)
	require.ErrorIs(t, e, m.ErrConflict)
	qr, e := f.r.GetRun(ctx, f.p, qh.DomainID)
	require.NoError(t, e)
	require.Equal(t, "queued", qr.Status)
	f.revoke.Store(true)
	_, e = adapter.Inspect(ctx, queued, *qh)
	require.ErrorIs(t, e, m.ErrForbidden)
	completed, e := adapter.Cancel(ctx, c, *handle)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, completed.Status)
	require.Empty(t, completed.OutputRefs)
	cancelled, e := adapter.Cancel(ctx, queued, *qh)
	require.NoError(t, e)
	require.Equal(t, m.DomainCancelled, cancelled.Status)
	tomb := command("not-submitted")
	th, e := adapter.CancelSubmission(ctx, tomb)
	require.NoError(t, e)
	f.revoke.Store(false)
	late, e := adapter.Submit(ctx, tomb)
	require.NoError(t, e)
	require.Equal(t, th, late)
	observed, e := adapter.Inspect(ctx, tomb, *late)
	require.NoError(t, e)
	require.Equal(t, m.DomainCancelled, observed.Status)
}

func TestWarehouseMySQLDynamicFactUsesActualSnapshot(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	anchor := f.source(t, [][]any{{"a", "north", "1.25", "2026-01-01T00:00:00Z"}})
	actual := f.source(t, [][]any{{"b", "south", "99.25", "2026-02-01T00:00:00Z"}})
	model, e := f.s.SaveModel(ctx, f.p, "", 0, definition(anchor.RefID.String()), "model")
	require.NoError(t, e)
	adapter, e := warehouseadapter.New(f.s)
	require.NoError(t, e)
	cap := adapter.Capability()
	c := tc.ExecutionCommand{Scope: m.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}, TaskID: "dynamic", PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(time.Minute), InputRefs: []l.RefID{anchor.RefID, actual.RefID}, Node: tc.Node{ID: "warehouse", Capability: cap.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &anchor.RefID, Kind: anchor.Kind}, {FromNode: "upstream", Kind: actual.Kind}}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 104, TimeoutSeconds: 120, Parameters: &tc.NodeParameters{Warehouse: &tc.WarehouseParameters{ModelID: model.ID, Version: 1, Mode: "full", FactFromNode: "upstream"}}}}
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
	require.NoError(t, adapter.Validate(ctx, c.Scope, c.Node))
	h, e := adapter.Submit(ctx, c)
	require.NoError(t, e)
	require.NoError(t, f.s.Process(ctx, f.p, h.DomainID))
	run, e := f.s.GetRun(ctx, f.p, h.DomainID)
	require.NoError(t, e)
	require.Equal(t, actual.RefID.String(), run.Request.FactSourceRef)
	observed, e := adapter.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, observed.Status)
	require.Len(t, observed.OutputRefs, 1)
	ref, body, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, observed.OutputRefs[0])
	require.NoError(t, e)
	require.Contains(t, ref.Parents, actual.RefID)
	require.Contains(t, ref.Parents, anchor.RefID)
	require.Contains(t, string(body), "99.25")
	require.Contains(t, string(body), "south")
	// Consume the actual warehouse result through the feature capability, retaining
	// its independent definition anchor and the full warehouse source lineage.
	require.Equal(t, l.Result, ref.Kind)
	featureAnchor, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"region", "revenue"}, Rows: [][]any{{"unused", "1"}}}, "feature schema anchor", "feature-anchor")
	require.NoError(t, e)
	feature, e := f.gov.CreateFeature(ctx, f.p, g.Feature{Name: "revenue plus one", Type: "numerical", SourceRefID: featureAnchor.RefID.String(), Definition: json.RawMessage(`{"kind":"expression","entity_field":"region","expression":{"op":"add","args":[{"op":"column","field":"revenue"},{"op":"literal","value":1}]}}`)}, "feature")
	require.NoError(t, e)
	featureAdapter, e := governanceadapter.New(f.gov, f.assets, "governance.feature-engineering")
	require.NoError(t, e)
	fc := featureAdapter.Capability()
	node := tc.Node{ID: "feature", Capability: fc.ID, CapabilityVersion: fc.Version, Inputs: []tc.InputBinding{{FromNode: "warehouse", Kind: ref.Kind}}, OutputKinds: fc.OutputKinds, Tools: fc.Tools, Permissions: fc.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 3, TimeoutSeconds: 60, Parameters: &tc.NodeParameters{Feature: &tc.FeatureParameters{FeatureID: feature.ID, Version: 1}}}
	require.NoError(t, featureAdapter.Validate(ctx, c.Scope, node))
	require.NoError(t, f.gov.ValidateBoundFeatureVersion(ctx, f.p, feature.ID, 1, ref.RefID.String()))
	gr := repo.NewGovernanceRepository(f.db).(g.GovernedRepository)
	invocation := g.GovernedInvocation{OperationKey: strings.Repeat("c", 64), PayloadHash: strings.Repeat("d", 64), Profile: g.FeatureProfile, FeatureID: feature.ID, FeatureVersion: 1, InputRef: ref.RefID, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, CallsReserved: 3}
	op, e := gr.SubmitGoverned(ctx, f.p, invocation, false)
	require.NoError(t, e)
	_, e = gr.UpdateGoverned(ctx, f.p, op.ID, func(record *g.Record, _ *g.GovernedExecution, _ g.Repository) error {
		record.Status = "running"
		return nil
	})
	require.NoError(t, e)
	receipt, e := f.gov.MaterializeGovernedFeature(ctx, f.p, feature.ID, 1, op.ID)
	require.NoError(t, e)
	require.Equal(t, ref.RefID.String(), receipt.SourceRefID)
	output, e := l.ParseRefID(receipt.OutputRefID)
	require.NoError(t, e)
	featureRef, featureBytes, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, output)
	require.NoError(t, e)
	require.Equal(t, []l.RefID{ref.RefID, featureAnchor.RefID}, featureRef.Parents)
	require.Contains(t, string(featureBytes), "100.25")
}

func TestWarehouseMySQLImpactIncludesRegisteredConsumersAndHidesOtherOwners(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, [][]any{{"a", "north", "1.25", "2026-01-01T00:00:00Z"}})
	def := definition(source.RefID.String())
	model, e := f.s.SaveModel(ctx, f.p, "", 0, def, "model")
	require.NoError(t, e)
	report, e := f.assets.ImportDerived(ctx, f.p, l.Insight, "integration-report", []l.RefID{source.RefID}, json.RawMessage(`{"narrative":"computed"}`), l.PolicySnapshot{Scopes: []string{"read"}, Version: "1"})
	require.NoError(t, e)
	gov := repo.NewGovernanceRepository(f.db)
	create := func(user int64) *g.Record {
		payload, _ := json.Marshal(map[string]string{"source_ref": source.RefID.String()})
		record, e := gov.Create(ctx, &g.Record{Meta: g.Meta{TenantID: f.p.TenantID, UserID: user}, Kind: "feature", Payload: payload})
		require.NoError(t, e)
		return record
	}
	own := create(f.p.UserID)
	foreign := create(f.p.UserID + 1)
	dataset, e := repo.NewDatasetVersionRepository(f.db).Put(ctx, f.p, "dataset", &ev.DatasetVersion{DatasetID: "dataset", Name: "reviewed samples", SourceRefs: []string{source.RefID.String()}}, map[string]int{})
	require.NoError(t, e)
	preview, e := f.s.Preview(ctx, f.p, def, 20)
	require.NoError(t, e)
	impact := map[string]string{}
	unknown := false
	for _, item := range preview.Impact {
		impact[item.ID] = item.Kind
		unknown = unknown || item.Unknown
	}
	require.Equal(t, "warehouse_model", impact[model.ID])
	require.Equal(t, "feature", impact[own.ID])
	require.NotContains(t, impact, foreign.ID)
	require.Equal(t, "dataset_version", impact[dataset.ID])
	require.Equal(t, string(l.Insight), impact[report.RefID.String()])
	require.True(t, unknown)
}
func TestWarehouseMySQLExpiredDeadlineBlocksNewPartitionReceipt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, [][]any{{"a", "north", "1.25", "2026-01-01T00:00:00Z"}})
	model, e := f.s.SaveModel(ctx, f.p, "", 0, definition(source.RefID.String()), "model")
	require.NoError(t, e)
	run, e := f.s.CreateRun(ctx, f.p, d.RunRequest{ModelID: model.ID, ModelVersion: 1, Mode: "full"}, "deadline", nil)
	require.NoError(t, e)
	claim, e := f.r.Claim(ctx, f.p, run.ID)
	require.NoError(t, e)
	expired := time.Now().Add(-time.Minute)
	claim.Deadline = &expired
	raw, e := json.Marshal(claim)
	require.NoError(t, e)
	require.NoError(t, f.db.Table("warehouse_runs").Where("id=? AND tenant_id=?", run.ID, f.p.TenantID).Update("body", string(raw)).Error)
	e = f.r.Update(ctx, claim, func(v *d.Run) error {
		v.Partitions = append(v.Partitions, d.Partition{Index: 0, Count: 1, OutputRef: "uncommitted:1"})
		return nil
	})
	require.ErrorIs(t, e, d.ErrConflict)
	stored, e := f.r.GetRun(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Empty(t, stored.Partitions)
	require.NoError(t, f.r.Update(ctx, claim, func(v *d.Run) error { v.Status = "failed"; v.Error = "deadline"; return nil }))
	stored, e = f.r.GetRun(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "failed", stored.Status)
}
