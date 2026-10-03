package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCronSemantics(t *testing.T) {
	for _, tc := range []struct{ cron, from, want string }{
		{"5/10 * * * *", "2026-01-01T00:06:00Z", "2026-01-01T00:15:00Z"},
		{"0 0 * * 7", "2026-01-01T00:00:00Z", "2026-01-04T00:00:00Z"},
		{"0 0 15 * 1", "2026-01-12T00:00:00Z", "2026-01-15T00:00:00Z"},
		{"0 0 */2 * 1", "2026-01-05T00:00:00Z", "2026-01-19T00:00:00Z"},
		{"0 0 29 2 *", "2025-03-01T00:00:00Z", "2028-02-29T00:00:00Z"},
	} {
		t.Run(tc.cron, func(t *testing.T) {
			after, _ := time.Parse(time.RFC3339, tc.from)
			next, e := NextRun(d.Schedule{Cron: tc.cron, Timezone: "UTC"}, after)
			require.NoError(t, e)
			require.Equal(t, tc.want, next.Format(time.RFC3339))
		})
	}
	after, _ := time.Parse(time.RFC3339, "2026-11-01T05:30:00Z")
	next, e := NextRun(d.Schedule{Cron: "30 1 * * *", Timezone: "America/New_York"}, after)
	require.NoError(t, e)
	require.Equal(t, "2026-11-01T06:30:00Z", next.Format(time.RFC3339))
	for _, cron := range []string{"1/0 * * * *", "60 * * * *", "* * * * 8", "1--2 * * * *", "* * * *"} {
		_, e := NextRun(d.Schedule{Cron: cron, Timezone: "UTC"}, time.Now())
		require.ErrorIs(t, e, d.ErrInvalid)
	}
}
func TestWatchAggregatesPeriodsAndExplainsDimensions(t *testing.T) {
	input := l.Input{Kind: "table", Columns: []string{"day", "region", "revenue"}, Rows: [][]any{}}
	for day := 1; day <= 4; day++ {
		v := 10
		if day == 4 {
			v = 100
		}
		input.Rows = append(input.Rows, []any{time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), "A", v}, []any{time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), "B", 20})
	}
	in := d.AnalysisInput{TimeColumn: "day", ValueColumn: "revenue"}
	watch := &d.Watch{Method: "zscore", Threshold: 3, AttributionDimensions: []string{"region"}}
	out, alert, e := WatchMetric(input, in, watch)
	require.NoError(t, e)
	require.True(t, alert)
	require.Equal(t, 120.0, out["latest"])
	require.Equal(t, 3, out["baseline_count"])
	groups := out["attribution"].([]map[string]any)
	require.Len(t, groups, 2)
	require.Equal(t, 90.0, groups[0]["delta"])
	require.Equal(t, 1.0, groups[0]["share_of_total_delta"])
	require.Equal(t, "A", groups[0]["dimensions"].(map[string]any)["region"])
	watch.AttributionDimensions = []string{"missing"}
	_, _, e = WatchMetric(input, in, watch)
	require.ErrorIs(t, e, d.ErrInvalid)
}
func TestVersionViewKeepsDefinitionVersion(t *testing.T) {
	v, e := View(&d.Record{Kind: "feature_version", Meta: d.Meta{Version: 1}, Payload: json.RawMessage(`{"version":7}`)})
	require.NoError(t, e)
	require.Equal(t, float64(7), v["version"])
}

type unitRepo struct {
	d.Repository
	records   map[string]*d.Record
	published []string
	creates   int
}

