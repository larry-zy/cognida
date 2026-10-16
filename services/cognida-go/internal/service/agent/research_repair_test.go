package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cognida/internal/service/agent/tools"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

const researchMissingFindings = `{"summary":"Summary [S1]","analysis":"Analysis [S1]"}`

func TestResearchStructureRepairKeepsIndependentReviewAndSkipsUnfundedSupplement(t *testing.T) {
	for _, governed := range []bool{true, false} {
		t.Run(map[bool]string{true: "governed", false: "ordinary requires review after repair"}[governed], func(t *testing.T) {
			calls, retrievals := 0, 0
			svc := newResearchTestService(t, false, researchGoodDraft())
			svc.tools.(researchTestRegistry)["rag_query"] = tools.NewTypedBaseTool("rag_query", "fixture", func(context.Context, *tools.RAGQueryRequest) (*tools.RAGQueryResult, error) {
				retrievals++
				return &tools.RAGQueryResult{Chunks: []tools.DocumentChunk{{Content: researchExcerpt, ChunkID: "c1", KnowledgeID: "d1", KnowledgeBaseID: "allowed"}, {Content: partialConflictQuote, ChunkID: "c2", KnowledgeID: "d2", KnowledgeBaseID: "allowed"}}}, nil
			})
			var originalSources json.RawMessage
			svc.model = researchTestModel{run: func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
				calls++
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 120*time.Second)
				switch calls {
				case 1:
					return schema.AssistantMessage(`{"questions":[{"question":"specific question","strategy":"hybrid"}]}`, nil), nil
				case 2:
					var input map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(messages[1].Content), &input))
					originalSources = input["sources"]
					return schema.AssistantMessage(researchMissingFindings, nil), nil
				case 3:
					require.Contains(t, messages[0].Content, "一次独立的结构修复")
					var input map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(messages[1].Content), &input))
					require.JSONEq(t, string(originalSources), string(input["sources"]))
					require.Contains(t, input, "untrusted_draft")
					return schema.AssistantMessage(researchGoodDraft(), nil), nil
				case 4:
					require.Contains(t, messages[0].Content, "证据审查员")
					body, _ := json.Marshal(map[string]any{"claims": []ResearchClaim{{ID: "C1", Status: "insufficient", Reason: "Evidence does not establish the claim.", Evidence: []ResearchClaimEvidence{{SourceID: "S2", Quote: partialConflictQuote, Relation: "context"}}}}})
					return schema.AssistantMessage(string(body), nil), nil
				default:
					t.Fatal("fifth model call")
					return nil, nil
				}
			}}
			var checkpoints []*ResearchProgressDTO
			emit := func(p *ResearchProgressDTO) {
				checkpoints = append(checkpoints, p)
				if p.Stage == "repairing" {
					require.Equal(t, 2, p.Report.Metadata.ModelCallsUsed)
					require.Equal(t, 2, p.Report.Metadata.ModelCallsReserved)
					require.Zero(t, p.Report.Metadata.DraftRepair.Attempts)
					require.Empty(t, p.Report.Claims)
					require.Empty(t, p.Report.ExecutiveSummary)
				}
			}
			req := &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}}
			var report *DeepResearchResponse
			var err error
			if governed {
				report, err = svc.ExecuteGovernedWithProgress(researchTestContext(), req, emit)
			} else {
				limits, e := svc.validate(researchTestContext(), req)
				require.NoError(t, e)
				report, err = svc.execute(researchTestContext(), req, limits, emit)
			}
			require.NoError(t, err)
			require.True(t, report.Success)
			require.True(t, report.Metadata.Partial)
			require.True(t, report.Metadata.CitationChecked)
			require.True(t, report.Metadata.SemanticReview)
			require.Equal(t, 4, calls)
			require.Equal(t, 1, retrievals)
			require.Equal(t, 4, report.Metadata.ModelCallsUsed)
			require.Zero(t, report.Metadata.ModelCallsReserved)
			require.Equal(t, "completed", report.Metadata.DraftRepair.Status)
			require.Equal(t, 1, report.Metadata.DraftRepair.Attempts)
			require.Equal(t, "draft_required_fields", report.Metadata.DraftRepair.Trigger.Code)
			require.Equal(t, "model_call_budget_exhausted", report.Metadata.SupplementalSkippedReason)
			require.Len(t, report.ReviewHistory, 1)
			require.Len(t, report.Sources, 2)
			require.Contains(t, report.Sources[1].Excerpt, partialConflictQuote)
			require.NotEmpty(t, report.Unverified)
			require.False(t, UsableResearchPartial(report, errors.New("not a supplemental failure")))
		})
	}
}

func TestResearchRepairCheckpointCancellationDoesNotInvokeModel(t *testing.T) {
	ctx, cancel := context.WithCancel(researchTestContext())
	defer cancel()
	svc := newResearchTestService(t, false, researchGoodDraft())
	calls := 0
	svc.model = researchTestModel{run: func(context.Context, []*schema.Message) (*schema.Message, error) {
		calls++
		if calls == 1 {
			return schema.AssistantMessage(`{"questions":[{"question":"question","strategy":"hybrid"}]}`, nil), nil
		}
		return schema.AssistantMessage(researchMissingFindings, nil), nil
	}}
	report, err := svc.ExecuteGovernedWithProgress(ctx, &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}}, func(p *ResearchProgressDTO) {
		if p.Stage == "repairing" {
			require.Equal(t, 2, p.Report.Metadata.ModelCallsReserved)
			cancel()
		}
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, calls)
	require.Equal(t, 2, report.Metadata.ModelCallsUsed)
	require.Zero(t, report.Metadata.ModelCallsReserved)
	require.Zero(t, report.Metadata.DraftRepair.Attempts)
	require.False(t, report.Success)
	require.False(t, UsableResearchPartial(report, err))
}

func TestResearchRepairRejectsCompoundCitationErrors(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(d map[string]any) { d["extra"] = "private-field" },
		func(d map[string]any) { delete(d, "summary") },
		func(d map[string]any) {
			one := d["findings"].([]any)[0]
			d["findings"] = []any{one, one, one, one, one, one, one, one, one}
		},
	} {
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(researchGoodDraft(), researchExcerpt, "Fabricated quotation not in any source.")), &body))
		change(body)
		raw, _ := json.Marshal(body)
		var draft researchDraft
		require.NoError(t, json.Unmarshal(raw, &draft))
		sources := []ReportSourceDTO{{ID: "S1", Excerpt: researchExcerpt}}
		err := validateResearchDraft(&draft, sources)
		require.Error(t, err)
		require.False(t, repairableResearchDraft(&draft, sources, err))
	}
}

func TestResearchRepairReservationProtectsBothBudgetsAndReleasesUnusedCalls(t *testing.T) {
	for _, models := range []int{2, 3} {
		for _, total := range []int{18, 19} {
			ctx := withResearchBudget(context.Background())
			b := ctx.Value(researchBudgetKey{}).(*researchBudget)
			b.models = models
			b.calls = total
			reserved, release, err := reserveResearchModelCalls(ctx, 2)
			if models == 3 || total == 19 {
				require.Error(t, err)
				require.Nil(t, release)
				continue
			}
			require.NoError(t, err)
			require.Error(t, useResearchCall(ctx, true))
			require.Error(t, useResearchCall(ctx, false))
			require.NoError(t, useResearchCall(reserved, true))
			release()
			release()
			calls, used := researchCalls(ctx)
			require.Equal(t, 19, calls)
			require.Equal(t, 3, used)
			rc, rm := researchReservedCalls(ctx)
			require.Zero(t, rc)
			require.Zero(t, rm)
		}
	}
}
