package resilience

import (
	"context"
	"math/rand"
	"time"

	domainllm "cognida/internal/model/llm"
)

// backoff 计算第 attempt 次重试前的等待时长（attempt 从 0 计）。
// 采用指数退避 + full jitter：wait ∈ [0, min(maxBackoff, base*2^attempt)]。
// rate_limited 且携带 retryAfter 时，取 max(退避, retryAfter)（retryAfter 已封顶）。
func backoff(attempt int, cfg domainllm.ResilienceConfig, retryAfter time.Duration) time.Duration {
	ceiling := cfg.BaseBackoff << uint(attempt)
	if ceiling <= 0 || ceiling > cfg.MaxBackoff {
		ceiling = cfg.MaxBackoff
	}
	jittered := time.Duration(rand.Int63n(int64(ceiling) + 1))
	if retryAfter > jittered {
		return retryAfter
	}
	return jittered
}

// sleep 在等待 d 期间尊重 ctx 取消；ctx 先结束则返回其错误。
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
