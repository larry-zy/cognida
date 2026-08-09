package resilience

import (
	"context"
	"testing"
	"time"

	domainllm "cognida/internal/model/llm"
)

func TestBackoff_WithinCeiling(t *testing.T) {
	cfg := domainllm.DefaultResilienceConfig() // Base=200ms, Max=5s
	for attempt := 0; attempt < 8; attempt++ {
		ceiling := cfg.BaseBackoff << uint(attempt)
		if ceiling <= 0 || ceiling > cfg.MaxBackoff {
			ceiling = cfg.MaxBackoff
		}
		for i := 0; i < 200; i++ {
			d := backoff(attempt, cfg, 0)
			if d < 0 || d > ceiling {
				t.Fatalf("attempt %d: backoff %v out of [0,%v]", attempt, d, ceiling)
			}
		}
	}
}

func TestBackoff_HonorsRetryAfter(t *testing.T) {
	cfg := domainllm.DefaultResilienceConfig()
	ra := 4 * time.Second
	// retryAfter 大于任何 jitter 时应至少返回 retryAfter
	for i := 0; i < 100; i++ {
		if d := backoff(0, cfg, ra); d < ra {
			t.Fatalf("backoff %v < retryAfter %v", d, ra)
		}
	}
}

func TestBackoff_CeilingCappedAtMax(t *testing.T) {
	cfg := domainllm.DefaultResilienceConfig()
	for i := 0; i < 500; i++ {
		if d := backoff(30, cfg, 0); d > cfg.MaxBackoff {
			t.Fatalf("attempt 30: backoff %v exceeds max %v", d, cfg.MaxBackoff)
		}
	}
}

func TestSleep_RespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleep(ctx, time.Hour); err == nil {
		t.Fatal("expected ctx error on canceled sleep")
	}
}

func TestSleep_ZeroDurationReturnsCtxErr(t *testing.T) {
	if err := sleep(context.Background(), 0); err != nil {
		t.Fatalf("zero sleep on live ctx: %v", err)
	}
}
