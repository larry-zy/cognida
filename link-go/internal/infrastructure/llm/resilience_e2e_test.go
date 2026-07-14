package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	domainllm "link/internal/model/llm"
)

// ========================================
// Group 7：端到端与回归（真实 HTTP 栈：provider client → 弹性装饰器 → 工厂链）
// ========================================

// scriptedServer 是一个可注入故障的假 provider。每次 /chat/completions 请求
// 按 responses 序列应答（耗尽后沿用最后一项），并记录命中次数。
type scriptedServer struct {
	*httptest.Server
	hits      int32
	responses []func(w http.ResponseWriter)
}

func (s *scriptedServer) Hits() int { return int(atomic.LoadInt32(&s.hits)) }

func newScriptedServer(responses ...func(w http.ResponseWriter)) *scriptedServer {
	s := &scriptedServer{responses: responses}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&s.hits, 1)) - 1
		idx := n
		if idx >= len(s.responses) {
			idx = len(s.responses) - 1
		}
		s.responses[idx](w)
	}))
	return s
}

// 常用应答构造子。
func respOK(content string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"1","object":"chat.completion","model":"m",
			"choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, content)
	}
}

func respStatus(code int) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(code)
		fmt.Fprintf(w, `{"error":{"message":"status %d"}}`, code)
	}
}

