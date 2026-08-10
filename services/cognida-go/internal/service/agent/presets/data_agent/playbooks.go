package dataagent

import prompts "cognida/internal/prompt"

// systemPrompt 是 Data Agent 单一 ReAct 内核的系统提示词：声明四类能力、result_id
// 传递契约、循环上限意识与澄清纪律。具体意图的编排偏好由 playbook 在入口注入（见 routing.go）。
// 正文集中于 internal/prompt/templates/data_agent.yaml。
var systemPrompt = prompts.MustGet("data_agent", "system")

// 意图 playbook：在入口按分类结果注入到用户消息之前，引导 ReAct 循环选择工具子集与编排。
// 正文集中于 internal/prompt/templates/data_agent.yaml。
var (
	playbookFetch       = prompts.MustGet("data_agent", "playbook_fetch")
	playbookTrend       = prompts.MustGet("data_agent", "playbook_trend")
	playbookAttribution = prompts.MustGet("data_agent", "playbook_attribution")
	playbookReport      = prompts.MustGet("data_agent", "playbook_report")
	playbookGeneral     = prompts.MustGet("data_agent", "playbook_general")
	playbookAmbiguous   = prompts.MustGet("data_agent", "playbook_ambiguous")
)

// playbookFor 返回意图对应的 playbook 文本。
func playbookFor(intent Intent) string {
	switch intent {
	case IntentFetch:
		return playbookFetch
	case IntentTrend:
		return playbookTrend
	case IntentAttribution:
		return playbookAttribution
	case IntentReport:
		return playbookReport
	case IntentAmbiguous:
		return playbookAmbiguous
	default:
		return playbookGeneral
	}
}
