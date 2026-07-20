package chat

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeToolModel 是一个可编排的内层 ToolCallingChatModel，用于驱动装饰器测试。
// genMsg 决定 Generate 返回的消息；streamChunks 决定 Stream 逐条下发的消息。
type fakeToolModel struct {
	genMsg       *schema.Message
	streamChunks []*schema.Message
	withToolsErr error
	lastTools    []*schema.ToolInfo
}

func (f *fakeToolModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.genMsg, nil
}

func (f *fakeToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](10)
	go func() {
		defer writer.Close()
		for _, m := range f.streamChunks {
			writer.Send(m, nil)
		}
	}()
	return reader, nil
}

func (f *fakeToolModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if f.withToolsErr != nil {
		return nil, f.withToolsErr
	}
	f.lastTools = tools
	return f, nil
}

// drain 把装饰器输出的流收敛为可见正文与全部工具调用。
func drain(t *testing.T, sr *schema.StreamReader[*schema.Message]) (string, []schema.ToolCall) {
	t.Helper()
	var visible strings.Builder
	var calls []schema.ToolCall
	for {
		msg, err := sr.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
		visible.WriteString(msg.Content)
		calls = append(calls, msg.ToolCalls...)
	}
	return visible.String(), calls
}

func TestDSMLModel_Generate_ParsesInlineToolCall(t *testing.T) {
	inner := &fakeToolModel{genMsg: &schema.Message{Role: schema.Assistant, Content: realDSMLContent}}
	m := NewDSMLNormalizingModel(inner)

	msg, err := m.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "data_analysis" {
		t.Fatalf("expected 1 data_analysis tool call, got %d", len(msg.ToolCalls))
	}
	if strings.Contains(msg.Content, "DSML") {
		t.Errorf("cleaned content still leaks DSML markers: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "现在用分析引擎") {
		t.Errorf("cleaned content dropped prose: %q", msg.Content)
	}
}

func TestDSMLModel_Generate_StructuredPassThrough(t *testing.T) {
	structured := &schema.Message{
		Role:      schema.Assistant,
		Content:   "普通回答",
		ToolCalls: []schema.ToolCall{{ID: "x", Type: "function", Function: schema.FunctionCall{Name: "already"}}},
	}
	m := NewDSMLNormalizingModel(&fakeToolModel{genMsg: structured})

	msg, err := m.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "already" {
		t.Errorf("structured tool call should pass through unchanged, got %+v", msg.ToolCalls)
	}
	if msg.Content != "普通回答" {
		t.Errorf("content should be untouched, got %q", msg.Content)
	}
}

func TestDSMLModel_Stream_ParsesInlineToolCall(t *testing.T) {
	// 把真实 DSML 内容按每 4 字节切片，模拟标记被切在任意分片边界。
	var chunks []*schema.Message
	b := []byte(realDSMLContent)
	for i := 0; i < len(b); i += 4 {
		end := i + 4
		if end > len(b) {
			end = len(b)
		}
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, Content: string(b[i:end])})
	}
	m := NewDSMLNormalizingModel(&fakeToolModel{streamChunks: chunks})

	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	visible, calls := drain(t, sr)

	if strings.Contains(visible, "DSML") {
		t.Errorf("streamed visible text leaked DSML markers: %q", visible)
	}
	if !strings.Contains(visible, "现在用分析引擎") {
		t.Errorf("streamed visible text dropped prose: %q", visible)
	}
	if len(calls) != 1 || calls[0].Function.Name != "data_analysis" {
		t.Fatalf("expected 1 data_analysis call from stream, got %d", len(calls))
	}
}

func TestDSMLModel_Stream_StructuredAndReasoningPassThrough(t *testing.T) {
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "先说点正文。"},
		{Role: schema.Assistant, ReasoningContent: "思考中"},
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{ID: "1", Type: "function", Function: schema.FunctionCall{Name: "sql_execute", Arguments: `{"sql":"SELECT 1"}`}},
		}},
	}
	m := NewDSMLNormalizingModel(&fakeToolModel{streamChunks: chunks})

	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	visible, calls := drain(t, sr)

	if visible != "先说点正文。" {
		t.Errorf("plain content altered: %q", visible)
	}
	if len(calls) != 1 || calls[0].Function.Name != "sql_execute" {
		t.Fatalf("structured tool call should pass through, got %d", len(calls))
	}
}

// TestDSMLModel_Stream_MalformedBlockStillEmits：DSML 起始标记出现但没有有效 invoke，
// 整块被剥除、无工具调用；装饰器仍须兜一条消息，不能让下游收到零消息流。
func TestDSMLModel_Stream_MalformedBlockStillEmits(t *testing.T) {
	chunks := []*schema.Message{
		{Role: schema.Assistant, Content: "<｜｜DSML｜｜tool_calls></｜｜DSML｜｜tool_calls>"},
	}
	m := NewDSMLNormalizingModel(&fakeToolModel{streamChunks: chunks})

	sr, err := m.Stream(context.Background(), nil)
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	count := 0
	for {
		msg, err := sr.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		count++
		if strings.Contains(msg.Content, "DSML") {
			t.Errorf("leaked DSML markers: %q", msg.Content)
		}
	}
	if count == 0 {
		t.Errorf("expected at least one message even for a malformed DSML block")
	}
}

// TestDSMLModel_Generate_MalformedBlockStrips：非流式下含标记但无有效 invoke 时，
// 也要剥掉标记文本，与流式路径一致。
func TestDSMLModel_Generate_MalformedBlockStrips(t *testing.T) {
	genMsg := &schema.Message{Role: schema.Assistant, Content: "分析完成。<｜｜DSML｜｜tool_calls></｜｜DSML｜｜tool_calls>"}
	m := NewDSMLNormalizingModel(&fakeToolModel{genMsg: genMsg})

	msg, err := m.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if strings.Contains(msg.Content, "DSML") {
		t.Errorf("malformed block markers leaked to user: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "分析完成。") {
		t.Errorf("legit prose dropped: %q", msg.Content)
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("expected no tool calls from malformed block, got %d", len(msg.ToolCalls))
	}
}

func TestDSMLModel_WithTools(t *testing.T) {
	inner := &fakeToolModel{}
	m := NewDSMLNormalizingModel(inner)
	tools := []*schema.ToolInfo{{Name: "t1"}}

	wrapped, err := m.WithTools(tools)
	if err != nil {
		t.Fatalf("WithTools error: %v", err)
	}
	if _, ok := wrapped.(*dsmlNormalizingModel); !ok {
		t.Errorf("WithTools should preserve the decorator, got %T", wrapped)
	}
	if len(inner.lastTools) != 1 || inner.lastTools[0].Name != "t1" {
		t.Errorf("tools not forwarded to inner model: %+v", inner.lastTools)
	}
}

func TestDSMLModel_WithTools_PropagatesError(t *testing.T) {
	m := NewDSMLNormalizingModel(&fakeToolModel{withToolsErr: errors.New("boom")})
	if _, err := m.WithTools(nil); err == nil {
		t.Errorf("expected WithTools error to propagate")
	}
}
