package taskcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cognida/internal/model/llm"
	"github.com/stretchr/testify/require"
)

func TestPlannerOutputDiagnosticsAreSafeAndDoNotReplay(t *testing.T) {
	valid := plannerDecisionJSON(t, plannerClarification())
	for _, tc := range []struct{ name, content, finish, code string }{
		{"valid JSON but truncated", valid, "length", "truncated_output"},
		{"malformed", `{"secret":"sensitive-provider-body"`, "stop", "invalid_json"},
		{"missing contract", `{"secret":"sensitive-provider-body"}`, "stop", "invalid_contract"},
		{"empty", "", "stop", "empty_response"},
		{"unknown finish", valid, "sensitive-provider-body", "unsupported_finish_reason"},
		{"oversized", strings.Repeat("a", plannerOutputBytes+1), "stop", "output_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &llm.ChatResponse{Content: tc.content, Thinking: "sensitive-private-reasoning", FinishReason: tc.finish, Usage: &llm.Usage{PromptTokens: 100, CompletionTokens: 4096, TotalTokens: 4196}}
			client := &plannerFakeClient{chat: func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) { return response, nil }}
			decision, err := NewLLMPlanner(client).Propose(context.Background(), plannerTestInput())
			require.ErrorIs(t, err, ErrPlannerOutput)
			require.Empty(t, decision.Kind)
			require.Equal(t, 1, client.calls)
			diagnostic := PlannerDiagnostic(err)
			require.NotNil(t, diagnostic)
			require.Equal(t, tc.code, diagnostic.Code)
			require.Equal(t, 4096, diagnostic.MaxOutputTokens)
			require.Equal(t, 4096, diagnostic.Usage.CompletionTokens)
			raw, e := json.Marshal(diagnostic)
			require.NoError(t, e)
			require.NotContains(t, string(raw), "sensitive")
			require.NotContains(t, err.Error(), "sensitive")
			if tc.name == "unknown finish" {
				require.Equal(t, "unknown", diagnostic.FinishReason)
			}
			// Callers cannot alter the failure's preserved diagnostic through its DTO.
			diagnostic.Usage.CompletionTokens = 0
			require.Equal(t, 4096, PlannerDiagnostic(err).Usage.CompletionTokens)
		})
	}
	require.Nil(t, PlannerDiagnostic(errors.New("sensitive generic error")))
	err := plannerOutputFailure("empty_response", &llm.ChatResponse{Usage: &llm.Usage{TotalTokens: -1}})
	require.Nil(t, PlannerDiagnostic(err).Usage)
}
