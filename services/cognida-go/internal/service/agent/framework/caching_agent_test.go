package framework

import (
	"context"
	"errors"
	"sync"
	"testing"

	domainagent "cognida/internal/model/agent"
	domaincache "cognida/internal/model/cache"
)

// fakeAgent 是可控内层 Agent：记录被调用次数，返回预置应答/流。
type fakeAgent struct {
	name      string
	reply     string
	chatCalls int
	streamHit int
	err       error
}

func (f *fakeAgent) Name() string { return f.name }

func (f *fakeAgent) Chat(ctx context.Context, message string) (*Response, error) {
	f.chatCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &Response{Content: f.reply}, nil
}

func (f *fakeAgent) Stream(ctx context.Context, message string) (<-chan *Chunk, error) {
	f.streamHit++
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan *Chunk, 2)
	ch <- &Chunk{Content: f.reply}
	ch <- &Chunk{Done: true}
	close(ch)
	return ch, nil
}

// fakeCache 是可控 ResponseCache：记录 Set 次数，可预置 Get 命中。
type fakeCache struct {
	enabled  bool
	hit      *domaincache.CacheResponse
	mu       sync.Mutex
	setCalls int
	setResp  string
	getCalls int
}

func (c *fakeCache) IsEnabled(agentType string) bool { return c.enabled }

func (c *fakeCache) Get(ctx context.Context, req *domaincache.CacheRequest) (*domaincache.CacheResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.hit != nil {
		return c.hit, nil
	}
	return &domaincache.CacheResponse{FromCache: false}, nil
}

func (c *fakeCache) Set(ctx context.Context, req *domaincache.CacheRequest, resp string, tokensUsed int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCalls++
	c.setResp = resp
	return nil
}

func tenantCtx() context.Context {
	return domainagent.WithTenantID(context.Background(), 1)
}

// 未启用该逻辑键时，装饰器恒等透传（返回内层本身），零开销零回归。
func TestNewCachingAgent_DisabledIsPassthrough(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "hi"}
	cache := &fakeCache{enabled: false}
	got := NewCachingAgent(inner, cache, "rag_assistant")
	if got != Agent(inner) {
		t.Fatalf("未启用时应返回内层 Agent 本身（恒等透传），got %T", got)
	}
	gotWT := NewWriteThroughAgent(inner, cache, "data_agent")
	if gotWT != Agent(inner) {
		t.Fatalf("未启用时软写穿也应恒等透传，got %T", gotWT)
	}
}

// nil 缓存 → 恒等透传。
func TestNewCachingAgent_NilCacheIsPassthrough(t *testing.T) {
	inner := &fakeAgent{name: "inner"}
	if got := NewCachingAgent(inner, nil, "rag_assistant"); got != Agent(inner) {
		t.Fatalf("nil 缓存应恒等透传")
	}
	if got := NewWriteThroughAgent(inner, nil, "data_agent"); got != Agent(inner) {
		t.Fatalf("nil 缓存软写穿应恒等透传")
	}
}

// 硬模式命中：短路返回旧答案，绝不触达内层 LLM。
func TestCachingAgent_HardHitShortCircuits(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "fresh"}
	cache := &fakeCache{
		enabled: true,
		hit:     &domaincache.CacheResponse{FromCache: true, Response: "cached", Similarity: 0.99, CacheID: "c1"},
	}
	agent := NewCachingAgent(inner, cache, "rag_assistant")

	resp, err := agent.Chat(tenantCtx(), "q")
	if err != nil {
		t.Fatalf("Chat 出错: %v", err)
	}
	if resp.Content != "cached" {
		t.Fatalf("硬命中应返回缓存答案，got %q", resp.Content)
	}
	if inner.chatCalls != 0 {
		t.Fatalf("硬命中不应触达内层 LLM，chatCalls=%d", inner.chatCalls)
	}
	if fc, ok := resp.Metadata["from_cache"].(bool); !ok || !fc {
		t.Fatalf("硬命中应打 from_cache=true 元数据")
	}
}

// 硬模式未命中：委派内层并回写。
func TestCachingAgent_HardMissDelegatesAndWrites(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "fresh"}
	cache := &fakeCache{enabled: true} // hit=nil → FromCache=false
	agent := NewCachingAgent(inner, cache, "rag_assistant")

	resp, err := agent.Chat(tenantCtx(), "q")
	if err != nil {
		t.Fatalf("Chat 出错: %v", err)
	}
	if resp.Content != "fresh" {
		t.Fatalf("未命中应返回内层答案，got %q", resp.Content)
	}
	if inner.chatCalls != 1 {
		t.Fatalf("未命中应委派内层一次，chatCalls=%d", inner.chatCalls)
	}
	if cache.setCalls != 1 || cache.setResp != "fresh" {
		t.Fatalf("未命中应回写内层答案，setCalls=%d resp=%q", cache.setCalls, cache.setResp)
	}
}

