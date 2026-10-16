package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cognida/internal/infrastructure/llm/chat"
	"cognida/internal/service/agent/tools"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

const partialSampleQuote = "The reported sample contains ten rows."
const partialConflictQuote = "The treatment did not improve survival."

func partialResearchFixture(t *testing.T, secondError error, firstInvalid bool) (*DeepResearchResponse, error) {
	t.Helper()
	sources := []tools.DocumentChunk{
		{Content: partialSampleQuote + " Original record.", ChunkID: "c1", KnowledgeID: "d1", KnowledgeBaseID: "allowed"},
		{Content: partialSampleQuote + " " + partialConflictQuote, ChunkID: "c2", KnowledgeID: "d2", KnowledgeBaseID: "allowed"},
	}
	first := []ResearchClaim{
		{ID: "C1", Status: "supported", Reason: "Two records report the same count.", Evidence: []ResearchClaimEvidence{{SourceID: "S1", Quote: partialSampleQuote, Relation: "supports"}, {SourceID: "S2", Quote: partialSampleQuote, Relation: "supports"}}},
		{ID: "C2", Status: "conflict", Reason: "An explicit contrary finding exists.", Evidence: []ResearchClaimEvidence{{SourceID: "S2", Quote: partialConflictQuote, Relation: "contradicts"}}},
		{ID: "C3", Status: "insufficient", Reason: "Long-term outcomes are not established.", Evidence: []ResearchClaimEvidence{{SourceID: "S1", Quote: partialSampleQuote, Relation: "context"}}},
	}
	draft, err := json.Marshal(researchDraft{Summary: "Recorded sample [S1]", Analysis: "Conflicting and limited observations [S2]", Findings: []researchFinding{
		{Statement: "The reported sample has ten rows.", Citations: []researchCitation{{SourceID: "S1", Quote: partialSampleQuote}}},
		{Statement: "The treatment improved survival.", Citations: []researchCitation{{SourceID: "S2", Quote: partialConflictQuote}}},
		{Statement: "Long-term outcomes improved.", Citations: []researchCitation{{SourceID: "S1", Quote: partialSampleQuote}}},
	}})
	require.NoError(t, err)
	modelCalls, retrievalCalls := 0, 0
	model := researchTestModel{run: func(_ context.Context, msg []*schema.Message) (*schema.Message, error) {
		modelCalls++
		switch modelCalls {
		case 1:
			return schema.AssistantMessage(`{"questions":[{"question":"sample outcomes","strategy":"hybrid"}]}`, nil), nil
		case 2:
			return schema.AssistantMessage(string(draft), nil), nil
		case 3:
			if firstInvalid {
				return schema.AssistantMessage(`{"claims":[]}`, nil), nil
			}
			b, _ := json.Marshal(map[string]any{"claims": first})
			return schema.AssistantMessage(string(b), nil), nil
		case 4:
			if secondError != nil {
				return nil, secondError
			}
			return schema.AssistantMessage(`{"claims":[]}`, nil), nil
		default:
			t.Fatal("automatic replay")
			return nil, nil
		}
	}}
	rag := tools.NewTypedBaseTool("rag_query", "fixture", func(context.Context, *tools.RAGQueryRequest) (*tools.RAGQueryResult, error) {
		retrievalCalls++
		if retrievalCalls == 1 {
			return &tools.RAGQueryResult{Chunks: sources}, nil
		}
		// The bridge changes G2 into G1; a separate new source also contains a
		// possible counterexample. Neither may be silently treated as reviewed.
		return &tools.RAGQueryResult{Chunks: []tools.DocumentChunk{
			{Content: sources[0].Content, ChunkID: "bridge", KnowledgeID: "d2", KnowledgeBaseID: "allowed"},
			{Content: "The sample contains nine rows, not ten.", ChunkID: "contrary", KnowledgeID: "d4", KnowledgeBaseID: "allowed"},
		}}, nil
	})
	svc := NewResearchService(nil, nil).WithDependencies(model, researchTestRegistry{"rag_query": rag})
	report, err := svc.ExecuteGoverned(researchTestContext(), &DeepResearchRequest{Query: "sample outcomes", KBIDs: []string{"allowed"}})
	if firstInvalid {
		require.Equal(t, 3, modelCalls)
	} else {
		require.Equal(t, 4, modelCalls)
	}
	return report, err
}

