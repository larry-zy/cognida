package resilience

import (
	"sync"
	"time"

	domainllm "link/internal/model/llm"
)

// breakerRegistry 按 target key（provider+model）托管进程内共享的熔断器单例。
// 每请求新建的装饰器实例通过它复用同一 target 的熔断状态，否则熔断永远失效。
type breakerRegistry struct {
	mu           sync.Mutex
	breakers     map[string]*breaker
	now          func() time.Time
	onTransition func(key string, from, to CircuitState)
}

// newRegistry 创建一个独立的熔断器注册表（测试可用其隔离状态）。
func newRegistry() *breakerRegistry {
	return &breakerRegistry{breakers: make(map[string]*breaker)}
}

// globalRegistry 进程内共享的默认注册表。
var globalRegistry = newRegistry()

// get 返回 key 对应的熔断器，不存在则按 cfg 创建。
// 已存在的熔断器沿用其既有状态与创建时的 cfg（首次创建者定调）。
func (r *breakerRegistry) get(key string, cfg domainllm.ResilienceConfig) *breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.breakers[key]; ok {
		return b
	}
	b := newBreaker(key, cfg, r.now)
	b.onTransition = r.onTransition
	r.breakers[key] = b
	return b
}
