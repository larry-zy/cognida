package resilience

import (
	"context"
	"testing"
	"time"

	domainllm "cognida/internal/model/llm"
)

// fakeClock 可控时钟。
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func testCfg() domainllm.ResilienceConfig {
	return domainllm.ResilienceConfig{
		Enabled:        true,
		MaxAttempts:    3,
		BaseBackoff:    time.Millisecond,
		MaxBackoff:     time.Millisecond,
		FailThreshold:  3,
		Cooldown:       30 * time.Second,
		HalfOpenProbes: 1,
	}
}

func TestBreaker_OpensAtThreshold(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newBreaker("p/m", testCfg(), clk.now)

	for i := 0; i < 2; i++ {
		if !b.Allow() {
			t.Fatalf("closed should allow (i=%d)", i)
		}
		b.OnResult(false, true)
	}
	if b.State() != StateClosed {
		t.Fatalf("after 2 fails still closed, got %s", b.State())
	}
	// 第 3 次计数失败触发熔断
	b.Allow()
	b.OnResult(false, true)
	if b.State() != StateOpen {
		t.Fatalf("expected open at threshold, got %s", b.State())
	}
	if b.Allow() {
		t.Fatal("open within cooldown must reject")
	}
}

func TestBreaker_TerminalNotCounted(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newBreaker("p/m", testCfg(), clk.now)
	for i := 0; i < 10; i++ {
		b.Allow()
		b.OnResult(false, false) // terminal/canceled 不计入
	}
	if b.State() != StateClosed {
		t.Fatalf("terminal failures must not open breaker, got %s", b.State())
	}
}

func TestBreaker_HalfOpenSingleProbe(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newBreaker("p/m", testCfg(), clk.now)
	// 打开
	for i := 0; i < 3; i++ {
		b.Allow()
		b.OnResult(false, true)
	}
	if b.State() != StateOpen {
		t.Fatalf("want open, got %s", b.State())
	}
	// 冷却未到
	clk.advance(29 * time.Second)
	if b.Allow() {
		t.Fatal("before cooldown must reject")
	}
	// 冷却到期 → 放行一个探测
	clk.advance(2 * time.Second)
	if !b.Allow() {
		t.Fatal("after cooldown must allow one probe")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("want half-open, got %s", b.State())
	}
	// 半开下第二个探测被拒
	if b.Allow() {
		t.Fatal("half-open must limit to HalfOpenProbes")
	}
	// 探测成功 → 闭合
	b.OnResult(true, true)
	if b.State() != StateClosed {
		t.Fatalf("probe success should close, got %s", b.State())
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newBreaker("p/m", testCfg(), clk.now)
	for i := 0; i < 3; i++ {
		b.Allow()
		b.OnResult(false, true)
	}
	clk.advance(31 * time.Second)
	b.Allow() // half-open probe
	b.OnResult(false, true)
	if b.State() != StateOpen {
		t.Fatalf("half-open failure should reopen, got %s", b.State())
	}
}

// TestBreaker_HalfOpenNonCountableReleasesProbe 确保非计入结果（如探测被取消）
// 释放 half-open 已占用的探测名额，避免熔断器永久卡死拒绝所有请求。
func TestBreaker_HalfOpenNonCountableReleasesProbe(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newBreaker("p/m", testCfg(), clk.now)
	for i := 0; i < 3; i++ {
		b.Allow()
		b.OnResult(false, true)
	}
	clk.advance(31 * time.Second)
	if !b.Allow() { // 占用唯一探测名额
		t.Fatal("after cooldown must allow one probe")
	}
	if b.Allow() { // 名额已满
		t.Fatal("half-open must reject a second concurrent probe")
	}
	// 探测未真正执行（被取消）→ 非计入结果释放名额
	b.OnResult(false, false)
	if b.State() != StateHalfOpen {
		t.Fatalf("non-countable probe must keep half-open, got %s", b.State())
	}
	if !b.Allow() {
		t.Fatal("released probe slot must permit a subsequent probe (breaker not wedged)")
	}
}

// TestAttemptTarget_CancelAfterAllowReleasesProbe 覆盖 Allow() 放行后、发起请求前
// ctx 被取消的窗口：attemptTarget 必须不 invoke 且释放已占用的 half-open 探测名额，
// 使熔断器不会被永久卡死（回归 half-open 探测名额泄漏 bug）。
func TestAttemptTarget_CancelAfterAllowReleasesProbe(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	cfg := testCfg()
	b := newBreaker(domainllm.TargetKey(domainllm.ProviderOpenAI, "wedge"), cfg, clk.now)
	// 打开并推进到 half-open，占用唯一探测名额（模拟 executeChain 的 Allow()）。
	for i := 0; i < 3; i++ {
		b.Allow()
		b.OnResult(false, true)
	}
	clk.advance(31 * time.Second)
	if !b.Allow() {
		t.Fatal("cooldown elapsed: probe should be allowed")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("want half-open, got %s", b.State())
	}

	tgt := target[*scriptedClient]{
		provider: domainllm.ProviderOpenAI, model: "wedge",
		key: b.key, breaker: b, client: &scriptedClient{name: "x"},
	}
	invoked := 0
	invoke := func(_ context.Context, _ *scriptedClient) (string, error) {
		invoked++
		return "ok", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Allow 之后、invoke 之前取消

	_, err, class := attemptTarget[*scriptedClient, string](ctx, &tgt, cfg, NoopObserver{}, "chat", invoke)
	if class != domainllm.ClassCanceled || err == nil {
		t.Fatalf("expected canceled, got class=%s err=%v", class, err)
	}
	if invoked != 0 {
		t.Fatalf("canceled pre-invoke must not call client, got %d", invoked)
	}
	// 关键回归断言：名额已释放，熔断器仍可放行后续探测，未被永久卡死。
	if b.State() != StateHalfOpen {
		t.Fatalf("breaker should stay half-open, got %s", b.State())
	}
	if !b.Allow() {
		t.Fatal("probe slot leaked: breaker wedged in half-open, rejecting all requests")
	}
}

func TestRegistry_ReusesBreaker(t *testing.T) {
	reg := newRegistry()
	cfg := testCfg()
	b1 := reg.get("openai/gpt-4", cfg)
	b2 := reg.get("openai/gpt-4", cfg)
	if b1 != b2 {
		t.Fatal("same key must reuse breaker instance")
	}
	b3 := reg.get("openai/gpt-3.5", cfg)
	if b1 == b3 {
		t.Fatal("different key must yield distinct breaker")
	}
}

func TestBreaker_Transitions(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newBreaker("p/m", testCfg(), clk.now)
	var seen []string
	b.onTransition = func(_ string, from, to CircuitState) {
		seen = append(seen, from.String()+"->"+to.String())
	}
	for i := 0; i < 3; i++ {
		b.Allow()
		b.OnResult(false, true)
	}
	clk.advance(31 * time.Second)
	b.Allow()
	b.OnResult(true, true)
	want := []string{"closed->open", "open->half-open", "half-open->closed"}
	if len(seen) != len(want) {
		t.Fatalf("transitions=%v want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("transition[%d]=%s want %s", i, seen[i], want[i])
		}
	}
}
