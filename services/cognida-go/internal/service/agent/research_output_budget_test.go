package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cognida/internal/infrastructure/llm/chat"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

// A local HTTP provider fixture exercises the production chat adapter and all
// three structured research stages. It is not a live-model quality assertion.
func TestResearchBoundedStructuredGenerationHTTPKeepsIndependentReview(t *testing.T) {
	for _, truncateReview := range []bool{false, true} {
		name := "complete"
		if truncateReview {
			name = "valid review JSON marked truncated"
		}
		t.Run(name, func(t *testing.T) {
			requests := make(chan map[string]json.RawMessage, 4)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- request
				content, finish := "", "stop"
				switch calls.Add(1) {
				case 1:
					content = `{"questions":[{"question":"specific question","strategy":"hybrid"}]}`
				case 2:
					content = researchGoodDraft()
				case 3:
					out, _ := json.Marshal(map[string]any{"claims": []ResearchClaim{{ID: "C1", Status: "supported", Reason: "The exact source supports this finding.", Evidence: []ResearchClaimEvidence{{SourceID: "S1", Quote: researchExcerpt, Relation: "supports"}}}}})
					content = string(out)
					if truncateReview {
						finish = "length"
					}
				default:
					t.Error("unexpected automatic paid-call retry")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 150, "total_tokens": 250}})
			}))
			defer server.Close()
			llm, err := chat.NewToolCallingChatModel(context.Background(), &chat.ChatConfig{BaseURL: server.URL, APIKey: "fixture-only", Provider: "deepseek", ModelName: "deepseek-v4-flash"})
			require.NoError(t, err)
			svc := newResearchTestService(t, false, researchGoodDraft())
			svc.model = llm
			report, err := svc.ExecuteGovernedWithProgress(researchTestContext(), &DeepResearchRequest{Query: "question", KBIDs: []string{"allowed"}}, func(p *ResearchProgressDTO) {
				require.Equal(t, researchVerificationLimit(p.Report.Metadata), p.Report.Metadata.VerificationLimit, "every persisted checkpoint must describe its actual validation state")
			})
			require.EqualValues(t, 3, calls.Load(), "semantic review must remain a separate model call")
			require.Equal(t, 3, report.Metadata.ModelCallsUsed)
			require.True(t, report.Metadata.CitationChecked)
			require.False(t, report.Metadata.FactChecked)
			for _, expected := range []int{2048, 8192, 8192} {
				request := <-requests
				var maxTokens int
				require.NoError(t, json.Unmarshal(request["max_tokens"], &maxTokens))
				require.Equal(t, expected, maxTokens)
				require.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
				require.JSONEq(t, `{"type":"json_object"}`, string(request["response_format"]))
				require.NotContains(t, request, "tools")
			}
			if truncateReview {
				require.Error(t, err)
				require.False(t, report.Success)
				require.False(t, report.Metadata.SemanticReview)
				require.Equal(t, "reviewing", report.Metadata.Failure.Stage)
				require.Equal(t, "truncated_output", report.Metadata.Failure.Code)
				require.Equal(t, 150, report.Metadata.Failure.Output.Usage.CompletionTokens)
				require.Contains(t, report.Metadata.VerificationLimit, "语义审查尚未完成")
			} else {
				require.NoError(t, err)
				require.True(t, report.Success)
				require.True(t, report.Metadata.SemanticReview)
				require.Equal(t, 1, report.Metadata.ReviewRounds)
				require.True(t, report.Claims[0].Evidence[0].QuoteMatched)
				require.Contains(t, report.Metadata.VerificationLimit, "受限模型审查意见")
			}
		})
	}
}

func TestResearchRejectsInvalidOutputBudgetBeforeCallingModel(t *testing.T) {
	calls := 0
	svc := newResearchTestService(t, false, researchGoodDraft())
	svc.model = researchTestModel{run: func(context.Context, []*schema.Message) (*schema.Message, error) {
		calls++
		return nil, nil
	}}
	for _, maxTokens := range []int{-1, 0, 8193} {
		var out map[string]any
		require.Error(t, svc.generateJSON(withResearchBudget(context.Background()), "Return JSON.", nil, &out, maxTokens))
	}
	require.Zero(t, calls)
}
