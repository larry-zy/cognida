package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func citationRepairDraft(t *testing.T, change func(map[string]any)) string {
	t.Helper()
	var draft map[string]any
	require.NoError(t, json.Unmarshal([]byte(researchGoodDraft()), &draft))
	change(draft)
	raw, err := json.Marshal(draft)
	require.NoError(t, err)
	return string(raw)
}

func TestResearchMissingCitationsRequireRepairAndIndependentReview(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(map[string]any)
	}{
		{"summary marker", "draft_citations_missing", func(d map[string]any) { d["summary"] = "Summary without a marker" }},
		{"analysis marker", "draft_citations_missing", func(d map[string]any) { d["analysis"] = "Analysis without a marker" }},
		{"empty summary", "draft_citations_missing", func(d map[string]any) { d["summary"] = "" }},
		{"missing finding citations", "draft_finding_citations_missing", func(d map[string]any) { delete(d["findings"].([]any)[0].(map[string]any), "citations") }},
		{"null finding citations", "draft_finding_citations_missing", func(d map[string]any) { d["findings"].([]any)[0].(map[string]any)["citations"] = nil }},
		{"empty finding citations", "draft_finding_citations_missing", func(d map[string]any) { d["findings"].([]any)[0].(map[string]any)["citations"] = []any{} }},
	} {
		for _, corrected := range []bool{true, false} {
			t.Run(tc.name+map[bool]string{true: "/review required", false: "/still missing fails"}[corrected], func(t *testing.T) {
				draft := citationRepairDraft(t, tc.change)
				calls, checkpoints := 0, 0
				svc := newResearchTestService(t, false, researchGoodDraft())
				var originalSources json.RawMessage
				svc.model = researchTestModel{run: func(_ context.Context, messages []*schema.Message) (*schema.Message, error) {
					calls++
					switch calls {
					case 1:
						return schema.AssistantMessage(`{"questions":[{"question":"specific question","strategy":"hybrid"}]}`, nil), nil
					case 2:
						var input map[string]json.RawMessage
						require.NoError(t, json.Unmarshal([]byte(messages[1].Content), &input))
						originalSources = input["sources"]
						return schema.AssistantMessage(draft, nil), nil
					case 3:
						require.Equal(t, 1, checkpoints, "reservation is checkpointed before paid repair")
						require.Contains(t, messages[0].Content, "一次独立的结构修复")
						var input map[string]json.RawMessage
						require.NoError(t, json.Unmarshal([]byte(messages[1].Content), &input))
						require.JSONEq(t, string(originalSources), string(input["sources"]))
						if !corrected {
							return schema.AssistantMessage(draft, nil), nil
						}
						return schema.AssistantMessage(researchGoodDraft(), nil), nil
					case 4:
						require.Contains(t, messages[0].Content, "证据审查员")
						body, _ := json.Marshal(map[string]any{"claims": []ResearchClaim{{ID: "C1", Status: "insufficient", Reason: "The source alone does not establish the claim.", Evidence: []ResearchClaimEvidence{{SourceID: "S1", Quote: researchExcerpt, Relation: "context"}}}}})
						return schema.AssistantMessage(string(body), nil), nil
					default:
						t.Fatal("no fifth model call is allowed")
						return nil, nil
					}
				}}
				report, err := svc.ExecuteGovernedWithProgress(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}}, func(p *ResearchProgressDTO) {
					if p.Stage == "repairing" {
						checkpoints++
						require.Equal(t, 2, p.Report.Metadata.ModelCallsUsed)
						require.Equal(t, 2, p.Report.Metadata.ModelCallsReserved)
						require.Zero(t, p.Report.Metadata.DraftRepair.Attempts)
						require.Empty(t, p.Report.Claims)
						require.Empty(t, p.Report.ExecutiveSummary)
					}
				})
				require.Equal(t, tc.code, report.Metadata.DraftRepair.Trigger.Code)
				require.Equal(t, 1, report.Metadata.DraftRepair.Attempts)
				require.Zero(t, report.Metadata.ModelCallsReserved)
				if corrected {
					require.NoError(t, err)
					require.Equal(t, 4, calls)
					require.True(t, report.Metadata.CitationChecked)
					require.True(t, report.Metadata.SemanticReview)
					require.Equal(t, 1, report.Metadata.ReviewRounds)
					require.Equal(t, "insufficient", report.Claims[0].Status)
					require.NotEmpty(t, report.Unverified)
					require.Equal(t, "model_call_budget_exhausted", report.Metadata.SupplementalSkippedReason)
				} else {
					require.Error(t, err)
					require.Equal(t, 3, calls)
					require.Equal(t, tc.code, report.Metadata.Failure.Code)
					require.Equal(t, "repair_validating", report.Metadata.Failure.Stage)
					require.False(t, report.Success)
					require.False(t, report.Metadata.CitationChecked)
					require.False(t, report.Metadata.SemanticReview)
					require.False(t, UsableResearchPartial(report, err))
					require.Empty(t, report.Claims)
				}
			})
		}
	}
}

func TestResearchMissingCitationDoesNotHideAnyExistingInvalidCitation(t *testing.T) {
	for _, omission := range []string{"summary", "first finding"} {
		for _, invalid := range []string{"summary source", "analysis source", "statement source", "citation source", "empty source", "short quote", "empty quote", "fabricated quote"} {
			t.Run(omission+"/"+invalid, func(t *testing.T) {
				draft := citationRepairDraft(t, func(d map[string]any) {
					// Put the invalid evidence in a later finding, beyond the first
					// missing-citation error returned by ordinary validation.
					first := d["findings"].([]any)[0].(map[string]any)
					second := map[string]any{"statement": "Another finding", "citations": []any{map[string]any{"source_id": "S1", "quote": researchExcerpt}}}
					d["findings"] = []any{first, second}
					if omission == "summary" {
						d["summary"] = "Summary without marker"
					} else {
						delete(first, "citations")
					}
					citation := second["citations"].([]any)[0].(map[string]any)
					switch invalid {
					case "summary source":
						d["summary"] = "Unknown [S999]"
					case "analysis source":
						d["analysis"] = "Unknown [S999]"
					case "statement source":
						second["statement"] = "Unknown [S999]"
					case "citation source":
						citation["source_id"] = "S999"
					case "empty source":
						citation["source_id"] = ""
					case "short quote":
						citation["quote"] = "short"
					case "empty quote":
						citation["quote"] = ""
					case "fabricated quote":
						citation["quote"] = "An invented quotation that is not evidence."
					}
				})
				calls := 0
				svc := newResearchTestService(t, false, researchGoodDraft())
				svc.model = researchTestModel{run: func(context.Context, []*schema.Message) (*schema.Message, error) {
					calls++
					if calls == 1 {
						return schema.AssistantMessage(`{"questions":[{"question":"question","strategy":"hybrid"}]}`, nil), nil
					}
					return schema.AssistantMessage(draft, nil), nil
				}}
				report, err := svc.ExecuteGoverned(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}})
				require.Error(t, err)
				require.Equal(t, 2, calls, "present invalid evidence must not enter repair")
				require.Nil(t, report.Metadata.DraftRepair)
				require.False(t, report.Success)
				require.False(t, report.Metadata.SemanticReview)
				require.False(t, UsableResearchPartial(report, err))
				require.Empty(t, report.Claims)
			})
		}
	}
}