func (r *unitRepo) Get(_ context.Context, tenant int64, kind, id string) (*d.Record, error) {
	v := r.records[id]
	if v == nil || v.TenantID != tenant || v.Kind != kind {
		return nil, d.ErrNotFound
	}
	cp := *v
	return &cp, nil
}
func (r *unitRepo) Create(_ context.Context, v *d.Record) (*d.Record, error) {
	r.creates++
	v.ID = uuid.NewString()
	v.Version = 1
	if r.records == nil {
		r.records = map[string]*d.Record{}
	}
	r.records[v.ID] = v
	return v, nil
}
func (r *unitRepo) PublishedRefs(_ context.Context, _ int64, cursor string, limit int) ([]string, error) {
	start := 0
	for i, v := range r.published {
		if v == cursor {
			start = i + 1
		}
	}
	end := start + limit
	if end > len(r.published) {
		end = len(r.published)
	}
	return r.published[start:end], nil
}

type unitAssets struct {
	refs  map[string]l.DataAssetRef
	input l.Input
	deny  bool
}

func (a *unitAssets) Resolve(_ context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if a.deny {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	ref, ok := a.refs[id.String()]
	if !ok || ref.TenantID != tenant {
		return ref, nil, l.ErrNotFound
	}
	raw, _ := json.Marshal(a.input)
	return ref, raw, nil
}
func (a *unitAssets) ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error) {
	return l.DataAssetRef{}, errors.New("unused")
}
func testAsset(source string, at time.Time) l.DataAssetRef {
	id := l.RefID{AssetID: uuid.NewString(), Version: 1}
	return l.DataAssetRef{RefID: id, AssetRefID: id.String(), TenantID: 1, Kind: l.PublishedAsset, Policy: l.PolicySnapshot{Approved: true}, Provenance: l.Provenance{SourceType: "datasource", SourceID: source}, CreatedAt: at}
}
func TestRefreshSelectsOnlySameApprovedSource(t *testing.T) {
	old := testAsset("db/table", time.Now().Add(-time.Hour))
	latest := testAsset("db/table", time.Now())
	other := testAsset("other", time.Now().Add(time.Hour))
	unapproved := testAsset("db/table", time.Now().Add(2*time.Hour))
	unapproved.Policy.Approved = false
	assets := &unitAssets{refs: map[string]l.DataAssetRef{}, input: l.Input{Kind: "text", Text: "valid"}}
	repo := &unitRepo{}
	for _, ref := range []l.DataAssetRef{old, latest, other, unapproved} {
		assets.refs[ref.AssetRefID] = ref
		repo.published = append(repo.published, ref.AssetRefID)
	}
	s := NewService(repo, assets, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	p := l.Principal{TenantID: 1, UserID: 1}
	in := d.AnalysisInput{AssetRefID: old.AssetRefID, Refresh: &d.RefreshSource{Mode: "latest_published", SourceType: "datasource", SourceID: "db/table"}}
	got, e := s.resolveSubscriptionInput(context.Background(), p, in)
	require.NoError(t, e)
	require.Equal(t, latest.AssetRefID, got)
	in.Refresh.SourceID = "wrong"
	_, e = s.resolveSubscriptionInput(context.Background(), p, in)
	require.ErrorIs(t, e, d.ErrInvalid)
	in.Refresh = nil
	got, e = s.resolveSubscriptionInput(context.Background(), p, in)
	require.NoError(t, e)
	require.Equal(t, old.AssetRefID, got)
}
func TestInvalidAnnotationDoesNotLeaveBuildingRecord(t *testing.T) {
	repo := &unitRepo{}
	s := NewService(repo, &unitAssets{}, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	_, e := s.CreateAnnotation(context.Background(), l.Principal{TenantID: 1, UserID: 1}, d.AnnotationTask{Name: "a", Type: "classification", Labels: []d.Label{{ID: "a", Name: "A"}}, SourceRefID: "invalid"}, "key")
	require.Error(t, e)
	require.Zero(t, repo.creates)
}
func TestDerivedReadRetainsSensitiveSourcePolicy(t *testing.T) {
	ref := testAsset("private", time.Now())
	raw, _ := json.Marshal(d.AnnotationTask{SourceRefID: ref.AssetRefID})
	task := &d.Record{Meta: d.Meta{ID: "task", TenantID: 1}, Kind: "annotation_task", Payload: raw}
	item := &d.Record{Meta: d.Meta{ID: "item", TenantID: 1}, Kind: "annotation_item", ParentID: "task", Payload: json.RawMessage(`{"content":"secret"}`)}
	repo := &unitRepo{records: map[string]*d.Record{"task": task, "item": item}}
	s := NewService(repo, &unitAssets{deny: true}, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	_, e := s.Get(context.Background(), l.Principal{TenantID: 1, UserID: 2}, "annotation_item", "item")
	require.ErrorIs(t, e, d.ErrForbidden)
}

func TestRefreshCandidateLimitReturnsExplicitError(t *testing.T) {
	ref := testAsset("db/table", time.Now())
	assets := &unitAssets{refs: map[string]l.DataAssetRef{ref.AssetRefID: ref}, input: l.Input{Kind: "text", Text: "valid"}}
	repo := &unitRepo{}
	for i := 0; i < maxRefreshCandidates+1; i++ {
		repo.published = append(repo.published, (l.RefID{AssetID: uuid.NewString(), Version: 1}).String())
	}
	s := NewService(repo, assets, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	_, err := s.resolveSubscriptionInput(context.Background(), l.Principal{TenantID: 1, UserID: 1}, d.AnalysisInput{AssetRefID: ref.AssetRefID, Refresh: &d.RefreshSource{Mode: "latest_published", SourceType: "datasource", SourceID: "db/table"}})
	require.ErrorContains(t, err, "exceeds 10000")
}

func TestSuggestionReadRequiresCreatorOrAdminAndEvidencePolicy(t *testing.T) {
	ref := testAsset("secret", time.Now())
	raw, _ := json.Marshal(d.Suggestion{Source: d.SuggestionSource{Type: "conversation_insight", ID: "message"}, EvidenceRefIDs: []string{ref.AssetRefID}})
	record := &d.Record{Meta: d.Meta{ID: "s", TenantID: 1, UserID: 1}, Kind: "suggestion", Payload: raw}
	repo := &unitRepo{records: map[string]*d.Record{"s": record}}
	assets := &unitAssets{refs: map[string]l.DataAssetRef{ref.AssetRefID: ref}, input: l.Input{Kind: "text", Text: "secret"}}
	s := NewService(repo, assets, nil, nil, func(_ context.Context, p l.Principal, scope string) error {
		if scope == "admin" && p.UserID != 3 {
			return d.ErrForbidden
		}
		return nil
	})
	s.ValidateSource = func(context.Context, l.Principal, d.SuggestionSource) error { return nil }
	_, e := s.Get(context.Background(), l.Principal{TenantID: 1, UserID: 2}, "suggestion", "s")
	require.ErrorIs(t, e, d.ErrForbidden)
	_, e = s.Get(context.Background(), l.Principal{TenantID: 1, UserID: 1}, "suggestion", "s")
	require.NoError(t, e)
	_, e = s.Get(context.Background(), l.Principal{TenantID: 1, UserID: 3}, "suggestion", "s")
	require.NoError(t, e)
	assets.deny = true
	_, e = s.Get(context.Background(), l.Principal{TenantID: 1, UserID: 1}, "suggestion", "s")
	require.ErrorIs(t, e, d.ErrForbidden)
}

func (r *unitRepo) Update(_ context.Context, v *d.Record, expected int) error {
	old := r.records[v.ID]
	if old == nil {
		return d.ErrNotFound
	}
	if old.Version != expected {
		return d.ErrConflict
	}
	v.Version = expected + 1
	copy := *v
	r.records[v.ID] = &copy
	return nil
}
func TestTaggingReviewKeepsEvidenceAndRequiresAdminCAS(t *testing.T) {
	ref := testAsset("source", time.Now())
	taskBody, _ := json.Marshal(d.TaggingTask{SourceRefID: ref.AssetRefID})
	task := &d.Record{Meta: d.Meta{ID: "task", TenantID: 1}, Kind: "tagging_task", Payload: taskBody}
	body := json.RawMessage(`{"tags":["sales"],"provenance":{"source":"rules","method":"rules-v1","confidence":0.75},"evidence":{"tags":{"value":["sales"],"confidence":0.75,"evidence":["line one"]}},"review_status":"pending"}`)
	item := &d.Record{Meta: d.Meta{ID: "item", TenantID: 1, UserID: 1, Version: 1}, Kind: "tagging_result", ParentID: "task", Status: "pending", Payload: body}
	repo := &unitRepo{records: map[string]*d.Record{"task": task, "item": item}}
	assets := &unitAssets{refs: map[string]l.DataAssetRef{ref.AssetRefID: ref}, input: l.Input{Kind: "text", Text: "sales"}}
	s := NewService(repo, assets, nil, nil, func(_ context.Context, p l.Principal, scope string) error {
		if scope == "admin" && p.UserID != 9 {
			return d.ErrForbidden
		}
		return nil
	})
	p := l.Principal{TenantID: 1, UserID: 9}
	ctx := context.Background()
	_, e := s.ReviewTagging(ctx, l.Principal{TenantID: 1, UserID: 1}, "item", "approved", "", 1)
	require.ErrorIs(t, e, d.ErrForbidden)
	approved, e := s.ReviewTagging(ctx, p, "item", "approved", "verified", 1)
	require.NoError(t, e)
	require.Equal(t, "approved", approved.Status)
	var before, after map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &before))
	require.NoError(t, json.Unmarshal(approved.Payload, &after))
	require.True(t, d.JSONEqual(before["provenance"], after["provenance"]))
	require.True(t, d.JSONEqual(before["evidence"], after["evidence"]))
	_, e = s.ReviewTagging(ctx, p, "item", "rejected", "stale", 1)
	require.ErrorIs(t, e, d.ErrConflict)
	rejected, e := s.ReviewTagging(ctx, p, "item", "rejected", "needs correction", 2)
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.Status)
}

func TestGovernanceInputPreservesExactNumberTokens(t *testing.T) {
	ref := testAsset("exact", time.Now())
	assets := &unitAssets{refs: map[string]l.DataAssetRef{ref.AssetRefID: ref}, input: l.Input{Kind: "table", Columns: []string{"id", "amount"}, Rows: [][]any{{int64(9007199254740993), json.Number("0.1234567890123456789")}}}}
	s := NewService(nil, assets, nil, nil, nil)
	_, input, err := s.input(context.Background(), l.Principal{TenantID: 1, UserID: 9}, ref.AssetRefID, false)
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), input.Rows[0][0])
	require.Equal(t, json.Number("0.1234567890123456789"), input.Rows[0][1])
}
func TestSuspensionPreservesPayloadAndClearsDue(t *testing.T) {
	due := time.Now()
	record := &d.Record{Meta: d.Meta{ID: "subscription", Version: 1}, Kind: "subscription", Status: "enabled", DueAt: &due, Payload: json.RawMessage(`{"enabled":true,"custom_metadata":{"exact":9007199254740993}}`)}
	repo := &unitRepo{records: map[string]*d.Record{record.ID: record}}
	s := NewService(repo, nil, nil, nil, nil)
	require.ErrorContains(t, s.suspendSubscription(context.Background(), record, "invalid_schedule_configuration"), "suspended")
	require.Nil(t, record.DueAt)
	require.Equal(t, "disabled", record.Status)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(record.Payload, &payload))
	require.JSONEq(t, `{"exact":9007199254740993}`, string(payload["custom_metadata"]))
	require.Equal(t, "false", string(payload["enabled"]))
}

func TestConfidenceSupportsExactComputeNumbers(t *testing.T) {
	for _, v := range []any{0.7, json.Number("0.7")} {
		n, ok := confidenceValue(v)
		require.True(t, ok)
		require.Equal(t, 0.7, n)
	}
	for _, v := range []any{2.0, json.Number("-1"), json.Number("1e999"), "0.7"} {
		_, ok := confidenceValue(v)
		require.False(t, ok)
	}
}
