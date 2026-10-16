package governance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "cognida/internal/infrastructure/llm"
	"cognida/internal/model/llm"
	"github.com/stretchr/testify/require"
)

// Actual HTTP request capture: the benchmark changes no semantic instructions,
// token limit or retry policy, and unsupported provider/model pairs stay intact.
func TestLabelBenchmarkStructuredRequestContract(t *testing.T) {
	for _, tc := range []struct {
		provider   llm.Provider
		model      string
		structured bool
	}{
		{llm.ProviderDeepSeek, "deepseek-v4-flash", true},
		{llm.ProviderDeepSeek, "deepseek-chat", false},
		{llm.ProviderOpenAI, "deepseek-v4-flash", false},
	} {
		t.Run(string(tc.provider)+tc.model, func(t *testing.T) {
			in, candidate := labelCandidateFixture()
			raw, _ := json.Marshal(candidate)
			var requests []map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					w.WriteHeader(400)
					return
				}
				requests = append(requests, req)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": string(raw)}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			client, err := adapter.NewEinoLLMClient(&llm.ModelConfig{Provider: tc.provider, ModelName: tc.model, BaseURL: server.URL, APIKey: "fixture-only"})
			require.NoError(t, err)
			_, err = NewLLMLabelSuggester(client, tc.model).Suggest(context.Background(), in)
			require.NoError(t, err)
			require.Len(t, requests, 1)
			req := requests[0]
			require.JSONEq(t, `4096`, string(req["max_tokens"]))
			require.NotContains(t, req, "tools")
			if tc.structured {
				require.JSONEq(t, `{"type":"disabled"}`, string(req["thinking"]))
				require.JSONEq(t, `{"type":"json_object"}`, string(req["response_format"]))
			} else {
				require.NotContains(t, req, "thinking")
				require.NotContains(t, req, "response_format")
			}
			_, err = client.Chat(context.Background(), &llm.ChatRequest{Messages: []*llm.Message{{Role: llm.RoleUser, Content: "ordinary"}}})
			require.NoError(t, err)
			require.Len(t, requests, 2)
			require.NotContains(t, requests[1], "thinking")
			require.NotContains(t, requests[1], "response_format")
		})
	}
}
