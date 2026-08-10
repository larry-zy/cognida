// 由 eino_agent.go 拆出——同包、行为等价（M1 god-file 拆分）。
package framework

import (
	"context"

	"github.com/cloudwego/eino/schema"

	ctxeng "cognida/internal/service/agent/context"
)

// tokenCounter 返回本 agent 三级压缩统一使用的 token 计数器。未注入时降级为字符启发式
// ApproxTokenCounter，保证零配置也能工作（能力自足、不反向依赖基础设施层）。
func (a *agentImpl) tokenCounter() ctxeng.TokenCounter {
	if a.counter != nil {
		return a.counter
	}
	return ctxeng.ApproxTokenCounter{}
}

// normalizeContext 施加三级压缩之 ①单条消息 与 ③整段对话，就地治理运行时上下文。
// 每次 ReAct 生成前调用；返回治理后的消息切片（可能是新切片）。三个阈值都未配置时原样返回。
//
//   - ① a.maxMessageTokens>0：任一条消息 token 超限 → 压该条（复用 ctxeng.CapContentByTokens，保 result_id）。
//     系统提示 messages[0] 为 Pinned，永不裁。压缩时复制消息对象，绝不改写落库/历史引用的原对象。
//   - ③ a.compactTrigger/Target>0：累计 token ≥ trigger → 折叠最早的整轮为一条摘要系统消息，压回 target 附近。
//   - ③.5 a.reasoningEvictTokens>0：最近一轮之前累计 reasoning ≥ 该值 → 把旧轮整轮折走（陈旧思考随之离场），
//     只让最近一轮带 thinking；DeepSeek 契约禁止单删旧工具轮的 reasoning，故靠整轮折叠实现。
func (a *agentImpl) normalizeContext(ctx context.Context, messages []*schema.Message) []*schema.Message {
	if len(messages) == 0 {
		return messages
	}
	counter := a.tokenCounter()

	// ① 单条消息超限 → 就地压缩（保 result_id）。跳过 messages[0]（Pinned 系统提示）。
	if a.maxMessageTokens > 0 {
		for i := 1; i < len(messages); i++ {
			m := messages[i]
			if m == nil || m.Content == "" {
				continue
			}
			capped := ctxeng.CapContentByTokens(m.Content, a.maxMessageTokens, counter)
			if capped != m.Content {
				nm := *m // 复制，避免改写历史/落库引用的原消息对象
				nm.Content = capped
				messages[i] = &nm
			}
		}
	}

	// ③ 累计上下文超触发线 → 折叠最早整轮，压回目标水位（不停机）。
	if a.compactTrigger > 0 && a.compactTarget > 0 && totalMessageTokens(messages, counter) >= a.compactTrigger {
		messages = a.foldOlderMessages(ctx, messages, counter)
	}

	// ③.5 陈旧思考剥离：只让最近一轮带 reasoning，更早轮的思考不在上下文里累积。
	// 背景：DeepSeek V4 契约要求「带 tool_calls 的 assistant 轮，reasoning_content 必须原样回传后续所有轮，
	// 否则 400」，故不能「保留旧工具轮却单删其 reasoning」——唯一 400-安全的移除方式是把旧轮整轮折走
	// （思考随整轮离场），最近一轮逐字保留（本轮契约必须带）。攒到阈值才动手：平时由 ③ 的 Level A 观察屏蔽
	// 保住决策链，仅当「最近一轮之前」累计的思考 ≥ reasoningEvictTokens 才折，避免每步压摘要抹掉近期决策链。
	if a.reasoningEvictTokens > 0 && a.compactTarget > 0 {
		if cut := recentTailCut(messages, 0, counter); cut > 1 &&
			oldReasoningTokens(messages, cut, counter) >= a.reasoningEvictTokens {
			messages = a.summarizeOlder(ctx, messages, cut, counter)
		}
	}
	return messages
}

// oldReasoningTokens 估算 messages[1:cut) 里累计的 thinking（ReasoningContent）token——
// 即「最近一轮之前」被迫回传、正在上下文里堆积的陈旧思考重量，用于 ③.5 剥离触发判定。
// 只数 ReasoningContent（Content 的治理归 ①/③），系统提示 messages[0] 不计。
func oldReasoningTokens(messages []*schema.Message, cut int, counter ctxeng.TokenCounter) int {
	t := 0
	for i := 1; i < cut && i < len(messages); i++ {
		if m := messages[i]; m != nil && m.ReasoningContent != "" {
			t += counter.Count(m.ReasoningContent)
		}
	}
	return t
}

