package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cognida/internal/model/common"
)

// newOllamaTestServer 启动一个模拟 Ollama /api/chat 的测试服务器
func newOllamaTestServer(t *testing.T, handler func(req ollamaRequest, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req ollamaRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		handler(req, w)
	}))
}

func TestOllamaChat_NonStream(t *testing.T) {
	srv := newOllamaTestServer(t, func(req ollamaRequest, w http.ResponseWriter) {
		if req.Model != "llama3" {
			t.Errorf("expected model llama3, got %s", req.Model)
		}
		if req.Stream {
			t.Errorf("expected non-stream request")
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "hello" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}
		resp := ollamaChatResponse{
			Model:   "llama3",
			Message: ollamaMessage{Role: "assistant", Content: "hi there"},
			Done:    true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	c, err := NewOllamaChat(&ChatConfig{
		Source:    common.ModelSourceLocal,
		BaseURL:   srv.URL,
		ModelName: "llama3",
		ModelID:   "m1",
	})
	if err != nil {
		t.Fatalf("NewOllamaChat failed: %v", err)
	}

	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, &ChatOptions{Temperature: 0.5})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content != "hi there" {
		t.Errorf("expected content 'hi there', got %q", resp.Content)
	}
	if c.GetModelName() != "llama3" || c.GetModelID() != "m1" {
		t.Errorf("unexpected model info: %s / %s", c.GetModelName(), c.GetModelID())
	}
}

func TestOllamaChat_ToolCalls(t *testing.T) {
	srv := newOllamaTestServer(t, func(req ollamaRequest, w http.ResponseWriter) {
		resp := ollamaChatResponse{
			Model: "llama3",
			Message: ollamaMessage{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{
					{Function: ollamaToolCallFunction{
						Name:      "get_weather",
						Arguments: map[string]interface{}{"city": "Beijing"},
					}},
				},
			},
			Done: true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	c, _ := NewOllamaChat(&ChatConfig{Source: common.ModelSourceLocal, BaseURL: srv.URL, ModelName: "llama3"})
	resp, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "weather?"}}, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected tool get_weather, got %s", tc.Function.Name)
	}
	if !strings.Contains(tc.Function.Arguments, "Beijing") {
		t.Errorf("expected arguments to contain Beijing, got %s", tc.Function.Arguments)
	}
}

func TestOllamaChat_Stream(t *testing.T) {
	srv := newOllamaTestServer(t, func(req ollamaRequest, w http.ResponseWriter) {
		if !req.Stream {
			t.Errorf("expected stream request")
		}
		flusher, _ := w.(http.Flusher)
		chunks := []ollamaChatResponse{
			{Message: ollamaMessage{Role: "assistant", Content: "Hello"}, Done: false},
			{Message: ollamaMessage{Role: "assistant", Content: " world"}, Done: false},
			{Message: ollamaMessage{Role: "assistant", Content: ""}, Done: true},
		}
		for _, ch := range chunks {
			b, _ := json.Marshal(ch)
			_, _ = w.Write(append(b, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
	defer srv.Close()

	c, _ := NewOllamaChat(&ChatConfig{Source: common.ModelSourceLocal, BaseURL: srv.URL, ModelName: "llama3"})
	ch, err := c.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var content strings.Builder
	gotEnd := false
	for resp := range ch {
		switch resp.Event {
		case EventContent:
			content.WriteString(resp.Content)
		case EventEnd:
			gotEnd = true
		case EventError:
			t.Fatalf("stream error: %s", resp.Error)
		}
	}
	if content.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", content.String())
	}
	if !gotEnd {
		t.Errorf("expected end event")
	}
}

func TestOllamaChat_DefaultBaseURL(t *testing.T) {
	c, err := NewOllamaChat(&ChatConfig{Source: common.ModelSourceLocal, ModelName: "llama3"})
	if err != nil {
		t.Fatalf("NewOllamaChat failed: %v", err)
	}
	oc := c.(*ollamaChat)
	if oc.client.baseURL != "http://localhost:11434" {
		t.Errorf("expected default baseURL, got %s", oc.client.baseURL)
	}
}

func TestOllamaChat_MissingModel(t *testing.T) {
	_, err := NewOllamaChat(&ChatConfig{Source: common.ModelSourceLocal})
	if err == nil {
		t.Error("expected error for missing model_name")
	}
}

func TestNewChat_LocalRoutesToOllama(t *testing.T) {
	c, err := NewChat(&ChatConfig{Source: common.ModelSourceLocal, ModelName: "llama3"})
	if err != nil {
		t.Fatalf("NewChat failed: %v", err)
	}
	if _, ok := c.(*ollamaChat); !ok {
		t.Errorf("expected *ollamaChat, got %T", c)
	}
}
