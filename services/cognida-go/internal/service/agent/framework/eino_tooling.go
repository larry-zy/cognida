// 由 eino_agent.go 拆出——同包、行为等价（M1 god-file 拆分）。
package framework

import (
	"cognida/internal/pkg/safego"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// handleToolCall 执行单个工具调用：解析参数 → 下发工具事件（streaming）→ 执行 → 记录 ToolCall
// → 返回追加到历史的观察消息。ok=false 表示下游断开（streaming）。
// 愈合：参数不可解析时合成错误观察、不执行工具（原 buffered 变体会带残缺参数执行）；
// 无论解析成败都追加一条 tool 消息，保证 assistant.tool_calls 与 tool 消息严格 1:1。
func (a *agentImpl) handleToolCall(ctx context.Context, tc schema.ToolCall, iteration int, sink execSink, response *Response, guard *failureGuard) (*schema.Message, bool) {
	toolCall := &ToolCall{Name: tc.Function.Name}

	// 解析参数
	var args map[string]interface{}
	parseErr := false
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			toolCall.Error = fmt.Errorf("invalid arguments: %w", err)
			toolCall.Input = nil
			parseErr = true
		} else {
			toolCall.Input = args
		}
	}

	// 参数不可解析：合成错误观察，不执行工具。仍须为该 tool_call 追加一条 tool 消息，
	// 保证 assistant.tool_calls 与 tool 消息严格 1:1，杜绝下一轮生成因悬空
	// tool_call 触发 "insufficient tool messages following tool_calls" 400。
	if parseErr {
		if !sink.onToolCall(ctx, &ToolCallInStream{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Status: "error",
			Error:  fmt.Sprintf("参数解析失败: %v", toolCall.Error),
		}) {
			return nil, false
		}
		toolCall.Output = fmt.Sprintf("Error: 参数解析失败: %v", toolCall.Error)
		response.ToolCalls = append(response.ToolCalls, toolCall)
		return schema.ToolMessage(compactObservation(toolCall.Output), tc.ID), true
	}

	// 发送工具调用事件（buffered 空实现）
	if !sink.onToolCall(ctx, &ToolCallInStream{
		ID:     tc.ID,
		Name:   tc.Function.Name,
		Input:  args,
		Status: "calling",
	}) {
		return nil, false
	}

	// 执行工具
	output, synthetic, execErr := a.invokeTool(ctx, tc)

	// 逐工具输出护栏（post-invoke，任务 5）：成功观察在回灌 LLM 与下发事件前脱敏；
	// 未装配时零开销、观察逐字节不变；错误观察不脱敏（保留原始报错供自我修正）。
	// synthetic（工具门合成的 tool_blocked / pending_confirm 控制面载荷）不脱敏——
	// 其非真实工具观察，且携 confirm_token 等控制字段，脱敏会破坏确认续跑（不可误伤）。
	if execErr == nil && !synthetic && a.toolOutputHook != nil {
		output = a.toolOutputHook(ctx, tc.Function.Name, output)
	}

	// 组装工具结果事件
	resultEvent := &ToolCallInStream{
		ID:     tc.ID,
		Name:   tc.Function.Name,
		Input:  args,
		Status: "success",
		Output: output,
	}
	// 可修复观察（RepairableToolError）不是裸报错：其 Observation 为结构化 JSON（error_kind/
	// retriable/hint/detail），应原样回灌 LLM 引导定向修正，而非渲染成 "Error: %v"。
	// 同时把 error_kind 计入失败签名（下方 recordToolFailure），触发再规划/提前收尾。
	obs := output
	if execErr != nil {
		toolCall.Error = execErr
		resultEvent.Status = "error"
		resultEvent.Error = execErr.Error()
		if repair, ok := AsRepairable(execErr); ok {
			obs = repair.Observation
			toolCall.Output = repair.Observation
			guard.recordFailure(tc.Function.Name, repair.ErrorKind)
		} else {
			obs = fmt.Sprintf("Error: %v", execErr)
			toolCall.Output = obs
			guard.recordFailure(tc.Function.Name, "")
		}
	} else {
		toolCall.Output = output
		guard.recordSuccess(tc.Function.Name)
	}

	if !sink.onToolResult(ctx, resultEvent, iteration) {
		return nil, false
	}
	response.ToolCalls = append(response.ToolCalls, toolCall)

	// 追加该 tool_call 对应的结果消息
	return schema.ToolMessage(compactObservation(obs), tc.ID), true
}

