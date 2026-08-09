// Package reliability 提供与传输通路无关的统一可靠性策略：请求级重试（退避）、
// per-target 三态熔断。它把原先散落在各通路（gRPC docreader/quality、HTTP 评测、MCP）
// 各写一套或干脆缺失的重试/熔断收敛为单一实现（〔X-4〕）。
//
// 分层：本包属 infrastructure，仅依赖 grpc-go 与标准库，不反向依赖任何 service/model，
// 故 gRPC 基础客户端、HTTP 客户端均可复用而不产生环。LLM 领域另有一套按 provider/model
// 粒度的弹性装饰器（internal/infrastructure/llm/resilience，含降级链），二者策略同构、
// 各自服务不同粒度：本包按「服务目标」，LLM 那套按「模型目标」。
package reliability

import "time"

// Config 统一可靠性配置。零值不可用，须经 WithDefaults 或 DefaultConfig 取得安全默认。
type Config struct {
	// Enabled 关闭后重试与熔断均退化为直连（不加拦截器/服务配置）。
	Enabled bool
	// MaxAttempts 单次调用最大尝试次数（含首次）。用于 gRPC retryPolicy，
	// gRPC 服务配置要求取值 ∈ [2,5]，WithDefaults 会夹取到该区间。
	MaxAttempts int
	// BaseBackoff 重试退避基准（gRPC retryPolicy 的 InitialBackoff）。
	BaseBackoff time.Duration
	// MaxBackoff 单次退避封顶。
	MaxBackoff time.Duration
	// FailThreshold 熔断器 closed 态连续可计失败达到该值即 open。
	FailThreshold int
	// Cooldown 熔断 open 后进入 half-open 前的冷却窗口。
	Cooldown time.Duration
	// HalfOpenProbes half-open 态放行的探测请求数。
	HalfOpenProbes int
}

// DefaultConfig 返回带安全默认值的统一可靠性配置。
func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		MaxAttempts:    3,
		BaseBackoff:    200 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		FailThreshold:  5,
		Cooldown:       30 * time.Second,
		HalfOpenProbes: 1,
	}
}

// WithDefaults 用默认值填充未设置（非法/零值）的字段，并把 MaxAttempts 夹取到 gRPC
// 服务配置允许的 [2,5]。Enabled 不在此填充（其零值 false 是显式意图，由构造方决定）。
func (c Config) WithDefaults() Config {
	d := DefaultConfig()
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = d.MaxAttempts
	}
	if c.MaxAttempts < 2 {
		c.MaxAttempts = 2
	}
	if c.MaxAttempts > 5 {
		c.MaxAttempts = 5
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = d.BaseBackoff
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = d.MaxBackoff
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
	return c
}
