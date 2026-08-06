// 硬工具门策略装配（Phase 6 任务 7.3；Bug A 修复后调整为「显式激活」模型）：
// 入口只装配会话 scope（read/write/etl）注入 context；skill 的 allowed/disallowed_tools
// 不再由入口词法预匹配自动装成硬白名单——那会把弱相关技能（如对数据问题误命中 doc-qa）
// 的窄工具面强加给整轮，导致 get_schema/sql_execute/render_ui 被误拦、且无从自救。
// 工具约束改为显式激活：仅当 LLM 主动调用 skill_invoke 聚焦某技能时，才经
// framework.ActivateSkillPolicy 收窄工具面（导航元工具永远豁免，保证可再切换）。
// 后续 ReAct 循环内每次工具执行前仍经 framework.gateToolCall 硬拦截（scope + 已激活的白/黑名单）。
package dataagent

import (
	"context"

	domainagent "link/internal/model/agent"
	infraagent "link/internal/service/agent/framework"
	"link/internal/service/agent/skills"
)

// BuildToolPolicy 装配本次请求的 scope-only 工具策略：
//   - scope 空串按 read 最小权限兜底（授权来自入口，未授权即只读）；
//   - 不预置 Allow/Deny：技能工具约束改由 LLM 显式 skill_invoke 时激活（见包注释）。
//
// 仍注入非 nil 策略：一方面 scope 硬门始终生效，另一方面为后续 skill_invoke 的
// 就地激活（framework.ActivateSkillPolicy 依赖 ctx 中已有 *ToolPolicy 指针）留好挂载点。
func BuildToolPolicy(scope string) *infraagent.ToolPolicy {
	if scope == "" {
		scope = infraagent.ScopeRead
	}
	return &infraagent.ToolPolicy{Scope: scope}
}

// toolPolicyHook 返回硬工具门 BeforeHook：装配 scope-only 策略注入 ctx；并在极高置信
// （FallbackInjectThreshold）时把命中 skill 暂存到 ctx，供末位 InjectFromContextHook 兜底注入。
//
// 说明：此处只定 scope，不再据词法预匹配装配工具白名单（避免误挂无关技能锁死工具）；
// 技能选择主要交给 LLM（system prompt 技能目录 + skill_invoke 工具），工具约束由
// skill_invoke 显式激活。匹配只用于「是否自动注入指导内容」，基于原始消息、不受 playbook 改写污染。
func toolPolicyHook() infraagent.BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		scope := domainagent.MustGetToolScope(ctx)
		policy := BuildToolPolicy(scope)
		ctx = infraagent.WithToolPolicy(ctx, policy)
		if skill, ok := skills.MatchTopSkill(message, skills.FallbackInjectThreshold); ok {
			ctx = skills.ContextWithMatchedSkill(ctx, skill)
		}
		return ctx, message, nil
	}
}
