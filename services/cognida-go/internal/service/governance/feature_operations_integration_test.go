//go:build integration

package governance_test

import (
	"bytes"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func featureOperationsDB(t *testing.T) *featureDBFixture {
	f := featureDB(t)
	t.Cleanup(func() {
		for _, table := range []string{"feature_job_values", "feature_jobs", "feature_watermarks", "drift_events", "drift_records"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	return f
}
func operationsAPI(t *testing.T, f *featureDBFixture, p l.Principal, method, path, key string, body any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID); c.Next() })
	handler.NewGovernanceHandler(f.s).RegisterFeatureOperations(r.Group("/api"))
	raw, e := json.Marshal(body)
	require.NoError(t, e)
	req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return w.Code, out
}
func TestFeatureJobsMySQLHTTPIncrementalBackfillAndWithdrawal(t *testing.T) {
	f := featureOperationsDB(t)
	ctx := context.Background()
	head, source := f.feature(t, "definition")
	request := d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: source.RefID.String(), Mode: "full", PartitionSize: 1}
	code, out := operationsAPI(t, f, f.p, "POST", "/feature-jobs", "full", request)
	require.Equal(t, 200, code, out)
	id := out["data"].(map[string]any)["id"].(string)
	j, e := f.s.GetFeatureJob(ctx, f.p, id)
	require.NoError(t, e)
	replay, e := f.s.CreateFeatureJob(ctx, f.p, request, "full")
	require.NoError(t, e)
	require.Equal(t, j.ID, replay.ID)
	require.NoError(t, f.s.RunFeatureJob(ctx, f.p.TenantID, f.p.UserID, id))
	j, e = f.s.GetFeatureJob(ctx, f.p, id)
	require.NoError(t, e)
	require.Equal(t, "completed", j.Status)
	require.Equal(t, 2, j.CommittedRows)
	require.Len(t, j.Partitions, 2)
	require.NotNil(t, j.WatermarkAfter)
	filters := []d.FeatureValue{{FeatureID: head.ID, Version: 1}}
	at := j.Partitions[0].AvailableAt.Add(time.Microsecond)
	before := j.Partitions[0].AvailableAt.Add(-time.Microsecond)
	values, e := f.s.QueryFeatures(ctx, f.p, []string{"00123"}, filters, before)
	require.NoError(t, e)
	require.Empty(t, values)
	values, e = f.s.QueryFeatures(ctx, f.p, []string{"00123"}, filters, at)
	require.NoError(t, e)
	require.Equal(t, "9007199254740993.223456789012345678", values[0].ValueExact)
	newSource, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "amount", "event"}, Rows: [][]any{{"c", "1.2", "2026-01-01T03:00:00Z"}}}, "incremental", uuid.NewString())
	require.NoError(t, e)
	inc, e := f.s.CreateFeatureJob(ctx, f.p, d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: newSource.RefID.String(), Mode: "incremental", PartitionSize: 1}, "incremental")
	require.NoError(t, e)
	require.Equal(t, j.WatermarkAfter, inc.WatermarkBefore)
	require.NoError(t, f.s.RunFeatureJob(ctx, f.p.TenantID, f.p.UserID, inc.ID))
	inc, e = f.s.GetFeatureJob(ctx, f.p, inc.ID)
	require.NoError(t, e)
	require.Equal(t, 1, inc.CommittedRows)
	require.True(t, inc.WatermarkAfter.After(*j.WatermarkAfter))
	backSource, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "amount", "event"}, Rows: [][]any{{"00123", "0.3", "2026-01-01T01:00:00Z"}}}, "backfill", uuid.NewString())
	require.NoError(t, e)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	back, e := f.s.CreateFeatureJob(ctx, f.p, d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: backSource.RefID.String(), Mode: "backfill", PartitionSize: 1, EventFrom: &from, EventTo: &to}, "back")
	require.NoError(t, e)
	require.NoError(t, f.s.RunFeatureJob(ctx, f.p.TenantID, f.p.UserID, back.ID))
	back, e = f.s.GetFeatureJob(ctx, f.p, back.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", back.Status)
	require.Equal(t, inc.WatermarkAfter, back.WatermarkAfter, "backfill must not move watermark")
	values, e = f.s.QueryFeatures(ctx, f.p, []string{"00123"}, filters, time.Now().UTC())
	require.NoError(t, e)
	require.Equal(t, "0.4", values[0].ValueExact)
	_, e = f.s.ControlFeatureJob(ctx, f.p, back.ID, "withdraw", back.Version)
	require.NoError(t, e)
	values, e = f.s.QueryFeatures(ctx, f.p, []string{"00123"}, filters, time.Now().UTC())
	require.NoError(t, e)
	require.Equal(t, "9007199254740993.223456789012345678", values[0].ValueExact)
	values, e = f.s.QueryFeatures(ctx, f.p, []string{"00123"}, filters, *back.Partitions[0].AvailableAt)
	require.NoError(t, e)
	require.Equal(t, "0.4", values[0].ValueExact, "historical availability remains true after withdrawal")
	other := f.p
	other.UserID++
	code, _ = operationsAPI(t, f, other, "GET", "/feature-jobs/"+id, "", nil)
	require.NotEqual(t, 200, code)
	f.revoked.Store(true)
	code, _ = operationsAPI(t, f, f.p, "GET", "/feature-jobs/"+id, "", nil)
	require.Equal(t, 403, code)
}
func TestFeatureJobsMySQLResumeAndUnconfirmedEmbedding(t *testing.T) {
	f := featureOperationsDB(t)
	ctx := context.Background()
	head, source := f.feature(t, "resume")
	job, e := f.s.CreateFeatureJob(ctx, f.p, d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: source.RefID.String(), Mode: "full", PartitionSize: 1}, "resume")
	require.NoError(t, e)
	r := f.repo.(d.FeatureJobRepository)
	claimed, e := r.ClaimFeatureJob(ctx, f.p.TenantID, f.p.UserID, job.ID)
	require.NoError(t, e)
	require.NotNil(t, claimed)
	require.NoError(t, f.db.Exec("UPDATE feature_jobs SET lease_until=DATE_SUB(NOW(6),INTERVAL 1 MINUTE) WHERE id=?", job.ID).Error)
	require.NoError(t, f.s.RunFeatureJob(ctx, f.p.TenantID, f.p.UserID, job.ID))
	job, e = f.s.GetFeatureJob(ctx, f.p, job.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", job.Status)
	input := d.FeatureJob{TenantID: f.p.TenantID, UserID: f.p.UserID, Request: d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: source.RefID.String(), Mode: "full", PartitionSize: 1}, DefinitionDigest: strings.Repeat("a", 64), SourceDigest: strings.Repeat("b", 64), Embedding: true, Partitions: []d.FeaturePartition{{Index: 0, Status: "pending"}}}
	embedding, e := r.CreateFeatureJob(ctx, &input, "unconfirmed")
	require.NoError(t, e)
	_, e = r.ClaimFeatureJob(ctx, f.p.TenantID, f.p.UserID, embedding.ID)
	require.NoError(t, e)
	require.NoError(t, f.db.Exec("UPDATE feature_jobs SET lease_until=DATE_SUB(NOW(6),INTERVAL 1 MINUTE) WHERE id=?", embedding.ID).Error)
	claim, e := r.ClaimFeatureJob(ctx, f.p.TenantID, f.p.UserID, embedding.ID)
	require.NoError(t, e)
	require.Nil(t, claim)
	embedding, e = r.GetFeatureJob(ctx, f.p.TenantID, f.p.UserID, embedding.ID)
	require.NoError(t, e)
	require.Equal(t, "reconcile_required", embedding.Status)
}
func TestDriftMySQLHTTPApprovalDedupeScopeAndMonitoring(t *testing.T) {
	f := featureOperationsDB(t)
	ctx := context.Background()
	_, source := f.feature(t, "drift-source")
	code, out := operationsAPI(t, f, f.p, "POST", "/drift/baselines", "base", map[string]any{"name": "真实参考", "source_ref_id": source.RefID.String(), "sampling": map[string]any{"limit": 100, "minimum": 2, "seed": "fixed"}})
	require.Equal(t, 200, code, out)
	base := out["data"].(map[string]any)["id"].(string)
	code, _ = operationsAPI(t, f, f.p, "POST", "/drift/compare", "", map[string]any{"baseline_id": base, "current_ref_id": source.RefID.String()})
	require.Equal(t, 409, code)
	code, out = operationsAPI(t, f, f.p, "POST", "/drift/baselines/"+base+"/approve", "approve", map[string]any{"expected_version": 1})
	require.Equal(t, 200, code, out)
	changed, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "amount", "event"}, Rows: [][]any{{"x", json.Number("50"), "2026-01-01T01:00:00Z"}, {"y", json.Number("60"), "2026-01-01T02:00:00Z"}}}, "current", uuid.NewString())
	require.NoError(t, e)
	report, e := f.s.CompareDrift(ctx, f.p, base, changed.RefID.String(), .2)
	require.NoError(t, e)
	require.Equal(t, "changed", report.Status)
	again, e := f.s.CompareDrift(ctx, f.p, base, changed.RefID.String(), .2)
	require.NoError(t, e)
	require.Equal(t, report.ID, again.ID)
	var body d.DriftReport
	require.NoError(t, json.Unmarshal(report.Body, &body))
	require.Equal(t, 2, body.BaselineRows)
	require.Equal(t, 2, body.CurrentRows)
	monitor, e := f.s.CreateDriftMonitor(ctx, f.p, d.DriftMonitor{Name: "monitor", BaselineID: base, CurrentRefID: changed.RefID.String(), Threshold: .2, IntervalSeconds: 60}, "monitor")
	require.NoError(t, e)
	// Exercise the real scheduler against this test's tenant only. Other test
	// packages can create due monitors concurrently in the same isolated DB.
	// Pin this fixture in the past instead of depending on app/DB clock skew.
	require.NoError(t, f.db.Exec("UPDATE drift_records SET next_run_at='2000-01-01 00:00:00' WHERE id=? AND tenant_id=?", monitor.ID, f.p.TenantID).Error)
	r := repo.NewGovernanceRepository(f.db.Where("tenant_id=?", f.p.TenantID)).(d.DriftRepository)
	due, e := r.ClaimDriftMonitors(ctx, 10)
	require.NoError(t, e)
	require.Len(t, due, 1)
	require.Equal(t, monitor.ID, due[0].ID)
	require.NoError(t, f.s.RunDriftMonitor(ctx, due[0]))
	monitor, e = f.s.GetDrift(ctx, f.p, monitor.ID)
	require.NoError(t, e)
	var m d.DriftMonitor
	require.NoError(t, json.Unmarshal(monitor.Body, &m))
	require.Equal(t, report.ID, m.LastReportID)
	events, e := f.s.DriftAudit(ctx, f.p, base)
	require.NoError(t, e)
	require.Len(t, events, 2)
	require.Equal(t, "baseline_approved", events[1].Action)
	f.revoked.Store(true)
	code, _ = operationsAPI(t, f, f.p, "GET", "/drift/records/"+report.ID, "", nil)
	require.Equal(t, 403, code)
}

