// 硬工具门策略装配测试（Phase 6 任务 7.3/7.5）：
// 命中 skill 的 allowed/disallowed_tools 构建 Allow/Deny、叠加会话 scope、
// 未命中/纯指导性 skill 走 scope-only、空 scope 按 read 最小权限兜底。
package dataagent

import (
	"testing"

	infraagent "link/internal/service/agent/framework"
	"link/internal/service/agent/skills"
)

// newTestSkillManager 构造独立注册表的 skill 管理器（不污染全局单例）。
func newTestSkillManager(t *testing.T, skill ...*skills.Skill) skills.SkillManager {
	t.Helper()
	manager := skills.NewSkillManager()
	for _, s := range skill {
		if err := manager.GetRegistry().Register(s); err != nil {
			t.Fatalf("register skill %s: %v", s.Name, err)
		}
	}
	return manager
}

// TestBuildToolPolicy_FromMatchedSkill 命中带工具约束的 skill → Allow/Deny 来自
// allowed/disallowed_tools，Skill 记录命中名，scope 原样叠加。
func TestBuildToolPolicy_FromMatchedSkill(t *testing.T) {
	manager := newTestSkillManager(t, &skills.Skill{
		Name:            "sales-report",
		Description:     "销售报表 汇总 生成",
		AllowedTools:    []string{"sql_execute", "data_analysis", "render_ui"},
		DisallowedTools: []string{"sql_mutate"},
	})

	// 名称精确/部分命中保证相关度过阈值
	policy := BuildToolPolicy(manager, "sales-report", infraagent.ScopeWrite)
	if policy.Skill != "sales-report" {
		t.Fatalf("应记录命中 skill, got %q", policy.Skill)
	}
	if len(policy.Allow) != 3 || policy.Allow[0] != "sql_execute" {
		t.Errorf("Allow 应来自 allowed_tools: %v", policy.Allow)
	}
	if len(policy.Deny) != 1 || policy.Deny[0] != "sql_mutate" {
		t.Errorf("Deny 应来自 disallowed_tools: %v", policy.Deny)
	}
	if policy.Scope != infraagent.ScopeWrite {
		t.Errorf("scope 应原样叠加: %s", policy.Scope)
	}

	// 策略语义抽查：skill 禁写 + 白名单外全拦
	if ok, reason := policy.Permits("sql_mutate"); ok || reason != infraagent.BlockReasonDisallowed {
		t.Errorf("命中 skill 的 disallowed 应拦截, ok=%v reason=%s", ok, reason)
	}
	if ok, reason := policy.Permits("etl_run"); ok || reason != infraagent.BlockReasonNotInAllowlist {
		t.Errorf("白名单外应拦截, ok=%v reason=%s", ok, reason)
	}
	if ok, _ := policy.Permits("sql_execute"); !ok {
		t.Error("白名单内且 scope 授予的工具应放行")
	}
}

// TestBuildToolPolicy_NoMatchScopeOnly 未命中 skill → scope-only 策略；
// 空 scope 按 read 最小权限兜底（只读会话拦写）。
func TestBuildToolPolicy_NoMatchScopeOnly(t *testing.T) {
	manager := newTestSkillManager(t)

	policy := BuildToolPolicy(manager, "上个月各区域 GMV 多少", "")
	if policy.Skill != "" || len(policy.Allow) != 0 || len(policy.Deny) != 0 {
		t.Fatalf("未命中 skill 应为 scope-only 策略: %+v", policy)
	}
	if policy.Scope != infraagent.ScopeRead {
		t.Fatalf("空 scope 必须按 read 最小权限兜底: %s", policy.Scope)
	}
	if ok, reason := policy.Permits("sql_mutate"); ok || reason != infraagent.BlockReasonScopeDenied {
		t.Errorf("只读兜底必须拦写, ok=%v reason=%s", ok, reason)
	}
	if ok, _ := policy.Permits("sql_execute"); !ok {
		t.Error("scope-only 策略应放行只读工具")
	}
}

// TestBuildToolPolicy_PureGuidanceSkillIgnored 纯指导性 skill（无工具约束）
// 即便命中也不产生 Allow/Deny，仍为 scope-only。
func TestBuildToolPolicy_PureGuidanceSkillIgnored(t *testing.T) {
	manager := newTestSkillManager(t, &skills.Skill{
		Name:        "writing-style",
		Description: "写作风格 指导",
	})

	policy := BuildToolPolicy(manager, "writing-style", infraagent.ScopeRead)
	if policy.Skill != "" || len(policy.Allow) != 0 || len(policy.Deny) != 0 {
		t.Errorf("纯指导性 skill 不应产生工具约束: %+v", policy)
	}
}

// TestBuildToolPolicy_LowRelevanceIgnored 相关度低于阈值的弱匹配不视为命中，
// 避免误挂无关 skill 的工具约束。
func TestBuildToolPolicy_LowRelevanceIgnored(t *testing.T) {
	manager := newTestSkillManager(t, &skills.Skill{
		Name:            "etl-pipeline",
		Description:     "管道 派生 清洗",
		AllowedTools:    []string{"etl_run"},
		DisallowedTools: []string{"sql_mutate"},
	})

	// 与该 skill 无词法交集的问题 → 不应挂其约束
	policy := BuildToolPolicy(manager, "昨天订单总数多少", infraagent.ScopeRead)
	if policy.Skill != "" || len(policy.Allow) != 0 {
		t.Errorf("弱相关 skill 不应命中: %+v", policy)
	}
}

// TestBuildToolPolicy_NilManager 无 skill 管理器 → scope-only 策略（防御路径）。
func TestBuildToolPolicy_NilManager(t *testing.T) {
	policy := BuildToolPolicy(nil, "任意问题", infraagent.ScopeETL)
	if policy.Scope != infraagent.ScopeETL || policy.Skill != "" {
		t.Errorf("nil 管理器应返回 scope-only 策略: %+v", policy)
	}
}