// invokeTool 执行单个工具调用。synthetic 为 true 时 output 为工具门合成的控制面载荷
// （tool_blocked / pending_confirm，未触达底层工具），调用方据此跳过逐工具输出护栏，
// 避免脱敏破坏 confirm_token 等控制字段。
func (a *agentImpl) invokeTool(ctx context.Context, toolCall schema.ToolCall) (output string, synthetic bool, err error) {
	// 工具级 span：挂在外层 agent.chat 之下，还原调用链瀑布（含被门拒/待确认的合成结果，
	// 便于排查为何某工具未真正执行）。otel 未注册真实 provider 时为 no-op、零开销。
	ctx, span := frameworkTracer.StartToolSpan(ctx, a.name, toolCall.Function.Name, toolCall.Function.Arguments)
	defer func() {
		span.SetAttribute("tool.output_length", len(output))
		span.SetAttribute("tool.synthetic", synthetic)
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()

	// 执行前硬工具门（Phase 6 + 护栏三值门）：skill 策略 + 会话 scope 同为必要条件，
	// 其上叠加写/导出审批第三态。被拒/需审批调用不触达底层执行，以合成
	// tool_blocked / 待人工确认 ToolMessage 回灌 LLM。
	if blocked, ok := gateToolCall(ctx, toolCall.Function.Name, toolCall.Function.Arguments); !ok {
		return blocked, true, nil
	}

	// 查找工具
	var selectedTool tool.BaseTool
	for _, t := range a.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == toolCall.Function.Name {
			selectedTool = t
			break
		}
	}

	if selectedTool == nil {
		return "", false, fmt.Errorf("tool not found: %s", toolCall.Function.Name)
	}

	// 执行工具：可选套统一的单次工具超时兜底。委派类协作元工具自带更长的内部超时
	// （delegationTimeout=180s），需给更宽的单工具挂钟上限——否则通用 toolTimeout（90s）会
	// 先于其内部超时把三路并行委派掐断，退化回主循环内联，白费子代理隔离（历史 bug）。
	timeout := a.effectiveToolTimeout(toolCall.Function.Name)
	if timeout <= 0 {
		out, err := a.runSelectedTool(ctx, selectedTool, toolCall)
		return out, false, err
	}
	out, err := a.runToolWithTimeout(ctx, selectedTool, toolCall, timeout)
	return out, false, err
}

// collabToolWallClock 委派类协作元工具的单工具挂钟上限：须高于其内部 delegationTimeout，
// 让内部超时先触发、产出「失败项可携原信封单独重试」的优雅信封，本值只作真·卡死的外层兜底。
const collabToolWallClock = delegationTimeout + 30*time.Second

// selfBoundedCollabTools 自带内部超时的协作元工具名集合：委派走 delegationTimeout（executeDelegation），
// ask/handoff 走各自 timeout（见 collab_tools.go）。这些工具不该受通用 toolTimeout 提前掐断。
var selfBoundedCollabTools = map[string]bool{
	"delegate_to_agent": true,
	"delegate_parallel": true,
	"ask_agent":         true,
	"handoff_to":        true,
}

// effectiveToolTimeout 返回某工具单次执行的挂钟上限：通用 toolTimeout<=0（不限）时一律不设限；
// 委派类协作元工具给更宽的 collabToolWallClock（避免通用 toolTimeout 先掐断退化回内联），其余工具用通用值。
func (a *agentImpl) effectiveToolTimeout(toolName string) time.Duration {
	if a.toolTimeout <= 0 {
		return 0
	}
	if selfBoundedCollabTools[toolName] && collabToolWallClock > a.toolTimeout {
		return collabToolWallClock
	}
	return a.toolTimeout
}

// runToolWithTimeout 在 toolTimeout 挂钟上限内执行工具：派生带 deadline 的子 ctx 传给工具（协作型
// I/O 会据此及时取消，不泄漏），同时把执行放到 goroutine 并 select ctx.Done()——即便某工具忽略 ctx、
// 纯阻塞，invokeTool 也能在到点后返回一次可恢复的超时观察，不把 ReAct 循环拖死（这正是「单步 hang 使
// wallClock 失效」的堵口）。到点后被遗弃的 goroutine 会在其自身返回时自然结束，不影响主循环推进。
func (a *agentImpl) runToolWithTimeout(ctx context.Context, selectedTool tool.BaseTool, toolCall schema.ToolCall, timeout time.Duration) (string, error) {
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type toolOutcome struct {
		out string
		err error
	}
	done := make(chan toolOutcome, 1) // 缓冲 1：超时遗弃后 goroutine 仍能无阻塞写入并退出，不泄漏
	go func() {
		defer safego.Recover("eino-agent-stream")
		out, err := a.runSelectedTool(toolCtx, selectedTool, toolCall)
		done <- toolOutcome{out: out, err: err}
	}()

	select {
	case r := <-done:
		return r.out, r.err
	case <-toolCtx.Done():
		// 到点即返回：区分整体取消（客户端断开）与单纯超时，便于上层归因。
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("工具 %s 执行超过 %s 未返回（已中止本步，交由循环换路径或收尾）", toolCall.Function.Name, timeout)
	}
}

// runSelectedTool 执行已定位的工具（Invokable / Streamable），不含超时逻辑。
func (a *agentImpl) runSelectedTool(ctx context.Context, selectedTool tool.BaseTool, toolCall schema.ToolCall) (string, error) {
	switch t := selectedTool.(type) {
	case tool.InvokableTool:
		return t.InvokableRun(ctx, toolCall.Function.Arguments)
	case tool.StreamableTool:
		stream, err := t.StreamableRun(ctx, toolCall.Function.Arguments)
		if err != nil {
			return "", fmt.Errorf("streamable run failed: %w", err)
		}

		// 收集流式结果
		var result strings.Builder
		for {
			chunk, err := stream.Recv()
			if err != nil {
				// errors.Is 而非字符串比较：包装过的 io.EOF 也能识别为正常结束
				if errors.Is(err, io.EOF) {
					break
				}
				stream.Close()
				return "", fmt.Errorf("recv failed: %w", err)
			}
			result.WriteString(chunk)
		}
		stream.Close()
		return result.String(), nil
	default:
		return "", fmt.Errorf("unsupported tool type: %T", t)
	}
}
