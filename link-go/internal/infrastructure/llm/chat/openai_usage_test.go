package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestGenerate_PopulatesUsage 锁定回归：非流式响应的 usage 必须回填进 ResponseMeta.Usage，
// 否则 Agent 侧 usageTotalTokens(msg) 恒为 0，评测 tokens_used 全落 0（本次修复的核心缺陷）。
func TestGenerate_PopulatesUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openaiChatResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Role: "assistant", Content: "hello"}}},
			Usage:   openaiUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20},
		})
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-chat")
	msg, err := c.Generate(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		t.Fatalf("ResponseMeta.Usage not populated: %+v", msg.ResponseMeta)
	}
	if got := msg.ResponseMeta.Usage.TotalTokens; got != 20 {
		t.Errorf("TotalTokens = %d, want 20", got)
	}
	if msg.ResponseMeta.Usage.PromptTokens != 12 || msg.ResponseMeta.Usage.CompletionTokens != 8 {
		t.Errorf("prompt/completion tokens wrong: %+v", msg.ResponseMeta.Usage)
	}
}

// TestGenerate_NoUsage_OmitsResponseMeta 断言 usage 全 0（供应商未上报）时不伪造零用量，
// 保持"未上报"与"确为 0"可区分。
func TestGenerate_NoUsage_OmitsResponseMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openaiChatResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Role: "assistant", Content: "hi"}}},
		})
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-chat")
	msg, err := c.Generate(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if msg.ResponseMeta != nil {
		t.Errorf("expected nil ResponseMeta when usage absent, got %+v", msg.ResponseMeta)
	}
}

// TestBuildRequest_StreamRequestsUsage 断言流式请求显式带上 stream_options.include_usage，
// 否则服务端不追发 usage chunk，流式 token 用量拿不到。
func TestBuildRequest_StreamRequestsUsage(t *testing.T) {
	c := newOpenAITestClient(t, "http://unused", "deepseek-chat")

	streamReq := c.buildRequest([]*schema.Message{schema.UserMessage("hi")}, nil, true)
	if streamReq.StreamOptions == nil || !streamReq.StreamOptions.IncludeUsage {
		t.Errorf("stream request must set stream_options.include_usage=true, got %+v", streamReq.StreamOptions)
	}

	genReq := c.buildRequest([]*schema.Message{schema.UserMessage("hi")}, nil, false)
	if genReq.StreamOptions != nil {
		t.Errorf("non-stream request must not set stream_options, got %+v", genReq.StreamOptions)
	}
}

// TestStream_EmitsUsageFromFinalChunk 断言流末仅含 usage 的 chunk 被解析，并作为结尾消息
// 携带 ResponseMeta.Usage 下发，供 eino 拼接时并入 token 用量。
func TestStream_EmitsUsageFromFinalChunk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, _ := w.(http.Flusher)
		write := func(s string) {
			_, _ = w.Write([]byte(s))
			if fl != nil {
				fl.Flush()
			}
		}
		// 正文分片
		write("data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}\n")
		// finish
		write("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n")
		// 流末 usage chunk（choices 为空）
		write("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n")
		write("data: [DONE]\n")
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-chat")
	reader, err := c.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer reader.Close()

	var total int
	for {
		msg, recvErr := reader.Recv()
		if recvErr != nil {
			break
		}
		if msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
			total = msg.ResponseMeta.Usage.TotalTokens
		}
	}
	if total != 8 {
		t.Errorf("stream usage total_tokens = %d, want 8", total)
	}
}
