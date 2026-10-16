//go:build integration

package feedback_test

import (
	"bytes"
	"cognida/internal/handler"
	ev "cognida/internal/model/evaluation"
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/evaluation"
	svc "cognida/internal/service/feedback"
	control "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"sync"
	"testing"
	"time"
)

// Real MySQL/CAS/HTTP coverage; external projection/evaluation ports below are
// deliberately local deterministic fixtures. Real model/Milvus evidence is E2E.
type fixturePorts struct {
	svc.Projection
	svc.Evaluations
	denied   bool
	evalGate string
}

func (f *fixturePorts) Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if f.denied {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	return l.DataAssetRef{}, json.RawMessage(`{"kind":"text","text":"candidate"}`), nil
}
func (f *fixturePorts) CurrentLabels(context.Context, l.Principal, string) ([]string, error) {
	return []string{}, nil
}
func (f *fixturePorts) Validate(context.Context, l.Principal, string, string, string) error {
	if f.denied {
		return l.ErrForbidden
	}
	return nil
}
func (f *fixturePorts) FeedbackValidate(context.Context, l.Principal, json.RawMessage) error {
	return nil
}
func (f *fixturePorts) FeedbackCompare(context.Context, l.Principal, string, string, json.RawMessage, d.View, d.View, d.Gate) (*evaluation.Comparison, error) {
	return &evaluation.Comparison{Gate: f.evalGate, Improvement: .3}, nil
}
func (f *fixturePorts) Lookup(context.Context, l.Principal, *d.Case, *d.Publication) (json.RawMessage, error) {
	return json.RawMessage(`{"index":"integration-port","state":"ready"}`), nil
}

type dbFixture struct {
	db *gorm.DB
	r  d.Repository
	s  *svc.Service
	f  *fixturePorts
	p  l.Principal
	c  *d.Case
	v  *d.Publication
}

func database(t *testing.T) *dbFixture {
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
	conn, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 71321}
	f := &dbFixture{db: db, r: repo.NewFeedbackRepository(db), f: &fixturePorts{evalGate: "eligible_for_review"}, p: p}
	f.s, e = svc.NewService(f.r, f.f, f.f, f.f, nil, nil, func(_ context.Context, who l.Principal, _ string) error {
		if who != p {
			return l.ErrForbidden
		}
		return nil
	})
	require.NoError(t, e)
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	f.c = &d.Case{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Status: "open", Create: d.Create{Title: "integration evidence", BaselineRef: ref, KnowledgeBaseID: uuid.NewString(), Evidence: d.Evidence{OutputRef: ref, SourceRefs: []l.RefID{ref}, Question: "question", Problem: "problem"}, Gate: d.Gate{MinImprovement: .01}}, BaselineKnowledgeID: uuid.NewString(), BaselineLabels: []string{}, Deadline: time.Now().Add(time.Hour)}
	f.c, e = f.r.CreateCase(context.Background(), f.c, "create", d.Digest(f.c.Create))
	require.NoError(t, e)
	f.v = &d.Publication{ID: uuid.NewString(), CaseID: f.c.ID, TenantID: p.TenantID, UserID: p.UserID, Status: "validating", KnowledgeID: uuid.NewString(), ContentDigest: d.Digest("candidate"), CandidateInput: d.CandidateInput{SourceRef: ref, RoundID: "round"}, Projection: json.RawMessage(`{"index":"integration-port","state":"ready"}`)}
	f.v, e = f.r.CreatePublication(context.Background(), f.v, "candidate", d.Digest(f.v.CandidateInput))
	require.NoError(t, e)
	require.NoError(t, db.Create(&repo.KnowledgeModel{ID: f.v.KnowledgeID, TenantID: p.TenantID, UserID: p.UserID, KnowledgeBaseID: f.c.KnowledgeBaseID, Title: "candidate", Type: "text", Source: "feedback", ParseStatus: "completed", EnableStatus: "disabled", FileHash: f.v.ContentDigest}).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM quality_feedback_events WHERE case_id=?", f.c.ID).Error)
		for _, table := range []string{"quality_feedback_publications", "quality_feedback_cases", "knowledge"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, conn.Close())
	})
	return f
}
func (f *dbFixture) api(t *testing.T, p l.Principal, method, path string, body any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID); c.Next() })
	handler.NewFeedbackHandler(f.s).Register(engine.Group("/api"))
	raw, e := json.Marshal(body)
	require.NoError(t, e)
	req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "feedback-http-fixture")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	return w.Code, out
}

// This adapter is only used for the real persisted start/readiness boundary;
// no controller is run and no external compute result is fabricated.
type startBoundaryAdapter struct{ control.ExecutionAdapter }

