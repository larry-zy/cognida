package resilience

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// ========================================
// 指标观测器
// ========================================

// MetricsObserver 以进程内原子计数聚合弹性事件，供 /metrics 或测试读取。
type MetricsObserver struct {
	attemptsTotal   atomic.Int64
	attemptsSuccess atomic.Int64
	attemptsFailed  atomic.Int64
	retriesTotal    atomic.Int64
	fallbacksTotal  atomic.Int64
	circuitRejects  atomic.Int64
	circuitOpened   atomic.Int64
	circuitClosed   atomic.Int64
	circuitHalfOpen atomic.Int64
}

// NewMetricsObserver 创建指标观测器。
func NewMetricsObserver() *MetricsObserver { return &MetricsObserver{} }

func (m *MetricsObserver) OnAttempt(_ string, _ string, _ int, success bool) {
	m.attemptsTotal.Add(1)
	if success {
		m.attemptsSuccess.Add(1)
	} else {
		m.attemptsFailed.Add(1)
	}
}
func (m *MetricsObserver) OnRetry(string, string, int)      { m.retriesTotal.Add(1) }
func (m *MetricsObserver) OnFallback(string, string, string) { m.fallbacksTotal.Add(1) }
func (m *MetricsObserver) OnCircuitReject(string)            { m.circuitRejects.Add(1) }

func (m *MetricsObserver) OnCircuitTransition(_ string, _ CircuitState, to CircuitState) {
	switch to {
	case StateOpen:
		m.circuitOpened.Add(1)
	case StateClosed:
		m.circuitClosed.Add(1)
	case StateHalfOpen:
		m.circuitHalfOpen.Add(1)
	}
}

// MetricsSnapshot 指标只读快照。
type MetricsSnapshot struct {
	AttemptsTotal   int64
	AttemptsSuccess int64
	AttemptsFailed  int64
	RetriesTotal    int64
	FallbacksTotal  int64
	CircuitRejects  int64
	CircuitOpened   int64
	CircuitClosed   int64
	CircuitHalfOpen int64
}

// Snapshot 返回当前累计计数。
func (m *MetricsObserver) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		AttemptsTotal:   m.attemptsTotal.Load(),
		AttemptsSuccess: m.attemptsSuccess.Load(),
		AttemptsFailed:  m.attemptsFailed.Load(),
		RetriesTotal:    m.retriesTotal.Load(),
		FallbacksTotal:  m.fallbacksTotal.Load(),
		CircuitRejects:  m.circuitRejects.Load(),
		CircuitOpened:   m.circuitOpened.Load(),
		CircuitClosed:   m.circuitClosed.Load(),
		CircuitHalfOpen: m.circuitHalfOpen.Load(),
	}
}

// ========================================
// 结构化日志观测器
// ========================================

// LoggingObserver 用 slog 输出结构化弹性事件，request_id 由降级事件透传以便 Loki 关联。
type LoggingObserver struct {
	logger *slog.Logger
}

// NewLoggingObserver 创建结构化日志观测器；logger 为空时用 slog.Default()。
func NewLoggingObserver(logger *slog.Logger) *LoggingObserver {
	if logger == nil {
		logger = slog.Default()
	}
	return &LoggingObserver{logger: logger.With(slog.String("component", "llm-resilience"))}
}

// OnAttempt 仅在失败时记 debug，避免正常路径刷屏。
func (l *LoggingObserver) OnAttempt(target, op string, attempt int, success bool) {
	if success {
		return
	}
	l.logger.LogAttrs(context.Background(), slog.LevelDebug, "llm attempt failed",
		slog.String("target", target), slog.String("op", op), slog.Int("attempt", attempt))
}

func (l *LoggingObserver) OnRetry(target, op string, nextAttempt int) {
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, "llm retry",
		slog.String("target", target), slog.String("op", op), slog.Int("next_attempt", nextAttempt))
}

func (l *LoggingObserver) OnFallback(from, to, requestID string) {
	l.logger.LogAttrs(context.Background(), slog.LevelWarn, "llm fallback",
		slog.String("from", from), slog.String("to", to), slog.String("request_id", requestID))
}

func (l *LoggingObserver) OnCircuitReject(target string) {
	l.logger.LogAttrs(context.Background(), slog.LevelWarn, "llm circuit rejected request",
		slog.String("target", target))
}

func (l *LoggingObserver) OnCircuitTransition(target string, from, to CircuitState) {
	l.logger.LogAttrs(context.Background(), slog.LevelWarn, "llm circuit transition",
		slog.String("target", target), slog.String("from", from.String()), slog.String("to", to.String()))
}

// ========================================
// 组合观测器
// ========================================

// CompositeObserver 将事件扇出到多个观测器（如指标 + 日志）。
type CompositeObserver struct {
	observers []Observer
}

// NewCompositeObserver 组合多个观测器（nil 元素被忽略）。
func NewCompositeObserver(observers ...Observer) *CompositeObserver {
	filtered := make([]Observer, 0, len(observers))
	for _, o := range observers {
		if o != nil {
			filtered = append(filtered, o)
		}
	}
	return &CompositeObserver{observers: filtered}
}

func (c *CompositeObserver) OnAttempt(target, op string, attempt int, success bool) {
	for _, o := range c.observers {
		o.OnAttempt(target, op, attempt, success)
	}
}
func (c *CompositeObserver) OnRetry(target, op string, nextAttempt int) {
	for _, o := range c.observers {
		o.OnRetry(target, op, nextAttempt)
	}
}
func (c *CompositeObserver) OnFallback(from, to, requestID string) {
	for _, o := range c.observers {
		o.OnFallback(from, to, requestID)
	}
}
func (c *CompositeObserver) OnCircuitReject(target string) {
	for _, o := range c.observers {
		o.OnCircuitReject(target)
	}
}
func (c *CompositeObserver) OnCircuitTransition(target string, from, to CircuitState) {
	for _, o := range c.observers {
		o.OnCircuitTransition(target, from, to)
	}
}
