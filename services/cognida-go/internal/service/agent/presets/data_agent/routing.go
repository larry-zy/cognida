// Package dataagent 提供单一 ReAct 内核的 Data Agent 预设。
//
// 与旧的 Text2SQL PER（Plan-Execute-Reflect）流水线不同，本预设以「单一 ReAct 循环」
// 承载查/析/渲/操四类能力：由 LLM 自主决定下一步动作与工具顺序，循环受 maxIter 与
// token 预算共同约束（见 framework 统一主干的 execLoop）。入口先做**意图分类路由**，为不同
// 意图注入对应 playbook 引导，歧义时引导反问而非硬猜。
//
// 意图分类以 LLM 为主（ClassifyIntentLLM，理解同义/口语/长句），词法规则
// （ClassifyIntent）为兜底：LLM 不可用、超时或输出非法时自动回退，保证路由始终可用。
package dataagent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	infraagent "cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/skills"
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

// allIntents 是全部可判定意图，供 LLM 输出解析按此集合校验。
var allIntents = []Intent{
	IntentAttribution, IntentTrend, IntentReport, IntentFetch, IntentAmbiguous, IntentGeneral,
}

// decisionFor 由意图构造路由判定（统一 playbook 与 ambiguous 标注，供 LLM/词法两条路径复用）。
func decisionFor(intent Intent) RouteDecision {
	return RouteDecision{
		Intent:    intent,
		Playbook:  playbookFor(intent),
		Ambiguous: intent == IntentAmbiguous,
	}
}

// 各意图的关键词信号（词法兜底：LLM 分类不可用时按确定性规则判定，亦保证纯函数可测）。
var intentSignals = []struct {
	intent   Intent
	keywords []string
}{
	{IntentAttribution, []string{"为什么", "原因", "归因", "根因", "驱动", "下钻", "拆解", "贡献", "influence", "why", "root cause", "driver"}},
	{IntentTrend, []string{"趋势", "增长", "下降", "变化", "环比", "同比", "走势", "波动", "时间序列", "trend", "growth", "over time"}},
	{IntentReport, []string{"报告", "看板", "周报", "月报", "日报", "综合", "解读", "概览", "报表", "dashboard", "report", "summary"}},
	{IntentFetch, []string{"查询", "取数", "查一下", "查下", "多少", "列出", "明细", "统计", "总数", "排名", "top", "查看", "select", "count", "list"}},
}

// ClassifyIntent 以词法启发式对用户问题做意图分类（LLM 分类的兜底路径）。
// 纯函数、无副作用，便于单测；返回对应 playbook 与是否需要澄清。
func ClassifyIntent(message string) RouteDecision {
	trimmed := strings.TrimSpace(message)
	lower := strings.ToLower(trimmed)

	// 过短或纯问候/含糊 → 歧义，引导反问。
	if isAmbiguous(trimmed, lower) {
		return decisionFor(IntentAmbiguous)
	}

	// 按优先级（归因 > 趋势 > 报告 > 取数）匹配首个命中信号。
	for _, sig := range intentSignals {
		for _, kw := range sig.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return decisionFor(sig.intent)
			}
		}
	}

	// 有实质内容但无明确细分信号 → 通用数据编排。
	return decisionFor(IntentGeneral)
}

// intentClassifierSystemPrompt 是 LLM 意图分类器的系统提示：把问题分到 6 类之一，
// 只回一个类别词，便于确定性解析。定义与词法信号对齐，保证两条路径语义一致。
const intentClassifierSystemPrompt = `你是数据分析对话的意图分类器。请把用户问题分到且仅分到以下 6 类之一：

- fetch：直接取数、查询明细、汇总、计数、排名（如「查一下」「多少」「列出」「top」）。
- trend：随时间的变化、增长/下降、环比/同比、走势、时间序列。
- attribution：追问「为什么变化」、原因、根因、驱动因子、贡献拆解、下钻。
- report：多指标综合、看板、周报/月报/日报、报表、概览、解读。
- ambiguous：信号不足、纯问候、过短含糊，无法判断真实数据诉求。
- general：有明确数据诉求但不属于以上任何细分。

严格只输出一个英文类别词本身（fetch / trend / attribution / report / ambiguous / general），
不要输出解释、标点、引号或任何多余文字。`

// ClassifyIntentLLM 用 LLM 对用户问题做意图分类。
// 成功且输出可解析时返回 (决定, true)；模型缺失/为空/调用出错/输出非法时返回 (零值, false)，
// 由调用方回退到 ClassifyIntent 词法兜底。
func ClassifyIntentLLM(ctx context.Context, m model.BaseChatModel, message string) (RouteDecision, bool) {
	trimmed := strings.TrimSpace(message)
	if m == nil || trimmed == "" {
		return RouteDecision{}, false
	}

	messages := []*schema.Message{
		schema.SystemMessage(intentClassifierSystemPrompt),
		schema.UserMessage(trimmed),
	}
	resp, err := m.Generate(ctx, messages)
	if err != nil || resp == nil {
		return RouteDecision{}, false
	}

	intent, ok := parseIntent(resp.Content)
	if !ok {
		return RouteDecision{}, false
	}
	return decisionFor(intent), true
}

// parseIntent 从 LLM 输出中解析意图词：精确匹配优先，其次容忍前后缀的包含匹配。
func parseIntent(content string) (Intent, bool) {
	s := strings.ToLower(strings.TrimSpace(content))
	if s == "" {
		return "", false
	}
	for _, in := range allIntents {
		if s == string(in) {
			return in, true
		}
	}
	// 容错：模型偶尔带前后缀（如「intent: trend」「归因(attribution)」）。
	for _, in := range allIntents {
		if strings.Contains(s, string(in)) {
			return in, true
		}
	}
	return "", false
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

// SkillFor 返回意图确定性绑定的领域技能名（无绑定则空串）。
// 趋势/归因/报告三类有专门方法论 skill，路由时直接把该 skill 存入 ctx，让 skill 注入
// 走单一注入路径（覆盖词法兜底的自动命中），避免「意图 playbook + 词法技能」双重注入打架。
// 取数/通用/歧义无绑定技能：取数可由内核按需 skill_invoke，通用/歧义不需要方法论引导。
func SkillFor(intent Intent) string {
	switch intent {
	case IntentTrend:
		return "trend-analysis"
	case IntentAttribution:
		return "attribution-analysis"
	case IntentReport:
		return "report-composition"
	default:
		return ""
	}
}

// intentRoutingHook 返回意图路由 BeforeHook：分类后把对应 playbook 注入到消息之前，
// 引导单一 ReAct 循环按该意图选择工具子集与编排顺序；歧义时注入反问引导。
// 命中有专门技能的意图时，把该 skill 确定性存入 ctx（覆盖 toolPolicyHook 的词法兜底命中），
// 由后续 InjectFromContextHook 走单一路径注入技能方法论。
//
// 分类以 LLM 为主（传入的分类模型），失败时回退词法兜底 ClassifyIntent；m 为 nil
// 时直接走词法（便于单测与未配置模型的降级运行）。
func intentRoutingHook(m model.BaseChatModel) infraagent.BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		decision, ok := ClassifyIntentLLM(ctx, m, message)
		if !ok {
			decision = ClassifyIntent(message)
		}
		if name := SkillFor(decision.Intent); name != "" {
			if skill, err := skills.FindSkill(name); err == nil {
				ctx = skills.ContextWithMatchedSkill(ctx, skill)
			}
		}
		routed := decision.Playbook + "\n\n【用户问题】\n" + message
		return ctx, routed, nil
	}
}
