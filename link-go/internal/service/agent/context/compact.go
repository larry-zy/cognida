package context

import "strings"

// 本文件是「上下文工程能力层」的单条压缩原语，配合 window.go 的窗口/摘要一起，
// 供业务层（convcontext 开场装配、framework 运行时折叠）复用：
//   - CapContentByTokens：把单条内容压到 token 上限内，且保住其中被引用的 result_id。
//   - MaskObservation：把一条陈旧的工具观察屏蔽成极短占位符，保住 result_id 供按引用取回。
//   - CollectResultIDs：导出 collectResultIDs，供框架层从任意轮次汇总 result_id。
// 纯函数、不依赖基础设施层，阈值全部由调用方（业务策略）传入。

// ObservationMaskMarker 观察屏蔽占位符的稳定前缀，也用作幂等判定（已屏蔽的观察不再重复屏蔽）。
const ObservationMaskMarker = "【工具观察已折叠"

// CapContentByTokens 把单条内容 content 压到不超过 maxTokens 个 token，
// 同时保住其中被引用的 result_id——data-by-reference 是硬不变量，其优先级高于软 token 上限，
// 因此补挂 result_id 可能令结果略超 maxTokens（有界溢出，与分层预算治理的取舍一致）。
//
// maxTokens<=0、无计数器、内容为空或已在限内时原样返回（零开销）。
// 机制：按 token 对 rune 前缀二分截断（复用 truncateToTokens），再把截断丢掉的 result_id 兜回末尾。
func CapContentByTokens(content string, maxTokens int, counter TokenCounter) string {
	if maxTokens <= 0 || counter == nil || content == "" {
		return content
	}
	if counter.Count(content) <= maxTokens {
		return content
	}
	ids := collectResultIDs([]Turn{{Content: content}})
	truncated := truncateToTokens(content, maxTokens, counter)
	return EnsureResultIDsPresent(truncated, ids)
}

// MaskObservation 观察屏蔽：把一条（通常体积很大且已陈旧的）工具观察压成极短占位符，
// 但保住其中被引用的 result_id，供后续按引用取回完整结果集。
//
// 依据：JetBrains 2025（SWE-bench 250 轮）实测——保留推理/动作、仅屏蔽旧工具观察，
// 稳定性常追平甚至超过对整段重新摘要，且零摘要失真、成本更低。故运行时折叠优先用它腾出
// 体积最大的陈旧观察，而完整保留推理与工具调用意图（决策链）。
//
// 幂等：已屏蔽（前缀命中 ObservationMaskMarker）的内容原样返回；占位符不比原文短时也原样返回
// （不做负优化，例如本就极短的观察）。空内容或无计数器时原样返回。
func MaskObservation(content string, counter TokenCounter) string {
	if content == "" || counter == nil {
		return content
	}
	if strings.HasPrefix(content, ObservationMaskMarker) {
		return content // 幂等：已屏蔽
	}
	ids := collectResultIDs([]Turn{{Content: content}})
	placeholder := ObservationMaskMarker + "，完整数据见 result_id】"
	if len(ids) > 0 {
		placeholder = ObservationMaskMarker + "，完整数据见 result_id：" + strings.Join(ids, ", ") + "】"
	}
	// 占位符不短于原文则不屏蔽（避免负优化）。
	if counter.Count(placeholder) >= counter.Count(content) {
		return content
	}
	return placeholder
}

// CollectResultIDs 导出 collectResultIDs：汇总若干轮中出现的 result_id（显式字段 + 文本内嵌），
// 去重且稳定排序。供框架层在按整轮折叠消息时，从被折叠的早期块收集需保真的引用。
func CollectResultIDs(turns []Turn) []string {
	return collectResultIDs(turns)
}