func (startBoundaryAdapter) Support() control.ExecutionSupport {
	return control.ExecutionSupport{Profile: "feedback-start-fixture", BaseCalls: 1, MaxTimeoutSeconds: 60}
}
func (startBoundaryAdapter) Validate(context.Context, tc.Scope, control.Node) error    { return nil }
func (startBoundaryAdapter) ValidateInputs(context.Context, tc.Scope, []l.RefID) error { return nil }

func TestFeedbackMySQLHTTPReworkStartsSavedDraftAndReplaysOnce(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint("runtime_enabled=", enabled), func(t *testing.T) {
			f := database(t)
			ctx := context.Background()
			capability := control.Capability{ID: "lifecycle.data-cleaning", Version: 1, Domain: "lifecycle", InputKinds: []l.Kind{l.RawAsset}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"cleaning.transform"}, Permissions: []string{"read"}}
			validator, e := control.NewValidator([]control.Capability{capability}, func(_ context.Context, who tc.Scope, _ string) error {
				if who.TenantID != f.p.TenantID || who.UserID != f.p.UserID {
					return tc.ErrForbidden
				}
				return nil
			}, func(_ context.Context, _ tc.Scope, ref l.RefID) (l.Kind, error) {
				if ref != f.c.BaselineRef {
					return "", tc.ErrForbidden
				}
				return l.RawAsset, nil
			})
			require.NoError(t, e)
			tasksRepo := repo.NewTaskControlRepository(f.db)
			f.s.Tasks, e = control.NewService(tasksRepo, validator)
			require.NoError(t, e)
			f.s.Runtime, e = control.NewRuntime(f.s.Tasks, tasksRepo, map[string]control.ExecutionAdapter{capability.ID: startBoundaryAdapter{}}, enabled)
			require.NoError(t, e)
			t.Cleanup(func() {
				for _, table := range []string{"task_control_outbox", "task_control_children", "task_control_plans", "task_control_leases", "task_control_tasks"} {
					require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
				}
			})
			f.c.Budget = d.Budget{MaxRounds: 2, MaxCalls: 10, TimeoutSeconds: 3600}
			require.NoError(t, f.r.SaveCase(ctx, f.c, f.c.Version, d.Event{Action: "fixture_budget"}))
			plan := control.PlanSpec{SchemaVersion: 1, Budget: control.Budget{MaxNodes: 1, MaxConcurrency: 1, MaxAttempts: 1, MaxCalls: 1, TimeoutSeconds: 60}, Nodes: []control.Node{{ID: "repair", Capability: capability.ID, CapabilityVersion: 1, Inputs: []control.InputBinding{{Asset: &f.c.BaselineRef, Kind: l.RawAsset}}, OutputKinds: capability.OutputKinds, Tools: capability.Tools, Permissions: capability.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 1, TimeoutSeconds: 60}}}
			body := map[string]any{"expected_version": f.c.Version, "plan": plan}
			code, out := f.api(t, f.p, "POST", "/feedback-cases/"+f.c.ID+"/rework", body)
			if !enabled {
				require.Equal(t, 400, code, out)
				var started int64
				require.NoError(t, f.db.Table("task_control_tasks").Where("tenant_id=? AND started_at IS NOT NULL", f.p.TenantID).Count(&started).Error)
				require.Zero(t, started, "readiness must still prevent execution")
				return
			}
			require.Equal(t, 200, code, out)
			c, e := f.r.GetCase(ctx, f.p, f.c.ID)
			require.NoError(t, e)
			require.Len(t, c.Rounds, 1)
			task, e := f.s.Tasks.Get(ctx, tc.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}, c.Rounds[0].TaskID)
			require.NoError(t, e)
			require.NotNil(t, task.StartedAt)
			require.Equal(t, int64(1), task.ExecutionPlanVersion)
			require.NotEqual(t, tc.StatusDraft, task.Status)
			code, out = f.api(t, f.p, "POST", "/feedback-cases/"+f.c.ID+"/rework", body)
			require.Equal(t, 200, code, out)
			var count int64
			require.NoError(t, f.db.Table("task_control_tasks").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
			require.EqualValues(t, 1, count)
			c, e = f.r.GetCase(ctx, f.p, f.c.ID)
			require.NoError(t, e)
			require.Len(t, c.Rounds, 1)
			require.Equal(t, 1, c.ReservedCalls)
		})
	}
}
func TestFeedbackMySQLHTTPExplicitValidationApprovalActivationAndWithdrawal(t *testing.T) {
	f := database(t)
	ctx := context.Background()
	path := "/feedback-publications/" + f.v.ID
	code, _ := f.api(t, f.p, "POST", path+"/activate", map[string]any{"expected_version": f.v.Version, "expected_case_version": f.c.Version})
	require.Equal(t, 409, code)
	code, _ = f.api(t, f.p, "POST", path+"/compare", map[string]any{})
	require.Equal(t, 200, code)
	v, e := f.r.GetPublication(ctx, f.p, f.v.ID)
	require.NoError(t, e)
	require.Equal(t, "validated", v.Status)
	code, out := f.api(t, f.p, "POST", path+"/approve", map[string]any{"expected_version": v.Version, "comment": "checked fixed comparison"})
	require.Equal(t, 200, code, out)
	v, e = f.r.GetPublication(ctx, f.p, v.ID)
	require.NoError(t, e)
	require.Equal(t, "approved", v.Status)
	code, out = f.api(t, f.p, "POST", path+"/activate", map[string]any{"expected_version": v.Version, "expected_case_version": f.c.Version})
	require.Equal(t, 200, code, out)
	var doc repo.KnowledgeModel
	require.NoError(t, f.db.Where("id=?", v.KnowledgeID).Take(&doc).Error)
	require.Equal(t, "enabled", doc.EnableStatus)
	restarted := repo.NewFeedbackRepository(f.db)
	c, e := restarted.GetCase(ctx, f.p, f.c.ID)
	require.NoError(t, e)
	require.Equal(t, v.ID, c.ActiveVersionID)
	code, out = f.api(t, f.p, "POST", "/feedback-cases/"+c.ID+"/rollback", map[string]any{"expected_version": c.Version, "publication_id": "baseline"})
	require.Equal(t, 200, code, out)
	c, e = restarted.GetCase(ctx, f.p, c.ID)
	require.NoError(t, e)
	require.Empty(t, c.ActiveVersionID)
	require.Equal(t, "active", c.Status)
	code, out = f.api(t, f.p, "POST", path+"/activate", map[string]any{"expected_version": v.Version, "expected_case_version": c.Version})
	require.Equal(t, 200, code, out)
	c, e = restarted.GetCase(ctx, f.p, c.ID)
	require.NoError(t, e)
	require.Equal(t, v.ID, c.ActiveVersionID)
	code, _ = f.api(t, l.Principal{TenantID: f.p.TenantID, UserID: f.p.UserID + 1}, "GET", "/feedback-cases/"+c.ID, nil)
	require.Equal(t, 403, code)
	f.f.denied = true
	code, _ = f.api(t, f.p, "GET", "/feedback-cases/"+c.ID, nil)
	require.Equal(t, 403, code)
	code, out = f.api(t, f.p, "POST", "/feedback-cases/"+c.ID+"/withdraw", map[string]any{"expected_version": c.Version})
	require.Equal(t, 200, code, out)
	value := out["data"].(map[string]any)
	require.Equal(t, "withdrawn", value["status"])
	require.Empty(t, value["baseline_knowledge_id"])
	require.Empty(t, value["corrections"])
}

