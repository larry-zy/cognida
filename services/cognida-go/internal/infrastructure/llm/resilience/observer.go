package resilience

// Observer 弹性事件的观测钩子。用于指标计数与结构化日志。
// 所有方法 MUST 保持轻量、非阻塞（部分在持锁上下文中同步触发）。
type Observer interface {
	// OnAttempt 每次底层调用尝试结束时触发（attempt 从 0 计）。
	OnAttempt(target, op string, attempt int, success bool)
	// OnRetry 在同目标发起下一次重试前触发（nextAttempt 从 1 计）。
	OnRetry(target, op string, nextAttempt int)
	// OnFallback 从 from 目标降级到 to 目标时触发。
	OnFallback(from, to, requestID string)
	// OnCircuitReject 因熔断打开跳过某目标时触发。
	OnCircuitReject(target string)
	// OnCircuitTransition 熔断器状态跃迁时触发。
	OnCircuitTransition(target string, from, to CircuitState)
}

// NoopObserver 默认空实现。
type NoopObserver struct{}

func (NoopObserver) OnAttempt(string, string, int, bool)          {}
func (NoopObserver) OnRetry(string, string, int)                  {}
func (NoopObserver) OnFallback(string, string, string)            {}
func (NoopObserver) OnCircuitReject(string)                       {}
func (NoopObserver) OnCircuitTransition(string, CircuitState, CircuitState) {}

// pkgObserver 进程级默认观测器；熔断状态跃迁经 globalRegistry 路由至此。
var pkgObserver Observer = NoopObserver{}

func init() {
	// 全局注册表的熔断跃迁统一转发到当前包级观测器。
	// 钩子本身稳定，观测器可经 SetObserver 热替换。
	globalRegistry.onTransition = func(key string, from, to CircuitState) {
		pkgObserver.OnCircuitTransition(key, from, to)
	}
}

// SetObserver 设置进程级默认观测器（组合根在启动期调用一次）。
// 未显式传入观测器的弹性装饰器将使用它。
func SetObserver(o Observer) {
	if o == nil {
		o = NoopObserver{}
	}
	pkgObserver = o
}

// resolveObserver 返回可用的观测器：优先显式传入，否则用包级默认。
func resolveObserver(o Observer) Observer {
	if o == nil {
		return pkgObserver
	}
	return o
}