func TestGovernedEmbeddingDurableJobRealMySQL(t *testing.T) {
	f := featureOperationsDB(t)
	ctx := context.Background()
	f.s.SetGovernedInputValidator(func(ctx context.Context, p l.Principal, refs []l.RefID) error {
		for _, id := range refs {
			if _, _, e := f.assets.Resolve(ctx, p.TenantID, p.UserID, id); e != nil {
				return e
			}
		}
		return nil
	})
	calls := 0
	f.s.SetFeatureEmbedding("test-real-port", "revision-pinned", func(_ context.Context, in []string) ([][]float64, error) {
		calls++
		out := [][]float64{}
		for range in {
			out = append(out, []float64{.2, .8})
		}
		return out, nil
	})
	source, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "text"}, Rows: [][]any{{"a", "actual first input"}, {"b", "actual second input"}}}, "embedding adapter fixture", uuid.NewString())
	require.NoError(t, e)
	head, e := f.s.CreateFeature(ctx, f.p, d.Feature{Name: "vectors", Type: "vector", SourceRefID: source.RefID.String(), Definition: []byte(`{"kind":"embedding","field":"text","entity_field":"entity","embedding":{"model_id":"test-real-port","revision":"revision-pinned","dimension":2}}`)}, "vectors")
	require.NoError(t, e)
	in := d.GovernedInvocation{OperationKey: strings.Repeat("c", 64), PayloadHash: strings.Repeat("d", 64), Profile: d.FeatureProfile, InputRef: source.RefID, FeatureID: head.ID, FeatureVersion: 1, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, CallsReserved: 3}
	r := f.repo.(d.GovernedRepository)
	op, e := r.SubmitGoverned(ctx, f.p, in, false)
	require.NoError(t, e)
	payload, _ := json.Marshal(map[string]any{"tenant_id": f.p.TenantID, "user_id": f.p.UserID, "kind": d.GovernedKind, "id": op.ID})
	require.NoError(t, f.s.Handle(ctx, job.Job{ID: op.ID, Type: "governance", Payload: payload}))
	op, e = r.FindGoverned(ctx, f.p, in.OperationKey)
	require.NoError(t, e)
	require.Equal(t, "completed", op.Status)
	var state d.GovernedExecution
	require.NoError(t, json.Unmarshal(op.Payload, &state))
	require.NotEmpty(t, state.FeatureJobID)
	require.Len(t, state.OutputRefs, 1)
	require.Equal(t, 1, calls)
	require.NoError(t, f.s.Handle(ctx, job.Job{ID: op.ID, Type: "governance", Payload: payload}))
	require.Equal(t, 1, calls, "recovery must reuse committed model output")
	values, e := f.s.QueryFeatures(ctx, f.p, []string{"a"}, []d.FeatureValue{{FeatureID: head.ID, Version: 1}}, time.Now().UTC())
	require.NoError(t, e)
	require.Equal(t, "[0.2,0.8]", values[0].ValueExact)
	// A cancellation during the actual provider port fences the partition commit.
	in.OperationKey = strings.Repeat("e", 64)
	in.PayloadHash = strings.Repeat("f", 64)
	op, e = r.SubmitGoverned(ctx, f.p, in, false)
	require.NoError(t, e)
	f.s.SetFeatureEmbedding("test-real-port", "revision-pinned", func(_ context.Context, inText []string) ([][]float64, error) {
		_, err := r.SubmitGoverned(ctx, f.p, in, true)
		require.NoError(t, err)
		return [][]float64{{.9, .1}, {.8, .2}}, nil
	})
	payload, _ = json.Marshal(map[string]any{"tenant_id": f.p.TenantID, "user_id": f.p.UserID, "kind": d.GovernedKind, "id": op.ID})
	require.NoError(t, f.s.Handle(ctx, job.Job{ID: op.ID, Type: "governance", Payload: payload}))
	op, e = r.FindGoverned(ctx, f.p, in.OperationKey)
	require.NoError(t, e)
	require.Equal(t, "cancelled", op.Status)
	require.NoError(t, json.Unmarshal(op.Payload, &state))
	require.Empty(t, state.OutputRefs)
}

