package resilience

import "testing"

func TestMetricsObserver_CountsEvents(t *testing.T) {
	m := NewMetricsObserver()

	m.OnAttempt("openai/gpt", "chat", 0, true)
	m.OnAttempt("openai/gpt", "chat", 1, false)
	m.OnRetry("openai/gpt", "chat", 1)
	m.OnFallback("openai/gpt", "aliyun/qwen", "req-1")
	m.OnCircuitReject("openai/gpt")
	m.OnCircuitTransition("openai/gpt", StateClosed, StateOpen)
	m.OnCircuitTransition("openai/gpt", StateOpen, StateHalfOpen)
	m.OnCircuitTransition("openai/gpt", StateHalfOpen, StateClosed)

	s := m.Snapshot()
	if s.AttemptsTotal != 2 || s.AttemptsSuccess != 1 || s.AttemptsFailed != 1 {
		t.Fatalf("attempts: %+v", s)
	}
	if s.RetriesTotal != 1 {
		t.Fatalf("retries=%d want 1", s.RetriesTotal)
	}
	if s.FallbacksTotal != 1 {
		t.Fatalf("fallbacks=%d want 1", s.FallbacksTotal)
	}
	if s.CircuitRejects != 1 {
		t.Fatalf("rejects=%d want 1", s.CircuitRejects)
	}
	if s.CircuitOpened != 1 || s.CircuitHalfOpen != 1 || s.CircuitClosed != 1 {
		t.Fatalf("transitions: %+v", s)
	}
}

// countingObserver 记录被扇出的调用次数，用于校验 CompositeObserver。
type countingObserver struct {
	attempt, retry, fallback, reject, transition int
}

func (c *countingObserver) OnAttempt(string, string, int, bool)  { c.attempt++ }
func (c *countingObserver) OnRetry(string, string, int)          { c.retry++ }
func (c *countingObserver) OnFallback(string, string, string)    { c.fallback++ }
func (c *countingObserver) OnCircuitReject(string)               { c.reject++ }
func (c *countingObserver) OnCircuitTransition(string, CircuitState, CircuitState) {
	c.transition++
}

func TestCompositeObserver_FansOut(t *testing.T) {
	a, b := &countingObserver{}, &countingObserver{}
	// nil 元素应被忽略，不引发 panic。
	c := NewCompositeObserver(a, nil, b)

	c.OnAttempt("t", "chat", 0, true)
	c.OnRetry("t", "chat", 1)
	c.OnFallback("t", "u", "req")
	c.OnCircuitReject("t")
	c.OnCircuitTransition("t", StateClosed, StateOpen)

	for _, o := range []*countingObserver{a, b} {
		if o.attempt != 1 || o.retry != 1 || o.fallback != 1 || o.reject != 1 || o.transition != 1 {
			t.Fatalf("fan-out mismatch: %+v", o)
		}
	}
}

func TestCompositeObserver_IgnoresAllNil(t *testing.T) {
	c := NewCompositeObserver(nil, nil)
	// 不应 panic。
	c.OnAttempt("t", "chat", 0, false)
	c.OnCircuitTransition("t", StateClosed, StateOpen)
}
