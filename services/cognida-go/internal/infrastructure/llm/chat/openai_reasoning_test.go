package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// newOpenAITestClient 构造指向测试服务器的 openai 客户端
func newOpenAITestClient(t *testing.T, baseURL, modelName string) *openaiClient {
	t.Helper()
	c, err := newOpenAIClient(&ChatConfig{
		BaseURL:   baseURL,
		ModelName: modelName,
		APIKey:    "test-key",
	})
	if err != nil {
		t.Fatalf("newOpenAIClient failed: %v", err)
	}
	return c
}

func TestNeedsReasoningRoundTrip(t *testing.T) {
	cases := map[string]bool{
		"deepseek-v4-flash": true,
		"deepseek-v4-pro":   true,
		"DeepSeek-V4-Flash": true, // 大小写不敏感
		"deepseek-reasoner": false,
		"deepseek-chat":     false,
		"gpt-4o":            true, // 非 deepseek：默认放行（响应无 reasoning 时为空回传，无副作用）
	}
	for model, want := range cases {
		if got := needsReasoningRoundTrip(model); got != want {
			t.Errorf("needsReasoningRoundTrip(%q) = %v, want %v", model, got, want)
		}
	}
}

// TestGenerate_ParsesReasoningContent 断言非流式响应里的 reasoning_content 被解析进 schema.Message
func TestGenerate_ParsesReasoningContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiChatResponse{
			Choices: []openaiChoice{{
				Message: openaiMessage{
					Role:             "assistant",
					Content:          "",
					ReasoningContent: "让我想想……需要调用工具",
					ToolCalls: []openaiToolCall{{
						ID:       "call_1",
						Type:     "function",
						Function: openaiToolCallFunction{Name: "kb_list", Arguments: "{}"},
					}},
				},
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-v4-flash")
	msg, err := c.Generate(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if msg.ReasoningContent != "让我想想……需要调用工具" {
		t.Errorf("reasoning_content not parsed, got %q", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "kb_list" {
		t.Errorf("tool_calls not parsed: %+v", msg.ToolCalls)
	}
}

// captureRequest 启动服务器并回传收到的请求体（解码为 openaiRequest）
func captureRequest(t *testing.T) (*httptest.Server, *openaiRequest) {
	t.Helper()
	captured := &openaiRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, captured)
		// 返回一个无工具调用的收尾响应
		_ = json.NewEncoder(w).Encode(openaiChatResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Role: "assistant", Content: "done"}}},
		})
	}))
	return srv, captured
}

// assistantWithReasoning 构造一条带 reasoning_content + tool_calls 的 assistant 历史消息
func assistantWithReasoning() *schema.Message {
	return &schema.Message{
		Role:             schema.Assistant,
		Content:          "",
		ReasoningContent: "prior thinking",
		ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: schema.FunctionCall{Name: "kb_list", Arguments: "{}"},
		}},
	}
}

// TestBuildRequest_RoundTripsReasoning_V4 断言 V4 模型会把 assistant 轮的 reasoning_content 回传
func TestBuildRequest_RoundTripsReasoning_V4(t *testing.T) {
	srv, captured := captureRequest(t)
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-v4-flash")
	history := []*schema.Message{
		schema.UserMessage("q"),
		assistantWithReasoning(),
		schema.ToolMessage("result", "call_1"),
	}
	if _, err := c.Generate(context.Background(), history); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var found bool
	for _, m := range captured.Messages {
		if m.Role == "assistant" && m.ReasoningContent == "prior thinking" {
			found = true
		}
	}
	if !found {
		t.Errorf("V4: assistant reasoning_content not round-tripped; messages=%+v", captured.Messages)
	}
}

// TestBuildRequest_StripsReasoning_Legacy 断言旧版 deepseek-reasoner 不回传 reasoning_content（否则 400）
func TestBuildRequest_StripsReasoning_Legacy(t *testing.T) {
	srv, captured := captureRequest(t)
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-reasoner")
	history := []*schema.Message{
		schema.UserMessage("q"),
		assistantWithReasoning(),
		schema.ToolMessage("result", "call_1"),
	}
	if _, err := c.Generate(context.Background(), history); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, m := range captured.Messages {
		if m.ReasoningContent != "" {
			t.Errorf("legacy reasoner: reasoning_content must be stripped, got %q", m.ReasoningContent)
		}
	}
}