func TestGovernedEmbeddingBudgetUsesActualSnapshotBeforeProvider(t *testing.T) {
	f := featureOperationsDB(t)
	ctx := context.Background()
	f.s.SetGovernedInputValidator(f.assets.ValidateGovernedInputs)
	calls := 0
	f.s.SetFeatureEmbedding("test-real-port", "revision-pinned", func(context.Context, []string) ([][]float64, error) {
		calls++
		return nil, errors.New("must not call provider")
	})
	anchor, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "text"}, Rows: [][]any{{"anchor", "one row"}}}, "anchor", uuid.NewString())
	require.NoError(t, e)
	head, e := f.s.CreateFeature(ctx, f.p, d.Feature{Name: "vectors", Type: "vector", SourceRefID: anchor.RefID.String(), Definition: []byte(`{"kind":"embedding","field":"text","entity_field":"entity","embedding":{"model_id":"test-real-port","revision":"revision-pinned","dimension":2}}`)}, "vectors")
	require.NoError(t, e)
	rows := [][]any{}
	for i := 0; i < 65; i++ {
		rows = append(rows, []any{fmt.Sprint(i), "text"})
	}
	actual, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "text"}, Rows: rows}, "larger snapshot", uuid.NewString())
	require.NoError(t, e)
	required, embedding, e := f.s.FeatureExecutionRequirementsForSource(ctx, f.p, head.ID, 1, actual.RefID.String())
	require.NoError(t, e)
	require.True(t, embedding)
	require.Equal(t, 5, required)
	in := d.GovernedInvocation{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), Profile: d.FeatureProfile, InputRef: actual.RefID, FeatureID: head.ID, FeatureVersion: 1, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, CallsReserved: 3}
	_, e = f.s.SubmitGoverned(ctx, f.p, in, false)
	require.ErrorIs(t, e, d.ErrInvalid)
	r := f.repo.(d.GovernedRepository)
	op, e := r.SubmitGoverned(ctx, f.p, in, false)
	require.NoError(t, e)
	payload, _ := json.Marshal(map[string]any{"tenant_id": f.p.TenantID, "user_id": f.p.UserID, "kind": d.GovernedKind, "id": op.ID})
	require.NoError(t, f.s.Handle(ctx, job.Job{ID: op.ID, Type: "governance", Payload: payload}))
	op, e = r.FindGoverned(ctx, f.p, in.OperationKey)
	require.NoError(t, e)
	require.Equal(t, "failed", op.Status)
	require.Zero(t, calls)
	for len(rows) < 1001 {
		rows = append(rows, []any{fmt.Sprint(len(rows)), "text"})
	}
	tooLarge, e := f.assets.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"entity", "text"}, Rows: rows}, "above governed limit", uuid.NewString())
	require.NoError(t, e)
	_, _, e = f.s.FeatureExecutionRequirementsForSource(ctx, f.p, head.ID, 1, tooLarge.RefID.String())
	require.Error(t, e)
	require.Zero(t, calls)
}