func TestResearchSupplementFailurePreservesScopedReviewAndCounterEvidence(t *testing.T) {
	report, err := partialResearchFixture(t, nil, false)
	require.ErrorIs(t, err, ErrResearchSupplementIncomplete)
	require.True(t, UsableResearchPartial(report, err))
	require.False(t, report.Success)
	require.True(t, report.Metadata.Partial)
	require.Equal(t, "review_claim_count", report.Metadata.Failure.Code)
	require.Equal(t, "supplement_reviewing", report.Metadata.Failure.Stage)
	require.Equal(t, 1, report.Metadata.ReviewRounds)
	require.Len(t, report.ReviewHistory, 1)
	require.Equal(t, []string{"S1", "S2"}, report.ReviewHistory[0].SourceIDs)
	require.Equal(t, []string{"S3", "S4"}, report.Metadata.UnreviewedSourceIDs)
	require.Equal(t, "supported", report.ReviewHistory[0].Claims[0].Status)
	require.Equal(t, 2, report.ReviewHistory[0].Claims[0].IndependentSources)
	require.Equal(t, "G2", report.ReviewHistory[0].SourceGroups["S2"])
	require.Equal(t, "G1", report.Sources[1].IndependenceGroup)
	require.Equal(t, "insufficient", report.Claims[0].Status)
	require.Equal(t, 1, report.Claims[0].IndependentSources)
	require.Equal(t, "conflict", report.Claims[1].Status)
	require.Equal(t, partialConflictQuote, report.Claims[1].Evidence[0].Quote)
	require.Contains(t, report.Sources[3].Excerpt, "nine rows")
	require.Len(t, report.Unverified, 4)
	require.Contains(t, report.Metadata.VerificationLimit, "首轮来源")
	for name, mutate := range map[string]func(*DeepResearchResponse){
		"missing snapshot":        func(r *DeepResearchResponse) { r.ReviewHistory = nil },
		"wrong digest":            func(r *DeepResearchResponse) { r.ReviewHistory[0].SourceDigest = strings.Repeat("0", 64) },
		"missing pending":         func(r *DeepResearchResponse) { r.Metadata.UnreviewedSourceIDs = nil },
		"changed reviewed source": func(r *DeepResearchResponse) { r.Sources[0].Excerpt += " changed" },
		"wrong stage":             func(r *DeepResearchResponse) { r.Metadata.Failure.Stage = "reviewing" },
		"lost conflict":           func(r *DeepResearchResponse) { r.Claims[1].Status = "insufficient"; r.Claims[1].Evidence = nil },
		"claims still supported":  func(r *DeepResearchResponse) { r.Claims[0].Status = "supported" },
		"invalid writer quote":    func(r *DeepResearchResponse) { r.CitationChecks[0].Quote = "invented quote" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := cloneResearchReport(report)
			mutate(copy)
			require.False(t, UsableResearchPartial(copy, err))
		})
	}
}

func TestResearchSupplementDoesNotConvertPrimaryOrUnclassifiedFailure(t *testing.T) {
	report, err := partialResearchFixture(t, nil, true)
	require.Error(t, err)
	require.False(t, UsableResearchPartial(report, err))
	require.Empty(t, report.ReviewHistory)
	require.False(t, report.Metadata.SemanticReview)
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("unknown scope or provider failure"), &chat.HTTPStatusError{StatusCode: 403, Body: "private"}} {
		report, err = partialResearchFixture(t, cause, false)
		require.Error(t, err)
		require.False(t, UsableResearchPartial(report, err))
		require.False(t, errors.Is(err, ErrResearchSupplementIncomplete))
		require.Len(t, report.ReviewHistory, 1)
		require.Equal(t, "insufficient", report.Claims[0].Status)
	}
	report, err = partialResearchFixture(t, &chat.HTTPStatusError{StatusCode: 402, Body: "private quota details"}, false)
	require.True(t, UsableResearchPartial(report, err))
	require.Equal(t, 402, report.Metadata.Failure.HTTPStatus)
	body, _ := json.Marshal(report)
	require.NotContains(t, string(body), "private quota details")
}

func TestResearchSemanticValidationHasStableSiteCodes(t *testing.T) {
	sources := []ReportSourceDTO{{ID: "S1", Excerpt: partialSampleQuote, IndependenceGroup: "G1"}}
	valid := []ResearchClaim{{ID: "C1", Statement: "Sample count.", Status: "supported", Reason: "Matches.", Evidence: []ResearchClaimEvidence{{SourceID: "S1", Quote: partialSampleQuote, Relation: "supports"}}}}
	for _, tc := range []struct {
		code   string
		mutate func([]ResearchClaim) []ResearchClaim
	}{
		{"review_claim_count", func(_ []ResearchClaim) []ResearchClaim { return nil }},
		{"review_claim_identity", func(c []ResearchClaim) []ResearchClaim { c[0].ID = "unknown"; return c }},
		{"review_reason_bounds", func(c []ResearchClaim) []ResearchClaim { c[0].Reason = ""; return c }},
		{"review_evidence_bounds", func(c []ResearchClaim) []ResearchClaim { c[0].Evidence = make([]ResearchClaimEvidence, 9); return c }},
		{"review_status_invalid", func(c []ResearchClaim) []ResearchClaim { c[0].Status = "verified"; return c }},
		{"review_source_unknown", func(c []ResearchClaim) []ResearchClaim { c[0].Evidence[0].SourceID = "private-provider-text"; return c }},
		{"review_quote_bounds", func(c []ResearchClaim) []ResearchClaim { c[0].Evidence[0].Quote = "short"; return c }},
		{"review_quote_mismatch", func(c []ResearchClaim) []ResearchClaim { c[0].Evidence[0].Quote = "private-provider-text"; return c }},
		{"review_relation_invalid", func(c []ResearchClaim) []ResearchClaim { c[0].Evidence[0].Relation = "unknown"; return c }},
		{"review_status_evidence_conflict", func(c []ResearchClaim) []ResearchClaim { c[0].Evidence[0].Relation = "contradicts"; return c }},
	} {
		t.Run(tc.code, func(t *testing.T) {
			_, err := validateResearchClaims(tc.mutate(cloneResearchClaims(valid)), valid, sources)
			require.Error(t, err)
			require.Equal(t, tc.code, researchFailure("supplement_reviewing", err).Code)
			require.NotContains(t, err.Error(), "private-provider-text")
		})
	}
}
