//go:build integration

package governanceadapter_test

import (
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/scenariotools"
	tc "cognida/internal/service/taskcontrol"
	ga "cognida/internal/service/taskcontrol/governanceadapter"
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/eino/components/tool"
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

// Real MySQL + local objects + HTTP, with explicit fixture identity, queue and
// optional metadata compute. The labeling rules themselves are production code.
type testQueue struct {
	job.Queue
	mu   sync.Mutex
	jobs map[string]job.Job
	fail atomic.Bool
}

func (q *testQueue) Enqueue(_ context.Context, j job.Job) error {
	if q.fail.Load() {
		return errors.New("delivery unavailable")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[j.ID] = j
	return nil
}
func (q *testQueue) get(id string) job.Job { q.mu.Lock(); defer q.mu.Unlock(); return q.jobs[id] }

type compute struct{}

func (compute) Call(_ context.Context, op string, input l.Input, _ map[string]any, _ string) (lifecycle.ComputeResult, error) {
	if op == "clean" {
		return lifecycle.ComputeResult{SchemaVersion: "1", Method: "rules", Result: map[string]any{"data": input}}, nil
	}
	if op == "profile" {
		return lifecycle.ComputeResult{SchemaVersion: "1", Method: "rules", Result: map[string]any{"overall_score": 100}}, nil
	}
	return lifecycle.ComputeResult{Method: "rules", Result: map[string]any{"summary": map[string]any{"value": "fixture metadata"}}}, nil
}

type fixture struct {
	db         *gorm.DB
	s          *svc.Service
	assets     *lifecycle.Service
	p          l.Principal
	q          *testQueue
	revoked    atomic.Bool
	sharedUser atomic.Bool
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_DOMAIN_TEST_DSN not set")
	}
	cfg, e := sqlmysql.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated database required")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 713}, q: &testQueue{jobs: map[string]job.Job{}}}
	t.Cleanup(func() {
		for _, table := range []string{"feature_materializations", "feature_values", "governance_records", "lifecycle_assets"} {
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
		if f.revoked.Load() || p.TenantID != f.p.TenantID || (p.UserID != f.p.UserID && !(f.sharedUser.Load() && p.UserID == f.p.UserID+1)) {
			return l.ErrForbidden
		}
		return nil
	}
	origin := func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		if strings.HasPrefix(ref.ProducerID, "feature-materialization:") {
			return f.s.ValidateFeatureMaterializationOrigin(ctx, p, ref)
		}
		return f.s.ValidateGovernedOrigin(ctx, p, ref)
	}
	f.assets, e = lifecycle.NewService(repo.NewLifecycleRepository(db), store, f.q, lifecycle.Config{Authorize: auth, ValidateGovernedOrigin: origin, Compute: compute{}})
	require.NoError(t, e)
	f.s = svc.NewService(repo.NewGovernanceRepository(db), f.assets, compute{}, f.q, auth)
	return f
}
func (f *fixture) input(t *testing.T, in l.Input) l.DataAssetRef {
	t.Helper()
	ref, e := f.assets.ImportWithKey(context.Background(), f.p, in, "synthetic labeling fixture", uuid.NewString())
	require.NoError(t, e)
	cleaned, e := f.assets.Invoke(context.Background(), f.p, "data-cleaning", []l.RefID{ref.RefID}, nil, uuid.NewString(), "")
	require.NoError(t, e)
	require.NoError(t, f.assets.Handle(context.Background(), f.q.get("lifecycle:"+cleaned.ID)))
	cleaned, e = f.assets.Repository().FindStage(context.Background(), f.p.TenantID, cleaned.ID, false)
	require.NoError(t, e)
	require.Equal(t, l.StatusSucceeded, cleaned.Status)
	prepared, e := f.assets.PrepareAsset(context.Background(), f.p, cleaned.Output.Refs[0].RefID, l.PreparationOptions{}, uuid.NewString(), "")
	require.NoError(t, e)
	require.NoError(t, f.assets.Handle(context.Background(), f.q.get("lifecycle:"+prepared.ID)))
	prepared, e = f.assets.Repository().FindStage(context.Background(), f.p.TenantID, prepared.ID, false)
	require.NoError(t, e)
	require.Equal(t, l.StatusSucceeded, prepared.Status)
	return prepared.Output.Refs[0]
}
func (f *fixture) standard(t *testing.T, kind string, labels []d.Label) string {
	t.Helper()
	in := d.LabelStandardInput{Name: "Fixture " + kind, TaskType: kind, Guidelines: "Use only the explicitly supplied rules; abstain on unmatched content.", Constraints: d.StandardConstraints{AllowEmpty: true}}
	switch kind {
	case "tagging":
		in.Constraints.MaxLabels = 30
	case "classification":
		in.Constraints.MaxLabels = 100
	case "named_entity":
		in.Constraints.MaxEntities = 100
	case "relation":
		in.Constraints.MaxEntities = 100
		in.Constraints.MaxRelations = 100
	case "qa":
		in.Constraints.AllowEmpty = false
	}
	for _, label := range labels {
		in.Labels = append(in.Labels, d.StandardLabel{Label: label, Definition: "Explicit fixture category " + label.Name, IncludeCriteria: []string{"Match the configured rules"}, ExcludeCriteria: []string{"Do not infer unstated content"}})
	}
	raw, _ := json.Marshal(in)
	key := m.Digest(raw)
	page, e := f.s.List(context.Background(), f.p, d.Filter{Kind: d.LabelStandardVersionKind, Limit: 100})
	require.NoError(t, e)
	for _, row := range page.Items {
		var v d.LabelStandardVersion
		require.NoError(t, json.Unmarshal(row.Payload, &v))
		vraw, _ := json.Marshal(v.LabelStandardInput)
		if string(vraw) == string(raw) {
			return row.ID
		}
	}
	draft, e := f.s.CreateLabelStandard(context.Background(), f.p, in, key)
	require.NoError(t, e)
	version, e := f.s.PublishLabelStandard(context.Background(), f.p, draft.ID, draft.Version, key)
	require.NoError(t, e)
	return version.ID
}
func (f *fixture) http(t *testing.T, user int64, method, path string, body any, key string) *httptest.ResponseRecorder {
	t.Helper()
	standardOwner := *f
	standardOwner.p.UserID = user
	switch in := body.(type) {
	case d.AnnotationTask:
		if in.StandardVersionID == "" {
			in.StandardVersionID = standardOwner.standard(t, in.Type, in.Labels)
		}
		body = in
	case d.TaggingTask:
		if in.StandardVersionID == "" {
			in.StandardVersionID = standardOwner.standard(t, "tagging", in.Config.LabelSchema)
		}
		body = in
	}
	raw, _ := json.Marshal(body)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", f.p.TenantID); c.Set("user_id", user); c.Next() })
	handler.NewGovernanceHandler(f.s).Register(router.Group("/api/v1"))
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func responseID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	require.True(t, w.Code == 200 || w.Code == 201, "status %d body %s", w.Code, w.Body.String())
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.NotEmpty(t, out.Data.ID)
	return out.Data.ID
}
func annotationItem(t *testing.T, f *fixture, taskID string) (d.Record, d.AnnotationItem) {
	t.Helper()
	page, e := f.s.List(context.Background(), f.p, d.Filter{Kind: "annotation_item", ParentID: taskID, Limit: 100})
	require.NoError(t, e)
	require.NotEmpty(t, page.Items)
	var item d.AnnotationItem
	require.NoError(t, json.Unmarshal(page.Items[0].Payload, &item))
	return page.Items[0], item
}
func TestLabelingIntegrationHTTPRulesReviewAndExport(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.input(t, l.Input{Kind: "text", Text: "😀小李带来了 invoice"})
	labels := []d.Label{{ID: "finance", Name: "财务", Keywords: []string{"invoice"}}}
	request := d.AnnotationTask{Name: "classification", Type: "classification", SourceRefID: source.AssetRefID, Labels: labels}
	id := responseID(t, f.http(t, f.p.UserID, "POST", "/annotation-tasks", request, "classification"))
	same := responseID(t, f.http(t, f.p.UserID, "POST", "/annotation-tasks", request, "classification"))
	require.Equal(t, id, same)
	changed := request
	changed.Labels = []d.Label{{ID: "legal", Name: "法务"}}
	require.Equal(t, 409, f.http(t, f.p.UserID, "POST", "/annotation-tasks", changed, "classification").Code)
	require.Equal(t, 200, f.http(t, f.p.UserID, "POST", "/annotation-tasks/"+id+"/preannotate", nil, "").Code)
	record, item := annotationItem(t, f, id)
	require.Equal(t, []string{"finance"}, item.Annotation.Labels)
	require.Equal(t, "matched", item.Provenance.DecisionStatus)
	require.Nil(t, item.Provenance.Confidence)
	require.NotEmpty(t, item.Provenance.EvidenceSpans)
	require.Empty(t, f.http(t, f.p.UserID, "GET", "/annotation-tasks/"+id+"/export?format=jsonl", nil, "").Body.String())
	review := map[string]any{"decision": "approved", "expected_version": record.Version}
	require.Equal(t, 200, f.http(t, f.p.UserID, "POST", "/annotation-tasks/"+id+"/items/"+record.ID+"/review", review, "").Code)
	export := f.http(t, f.p.UserID, "GET", "/annotation-tasks/"+id+"/export?format=jsonl", nil, "")
	require.Equal(t, 200, export.Code)
	require.Contains(t, export.Body.String(), "approved")
	require.Contains(t, export.Body.String(), "invoice")
	before, _ := f.s.Get(ctx, f.p, "annotation_item", record.ID)
	require.NoError(t, f.s.Preannotate(ctx, f.p, id))
	after, _ := f.s.Get(ctx, f.p, "annotation_item", record.ID)
	require.Equal(t, before.Version, after.Version, "approved records must not be overwritten")
	update := map[string]any{"annotation": d.Annotation{Labels: []string{}}, "expected_version": after.Version}
	require.Equal(t, 200, f.http(t, f.p.UserID, "PUT", "/annotation-tasks/"+id+"/items/"+record.ID, update, "").Code)
	record, item = annotationItem(t, f, id)
	require.Equal(t, "pending", item.ReviewStatus)
	require.Zero(t, item.ReviewedBy)
	require.Equal(t, "manual", item.Provenance.DecisionStatus)
	require.NoError(t, f.s.Preannotate(ctx, f.p, id))
	preserved, _ := f.s.Get(ctx, f.p, "annotation_item", record.ID)
	require.Equal(t, record.Version, preserved.Version)
	require.Equal(t, 403, f.http(t, f.p.UserID+1, "GET", "/annotation-tasks/"+id, nil, "").Code)
	entity := d.AnnotationTask{Name: "entities", Type: "named_entity", SourceRefID: source.AssetRefID, Labels: []d.Label{{ID: "person", Name: "人物", Terms: []string{"小李"}}}}
	entityID := responseID(t, f.http(t, f.p.UserID, "POST", "/annotation-tasks", entity, "entities"))
	require.NoError(t, f.s.Preannotate(ctx, f.p, entityID))
	_, ent := annotationItem(t, f, entityID)
	require.Len(t, ent.Annotation.Entities, 1)
	require.Equal(t, 1, ent.Annotation.Entities[0].Start)
	require.Equal(t, 3, ent.Annotation.Entities[0].End)
}
func TestLabelingIntegrationQASourceMappingAndTagEvidence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.input(t, l.Input{Kind: "table", Columns: []string{"question", "answer"}, Rows: [][]any{{"编号？", "00123"}}})
	task := d.AnnotationTask{Name: "qa", Type: "qa", SourceRefID: source.AssetRefID, QAMapping: &d.QAMapping{QuestionColumn: "question", ReferenceAnswerColumn: "answer"}}
	id := responseID(t, f.http(t, f.p.UserID, "POST", "/annotation-tasks", task, "qa"))
	record, _ := annotationItem(t, f, id)
	require.Equal(t, 400, f.http(t, f.p.UserID, "POST", "/annotation-tasks/"+id+"/items/"+record.ID+"/review", map[string]any{"decision": "approved", "expected_version": record.Version}, "").Code, "unlabeled QA cannot be exported as ground truth")
	require.NoError(t, f.s.Preannotate(ctx, f.p, id))
	record, item := annotationItem(t, f, id)
	require.Equal(t, "00123", item.Annotation.QA.ReferenceAnswer)
	require.Equal(t, "explicit_qa_columns", item.Provenance.Method)
	require.Nil(t, item.Provenance.Confidence)
	_, e := f.s.ReviewAnnotation(ctx, f.p, id, record.ID, "approved", "checked exact source", record.Version)
	require.NoError(t, e)
	require.Contains(t, f.http(t, f.p.UserID, "GET", "/annotation-tasks/"+id+"/export?format=jsonl", nil, "").Body.String(), `"reference_answer":"00123"`)
	invalid := task
	invalid.QAMapping = &d.QAMapping{QuestionColumn: "question", ReferenceAnswerColumn: "missing"}
	require.Equal(t, 400, f.http(t, f.p.UserID, "POST", "/annotation-tasks", invalid, "missing").Code)
	text := f.input(t, l.Input{Kind: "text", Text: "invoice and contract"})
	tagTask := d.TaggingTask{Name: "tags", SourceRefID: text.AssetRefID, Config: d.TaggingConfig{EnableTags: true, EnableClassification: true, LabelSchema: []d.Label{{ID: "finance", Name: "财务", Keywords: []string{"invoice"}}, {ID: "legal", Name: "法务", Keywords: []string{"contract"}}}}}
	tagID := responseID(t, f.http(t, f.p.UserID, "POST", "/tagging-tasks", tagTask, "tag"))
	require.NoError(t, f.s.Handle(ctx, f.q.get(tagID)))
	page, e := f.s.List(ctx, f.p, d.Filter{Kind: "tagging_result", ParentID: tagID, Limit: 100})
	require.NoError(t, e)
	require.Len(t, page.Items, 1)
	var result d.TaggingResult
	require.NoError(t, json.Unmarshal(page.Items[0].Payload, &result))
	require.ElementsMatch(t, []string{"财务", "法务"}, result.Tags)
	require.Empty(t, result.Category)
	require.Nil(t, result.CategoryConfidence)
	require.Nil(t, result.TagConfidence)
	require.Equal(t, "uncertain", result.Provenance.DecisionStatus)
	require.NotEmpty(t, result.Provenance.EvidenceSpans)
	// Approval preserves evidence; human edits clear both approval and stale model/rule scores.
	reviewed, e := f.s.ReviewTagging(ctx, f.p, page.Items[0].ID, "approved", "reviewed", page.Items[0].Version)
	require.NoError(t, e)
	require.Contains(t, string(reviewed.Payload), "evidence_spans")
	edited, e := f.s.UpdateTagging(ctx, f.p, tagID, reviewed.ID, d.TaggingResult{Tags: []string{"财务"}}, reviewed.Version)
	require.NoError(t, e)
	require.NoError(t, json.Unmarshal(edited.Payload, &result))
	require.Equal(t, "pending", result.ReviewStatus)
	require.Equal(t, "manual", result.Provenance.DecisionStatus)
	require.Zero(t, result.ReviewedBy)
}
func command(t *testing.T, f *fixture, a *ga.Adapter, source l.DataAssetRef, params *tc.GovernanceParameters) tc.ExecutionCommand {
	t.Helper()
	cap := a.Capability()
	if params != nil && params.StandardVersionID == "" {
		params.StandardVersionID = f.standard(t, strings.TrimPrefix(cap.ID, "governance."), params.LabelSchema)
	}
	out := tc.ExecutionCommand{Scope: m.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}, TaskID: uuid.NewString(), PlanVersion: 1, Attempt: 1, InputRefs: []l.RefID{source.RefID}, Deadline: time.Now().UTC().Add(3 * time.Minute), Node: tc.Node{ID: "label", Capability: cap.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &source.RefID, Kind: source.Kind}}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 34, TimeoutSeconds: 120, Parameters: &tc.NodeParameters{Governance: params}}}
	seal(&out)
	return out
}
func seal(c *tc.ExecutionCommand) {
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	c.OperationKey = m.Digest(raw)
	raw, _ = json.Marshal(c)
	raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	c.PayloadHash = m.Digest(raw)
}
func TestLabelingGovernedIntegrationDurableCandidatesAndCancellation(t *testing.T) {
	for _, cap := range []string{"governance.classification", "governance.named_entity", "governance.qa", "governance.tagging"} {
		t.Run(cap, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			a, e := ga.New(f.s, f.assets, cap)
			require.NoError(t, e)
			source := f.input(t, l.Input{Kind: "text", Text: "小李带来invoice"})
			params := &tc.GovernanceParameters{LabelSchema: []d.Label{{ID: "person", Name: "人物", Keywords: []string{"invoice"}, Terms: []string{"小李"}}}}
			if cap == "governance.qa" {
				source = f.input(t, l.Input{Kind: "table", Columns: []string{"q", "a"}, Rows: [][]any{{"编号", "00123"}}})
				params = &tc.GovernanceParameters{QAMapping: &d.QAMapping{QuestionColumn: "q", ReferenceAnswerColumn: "a"}}
			}
			c := command(t, f, a, source, params)
			f.q.fail.Store(true)
			_, e = a.Submit(ctx, c)
			require.Error(t, e)
			h, e := a.Lookup(ctx, c)
			require.NoError(t, e)
			require.NotNil(t, h)
			f.q.fail.Store(false)
			again, e := a.Submit(ctx, c)
			require.NoError(t, e)
			require.Equal(t, h, again)
			require.NoError(t, f.s.Handle(ctx, f.q.get(h.DomainID)))
			out, e := a.Inspect(ctx, c, *h)
			require.NoError(t, e)
			require.Equal(t, "succeeded", out.Status)
			require.Len(t, out.OutputRefs, 1)
			_, body, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, out.OutputRefs[0])
			require.NoError(t, e)
			require.Contains(t, string(body), `"review_required":true`)
			require.Contains(t, string(body), `"dataset_published":false`)
			require.Contains(t, string(body), `"knowledge_labels_applied":false`)
			var state d.GovernedExecution
			op, e := f.s.Repo.Get(ctx, f.p.TenantID, d.GovernedKind, h.DomainID)
			require.NoError(t, e)
			require.NoError(t, json.Unmarshal(op.Payload, &state))
			kind := "annotation_item"
			if cap == "governance.tagging" {
				kind = "tagging_result"
			}
			page, e := f.s.List(ctx, f.p, d.Filter{Kind: kind, ParentID: state.DomainID, Limit: 100})
			require.NoError(t, e)
			require.Len(t, page.Items, 1)
			require.Equal(t, "pending", page.Items[0].Status)
			require.NoError(t, f.s.Handle(ctx, f.q.get(h.DomainID)))
			cancel, e := a.Cancel(ctx, c, *h)
			require.NoError(t, e)
			require.Equal(t, "succeeded", cancel.Status)
			require.Equal(t, out.OutputRefs, cancel.OutputRefs)
			f.revoked.Store(true)
			_, e = a.Inspect(ctx, c, *h)
			require.ErrorIs(t, e, m.ErrForbidden)
			cancel, e = a.Cancel(ctx, c, *h)
			require.NoError(t, e)
			require.Equal(t, "succeeded", cancel.Status)
			require.Empty(t, cancel.OutputRefs)
			f.revoked.Store(false)
			c = command(t, f, a, source, params)
			h, e = a.CancelSubmission(ctx, c)
			require.NoError(t, e)
			late, e := a.Submit(ctx, c)
			require.NoError(t, e)
			require.Equal(t, h, late)
			cancel, e = a.Inspect(ctx, c, *h)
			require.NoError(t, e)
			require.Equal(t, "cancelled", cancel.Status)
			require.Empty(t, cancel.OutputRefs)
		})
	}
}

