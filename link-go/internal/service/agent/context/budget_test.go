package context

import (
	"strings"
	"testing"
)

// runeCounter 是确定性的测试用 token 计数器：1 rune = 1 token。
type runeCounter struct{}

func (runeCounter) Count(text string) int { return len([]rune(text)) }

func fourLayers() []Layer {
	return []Layer{
		{Name: "system", Content: "SYSTEM", Priority: 100, Pinned: true},
		{Name: "safety", Content: "SAFETY", Priority: 90, Pinned: true},
		{Name: "capability", Content: "CAPS_LAYER", Priority: 50},
		{Name: "memory", Content: "MEMORY_LAYER", Priority: 10},
	}
}

func TestAssemble_WithinBudgetKeepsAllInOrder(t *testing.T) {
	res := Assemble(fourLayers(), BudgetConfig{MaxTokens: 1000}, runeCounter{})
	if len(res.Kept) != 4 || len(res.Dropped) != 0 || len(res.Truncated) != 0 {
		t.Fatalf("expected all kept, got kept=%v dropped=%v trunc=%v", res.Kept, res.Dropped, res.Truncated)
	}
	// 显示顺序：system, safety, capability, memory。
	wantOrder := "SYSTEM\n\nSAFETY\n\nCAPS_LAYER\n\nMEMORY_LAYER"
	if res.Prompt != wantOrder {
		t.Errorf("prompt order = %q, want %q", res.Prompt, wantOrder)
	}
	if res.OverBudget {
		t.Error("should not be over budget")
	}
}

func TestAssemble_DropsLowPriorityKeepsPinned(t *testing.T) {
	// pinned=12, +capability(10)=22 恰好放下，memory 被丢弃。
	res := Assemble(fourLayers(), BudgetConfig{MaxTokens: 22}, runeCounter{})
	assertContainsAll(t, res.Kept, "system", "safety", "capability")
	assertContainsAll(t, res.Dropped, "memory")
	if strings.Contains(res.Prompt, "MEMORY_LAYER") {
		t.Error("dropped memory layer must not appear in prompt")
	}
	if !strings.Contains(res.Prompt, "SYSTEM") || !strings.Contains(res.Prompt, "SAFETY") {
		t.Error("pinned layers must be retained")
	}
	if res.OverBudget {
		t.Error("pinned fit within budget, should not be over budget")
	}
}

func TestAssemble_TruncatesBoundaryLayer(t *testing.T) {
	// pinned=12, remaining=5 → capability 截断到 5 token，memory 丢弃。
	res := Assemble(fourLayers(), BudgetConfig{MaxTokens: 17}, runeCounter{})
	assertContainsAll(t, res.Truncated, "capability")
	assertContainsAll(t, res.Dropped, "memory")
	if strings.Contains(res.Prompt, "CAPS_LAYER") {
		t.Error("capability should be truncated, not full")
	}
	if !strings.Contains(res.Prompt, "CAPS_") {
		t.Errorf("truncated capability prefix missing: %q", res.Prompt)
	}
}

func TestAssemble_PinnedOverBudgetStillRetained(t *testing.T) {
	// 预算比 pinned 还小：pinned 仍保留，非 pinned 全丢，OverBudget=true。
	res := Assemble(fourLayers(), BudgetConfig{MaxTokens: 5}, runeCounter{})
	if !res.OverBudget {
		t.Error("expected OverBudget when pinned exceeds budget")
	}
	if !strings.Contains(res.Prompt, "SYSTEM") || !strings.Contains(res.Prompt, "SAFETY") {
		t.Error("safety/system layers must survive even over budget")
	}
	assertContainsAll(t, res.Dropped, "capability", "memory")
}

func TestAssemble_PriorityOrderNotDisplayOrder(t *testing.T) {
	// 低显示位但高优先级的层应优先于高显示位低优先级的层被保留。
	layers := []Layer{
		{Name: "sys", Content: "S", Priority: 100, Pinned: true}, // 1 tok
		{Name: "low", Content: "LOWLOWLOW", Priority: 1},         // 9 tok, 显示在前
		{Name: "high", Content: "HIGH", Priority: 99},            // 4 tok, 显示在后
	}
	// 预算 = 1(pinned) + 4(high) = 5 → high 保留，low 丢弃。
	res := Assemble(layers, BudgetConfig{MaxTokens: 5}, runeCounter{})
	assertContainsAll(t, res.Kept, "sys", "high")
	assertContainsAll(t, res.Dropped, "low")
}

func assertContainsAll(t *testing.T, got []string, want ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("expected %q in %v", w, got)
		}
	}
}
