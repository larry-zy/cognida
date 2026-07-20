package experience

import (
	"testing"
	"time"
)

func TestTimeEnv_ParsesFormatsAndFallsBack(t *testing.T) {
	const key = "EXPERIENCE_START_FROM_TEST"

	// 日期格式：按本地零点解释。
	t.Setenv(key, "2026-07-20")
	got := timeEnv(key, time.Time{})
	want := time.Date(2026, 7, 20, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("日期解析 = %v, want %v", got, want)
	}

	// 日期时间格式。
	t.Setenv(key, "2026-07-20 09:30:00")
	got = timeEnv(key, time.Time{})
	want = time.Date(2026, 7, 20, 9, 30, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("日期时间解析 = %v, want %v", got, want)
	}

	// 空值回落 def（零值=不限）。
	t.Setenv(key, "")
	if got := timeEnv(key, time.Time{}); !got.IsZero() {
		t.Fatalf("空值应回落零值, got %v", got)
	}

	// 非法值回落 def，不 panic。
	def := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	t.Setenv(key, "not-a-date")
	if got := timeEnv(key, def); !got.Equal(def) {
		t.Fatalf("非法值应回落 def=%v, got %v", def, got)
	}
}

func TestConfigFromEnv_StartFrom(t *testing.T) {
	t.Setenv("EXPERIENCE_START_FROM", "2026-07-20")
	cfg := ConfigFromEnv()
	if cfg.StartFrom.IsZero() {
		t.Fatal("StartFrom 应被 EXPERIENCE_START_FROM 装配")
	}
	if y, m, d := cfg.StartFrom.Date(); y != 2026 || m != 7 || d != 20 {
		t.Fatalf("StartFrom = %v, want 2026-07-20", cfg.StartFrom)
	}
}
