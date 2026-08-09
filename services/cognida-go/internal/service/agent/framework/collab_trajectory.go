// 委派痕迹侧信道（评测可观测性）：委派边界（executeDelegation）默认只回传子代理的
// 最终内容（handle/摘要），刻意不回灌其内部工具往返——这对生产对话是正确的上下文防火墙，
// 但会让「指挥官」型 agent（如 data_agent）的顶层 Response 完全看不到子代理真实执行的
// get_schema/sql_execute 等工具与其 token 消耗，导致 Agent 评测的工具/轨迹/用量指标失真。
//
// 本文件提供一个挂在 ctx 上的累积器：顶层 agentImpl.run 安装一个空痕迹，
// executeDelegation 在子代理成功返回后把子代理的 ToolCalls 与运行时用量追加进来，
// run 结束时把痕迹并入顶层 Response。仅供评测/审计观测，不改变回传给 LLM 的内容。
package framework

import (
	"context"
	"sync"
)

// delegationTrace 累积委派链中各子代理暴露给评测的执行痕迹：子代理内部的工具调用轨迹
// 与运行时用量（tokens/iterations）。并发安全——并行委派（collab_parallel）在多 goroutine
// 中对同一父痕迹追加。
type delegationTrace struct {
	mu         sync.Mutex
	toolCalls  []*ToolCall
	tokens     int
	iterations int
}

// add 追加一次子代理执行的工具轨迹与用量（nil 接收者按无操作，简化调用点判空）。
func (d *delegationTrace) add(toolCalls []*ToolCall, tokens, iterations int) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.toolCalls = append(d.toolCalls, toolCalls...)
	d.tokens += tokens
	d.iterations += iterations
}

// drain 取出累积痕迹（返回后清空，供 run 一次性并入顶层 Response）。
func (d *delegationTrace) drain() (toolCalls []*ToolCall, tokens, iterations int) {
	if d == nil {
		return nil, 0, 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tc, tok, it := d.toolCalls, d.tokens, d.iterations
	d.toolCalls, d.tokens, d.iterations = nil, 0, 0
	return tc, tok, it
}

type delegationTraceKey struct{}

// withDelegationTrace 把委派痕迹累积器挂到 ctx（顶层 agentImpl.run 安装）。
func withDelegationTrace(ctx context.Context, t *delegationTrace) context.Context {
	return context.WithValue(ctx, delegationTraceKey{}, t)
}

// delegationTraceFrom 从 ctx 取委派痕迹累积器（未安装返回 nil）。
func delegationTraceFrom(ctx context.Context) *delegationTrace {
	t, _ := ctx.Value(delegationTraceKey{}).(*delegationTrace)
	return t
}

// metaUsageInt 从子代理响应 Metadata 安全读取整型用量指标（缺失或类型不符按 0 计）。
func metaUsageInt(meta map[string]interface{}, key string) int {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