func TestFeedbackMySQLHTTPRegressionCannotApproveOrActivate(t *testing.T) {
	f := database(t)
	f.f.evalGate = "regression" // Fixed comparison port; this is not model-quality evidence.
	path := "/feedback-publications/" + f.v.ID
	code, out := f.api(t, f.p, "POST", path+"/compare", map[string]any{})
	require.Equal(t, 200, code, out)
	v, e := f.r.GetPublication(context.Background(), f.p, f.v.ID)
	require.NoError(t, e)
	require.Equal(t, "rejected", v.Status)
	code, _ = f.api(t, f.p, "POST", path+"/approve", map[string]any{"expected_version": v.Version, "comment": "Cannot override measured regression"})
	require.Equal(t, 409, code)
	code, _ = f.api(t, f.p, "POST", path+"/activate", map[string]any{"expected_version": v.Version, "expected_case_version": f.c.Version})
	require.Equal(t, 409, code)
	var doc repo.KnowledgeModel
	require.NoError(t, f.db.Where("id=?", v.KnowledgeID).Take(&doc).Error)
	require.Equal(t, "disabled", doc.EnableStatus)
	c, e := f.r.GetCase(context.Background(), f.p, f.c.ID)
	require.NoError(t, e)
	require.Empty(t, c.ActiveVersionID)
}

