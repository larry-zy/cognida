package context

import (
	"strings"
	"testing"
)

// 规范四层在预算收紧时的裁剪次序：先丢会话记忆，再丢 skill playbook，
// 能力层次之，系统与安全层（Pinned）始终保留。
func TestCanonicalLayers_TrimOrderPreservesSafety(t *testing.T) {
	layers := []Layer{
		SystemLayer("SYS"),               // pinned, 3
		SafetyLayer("SAFE_SCOPE"),        // pinned, 10
		CapabilityLayer("TOOLS_CONTRACT"), // 14
		SkillPlaybookLayer("PLAYBOOK"),    // 8
		MemoryLayer("SESSION_MEMORY"),     // 14
	}
	// pinned=13；给能力层留 14，其余 (playbook+memory) 应被丢弃。
	res := Assemble(layers, BudgetConfig{MaxTokens: 27}, runeCounter{})

	if !strings.Contains(res.Prompt, "SYS") || !strings.Contains(res.Prompt, "SAFE_SCOPE") {
		t.Fatal("safety/system layers must always be retained")
	}
	assertContainsAll(t, res.Kept, LayerSystem, LayerSafety, LayerCapability)
	assertContainsAll(t, res.Dropped, LayerSkillPlaybook, LayerMemory)
}

func TestCanonicalLayers_MemoryDroppedBeforePlaybook(t *testing.T) {
	layers := []Layer{
		SystemLayer("S"),             // pinned 1
		SafetyLayer("SAFE"),          // pinned 4
		SkillPlaybookLayer("PLAYBK"), // 6, pri 50
		MemoryLayer("MEMORYX"),       // 7, pri 20
	}
	// pinned=5；只够再放一层。playbook(pri50) 优先于 memory(pri20)。
	res := Assemble(layers, BudgetConfig{MaxTokens: 11}, runeCounter{})
	assertContainsAll(t, res.Kept, LayerSkillPlaybook)
	assertContainsAll(t, res.Dropped, LayerMemory)
}