func TestLabelingQAMultilineMappingRoundtrip(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	pairs := []d.QAPair{{Question: "A\nreference_answer: B", ReferenceAnswer: "C"}, {Question: "A", ReferenceAnswer: "B\nreference_answer: C"}}
	rows := [][]any{}
	for _, pair := range pairs {
		rows = append(rows, []any{pair.Question, pair.ReferenceAnswer})
	}
	source := f.input(t, l.Input{Kind: "table", Columns: []string{"q", "a"}, Rows: rows})
	task := d.AnnotationTask{Name: "multiline QA", Type: "qa", SourceRefID: source.AssetRefID, QAMapping: &d.QAMapping{QuestionColumn: "q", ReferenceAnswerColumn: "a"}}
	id := responseID(t, f.http(t, f.p.UserID, "POST", "/annotation-tasks", task, "multiline"))
	require.Equal(t, 200, f.http(t, f.p.UserID, "POST", "/annotation-tasks/"+id+"/preannotate", nil, "").Code)
	page, e := f.s.List(ctx, f.p, d.Filter{Kind: "annotation_item", ParentID: id, Limit: 100})
	require.NoError(t, e)
	require.Len(t, page.Items, 2)
	got := []d.QAPair{}
	for _, row := range page.Items {
		var item d.AnnotationItem
		require.NoError(t, json.Unmarshal(row.Payload, &item))
		require.NotNil(t, item.Annotation.QA)
		got = append(got, *item.Annotation.QA)
	}
	require.ElementsMatch(t, pairs, got)
}