func TestFeedbackMySQLHTTPFixedComparisonEmptyLabelsRoundTrip(t *testing.T) {
	f := database(t)
	ctx := context.Background()
	// Persist real task/snapshot/result rows. These deterministic scores and
	// projection/source ports test the contract, not external model quality.
	versionsRepo := repo.NewDatasetVersionRepository(f.db)
	pairs := []ev.QAPair{{Question: "one", ReferenceAnswer: "answer one"}, {Question: "two", ReferenceAnswer: "answer two"}}
	version, e := versionsRepo.Put(ctx, f.p, "fixed-comparison", &ev.DatasetVersion{DatasetID: "fixed-" + uuid.NewString(), Profile: "rag", ManifestDigest: d.Digest(pairs), QAPairs: pairs, SourceRefs: []string{f.c.BaselineRef.String()}}, nil)
	require.NoError(t, e)
	versions := evaluation.NewVersionService(versionsRepo, nil, f.f, nil)
	snapshots := evaluation.NewSnapshotService(versions, nil, nil)
	tasksRepo := repo.NewEvaluationTaskRepository(f.db)
	resultsRepo := repo.NewEvaluationResultRepository(f.db)
	actual := evaluation.NewService(nil, tasksRepo, resultsRepo, nil, nil, nil).WithSnapshots(snapshots)
	snapshots.FeedbackView = f.s.CheckView
	f.s.Evaluations = actual
	fixed := ev.EvaluationSnapshot{DatasetVersionID: version.ID, DatasetManifest: version.ManifestDigest, Profile: "rag", QAPairs: pairs, Configuration: json.RawMessage(`{"type":"rag","config":{"top_k":5}}`), Target: json.RawMessage(`{"model":"fixed-test-target"}`), GraderVersion: "fixed", CreatedAt: time.Now().UTC()}
	f.c.EvaluationSnapshot, e = json.Marshal(fixed)
	require.NoError(t, e)
	f.c.Gate = d.Gate{Metric: "rouge_l", HigherIsBetter: true, MinPairs: 2, MinImprovement: .1}
	require.NoError(t, f.r.SaveCase(ctx, f.c, f.c.Version, d.Event{Action: "fixed_fixture"}))
	ids := []string{uuid.NewString(), uuid.NewString()}
	f.v.BaselineEvaluationID, f.v.CandidateEvaluationID = ids[0], ids[1]
	f.v.Labels = []string{}
	require.NoError(t, f.r.SavePublication(ctx, f.v, f.v.Version, d.Event{Action: "fixed_fixture"}))
	t.Cleanup(func() {
		for _, id := range ids {
			require.NoError(t, resultsRepo.DeleteByTaskID(ctx, id))
			require.NoError(t, f.db.Exec("DELETE FROM evaluation_tasks WHERE id=? AND tenant_id=?", id, f.p.TenantID).Error)
		}
		for _, table := range []string{"evaluation_dataset_versions", "evaluation_dataset_heads"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	var original []byte
	for i, id := range ids {
		snap := fixed
		view := d.View{CaseID: f.c.ID, PublicationID: "baseline", KnowledgeBaseID: f.c.KnowledgeBaseID, KnowledgeID: f.c.BaselineKnowledgeID, Digest: f.c.BaselineDigest, Labels: []string{}, Deadline: f.c.Deadline}
		if i == 1 {
			view.PublicationID, view.KnowledgeID, view.Digest = f.v.ID, f.v.KnowledgeID, f.v.ContentDigest
		}
		snap.FeedbackView = &view
		task := ev.NewEvaluationTask(id, f.p.TenantID, f.p.UserID, version.DatasetID, ev.EvaluationTypeRAG, len(pairs))
		task.Status = ev.TaskStatusCompleted
		task.Snapshot, _ = json.Marshal(snap)
		require.NoError(t, tasksRepo.Create(ctx, task))
		if i == 1 {
			original = append([]byte{}, task.Snapshot...)
		}
		for _, pair := range pairs {
			require.NoError(t, resultsRepo.Create(ctx, &ev.EvaluationResult{TaskID: id, Question: pair.Question, ReferenceAnswer: pair.ReferenceAnswer, Success: true, Scores: map[string]float64{"rouge_l": .2 + float64(i)*.6, "feedback_view_applied": 1}, CreatedAt: time.Now().UTC()}))
		}
	}
	path := "/feedback-publications/" + f.v.ID + "/compare"
	code, out := f.api(t, f.p, "POST", path, map[string]any{})
	require.Equal(t, 200, code, out)
	v, e := f.r.GetPublication(ctx, f.p, f.v.ID)
	require.NoError(t, e)
	require.Equal(t, "validated", v.Status)
	var comparison evaluation.Comparison
	require.NoError(t, json.Unmarshal(v.Comparison, &comparison))
	require.Equal(t, 2, comparison.Pairs)
	require.InDelta(t, .6, comparison.Improvement, 1e-9)
	var changed ev.EvaluationSnapshot
	require.NoError(t, json.Unmarshal(original, &changed))
	changed.Target = json.RawMessage(`{"model":"different-target"}`)
	raw, _ := json.Marshal(changed)
	require.NoError(t, f.db.Table("evaluation_tasks").Where("id=?", ids[1]).Update("snapshot", string(raw)).Error)
	code, _ = f.api(t, f.p, "POST", path, map[string]any{})
	require.Equal(t, 409, code, "changing the fixed model must still fail")
	require.NoError(t, json.Unmarshal(original, &changed))
	changed.FeedbackView.Labels = []string{"unapproved"}
	raw, _ = json.Marshal(changed)
	require.NoError(t, f.db.Table("evaluation_tasks").Where("id=?", ids[1]).Update("snapshot", string(raw)).Error)
	code, _ = f.api(t, f.p, "POST", path, map[string]any{})
	require.Equal(t, 403, code, "changing the authorized view must still fail")

	// Historical result endpoints recheck the same source. Revocation must
	// produce an authorization response without returning persisted answers.
	require.NoError(t, f.db.Table("evaluation_tasks").Where("id=?", ids[1]).Update("snapshot", string(original)).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("tenant_id", f.p.TenantID); c.Set("user_id", f.p.UserID); c.Next() })
	readHandler := handler.NewEvaluationHandler(actual, nil, nil, nil)
	engine.GET("/evaluation/results/:task_id", readHandler.GetResults)
	engine.GET("/evaluation/results/:task_id/qa", readHandler.GetQAResults)
	for _, suffix := range []string{"", "/qa?cursor="} {
		f.f.denied = false
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest("GET", "/evaluation/results/"+ids[1]+suffix, nil))
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "answer one")
		f.f.denied = true
		w = httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest("GET", "/evaluation/results/"+ids[1]+suffix, nil))
		require.Equal(t, 403, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "answer one")
	}
}
func TestFeedbackMySQLConcurrentCaseCASAndAtomicPublicationFence(t *testing.T) {
	f := database(t)
	ctx := context.Background()
	first, e := f.r.GetCase(ctx, f.p, f.c.ID)
	require.NoError(t, e)
	second, e := f.r.GetCase(ctx, f.p, f.c.ID)
	require.NoError(t, e)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, c := range []*d.Case{first, second} {
		wg.Add(1)
		go func(c *d.Case) {
			defer wg.Done()
			c.ReservedCalls = 20
			results <- f.r.SaveCase(ctx, c, c.Version, d.Event{Action: "reserve"})
		}(c)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, d.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	c, e := f.r.GetCase(ctx, f.p, f.c.ID)
	require.NoError(t, e)
	require.Equal(t, 20, c.ReservedCalls)
	require.ErrorIs(t, f.r.Commit(ctx, f.p, c.ID, f.v.ID, c.Version, f.v.Version, "forged", "activate"), d.ErrGate)
	var doc repo.KnowledgeModel
	require.NoError(t, f.db.Where("id=?", f.v.KnowledgeID).Take(&doc).Error)
	require.Equal(t, "disabled", doc.EnableStatus)
	c, e = f.r.GetCase(ctx, f.p, c.ID)
	require.NoError(t, e)
	require.Empty(t, c.ActiveVersionID)
	_, e = f.r.GetPublication(ctx, l.Principal{TenantID: f.p.TenantID + 1, UserID: f.p.UserID}, f.v.ID)
	require.ErrorIs(t, e, d.ErrNotFound)
}
func TestFeedbackMySQLIdempotencyRejectsDifferentCandidate(t *testing.T) {
	f := database(t)
	ctx := context.Background()
	replay, e := f.r.CreatePublication(ctx, f.v, "candidate", d.Digest(f.v.CandidateInput))
	require.NoError(t, e)
	require.Equal(t, f.v.ID, replay.ID)
	changed := *f.v
	changed.SourceRef = l.RefID{AssetID: uuid.NewString(), Version: 1}
	_, e = f.r.CreatePublication(ctx, &changed, "candidate", d.Digest(changed.CandidateInput))
	require.ErrorIs(t, e, d.ErrConflict)
	other := *f.c
	other.ID = uuid.NewString()
	other.UserID++
	_, e = f.r.CreateCase(ctx, &other, "other", d.Digest(other.Create))
	require.Error(t, e, "one authoritative activity pointer per knowledge source")
}
