// 由 eino_agent.go 拆出——同包、行为等价（M1 god-file 拆分）。
package framework

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ========================================
// ToolLoop：ReAct 执行循环（token 预算 + wind-down）
// ========================================

// ReAct 循环终止原因（写入 Response.Metadata["terminated_by"]）。
const (
	// TerminatedByMaxIter 表示达到最大迭代次数而终止。
	TerminatedByMaxIter = "max_iter"
	// TerminatedByTokenBudget 表示 token 预算耗尽而终止。
	TerminatedByTokenBudget = "token_budget"
	// TerminatedByDeadline 表示挂钟预算耗尽（运行超时）而终止。
	TerminatedByDeadline = "deadline"
	// TerminatedByTruncated 表示模型输出被 finish_reason=length 截断（thinking 链撑爆输出预算），
	// 有界重试后仍未产出工具调用而转 wind-down 收尾。
	TerminatedByTruncated = "output_truncated"
)

// maxTruncationRetries 是「finish_reason=length 截断且无工具调用」时注入精简提示后的最大重试轮数。
// 超过即转 wind-down，避免在持续截断下无谓消耗迭代预算。
const maxTruncationRetries = 2

// truncationNudge 在检测到截断后注入，引导模型压缩思考、直接产出工具调用而非长篇叙述。
const truncationNudge = "上一轮的思考过长，在发出工具调用前就触达了输出长度上限被截断，导致本轮没有产出任何工具调用。" +
	"请大幅精简你的思考过程，不要在正文里长篇复述计划，直接输出你要调用的下一个工具及其参数。"

// isLengthTruncated 判定一次生成是否因输出长度上限被截断（finish_reason=length）。
func isLengthTruncated(msg *schema.Message) bool {
	return msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.FinishReason == "length"
}

// windDownPrompt 是达到 maxIter/预算上限时注入的收尾指令：要求模型不再调用工具，
// 直接基于已获得的观察结果交付一个自洽的最终答复，避免返回空内容或过程性话术。
//
// Bug B 修复：旧版只要求「部分结论」，模型在原地打转（如治理口径不匹配致 SUM=NULL）后
// 被动收尾时，常把「我发现了问题…我修正后重新执行」这类过程叙述当成最终答案交付——
// 用户拿到的是一句「我待会儿再算」而非任何数值或可用结论。此处显式禁止「将/会/稍后/
// 重新执行/继续尝试」等承诺性话术（本条回复之后没有新的执行轮次），并强制模型要么给出
// 已取到的具体数值/结果，要么直接说明卡点根因与基于现有信息的最佳可得结论。
const windDownPrompt = "已达到本轮的最大步数或 token 预算上限，请勿再调用任何工具。" +
	"注意：本条回复就是最终答复，之后不会再有新的执行轮次——" +
	"因此严禁使用「我将/我会/接下来/稍后/我修正后重新执行/继续尝试」之类的过程性或承诺性话术，" +
	"不要只描述你做了什么或打算怎么做。请直接用中文交付一个自洽的最终结论：" +
	"（1）优先给出你已经从观察结果中取到的具体数据/数值/结果，哪怕只是部分口径或中间结果；" +
	"（2）若因某个明确卡点（如口径/枚举值不匹配、工具报错）始终未取到目标结果，" +
	"请直接说明该卡点与你已定位到的根因或正确口径，并据此给出基于现有信息的最佳可得结论或估计；" +
	"（3）明确标注结果可能不完整及原因，必要时提示用户可如何继续（如按 result_id 取用、缩小范围或修正口径）。"

// usageTotalTokens 从一次生成响应中安全提取本轮消耗的 total tokens（缺失则 0）。
func usageTotalTokens(msg *schema.Message) int {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return 0
	}
	return msg.ResponseMeta.Usage.TotalTokens
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// execResult 汇总一次执行循环的产物，供 run 落库与填充元数据。
type execResult struct {
	content      string
	iterations   int
	tokensUsed   int
	terminatedBy string
	partial      bool
	role         string
	aborted      bool // 流式下游断开
}

