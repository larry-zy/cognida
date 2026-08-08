package framework

import (
	"context"
	"log"
	"strings"

	domainagent "link/internal/model/agent"
	domaincache "link/internal/model/cache"
)

// ResponseCache 是硬语义缓存对 Agent 装饰器暴露的窄接口（由 infrastructure/cache.SemanticCacheService 实现）。
// 只含短路所需的三方法，让装饰器不依赖缓存服务的软召回/失效侧。
type ResponseCache interface {
	// IsEnabled 判断该逻辑 agent 键是否启用硬缓存。
	IsEnabled(agentType string) bool
	// Get 命中高相似历史答案时返回 FromCache=true（短路返回，跳过 LLM）。
	Get(ctx context.Context, req *domaincache.CacheRequest) (*domaincache.CacheResponse, error)
	// Set 应答后异步写入缓存（含 KBScope，供按库失效）。
	Set(ctx context.Context, req *domaincache.CacheRequest, resp string, tokensUsed int) error
}

// cacheMode 区分两种消费模式，二者共用同一向量检索/回写底座：
type cacheMode int

const (
	// modeHard 硬语义缓存：命中高相似历史答案即**短路返回、跳过 LLM**（RAG 知识库助手——知识稳定，安全）。
	modeHard cacheMode = iota
	// modeSoftWrite 软写穿：**从不短路**，只把每次应答回写缓存，供后续会话作 few-shot 召回
	//（data agent——数据会变，读侧由 semanticFewShotHook 注入参考、由 LLM 结合实时数据重算）。
	modeSoftWrite
)

// cachingAgent 是语义缓存装饰器：在内层 Agent 外包一层向量语义缓存。
//   - 硬模式（modeHard）：命中即直接返回旧答案跳过 LLM；未命中委派内层并回写。用于 RAG 助手。
//   - 软写穿（modeSoftWrite）：从不短路，只回写每次应答，让 data agent 的 few-shot 读侧有历史可召回。
//
// 装饰器而非 BeforeHook：短路返回需要接管整个 Chat/Stream 出口，Before 只能改写入参、无法拦截返回。
type cachingAgent struct {
	inner    Agent
	cache    ResponseCache
	agentKey string    // 逻辑缓存键（如 "rag_assistant"/"data_agent"），隔离命名空间与开关
	mode     cacheMode // 硬短路 vs 软写穿
}

// NewCachingAgent 用硬语义缓存装饰内层 Agent（命中短路返回）。
// cache 为 nil 或未启用该 agentKey 时，返回内层 Agent 本身（零开销、零回归恒等透传）。
func NewCachingAgent(inner Agent, cache ResponseCache, agentKey string) Agent {
	if inner == nil {
		return inner
	}
	if cache == nil || !cache.IsEnabled(agentKey) {
		return inner
	}
	return &cachingAgent{inner: inner, cache: cache, agentKey: agentKey, mode: modeHard}
}

// NewWriteThroughAgent 用软写穿装饰内层 Agent：从不短路，只把每次应答回写缓存，
// 供 few-shot 读侧（semanticFewShotHook）召回。cache 为 nil 或未启用该 agentKey 时恒等透传。
func NewWriteThroughAgent(inner Agent, cache ResponseCache, agentKey string) Agent {
	if inner == nil {
		return inner
	}
	if cache == nil || !cache.IsEnabled(agentKey) {
		return inner
	}
	return &cachingAgent{inner: inner, cache: cache, agentKey: agentKey, mode: modeSoftWrite}
}

// Name 透传内层名称（对上层不可见地插入了缓存层）。
func (a *cachingAgent) Name() string { return a.inner.Name() }

// buildRequest 从 ctx + message 组装缓存请求（租户强制取自 ctx；KBScope 取选定知识库范围）。
// 返回 ok=false 表示缺租户等关键信息，调用方应跳过缓存直接走内层（绝不落 tenant=0 缓存）。
func (a *cachingAgent) buildRequest(ctx context.Context, message string) (*domaincache.CacheRequest, bool) {
	tenantID, ok := domainagent.GetTenantID(ctx)
	if !ok || tenantID <= 0 {
		return nil, false
	}
	query := UserQueryFromContext(ctx)
	if strings.TrimSpace(query) == "" {
		query = message
	}
	kbScope, _ := domainagent.GetAllowedKBIDs(ctx)
	return &domaincache.CacheRequest{
		Query:     query,
		TenantID:  tenantID,
		AgentType: a.agentKey,
		KBScope:   kbScope,
	}, true
}

// Chat 硬模式命中即返回旧答案跳过 LLM；软写穿模式跳过短路直接委派。两种模式都在应答后异步回写。
func (a *cachingAgent) Chat(ctx context.Context, message string) (*Response, error) {
	req, ok := a.buildRequest(ctx, message)
	if !ok {
		return a.inner.Chat(ctx, message)
	}

	if a.mode == modeHard {
		if cached, err := a.cache.Get(ctx, req); err == nil && cached != nil && cached.FromCache {
			log.Printf("[Cache] RAG hard-cache hit: agent=%s similarity=%.4f", a.agentKey, cached.Similarity)
			return &Response{
				Content: cached.Response,
				Metadata: map[string]interface{}{
					"from_cache":       true,
					"cache_similarity": cached.Similarity,
					"cache_id":         cached.CacheID,
				},
			}, nil
		}
	}

	resp, err := a.inner.Chat(ctx, message)
	if err != nil {
		return resp, err
	}
	if resp != nil && strings.TrimSpace(resp.Content) != "" {
		_ = a.cache.Set(ctx, req, resp.Content, 0) // 异步写入，忽略错误（不阻断应答）
	}
	return resp, nil
}

// Stream 硬模式命中即合成单块流返回；软写穿模式跳过短路。两种模式都在流结束后用累积全文回写。
func (a *cachingAgent) Stream(ctx context.Context, message string) (<-chan *Chunk, error) {
	req, ok := a.buildRequest(ctx, message)
	if !ok {
		return a.inner.Stream(ctx, message)
	}

	if a.mode == modeHard {
		if cached, err := a.cache.Get(ctx, req); err == nil && cached != nil && cached.FromCache {
			log.Printf("[Cache] RAG hard-cache hit (stream): agent=%s similarity=%.4f", a.agentKey, cached.Similarity)
			out := make(chan *Chunk, 2)
			out <- &Chunk{Content: cached.Response, Metadata: map[string]interface{}{
				"from_cache":       true,
				"cache_similarity": cached.Similarity,
				"cache_id":         cached.CacheID,
			}}
			out <- &Chunk{Done: true}
			close(out)
			return out, nil
		}
	}

	innerCh, err := a.inner.Stream(ctx, message)
	if err != nil {
		return innerCh, err
	}

	out := make(chan *Chunk)
	go func() {
		defer close(out)
		var full strings.Builder
		for chunk := range innerCh {
			if chunk != nil && chunk.Content != "" {
				full.WriteString(chunk.Content)
			}
			out <- chunk
		}
		// 流结束后回写（用累积全文）。写缓存脱离 ctx 取消，用后台 ctx 由缓存服务内部兜超时。
		if content := strings.TrimSpace(full.String()); content != "" {
			_ = a.cache.Set(context.WithoutCancel(ctx), req, content, 0)
		}
	}()
	return out, nil
}
