package resilience

import (
	"context"
	"strings"
	"time"

	domainllm "cognida/internal/model/llm"
)

// ========================================
// 公开的目标类型（组合根装配 fallback 链时使用）
// ========================================

// ChatTarget 一个 chat 弹性目标（底层客户端 + 其 provider/model 标识）。
type ChatTarget struct {
	Provider domainllm.Provider
	Model    string
	Client   domainllm.LLMClient
}

// EmbeddingTarget 一个 embedding 弹性目标。
type EmbeddingTarget struct {
	Provider domainllm.Provider
	Model    string
	Client   domainllm.EmbeddingRepository
}

// RerankTarget 一个 rerank 弹性目标。
type RerankTarget struct {
	Provider domainllm.Provider
	Model    string
	Client   domainllm.RerankRepository
}

// ========================================
// 内部目标状态
// ========================================

// target[C] 绑定底层客户端与其共享熔断器。
type target[C any] struct {
	provider domainllm.Provider
	model    string
	key      string
	breaker  *breaker
	client   C
}

func newTarget[C any](reg *breakerRegistry, cfg domainllm.ResilienceConfig, provider domainllm.Provider, model string, client C) target[C] {
	key := domainllm.TargetKey(provider, model)
	return target[C]{
		provider: provider,
		model:    model,
		key:      key,
		breaker:  reg.get(key, cfg),
		client:   client,
	}
}

// ========================================
// 链式执行编排（重试 + 熔断 + 降级），三类客户端共用
// ========================================

// executeChain 逐目标尝试一次逻辑调用：先在单目标上重试，再跨目标降级。
//   - invoke 对给定目标的底层客户端执行一次实际调用。
//   - 返回首个成功结果；全链耗尽返回聚合终态错。
//   - 调用方 ctx 取消立即返回 canceled，MUST NOT 继续降级。
func executeChain[C, R any](
	ctx context.Context,
	targets []target[C],
	cfg domainllm.ResilienceConfig,
	obs Observer,
	requestID string,
	op string,
	invoke func(context.Context, C) (R, error),
) (R, error) {
	var zero R
	targetErrs := make([]error, 0, len(targets))

	for ti := range targets {
		t := &targets[ti]

		if err := ctx.Err(); err != nil {
			return zero, canceledError(t.provider, t.model, err)
		}

		if !t.breaker.Allow() {
			obs.OnCircuitReject(t.key)
			targetErrs = append(targetErrs, circuitOpenError(t.provider, t.model))
			if ti+1 < len(targets) {
				obs.OnFallback(t.key, targets[ti+1].key, requestID)
			}
			continue
		}

		res, err, class := attemptTarget(ctx, t, cfg, obs, op, invoke)
		if err == nil {
			return res, nil
		}
		if class == domainllm.ClassCanceled {
			return zero, err
		}
		targetErrs = append(targetErrs, err)
		if ti+1 < len(targets) {
			obs.OnFallback(t.key, targets[ti+1].key, requestID)
		}
	}

	// 单目标：直接返回其原始错误，保留 StatusCode 等信息。
	if len(targets) == 1 && len(targetErrs) == 1 {
		return zero, targetErrs[0]
	}
	return zero, aggregateError(targets, targetErrs)
}

// attemptTarget 在单个目标上按退避重试；返回结果、最终错误与其分级。
func attemptTarget[C, R any](
	ctx context.Context,
	t *target[C],
	cfg domainllm.ResilienceConfig,
	obs Observer,
	op string,
	invoke func(context.Context, C) (R, error),
) (R, error, domainllm.ErrorClass) {
	var zero R
	var lastErr error
	var lastClass domainllm.ErrorClass

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			// ctx 在 Allow() 放行后、发起请求前被取消：本次探测未真正执行，
			// 以非计入结果上报，释放 half-open 已占用的探测名额，避免熔断器永久卡在 half-open。
			t.breaker.OnResult(false, false)
			return zero, canceledError(t.provider, t.model, err), domainllm.ClassCanceled
		}

		res, err := invoke(ctx, t.client)
		if err == nil {
			t.breaker.OnResult(true, true)
			obs.OnAttempt(t.key, op, attempt, true)
			return res, nil, ""
		}

		class, retryAfter := classifyTargetErr(err)
		t.breaker.OnResult(false, class.Countable())
		obs.OnAttempt(t.key, op, attempt, false)
		lastErr, lastClass = err, class

		if class == domainllm.ClassCanceled {
			return zero, err, class
		}
		if !class.Retryable() {
			return zero, err, class // 终态：不重试，交由外层降级到下一目标
		}
		if attempt+1 >= cfg.MaxAttempts {
			break
		}
		obs.OnRetry(t.key, op, attempt+1)
		if serr := sleep(ctx, backoff(attempt, cfg, retryAfter)); serr != nil {
			return zero, canceledError(t.provider, t.model, serr), domainllm.ClassCanceled
		}
	}

	return zero, lastErr, lastClass
}

// ========================================
// 错误分级与聚合辅助
// ========================================

// classifyTargetErr 提取错误分级与 Retry-After。底层已是 *APIError 则沿用其判定。
func classifyTargetErr(err error) (domainllm.ErrorClass, time.Duration) {
	if ae, ok := domainllm.AsAPIError(err); ok {
		return ae.Class, ae.RetryAfter
	}
	return Classify(0, err), 0
}

// canceledError 构造 canceled 类 APIError。
func canceledError(provider domainllm.Provider, model string, err error) *domainllm.APIError {
	return &domainllm.APIError{
		Provider: provider,
		Model:    model,
		Class:    domainllm.ClassCanceled,
		Detail:   domainllm.SummarizeDetail(errMsg(err)),
		Err:      err,
	}
}

// circuitOpenError 构造熔断打开导致的跳过错误（用于聚合摘要）。
func circuitOpenError(provider domainllm.Provider, model string) *domainllm.APIError {
	return &domainllm.APIError{
		Provider: provider,
		Model:    model,
		Class:    domainllm.ClassTransient,
		Detail:   "circuit open, skipped",
	}
}

// aggregateError 全链耗尽时聚合各目标失败摘要为终态错。
func aggregateError[C any](targets []target[C], errs []error) *domainllm.APIError {
	var provider domainllm.Provider
	var model string
	if len(targets) > 0 {
		provider = targets[0].provider
		model = targets[0].model
	}
	parts := make([]string, 0, len(errs))
	for i, e := range errs {
		key := ""
		if i < len(targets) {
			key = targets[i].key
		}
		parts = append(parts, key+": "+errMsg(e))
	}
	return &domainllm.APIError{
		Provider: provider,
		Model:    model,
		Class:    domainllm.ClassTerminal,
		Detail:   domainllm.SummarizeDetail("all targets failed [" + strings.Join(parts, "; ") + "]"),
	}
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
