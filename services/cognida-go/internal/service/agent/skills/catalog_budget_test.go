package skills

import (
	"strings"
	"testing"
)

// mkSkill 造一个目录测试用的极简技能。
func mkSkill(name, desc, when string, experimental bool) *Skill {
	return &Skill{Name: name, Description: desc, WhenToUse: when, Experimental: experimental}
}

// TestRenderCatalog_Empty 无技能时返回空串，调用方据此决定是否拼接目录段。
func TestRenderCatalog_Empty(t *testing.T) {
	if got := renderCatalog(nil); got != "" {
		t.Fatalf("空技能应返回空串, got %q", got)
	}
}

// TestRenderCatalog_CapsAtBudget 技能数超预算时只展开 maxCatalogSkills 个，其余计入提示。
func TestRenderCatalog_CapsAtBudget(t *testing.T) {
	total := maxCatalogSkills + 5
	all := make([]*Skill, 0, total)
	for i := 0; i < total; i++ {
		all = append(all, mkSkill(string(rune('a'+i%26))+string(rune('0'+i/26)), "desc", "", false))
	}

	out := renderCatalog(all)

	if shown := strings.Count(out, "- **"); shown != maxCatalogSkills {
		t.Fatalf("目录应只展开 %d 个技能, got %d", maxCatalogSkills, shown)
	}
	if !strings.Contains(out, "另有 5 个技能未在目录展开") {
		t.Fatalf("应提示被省略的 5 个技能, got:\n%s", out)
	}
}

// TestRenderCatalog_CuratedBeforeExperimental 预算裁剪时优先保留策展技能、先裁实验性技能。
func TestRenderCatalog_CuratedBeforeExperimental(t *testing.T) {
	all := make([]*Skill, 0, maxCatalogSkills+2)
	// 造 maxCatalogSkills 个策展 + 2 个实验性；实验性应被裁掉。
	for i := 0; i < maxCatalogSkills; i++ {
		all = append(all, mkSkill("curated-"+string(rune('a'+i%26))+string(rune('0'+i/26)), "d", "", false))
	}
	all = append(all, mkSkill("exp-one", "d", "", true))
	all = append(all, mkSkill("exp-two", "d", "", true))

	out := renderCatalog(all)

	if strings.Contains(out, "exp-one") || strings.Contains(out, "exp-two") {
		t.Fatalf("实验性技能应被优先裁掉, got:\n%s", out)
	}
	if !strings.Contains(out, "另有 2 个技能未在目录展开") {
		t.Fatalf("应提示 2 个实验性技能被省略, got:\n%s", out)
	}
}

// TestRenderCatalog_TruncatesLongText 超长描述/场景按 rune 截断并追加省略号，防单条撑爆预算。
func TestRenderCatalog_TruncatesLongText(t *testing.T) {
	longDesc := strings.Repeat("描", maxCatalogDescRunes+50)
	longWhen := strings.Repeat("景", maxCatalogWhenRunes+50)
	out := renderCatalog([]*Skill{mkSkill("s1", longDesc, longWhen, false)})

	if !strings.Contains(out, "…") {
		t.Fatalf("超长文本应截断并追加省略号, got:\n%s", out)
	}
	// 完整长串不应原样出现（已被截断）。
	if strings.Contains(out, longDesc) {
		t.Fatal("超长描述未被截断")
	}
	if strings.Contains(out, longWhen) {
		t.Fatal("超长适用场景未被截断")
	}
}

// TestRenderCatalog_NoNoteWhenWithinBudget 未超预算时不应出现「另有 N 个」提示。
func TestRenderCatalog_NoNoteWhenWithinBudget(t *testing.T) {
	out := renderCatalog([]*Skill{
		mkSkill("s1", "d", "", false),
		mkSkill("s2", "d", "w", true),
	})
	if strings.Contains(out, "未在目录展开") {
		t.Fatalf("未超预算不应出现省略提示, got:\n%s", out)
	}
	if c := strings.Count(out, "- **"); c != 2 {
		t.Fatalf("应展开全部 2 个技能, got %d", c)
	}
}
