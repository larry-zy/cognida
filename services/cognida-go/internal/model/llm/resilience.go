// Package llm 提供 LLM 领域的弹性配置结构。
package llm

import "time"

// ResilienceConfig LLM 调用弹性策略配置。
type ResilienceConfig struct {
	// Enabled 是否启用弹性；false 时装饰器退化为直连底层客户端。
	Enabled bool
	// MaxAttempts 单目标最大尝试次数（含首次）。
	MaxAttempts int
	// BaseBackoff 退避基准时长。
	BaseBackoff time.Duration
	// MaxBackoff 单次退避封顶。
	MaxBackoff time.Duration
	// RetryAfterCap Retry-After 等待封顶，防止超长/恶意等待。
	RetryAfterCap time.Duration
	// FailThreshold 熔断器连续失败阈值。
	FailThreshold int
	// Cooldown 熔断 open 后的冷却窗口。
	Cooldown time.Duration
	// HalfOpenProbes half-open 状态放行的探测请求数。
	HalfOpenProbes int
	// PerCallTimeout 单次一元底层调用（单个 attempt）的超时上限。
	// 仅作用于一元调用（Chat/Embed/Rerank）——为每个 attempt 派生带 deadline 的子 ctx，
	// 防止上游 accept 后不回包导致请求永久挂起（SharedTransport 只保连接阶段超时）。
	// 流式调用（ChatStream）的 channel 生命周期长于单次 invoke，不适用此超时。
	PerCallTimeout time.Duration
}

// DefaultResilienceConfig 返回带安全默认值的弹性配置。
func DefaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		Enabled:        true,
		MaxAttempts:    3,
		BaseBackoff:    200 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		RetryAfterCap:  30 * time.Second,
		FailThreshold:  5,
		Cooldown:       30 * time.Second,
		HalfOpenProbes: 1,
		PerCallTimeout: 120 * time.Second,
	}
}

// WithDefaults 用默认值填充未设置（零值）的字段，返回补全后的配置。
// Enabled 字段不在此填充（零值 false 是显式意图，由构造方决定）。
func (c ResilienceConfig) WithDefaults() ResilienceConfig {
	d := DefaultResilienceConfig()
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = d.MaxAttempts
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = d.BaseBackoff
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = d.MaxBackoff
	}
	if c.RetryAfterCap <= 0 {
		c.RetryAfterCap = d.RetryAfterCap
	}
	if c.FailThreshold <= 0 {
		c.FailThreshold = d.FailThreshold
	}
	if c.Cooldown <= 0 {
		c.Cooldown = d.Cooldown
	}
	if c.HalfOpenProbes <= 0 {
		c.HalfOpenProbes = d.HalfOpenProbes
	}
	if c.PerCallTimeout <= 0 {
		c.PerCallTimeout = d.PerCallTimeout
	}
	return c
}

// FallbackChain 有序的模型降级链（主目标在首位）。
type FallbackChain struct {
	// Targets 有序目标列表，Targets[0] 为主目标。
	Targets []*ModelConfig
}

// TargetKey 返回目标的唯一键（provider+model），用于熔断器按目标共享。
func TargetKey(provider Provider, model string) string {
	return string(provider) + "/" + model
}
