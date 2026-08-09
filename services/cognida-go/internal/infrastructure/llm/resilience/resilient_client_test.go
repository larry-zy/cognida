package resilience

import (
	"context"
	"testing"
	"time"

	domainllm "cognida/internal/model/llm"
)

// scriptedClient 按预设错误序列应答；nil 表示成功。序列耗尽后沿用最后一项。
type scriptedClient struct {
	name    string
	seq     []error
	calls   int
	streams int
}

func (c *scriptedClient) next() error {
	var e error
	if c.calls < len(c.seq) {
		e = c.seq[c.calls]
	} else if len(c.seq) > 0 {
		e = c.seq[len(c.seq)-1]
	}
	c.calls++
	return e
}

func (c *scriptedClient) Chat(_ context.Context, _ *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	if err := c.next(); err != nil {
		return nil, err
	}
	return &domainllm.ChatResponse{Content: c.name}, nil
}

func (c *scriptedClient) ChatStream(_ context.Context, _ *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	c.streams++
	if err := c.next(); err != nil {
		return nil, err
	}
	ch := make(chan *domainllm.ChatChunk, 1)
	ch <- &domainllm.ChatChunk{Content: c.name, Done: true}
	close(ch)
	return ch, nil
}

func (c *scriptedClient) GetModelInfo(context.Context) (*domainllm.ModelInfo, error) {
	return &domainllm.ModelInfo{Name: c.name}, nil
}
func (c *scriptedClient) SupportsTools() bool     { return true }
func (c *scriptedClient) SupportsStreaming() bool { return true }

func fastCfg() domainllm.ResilienceConfig {
	c := testCfg()
	c.BaseBackoff = time.Microsecond
	c.MaxBackoff = time.Microsecond
	return c
}

func transientErr() *domainllm.APIError {
	return &domainllm.APIError{Class: domainllm.ClassTransient, StatusCode: 503, Detail: "svc unavailable"}
}
func terminalErr() *domainllm.APIError {
	return &domainllm.APIError{Class: domainllm.ClassTerminal, StatusCode: 400, Detail: "bad request"}
}

func chatTarget(reg *breakerRegistry, name string, seq ...error) (*scriptedClient, ChatTarget) {
	c := &scriptedClient{name: name, seq: seq}
	return c, ChatTarget{Provider: domainllm.ProviderOpenAI, Model: name, Client: c}
}

func TestChat_RetryThenSuccess(t *testing.T) {
	reg := newRegistry()
	c, tgt := chatTarget(reg, "m1", transientErr(), nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, tgt)

	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Content != "m1" {
		t.Fatalf("content=%q", resp.Content)
	}
	if c.calls != 2 {
		t.Fatalf("expected 2 attempts (1 retry), got %d", c.calls)
	}
}

func TestChat_TerminalNoRetry(t *testing.T) {
	reg := newRegistry()
	c, tgt := chatTarget(reg, "m1", terminalErr(), nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, tgt)

	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err == nil {
		t.Fatal("expected terminal error")
	}
	if c.calls != 1 {
		t.Fatalf("terminal must not retry, got %d calls", c.calls)
	}
}

func TestChat_FallbackToNextTarget(t *testing.T) {
	reg := newRegistry()
	// 主目标全程 transient（耗尽重试），后备成功
	c1, t1 := chatTarget(reg, "primary", transientErr(), transientErr(), transientErr())
	c2, t2 := chatTarget(reg, "backup", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1, t2)

	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Content != "backup" {
		t.Fatalf("expected backup, got %q", resp.Content)
	}
	if c1.calls != 3 {
		t.Fatalf("primary should exhaust 3 attempts, got %d", c1.calls)
	}
	if c2.calls != 1 {
		t.Fatalf("backup should be called once, got %d", c2.calls)
	}
}

func TestChat_TerminalFallsBackImmediately(t *testing.T) {
	reg := newRegistry()
	c1, t1 := chatTarget(reg, "primary", terminalErr())
	_, t2 := chatTarget(reg, "backup", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1, t2)

	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Content != "backup" {
		t.Fatalf("expected backup, got %q", resp.Content)
	}
	if c1.calls != 1 {
		t.Fatalf("primary terminal should not retry, got %d", c1.calls)
	}
}

