package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// TestGenerate_CapturesFinishReason 锁定回归：非流式响应的 finish_reason 必须回填进
// ResponseMeta.FinishReason，否则上层无从识别 length 截断（thinking 链撑爆输出预算）。
func TestGenerate_CapturesFinishReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openaiChatResponse{
			Choices: []openaiChoice{{
				Message:      openaiMessage{Role: "assistant", Content: "半句被切断的正文"},
				FinishReason: "length",
			}},
			Usage: openaiUsage{PromptTokens: 10, CompletionTokens: 8192, TotalTokens: 8202},
		})
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-v4-flash")
	msg, err := c.Generate(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if msg.ResponseMeta == nil || msg.ResponseMeta.FinishReason != "length" {
		t.Fatalf("FinishReason not captured, got %+v", msg.ResponseMeta)
	}
}

// TestGenerate_FinishReasonWithoutUsage 断言即便 usage 缺失（部分供应商省略），只要有 finish_reason
// 就必须返回非 nil ResponseMeta——否则截断信号被 newResponseMeta 的「usage 全 0 返回 nil」吞掉。
func TestGenerate_FinishReasonWithoutUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openaiChatResponse{
			Choices: []openaiChoice{{
				Message:      openaiMessage{Role: "assistant", Content: "半句"},
				FinishReason: "length",
			}},
		})
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-v4-flash")
	msg, err := c.Generate(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if msg.ResponseMeta == nil || msg.ResponseMeta.FinishReason != "length" {
		t.Fatalf("FinishReason must survive even without usage, got %+v", msg.ResponseMeta)
	}
}

// TestStream_EmitsFinishReasonOnTruncation 复现生产故障的关键链路：thinking 模型流式输出正文分片后
// 以 finish_reason=length 结束、且没有任何 tool_calls。修复前该分支只在有 tool_calls 时下发消息，
// finish_reason 被整个丢弃；修复后必须始终下发一条携带 FinishReason 的收尾消息供上层识别截断。
func TestStream_EmitsFinishReasonOnTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, _ := w.(http.Flusher)
		write := func(s string) {
			_, _ = w.Write([]byte(s))
			if fl != nil {
				fl.Flush()
			}
		}
		// 正文分片（半句被切断），随后 finish_reason=length、无 tool_calls。
		write("data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"我先查看可用的\"}}]}\n")
		write("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"length\"}]}\n")
		write("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":8192,\"total_tokens\":8202}}\n")
		write("data: [DONE]\n")
	}))
	defer srv.Close()

	c := newOpenAITestClient(t, srv.URL, "deepseek-v4-flash")
	reader, err := c.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer reader.Close()

	var sawFinish bool
	for {
		msg, recvErr := reader.Recv()
		if recvErr != nil {
			break
		}
		if msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.FinishReason == "length" {
			sawFinish = true
		}
	}
	if !sawFinish {
		t.Error("stream must surface finish_reason=length even when no tool_calls (else truncation is invisible)")
	}
}

// TestBuildRequest_DefaultsMaxTokens 断言调用方未显式指定 max_tokens 时兜一个宽松默认，
// 避免依赖 provider 较小默认上限被 thinking 链耗尽而截断；显式指定时则原样尊重。
func TestBuildRequest_DefaultsMaxTokens(t *testing.T) {
	c := newOpenAITestClient(t, "http://unused", "deepseek-v4-flash")

	def := c.buildRequest([]*schema.Message{schema.UserMessage("hi")}, nil, true)
	if def.MaxTokens != defaultMaxOutputTokens {
		t.Errorf("expected default max_tokens=%d when caller omits it, got %d", defaultMaxOutputTokens, def.MaxTokens)
	}

	explicit := c.buildRequest(
		[]*schema.Message{schema.UserMessage("hi")},
		[]model.Option{model.WithMaxTokens(1234)},
		true,
	)
	if explicit.MaxTokens != 1234 {
		t.Errorf("explicit max_tokens must be respected, got %d", explicit.MaxTokens)
	}
}
