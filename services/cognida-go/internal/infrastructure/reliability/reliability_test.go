package reliability

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeClock 提供可控时钟，便于驱动熔断冷却。
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestBreaker(cfg Config, clk *fakeClock) *Breaker {
	cfg = cfg.WithDefaults()
	return &Breaker{key: "t", cfg: cfg, now: clk.now, state: StateClosed}
}

func TestBreaker_TripsAfterThreshold(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newTestBreaker(Config{FailThreshold: 3, Cooldown: 10 * time.Second, HalfOpenProbes: 1}, clk)

	for i := 0; i < 2; i++ {
		b.OnResult(false, true)
		if b.State() != StateClosed {
			t.Fatalf("失败 %d 次后应仍为 closed，得 %s", i+1, b.State())
		}
		if !b.Allow() {
			t.Fatalf("closed 态应放行")
		}
	}
	// 第 3 次达到阈值 → open。
	b.OnResult(false, true)
	if b.State() != StateOpen {
		t.Fatalf("达到阈值应 open，得 %s", b.State())
	}
	if b.Allow() {
		t.Fatalf("open 态应拒绝")
	}
}

func TestBreaker_NonCountableDoesNotTrip(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newTestBreaker(Config{FailThreshold: 2, Cooldown: 10 * time.Second}, clk)

	for i := 0; i < 10; i++ {
		b.OnResult(false, false) // 不可计失败（如调用方取消）
	}
	if b.State() != StateClosed {
		t.Fatalf("不可计失败不应触发熔断，得 %s", b.State())
	}
}

func TestBreaker_HalfOpenRecovery(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newTestBreaker(Config{FailThreshold: 1, Cooldown: 10 * time.Second, HalfOpenProbes: 1}, clk)

	b.OnResult(false, true) // 阈值=1，立即 open
	if b.State() != StateOpen {
		t.Fatalf("应 open")
	}
	// 冷却未到：仍拒绝。
	clk.advance(5 * time.Second)
	if b.Allow() {
		t.Fatalf("冷却未到应拒绝")
	}
	// 冷却到期：放行一个探测（half-open），额外探测被限流。
	clk.advance(6 * time.Second)
	if !b.Allow() {
		t.Fatalf("冷却到期应放行探测")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("应 half-open，得 %s", b.State())
	}
	if b.Allow() {
		t.Fatalf("half-open 探测数已用尽应拒绝")
	}
	// 探测成功 → 闭合恢复。
	b.OnResult(true, false)
	if b.State() != StateClosed {
		t.Fatalf("探测成功应恢复 closed，得 %s", b.State())
	}
	if !b.Allow() {
		t.Fatalf("恢复后应放行")
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newTestBreaker(Config{FailThreshold: 1, Cooldown: 10 * time.Second, HalfOpenProbes: 1}, clk)

	b.OnResult(false, true)
	clk.advance(11 * time.Second)
	b.Allow() // → half-open 探测
	b.OnResult(false, true)
	if b.State() != StateOpen {
		t.Fatalf("half-open 探测失败应重新 open，得 %s", b.State())
	}
}

func TestRegistry_PerTargetIsolation(t *testing.T) {
	reg := NewRegistry(Config{FailThreshold: 1, Cooldown: time.Second})
	a := reg.Get("a:1")
	b := reg.Get("b:1")
	if a == b {
		t.Fatalf("不同 target 应得不同熔断器")
	}
	if reg.Get("a:1") != a {
		t.Fatalf("同 target 应复用同一熔断器")
	}
	a.OnResult(false, true) // 仅 a open
	if a.State() != StateOpen || b.State() != StateClosed {
		t.Fatalf("熔断状态应按 target 隔离：a=%s b=%s", a.State(), b.State())
	}
}

func TestServiceConfigJSON_ValidAndRetryable(t *testing.T) {
	js := ServiceConfigJSON(Config{MaxAttempts: 4, BaseBackoff: 200 * time.Millisecond, MaxBackoff: 5 * time.Second})
	var parsed struct {
		MethodConfig []struct {
			RetryPolicy struct {
				MaxAttempts          int      `json:"MaxAttempts"`
				InitialBackoff       string   `json:"InitialBackoff"`
				RetryableStatusCodes []string `json:"RetryableStatusCodes"`
			} `json:"retryPolicy"`
		} `json:"methodConfig"`
	}
	if err := json.Unmarshal([]byte(js), &parsed); err != nil {
		t.Fatalf("服务配置须为合法 JSON: %v\n%s", err, js)
	}
	if len(parsed.MethodConfig) != 1 {
		t.Fatalf("应有一条 methodConfig")
	}
	rp := parsed.MethodConfig[0].RetryPolicy
	if rp.MaxAttempts != 4 {
		t.Fatalf("MaxAttempts 应为 4，得 %d", rp.MaxAttempts)
	}
	if rp.InitialBackoff != "0.200s" {
		t.Fatalf("InitialBackoff 格式应为秒制 duration，得 %q", rp.InitialBackoff)
	}
	want := map[string]bool{"UNAVAILABLE": true, "RESOURCE_EXHAUSTED": true}
	if len(rp.RetryableStatusCodes) != len(want) {
		t.Fatalf("可重试码集合不符：%v", rp.RetryableStatusCodes)
	}
	for _, c := range rp.RetryableStatusCodes {
		if !want[c] {
			t.Fatalf("不应出现可重试码 %q（DEADLINE_EXCEEDED 等须排除）", c)
		}
	}
}

func TestServiceConfigJSON_ClampsMaxAttempts(t *testing.T) {
	// gRPC 服务配置要求 MaxAttempts ∈ [2,5]。
	js := ServiceConfigJSON(Config{MaxAttempts: 99})
	if !contains(js, `"MaxAttempts": 5`) {
		t.Fatalf("MaxAttempts 应夹取到 5：%s", js)
	}
	js = ServiceConfigJSON(Config{MaxAttempts: 1})
	if !contains(js, `"MaxAttempts": 2`) {
		t.Fatalf("MaxAttempts 应夹取到 2：%s", js)
	}
}

func TestCountableCode(t *testing.T) {
	countable := []codes.Code{codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded}
	for _, c := range countable {
		if !countableCode(c) {
			t.Fatalf("%s 应计入熔断", c)
		}
	}
	notCountable := []codes.Code{codes.OK, codes.InvalidArgument, codes.NotFound, codes.Canceled, codes.Unauthenticated}
	for _, c := range notCountable {
		if countableCode(c) {
			t.Fatalf("%s 不应计入熔断", c)
		}
	}
}

func TestRecordResult_CallerCancelNotCounted(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	b := newTestBreaker(Config{FailThreshold: 1, Cooldown: time.Second}, clk)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// 调用方取消：即便阈值=1 也不应 open。
	recordResult(b, ctx, status.Error(codes.Canceled, "cancelled"))
	if b.State() != StateClosed {
		t.Fatalf("调用方取消不应触发熔断，得 %s", b.State())
	}
	// 目标不可用：应立即 open。
	recordResult(b, context.Background(), status.Error(codes.Unavailable, "down"))
	if b.State() != StateOpen {
		t.Fatalf("目标 Unavailable 应触发熔断，得 %s", b.State())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
