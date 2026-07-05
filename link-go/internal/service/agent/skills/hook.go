// Package skills：Skill 自动注入 Hook。
//
// 把「已加载 Skill 的指导内容」在 Agent 处理消息前自动注入到用户消息之前，使 LLM 无需
// 主动调用 skill_invoke 即可遵循命中 Skill 的方法论——即「skillTool 自动注入」。所有 Agent
// （默认 / RAG / Chat / Text2SQL / Data）复用同一套 Hook，保证一致接入。
package skills

import (
	"context"

	"link/internal/service/agent/framework"
)

// DefaultInjectRelevanceThreshold 自动注入的命中阈值：低于该相关度的 Skill 不注入，
// 与 Data Agent 硬工具门阈值一致（见 data_agent.tool_gate），避免误注入无关指导。
const DefaultInjectRelevanceThreshold = 0.5

// FallbackInjectThreshold 渐进式披露（方案 A）下的兜底注入阈值：技能选择主要交给
// LLM（system prompt 内的技能目录 + skill_invoke 工具），词法自动注入仅在"极高置信"的
// 命中时兜底触发，作为确定性保障。取值高于 DefaultInjectRelevanceThreshold，只放行铁板钉钉的匹配。
const FallbackInjectThreshold = 0.75

// MatchTopSkill 返回相关度达到阈值的最相关已加载 Skill。
// 无已加载 Skill、或最高相关度低于阈值时返回 (nil, false)。
func MatchTopSkill(message string, threshold float64) (*Skill, bool) {
	if threshold <= 0 {
		threshold = DefaultInjectRelevanceThreshold
	}
	matches := GetGlobalManager().MatchSkillsForTask(message, 1)
	if len(matches) == 0 {
		return nil, false
	}
	top := matches[0]
	if top.Skill == nil || top.Relevance < threshold {
		return nil, false
	}
	return top.Skill, true
}

// injectGuidance 把 Skill 指导内容包裹后前置到消息之前，保留原始用户问题。
func injectGuidance(ctx context.Context, skill *Skill, message string) string {
	integration := NewSkillAgentIntegration(WithIncludeReason(false), WithMaxContentLength(4000))
	sc, err := integration.FormatForAgent(ctx, skill)
	if err != nil || sc == nil {
		return message
	}
	return "--- 已激活技能指导（自动注入，请据此完成任务）---\n\n" +
		sc.FormattedContent +
		"\n\n--- 技能指导结束 ---\n\n【用户问题】\n" + message
}

// AutoInjectHook 返回一个 BeforeHook：以原始用户消息匹配最相关 Skill，命中（相关度达阈值）
// 时把其指导内容自动注入到消息之前；未命中则原样透传。
//
// 供无意图路由的简单 Agent（默认 / RAG / Chat / Text2SQL）直接挂载——一步完成匹配 + 注入。
// Data Agent 因入口另有意图分类改写消息，改用 InjectFromContextHook（见下）避免重复匹配污染文本。
func AutoInjectHook(threshold float64) framework.BeforeHook {
	if threshold <= 0 {
		threshold = DefaultInjectRelevanceThreshold
	}
	return func(ctx context.Context, message string) (context.Context, string, error) {
		skill, ok := MatchTopSkill(message, threshold)
		if !ok {
			return ctx, message, nil
		}
		ctx = ContextWithMatchedSkill(ctx, skill)
		return ctx, injectGuidance(ctx, skill, message), nil
	}
}

// InjectFromContextHook 返回一个 BeforeHook：读取由更早 Hook 在 ctx 中暂存的命中 Skill
// （见 ContextWithMatchedSkill），把其指导内容注入到（可能已被意图路由改写的）消息之前。
//
// 专供 Data Agent：入口先由 toolPolicyHook 以「原始消息」匹配并暂存 Skill，再由意图路由
// 改写消息，最后本 Hook 从 ctx 取回 Skill 注入指导——匹配只发生一次、且基于原始消息，
// 不受 playbook 注入文本干扰。ctx 中无暂存 Skill 时原样透传。
func InjectFromContextHook() framework.BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		skill, ok := MatchedSkillFromContext(ctx)
		if !ok || skill == nil {
			return ctx, message, nil
		}
		return ctx, injectGuidance(ctx, skill, message), nil
	}
}

// ========================================
// 命中 Skill 的 ctx 传递（供匹配与注入解耦）
// ========================================

type matchedSkillCtxKey struct{}

// ContextWithMatchedSkill 把命中 Skill 暂存到 ctx，供后续 InjectFromContextHook 取用。
func ContextWithMatchedSkill(ctx context.Context, skill *Skill) context.Context {
	if skill == nil {
		return ctx
	}
	return context.WithValue(ctx, matchedSkillCtxKey{}, skill)
}

// MatchedSkillFromContext 从 ctx 取回命中 Skill；不存在时返回 (nil, false)。
func MatchedSkillFromContext(ctx context.Context) (*Skill, bool) {
	skill, ok := ctx.Value(matchedSkillCtxKey{}).(*Skill)
	return skill, ok && skill != nil
}
