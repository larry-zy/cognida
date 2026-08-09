package llm

import (
	"os"
	"strconv"
	"time"

	domainllm "cognida/internal/model/llm"
)

// ResilienceConfigFromEnv 在默认弹性配置基础上应用环境变量覆盖。
// 未设置的变量保持默认值，便于运维按需调参而不改代码。
//
// 支持的变量：
//   - LLM_RESILIENCE_ENABLED         (bool)
//   - LLM_RESILIENCE_MAX_ATTEMPTS    (int)
//   - LLM_RESILIENCE_BASE_BACKOFF    (duration, 如 200ms)
//   - LLM_RESILIENCE_MAX_BACKOFF     (duration, 如 5s)
//   - LLM_RESILIENCE_RETRY_AFTER_CAP (duration)
//   - LLM_RESILIENCE_FAIL_THRESHOLD  (int)
//   - LLM_RESILIENCE_COOLDOWN        (duration)
//   - LLM_RESILIENCE_HALF_OPEN_PROBES(int)
func ResilienceConfigFromEnv() domainllm.ResilienceConfig {
	cfg := domainllm.DefaultResilienceConfig()

	if v, ok := os.LookupEnv("LLM_RESILIENCE_ENABLED"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = b
		}
	}
	if v := envInt("LLM_RESILIENCE_MAX_ATTEMPTS"); v > 0 {
		cfg.MaxAttempts = v
	}
	if v := envDuration("LLM_RESILIENCE_BASE_BACKOFF"); v > 0 {
		cfg.BaseBackoff = v
	}
	if v := envDuration("LLM_RESILIENCE_MAX_BACKOFF"); v > 0 {
		cfg.MaxBackoff = v
	}
	if v := envDuration("LLM_RESILIENCE_RETRY_AFTER_CAP"); v > 0 {
		cfg.RetryAfterCap = v
	}
	if v := envInt("LLM_RESILIENCE_FAIL_THRESHOLD"); v > 0 {
		cfg.FailThreshold = v
	}
	if v := envDuration("LLM_RESILIENCE_COOLDOWN"); v > 0 {
		cfg.Cooldown = v
	}
	if v := envInt("LLM_RESILIENCE_HALF_OPEN_PROBES"); v > 0 {
		cfg.HalfOpenProbes = v
	}

	return cfg.WithDefaults()
}

func envInt(key string) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}

func envDuration(key string) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 0
}
