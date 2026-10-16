//go:build integration

package handler

import (
	"bytes"
	d "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	ev "cognida/internal/service/evaluation"
	gov "cognida/internal/service/governance"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Real MySQL and production HTTP/service/repositories; synthetic authorization
// resolver is deliberate. Full source import/LLM/browser checks are separate.
type reviewedAssets struct {
	ref   l.DataAssetRef
	input l.Input
	deny  bool
}

func (a *reviewedAssets) Resolve(_ context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if a.deny || tenant != a.ref.TenantID || id != a.ref.RefID {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	raw, _ := json.Marshal(a.input)
	return a.ref, raw, nil
}
func (a *reviewedAssets) ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error) {
	return l.DataAssetRef{}, l.ErrInvalid
}

type reviewedFixture struct {
	db     *gorm.DB
	p      l.Principal
	assets *reviewedAssets
	gs     *gov.Service
	vs     *ev.VersionService
	es     *ev.Service
	router *gin.Engine
}

func newReviewedFixture(t *testing.T) *reviewedFixture {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN required")
	}
	require.True(t, strings.Contains(dsn, "/cognida_review_"), "isolated review database required")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 7191}
	var version int
	require.NoError(t, db.Raw("SELECT version FROM schema_migrations").Scan(&version).Error)
	require.GreaterOrEqual(t, version, 18)
	t.Cleanup(func() {
		for _, table := range []string{"knowledge_applied_labels", "knowledge_label_applications", "evaluation_tasks", "evaluation_dataset_records", "evaluation_datasets", "evaluation_dataset_versions", "evaluation_dataset_heads", "governance_records", "chunks", "knowledge", "knowledge_bases"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	id := l.RefID{AssetID: uuid.NewString(), Version: 1}
	assets := &reviewedAssets{ref: l.DataAssetRef{RefID: id, AssetRefID: id.String(), TenantID: p.TenantID, Policy: l.PolicySnapshot{Approved: true}, Provenance: l.Provenance{SourceType: "manual", Digest: "source"}, CreatedAt: time.Now().UTC()}, input: l.Input{Kind: "table", Columns: []string{"q", "a"}, Rows: [][]any{{"How much?", "42"}}}}
	gs := gov.NewService(repo.NewGovernanceRepository(db), assets, nil, nil, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }).WithKnowledgeLabels(repo.NewKnowledgeLabelRepository(db))
	vs := ev.NewVersionService(repo.NewDatasetVersionRepository(db), gs, assets, func(context.Context, l.Principal, g.EvaluationProfile) error { return nil })
	ss := ev.NewSnapshotService(vs, nil, func(context.Context, l.Principal, *ev.EvaluationTaskConfig) (json.RawMessage, error) {
		return json.RawMessage(`{"model":"synthetic-target","version":"1"}`), nil
	})
	es := ev.NewService(nil, repo.NewEvaluationTaskRepository(db), repo.NewEvaluationResultRepository(db), nil, nil, nil).WithSnapshots(ss)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		user := p.UserID
		if c.GetHeader("X-Other-User") == "1" {
			user++
		}
		tenant := p.TenantID
		if c.GetHeader("X-Other-Tenant") == "1" {
			tenant++
		}
		c.Set("user_id", user)
		c.Set("tenant_id", tenant)
		c.Next()
	})
	api := engine.Group("/api")
	gh := NewGovernanceHandler(gs)
	gh.Register(api)
	gh.RegisterKnowledgeLabelRoutes(api)
	NewDatasetVersionHandler(vs).Register(api)
	eh := NewEvaluationHandler(es, nil, nil, nil)
	api.POST("/evaluation/tasks", eh.CreateTask)
	api.GET("/evaluation/tasks/:id", eh.GetTask)
	return &reviewedFixture{db, p, assets, gs, vs, es, engine}
}
func (f *reviewedFixture) request(t *testing.T, method, path, key string, body any, headers ...string) (int, json.RawMessage) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	for _, h := range headers {
		r.Header.Set(h, "1")
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code >= 500 {
		t.Log(w.Body.String())
	}
	return w.Code, out.Data
}
func (f *reviewedFixture) approvedQA(t *testing.T) (string, string, int) {
	ctx := context.Background()
	task, e := f.gs.CreateAnnotation(ctx, f.p, g.AnnotationTask{Name: "reviewed qa", Type: "qa", SourceRefID: f.assets.ref.AssetRefID, QAMapping: &g.QAMapping{QuestionColumn: "q", ReferenceAnswerColumn: "a"}}, uuid.NewString())
	require.NoError(t, e)
	require.NoError(t, f.gs.Preannotate(ctx, f.p, task.ID))
	page, e := f.gs.List(ctx, f.p, g.Filter{Kind: "annotation_item", ParentID: task.ID, Limit: 10})
	require.NoError(t, e)
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	approved, e := f.gs.ReviewAnnotation(ctx, f.p, task.ID, item.ID, "approved", "checked", item.Version)
	require.NoError(t, e)
	return task.ID, item.ID, approved.Version
}
func TestReviewedDatasetVersionMySQLHTTPFreezeIsolationAndRevocation(t *testing.T) {
	f := newReviewedFixture(t)
	task, item, version := f.approvedQA(t)
	in := d.PublishDatasetVersion{DatasetID: "review-" + uuid.NewString(), Name: "Reviewed QA", Profile: "qa", Samples: []d.SampleSelection{{TaskID: task, ItemID: item, ExpectedVersion: version}}}
	code, raw := f.request(t, "POST", "/evaluation/dataset-versions", "publish", in)
	require.Equal(t, 200, code)
	var first d.DatasetVersion
	require.NoError(t, json.Unmarshal(raw, &first))
	require.Equal(t, 1, first.Version)
	require.Equal(t, "42", first.QAPairs[0].ReferenceAnswer)
	code, raw = f.request(t, "POST", "/evaluation/dataset-versions", "publish", in)
	require.Equal(t, 200, code)
	var replay d.DatasetVersion
	require.NoError(t, json.Unmarshal(raw, &replay))
	require.Equal(t, first.ID, replay.ID)
	changed := in
	changed.Name = "changed"
	code, _ = f.request(t, "POST", "/evaluation/dataset-versions", "publish", changed)
	require.Equal(t, 409, code)
	code, _ = f.request(t, "GET", "/evaluation/dataset-versions/"+first.ID, "", nil, "X-Other-User")
	require.Equal(t, 404, code)
	code, _ = f.request(t, "GET", "/evaluation/dataset-versions/"+first.ID, "", nil, "X-Other-Tenant")
	require.Equal(t, 404, code)
	_, e := f.gs.UpdateAnnotation(context.Background(), f.p, task, item, g.Annotation{QA: &g.QAPair{Question: "How much?", ReferenceAnswer: "99"}}, version)
	require.NoError(t, e)
	code, raw = f.request(t, "GET", "/evaluation/dataset-versions/"+first.ID, "", nil)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), `"reference_answer":"42"`)
	code, raw = f.request(t, "POST", "/evaluation/tasks", "evaluate", map[string]any{"dataset_id": in.DatasetID, "dataset_version_id": first.ID, "type": "qa"})
	require.Equal(t, 201, code)
	var created struct {
		ID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(raw, &created))
	require.NotEmpty(t, created.ID)
	stored, e := repo.NewEvaluationTaskRepository(f.db).FindByID(context.Background(), created.ID)
	require.NoError(t, e)
	require.Contains(t, string(stored.Snapshot), first.ManifestDigest)
	require.Contains(t, string(stored.Snapshot), `"reference_answer":"42"`)
	code, _ = f.request(t, "GET", "/evaluation/tasks/"+created.ID, "", nil, "X-Other-User")
	require.Equal(t, 403, code)
	f.assets.deny = true
	code, _ = f.request(t, "GET", "/evaluation/dataset-versions/"+first.ID, "", nil)
	require.Equal(t, 403, code)
	code, _ = f.request(t, "GET", "/evaluation/tasks/"+created.ID, "", nil)
	require.Equal(t, 403, code)
	code, _ = f.request(t, "POST", "/evaluation/tasks", "denied", map[string]any{"dataset_id": in.DatasetID, "dataset_version_id": first.ID, "type": "qa"})
	require.NotEqual(t, 200, code)
}
func TestKnowledgeLabelApplicationMySQLHTTPAtomicAndStaleReview(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	kbID, docID := uuid.NewString(), uuid.NewString()
	require.NoError(t, f.db.Create(&repo.KnowledgeBaseModel{ID: kbID, TenantID: f.p.TenantID, UserID: f.p.UserID, Name: "test", Status: 1}).Error)
	require.NoError(t, f.db.Create(&repo.KnowledgeModel{ID: docID, TenantID: f.p.TenantID, UserID: f.p.UserID, KnowledgeBaseID: kbID, Type: "text", Title: "doc", Source: "test", EnableStatus: "enabled", ParseStatus: "completed"}).Error)
	require.NoError(t, f.db.Create(&repo.ChunkModel{ID: uuid.NewString(), TenantID: f.p.TenantID, KnowledgeID: docID, KnowledgeBaseID: kbID, Content: "sales facts", IsEnabled: true}).Error)
	f.assets.ref.Provenance.SourceType = "knowledge"
	f.assets.ref.Provenance.SourceID = docID
	f.assets.input = l.Input{Kind: "text", Text: "sales facts\n"}
	payload, _ := json.Marshal(g.TaggingTask{Name: "tags", SourceRefID: f.assets.ref.AssetRefID, Config: g.TaggingConfig{EnableTags: true, LabelSchema: []g.Label{{ID: "sales", Name: "sales"}}}})
	task, e := f.gs.Repo.Create(ctx, &g.Record{Meta: g.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "tagging_task", Status: "completed", Payload: payload})
	require.NoError(t, e)
	payload, _ = json.Marshal(g.TaggingResult{TaskID: task.ID, Content: "sales facts\n", Tags: []string{"sales"}, ReviewStatus: "pending"})
	result, e := f.gs.Repo.Create(ctx, &g.Record{Meta: g.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "tagging_result", ParentID: task.ID, Status: "pending", Payload: payload})
	require.NoError(t, e)
	approved, e := f.gs.ReviewTagging(ctx, f.p, result.ID, "approved", "checked", result.Version)
	require.NoError(t, e)
	in := g.ApplyKnowledgeLabels{KnowledgeID: docID, ExpectedVersion: approved.Version}
	path := "/tagging-results/" + result.ID + "/apply"
	code, raw := f.request(t, "POST", path, "apply", in)
	require.Equal(t, 200, code)
	var receipt g.KnowledgeLabelApplication
	require.NoError(t, json.Unmarshal(raw, &receipt))
	require.Equal(t, "applied", receipt.Status)
	code, raw = f.request(t, "POST", path, "apply", in)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), receipt.ID)
	labels, e := repo.NewKnowledgeLabelRepository(f.db).FilterKnowledgeLabels(ctx, f.p.TenantID, []string{docID}, []string{"sales"})
	require.NoError(t, e)
	require.True(t, labels[docID])
	code, raw = f.request(t, "GET", "/knowledge/"+docID+"/applied-labels", "", nil)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), "sales")
	// Real MySQL collation boundary without altering any permanent table or
	// database default. These connection-local tables copy migration structures.
	require.NoError(t, f.db.Connection(func(tx *gorm.DB) error {
		for _, table := range []string{"knowledge_label_applications", "knowledge_applied_labels", "governance_records", "knowledge", "knowledge_bases"} {
			if err := tx.Exec("CREATE TEMPORARY TABLE label_fixture_copy LIKE " + table).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO label_fixture_copy SELECT * FROM "+table+" WHERE tenant_id=?", f.p.TenantID).Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE TEMPORARY TABLE " + table + " LIKE label_fixture_copy").Error; err != nil {
				return err
			}
			collation := "utf8mb4_unicode_ci"
			if strings.HasPrefix(table, "knowledge_label") || table == "knowledge_applied_labels" {
				collation = "utf8mb4_0900_ai_ci"
			}
			if err := tx.Exec("ALTER TABLE " + table + " CONVERT TO CHARACTER SET utf8mb4 COLLATE " + collation).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO " + table + " SELECT * FROM label_fixture_copy").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TEMPORARY TABLE label_fixture_copy").Error; err != nil {
				return err
			}
			defer tx.Exec("DROP TEMPORARY TABLE " + table)
		}
		var same int
		// Prove the fixture exercised both collations, independent of DB defaults.
		if err := tx.Raw("SELECT COLLATION(a.result_id) <> COLLATION(g.id) FROM knowledge_label_applications a JOIN governance_records g ON BINARY a.result_id=BINARY g.id WHERE a.tenant_id=?", f.p.TenantID).Scan(&same).Error; err != nil {
			return err
		}
		require.Equal(t, 1, same)
		labels, err := repo.NewKnowledgeLabelRepository(tx).Labels(ctx, f.p, docID)
		require.NoError(t, err)
		require.Equal(t, []string{"sales"}, labels)
		matched, err := repo.NewKnowledgeLabelRepository(tx).FilterKnowledgeLabels(ctx, f.p.TenantID, []string{docID}, []string{"sales"})
		require.NoError(t, err)
		require.True(t, matched[docID])
		if err := tx.Exec("DELETE FROM knowledge_applied_labels WHERE tenant_id=?", f.p.TenantID).Error; err != nil {
			return err
		}
		labels, err = repo.NewKnowledgeLabelRepository(tx).Labels(ctx, f.p, docID)
		require.NoError(t, err)
		require.Empty(t, labels)
		return nil
	}))
	in.KnowledgeID = uuid.NewString()
	code, _ = f.request(t, "POST", path, "apply", in)
	require.Equal(t, 403, code)
	_, e = f.gs.UpdateTagging(ctx, f.p, task.ID, result.ID, g.TaggingResult{Tags: []string{"new"}}, approved.Version)
	require.NoError(t, e)
	labels, e = repo.NewKnowledgeLabelRepository(f.db).FilterKnowledgeLabels(ctx, f.p.TenantID, []string{docID}, []string{"sales"})
	require.NoError(t, e)
	require.False(t, labels[docID])
	in.KnowledgeID = docID
	code, _ = f.request(t, "POST", path, "again", in)
	require.Equal(t, 409, code)
	var count int64
	require.NoError(t, f.db.Table("knowledge_label_applications").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.Equal(t, int64(1), count, fmt.Sprint(count))
}

