package framework

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
	lastInput     []*schema.Message // 最近一次 Generate 收到的输入（供断言注入的提示/观察）
	perCallTokens int
}

func (m *scriptedToolModel) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.calls++
	m.lastInput = input
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

	resp, err := a.Chat(context.Background(), "分析趋势")
	if err != nil {
		t.Fatalf("Chat: %v", err)
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

// TestReAct_EmptyFinishAfterToolsWindsDown 验证：已跑过工具轮后，模型返回「无工具调用 + 空正文」
// 的收尾并非有效最终答复——观察结果已在手却过早停口，用户只会拿到空答（前端表现为"卡在半句话"）。
// 应转 wind-down 从已有观察合成一份自洽答复，并标注 terminated_by=empty_finish、partial=true。
func TestReAct_EmptyFinishAfterToolsWindsDown(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 脚本：先 query（产生观察），再返回「无工具调用 + 空内容」的过早收尾；
	// 脚本随即耗尽，wind-down（第 3 次 Generate）走桩默认收尾语 "final"。
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

	resp, err := a.Chat(context.Background(), "工具后空收尾")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "final" {
		t.Errorf("empty finish after tools must wind-down to a real answer, got %q", resp.Content)
	}
	if resp.Metadata["terminated_by"] != TerminatedByEmptyFinish {
		t.Errorf("expected terminated_by=empty_finish, got %v", resp.Metadata["terminated_by"])
	}
	if resp.Metadata["partial"] != true {
		t.Errorf("wind-down recovery must be marked partial")
	}
	// query（观察）+ 空收尾 + wind-down 合成 = 3 次生成；query 工具只跑一次。
	if tm.calls != 3 {
		t.Errorf("expected 3 Generate calls (query + empty finish + wind-down), got %d", tm.calls)
	}
	if len(order) != 1 || order[0] != "query" {
		t.Errorf("expected query invoked once, got %v", order)
	}
}

// TestReAct_EmptyNaturalFinish_NoTools 验证：首轮即返回「无工具调用 + 空内容」（没有跑过任何工具、
// 无观察可合成）时仍判为自然结束——不误标 terminated_by、不触发 wind-down / partial。
// 这是与 TestReAct_EmptyFinishAfterToolsWindsDown 互补的另一半：只在「有观察」时才兜底。
func TestReAct_EmptyNaturalFinish_NoTools(t *testing.T) {
	var order []string
	qtool := &recordingTool{name: "query", calls: &order}

	// 脚本：首个生成即无工具调用 + 空内容（i==0，无观察）。
	tm := &scriptedToolModel{script: []*schema.Message{
		{Role: schema.Assistant, Content: ""},
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{qtool},
		maxIter:   10,
	}

	resp, err := a.Chat(context.Background(), "首轮空收尾")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if _, ok := resp.Metadata["terminated_by"]; ok {
		t.Errorf("first-turn empty natural finish must not set terminated_by: %v", resp.Metadata)
	}
	if resp.Metadata["partial"] == true {
		t.Errorf("first-turn empty natural finish must not be marked partial")
	}
	// 仅 1 次生成（空收尾）；无观察不触发 wind-down；query 从未被调用。
	if tm.calls != 1 {
		t.Errorf("expected exactly 1 Generate call (empty finish), got %d", tm.calls)
	}
	if len(order) != 0 {
		t.Errorf("expected no tool invocation, got %v", order)
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

	resp, err := a.Chat(context.Background(), "永动查询")
	if err != nil {
		t.Fatalf("Chat: %v", err)
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

// sleepingTool 是一个在执行时消耗挂钟时间的 InvokableTool 桩，用于驱动挂钟护栏路径
// （模拟长 SQL / 子代理委派阻塞让单步极慢）。
type sleepingTool struct {
	name  string
	delay time.Duration
	calls *[]string
}

func (t *sleepingTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name, Desc: "sleeping tool " + t.name}, nil
}

func (t *sleepingTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	time.Sleep(t.delay)
	*t.calls = append(*t.calls, t.name)
	return "ok:" + t.name, nil
}

// TestReAct_WallClockTermination 验证挂钟预算耗尽时循环提前终止并 wind-down 收尾，
// terminated_by=deadline、partial=true，且在越线的那一步（工具阻塞后）就地止损。
// 收敛「步数/计费 token 都没到上限、却因单步极慢让用户干等」的「感知到的卡住」。
func TestReAct_WallClockTermination(t *testing.T) {
	var order []string
	// 工具单次执行 40ms > 挂钟预算 20ms：i=0 前置检查（≈0ms）放行 → 生成工具调用 →
	// 工具阻塞 40ms → 工具执行后挂钟检查（≈40ms≥20ms）越线 → 终止，恰好 1 次工具调用。
	slow := &sleepingTool{name: "query", delay: 40 * time.Millisecond, calls: &order}

	// 脚本：首轮返回工具调用触发阻塞越线；脚本随即耗尽，wind-down 生成走默认收尾语（非空）。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
	}}

	a := &agentImpl{
		name:      "react",
		toolModel: tm,
		prompt:    "p",
		tools:     []tool.BaseTool{slow},
		maxIter:   10,
		wallClock: 20 * time.Millisecond,
	}

	resp, err := a.Chat(context.Background(), "长耗时查询")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Metadata["terminated_by"] != TerminatedByDeadline {
		t.Errorf("expected terminated_by=deadline, got %v", resp.Metadata["terminated_by"])
	}
	if len(order) >= 10 {
		t.Errorf("wall clock should terminate well before maxIter, got %d calls", len(order))
	}
	if resp.Metadata["partial"] != true {
		t.Errorf("expected partial=true on deadline termination")
	}
	if resp.Content == "" {
		t.Errorf("wind-down must fill a non-empty partial conclusion")
	}
}

// blockingTool 是一个忽略 ctx、纯阻塞直到被外部信号放行的 InvokableTool 桩，
// 用于验证「工具自身不响应 ctx 取消」时统一超时兜底仍能让循环及时返回。
type blockingTool struct {
	name    string
	release chan struct{} // 关闭即放行，避免 goroutine 永久泄漏
	started chan struct{}
}

func (t *blockingTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name, Desc: "blocking tool " + t.name}, nil
}

func (t *blockingTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	if t.started != nil {
		close(t.started)
	}
	<-t.release // 无视传入 ctx，只认外部放行信号
	return "released:" + t.name, nil
}

// TestReAct_ToolTimeout 验证单次工具超时兜底：即便工具忽略 ctx、纯阻塞，invokeTool 也在
// toolTimeout 到点后返回一次可恢复的超时观察，不把 ReAct 循环拖死（堵住「单步 hang 使 wallClock 失效」）。
func TestReAct_ToolTimeout(t *testing.T) {
	release := make(chan struct{})
	defer close(release) // 测试结束放行遗弃的 goroutine，防泄漏
	blocker := &blockingTool{name: "query", release: release, started: make(chan struct{})}

	// 脚本：首轮调用阻塞工具触发超时；随后脚本耗尽，wind-down 走默认收尾语。
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
	}}

	a := &agentImpl{
		name:        "react",
		toolModel:   tm,
		prompt:      "p",
		tools:       []tool.BaseTool{blocker},
		maxIter:     10,
		toolTimeout: 30 * time.Millisecond,
	}

	resp, err := a.Chat(context.Background(), "调用会卡死的工具")
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	// 循环没有被阻塞工具拖死：正常返回（wind-down 收尾）。
	if resp == nil {
		t.Fatal("expected a response despite the blocking tool")
	}
	// 阻塞工具确实被调用过（started 关闭），但超时后循环继续而非卡死。
	select {
	case <-blocker.started:
	default:
		t.Error("blocking tool should have been invoked")
	}
	// 超时观察应记为一次工具失败并进入历史（tool_calls 里带 error）。
	var sawTimeoutErr bool
	for _, tc := range resp.ToolCalls {
		if tc.Name == "query" && tc.Error != nil && strings.Contains(tc.Error.Error(), "执行超过") {
			sawTimeoutErr = true
		}
	}
	if !sawTimeoutErr {
		t.Errorf("expected a per-tool timeout error recorded in tool calls, got %+v", resp.ToolCalls)
	}
}

