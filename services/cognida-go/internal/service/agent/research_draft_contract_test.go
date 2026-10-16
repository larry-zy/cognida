package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestResearchDraftContractDistinguishesMissingEmptyAndExcessWithoutRawText(t *testing.T) {
	const private = "private-provider-field-or-body"
	var good map[string]any
	require.NoError(t, json.Unmarshal([]byte(researchGoodDraft()), &good))
	one := good["findings"].([]any)[0]
	for _, tc := range []struct {
		name, code    string
		fields        map[string]any
		count         int
		present, null bool
		unknown       int
	}{
		{"missing", "draft_required_fields", map[string]any{"summary": "Summary [S1]", "analysis": "Analysis [S1]"}, 0, false, false, 0},
		{"null", "draft_required_fields", map[string]any{"summary": "Summary [S1]", "analysis": "Analysis [S1]", "findings": nil}, 0, true, true, 0},
		{"renamed", "draft_required_fields", map[string]any{"summary": "Summary [S1]", "analysis": "Analysis [S1]", "key_findings": []any{one}}, 0, false, false, 1},
		{"empty", "draft_finding_count", map[string]any{"summary": "Summary [S1]", "analysis": "Analysis [S1]", "findings": []any{}}, 0, true, false, 0},
		{"excess", "draft_finding_count", map[string]any{"summary": "Summary [S1]", "analysis": "Analysis [S1]", "findings": []any{one, one, one, one, one, one, one, one, one}}, 9, true, false, 0},
		{"unknown key", "draft_fields_unknown", map[string]any{"summary": "Summary [S1]", "analysis": "Analysis [S1]", "findings": []any{one}, private: private}, 1, true, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, e := json.Marshal(tc.fields)
			require.NoError(t, e)
			calls := 0
			svc := newResearchTestService(t, false, researchGoodDraft())
			svc.model = researchTestModel{run: func(_ context.Context, messages []*schema.Message) (*schema.Message, error) {
				calls++
				if calls == 1 {
					return schema.AssistantMessage(`{"questions":[{"question":"specific question","strategy":"hybrid"}]}`, nil), nil
				}
				require.Contains(t, messages[0].Content, "至少1条、至多8条")
				require.Contains(t, messages[0].Content, "三个顶层字段 summary、analysis、findings 全部必填")
				return schema.AssistantMessage(string(body), nil), nil
			}}
			report, e := svc.ExecuteGoverned(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}})
			require.Error(t, e)
			require.Equal(t, 3, calls, "one structural repair may run, but invalid output never reaches review")
			require.False(t, report.Success)
			require.False(t, report.Metadata.CitationChecked)
			require.False(t, report.Metadata.SemanticReview)
			require.Equal(t, tc.code, report.Metadata.Failure.Code)
			require.Equal(t, "repair_validating", report.Metadata.Failure.Stage)
			require.Equal(t, 1, report.Metadata.DraftRepair.Attempts)
			require.Equal(t, tc.code, report.Metadata.DraftRepair.Trigger.Code)
			shape := report.Metadata.Failure.Structure
			require.NotNil(t, shape)
			require.Equal(t, tc.count, shape.FindingsCount)
			require.Equal(t, tc.present, shape.FindingsPresent)
			require.Equal(t, tc.null, shape.FindingsNull)
			require.Equal(t, tc.unknown, shape.UnknownFieldCount)
			require.Equal(t, 1, shape.MinFindings)
			require.Equal(t, 8, shape.MaxFindings)
			encoded, e := json.Marshal(report.Metadata.Failure)
			require.NoError(t, e)
			require.NotContains(t, string(encoded), private)
			require.NotContains(t, string(encoded), "Summary [S1]")
			require.Empty(t, report.Claims)
			require.Empty(t, report.KeyFindings)
		})
	}
}

func TestResearchDraftContractAcceptsBoundsAndRejectsUnknownQuotes(t *testing.T) {
	for _, count := range []int{1, 8} {
		var draft researchDraft
		require.NoError(t, json.Unmarshal([]byte(researchGoodDraft()), &draft))
		first := draft.Findings[0]
		draft.Findings = nil
		for i := 0; i < count; i++ {
			draft.Findings = append(draft.Findings, first)
		}
		require.NoError(t, validateResearchDraft(&draft, []ReportSourceDTO{{ID: "S1", Excerpt: researchExcerpt}}))
		draft.Findings[count-1].Citations = []researchCitation{{SourceID: "S1", Quote: strings.Repeat("invented", 2)}}
		err := validateResearchDraft(&draft, []ReportSourceDTO{{ID: "S1", Excerpt: researchExcerpt}})
		require.Error(t, err)
		require.Equal(t, "draft_quote_mismatch", researchFailure("validating", err).Code)
	}
}