func TestLegacyDatabaseSnapshotMySQLNeverRereadsMutableSamples(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	datasets := repo.NewDatasetRepository(f.db)
	id := "legacy-" + uuid.NewString()
	dataset := d.NewDataset(id, f.p.TenantID, f.p.UserID, "Legacy QA", d.EvaluationTypeQA)
	require.NoError(t, datasets.Create(ctx, dataset))
	require.NoError(t, datasets.AddSamples(ctx, id, []*d.DatasetRecord{d.NewDatasetRecord(id, f.p.TenantID, "original?", "00123")}))
	loader := ev.NewDatasetLoader(f.db, datasets)
	snapshots := ev.NewSnapshotService(f.vs, loader, func(context.Context, l.Principal, *ev.EvaluationTaskConfig) (json.RawMessage, error) {
		return json.RawMessage(`{"model":"fixed"}`), nil
	})
	config := &ev.EvaluationTaskConfig{DatasetID: id, Type: ev.EvaluationTypeQA}
	snapshot, e := snapshots.Freeze(ctx, f.p, config, "")
	require.NoError(t, e)
	require.Equal(t, "00123", snapshot.QAPairs[0].ReferenceAnswer)
	require.NoError(t, datasets.AddSamples(ctx, id, []*d.DatasetRecord{d.NewDatasetRecord(id, f.p.TenantID, "new?", "changed")}))
	_, e = snapshots.Validate(ctx, f.p, snapshot)
	require.NoError(t, e)
	require.Len(t, snapshot.QAPairs, 1)
	next, e := snapshots.Freeze(ctx, f.p, config, "")
	require.NoError(t, e)
	require.NotEqual(t, snapshot.DatasetVersionID, next.DatasetVersionID)
	require.Len(t, next.QAPairs, 2)
	_, e = snapshots.Freeze(ctx, l.Principal{TenantID: f.p.TenantID, UserID: f.p.UserID + 1}, config, "")
	require.ErrorIs(t, e, d.ErrVersionForbidden)
	require.NoError(t, datasets.SoftDelete(ctx, id, f.p.TenantID))
	_, e = snapshots.Validate(ctx, f.p, snapshot)
	require.ErrorIs(t, e, d.ErrVersionForbidden)
	_, e = f.vs.Read(ctx, f.p, snapshot.DatasetVersionID)
	require.ErrorIs(t, e, d.ErrVersionForbidden)
}

