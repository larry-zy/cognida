// 硬工具门策略装配（Phase 6 任务 7.3）：入口按用户问题匹配 skill，把命中 skill 的
// allowed_tools/disallowed_tools 装配为 ToolPolicy 的 Allow/Deny，叠加会话 scope
// （read/write/etl）注入 context；后续 ReAct 循环内每次工具执行前经
// framework.gateToolCall 硬拦截。skill 策略与 scope 同为放行的必要条件。
package dataagent

import (
	"context"

	domainagent "link/internal/model/agent"
	infraagent "link/internal/service/agent/framework"
	"link/internal/service/agent/skills"
)

// skillPolicyRelevanceThreshold 命中判定阈值：词法相关度低于该值的 skill 不视为
// 命中（描述全词命中约得 0.5，弱部分匹配低于该值），避免误挂无关 skill 的工具约束。
const skillPolicyRelevanceThreshold = 0.5

// BuildToolPolicy 装配本次请求的工具策略：
//   - scope 空串按 read 最小权限兜底（授权来自入口，未授权即只读）；
//   - 命中 skill（相关度达阈值且带工具约束）时以其 allowed/disallowed_tools
//     构建白名单/黑名单；纯指导性 skill 或未命中时为 scope-only 策略。
func BuildToolPolicy(manager skills.SkillManager, message, scope string) *infraagent.ToolPolicy {
	if scope == "" {
		scope = infraagent.ScopeRead
	}
	policy := &infraagent.ToolPolicy{Scope: scope}
	if manager == nil {
		return policy
	}

	matches := manager.MatchSkillsForTask(message, 1)
	if len(matches) == 0 {
		return policy
	}
	top := matches[0]
	if top.Skill == nil || top.Relevance < skillPolicyRelevanceThreshold || top.Skill.IsPureGuidance() {
		return policy
	}

	policy.Skill = top.Skill.Name
	policy.Allow = append([]string(nil), top.Skill.AllowedTools...)
	policy.Deny = append([]string(nil), top.Skill.DisallowedTools...)
	return policy
}

// toolPolicyHook 返回硬工具门 BeforeHook：以原始用户消息匹配 skill（须挂在
// intentRoutingHook 之前，避免 playbook 注入文本干扰匹配），装配策略后注入 ctx。
//
// 工具门策略（allowed/disallowed）沿用 skillPolicyRelevanceThreshold(0.5) 判定命中；
// 而"是否自动注入指导内容"改走渐进式披露（方案 A）：技能选择主要交给 LLM（system prompt
// 技能目录 + skill_invoke 工具），故仅在极高置信（FallbackInjectThreshold）时才把命中 skill
// 暂存到 ctx，供末位 InjectFromContextHook 兜底注入。匹配只发生一次且基于原始消息，
// 不受后续 playbook 改写污染。
func toolPolicyHook() infraagent.BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		scope := domainagent.MustGetToolScope(ctx)
		policy := BuildToolPolicy(skills.GetGlobalManager(), message, scope)
		ctx = infraagent.WithToolPolicy(ctx, policy)
		if skill, ok := skills.MatchTopSkill(message, skills.FallbackInjectThreshold); ok {
			ctx = skills.ContextWithMatchedSkill(ctx, skill)
		}
		return ctx, message, nil
	}
}
