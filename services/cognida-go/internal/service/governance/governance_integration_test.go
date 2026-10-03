//go:build integration

package governance_test

import (
	"bytes"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	objectstore "cognida/internal/model/objectstore"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/governance"
	lifecycle "cognida/internal/service/lifecycle"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"

	"testing"
	"time"
)

type fixture struct {
	s       *svc.Service
	assets  *lifecycle.Service
	db      *gorm.DB
	q       *job.RedisQueue
	objects objectstore.Store
	p       l.Principal
}
type computeFixture struct{}

func (computeFixture) Call(context.Context, string, l.Input, map[string]any, string) (lifecycle.ComputeResult, error) {
	return lifecycle.ComputeResult{SchemaVersion: "1", Method: "rules", Result: map[string]any{"tags": map[string]any{"value": []string{"sales"}, "confidence": json.Number("0.7"), "evidence": []string{"sales"}}, "summary": map[string]any{"value": "sales observation"}}}, nil
}
func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	require.Contains(t, dsn, "/cognida_review_", "isolated review database is required")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 991}
	t.Cleanup(func() {
		for _, table := range []string{"feature_values", "governance_records", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: os.Getenv("REDIS_PASSWORD")})
	require.NoError(t, client.Ping(context.Background()).Err())
	prefix := "cognida:governance-test:" + uuid.NewString()
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
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	auth := func(_ context.Context, p l.Principal, scope string) error {
		if scope != "read" && p.UserID != 991 {
			return l.ErrForbidden
		}
		return p.Validate()
	}
	assets, e := lifecycle.NewService(repo.NewLifecycleRepository(db), objects, q, lifecycle.Config{Authorize: auth})
	require.NoError(t, e)
	return &fixture{s: svc.NewService(scopedPendingRepo{Repository: repo.NewGovernanceRepository(db), tenant: p.TenantID}, assets, computeFixture{}, q, auth), assets: assets, db: db, q: q, objects: objects, p: p}
}
func (f *fixture) publish(t *testing.T, input l.Input, source, sensitivity string) l.DataAssetRef {
	t.Helper()
	ctx := context.Background()
	raw, e := json.Marshal(input)
	require.NoError(t, e)
	sum := sha256.Sum256(raw)
	id := l.RefID{AssetID: uuid.NewString(), Version: 1}
	key := "test/" + id.AssetID
	ref := l.DataAssetRef{RefID: id, AssetRefID: id.String(), SchemaVersion: "1", TenantID: f.p.TenantID, Kind: l.RawAsset, Status: "available", ProducerID: id.AssetID, Policy: l.PolicySnapshot{Sensitivity: sensitivity, Approved: true, Scopes: []string{"read"}}, Provenance: l.Provenance{SourceType: "datasource", SourceID: source, Digest: hex.EncodeToString(sum[:])}, CreatedAt: time.Now().UTC(), ObjectKey: key}
	_, e = f.objects.Put(ctx, objectstore.PutRequest{Key: key, Body: bytes.NewReader(raw), Size: int64(len(raw)), ContentType: "application/json"})
	require.NoError(t, e)
	require.NoError(t, f.assets.Repository().PutAsset(ctx, ref))
	published, e := f.assets.ImportDerived(ctx, f.p, l.PublishedAsset, uuid.NewString(), []l.RefID{id}, raw, ref.Policy)
	require.NoError(t, e)
	return published
}
func (f *fixture) api(t *testing.T, user, tenant int64, method, path string, body any) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("user_id", user); c.Set("tenant_id", tenant); c.Next() })
	handler.NewGovernanceHandler(f.s).Register(engine.Group("/api"))
	raw, e := json.Marshal(body)
	require.NoError(t, e)
	req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return w.Code, out
}
func TestGovernanceIntegrationFeatureVersionsValuesAndPolicies(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ref := f.publish(t, l.Input{Kind: "table", Columns: []string{"entity", "amount"}, Rows: [][]any{{"a", 42}}}, "db/sales", "restricted")
	input := d.Feature{Name: "amount", Type: "numerical", Definition: json.RawMessage(`{"kind":"field","field":"amount","entity_field":"entity"}`), SourceRefID: ref.AssetRefID}
	head, e := f.s.CreateFeature(ctx, f.p, input, "feature")
	require.NoError(t, e)
	replay, e := f.s.CreateFeature(ctx, f.p, input, "feature")
	require.NoError(t, e)
	require.Equal(t, head.ID, replay.ID)
	input.Description = "second"
	head, e = f.s.NewFeatureVersion(ctx, f.p, head.ID, input, 1)
	require.NoError(t, e)
	require.Equal(t, 2, head.Version)
	code, out := f.api(t, 991, f.p.TenantID, "GET", "/features/"+head.ID+"/versions", nil)
	require.Equal(t, 200, code, out)
	items := out["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 2)
	versions := map[float64]bool{}
	for _, v := range items {
		versions[v.(map[string]any)["version"].(float64)] = true
	}
	require.True(t, versions[2])
	at := time.Now().Add(-time.Hour)
	value := d.FeatureValue{FeatureID: head.ID, Version: 2, EntityID: "entity-a", Value: json.RawMessage(`42.0`), Timestamp: at}
	require.NoError(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{value}))
	value.Value = json.RawMessage(`4.2e1`)
	require.NoError(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{value}))
	value.Value = json.RawMessage(`43`)
	require.ErrorIs(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{value}), d.ErrConflict)
	query := map[string]any{"entity_ids": []string{"entity-a"}, "features": []map[string]any{{"feature_id": head.ID, "version": 2}}}
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/feature-values/query", query)
	require.Equal(t, 200, code, out)
	require.Len(t, out["data"].(map[string]any)["items"], 1)
	code, _ = f.api(t, 992, f.p.TenantID, "POST", "/feature-values/query", query)
	require.Equal(t, 403, code)
	code, _ = f.api(t, 991, f.p.TenantID+1, "POST", "/feature-values/query", query)
	require.Equal(t, 404, code)
	// The database's canonical JSON whitespace must not break vector idempotency.
	input.Type = "vector"
	input.Name = "vector"
	vector, e := f.s.CreateFeature(ctx, f.p, input, "vector")
	require.NoError(t, e)
	v := d.FeatureValue{FeatureID: vector.ID, Version: 1, EntityID: "e", Value: json.RawMessage(`[1,2.0,3]`), Timestamp: at}
	require.NoError(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{v}))
	v.Value = json.RawMessage(`[1.0, 2, 3.0]`)
	require.NoError(t, f.s.WriteFeatures(ctx, f.p, []d.FeatureValue{v}))
}

