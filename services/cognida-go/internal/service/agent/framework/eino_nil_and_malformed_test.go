package framework

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// 本文件覆盖两处回归：
//   - issue #4：execLoop 中 msg==nil 的处理——已跑过工具轮后不得按自然结束交付空答；
//   - issue #8：handleToolCall 参数不可解析分支须计入自我修复护栏。

// badToolCallMsg 构造一个参数为非法 JSON 的工具调用 assistant 响应（issue #8 场景）。
func badToolCallMsg(id, name string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       id,
			Function: schema.FunctionCall{Name: name, Arguments: `{"sql": "SELECT`},
		}},
	}
}

// ========================================
// issue #4：msg == nil 的两种走向
// ========================================

// TestReAct_NilResponseAfterToolsWindsDown 验证：已跑过工具轮后某轮生成返回 (nil, nil)
// （既无消息也无错误）时，不得按自然结束交付空答——应视作异常终止转 wind-down，
// 从已有观察合成答复，并标注 terminated_by=no_response、partial=true。
// 脚本：第 1 轮正常工具调用（产出观察）→ 第 2 轮 nil → 第 3 次生成（wind-down）走脚本默认 "final"。
func TestReAct_NilResponseAfterToolsWindsDown(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 脚本槽位为 nil 即让 scriptedToolModel.Generate 返回 (nil, nil)。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		nil,
	}}

	a := &agentImpl{
		name:      "nilresp",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "工具后空响应")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "final" {
		t.Errorf("nil 响应（已跑过工具轮）必须 wind-down 出非空结论, got %q", resp.Content)
	}
	if resp.Metadata["terminated_by"] != TerminatedByNoResponse {
		t.Errorf("expected terminated_by=no_response, got %v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Errorf("wind-down 恢复必须标记 partial")
	}
	// query（观察）+ nil 响应 + wind-down 合成 = 3 次生成；工具只真正执行一次。
	if tm.calls != 3 {
		t.Errorf("expected 3 Generate calls (tool + nil + wind-down), got %d", tm.calls)
	}
	if len(order) != 1 || order[0] != "query" {
		t.Errorf("expected query invoked exactly once, got %v", order)
	}
}

// TestReAct_NilResponseFirstRound_StaysNatural 验证：首轮（i==0）即无消息时仍按自然结束放行
// ——尚无任何工具观察可合成，wind-down 无从谈起（与 empty_finish 的 i>0 门槛同理）。
// 这是与上一用例互补的另一半：只在「有观察」时才兜底。
func TestReAct_NilResponseFirstRound_StaysNatural(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	tm := &scriptedToolModel{script: []*schema.Message{nil}}

	a := &agentImpl{
		name:      "nilresp",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "首轮即空响应")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if _, ok := resp.Metadata["terminated_by"]; ok {
		t.Errorf("首轮 nil 响应不应设置 terminated_by: %v", resp.Metadata)
	}
	if resp.Metadata["partial"] == true {
		t.Errorf("首轮 nil 响应不应标记 partial")
	}
	// 仅 1 次生成；无观察不触发 wind-down；工具从未被调用。
	if tm.calls != 1 {
		t.Errorf("expected exactly 1 Generate call, got %d", tm.calls)
	}
	if len(order) != 0 {
		t.Errorf("expected no tool invocation, got %v", order)
	}
}

// TestReAct_NilResponseAfterTruncationRetry_StaysNatural 验证：i>0 不等于「有观察」——
// 截断重试轮（注入精简提示后 continue）也消耗迭代但不产生任何工具观察。此路径上 nil 响应
// 仍按自然结束放行：wind-down 的「从观察给出结论」在零观察下会诱导模型凭空编造（幻觉）。
func TestReAct_NilResponseAfterTruncationRetry_StaysNatural(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 第 1 轮 finish_reason=length 且无工具调用（触发注入精简提示后重试），第 2 轮 nil。
	truncated := &schema.Message{
		Role:         schema.Assistant,
		Content:      "半句",
		ResponseMeta: &schema.ResponseMeta{FinishReason: "length"},
	}
	tm := &scriptedToolModel{script: []*schema.Message{truncated, nil}}

	a := &agentImpl{
		name:      "nilresp",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "截断后空响应")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if _, ok := resp.Metadata["terminated_by"]; ok {
		t.Errorf("零观察的 nil 响应不应设置 terminated_by: %v", resp.Metadata)
	}
	if resp.Metadata["partial"] == true {
		t.Errorf("零观察的 nil 响应不应标记 partial")
	}
	// 截断轮 + nil 轮 = 2 次生成；无 wind-down；工具从未被调用。
	if tm.calls != 2 {
		t.Errorf("expected 2 Generate calls (truncated + nil), got %d", tm.calls)
	}
	if len(order) != 0 {
		t.Errorf("expected no tool invocation, got %v", order)
	}
}