// 软写穿模式：即便缓存里有高相似历史，也**绝不短路**——始终走内层重算，并回写。
func TestWriteThroughAgent_NeverShortCircuits(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "fresh"}
	cache := &fakeCache{
		enabled: true,
		hit:     &domaincache.CacheResponse{FromCache: true, Response: "cached", Similarity: 0.99},
	}
	agent := NewWriteThroughAgent(inner, cache, "data_agent")

	resp, err := agent.Chat(tenantCtx(), "q")
	if err != nil {
		t.Fatalf("Chat 出错: %v", err)
	}
	if resp.Content != "fresh" {
		t.Fatalf("软写穿绝不短路，应返回内层实时答案，got %q", resp.Content)
	}
	if inner.chatCalls != 1 {
		t.Fatalf("软写穿应始终委派内层，chatCalls=%d", inner.chatCalls)
	}
	if cache.getCalls != 0 {
		t.Fatalf("软写穿不应查询短路（Get），getCalls=%d", cache.getCalls)
	}
	if cache.setCalls != 1 || cache.setResp != "fresh" {
		t.Fatalf("软写穿应回写内层答案，setCalls=%d resp=%q", cache.setCalls, cache.setResp)
	}
}

// 缺租户（tenant<=0）时跳过缓存直接走内层，绝不落 tenant=0 缓存。
func TestCachingAgent_NoTenantSkipsCache(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "fresh"}
	cache := &fakeCache{enabled: true, hit: &domaincache.CacheResponse{FromCache: true, Response: "cached"}}
	agent := NewCachingAgent(inner, cache, "rag_assistant")

	resp, err := agent.Chat(context.Background(), "q") // 无租户
	if err != nil {
		t.Fatalf("Chat 出错: %v", err)
	}
	if resp.Content != "fresh" {
		t.Fatalf("缺租户应绕过缓存走内层，got %q", resp.Content)
	}
	if cache.getCalls != 0 || cache.setCalls != 0 {
		t.Fatalf("缺租户不应读写缓存，get=%d set=%d", cache.getCalls, cache.setCalls)
	}
}

// 硬模式 Stream 命中：合成流返回缓存答案，不触达内层 Stream。
func TestCachingAgent_HardHitStream(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "fresh"}
	cache := &fakeCache{enabled: true, hit: &domaincache.CacheResponse{FromCache: true, Response: "cached", CacheID: "c1"}}
	agent := NewCachingAgent(inner, cache, "rag_assistant")

	ch, err := agent.Stream(tenantCtx(), "q")
	if err != nil {
		t.Fatalf("Stream 出错: %v", err)
	}
	var content string
	for c := range ch {
		content += c.Content
	}
	if content != "cached" {
		t.Fatalf("硬命中流应返回缓存答案，got %q", content)
	}
	if inner.streamHit != 0 {
		t.Fatalf("硬命中流不应触达内层 Stream，streamHit=%d", inner.streamHit)
	}
}

// 软写穿 Stream：透传内层流并在结束后用累积全文回写。
func TestWriteThroughAgent_StreamTeesAndWrites(t *testing.T) {
	inner := &fakeAgent{name: "inner", reply: "fresh"}
	cache := &fakeCache{enabled: true}
	agent := NewWriteThroughAgent(inner, cache, "data_agent")

	ch, err := agent.Stream(tenantCtx(), "q")
	if err != nil {
		t.Fatalf("Stream 出错: %v", err)
	}
	var content string
	for c := range ch {
		content += c.Content
	}
	if content != "fresh" {
		t.Fatalf("软写穿流应透传内层内容，got %q", content)
	}
	if inner.streamHit != 1 {
		t.Fatalf("软写穿应委派内层 Stream，streamHit=%d", inner.streamHit)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.setCalls != 1 || cache.setResp != "fresh" {
		t.Fatalf("软写穿流结束后应回写累积全文，setCalls=%d resp=%q", cache.setCalls, cache.setResp)
	}
}

// 内层出错时原样透传错误，不回写。
func TestCachingAgent_InnerErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom")
	inner := &fakeAgent{name: "inner", err: sentinel}
	cache := &fakeCache{enabled: true}
	agent := NewCachingAgent(inner, cache, "rag_assistant")

	_, err := agent.Chat(tenantCtx(), "q")
	if !errors.Is(err, sentinel) {
		t.Fatalf("内层错误应原样透传，got %v", err)
	}
	if cache.setCalls != 0 {
		t.Fatalf("内层出错不应回写，setCalls=%d", cache.setCalls)
	}
}
