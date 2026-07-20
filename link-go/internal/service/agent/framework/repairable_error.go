package framework

import "errors"

// RepairableToolError 是工具层产出的「可修复观察」错误：其 Error() 即一段紧凑 JSON，
// 由 execLoop 直接作为 tool 观察回灌 LLM（引导定向自我修正），而非渲染成裸 "Error: %v"。
//
// 与普通 error 的区别：
//   - Observation 为结构化 JSON（error_kind/retriable/hint/detail），LLM 可据此定向改写而非盲目重试。
//   - ErrorKind 供 execLoop 按 toolName+error_kind 做失败签名计数，触发再规划/提前收尾（自我修复护栏）。
//
// 工具返回 *RepairableToolError 后，TypedBaseTool 会原样透传该 error（不吞不裹），
// handleToolCall 经 errors.As 识别并渲染 Observation。framework 不 import tools，无环。
type RepairableToolError struct {
	Observation string // 紧凑 JSON 可修复观察，直接作为 ToolMessage 回灌
	ErrorKind   string // 失败签名维度（unknown_column/unknown_table/syntax/timeout/permission/transient/other）
}

// Error 返回可修复观察 JSON 本体，使其既满足 error 接口，又能被直接回灌为观察文本。
func (e *RepairableToolError) Error() string { return e.Observation }

// AsRepairable 从错误链中提取 *RepairableToolError；未命中返回 (nil,false)。
// 供 handleToolCall 渲染观察、execLoop 计数签名共用，避免各处重复 errors.As 样板。
func AsRepairable(err error) (*RepairableToolError, bool) {
	var re *RepairableToolError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}
