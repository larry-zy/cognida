package orchestration

import (
	"context"
	"testing"

	"link/internal/service/agent/framework"

	"github.com/stretchr/testify/assert"
)

// mockToolAgent 返回带工具调用轨迹的响应，用于验证 Sequential 跨段累积 ToolCalls。
type mockToolAgent struct {
	name     string
	response string
	tools    []string
}

func (m *mockToolAgent) Chat(ctx context.Context, message string) (*framework.Response, error) {
	calls := make([]*framework.ToolCall, 0, len(m.tools))
	for _, t := range m.tools {
		calls = append(calls, &framework.ToolCall{Name: t})
	}
	return &framework.Response{Content: m.response, ToolCalls: calls}, nil
}

func (m *mockToolAgent) Stream(ctx context.Context, message string) (<-chan *framework.Chunk, error) {
	ch := make(chan *framework.Chunk, 1)
	close(ch)
	return ch, nil
}

func (m *mockToolAgent) Name() string { return m.name }

// TestSequential_TwoAgents tests Sequential with two agents.
func TestSequential_TwoAgents(t *testing.T) {
	firstAgent := &mockAgentForOrchestration{name: "first", response: "first response"}
	secondAgent := &mockAgentForOrchestration{name: "second", response: "second response"}

	seqAgent := Sequential(firstAgent, secondAgent)

	ctx := context.Background()
	resp, err := seqAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	// Sequential passes output of one agent to the next
	// So the final response is just "second response"
	assert.Equal(t, "second response", resp.Content)
}

// TestSequential_ThreeAgents tests Sequential with three agents.
func TestSequential_ThreeAgents(t *testing.T) {
	firstAgent := &mockAgentForOrchestration{name: "first", response: "first"}
	secondAgent := &mockAgentForOrchestration{name: "second", response: "second"}
	thirdAgent := &mockAgentForOrchestration{name: "third", response: "third"}

	seqAgent := Sequential(firstAgent, secondAgent, thirdAgent)

	ctx := context.Background()
	resp, err := seqAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	// Sequential passes output through chain, so final is "third"
	assert.Equal(t, "third", resp.Content)
}

// TestSequential_Empty tests Sequential with no agents.
func TestSequential_Empty(t *testing.T) {
	seqAgent := Sequential()

	ctx := context.Background()
	_, err := seqAgent.Chat(ctx, "test")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agents provided")
}

// TestSequential_AccumulatesToolCalls 验证 Sequential 把各段的工具调用轨迹按序合并到
// 最终响应——回归 Plan-Execute-Reflect（text2sql）等管道中，Plan/Execute 段真实发生的
// get_schema/sql_execute 轨迹曾被末段（Reflect）响应覆盖丢弃，导致评测轨迹指标恒为 0 的缺陷。
func TestSequential_AccumulatesToolCalls(t *testing.T) {
	plan := &mockToolAgent{name: "plan", response: "planned", tools: []string{"get_schema"}}
	exec := &mockToolAgent{name: "execute", response: "executed", tools: []string{"sql_execute", "data_analysis"}}
	reflect := &mockToolAgent{name: "reflect", response: "final", tools: nil} // 末段无工具

	seqAgent := Sequential(plan, exec, reflect)

	resp, err := seqAgent.Chat(context.Background(), "test")
	assert.NoError(t, err)
	// 内容仍取末段
	assert.Equal(t, "final", resp.Content)
	// 轨迹按 Plan→Execute→Reflect 顺序完整保留，而非仅末段的空轨迹
	got := make([]string, 0, len(resp.ToolCalls))
	for _, tc := range resp.ToolCalls {
		got = append(got, tc.Name)
	}
	assert.Equal(t, []string{"get_schema", "sql_execute", "data_analysis"}, got)
}

// TestSequential_SingleAgent tests Sequential with one agent.
func TestSequential_SingleAgent(t *testing.T) {
	singleAgent := &mockAgentForOrchestration{name: "single", response: "single response"}

	seqAgent := Sequential(singleAgent)

	ctx := context.Background()
	resp, err := seqAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, "single response", resp.Content)
}