// TestReAct_NilResponseWindDownAlsoEmpty_FallsBackToNotice 验证：工具轮后 nil 响应转 wind-down，
// 而收尾生成本身也失败（继续返回 nil）时，交付诚实的降级说明而非空白回复。
func TestReAct_NilResponseWindDownAlsoEmpty_FallsBackToNotice(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 工具轮 → nil → wind-down 也 nil。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		nil,
		nil,
	}}

	a := &agentImpl{
		name:      "nilresp",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "收尾也失败")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Metadata["terminated_by"] != TerminatedByNoResponse {
		t.Errorf("expected terminated_by=no_response, got %v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Error("异常终止应标记 partial")
	}
	if !strings.Contains(resp.Content, "未能生成最终答复") || !strings.Contains(resp.Content, TerminatedByNoResponse) {
		t.Errorf("收尾失败应交付含终止原因的降级说明, got %q", resp.Content)
	}
	if tm.calls != 3 {
		t.Errorf("expected 3 Generate calls (tool + nil + wind-down-nil), got %d", tm.calls)
	}
}

// ========================================
// issue #8：参数不可解析须计入自我修复护栏
// ========================================

// TestMalformedArgs_NeverReachTool 校验畸形参数不触达工具执行，且 assistant.tool_calls
// 与 tool 消息保持 1:1（观察为合成的解析失败错误）。
func TestMalformedArgs_NeverReachTool(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	tm := &scriptedToolModel{script: []*schema.Message{
		badToolCallMsg("1", "query"),
	}}

	a := &agentImpl{
		name:      "badargs",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "畸形参数")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(order) != 0 {
		t.Errorf("畸形参数绝不应触达工具执行, got %v", order)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Error == nil ||
		!strings.Contains(resp.ToolCalls[0].Error.Error(), "invalid arguments") {
		t.Errorf("应记录一次解析失败的工具调用, got %+v", resp.ToolCalls)
	}
	// 1:1 配对：历史中 1 条带 tool_calls 的 assistant 消息对应 1 条 tool 观察消息。
	var assistantWithCalls, toolObs int
	for _, msg := range tm.lastInput {
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			assistantWithCalls++
		}
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "参数解析失败") {
			toolObs++
		}
	}
	if assistantWithCalls != 1 || toolObs != 1 {
		t.Errorf("tool_call 与 tool 消息应严格 1:1, got assistant=%d tool=%d", assistantWithCalls, toolObs)
	}
}

// TestMalformedArgs_RepeatedTriggersReplan 校验：同一工具的畸形参数失败累计达再规划阈值（2 次）后，
// 注入带签名的再规划提示（签名退化为工具名本身，kind 为空）；未达 wind-down 阈值则继续并自然收尾。
func TestMalformedArgs_RepeatedTriggersReplan(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 两轮畸形参数（触发再规划），脚本耗尽后第 3 次生成自然收尾 "final"。
	tm := &scriptedToolModel{script: []*schema.Message{
		badToolCallMsg("1", "query"),
		badToolCallMsg("2", "query"),
	}}

	a := &agentImpl{
		name:      "badargs",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "反复畸形参数")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// 累计 2 次失败 < windDownThreshold(4)：不应提前收尾，应自然收尾。
	if resp.Metadata["terminated_by"] == TerminatedByRepairExhausted {
		t.Fatal("累计 2 次失败不应触发 wind-down")
	}
	// 再规划提示（SystemMessage，含退化签名「query」）应注入后续输入。
	var noted bool
	for _, msg := range tm.lastInput {
		if msg.Role == schema.System && strings.Contains(msg.Content, "重复失败：query") {
			noted = true
			break
		}
	}
	if !noted {
		t.Fatalf("畸形参数反复失败后应注入带签名的再规划提示, lastInput=%+v", tm.lastInput)
	}
	if len(order) != 0 {
		t.Errorf("畸形参数绝不应触达工具执行, got %v", order)
	}
}

// TestMalformedArgs_WindDown 校验：模型永不收敛、持续以畸形参数调用同一工具时，护栏在
// windDownThreshold(4) 次失败后触发提前收尾（terminated_by=repair_exhausted、partial=true）——
// 修复前该失败模式对护栏不可见，会一直空转到脚本耗尽/自然收尾。
func TestMalformedArgs_WindDown(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 10 轮畸形参数脚本：足够触发护栏（4 次）且足以在修复前暴露「空转」差异。
	script := make([]*schema.Message, 0, 10)
	for i := 0; i < 10; i++ {
		script = append(script, badToolCallMsg(string(rune('0'+i)), "query"))
	}
	tm := &scriptedToolModel{script: script}

	a := &agentImpl{
		name:      "badargs",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   20,
	}

	resp, err := a.Chat(context.Background(), "永远畸形参数")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Metadata["terminated_by"] != TerminatedByRepairExhausted {
		t.Fatalf("持续畸形参数应触发护栏提前收尾, got terminated_by=%v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Error("提前收尾应标记 partial")
	}
	// 恰在第 4 次失败后收尾：4 次工具调用轮 + 1 次 wind-down 生成。
	if len(resp.ToolCalls) != windDownThreshold {
		t.Fatalf("畸形参数失败应恰计满 %d 次即收尾, got %d", windDownThreshold, len(resp.ToolCalls))
	}
	if tm.calls != windDownThreshold+1 {
		t.Fatalf("生成次数应为 %d（失败轮+wind-down）, got %d", windDownThreshold+1, tm.calls)
	}
	for _, tc := range resp.ToolCalls {
		if tc.Error == nil || !strings.Contains(tc.Error.Error(), "invalid arguments") {
			t.Errorf("每次工具调用都应记录解析失败: %+v", tc)
		}
	}
	if len(order) != 0 {
		t.Errorf("畸形参数绝不应触达工具执行, got %v", order)
	}
}
