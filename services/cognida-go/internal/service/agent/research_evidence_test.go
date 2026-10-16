package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

// Human-labelled cases exercise evidence semantics independently of the writer's
// prose. The model response is a deterministic fixture here, not model-quality proof.
func TestResearchEvidenceHumanLabelledCases(t *testing.T) {
	for _, tc := range []struct{ name, claim, quote, status, relation string }{
		{"supported", "Paris is the capital of France.", "Paris is the capital of France.", "supported", "supports"},
		{"real unrelated quote", "Rome is the capital of France.", "Paris has many historic museums.", "insufficient", "context"},
		{"contradiction", "The trial improved survival.", "The trial did not improve survival.", "conflict", "contradicts"},
		{"scope mismatch", "All patients recovered.", "Two of ten patients recovered in 2025.", "insufficient", "context"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &DeepResearchResponse{Claims: []ResearchClaim{{ID: "C1", Statement: tc.claim}}, Sources: []ReportSourceDTO{{ID: "S1", Excerpt: tc.quote}}}
			groupResearchSources(report.Sources)
			response, _ := json.Marshal(map[string]any{"claims": []any{map[string]any{"id": "C1", "status": tc.status, "reason": "Human-labelled scope and supporting relation", "evidence": []any{map[string]any{"source_id": "S1", "quote": tc.quote, "relation": tc.relation}}}}})
			model := researchTestModel{run: func(_ context.Context, msgs []*schema.Message) (*schema.Message, error) {
				require.Contains(t, msgs[0].Content, "真实引文不等于支持主张")
				return schema.AssistantMessage(string(response), nil), nil
			}}
			svc := NewResearchService(nil, nil).WithDependencies(model, nil)
			require.NoError(t, svc.assessClaims(withResearchBudget(context.Background()), report))
			require.Equal(t, tc.status, report.Claims[0].Status)
			require.True(t, report.Claims[0].Evidence[0].QuoteMatched)
			require.False(t, report.Metadata.FactChecked)
		})
	}
}
func TestResearchReviewerRejectsFakeQuoteAndContradictoryStatus(t *testing.T) {
	for _, raw := range []string{
		`{"claims":[{"id":"C1","status":"supported","reason":"r","evidence":[{"source_id":"S1","quote":"invented quotation","relation":"supports"}]}]}`,
		`{"claims":[{"id":"C1","status":"supported","reason":"r","evidence":[{"source_id":"S1","quote":"Original evidence string.","relation":"contradicts"}]}]}`,
		`{"claims":[{"id":"C404","status":"insufficient","reason":"r","evidence":[]}]}`,
	} {
		svc := NewResearchService(nil, nil).WithDependencies(researchTestModel{run: func(context.Context, []*schema.Message) (*schema.Message, error) {
			return schema.AssistantMessage(raw, nil), nil
		}}, nil)
		report := &DeepResearchResponse{Claims: []ResearchClaim{{ID: "C1", Statement: "Claim"}}, Sources: []ReportSourceDTO{{ID: "S1", Excerpt: "Original evidence string."}}}
		require.Error(t, svc.assessClaims(context.Background(), report))
		require.False(t, report.Metadata.SemanticReview)
	}
}
func TestResearchSameSourceRepublicationAndBoundedCalls(t *testing.T) {
	sources := []ReportSourceDTO{{ID: "S1", URL: "https://a.test/original", Excerpt: "Same original evidence."}, {ID: "S2", URL: "https://b.test/republication", Excerpt: "Same original evidence."}, {ID: "S3", URL: "https://a.test/original?utm_source=x", Excerpt: "Updated extract."}, {ID: "S4", URL: "https://c.test/independent", Excerpt: "Contrary independent evidence."}}
	groupResearchSources(sources)
	require.Equal(t, sources[0].IndependenceGroup, sources[1].IndependenceGroup)
	require.Equal(t, sources[0].IndependenceGroup, sources[2].IndependenceGroup)
	require.NotEqual(t, sources[0].IndependenceGroup, sources[3].IndependenceGroup)
	ctx := withResearchBudget(context.Background())
	for i := 0; i < 4; i++ {
		require.NoError(t, useResearchCall(ctx, true))
	}
	require.Error(t, useResearchCall(ctx, true))
	for i := 0; i < 16; i++ {
		require.NoError(t, useResearchCall(ctx, false))
	}
	require.Error(t, useResearchCall(ctx, false))
}
func TestGovernedResearchReviewFailureRetainsEvidenceWithoutProviderSecret(t *testing.T) {
	svc := newResearchTestService(t, false, researchGoodDraft())
	svc.model = researchTestModel{run: func(_ context.Context, msgs []*schema.Message) (*schema.Message, error) {
		if strings.Contains(msgs[1].Content, "max_questions") {
			return schema.AssistantMessage(`{"questions":[{"question":"question","strategy":"multi_hop"}]}`, nil), nil
		}
		if strings.Contains(msgs[0].Content, "证据审查员") {
			return nil, errors.New("provider secret-key-sensitive")
		}
		return schema.AssistantMessage(researchGoodDraft(), nil), nil
	}}
	report, e := svc.ExecuteGoverned(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}})
	require.Error(t, e)
	require.False(t, report.Success)
	require.True(t, report.Metadata.Partial)
	require.Len(t, report.Sources, 1)
	require.Len(t, report.Claims, 1)
	require.NotContains(t, report.Error, "secret-key")
	require.Equal(t, 3, report.Metadata.ModelCallsUsed)
}