func TestReviewedDatasetVersionMySQLHTTPListSkipsRevokedWithCursor(t *testing.T) {
	f := newReviewedFixture(t)
	publish := func() d.DatasetVersion {
		task, item, version := f.approvedQA(t)
		in := d.PublishDatasetVersion{DatasetID: "list-" + uuid.NewString(), Name: "Reviewed QA", Profile: "qa", Samples: []d.SampleSelection{{TaskID: task, ItemID: item, ExpectedVersion: version}}}
		code, raw := f.request(t, "POST", "/evaluation/dataset-versions", uuid.NewString(), in)
		require.Equal(t, 200, code)
		var out d.DatasetVersion
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}
	first := publish()
	f.assets.ref.RefID = l.RefID{AssetID: uuid.NewString(), Version: 1}
	f.assets.ref.AssetRefID = f.assets.ref.RefID.String()
	second := publish()
	// Make whichever record sorts first inaccessible, so limit=1 must return an
	// empty visible page with the original cursor rather than poisoning the list.
	revoked, valid := first, second
	if second.ID < first.ID {
		revoked, valid = second, first
	}
	ref, err := l.ParseRefID(valid.SourceRefs[0])
	require.NoError(t, err)
	f.assets.ref.RefID = ref
	f.assets.ref.AssetRefID = ref.String()
	code, raw := f.request(t, "GET", "/evaluation/dataset-versions?page_size=20", "", nil)
	require.Equal(t, 200, code)
	var page d.VersionPage
	require.NoError(t, json.Unmarshal(raw, &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, valid.ID, page.Items[0].ID)
	require.NotContains(t, string(raw), revoked.ID)
	code, raw = f.request(t, "GET", "/evaluation/dataset-versions?page_size=1", "", nil)
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &page))
	require.Empty(t, page.Items)
	require.True(t, page.HasMore)
	require.NotEmpty(t, page.NextCursor)
	code, raw = f.request(t, "GET", "/evaluation/dataset-versions?page_size=1&cursor="+page.NextCursor, "", nil)
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &page))
	require.Len(t, page.Items, 1)
	require.Equal(t, valid.ID, page.Items[0].ID)
	require.False(t, page.HasMore)
	code, _ = f.request(t, "GET", "/evaluation/dataset-versions/"+revoked.ID, "", nil)
	require.Equal(t, 403, code)
	code, raw = f.request(t, "GET", "/evaluation/dataset-versions", "", nil, "X-Other-User")
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &page))
	require.Empty(t, page.Items)
}