type failRepo struct {
	d.Repository
	failVersion, failTerminal *bool
}

func (r failRepo) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return r.Repository.Transaction(ctx, func(tx d.Repository) error { return fn(failRepo{tx, r.failVersion, r.failTerminal}) })
}
func (r failRepo) Create(ctx context.Context, v *d.Record) (*d.Record, error) {
	if r.failVersion != nil && *r.failVersion && v.Kind == "feature_version" {
		*r.failVersion = false
		return nil, errors.New("injected version storage failure")
	}
	return r.Repository.Create(ctx, v)
}
func (r failRepo) Update(ctx context.Context, v *d.Record, expected int) error {
	if r.failTerminal != nil && *r.failTerminal && v.Kind == "subscription_run" && v.Status == "completed" {
		*r.failTerminal = false
		return errors.New("injected terminal failure")
	}
	return r.Repository.Update(ctx, v, expected)
}
func TestGovernanceIntegrationAtomicFeatureVersion(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ref := f.publish(t, l.Input{Kind: "table", Columns: []string{"x"}, Rows: [][]any{{1}}}, "db/x", "public")
	in := d.Feature{Name: "x", Type: "numerical", SourceRefID: ref.AssetRefID, Definition: json.RawMessage(`{"kind":"field","field":"x"}`)}
	head, e := f.s.CreateFeature(ctx, f.p, in, "x")
	require.NoError(t, e)
	fail := true
	f.s.Repo = failRepo{Repository: f.s.Repo, failVersion: &fail}
	in.Description = "failed"
	_, e = f.s.NewFeatureVersion(ctx, f.p, head.ID, in, 1)
	require.Error(t, e)
	stored, e := f.s.Get(ctx, f.p, "feature", head.ID)
	require.NoError(t, e)
	require.Equal(t, 1, stored.Version)
	in.Description = "retry with corrected proposal"
	head, e = f.s.NewFeatureVersion(ctx, f.p, head.ID, in, 1)
	require.NoError(t, e)
	require.Equal(t, 2, head.Version)
}
func TestGovernanceIntegrationAnnotationsTaggingAndAPI(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ref := f.publish(t, l.Input{Kind: "text", Text: "sales increased"}, "db/text", "restricted")
	task, e := f.s.CreateAnnotation(ctx, f.p, d.AnnotationTask{Name: "review", Type: "classification", SourceRefID: ref.AssetRefID, Labels: []d.Label{{ID: "sales", Name: "sales"}}}, "labels")
	require.NoError(t, e)
	require.NoError(t, f.s.Preannotate(ctx, f.p, task.ID))
	code, out := f.api(t, 991, f.p.TenantID, "GET", "/annotation-tasks/"+task.ID+"/items", nil)
	require.Equal(t, 200, code, out)
	item := out["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	code, _ = f.api(t, 992, f.p.TenantID, "GET", "/annotation-tasks/"+task.ID+"/items", nil)
	require.Equal(t, 403, code)
	reviewed, e := f.s.ReviewAnnotation(ctx, f.p, task.ID, item["id"].(string), "approved", "verified", int(item["version"].(float64)))
	require.NoError(t, e)
	require.Equal(t, "approved", reviewed.Status)
	tagging, e := f.s.CreateTagging(ctx, f.p, d.TaggingTask{Name: "tag", SourceRefID: ref.AssetRefID, Config: d.TaggingConfig{EnableTags: true, TagSchema: []string{"sales"}}}, "tag")
	require.NoError(t, e)
	j, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	require.NoError(t, f.s.Handle(ctx, *j))
	require.NoError(t, f.q.Ack(ctx, *j))
	code, out = f.api(t, 991, f.p.TenantID, "GET", "/tagging-tasks/"+tagging.ID+"/results", nil)
	require.Equal(t, 200, code, out)
	result := out["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	require.Equal(t, "sales", result["tags"].([]any)[0])
	require.Equal(t, 0.7, result["tag_confidence"].(map[string]any)["sales"])
	require.NotNil(t, result["evidence"])
	code, _ = f.api(t, 992, f.p.TenantID, "GET", "/tagging-tasks/"+tagging.ID+"/results", nil)
	require.Equal(t, 403, code)
}
func TestGovernanceIntegrationSubscriptionRefreshCheckpoint(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	input := l.Input{Kind: "table", Columns: []string{"day", "amount"}, Rows: [][]any{{"2026-01-01", 10}, {"2026-01-02", 10}, {"2026-01-03", 10}, {"2026-01-04", 100}}}
	anchor := f.publish(t, input, "db/refresh", "public")
	latest := f.publish(t, input, "db/refresh", "public")
	f.publish(t, input, "db/other", "public")
	subscription, e := f.s.SaveSubscription(ctx, f.p, "", "subscription", d.Subscription{Name: "watch", Kind: "metric_watch", Enabled: true, Channel: "in_app", Schedule: d.Schedule{Cron: "0 * * * *", Timezone: "UTC"}, Input: d.AnalysisInput{AssetRefID: anchor.AssetRefID, TimeColumn: "day", ValueColumn: "amount", Refresh: &d.RefreshSource{Mode: "latest_published", SourceType: "datasource", SourceID: "db/refresh"}}, Watch: &d.Watch{Method: "zscore", Threshold: 3}}, 0)
	require.NoError(t, e)
	run, e := f.s.RunSubscription(ctx, f.p, subscription.ID, "manual")
	require.NoError(t, e)
	j, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	fail := true
	f.s.Repo = failRepo{Repository: f.s.Repo, failTerminal: &fail}
	require.ErrorContains(t, f.s.Handle(ctx, *j), "terminal failure")
	stored, e := f.s.Get(ctx, f.p, "subscription_run", run.ID)
	require.NoError(t, e)
	var body map[string]any
	require.NoError(t, json.Unmarshal(stored.Payload, &body))
	require.Equal(t, latest.AssetRefID, body["resolved_ref_id"])
	f.publish(t, input, "db/refresh", "public")
	require.NoError(t, f.s.Handle(ctx, *j))
	require.NoError(t, f.q.Ack(ctx, *j))
	stored, e = f.s.Get(ctx, f.p, "subscription_run", run.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", stored.Status)
	require.NoError(t, json.Unmarshal(stored.Payload, &body))
	require.Equal(t, latest.AssetRefID, body["resolved_ref_id"])
	require.True(t, body["alert_triggered"].(bool))
	page, e := f.s.List(ctx, f.p, d.Filter{Kind: "notification", Limit: 100})
	require.NoError(t, e)
	require.Len(t, page.Items, 1)
	_, e = f.s.Get(ctx, l.Principal{TenantID: f.p.TenantID, UserID: 992}, "subscription_run", run.ID)
	require.ErrorIs(t, e, d.ErrNotFound)
	code, out := f.api(t, 991, f.p.TenantID, "POST", "/notifications/"+page.Items[0].ID+"/read", nil)
	require.Equal(t, 200, code, out)
}

type failQueue struct {
	job.Queue
	fail bool
}

func (q *failQueue) Enqueue(ctx context.Context, j job.Job) error {
	if q.fail {
		q.fail = false
		return errors.New("queue temporarily unavailable")
	}
	return q.Queue.Enqueue(ctx, j)
}
func TestGovernanceIntegrationQueueRecoveryAndPublishedCursor(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ref := f.publish(t, l.Input{Kind: "text", Text: "sales"}, "db/recover", "public")
	// Multiple versions of one asset must survive the 100-item page boundary.
	for version := ref.Version + 1; version <= ref.Version+101; version++ {
		copy := ref
		copy.Version = version
		copy.AssetRefID = copy.RefID.String()
		require.NoError(t, f.assets.Repository().PutAsset(ctx, copy))
	}
	refs, e := f.s.Repo.PublishedRefs(ctx, f.p.TenantID, "", 100)
	require.NoError(t, e)
	require.Len(t, refs, 100)
	next, e := f.s.Repo.PublishedRefs(ctx, f.p.TenantID, refs[len(refs)-1], 100)
	require.NoError(t, e)
	require.Len(t, next, 2)
	require.Equal(t, (l.RefID{AssetID: ref.AssetID, Version: ref.Version + 101}).String(), next[1])
	f.s.Queue = &failQueue{Queue: f.q, fail: true}
	_, e = f.s.CreateTagging(ctx, f.p, d.TaggingTask{Name: "recover", SourceRefID: ref.AssetRefID, Config: d.TaggingConfig{EnableTags: true}}, "recover")
	require.ErrorContains(t, e, "queue temporarily unavailable")
	dispatcher := job.NewDispatcher(f.q, job.RetryPolicy{MaxAttempts: 3})
	dispatcher.Register("governance", f.s, f.s)
	n, e := dispatcher.Recover(ctx, 100)
	require.NoError(t, e)
	require.GreaterOrEqual(t, n, 1)
	j, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	require.NoError(t, f.s.Handle(ctx, *j))
	require.NoError(t, f.q.Ack(ctx, *j))
	records, e := f.s.List(ctx, f.p, d.Filter{Kind: "tagging_task", Limit: 100})
	require.NoError(t, e)
	require.Len(t, records.Items, 1)
	require.Equal(t, "completed", records.Items[0].Status)
}

func TestGovernanceIntegrationHTTPMutations(t *testing.T) {
	f := setup(t)
	ref := f.publish(t, l.Input{Kind: "table", Columns: []string{"amount"}, Rows: [][]any{{42}}}, "db/http", "public")
	code, out := f.api(t, 991, f.p.TenantID, "POST", "/features", map[string]any{"name": "http", "type": "numerical", "source_ref_id": ref.AssetRefID, "definition": map[string]any{"kind": "field", "field": "amount"}})
	require.Equal(t, 200, code, out)
	id := out["data"].(map[string]any)["id"].(string)
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/features/"+id+"/compute", map[string]any{"version": 1})
	require.Equal(t, 200, code, out)
	require.Equal(t, float64(1), out["data"].(map[string]any)["count"])
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/features/"+id+"/versions", map[string]any{"name": "http", "type": "numerical", "source_ref_id": ref.AssetRefID, "definition": map[string]any{"kind": "field", "field": "amount"}, "expected_version": 999})
	require.Equal(t, 409, code, out)
	var before int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind='annotation_task'", f.p.TenantID).Count(&before).Error)
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/annotation-tasks", map[string]any{"name": "bad", "type": "classification", "source_ref_id": "invalid", "labels": []map[string]any{{"id": "a", "name": "A"}}})
	require.Equal(t, 400, code, out)
	var after int64
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind='annotation_task'", f.p.TenantID).Count(&after).Error)
	require.Equal(t, before, after)
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/subscriptions", map[string]any{"name": "http report", "kind": "report", "enabled": false, "channel": "in_app", "schedule": map[string]any{"cron": "0 8 * * *", "timezone": "UTC"}, "input": map[string]any{"asset_ref_id": ref.AssetRefID}})
	require.Equal(t, 200, code, out)
	sub := out["data"].(map[string]any)["id"].(string)
	code, out = f.api(t, 991, f.p.TenantID, "POST", "/subscriptions/"+sub+"/run", nil)
	require.Equal(t, 200, code, out)
}

// Production recovery intentionally spans tenants; this test adapter scopes its
// source so a private queue never claims another test/application's review rows.
type scopedPendingRepo struct {
	d.Repository
	tenant int64
}

func (r scopedPendingRepo) Pending(ctx context.Context, limit int) ([]d.Record, error) {
	out := []d.Record{}
	for _, kind := range []string{"subscription_run", "tagging_task"} {
		for _, status := range []string{"queued", "running"} {
			page, e := r.Repository.List(ctx, r.tenant, d.Filter{Kind: kind, Status: status, Limit: limit})
			if e != nil {
				return nil, e
			}
			out = append(out, page.Items...)
		}
	}
	return out, nil
}

func TestGovernanceIntegrationTaggingReviewAPI(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ref := f.publish(t, l.Input{Kind: "text", Text: "sales"}, "db/review", "public")
	task, e := f.s.CreateTagging(ctx, f.p, d.TaggingTask{Name: "review", SourceRefID: ref.AssetRefID, Config: d.TaggingConfig{EnableTags: true}}, "review")
	require.NoError(t, e)
	j, e := f.q.TryClaim(ctx)
	require.NoError(t, e)
	require.NotNil(t, j)
	require.NoError(t, f.s.Handle(ctx, *j))
	require.NoError(t, f.q.Ack(ctx, *j))
	page, e := f.s.List(ctx, f.p, d.Filter{Kind: "tagging_result", ParentID: task.ID, Limit: 100})
	require.NoError(t, e)
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	var original map[string]any
	require.NoError(t, json.Unmarshal(item.Payload, &original))
	path := "/tagging-results/" + item.ID + "/review"
	body := map[string]any{"decision": "approved", "comment": "verified", "expected_version": item.Version}
	code, _ := f.api(t, 992, f.p.TenantID, "POST", path, body)
	require.Equal(t, 403, code)
	code, _ = f.api(t, 991, f.p.TenantID+1, "POST", path, body)
	require.Equal(t, 404, code)
	code, out := f.api(t, 991, f.p.TenantID, "POST", path, body)
	require.Equal(t, 200, code, out)
	reviewed := out["data"].(map[string]any)
	require.Equal(t, "approved", reviewed["review_status"])
	require.Equal(t, "approved", reviewed["status"])
	require.Equal(t, original["provenance"], reviewed["provenance"])
	require.Equal(t, original["evidence"], reviewed["evidence"])
	require.Equal(t, float64(991), reviewed["reviewed_by"])
	code, _ = f.api(t, 991, f.p.TenantID, "POST", path, body)
	require.Equal(t, 409, code)
	updated, e := f.s.UpdateTagging(ctx, f.p, task.ID, item.ID, d.TaggingResult{Tags: []string{"manual"}, ReviewStatus: "approved", ReviewedBy: 991, ReviewComment: "forged"}, item.Version+1)
	require.NoError(t, e)
	require.Equal(t, "pending", updated.Status)
	var edited d.TaggingResult
	require.NoError(t, json.Unmarshal(updated.Payload, &edited))
	require.Zero(t, edited.ReviewedBy)
	require.Empty(t, edited.ReviewComment)
	body["expected_version"] = updated.Version
	body["decision"] = "rejected"
	code, out = f.api(t, 991, f.p.TenantID, "POST", path, body)
	require.Equal(t, 200, code, out)
	require.Equal(t, "rejected", out["data"].(map[string]any)["status"])
}