func TestFeatureJobStopAfterSourceRevocationMySQL(t *testing.T) {
	f := featureOperationsDB(t)
	ctx := context.Background()
	head, source := f.feature(t, "stop")
	j, e := f.s.CreateFeatureJob(ctx, f.p, d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: source.RefID.String(), Mode: "full", PartitionSize: 1}, "stop")
	require.NoError(t, e)
	f.s.Authorize = func(_ context.Context, p l.Principal, _ string) error {
		if p != f.p {
			return l.ErrForbidden
		}
		return nil
	}
	f.revoked.Store(true)
	_, e = f.s.GetFeatureJob(ctx, f.p, j.ID)
	require.Error(t, e)
	out, e := f.s.ControlFeatureJob(ctx, f.p, j.ID, "cancel", j.Version)
	require.NoError(t, e)
	require.Equal(t, "cancelled", out.Status)
	require.Empty(t, out.Request.SourceRefID)
	require.Empty(t, out.SourceDigest)
	out, e = f.s.ControlFeatureJob(ctx, f.p, j.ID, "withdraw", out.Version)
	require.NoError(t, e)
	require.Equal(t, "withdrawn", out.Status)
	_, e = f.s.ControlFeatureJob(ctx, f.p, j.ID, "retry", out.Version)
	require.Error(t, e)
}

