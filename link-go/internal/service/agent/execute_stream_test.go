package agent

import (
	"context"
	"testing"

	modelagent "link/internal/model/agent"
)

// fakeStreamExecutor 是一个可注入固定事件序列的 AgentExecutor 假实现。
type fakeStreamExecutor struct {
	events []*modelagent.StreamEvent
}

func (f *fakeStreamExecutor) Execute(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}

func (f *fakeStreamExecutor) ExecuteStream(_ context.Context, _ string, _ string) (<-chan *modelagent.StreamEvent, error) {
	ch := make(chan *modelagent.StreamEvent, len(f.events))
	for _, e := range f.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (f *fakeStreamExecutor) GetStatus(_ string) (modelagent.AgentStatus, error) {
	return modelagent.AgentStatusIdle, nil
}

// TestExecuteStream_MapsStructuredEvents 验证领域层结构化事件被正确映射为 DTO：
// content 事件填充 Content，工具事件填充 Metadata，末尾追加 Done 标记。
func TestExecuteStream_MapsStructuredEvents(t *testing.T) {
	exec := &fakeStreamExecutor{events: []*modelagent.StreamEvent{
		{Type: modelagent.StreamEventContent, Content: "你好"},
		{Type: modelagent.StreamEventToolCall, Step: 1, ToolID: "call_1", ToolName: "sql_query", ToolInput: `{"q":"select 1"}`, Status: "calling"},
		{Type: modelagent.StreamEventToolResult, Step: 1, ToolID: "call_1", ToolName: "sql_query", Output: "1", Status: "success"},
		{Type: modelagent.StreamEventContent, Content: "结果是 1"},
	}}

	svc := NewExecuteService(exec)

	ch, err := svc.ExecuteStream(context.Background(), &AgenticRAGRequest{Query: "查询"})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var chunks []*ChatChunkDTO
	for c := range ch {
		chunks = append(chunks, c)
	}

	// 4 个事件 + 1 个 Done 标记
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}

	// chunk[0]: content
	if chunks[0].Content != "你好" || chunks[0].Metadata != nil {
		t.Errorf("chunk[0] expected content-only, got %+v", chunks[0])
	}

	// chunk[1]: tool_call
	m1 := chunks[1].Metadata
	if m1 == nil {
		t.Fatalf("chunk[1] expected metadata for tool_call")
	}
	if m1["type"] != "tool_call" || m1["tool_name"] != "sql_query" || m1["status"] != "calling" {
		t.Errorf("chunk[1] metadata mismatch: %+v", m1)
	}
	if m1["tool_input"] != `{"q":"select 1"}` {
		t.Errorf("chunk[1] tool_input mismatch: %v", m1["tool_input"])
	}
	if m1["step"] != 1 {
		t.Errorf("chunk[1] step mismatch: %v", m1["step"])
	}

	// chunk[2]: tool_result
	m2 := chunks[2].Metadata
	if m2 == nil || m2["type"] != "tool_result" || m2["tool_output"] != "1" || m2["status"] != "success" {
		t.Errorf("chunk[2] metadata mismatch: %+v", m2)
	}

	// chunk[3]: content
	if chunks[3].Content != "结果是 1" {
		t.Errorf("chunk[3] content mismatch: %q", chunks[3].Content)
	}

	// chunk[4]: Done
	if !chunks[4].Done {
		t.Errorf("chunk[4] expected Done=true, got %+v", chunks[4])
	}
}
