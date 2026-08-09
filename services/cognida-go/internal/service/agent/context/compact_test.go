package context

import (
	"strings"
	"testing"
)

func TestCapContentByTokens_UnderLimitUnchanged(t *testing.T) {
	c := ApproxTokenCounter{}
	s := "短内容不动"
	if got := CapContentByTokens(s, 1000, c); got != s {
		t.Errorf("在限内应原样返回, got %q", got)
	}
	// 边界：maxTokens<=0 / 空内容 / 无计数器 → 原样。
	if got := CapContentByTokens(s, 0, c); got != s {
		t.Errorf("maxTokens<=0 应原样, got %q", got)
	}
	if got := CapContentByTokens("", 10, c); got != "" {
		t.Errorf("空内容应原样, got %q", got)
	}
	if got := CapContentByTokens(s, 10, nil); got != s {
		t.Errorf("无计数器应原样, got %q", got)
	}
}

func TestCapContentByTokens_TruncatesOverLimit(t *testing.T) {
	c := ApproxTokenCounter{}
	// 一长串 CJK（约 0.7 token/字）。
	long := strings.Repeat("数据分析结论明细", 200) // 1600 字 ≈ 1120 token
	if c.Count(long) <= 50 {
		t.Fatalf("构造失败：长文本 token=%d 应远大于 50", c.Count(long))
	}
	got := CapContentByTokens(long, 50, c)
	if c.Count(got) > 50+5 { // 允许省略号等极小溢出
		t.Errorf("压缩后 token=%d 应 ~<=50", c.Count(got))
	}
	if len([]rune(got)) >= len([]rune(long)) {
		t.Errorf("应被截断变短")
	}
}

func TestCapContentByTokens_PreservesResultID(t *testing.T) {
	c := ApproxTokenCounter{}
	// result_id 在文本很靠后的位置，普通截断会把它砍掉。
	// 用无下划线分段的 id：collectResultIDs 的正则 rs_[0-9a-zA-Z-]+ 不含 '_'，会在下划线处断开。
	body := strings.Repeat("前置分析内容很长很长", 100) // ~700 token
	content := body + " 结果见 rs_eastq2"
	got := CapContentByTokens(content, 30, c)
	if !strings.Contains(got, "rs_eastq2") {
		t.Errorf("截断后必须仍含 result_id rs_eastq2, got 末尾: %q", got[len(got)-60:])
	}
}

func TestMaskObservation_ReplacesBulkKeepsResultID(t *testing.T) {
	c := ApproxTokenCounter{}
	obs := strings.Repeat("超大的工具观察结果明细", 300) + " 结果见 rs_alpha 与 rs_beta"
	got := MaskObservation(obs, c)
	if c.Count(got) >= c.Count(obs) {
		t.Errorf("屏蔽后应远小于原文, got token=%d, orig=%d", c.Count(got), c.Count(obs))
	}
	if !strings.HasPrefix(got, ObservationMaskMarker) {
		t.Errorf("屏蔽后应以占位前缀开头, got %q", got)
	}
	for _, id := range []string{"rs_alpha", "rs_beta"} {
		if !strings.Contains(got, id) {
			t.Errorf("屏蔽后必须保住 result_id %s, got %q", id, got)
		}
	}
}

func TestMaskObservation_Idempotent(t *testing.T) {
	c := ApproxTokenCounter{}
	obs := strings.Repeat("大观察", 200) + " rs_x"
	once := MaskObservation(obs, c)
	twice := MaskObservation(once, c)
	if once != twice {
		t.Errorf("重复屏蔽应幂等, once=%q twice=%q", once, twice)
	}
}

func TestMaskObservation_NoNegativeOptimization(t *testing.T) {
	c := ApproxTokenCounter{}
	// 本就极短的观察：占位符不比它短 → 原样返回，不做负优化。
	short := "OK"
	if got := MaskObservation(short, c); got != short {
		t.Errorf("极短观察不应被屏蔽（负优化）, got %q", got)
	}
	// 边界：空内容 / 无计数器 → 原样。
	if got := MaskObservation("", c); got != "" {
		t.Errorf("空内容应原样, got %q", got)
	}
	if got := MaskObservation(short, nil); got != short {
		t.Errorf("无计数器应原样, got %q", got)
	}
}

func TestCollectResultIDs_Exported(t *testing.T) {
	ids := CollectResultIDs([]Turn{
		{Content: "见 rs_a"},
		{Content: "又见 rs_a 和 rs_b"},
	})
	if len(ids) != 2 || ids[0] != "rs_a" || ids[1] != "rs_b" {
		t.Errorf("CollectResultIDs = %v, want [rs_a rs_b]", ids)
	}
}