// totalMessageTokens 估算整段消息的累计上行 token（用于 ③触发判定）。
//
// 必须把 ReasoningContent（DeepSeek V4 thinking 链）一并计入：thinking 默认开，且其官方契约要求
// 「带工具调用的 assistant 轮，reasoning_content 必须原样回传到后续所有轮，否则 400」——即中间每一步的
// 思考都是我们被迫上传的真实 token。只数 Content 会系统性低估上行体积，令 128k 触发线偏晚。
// 而回收 reasoning 的唯一杠杆是 Level B 整轮折叠（移除整轮连思考一起走；观察屏蔽只碰 tool 观察、
// 碰不到 assistant 的 reasoning），故触发判定必须先「看见」这坨思考重量。
func totalMessageTokens(messages []*schema.Message, counter ctxeng.TokenCounter) int {
	t := 0
	for _, m := range messages {
		t += msgTokens(m, counter)
	}
	return t
}

// msgTokens 单条消息的上行 token = Content + ReasoningContent（thinking 链，见 totalMessageTokens 说明）
// + ToolCalls（工具调用意图）。tool_calls 的 Function.Name/Arguments 在 data agent 里常是整段 SQL 或
// 大 JSON，且与 reasoning 同受回传契约约束——带工具调用的 assistant 轮在整个 ReAct 运行内会被逐轮上传。
// 若只数 Content/ReasoningContent、漏掉 ToolCalls，就与漏 reasoning 属同一类系统性低估：上行体积被算小，
// 128k 折叠 / reasoning 剥离触发线偏晚。故一并计入其 Name + Arguments 的 token。
func msgTokens(m *schema.Message, counter ctxeng.TokenCounter) int {
	if m == nil {
		return 0
	}
	t := counter.Count(m.Content)
	if m.ReasoningContent != "" {
		t += counter.Count(m.ReasoningContent)
	}
	for _, tc := range m.ToolCalls {
		t += counter.Count(tc.Function.Name) + counter.Count(tc.Function.Arguments)
	}
	return t
}

// foldOlderMessages 就地把最早的对话压回 compactTarget 附近，采用研究验证更优的两级降级链：
//
//   - Level A（主策略）观察屏蔽：保护近期尾巴 messages[cut:] 逐字不动，把更早区间里体积最大、
//     最陈旧的「工具观察」替换为占位符（保 result_id），推理与工具调用意图（决策链）逐字保留。
//     依据 JetBrains 2025（SWE-bench）：保推理、屏观察，稳定性常追平甚至超过整段重摘要，且零失真。
//     屏蔽后已回到触发线下即返回——**优先保住完整决策链，不做过度压缩**。
//   - Level B（兜底）整轮摘要：屏蔽后仍超触发线（如推理链本身极长/轮数极多）→ 把最早的整块
//     抽取式摘要成一条系统消息，压得更狠。
//
// 核心不变量（两级共用）：按「整轮」处理——近期尾巴的起点 cut 必须落在「轮首」（非 tool 消息），
// 绝不从孤立的 tool 消息起头，否则 assistant.tool_calls ↔ tool 的 1:1 配对被破坏，下一轮生成
// 会被 LLM API 以 400 拒绝。messages[0]（系统提示）始终 Pinned。
func (a *agentImpl) foldOlderMessages(ctx context.Context, messages []*schema.Message, counter ctxeng.TokenCounter) []*schema.Message {
	if len(messages) <= 2 {
		return messages // 仅系统提示 + 一条，无可折叠
	}
	cut := recentTailCut(messages, a.compactTarget, counter)
	if cut <= 1 {
		return messages // 无早期块可折叠（正文整体已在目标内，超线来自 Pinned 系统提示）
	}

	// —— Level A：观察屏蔽（主策略）——
	masked := maskOldObservations(messages, cut, counter)
	if totalMessageTokens(masked, counter) < a.compactTrigger {
		return masked // 屏蔽已足够压回触发线下：保住完整决策链，不再摘要
	}

	// —— Level B：整轮语义摘要（兜底）——
	// 在已屏蔽的消息上做，摘要更紧凑；result_id 已随屏蔽占位符保留，摘要仍能收集到。
	return a.summarizeOlder(ctx, masked, cut, counter)
}