func featureCommand(t *testing.T, f *fixture) (*ga.Adapter, tc.ExecutionCommand) {
	t.Helper()
	ctx := context.Background()
	source := f.input(t, l.Input{Kind: "table", Columns: []string{"entity", "amount", "event"}, Rows: [][]any{{"00123", "9007199254740993.123456789012345678", "2026-01-01T01:00:00Z"}}})
	head, e := f.s.CreateFeature(ctx, f.p, d.Feature{Name: "exact", Type: "numerical", SourceRefID: source.AssetRefID, Definition: []byte(`{"kind":"expression","entity_field":"entity","event_time_field":"event","expression":{"op":"add","args":[{"op":"column","field":"amount"},{"op":"literal","value":0.1}]}}`)}, uuid.NewString())
	require.NoError(t, e)
	a, e := ga.New(f.s, f.assets, "governance.feature-engineering")
	require.NoError(t, e)
	c := command(t, f, a, source, nil)
	c.Node.Parameters = &tc.NodeParameters{Feature: &tc.FeatureParameters{FeatureID: head.ID, Version: 1}}
	c.Node.MaxCallsPerAttempt = 3
	c.Node.TimeoutSeconds = 300
	c.PayloadHash = ""
	seal(&c)
	return a, c
}

func TestGovernedFeatureIntegrationRecoveryExhaustionAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, c := featureCommand(t, f)
	h, e := a.Submit(ctx, c)
	require.NoError(t, e)
	repo := f.s.Repo.(d.GovernedRepository)
	_, e = repo.UpdateGoverned(ctx, f.p, h.DomainID, func(op *d.Record, _ *d.GovernedExecution, _ d.Repository) error { op.Status = "running"; return nil })
	require.NoError(t, e)
	// Simulate a committed materialization followed by lost worker response.
	params := c.Node.Parameters.Feature
	committed, e := f.s.MaterializeGovernedFeature(ctx, f.p, params.FeatureID, params.Version, h.DomainID)
	require.NoError(t, e)
	f.revoked.Store(true)
	_, e = a.Inspect(ctx, c, *h)
	require.ErrorIs(t, e, m.ErrForbidden)
	pending, e := a.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "running", pending.Status)
	require.Empty(t, pending.OutputRefs)
	// Retry exhaustion must reconcile the SQL receipt under the operation lock,
	// even when ordinary reads are now revoked.
	require.NoError(t, f.s.Exhausted(ctx, f.q.get(h.DomainID), errors.New("lost response")))
	done, e := a.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "succeeded", done.Status)
	require.Empty(t, done.OutputRefs)
	f.revoked.Store(false)
	done, e = a.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "feature_execution", done.DomainKind)
	require.Len(t, done.OutputRefs, 1)
	require.Equal(t, committed.OutputRefID, done.OutputRefs[0].String())
	_, body, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, done.OutputRefs[0])
	require.NoError(t, e)
	require.Contains(t, string(body), "9007199254740993.223456789012345678")
	read := f.http(t, f.p.UserID, "GET", "/governance-executions/"+h.DomainID, nil, "")
	require.Equal(t, 200, read.Code)
	require.Contains(t, read.Body.String(), `"domain_kind":"feature_execution"`)
	var count int64
	require.NoError(t, f.db.Table("feature_materializations").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestGovernedFeatureIntegrationCancelAndExhaustionFence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	a, c := featureCommand(t, f)
	h, e := a.Submit(ctx, c)
	require.NoError(t, e)
	repo := f.s.Repo.(d.GovernedRepository)
	_, e = repo.UpdateGoverned(ctx, f.p, h.DomainID, func(op *d.Record, _ *d.GovernedExecution, _ d.Repository) error { op.Status = "running"; return nil })
	require.NoError(t, e)
	_, e = a.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.NoError(t, f.s.Exhausted(ctx, f.q.get(h.DomainID), context.Canceled))
	out, e := a.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, "cancelled", out.Status)
	_, e = f.s.MaterializeGovernedFeature(ctx, f.p, c.Node.Parameters.Feature.FeatureID, 1, h.DomainID)
	require.Error(t, e)
	var count int64
	require.NoError(t, f.db.Table("feature_materializations").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.Zero(t, count)
	// A queue delivered under the wrong user cannot reconcile or mutate the op.
	j, e := job.New(h.DomainID, "governance", map[string]any{"tenant_id": f.p.TenantID, "user_id": f.p.UserID + 1, "kind": d.GovernedKind, "id": h.DomainID})
	require.NoError(t, e)
	require.ErrorIs(t, f.s.Handle(ctx, j), d.ErrForbidden)
	require.ErrorIs(t, f.s.Exhausted(ctx, j, errors.New("failed")), d.ErrForbidden)
}

