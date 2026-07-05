package skills

import (
	"context"
	"strings"
	"testing"
)

// seedGlobalSkill 向全局管理器注册一个测试 Skill（忽略重复注册），供 Hook 层测试使用。
func seedGlobalSkill(t *testing.T, s *Skill) {
	t.Helper()
	_ = GetGlobalManager().GetRegistry().Register(s)
}

func TestMatchTopSkill_GlobalCJK(t *testing.T) {
	seedGlobalSkill(t, &Skill{
		Name:        "data-analysis",
		Description: "专注于数据查询和分析的技能，通过 SQL 查询获取和分析数据。",
		WhenToUse:   "当任务需要查询数据库、分析数据、生成报表时使用此技能。",
		Category:    "data",
		Tags:        []string{"sql", "analytics"},
		Content:     "数据分析方法论正文MARKER。",
	})

	skill, ok := MatchTopSkill("帮我查询数据库并分析销售数据", 0)
	if !ok {
		t.Fatalf("expected a global CJK match")
	}
	if skill.Name != "data-analysis" {
		t.Errorf("top skill = %s, want data-analysis", skill.Name)
	}

	if _, ok := MatchTopSkill("今天天气怎么样", 0); ok {
		t.Errorf("unrelated query should not match")
	}
}

// AutoInjectHook：命中时把 Skill 指导前置注入并保留原始问题；同时把命中 Skill 暂存进 ctx。
func TestAutoInjectHook_InjectsAndStashes(t *testing.T) {
	seedGlobalSkill(t, &Skill{
		Name:        "data-analysis",
		Description: "专注于数据查询和分析的技能，通过 SQL 查询获取和分析数据。",
		WhenToUse:   "当任务需要查询数据库、分析数据、生成报表时使用此技能。",
		Category:    "data",
		Content:     "数据分析方法论正文MARKER。",
	})
	hook := AutoInjectHook(0)

	msg := "帮我查询数据库里的订单并分析"
	newCtx, routed, err := hook(context.Background(), msg)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if !strings.Contains(routed, "data-analysis") {
		t.Errorf("injected message should carry skill name, got: %s", routed)
	}
	if !strings.Contains(routed, "数据分析方法论正文MARKER") {
		t.Errorf("injected message should carry skill content")
	}
	if !strings.Contains(routed, msg) {
		t.Errorf("injected message must preserve the original question")
	}
	if !strings.Contains(routed, "【用户问题】") {
		t.Errorf("injected message should delimit the user question")
	}
	if skill, ok := MatchedSkillFromContext(newCtx); !ok || skill.Name != "data-analysis" {
		t.Errorf("hook should stash matched skill in ctx")
	}
}

// AutoInjectHook：未命中时原样透传，且不污染 ctx。
func TestAutoInjectHook_PassthroughOnMiss(t *testing.T) {
	hook := AutoInjectHook(0)
	msg := "今天天气怎么样适合出去玩吗"
	ctx, routed, err := hook(context.Background(), msg)
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if routed != msg {
		t.Errorf("miss should passthrough unchanged, got: %s", routed)
	}
	if _, ok := MatchedSkillFromContext(ctx); ok {
		t.Errorf("miss should not stash any skill")
	}
}

// InjectFromContextHook：从 ctx 取回暂存 Skill 注入指导；ctx 无暂存时透传。
func TestInjectFromContextHook(t *testing.T) {
	skill := &Skill{Name: "code-review", Description: "代码审查", Content: "审查清单REVIEWBODY。"}

	// 有暂存 → 注入。
	ctx := ContextWithMatchedSkill(context.Background(), skill)
	_, routed, err := InjectFromContextHook()(ctx, "playbook 文本\n\n【用户问题】\n看看这段代码")
	if err != nil {
		t.Fatalf("hook err: %v", err)
	}
	if !strings.Contains(routed, "审查清单REVIEWBODY") || !strings.Contains(routed, "code-review") {
		t.Errorf("should inject stashed skill guidance, got: %s", routed)
	}
	if !strings.Contains(routed, "看看这段代码") {
		t.Errorf("should preserve downstream message")
	}

	// 无暂存 → 透传。
	orig := "无技能场景"
	_, out, _ := InjectFromContextHook()(context.Background(), orig)
	if out != orig {
		t.Errorf("no stashed skill should passthrough, got: %s", out)
	}
}