// TestEffectiveToolTimeout 验证委派类协作元工具拿到比通用 toolTimeout 更宽的单工具挂钟上限
// （collabToolWallClock，须高于内部 delegationTimeout），避免通用 90s 先掐断三路并行委派、
// 退化回主循环内联（历史 bug）；普通工具仍用通用 toolTimeout；toolTimeout<=0（不限）时一律 0。
func TestEffectiveToolTimeout(t *testing.T) {
	a := &agentImpl{toolTimeout: 90 * time.Second}

	// 委派类元工具须超过其内部 delegationTimeout，让内部超时先触发产出优雅信封。
	for _, name := range []string{"delegate_to_agent", "delegate_parallel", "ask_agent", "handoff_to"} {
		got := a.effectiveToolTimeout(name)
		if got != collabToolWallClock {
			t.Errorf("%s: expected collabToolWallClock=%s, got %s", name, collabToolWallClock, got)
		}
		if got <= delegationTimeout {
			t.Errorf("%s: 挂钟上限 %s 必须高于内部 delegationTimeout %s，否则内部优雅超时无从先触发",
				name, got, delegationTimeout)
		}
	}

	// 普通工具仍用通用 toolTimeout。
	if got := a.effectiveToolTimeout("sql_execute"); got != a.toolTimeout {
		t.Errorf("普通工具应用通用 toolTimeout=%s，got %s", a.toolTimeout, got)
	}

	// toolTimeout<=0（不限）时协作工具也不额外设限，返回 0（跳过超时包裹）。
	unlimited := &agentImpl{toolTimeout: 0}
	if got := unlimited.effectiveToolTimeout("delegate_parallel"); got != 0 {
		t.Errorf("toolTimeout=0 时应返回 0（不设限），got %s", got)
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

	resp, err := a.Chat(context.Background(), "预算受限查询")
	if err != nil {
		t.Fatalf("Chat: %v", err)
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
