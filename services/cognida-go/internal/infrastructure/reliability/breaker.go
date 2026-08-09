package reliability

import (
	"sync"
	"time"
)

// State 熔断器状态。
type State int

const (
	// StateClosed 正常放行，统计连续失败。
	StateClosed State = iota
	// StateOpen 快速失败，冷却期内拒绝所有请求。
	StateOpen
	// StateHalfOpen 冷却结束后放行有限探测，成功则闭合、失败则重新打开。
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Breaker 单个目标（服务地址）的三态熔断器。逻辑与 llm/resilience 那套同构，
// 但不耦合任何领域配置，可被 gRPC/HTTP 等任意通路复用。并发安全。
type Breaker struct {
	key string
	cfg Config
	now func() time.Time // 便于测试注入时钟

	mu               sync.Mutex
	state            State
	failures         int       // closed 态连续可计失败数
	openedAt         time.Time // 进入 open 的时刻，用于冷却判定
	halfOpenInFlight int       // half-open 态已放行、未回执的探测数
}

// Allow 判断当前是否放行一次请求，并按需推进状态（open→half-open 冷却到期）。
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if b.now().Sub(b.openedAt) >= b.cfg.Cooldown {
			b.state = StateHalfOpen
			b.halfOpenInFlight = 1
			return true
		}
		return false
	case StateHalfOpen:
		if b.halfOpenInFlight < b.cfg.HalfOpenProbes {
			b.halfOpenInFlight++
			return true
		}
		return false
	default:
		return true
	}
}

// OnResult 回执一次请求结果。success 为业务/传输是否成功；countable 表示该失败是否
// 应计入熔断（例如目标不可用/过载记，调用方取消不记）。成功恒重置。
func (b *Breaker) OnResult(success, countable bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if success {
		b.failures = 0
		b.halfOpenInFlight = 0
		b.state = StateClosed
		return
	}

	switch b.state {
	case StateHalfOpen:
		// 探测失败：立即重新打开并重置冷却计时。
		if b.halfOpenInFlight > 0 {
			b.halfOpenInFlight--
		}
		b.trip()
	case StateClosed:
		if !countable {
			return
		}
		b.failures++
		if b.failures >= b.cfg.FailThreshold {
			b.trip()
		}
	}
}

// trip 打开熔断，须在持锁下调用。
func (b *Breaker) trip() {
	b.state = StateOpen
	b.openedAt = b.now()
	b.halfOpenInFlight = 0
}

// State 返回当前状态（快照）。
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Registry 按 target key 管理熔断器集合。一个连接对应一个 target 的熔断器。
type Registry struct {
	cfg Config
	now func() time.Time

	mu       sync.Mutex
	breakers map[string]*Breaker
}

// NewRegistry 用给定配置创建注册表（配置经 WithDefaults 规整）。
func NewRegistry(cfg Config) *Registry {
	return &Registry{
		cfg:      cfg.WithDefaults(),
		now:      time.Now,
		breakers: make(map[string]*Breaker),
	}
}

// Get 返回 key 对应的熔断器，不存在则惰性创建。
func (r *Registry) Get(key string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.breakers[key]; ok {
		return b
	}
	b := &Breaker{key: key, cfg: r.cfg, now: r.now, state: StateClosed}
	r.breakers[key] = b
	return b
}
