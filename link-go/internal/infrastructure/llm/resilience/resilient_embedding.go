package resilience

import (
	"context"

	domainllm "link/internal/model/llm"
)

// resilientEmbedding 是 domainllm.EmbeddingRepository 的弹性装饰器。
type resilientEmbedding struct {
	targets []target[domainllm.EmbeddingRepository]
	cfg     domainllm.ResilienceConfig
	obs     Observer
	primary domainllm.EmbeddingRepository
}

// NewResilientEmbedding 装配一条 embedding 弹性链（targets[0] 为主，其余为后备）。
func NewResilientEmbedding(cfg domainllm.ResilienceConfig, obs Observer, targets ...EmbeddingTarget) domainllm.EmbeddingRepository {
	return newResilientEmbeddingWith(globalRegistry, cfg, obs, targets...)
}

func newResilientEmbeddingWith(reg *breakerRegistry, cfg domainllm.ResilienceConfig, obs Observer, targets ...EmbeddingTarget) domainllm.EmbeddingRepository {
	cfg = cfg.WithDefaults()
	ts := make([]target[domainllm.EmbeddingRepository], 0, len(targets))
	for _, t := range targets {
		ts = append(ts, newTarget(reg, cfg, t.Provider, t.Model, t.Client))
	}
	var primary domainllm.EmbeddingRepository
	if len(targets) > 0 {
		primary = targets[0].Client
	}
	return &resilientEmbedding{targets: ts, cfg: cfg, obs: resolveObserver(obs), primary: primary}
}

func (r *resilientEmbedding) Embed(ctx context.Context, req *domainllm.EmbeddingRequest) (*domainllm.EmbeddingResponse, error) {
	return executeChain(ctx, r.targets, r.cfg, r.obs, requestID(ctx), "embed",
		func(ctx context.Context, c domainllm.EmbeddingRepository) (*domainllm.EmbeddingResponse, error) {
			return c.Embed(ctx, req)
		})
}

func (r *resilientEmbedding) EmbedBatch(ctx context.Context, req *domainllm.EmbeddingRequest) (*domainllm.EmbeddingResponse, error) {
	return executeChain(ctx, r.targets, r.cfg, r.obs, requestID(ctx), "embed_batch",
		func(ctx context.Context, c domainllm.EmbeddingRepository) (*domainllm.EmbeddingResponse, error) {
			return c.EmbedBatch(ctx, req)
		})
}

// GetDimension 走主目标（维度是配置属性，不涉及降级）。
func (r *resilientEmbedding) GetDimension(ctx context.Context) (int, error) {
	if r.primary == nil {
		return 0, &domainllm.APIError{Class: domainllm.ClassTerminal, Detail: "no target configured"}
	}
	return r.primary.GetDimension(ctx)
}