// execLoop 是 ToolLoop 的核心：无工具时单轮生成；有工具时跑 ReAct 循环。
// 愈合：ReAct 循环的 token 预算前/后置检查与 wind-down 收尾对所有组合（含 memory+tools、streaming）统一生效。
func (a *agentImpl) execLoop(ctx context.Context, messages []*schema.Message, hasTools bool, sink execSink, response *Response) (execResult, error) {
	if !hasTools {
		// 确定使用的模型：优先 model，回退 toolModel。
		var chatModel model.BaseChatModel
		if a.model != nil {
			chatModel = a.model
		} else {
			chatModel = a.toolModel
		}

		msg, aborted, err := sink.generate(ctx, chatModel, messages, 1)
		if err != nil {
			return execResult{}, err
		}
		if aborted {
			return execResult{aborted: true}, nil
		}
		res := execResult{iterations: 1, tokensUsed: usageTotalTokens(msg)}
		if msg != nil {
			res.content = msg.Content
			res.role = string(msg.Role)
		}
		return res, nil
	}

	// 提取所有工具的 ToolInfo 并绑定到模型。
	// 按工具名去重：LLM 供应商（OpenAI/DeepSeek 兼容接口）硬性要求 tools 数组内工具名唯一，
	// 上游任一装配路径若混入同名工具都会导致 400 "Tool names must be unique"。此处收口去重
	// （保留首个、丢弃后续同名），并对被丢弃者打 WARN 便于回溯真正的重复来源。
	toolInfos := make([]*schema.ToolInfo, 0, len(a.tools))
	seenTools := make(map[string]struct{}, len(a.tools))
	for _, t := range a.tools {
		info, infoErr := t.Info(ctx)
		if infoErr != nil {
			continue // 跳过无法获取信息的工具
		}
		if _, dup := seenTools[info.Name]; dup {
			log.Printf("[agent:%s] 丢弃重名工具 %q（tools 数组要求唯一），请检查上游工具装配", a.name, info.Name)
			continue
		}
		seenTools[info.Name] = struct{}{}
		toolInfos = append(toolInfos, info)
	}
	var boundModel model.ToolCallingChatModel = a.toolModel
	if len(toolInfos) > 0 {
		bound, bindErr := a.toolModel.WithTools(toolInfos)
		if bindErr != nil {
			return execResult{}, fmt.Errorf("bind tools to model failed: %w", bindErr)
		}
		boundModel = bound
	}

	// 迭代处理（可能需要多轮工具调用）。
	// ReAct 循环受 maxIter 与 token 预算共同约束：任一到达上限即终止并收尾。
	var res execResult
	naturalFinish := false // 模型主动收尾（返回无工具调用的回复）；区别于达上限被动终止
	truncationRetries := 0 // finish_reason=length 截断且无工具调用时的已重试次数（有界）
	// 挂钟护栏起点：仅在配置了 wallClock 时计时；time.Since(loopStart) 到点即终止并 wind-down。
	loopStart := time.Now()
	// 自我修复护栏：每次运行私有，按失败签名计数触发再规划/提前收尾（并发安全）。
	guard := newFailureGuard()
	for i := 0; i < a.maxIter; i++ {
		// 三级压缩之①③：每轮生成前就地治理运行时上下文——单条超限压缩、累计超线折叠，
		// 不停机（区别于 tokenBudget 的达标即终止）。未配置阈值时为空操作，零回归。
		messages = a.normalizeContext(ctx, messages)

		// token 预算前置检查：预算已耗尽则不再发起新一轮生成。
		if a.tokenBudget > 0 && res.tokensUsed >= a.tokenBudget {
			res.terminatedBy = TerminatedByTokenBudget
			res.iterations = i
			break
		}

		// 挂钟预算前置检查：本轮运行累计耗时已达上限则不再发起新一轮生成，交由下方 wind-down 收尾。
		if a.wallClock > 0 && time.Since(loopStart) >= a.wallClock {
			res.terminatedBy = TerminatedByDeadline
			res.iterations = i
			break
		}

		msg, aborted, err := sink.generate(ctx, boundModel, messages, i+1)
		if err != nil {
			return execResult{}, err
		}
		if aborted {
			return execResult{aborted: true}, nil
		}
		res.tokensUsed += usageTotalTokens(msg)

		// sink 未返回消息也未报错/中止：视作自然结束，避免下面解引用 msg 崩溃。
		if msg == nil {
			res.iterations = i + 1
			naturalFinish = true
			break
		}

		// 检查是否有工具调用
		if len(msg.ToolCalls) == 0 {
			// finish_reason=length 截断且无工具调用：thinking 链撑爆输出预算，正文在发出 tool_call
			// 前被半途切断——绝非模型主动收尾。若直接当最终答案交付，用户拿到的是半句话、前端空轮询
			// 即表现为"卡住"。注入一次「精简思考、直接给工具」的提示后有界重试本轮，避免静默挂起。
			if hasTools && isLengthTruncated(msg) {
				if truncationRetries < maxTruncationRetries {
					truncationRetries++
					messages = append(messages, schema.SystemMessage(truncationNudge))
					log.Printf("[agent:%s] 第%d步 finish_reason=length 截断且无工具调用，注入精简提示后重试(%d/%d)",
						a.name, i+1, truncationRetries, maxTruncationRetries)
					continue
				}
				// 重试仍被截断：不把半句正文当最终答案，转 wind-down 给出诚实结论。
				res.terminatedBy = TerminatedByTruncated
				res.iterations = i + 1
				break
			}
			// 没有工具调用，模型主动收尾（Content 可能为空，但仍属自然结束，不应误判为达上限）
			res.content = msg.Content
			res.role = string(msg.Role)
			res.iterations = i + 1
			naturalFinish = true
			break
		}

		// assistant 轮整体只入队一次：保留 msg 自带的 reasoning_content 与全部 tool_calls，
		// 满足 DeepSeek V4 thinking "工具调用轮必须原样回传 reasoning_content" 的约束，
		// 同时避免把并行工具调用拆成多条畸形 assistant 消息。
		messages = append(messages, msg)

		// 处理工具调用
		for _, tc := range msg.ToolCalls {
			obs, ok := a.handleToolCall(ctx, tc, i+1, sink, response, guard)
			if !ok {
				return execResult{aborted: true}, nil
			}
			messages = append(messages, obs)
		}

		// 自我修复护栏：同一失败签名达阈值 → 注入一次再规划提示（引导换路径，非盲目重试）。
		if note := guard.replanNote(); note != "" {
			messages = append(messages, schema.SystemMessage(note))
		}
		// 累计失败过多（原地打转）→ 提前收尾，交由下方 wind-down 给出诚实的部分结论。
		if guard.shouldWindDown() {
			res.terminatedBy = TerminatedByRepairExhausted
			res.iterations = i + 1
			break
		}

		// 本轮工具执行后再判预算：耗尽则标记终止，交由下方 wind-down 收尾。
		if a.tokenBudget > 0 && res.tokensUsed >= a.tokenBudget {
			res.terminatedBy = TerminatedByTokenBudget
			res.iterations = i + 1
			break
		}

		// 本轮工具执行后再判挂钟：耗时达上限则标记终止，交由下方 wind-down 收尾。
		// 工具阻塞（长 SQL/子代理委派）常在此处才越线——放在工具执行后判定能及时兜住这类卡顿。
		if a.wallClock > 0 && time.Since(loopStart) >= a.wallClock {
			res.terminatedBy = TerminatedByDeadline
			res.iterations = i + 1
			break
		}
	}

	// 循环耗尽 maxIter 却未自然收尾（最后一步仍是工具调用）→ 标记 max_iter。
	// 以 naturalFinish 判定而非 Content==""：模型主动收尾但返回空内容时仍属自然结束。
	if !naturalFinish && res.terminatedBy == "" {
		res.terminatedBy = TerminatedByMaxIter
		res.iterations = a.maxIter
	}

	// wind-down：因达到 maxIter 或预算耗尽而被动终止（非自然收尾）时，做一次「无工具」收尾生成，
	// 让模型基于已观察到的结果（Scratchpad/result_id 信封）给出部分结论，而非返回空串。
	if !naturalFinish && res.terminatedBy != "" {
		messages = a.normalizeContext(ctx, messages)
		windMsgs := append(messages, schema.SystemMessage(windDownPrompt))
		final, aborted, ferr := sink.generate(ctx, a.toolModel, windMsgs, res.iterations+1)
		if aborted {
			return execResult{aborted: true}, nil
		}
		if ferr == nil && final != nil {
			res.content = final.Content
			res.role = string(final.Role)
			res.tokensUsed += usageTotalTokens(final)
		}
		res.partial = true
	}

	return res, nil
}