// recentTailCut 从尾部向前累计 tail token，返回近期尾巴的起点 cut：逐字保留 messages[cut:]，
// 处理 messages[1:cut)。首选「使 tail<=target 的最靠前轮首」（尽量多保留近期原文）；
// 兜底「最近的轮首」（tail 最小，用于连最近一轮都超 target 的极端情形）。cut<=1 表示无早期块可处理。
func recentTailCut(messages []*schema.Message, target int, counter ctxeng.TokenCounter) int {
	bestCut, fallbackCut, tailTok := -1, -1, 0
	for i := len(messages) - 1; i >= 1; i-- {
		tailTok += msgTokens(messages[i], counter) // 含 reasoning：被迫保留的思考也占尾巴预算
		if messages[i].Role == schema.Tool {
			continue // 不能在 tool 处切（会成孤儿 tool）
		}
		if fallbackCut == -1 {
			fallbackCut = i // 第一个遇到的轮首=最近的轮首
		}
		if tailTok <= target {
			bestCut = i
		} else if bestCut != -1 {
			break // 已有可行切点，再往前 tail 只增不减
		}
	}
	if bestCut != -1 {
		return bestCut
	}
	return fallbackCut
}

// maskOldObservations 观察屏蔽（Level A）：近期尾巴 messages[cut:] 逐字不动，把更早区间
// messages[1:cut) 内的「工具观察」替换为占位符（保 result_id），推理/动作(assistant/user)逐字保留。
// 保持全部消息在位、仅缩小 tool 内容 → assistant.tool_calls ↔ tool 配对天然不破坏。
// 复制被改消息对象，绝不改写落库/历史引用的原对象；返回新切片（浅拷贝头部）。
func maskOldObservations(messages []*schema.Message, cut int, counter ctxeng.TokenCounter) []*schema.Message {
	out := make([]*schema.Message, len(messages))
	copy(out, messages)
	for i := 1; i < cut; i++ {
		m := out[i]
		if m == nil || m.Role != schema.Tool || m.Content == "" {
			continue
		}
		if masked := ctxeng.MaskObservation(m.Content, counter); masked != m.Content {
			nm := *m // 复制，避免改写历史/落库引用的原消息对象
			nm.Content = masked
			out[i] = &nm
		}
	}
	return out
}

// summarizeOlder 整轮摘要（Level B 兜底）：把 messages[1:cut) 摘要成一条系统消息置于 index 1，
// 保住其中的 result_id（data-by-reference 硬不变量），逐字保留近期尾巴 messages[cut:]。
// 摘要作为独立系统消息，下次再折叠时会作为最早轮被并入新摘要（摘要之摘要），自然收敛不无限膨胀。
//
// 摘要器优先用注入的 a.summarizer（LLM 语义摘要，保真更高），未注入时用确定性抽取式摘要兜底；
// LLM 摘要器自身也内建抽取式降级，故任何 LLM 慢/失败都不会破坏折叠与主循环。
func (a *agentImpl) summarizeOlder(ctx context.Context, messages []*schema.Message, cut int, counter ctxeng.TokenCounter) []*schema.Message {
	older := messages[1:cut]
	turns := make([]ctxeng.Turn, 0, len(older))
	for _, m := range older {
		turns = append(turns, ctxeng.Turn{Role: string(m.Role), Content: m.Content})
	}
	preserved := ctxeng.CollectResultIDs(turns)
	var summarizer ctxeng.Summarizer = ctxeng.ExtractiveSummarizer{}
	if a.summarizer != nil {
		summarizer = a.summarizer
	}
	summary, err := summarizer.Summarize(ctx, turns, preserved)
	if err != nil {
		return messages // 摘要失败宁可不折叠，也不破坏配对
	}
	summary = ctxeng.EnsureResultIDsPresent(summary, preserved)
	summaryMsg := schema.SystemMessage("【历史已压缩，完整数据见对应 result_id】\n" + summary)

	folded := make([]*schema.Message, 0, 2+len(messages)-cut)
	folded = append(folded, messages[0], summaryMsg)
	folded = append(folded, messages[cut:]...)
	return folded
}
