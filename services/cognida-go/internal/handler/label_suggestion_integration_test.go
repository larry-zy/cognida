//go:build integration

package handler

import (
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	gov "cognida/internal/service/governance"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

// Real database and HTTP contract; inference is explicitly a fixed synthetic adapter.
// Live provider/browser acceptance remains a separate exercise.
type suggestionFixtureModel struct {
	calls int
	fail  bool
	after func()
}

func (m *suggestionFixtureModel) ModelID() string { return "synthetic-review-model" }
func (m *suggestionFixtureModel) Suggest(_ context.Context, in gov.LabelSuggestionInput) (gov.LabelCandidate, error) {
	m.calls++
	if m.after != nil {
		m.after()
	}
	if m.fail {
		return gov.LabelCandidate{}, errors.New("synthetic provider failure")
	}
	c := gov.LabelCandidate{DecisionStatus: "matched", Rationale: "Synthetic fixture evidence"}
	switch in.Kind {
	case "qa":
		c.Annotation = &g.Annotation{QA: in.QA}
	case "tagging":
		c.Tags = []string{"业务"}
		c.Evidence = []g.EvidenceSpan{{LabelID: "business", Rule: "model_evidence", Match: in.Content, Start: 0, End: len([]rune(in.Content))}}
	case "classification":
		c.Annotation = &g.Annotation{Labels: []string{"business"}}
		c.Evidence = []g.EvidenceSpan{{LabelID: "business", Rule: "model_evidence", Match: in.Content, Start: 0, End: len([]rune(in.Content))}}
	case "named_entity":
		c.Annotation = &g.Annotation{Entities: []g.Entity{{ID: "e", Label: "business", Text: in.Content, Start: 0, End: len([]rune(in.Content))}}}
	case "relation":
		c.Annotation = &g.Annotation{Entities: []g.Entity{{ID: "a", Label: "person", Text: "小李", Start: 0, End: 2}, {ID: "b", Label: "org", Text: "蓝鹭", Start: 4, End: 6}}, Relations: []g.Relation{{From: "a", To: "b", Label: "business"}}}
		c.Evidence = []g.EvidenceSpan{{LabelID: "business", Rule: "model_evidence", Match: in.Content, Start: 0, End: len([]rune(in.Content))}}
	}
	return c, nil
}
func newSuggestionItem(t *testing.T, f *reviewedFixture, kind string) (string, g.Record) {
	t.Helper()
	task := g.AnnotationTask{Name: "synthetic annotation", Type: kind, SourceRefID: f.assets.ref.AssetRefID, Labels: []g.Label{{ID: "business", Name: "业务"}}}
	if kind == "qa" {
		task.Labels = nil
		task.QAMapping = &g.QAMapping{QuestionColumn: "q", ReferenceAnswerColumn: "a"}
	} else {
		f.assets.input = l.Input{Kind: "text", Text: "小李任职蓝鹭"}
	}
	r, e := f.gs.CreateAnnotation(context.Background(), f.p, task, uuid.NewString())
	require.NoError(t, e)
	page, e := f.gs.List(context.Background(), f.p, g.Filter{Kind: "annotation_item", ParentID: r.ID, Limit: 10})
	require.NoError(t, e)
	require.Len(t, page.Items, 1)
	return r.ID, page.Items[0]
}
func TestLabelSuggestionMySQLHTTPFourProfilesReplayAndReview(t *testing.T) {
	for _, kind := range []string{"classification", "named_entity", "relation", "qa"} {
		t.Run(kind, func(t *testing.T) {
			f := newReviewedFixture(t)
			model := &suggestionFixtureModel{}
			f.gs.WithLabelSuggester(model)
			NewGovernanceHandler(f.gs).RegisterLabelSuggestionRoutes(f.router.Group("/api"))
			task, item := newSuggestionItem(t, f, kind)
			path := "/annotation-tasks/" + task + "/items/" + item.ID + "/suggest"
			body := gov.LabelSuggestionRequest{ExpectedVersion: item.Version, Guidelines: "Only literal source evidence"}
			code, raw := f.request(t, "POST", path, "suggest", body)
			require.Equal(t, 200, code)
			var result g.AnnotationItem
			require.NoError(t, json.Unmarshal(raw, &result))
			require.Equal(t, "pending", result.ReviewStatus)
			require.Equal(t, "model", result.Provenance.Source)
			require.Nil(t, result.Provenance.Confidence)
			require.NotEmpty(t, result.Provenance.ModelCallID)
			code, _ = f.request(t, "POST", path, "suggest", body)
			require.Equal(t, 200, code)
			require.Equal(t, 1, model.calls)
			code, _ = f.request(t, "POST", path, "suggest", body, "X-Other-Tenant")
			require.Equal(t, 404, code)
			require.Equal(t, 1, model.calls)
			approved, e := f.gs.ReviewAnnotation(context.Background(), f.p, task, item.ID, "approved", "independently checked", result.Version)
			require.NoError(t, e)
			code, _ = f.request(t, "POST", path, "new", gov.LabelSuggestionRequest{ExpectedVersion: approved.Version})
			require.Equal(t, 409, code)
			require.Equal(t, 1, model.calls)
			f.assets.deny = true
			code, _ = f.request(t, "POST", path, "suggest", body)
			require.Equal(t, 403, code)
			require.Equal(t, 1, model.calls)
		})
	}
}
func TestLabelSuggestionMySQLFailureUnknownAndManualRace(t *testing.T) {
	f := newReviewedFixture(t)
	model := &suggestionFixtureModel{fail: true}
	f.gs.WithLabelSuggester(model)
	NewGovernanceHandler(f.gs).RegisterLabelSuggestionRoutes(f.router.Group("/api"))
	task, item := newSuggestionItem(t, f, "classification")
	path := "/annotation-tasks/" + task + "/items/" + item.ID + "/suggest"
	body := gov.LabelSuggestionRequest{ExpectedVersion: item.Version}
	code, _ := f.request(t, "POST", path, "failure", body)
	require.Equal(t, 500, code)
	code, _ = f.request(t, "POST", path, "failure", body)
	require.Equal(t, 502, code)
	require.Equal(t, 1, model.calls)
	require.NoError(t, f.db.Table("governance_records").Where("tenant_id=? AND kind=?", f.p.TenantID, "label_model_call").Update("status", "running").Error)
	code, _ = f.request(t, "POST", path, "failure", body)
	require.Equal(t, 409, code)
	require.Equal(t, 1, model.calls)
	model.fail = false
	model.after = func() {
		_, e := f.gs.UpdateAnnotation(context.Background(), f.p, task, item.ID, g.Annotation{Labels: []string{"business"}}, item.Version)
		require.NoError(t, e)
	}
	code, _ = f.request(t, "POST", path, "human-race", body)
	require.Equal(t, 409, code)
	stored, e := f.gs.Get(context.Background(), f.p, "annotation_item", item.ID)
	require.NoError(t, e)
	var out g.AnnotationItem
	require.NoError(t, json.Unmarshal(stored.Payload, &out))
	require.Equal(t, "human", out.Provenance.Source)
	require.Equal(t, "pending", out.ReviewStatus)
}
