package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestBoundedJSONOutputHTTPProviderAndModelContract(t *testing.T) {
	for _, tc := range []struct {
		name, provider, model           string
		requested, supported, bindTools bool
	}{
		{"flash explicit", "deepseek", "deepseek-v4-flash", true, true, false},
		{"pro explicit", "deepseek", "deepseek-v4-pro", true, true, false},
		{"bound clone", "deepseek", "deepseek-v4-flash", true, true, true},
		{"ordinary agent unchanged", "deepseek", "deepseek-v4-flash", false, false, false},
		{"other provider unchanged", "openai", "deepseek-v4-flash", true, false, false},
		{"provider required", "", "deepseek-v4-flash", true, false, false},
		{"legacy reasoner unchanged", "deepseek", "deepseek-reasoner", true, false, false},
		{"legacy chat unchanged", "deepseek", "deepseek-chat", true, false, false},
		{"unknown future model unchanged", "deepseek", "deepseek-v5-flash", true, false, false},
		{"unrelated model unchanged", "deepseek", "gpt-4o", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]json.RawMessage, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				requests <- request
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`))
			}))
			defer srv.Close()
			client, err := NewToolCallingChatModel(context.Background(), &ChatConfig{Provider: tc.provider, ModelName: tc.model, APIKey: "fixture-only", BaseURL: srv.URL})
			require.NoError(t, err)
			if tc.bindTools {
				client, err = client.WithTools(nil)
				require.NoError(t, err)
			}
			opts := []model.Option{model.WithMaxTokens(8192)}
			if tc.requested {
				opts = append(opts, WithBoundedJSONOutput())
			}
			_, err = client.Generate(context.Background(), []*schema.Message{schema.SystemMessage("Return JSON."), schema.UserMessage("fixture")}, opts...)
			require.NoError(t, err)
			request := <-requests
			require.JSONEq(t, `8192`, string(request["max_tokens"]))
			if tc.supported {
				require.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
				require.JSONEq(t, `{"type":"json_object"}`, string(request["response_format"]))
			} else {
				require.NotContains(t, request, "thinking")
				require.NotContains(t, request, "response_format")
			}
		})
	}
}
