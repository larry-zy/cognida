// 硬工具门策略装配测试（Phase 6 任务 7.3/7.5；Bug A 修复后调整为「显式激活」模型）：
// 入口 BuildToolPolicy 只定会话 scope（read/write/etl），不再据词法预匹配自动装配
// 技能白名单；skill 的 allowed/disallowed_tools 改由 LLM 显式 skill_invoke 时经
// framework.ActivateSkillPolicy 激活（在 framework 包内测其语义与元工具豁免）。
package dataagent

import (
	"testing"

	infraagent "link/internal/service/agent/framework"
)

// TestBuildToolPolicy_ScopeOnly 入口只装配 scope-only 策略：不含 Allow/Deny/Skill，
// 无论问题文本如何都不据词法预匹配收窄工具面（修复 Bug A：弱相关技能误挂窄白名单锁死工具）。
func TestBuildToolPolicy_ScopeOnly(t *testing.T) {
	// 曾会误命中 report-composition / doc-qa 等技能的问题文本，如今一律 scope-only。
	for _, msg := range []string{
		"对比 6 月和 7 月已完成订单的 GMV 环比变化",
		"ecommerce_demo 里有哪些跟订单相关的表",
		"上个月各区域 GMV 多少",
	} {
		policy := BuildToolPolicy(infraagent.ScopeWrite)
		if policy.Skill != "" || len(policy.Allow) != 0 || len(policy.Deny) != 0 {
			t.Errorf("msg=%q 入口应为 scope-only（无 Allow/Deny/Skill）: %+v", msg, policy)
		}
		if policy.Scope != infraagent.ScopeWrite {
			t.Errorf("msg=%q scope 应原样装配: %s", msg, policy.Scope)
		}
		// scope-only 下只读工具放行、render_ui 不再被误挂窄白名单拦掉。
		if ok, _ := policy.Permits("get_schema"); !ok {
			t.Errorf("msg=%q scope-only 应放行只读工具 get_schema", msg)
		}
		if ok, _ := policy.Permits("render_ui"); !ok {
			t.Errorf("msg=%q scope-only 应放行 render_ui（不再被误挂窄白名单拦掉）", msg)
		}
	}
}

// TestBuildToolPolicy_EmptyScopeReadFallback 空 scope 按 read 最小权限兜底：拦写、放读。
func TestBuildToolPolicy_EmptyScopeReadFallback(t *testing.T) {
	policy := BuildToolPolicy("")
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