func TestGovernedFeatureIntegrationConcurrentCommitAndExhaustion(t *testing.T) {
	for i := 0; i < 5; i++ {
		f := setup(t)
		ctx := context.Background()
		a, c := featureCommand(t, f)
		h, e := a.Submit(ctx, c)
		require.NoError(t, e)
		repo := f.s.Repo.(d.GovernedRepository)
		_, e = repo.UpdateGoverned(ctx, f.p, h.DomainID, func(op *d.Record, _ *d.GovernedExecution, _ d.Repository) error { op.Status = "running"; return nil })
		require.NoError(t, e)
		var wg sync.WaitGroup
		var materializeErr, exhaustErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, materializeErr = f.s.MaterializeGovernedFeature(ctx, f.p, c.Node.Parameters.Feature.FeatureID, 1, h.DomainID)
		}()
		go func() {
			defer wg.Done()
			exhaustErr = f.s.Exhausted(ctx, f.q.get(h.DomainID), errors.New("delivery exhausted"))
		}()
		wg.Wait()
		require.NoError(t, exhaustErr)
		receipt, e := f.s.LookupFeatureMaterialization(ctx, f.p, "governed:"+h.DomainID)
		require.NoError(t, e)
		out, e := a.Inspect(ctx, c, *h)
		require.NoError(t, e)
		if receipt != nil {
			require.NoError(t, materializeErr)
			require.Equal(t, "succeeded", out.Status)
			require.Len(t, out.OutputRefs, 1)
		} else {
			require.Error(t, materializeErr)
			require.Equal(t, "failed", out.Status)
			require.Empty(t, out.OutputRefs)
		}
	}
}

