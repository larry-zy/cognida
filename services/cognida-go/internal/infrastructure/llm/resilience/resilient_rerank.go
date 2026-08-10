package resilience

import (
	"context"

	domainllm "cognida/internal/model/llm"
)

// resilientRerank 是 domainllm.RerankRepository 的弹性装饰器。
type resilientRerank struct {
	targets []target[domainllm.RerankRepository]
	cfg     domainllm.ResilienceConfig
	obs     Observer
	primary domainllm.RerankRepository
}

// NewResilientRerank 装配一条 rerank 弹性链（targets[0] 为主，其余为后备）。
func NewResilientRerank(cfg domainllm.ResilienceConfig, obs Observer, targets ...RerankTarget) domainllm.RerankRepository {
	return newResilientRerankWith(globalRegistry, cfg, obs, targets...)
}

func newResilientRerankWith(reg *breakerRegistry, cfg domainllm.ResilienceConfig, obs Observer, targets ...RerankTarget) domainllm.RerankRepository {
	cfg = cfg.WithDefaults()
	ts := make([]target[domainllm.RerankRepository], 0, len(targets))
	for _, t := range targets {
		ts = append(ts, newTarget(reg, cfg, t.Provider, t.Model, t.Client))
	}
	var primary domainllm.RerankRepository
	if len(targets) > 0 {
		primary = targets[0].Client
	}
	return &resilientRerank{targets: ts, cfg: cfg, obs: resolveObserver(obs), primary: primary}
}

func (r *resilientRerank) Rerank(ctx context.Context, req *domainllm.RerankRequest) (*domainllm.RerankResponse, error) {
	return executeChain(ctx, r.targets, r.cfg, r.obs, requestID(ctx), "rerank", true,
		func(ctx context.Context, c domainllm.RerankRepository) (*domainllm.RerankResponse, error) {
			return c.Rerank(ctx, req)
		})
}

// GetMaxDocs 走主目标（能力属性，不涉及降级）。
func (r *resilientRerank) GetMaxDocs() int {
	if r.primary == nil {
		return 0
	}
	return r.primary.GetMaxDocs()
}