func TestGovernanceMySQLHTTPListSkipsRevokedSources(t *testing.T) {
	f := newReviewedFixture(t)
	for _, tc := range []struct{ kind, path string }{{"annotation_task", "/annotation-tasks"}, {"tagging_task", "/tagging-tasks"}, {"feature", "/features"}} {
		t.Run(tc.kind, func(t *testing.T) {
			create := func(ref string) *g.Record {
				raw, _ := json.Marshal(map[string]string{"name": "visibility fixture", "source_ref_id": ref})
				r, err := f.gs.Repo.Create(context.Background(), &g.Record{Meta: g.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: tc.kind, Status: "completed", Payload: raw})
				require.NoError(t, err)
				return r
			}
			other := l.RefID{AssetID: uuid.NewString(), Version: 1}
			first := create(f.assets.ref.AssetRefID)
			second := create(other.String())
			// Repository orders IDs; bind only the latter record's source as readable.
			scanned, err := f.gs.Repo.List(context.Background(), f.p.TenantID, g.Filter{Kind: tc.kind, Limit: 2})
			require.NoError(t, err)
			require.Len(t, scanned.Items, 2)
			revoked, valid := scanned.Items[0], scanned.Items[1]
			var bound struct {
				SourceRefID string `json:"source_ref_id"`
			}
			require.NoError(t, json.Unmarshal(valid.Payload, &bound))
			id, err := l.ParseRefID(bound.SourceRefID)
			require.NoError(t, err)
			f.assets.ref.RefID = id
			f.assets.ref.AssetRefID = id.String()
			code, raw := f.request(t, "GET", tc.path+"?page_size=20", "", nil)
			require.Equal(t, 200, code)
			var page g.Page[map[string]any]
			require.NoError(t, json.Unmarshal(raw, &page))
			require.Len(t, page.Items, 1)
			require.Equal(t, valid.ID, page.Items[0]["id"])
			require.NotContains(t, string(raw), revoked.ID)
			code, raw = f.request(t, "GET", tc.path+"?page_size=1", "", nil)
			require.Equal(t, 200, code)
			require.NoError(t, json.Unmarshal(raw, &page))
			require.Empty(t, page.Items)
			require.True(t, page.HasMore)
			require.NotEmpty(t, page.NextCursor)
			code, raw = f.request(t, "GET", tc.path+"?page_size=1&cursor="+page.NextCursor, "", nil)
			require.Equal(t, 200, code)
			require.NoError(t, json.Unmarshal(raw, &page))
			require.Len(t, page.Items, 1)
			require.Equal(t, valid.ID, page.Items[0]["id"])
			code, _ = f.request(t, "GET", tc.path+"/"+revoked.ID, "", nil)
			require.Equal(t, 403, code)
			_ = first
			_ = second
		})
	}
}
