package framework

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// repairingTool 总是失败并返回一个可修复观察错误（RepairableToolError）。
type repairingTool struct {
	name  string
	obs   string
	kind  string
	calls *int
}

func (t *repairingTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name, Desc: "always repairable-fail"}, nil
}

func (t *repairingTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	*t.calls++
	return "", &RepairableToolError{Observation: t.obs, ErrorKind: t.kind}
}

// capturingModel 记录每次 Generate 收到的输入消息，按 maxToolTurns 回工具调用、之后自然收尾。
type capturingModel struct {
	toolName     string
	maxToolTurns int
	calls        int
	lastInput    []*schema.Message
}

func (m *capturingModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	m.lastInput = input
	if m.calls > m.maxToolTurns {
		return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
	}
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "x",
			Function: schema.FunctionCall{Name: m.toolName, Arguments: "{}"},
		}},
	}, nil
}

func (m *capturingModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Close()
	return reader, nil
}

func (m *capturingModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// TestSelfRepair_ObservationFedVerbatim 校验：RepairableToolError 的 Observation 原样作为工具观察
// 回灌 LLM（而非渲染成 "Error: %v"）。
func TestSelfRepair_ObservationFedVerbatim(t *testing.T) {
	obs := `{"error_kind":"unknown_column","retriable":true,"hint":{"tip":"改列名"}}`
	calls := 0
	tl := &repairingTool{name: "sql_execute", obs: obs, kind: "unknown_column", calls: &calls}
	tm := &capturingModel{toolName: "sql_execute", maxToolTurns: 1}

	a := &agentImpl{name: "r", toolModel: tm, prompt: "p", tools: []tool.BaseTool{tl}, maxIter: 6}
	resp, err := a.Chat(context.Background(), "查一下")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	_ = resp

	// 第二次 Generate 的输入里应含一条内容与 obs 逐字一致的 tool 消息。
	var found bool
	for _, msg := range tm.lastInput {
		if msg.Role == schema.Tool && msg.Content == obs {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("可修复观察未逐字回灌，最后输入=%+v", tm.lastInput)
	}
	// 且工具输出不得被渲染成裸 Error 前缀。
	for _, msg := range tm.lastInput {
		if msg.Role == schema.Tool && strings.HasPrefix(msg.Content, "Error:") {
			t.Fatalf("可修复观察被误渲染为裸报错: %q", msg.Content)
		}
	}
}

// TestSelfRepair_RepeatedFailureWindsDown 校验：同一失败签名反复出现累计达阈值 → 提前收尾
// （terminated_by=repair_exhausted, partial=true），且工具调用次数受护栏约束、远少于 maxIter。
func TestSelfRepair_RepeatedFailureWindsDown(t *testing.T) {
	obs := `{"error_kind":"unknown_column","retriable":true}`
	calls := 0
	tl := &repairingTool{name: "sql_execute", obs: obs, kind: "unknown_column", calls: &calls}
	// 模型永不主动收尾（一直调同一工具），交由自我修复护栏兜底。
	tm := &capturingModel{toolName: "sql_execute", maxToolTurns: 100}

	a := &agentImpl{name: "r", toolModel: tm, prompt: "p", tools: []tool.BaseTool{tl}, maxIter: 20}
	resp, err := a.Chat(context.Background(), "查一下")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Metadata["terminated_by"] != TerminatedByRepairExhausted {
		t.Fatalf("应因反复失败提前收尾, got terminated_by=%v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Fatal("提前收尾应标记 partial")
	}
	// windDownThreshold=4：恰在第 4 次失败后收尾，绝不空转到 maxIter。
	if calls != windDownThreshold {
		t.Fatalf("工具调用次数应受护栏约束为 %d，got %d", windDownThreshold, calls)
	}
}

// TestSelfRepair_ReplanNoteInjected 校验：同一失败签名达再规划阈值（2 次）后，向后续 LLM 输入
// 注入一条带签名的再规划 SystemMessage，引导其换思路而非盲目重试。
func TestSelfRepair_ReplanNoteInjected(t *testing.T) {
	obs := `{"error_kind":"unknown_column","retriable":true}`
	calls := 0
	tl := &repairingTool{name: "sql_execute", obs: obs, kind: "unknown_column", calls: &calls}
	// 失败两轮（触发再规划），第三轮自然收尾——不触及 wind-down（累计 2 < 4）。
	tm := &capturingModel{toolName: "sql_execute", maxToolTurns: 2}

	a := &agentImpl{name: "r", toolModel: tm, prompt: "p", tools: []tool.BaseTool{tl}, maxIter: 20}
	resp, err := a.Chat(context.Background(), "查一下")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// 未达 wind-down，应自然收尾。
	if resp.Metadata["terminated_by"] == TerminatedByRepairExhausted {
		t.Fatal("累计 2 次失败不应触发 wind-down")
	}
	// 再规划提示为 SystemMessage，累积保留在消息序列中，末轮输入应可见。
	var noted bool
	for _, msg := range tm.lastInput {
		if msg.Role == schema.System && strings.Contains(msg.Content, "sql_execute:unknown_column") {
			noted = true
			break
		}
	}
	if !noted {
		t.Fatalf("达阈值后应注入带签名的再规划提示，最后输入=%+v", tm.lastInput)
	}
}