func TestChat_AllTargetsFailAggregate(t *testing.T) {
	reg := newRegistry()
	_, t1 := chatTarget(reg, "primary", terminalErr())
	_, t2 := chatTarget(reg, "backup", terminalErr())
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1, t2)

	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err == nil {
		t.Fatal("expected aggregate error")
	}
	ae, ok := domainllm.AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if ae.Class != domainllm.ClassTerminal {
		t.Fatalf("aggregate class=%s want terminal", ae.Class)
	}
}

func TestChat_CtxCancelNoFallback(t *testing.T) {
	reg := newRegistry()
	c1, t1 := chatTarget(reg, "primary", &domainllm.APIError{Class: domainllm.ClassCanceled, Detail: "canceled", Err: context.Canceled})
	c2, t2 := chatTarget(reg, "backup", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1, t2)

	_, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if c1.calls != 1 {
		t.Fatalf("canceled must not retry, got %d", c1.calls)
	}
	if c2.calls != 0 {
		t.Fatalf("canceled must not fall back, backup called %d", c2.calls)
	}
}

func TestChat_ParentCtxCanceledBeforeStart(t *testing.T) {
	reg := newRegistry()
	c1, t1 := chatTarget(reg, "primary", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Chat(ctx, &domainllm.ChatRequest{})
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if c1.calls != 0 {
		t.Fatalf("canceled parent ctx must not invoke client, got %d", c1.calls)
	}
}

func TestChatStream_BuildFailover(t *testing.T) {
	reg := newRegistry()
	c1, t1 := chatTarget(reg, "primary", terminalErr())
	c2, t2 := chatTarget(reg, "backup", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1, t2)

	ch, err := client.ChatStream(context.Background(), &domainllm.ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	chunk := <-ch
	if chunk.Content != "backup" {
		t.Fatalf("expected backup stream, got %q", chunk.Content)
	}
	if c1.streams != 1 || c2.streams != 1 {
		t.Fatalf("stream build failover mismatch: primary=%d backup=%d", c1.streams, c2.streams)
	}
}

func TestChat_TransparentPassthrough(t *testing.T) {
	reg := newRegistry()
	c, tgt := chatTarget(reg, "solo", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, tgt)

	resp, err := client.Chat(context.Background(), &domainllm.ChatRequest{})
	if err != nil || resp.Content != "solo" {
		t.Fatalf("passthrough failed: resp=%v err=%v", resp, err)
	}
	if c.calls != 1 {
		t.Fatalf("healthy call should invoke once, got %d", c.calls)
	}
	if !client.SupportsTools() || !client.SupportsStreaming() {
		t.Fatal("capability passthrough failed")
	}
}

func TestChat_CircuitOpensAndSkips(t *testing.T) {
	reg := newRegistry()
	// FailThreshold=3；主目标每请求耗尽 3 次 transient，一次请求即开熔断
	c1, t1 := chatTarget(reg, "primary", transientErr())
	c2, t2 := chatTarget(reg, "backup", nil)
	client := newResilientClientWith(reg, fastCfg(), NoopObserver{}, t1, t2)

	// 第一次请求：主目标 3 次尝试全失败 → 熔断打开 → 降级 backup
	if _, err := client.Chat(context.Background(), &domainllm.ChatRequest{}); err != nil {
		t.Fatalf("req1 err: %v", err)
	}
	primaryCallsAfter1 := c1.calls
	// 第二次请求：主目标熔断打开，应被跳过（不再新增调用），直接走 backup
	if _, err := client.Chat(context.Background(), &domainllm.ChatRequest{}); err != nil {
		t.Fatalf("req2 err: %v", err)
	}
	if c1.calls != primaryCallsAfter1 {
		t.Fatalf("open circuit must skip primary: before=%d after=%d", primaryCallsAfter1, c1.calls)
	}
	if c2.calls < 2 {
		t.Fatalf("backup should serve both requests, got %d", c2.calls)
	}
}