func TestFeatureJobDeadlineDatabaseFenceMySQL(t *testing.T) {
	for _, boundary := range []string{"job_deadline", "parent_deadline", "parent_timeout"} {
		t.Run(boundary, func(t *testing.T) {
			f := featureOperationsDB(t)
			ctx := context.Background() // Deliberately not cancelled: the database must enforce the cutoff.
			head, source := f.feature(t, "deadline")
			r := f.repo.(d.FeatureJobRepository)
			operations := f.repo.(d.GovernedRepository)
			end := time.Now().UTC().Add(time.Hour)
			in := d.GovernedInvocation{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), Profile: d.FeatureProfile, InputRef: source.RefID, FeatureID: head.ID, FeatureVersion: 1, Deadline: end, TimeoutSeconds: 300, CallsReserved: 3}
			op, e := operations.SubmitGoverned(ctx, f.p, in, false)
			require.NoError(t, e)
			input := d.FeatureJob{TenantID: f.p.TenantID, UserID: f.p.UserID, OperationID: op.ID, Deadline: &end, Request: d.FeatureJobRequest{FeatureID: head.ID, FeatureVersion: 1, SourceRefID: source.RefID.String(), Mode: "full", PartitionSize: 1}, DefinitionDigest: strings.Repeat("a", 64), SourceDigest: strings.Repeat("b", 64), Partitions: []d.FeaturePartition{{Index: 0, Status: "pending"}}}
			j, e := r.CreateFeatureJob(ctx, &input, "before-deadline")
			require.NoError(t, e)
			j, e = r.ClaimFeatureJob(ctx, f.p.TenantID, f.p.UserID, j.ID)
			require.NoError(t, e)
			require.NotNil(t, j)
			past := time.Now().UTC().Add(-time.Hour)
			switch boundary {
			case "job_deadline":
				j.Deadline = &past
				body, err := json.Marshal(j)
				require.NoError(t, err)
				require.NoError(t, f.db.Table("feature_jobs").Where("id=?", j.ID).Update("body", body).Error)
			case "parent_deadline":
				var state d.GovernedExecution
				require.NoError(t, json.Unmarshal(op.Payload, &state))
				state.Invocation.Deadline = past
				body, err := json.Marshal(state)
				require.NoError(t, err)
				require.NoError(t, f.db.Table("governance_records").Where("id=?", op.ID).Update("payload", body).Error)
			case "parent_timeout":
				require.NoError(t, f.db.Table("governance_records").Where("id=?", op.ID).Update("created_at", past).Error)
			}
			part := d.FeaturePartition{Index: 0, Count: 1, Digest: strings.Repeat("c", 64), OutputRefID: "expired:1"}
			values := []d.FeatureValue{{EntityID: "entity", FeatureID: head.ID, Version: 1, SourceRefID: source.RefID.String(), Timestamp: past, Value: json.RawMessage(`0.123456789012345678`)}}
			require.ErrorIs(t, r.CommitFeaturePartition(ctx, j, part, values), d.ErrConflict)
			var count int64
			require.NoError(t, f.db.Table("feature_job_values").Where("job_id=?", j.ID).Count(&count).Error)
			require.Zero(t, count)
			stored, e := r.GetFeatureJob(ctx, f.p.TenantID, f.p.UserID, j.ID)
			require.NoError(t, e)
			require.Zero(t, stored.CommittedRows)
			candidate := *j
			_, e = r.CreateFeatureJob(ctx, &candidate, "after-deadline")
			require.ErrorIs(t, e, d.ErrConflict)
			var jobs int64
			require.NoError(t, f.db.Table("feature_jobs").Where("tenant_id=?", f.p.TenantID).Count(&jobs).Error)
			require.EqualValues(t, 1, jobs)
		})
	}
}