func respRateLimited() func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", "0") // 立即可重试，避免拖慢测试
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"rate limited"}}`)
	}
}

func respSSE(content string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q}}]}\n\n", content)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

// e2eConfig 返回退避极小的弹性配置，使故障注入测试快速收敛。
func e2eConfig() domainllm.ResilienceConfig {
	cfg := domainllm.DefaultResilienceConfig()
	cfg.BaseBackoff = time.Microsecond
	cfg.MaxBackoff = time.Microsecond
	cfg.MaxAttempts = 3
	cfg.FailThreshold = 3 // 单次请求耗尽 3 次尝试即打开熔断
	return cfg
}

// modelCfg 指向某个假 server。
func modelCfg(srv *scriptedServer, name string) *domainllm.ModelConfig {
	return &domainllm.ModelConfig{
		Provider:  domainllm.ProviderOpenAI,
		ModelName: name,
		BaseURL:   srv.URL,
		APIKey:    "test-key",
	}
}

func chainFactory(t *testing.T) interface {
	CreateChatModelChain([]*domainllm.ModelConfig) (domainllm.LLMClient, error)
} {
	t.Helper()
	f := NewModelFactory(WithResilience(e2eConfig()))
	cf, ok := f.(interface {
		CreateChatModelChain([]*domainllm.ModelConfig) (domainllm.LLMClient, error)
	})
	if !ok {
		t.Fatal("factory must expose CreateChatModelChain")
	}
	return cf
}

// 7.1 故障注入：主目标 503 耗尽重试 → 降级后备成功；再次请求主目标熔断打开被跳过。
func TestE2E_RetryFallbackThenCircuit(t *testing.T) {
	primary := newScriptedServer(respStatus(http.StatusServiceUnavailable))
	defer primary.Close()
	backup := newScriptedServer(respOK("backup-ok"))
	defer backup.Close()

	client, err := chainFactory(t).CreateChatModelChain([]*domainllm.ModelConfig{
		modelCfg(primary, "rfc-primary"), modelCfg(backup, "rfc-backup"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 请求 1：主 503×3 → 降级 backup
	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("req1 err: %v", err)
	}
	if resp.Content != "backup-ok" {
		t.Fatalf("req1 content=%q want backup-ok", resp.Content)
	}
	if primary.Hits() != 3 {
		t.Fatalf("primary should be tried 3×, got %d", primary.Hits())
	}
	hitsAfter1 := primary.Hits()

	// 请求 2：主目标熔断已打开 → 跳过，不再命中主 server
	resp, err = client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("req2 err: %v", err)
	}
	if resp.Content != "backup-ok" {
		t.Fatalf("req2 content=%q want backup-ok", resp.Content)
	}
	if primary.Hits() != hitsAfter1 {
		t.Fatalf("open circuit must skip primary: before=%d after=%d", hitsAfter1, primary.Hits())
	}
	if backup.Hits() != 2 {
		t.Fatalf("backup should serve both requests, got %d", backup.Hits())
	}
}

// 7.1 故障注入：429（带 Retry-After）视为可重试，同目标重试后成功，不降级。
func TestE2E_RateLimitedRetrySameTarget(t *testing.T) {
	primary := newScriptedServer(respRateLimited(), respOK("recovered"))
	defer primary.Close()
	backup := newScriptedServer(respOK("backup"))
	defer backup.Close()

	client, err := chainFactory(t).CreateChatModelChain([]*domainllm.ModelConfig{
		modelCfg(primary, "rl-primary"), modelCfg(backup, "rl-backup"),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Content != "recovered" {
		t.Fatalf("content=%q want recovered (same-target retry)", resp.Content)
	}
	if primary.Hits() != 2 {
		t.Fatalf("primary should retry once (2 hits), got %d", primary.Hits())
	}
	if backup.Hits() != 0 {
		t.Fatalf("retryable error must not fall back, backup hit %d", backup.Hits())
	}
}

// 7.1 故障注入：连接重置（关闭的 server）→ 传输错归 transient → 重试耗尽后降级。
func TestE2E_ConnectionResetFallsBack(t *testing.T) {
	dead := newScriptedServer(respOK("never"))
	deadURL := dead.URL
	dead.Close() // 立即关闭 → 连接被拒
	backup := newScriptedServer(respOK("backup-ok"))
	defer backup.Close()

	client, err := chainFactory(t).CreateChatModelChain([]*domainllm.ModelConfig{
		{Provider: domainllm.ProviderOpenAI, ModelName: "cr-primary", BaseURL: deadURL, APIKey: "k"},
		modelCfg(backup, "cr-backup"),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Content != "backup-ok" {
		t.Fatalf("content=%q want backup-ok (transport error → fallback)", resp.Content)
	}
	if backup.Hits() != 1 {
		t.Fatalf("backup should serve once, got %d", backup.Hits())
	}
}

// 7.2 流式端到端：首 chunk 前主目标构建失败 → 转移到后备并成功流式。
func TestE2E_StreamBuildFailover(t *testing.T) {
	primary := newScriptedServer(respStatus(http.StatusServiceUnavailable))
	defer primary.Close()
	backup := newScriptedServer(respSSE("hello-stream"))
	defer backup.Close()

	client, err := chainFactory(t).CreateChatModelChain([]*domainllm.ModelConfig{
		modelCfg(primary, "sf-primary"), modelCfg(backup, "sf-backup"),
	})
	if err != nil {
		t.Fatal(err)
	}

	ch, err := client.ChatStream(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("stream build err: %v", err)
	}
	var got string
	for chunk := range ch {
		got += chunk.Content
	}
	if got != "hello-stream" {
		t.Fatalf("stream content=%q want hello-stream", got)
	}
	if backup.Hits() != 1 {
		t.Fatalf("backup stream should build once, got %d", backup.Hits())
	}
}

// 7.3 透明性回归：健康主目标成功路径完全不受装饰影响（内容/用量逐字节透传）。
func TestE2E_TransparentSuccessPath(t *testing.T) {
	primary := newScriptedServer(respOK("healthy"))
	defer primary.Close()

	client, err := chainFactory(t).CreateChatModelChain([]*domainllm.ModelConfig{
		modelCfg(primary, "tsp-primary"),
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Content != "healthy" {
		t.Fatalf("content=%q want healthy", resp.Content)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 2 {
		t.Fatalf("usage passthrough failed: %+v", resp.Usage)
	}
	if primary.Hits() != 1 {
		t.Fatalf("healthy path should hit once, got %d", primary.Hits())
	}
}

// 7.4 ctx 取消端到端：调用方预先取消 → 立即终止，不产生任何 provider 请求。
func TestE2E_CtxCancelNoRequests(t *testing.T) {
	primary := newScriptedServer(respStatus(http.StatusServiceUnavailable))
	defer primary.Close()
	backup := newScriptedServer(respOK("backup"))
	defer backup.Close()

	client, err := chainFactory(t).CreateChatModelChain([]*domainllm.ModelConfig{
		modelCfg(primary, "cc-primary"), modelCfg(backup, "cc-backup"),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Chat(ctx, &domainllm.ChatRequest{}); err == nil {
		t.Fatal("expected canceled error")
	}
	if primary.Hits() != 0 || backup.Hits() != 0 {
		t.Fatalf("canceled ctx must not issue requests: primary=%d backup=%d", primary.Hits(), backup.Hits())
	}
}
