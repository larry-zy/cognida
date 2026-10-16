package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "cognida/internal/model/llm"
	"github.com/stretchr/testify/require"
)

func TestEinoBoundedJSONIsExplicitPerRequest(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		provider                  domain.Provider
		model                     string
		enabled, tools, supported bool
	}{
		{"supervisor", domain.ProviderDeepSeek, "deepseek-v4-flash", true, false, true},
		{"ordinary chat", domain.ProviderDeepSeek, "deepseek-v4-flash", false, false, false},
		{"ordinary tool caller", domain.ProviderDeepSeek, "deepseek-v4-flash", false, true, false},
		{"other provider", domain.ProviderOpenAI, "deepseek-v4-flash", true, false, false},
		{"legacy model", domain.ProviderDeepSeek, "deepseek-chat", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]json.RawMessage, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				requests <- request
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`))
			}))
			defer server.Close()
			client, err := NewEinoLLMClient(&domain.ModelConfig{Provider: tc.provider, ModelName: tc.model, APIKey: "fixture-only", BaseURL: server.URL})
			require.NoError(t, err)
			options := &domain.ChatOptions{MaxTokens: 4096, BoundedJSONOutput: tc.enabled}
			if tc.tools {
				options.Tools = []*domain.Tool{{Type: "function", Function: &domain.FunctionSpec{Name: "fixture", Description: "fixture", Parameters: map[string]any{"type": "object"}}}}
			}
			_, err = client.Chat(context.Background(), &domain.ChatRequest{Messages: []*domain.Message{{Role: domain.RoleUser, Content: "Return JSON."}}, Options: options})
			require.NoError(t, err)
			request := <-requests
			require.JSONEq(t, `4096`, string(request["max_tokens"]))
			if tc.supported {
				require.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
				require.JSONEq(t, `{"type":"json_object"}`, string(request["response_format"]))
			} else {
				require.NotContains(t, request, "thinking")
				require.NotContains(t, request, "response_format")
			}
			if tc.tools {
				require.Contains(t, request, "tools")
			} else {
				require.NotContains(t, request, "tools")
			}
			// The same configured client must not retain a previous request's hint.
			_, err = client.Chat(context.Background(), &domain.ChatRequest{Messages: []*domain.Message{{Role: domain.RoleUser, Content: "ordinary"}}})
			require.NoError(t, err)
			normal := <-requests
			require.NotContains(t, normal, "thinking")
			require.NotContains(t, normal, "response_format")
		})
	}
	var userOptions domain.ChatOptions
	require.NoError(t, json.Unmarshal([]byte(`{"BoundedJSONOutput":true,"bounded_json_output":true}`), &userOptions))
	require.False(t, userOptions.BoundedJSONOutput)
}
