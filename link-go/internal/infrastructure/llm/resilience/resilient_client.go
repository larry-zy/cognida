package resilience

import (
	"context"

	agentctx "link/internal/model/agent"
	domainllm "link/internal/model/llm"
)

// resilientClient 是 domainllm.LLMClient 的弹性装饰器：
// 对首个健康目标做请求级退避重试，失败后按有序链降级到后备目标，
// 每目标经共享熔断器保护。对上层调用方透明。
type resilientClient struct {
	targets []target[domainllm.LLMClient]
	cfg     domainllm.ResilienceConfig
	obs     Observer
	// primary 用于 GetModelInfo/SupportsTools 等元信息查询（取链首）。
	primary domainllm.LLMClient
}

// NewResilientClient 用全局熔断注册表装配一条 chat 弹性链（targets[0] 为主，其余为后备）。
func NewResilientClient(cfg domainllm.ResilienceConfig, obs Observer, targets ...ChatTarget) domainllm.LLMClient {
	return newResilientClientWith(globalRegistry, cfg, obs, targets...)
}

func newResilientClientWith(reg *breakerRegistry, cfg domainllm.ResilienceConfig, obs Observer, targets ...ChatTarget) domainllm.LLMClient {
	cfg = cfg.WithDefaults()
	ts := make([]target[domainllm.LLMClient], 0, len(targets))
	for _, t := range targets {
		ts = append(ts, newTarget(reg, cfg, t.Provider, t.Model, t.Client))
	}
	var primary domainllm.LLMClient
	if len(targets) > 0 {
		primary = targets[0].Client
	}
	return &resilientClient{targets: ts, cfg: cfg, obs: resolveObserver(obs), primary: primary}
}

func (r *resilientClient) Chat(ctx context.Context, req *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	return executeChain(ctx, r.targets, r.cfg, r.obs, requestID(ctx), "chat",
		func(ctx context.Context, c domainllm.LLMClient) (*domainllm.ChatResponse, error) {
			return c.Chat(ctx, req)
		})
}

// ChatStream 仅在建流阶段（返回 channel 之前）做重试与降级。
// 一旦拿到 channel，首 chunk 及其后的失败经 channel 关闭透传，MUST NOT 重放。
func (r *resilientClient) ChatStream(ctx context.Context, req *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	return executeChain(ctx, r.targets, r.cfg, r.obs, requestID(ctx), "chat_stream",
		func(ctx context.Context, c domainllm.LLMClient) (<-chan *domainllm.ChatChunk, error) {
			return c.ChatStream(ctx, req)
		})
}

func (r *resilientClient) GetModelInfo(ctx context.Context) (*domainllm.ModelInfo, error) {
	if r.primary == nil {
		return nil, &domainllm.APIError{Class: domainllm.ClassTerminal, Detail: "no target configured"}
	}
	return r.primary.GetModelInfo(ctx)
}

func (r *resilientClient) SupportsTools() bool {
	return r.primary != nil && r.primary.SupportsTools()
}

func (r *resilientClient) SupportsStreaming() bool {
	return r.primary != nil && r.primary.SupportsStreaming()
}

// requestID 从 context 提取全链路 request_id（缺失返回空串）。
func requestID(ctx context.Context) string {
	if id, ok := agentctx.GetRequestID(ctx); ok {
		return id
	}
	return ""
}
