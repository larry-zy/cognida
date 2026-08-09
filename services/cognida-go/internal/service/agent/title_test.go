package agent

import (
	"strings"
	"testing"
)

// TestGenerateSessionTitle_EmptyQuery 空/空白查询不应生成带空尾巴的标题。
func TestGenerateSessionTitle_EmptyQuery(t *testing.T) {
	for _, q := range []string{"", "   ", "\t\n"} {
		got := generateSessionTitle("agent-data-agent", q)
		if got != "SQL查询" {
			t.Errorf("空查询 %q 标题 = %q, 期望 %q", q, got, "SQL查询")
		}
		if strings.HasSuffix(got, " ") {
			t.Errorf("标题不应有空尾巴: %q", got)
		}
	}
}

// TestGenerateSessionTitle_CJKNoPanic 中文查询（字节数>30 但 rune<30）不得 panic，且按 rune 正常拼装。
func TestGenerateSessionTitle_CJKNoPanic(t *testing.T) {
	// 15 个汉字 = 45 字节 > 30，旧实现会在 []rune[:30] 越界 panic
	q := "查询北京上海广州深圳杭州成都武汉南京" // 18 runes
	got := generateSessionTitle("agent-data-agent", q)
	want := "[SQL查询] " + q
	if got != want {
		t.Errorf("标题 = %q, 期望 %q", got, want)
	}
}

// TestGenerateSessionTitle_TruncatesByRune 超过 30 runes 的查询按 rune 截断并加省略号。
func TestGenerateSessionTitle_TruncatesByRune(t *testing.T) {
	q := strings.Repeat("数", 40) // 40 runes
	got := generateSessionTitle("agent-data-agent", q)
	wantPrefix := "[SQL查询] " + strings.Repeat("数", 30) + "..."
	if got != wantPrefix {
		t.Errorf("标题 = %q, 期望 %q", got, wantPrefix)
	}
}