func TestLabelingIntegrationListsExcludeOtherGovernedOwners(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.sharedUser.Store(true)
	source := f.input(t, l.Input{Kind: "text", Text: "invoice"})
	schema := []d.Label{{ID: "finance", Name: "Finance", Keywords: []string{"invoice"}}}
	for _, kind := range []string{"annotation_task", "tagging_task"} {
		path, profile := "/annotation-tasks", d.ClassificationProfile
		var request any = d.AnnotationTask{Name: "shared ordinary", Type: "classification", SourceRefID: source.AssetRefID, Labels: schema}
		if kind == "tagging_task" {
			path = "/tagging-tasks"
			profile = d.TaggingProfile
			request = d.TaggingTask{Name: "shared ordinary", SourceRefID: source.AssetRefID, Config: d.TaggingConfig{EnableTags: true, LabelSchema: schema}}
		}
		sharedID := responseID(t, f.http(t, f.p.UserID+1, "POST", path, request, "shared-"+kind))
		privateIDs := map[int64]string{}
		for _, user := range []int64{f.p.UserID, f.p.UserID + 1} {
			in := d.GovernedInvocation{OperationKey: m.Digest([]byte(uuid.NewString())), PayloadHash: strings.Repeat("b", 64), Profile: profile, InputRef: source.RefID, LabelSchema: schema, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, CallsReserved: 34}
			op, e := f.s.SubmitGoverned(ctx, l.Principal{TenantID: f.p.TenantID, UserID: user}, in, true)
			require.NoError(t, e)
			var state d.GovernedExecution
			require.NoError(t, json.Unmarshal(op.Payload, &state))
			privateIDs[user] = state.DomainID
		}
		for _, user := range []int64{f.p.UserID, f.p.UserID + 1} {
			w := f.http(t, user, "GET", path, nil, "")
			require.Equal(t, 200, w.Code, w.Body.String())
			var response struct {
				Data struct {
					Items []struct {
						ID string `json:"id"`
					} `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			ids := []string{}
			for _, item := range response.Data.Items {
				ids = append(ids, item.ID)
			}
			require.ElementsMatch(t, []string{sharedID, privateIDs[user]}, ids)
			// Filtering precedes cursor/limit calculation; invisible tasks cannot
			// poison a page or suppress an accessible shared task.
			cursor := ""
			seen := []string{}
			for {
				page, e := f.s.List(ctx, l.Principal{TenantID: f.p.TenantID, UserID: user}, d.Filter{Kind: kind, Cursor: cursor, Limit: 1})
				require.NoError(t, e)
				for _, item := range page.Items {
					seen = append(seen, item.ID)
				}
				if !page.HasMore {
					break
				}
				cursor = page.NextCursor
			}
			require.ElementsMatch(t, ids, seen)
		}
		require.Equal(t, 403, f.http(t, f.p.UserID, "GET", path+"/"+privateIDs[f.p.UserID+1], nil, "").Code)
	}
}

func TestStandaloneStageKeysDoNotReturnAnotherUsersReceipt(t *testing.T) {
	f := setup(t)
	f.sharedUser.Store(true)
	ctx := context.Background()
	source := f.input(t, l.Input{Kind: "text", Text: "shared authorized data"})
	_, err := f.assets.Invoke(ctx, f.p, "data-quality", []l.RefID{source.RefID}, nil, "legacy-shared-key", "")
	require.NoError(t, err)
	other := f.p
	other.UserID++
	_, err = f.assets.Invoke(ctx, other, "data-quality", []l.RefID{source.RefID}, nil, "legacy-shared-key", "")
	require.ErrorIs(t, err, l.ErrConflict)
	var invoke tool.InvokableTool
	for _, item := range scenariotools.New(f.s, f.assets, nil) {
		info, e := item.Info(ctx)
		require.NoError(t, e)
		if info.Name == "data_stage_run" {
			invoke = item.(tool.InvokableTool)
		}
	}
	require.NotNil(t, invoke)
	body, _ := json.Marshal(scenariotools.StageInput{AgentID: "data-quality", InputRefs: []l.RefID{source.RefID}, IdempotencyKey: "same-key"})
	ids := []string{}
	for _, p := range []l.Principal{f.p, other} {
		callCtx := agentctx.WithUserID(agentctx.WithTenantID(ctx, p.TenantID), p.UserID)
		out, e := invoke.InvokableRun(callCtx, string(body))
		require.NoError(t, e)
		var stage l.StageRun
		require.NoError(t, json.Unmarshal([]byte(out), &stage))
		require.Equal(t, p.UserID, stage.UserID)
		ids = append(ids, stage.ID)
		replay, e := invoke.InvokableRun(callCtx, string(body))
		require.NoError(t, e)
		var same l.StageRun
		require.NoError(t, json.Unmarshal([]byte(replay), &same))
		require.Equal(t, stage.ID, same.ID)
	}
	require.NotEqual(t, ids[0], ids[1])
}
