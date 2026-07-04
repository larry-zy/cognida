package framework

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// scriptedToolModel 是一个可编排的 ToolCallingChatModel 桩：
// 每次 Generate 依次返回 script 中的下一个响应；用完后返回空内容（自然收尾）。
// perCallTokens 模拟每次生成消耗的 total tokens，用于驱动 token 预算路径。
type scriptedToolModel struct {
	script        []*schema.Message
	idx           int
	calls         int
	perCallTokens int
}

func (m *scriptedToolModel) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	var out *schema.Message
	if m.idx < len(m.script) {
		out = m.script[m.idx]
		m.idx++
	} else {
		out = &schema.Message{Role: schema.Assistant, Content: "final"}
	}
	if m.perCallTokens > 0 {
		out.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: m.perCallTokens}}
	}
	return out, nil
}

func (m *scriptedToolModel) Stream(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Close()
	return reader, nil
}

func (m *scriptedToolModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// recordingTool 是一个记录调用顺序的 InvokableTool 桩。
type recordingTool struct {
	name  string
	calls *[]string
}

func (t *recordingTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name, Desc: "test tool " + t.name}, nil
}

func (t *recordingTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	*t.calls = append(*t.calls, t.name)
	return fmt.Sprintf("ok:%s", t.name), nil
}

// toolCallMsg 构造一个携带单个工具调用的 assistant 响应。
func toolCallMsg(id, name string) *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       id,
			Function: schema.FunctionCall{Name: name, Arguments: "{}"},
		}},
	}
}

// TestReAct_DynamicToolOrder 验证单一 ReAct 循环按模型返回的动态顺序调用工具，
// 无工具调用时自然收尾并返回内容。
func TestReAct_DynamicToolOrder(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}
	atool := &recordingTool{name: "analyze", calls: &order}

	// 脚本：先 query，再 analyze，最后无工具调用给出结论。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		toolCallMsg("2", "analyze"),
		{Role: schema.Assistant, Content: "结论"},
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool, atool},
		maxIter:   10,
	}

	resp, err := a.chatWithTools(context.Background(), "分析趋势")
	if err != nil {
		t.Fatalf("chatWithTools: %v", err)
	}
	if resp.Content != "结论" {
		t.Errorf("expected natural conclusion, got %q", resp.Content)
	}
	if len(order) != 2 || order[0] != "query" || order[1] != "analyze" {
		t.Errorf("expected dynamic order [query analyze], got %v", order)
	}
	if _, ok := resp.Metadata["terminated_by"]; ok {
		t.Errorf("natural finish should not set terminated_by: %v", resp.Metadata)
	}
	if resp.Metadata["partial"] == true {
		t.Errorf("natural finish should not be partial")
	}
}

// TestReAct_EmptyNaturalFinish 验证模型主动收尾但返回空内容时，仍判为自然结束：
// 不应误标 terminated_by=max_iter，也不应触发 wind-down / partial。
func TestReAct_EmptyNaturalFinish(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 脚本：先 query，再返回「无工具调用 + 空内容」的主动收尾。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		{Role: schema.Assistant, Content: ""},
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.chatWithTools(context.Background(), "空结论收尾")
	if err != nil {
		t.Fatalf("chatWithTools: %v", err)
	}
	if _, ok := resp.Metadata["terminated_by"]; ok {
		t.Errorf("empty natural finish must not set terminated_by: %v", resp.Metadata)
	}
	if resp.Metadata["partial"] == true {
		t.Errorf("empty natural finish must not be marked partial")
	}
	// 仅调用了一次 query，未因误判触发额外的 wind-down 生成。
	if tm.calls != 2 {
		t.Errorf("expected exactly 2 Generate calls (query + finish), got %d", tm.calls)
	}
}

// TestReAct_MaxIterTermination 验证模型持续调用工具时，循环在 maxIter 处终止，
// 触发无工具 wind-down 收尾并标注 terminated_by=max_iter、partial=true。
func TestReAct_MaxIterTermination(t *testing.T) {
	var order []string
	loopTool := &recordingTool{name: "query", calls: &order}

	// 脚本：始终返回工具调用（永不自然收尾），最后 wind-down 生成收尾语。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		toolCallMsg("2", "query"),
		toolCallMsg("3", "query"),
		// wind-down（第 4 次 Generate，无工具）→ 脚本已尽，返回 "final"。
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{loopTool},
		maxIter:   3,
	}

	resp, err := a.chatWithTools(context.Background(), "永动查询")
	if err != nil {
		t.Fatalf("chatWithTools: %v", err)
	}
	if len(order) != 3 {
		t.Errorf("expected exactly maxIter=3 tool calls, got %d (%v)", len(order), order)
	}
	if resp.Metadata["terminated_by"] != TerminatedByMaxIter {
		t.Errorf("expected terminated_by=max_iter, got %v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Errorf("expected partial=true on maxIter termination")
	}
	if resp.Content == "" {
		t.Errorf("wind-down must fill a non-empty partial conclusion")
	}
}

// TestReAct_TokenBudgetTermination 验证 token 预算耗尽时循环提前终止并 wind-down 收尾，
// terminated_by=token_budget，且工具调用轮数少于 maxIter。
func TestReAct_TokenBudgetTermination(t *testing.T) {
	var order []string
	loopTool := &recordingTool{name: "query", calls: &order}

	// 每次生成消耗 100 tokens；预算 150 → 第 1 轮后（100）继续，第 2 轮后（200≥150）终止。
	tm := &scriptedToolModel{
		script: []*schema.Message{
			toolCallMsg("1", "query"),
			toolCallMsg("2", "query"),
			toolCallMsg("3", "query"),
		},
		perCallTokens: 100,
	}

	a := &agentImpl{
		name:        "react",
		toolModel:   tm,
		prompt:      "p",
		tools:       []tool.BaseTool{loopTool},
		maxIter:     10,
		tokenBudget: 150,
	}

	resp, err := a.chatWithTools(context.Background(), "预算受限查询")
	if err != nil {
		t.Fatalf("chatWithTools: %v", err)
	}
	if resp.Metadata["terminated_by"] != TerminatedByTokenBudget {
		t.Errorf("expected terminated_by=token_budget, got %v", resp.Metadata["terminated_by"])
	}
	if len(order) >= 10 {
		t.Errorf("token budget should terminate well before maxIter, got %d calls", len(order))
	}
	if resp.Metadata["partial"] != true {
		t.Errorf("expected partial=true on budget termination")
	}
	if tokens, ok := resp.Metadata["tokens_used"].(int); !ok || tokens < 150 {
		t.Errorf("expected tokens_used>=150 recorded, got %v", resp.Metadata["tokens_used"])
	}
}
