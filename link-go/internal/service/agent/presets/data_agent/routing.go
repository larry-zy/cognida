// Package dataagent 提供单一 ReAct 内核的 Data Agent 预设。
//
// 与旧的 Text2SQL PER（Plan-Execute-Reflect）流水线不同，本预设以「单一 ReAct 循环」
// 承载查/析/渲/操四类能力：由 LLM 自主决定下一步动作与工具顺序，循环受 maxIter 与
// token 预算共同约束（见 framework.chatWithTools）。入口先做**意图分类路由**，为不同
// 意图注入对应 playbook 引导，歧义时引导反问而非硬猜。
package dataagent

import (
	"context"
	"strings"

	infraagent "link/internal/service/agent/framework"
)

// Intent 表示入口意图分类结果。
type Intent string

const (
	// IntentFetch 取数：直接查询/明细/汇总。
	IntentFetch Intent = "fetch"
	// IntentTrend 趋势：随时间的变化、增长、环比/同比。
	IntentTrend Intent = "trend"
	// IntentAttribution 归因：为什么变化、根因、驱动因子、下钻。
	IntentAttribution Intent = "attribution"
	// IntentReport 报告：多指标综合、看板、周报/月报、解读。
	IntentReport Intent = "report"
	// IntentAmbiguous 歧义：信号不足、过短或含糊，需反问澄清。
	IntentAmbiguous Intent = "ambiguous"
	// IntentGeneral 通用：有明确数据诉求但不属于上述细分，走通用取数-分析编排。
	IntentGeneral Intent = "general"
)

// RouteDecision 是意图路由的判定结果。
type RouteDecision struct {
	Intent    Intent // 命中意图
	Playbook  string // 注入到消息前的 playbook 引导
	Ambiguous bool   // 是否需要反问澄清
}

// 各意图的关键词信号（词法启发式；语义分类为后续升级，暂以确定性规则保证可测）。
var intentSignals = []struct {
	intent   Intent
	keywords []string
}{
	{IntentAttribution, []string{"为什么", "原因", "归因", "根因", "驱动", "下钻", "拆解", "贡献", "influence", "why", "root cause", "driver"}},
	{IntentTrend, []string{"趋势", "增长", "下降", "变化", "环比", "同比", "走势", "波动", "时间序列", "trend", "growth", "over time"}},
	{IntentReport, []string{"报告", "看板", "周报", "月报", "日报", "综合", "解读", "概览", "报表", "dashboard", "report", "summary"}},
	{IntentFetch, []string{"查询", "取数", "查一下", "查下", "多少", "列出", "明细", "统计", "总数", "排名", "top", "查看", "select", "count", "list"}},
}

// ClassifyIntent 以词法启发式对用户问题做意图分类。
// 纯函数、无副作用，便于单测；返回对应 playbook 与是否需要澄清。
func ClassifyIntent(message string) RouteDecision {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)

	// 过短或纯问候/含糊 → 歧义，引导反问。
	if isAmbiguous(trimmed, lower) {
		return RouteDecision{Intent: IntentAmbiguous, Ambiguous: true, Playbook: playbookAmbiguous}
	}

	// 按优先级（归因 > 趋势 > 报告 > 取数）匹配首个命中信号。
	for _, sig := range intentSignals {
		for _, kw := range sig.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return RouteDecision{Intent: sig.intent, Playbook: playbookFor(sig.intent)}
			}
		}
	}

	// 有实质内容但无明确细分信号 → 通用数据编排。
	return RouteDecision{Intent: IntentGeneral, Playbook: playbookFor(IntentGeneral)}
}

// 纯问候/过短含糊语判定：无数据诉求信号且长度过短，视为歧义。
var greetings = []string{"你好", "您好", "hi", "hello", "在吗", "hey", "早上好", "晚上好"}

func isAmbiguous(trimmed, lower string) bool {
	if trimmed == "" {
		return true
	}
	for _, g := range greetings {
		if lower == g || trimmed == g {
			return true
		}
	}
	// 极短且不含任何数据/指标线索的输入（如「数据」「帮忙」）视为歧义。
	runes := []rune(trimmed)
	if len(runes) <= 3 && !strings.ContainsAny(lower, "0123456789") {
		hasSignal := false
		for _, sig := range intentSignals {
			for _, kw := range sig.keywords {
				if strings.Contains(lower, strings.ToLower(kw)) {
					hasSignal = true
					break
				}
			}
		}
		if !hasSignal {
			return true
		}
	}
	return false
}

// CapabilityFor 返回意图对应的 data_analysis 命名能力（analysis_type）。
// 意图 → 能力的显式映射使「路由驱动分析编排」可独立测试；
// 取数/通用/歧义无预设分析能力，返回空串（由 ReAct 循环按需决定）。
func CapabilityFor(intent Intent) string {
	switch intent {
	case IntentTrend:
		return "trend"
	case IntentAttribution:
		return "attribution"
	case IntentReport:
		return "report"
	default:
		return ""
	}
}

// intentRoutingHook 返回意图路由 BeforeHook：分类后把对应 playbook 注入到消息之前，
// 引导单一 ReAct 循环按该意图选择工具子集与编排顺序；歧义时注入反问引导。
func intentRoutingHook() infraagent.BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		decision := ClassifyIntent(message)
		routed := decision.Playbook + "\n\n【用户问题】\n" + message
		return ctx, routed, nil
	}
}
