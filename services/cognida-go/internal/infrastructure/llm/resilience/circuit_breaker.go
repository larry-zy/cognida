package resilience

import (
	"sync"
	"time"

	domainllm "cognida/internal/model/llm"
)

// CircuitState 熔断器状态。
type CircuitState int

const (
	// StateClosed 正常放行。
	StateClosed CircuitState = iota
	// StateOpen 熔断打开，冷却窗口内快速失败（不发请求）。
	StateOpen
	// StateHalfOpen 半开，仅放行有限探测。
	StateHalfOpen
)

func (s CircuitState) String() string {
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

// breaker per-target 三态熔断器，并发安全。
type breaker struct {
	key string
	cfg domainllm.ResilienceConfig
	now func() time.Time // 可注入的时钟，便于测试

	mu               sync.Mutex
	state            CircuitState
	failures         int
	openedAt         time.Time
	halfOpenInFlight int

	// onTransition 状态跃迁回调（可选，用于可观测性）。
	onTransition func(key string, from, to CircuitState)
}

func newBreaker(key string, cfg domainllm.ResilienceConfig, now func() time.Time) *breaker {
	if now == nil {
		now = time.Now
	}
	return &breaker{key: key, cfg: cfg, now: now, state: StateClosed}
}

// Allow 报告当前是否允许向该目标发起请求，并在必要时推进状态（open→half-open）。
// half-open 下仅放行 HalfOpenProbes 个探测。
func (b *breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if b.now().Sub(b.openedAt) >= b.cfg.Cooldown {
			b.transition(StateHalfOpen)
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

// OnResult 上报一次调用结果。
//   - success：调用成功
//   - countable：该失败是否计入熔断（terminal/canceled 不计入）
func (b *breaker) OnResult(success bool, countable bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateHalfOpen:
		switch {
		case success:
			b.failures = 0
			b.halfOpenInFlight = 0
			b.transition(StateClosed)
		case countable:
			b.halfOpenInFlight = 0
			b.openedAt = b.now()
			b.transition(StateOpen)
		default:
			// terminal/canceled 探测：不判定目标健康度，释放探测名额，保持 half-open。
			if b.halfOpenInFlight > 0 {
				b.halfOpenInFlight--
			}
		}
	case StateClosed:
		switch {
		case success:
			b.failures = 0
		case countable:
			b.failures++
			if b.failures >= b.cfg.FailThreshold {
				b.openedAt = b.now()
				b.transition(StateOpen)
			}
		}
	case StateOpen:
		// Allow 已拦截，正常不会到此；成功（如 half-open 竞态）则闭合。
		if success {
			b.failures = 0
			b.transition(StateClosed)
		}
	}
}

// State 返回当前状态（只读快照）。
func (b *breaker) State() CircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// transition 切换状态并触发回调（调用方须持有锁）。
func (b *breaker) transition(to CircuitState) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	if b.onTransition != nil {
		// 回调在持锁下同步触发；实现应保持轻量、无阻塞。
		b.onTransition(b.key, from, to)
	}
}
