// 组合矩阵回归测试（3.1）：memory×tool×streaming 八组合驱动统一主干 run。
//
// 全部仅经公开入口 Chat/Stream，锚定「愈合后」的一致行为（Decision C）：
//   - 有工具时，无论有无记忆、buffered 还是 streaming，都跑同一 ToolLoop（工具真的被执行）；
//   - 记忆激活时，无论有无工具、buffered 还是 streaming，都统一构建上下文并落库；
//   - 元数据键位（with_memory / with_tools / iterations / terminated_by / partial）在各组合一致。
//
// 复用既有桩：mockChatModel / scriptedToolModel / recordingTool / errStreamModel /
// fakeContextBuilder / collect / lastEvent；仅新增 recordingMemory 与流式工具桩 scriptedStreamToolModel。
package framework

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainagent "cognida/internal/model/agent"
	"cognida/internal/model/memory"
)

// recordingMemory 线程安全记录记忆写入，用于断言「愈合后」的统一落库行为。
type recordingMemory struct {
	mu        chan struct{} // 简易互斥（1 容量）
	saved     []string
	summaries int
}

func newRecordingMemory() *recordingMemory {
	m := &recordingMemory{mu: make(chan struct{}, 1)}
	m.mu <- struct{}{}
	return m
}

func (m *recordingMemory) lock()   { <-m.mu }
func (m *recordingMemory) unlock() { m.mu <- struct{}{} }

func (m *recordingMemory) SaveMessage(_ context.Context, msg *memory.Message) error {
	m.lock()
	m.saved = append(m.saved, msg.Role+":"+msg.Content)
	m.unlock()
	return nil
}
func (m *recordingMemory) LoadHistoryWithLimit(context.Context, string, int) ([]*memory.Message, error) {
	return nil, nil
}
func (m *recordingMemory) UpdateSummary(context.Context, string, string) error {
	m.lock()
	m.summaries++
	m.unlock()
	return nil
}
func (m *recordingMemory) GetSummary(context.Context, string) (string, error) { return "", nil }

func (m *recordingMemory) savedRoles() []string {
	m.lock()
	defer m.unlock()
	out := make([]string, len(m.saved))
	copy(out, m.saved)
	return out
}

// scriptedStreamToolModel 是流式版可编排工具模型：每次 Stream 把脚本中的下一条消息
// 作为单一分块下发（携带其 ToolCalls / Content），脚本尽后回退为自然收尾。
// 用于在 streaming 路径上真正驱动 ReAct 工具循环（既有 scriptedToolModel.Stream 是空流）。
type scriptedStreamToolModel struct {
	script []*schema.Message
	idx    int
}

func (m *scriptedStreamToolModel) next() *schema.Message {
	if m.idx < len(m.script) {
		out := m.script[m.idx]
		m.idx++
		return out
	}
	return &schema.Message{Role: schema.Assistant, Content: "final"}
}

func (m *scriptedStreamToolModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return m.next(), nil
}

func (m *scriptedStreamToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	out := m.next()
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer writer.Close()
		writer.Send(out, nil)
	}()
	return reader, nil
}

func (m *scriptedStreamToolModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// ---- buffered（Chat）四组合 ----

// TestMatrix_Buffered_NoMem_NoTools 基线：无记忆无工具，单轮生成。
func TestMatrix_Buffered_NoMem_NoTools(t *testing.T) {
	a := &agentImpl{
		name:  "m",
		model: &mockChatModel{generateFunc: func(context.Context, []*schema.Message) (*schema.Message, error) {
			return &schema.Message{Role: schema.Assistant, Content: "R"}, nil
		}},
	}
	resp, err := a.Chat(context.Background(), "hi")
	require.NoError(t, err)
	assert.Equal(t, "R", resp.Content)
	assert.NotContains(t, resp.Metadata, "with_memory")
	assert.NotContains(t, resp.Metadata, "with_tools")
	assert.Equal(t, 1, resp.Metadata["iterations"])
}

// TestMatrix_Buffered_NoMem_Tools 无记忆有工具：工具执行 + 自然收尾。
func TestMatrix_Buffered_NoMem_Tools(t *testing.T) {
	var order []string
	rt := &recordingTool{name: "query", calls: &order}
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		{Role: schema.Assistant, Content: "done"},
	}}
	a := &agentImpl{name: "m", toolModel: tm, prompt: "p", tools: []tool.BaseTool{rt}, maxIter: 5}

	resp, err := a.Chat(context.Background(), "hi")
	require.NoError(t, err)
	assert.Equal(t, "done", resp.Content)
	assert.Equal(t, []string{"query"}, order)
	assert.Equal(t, true, resp.Metadata["with_tools"])
	assert.NotContains(t, resp.Metadata, "with_memory")
	assert.NotContains(t, resp.Metadata, "terminated_by")
}

// TestMatrix_Buffered_Mem_NoTools 有记忆无工具：构建上下文 + 落库（用户同步 + 助手异步）。
func TestMatrix_Buffered_Mem_NoTools(t *testing.T) {
	mem := newRecordingMemory()
	a := &agentImpl{
		name:           "m",
		model:          &mockChatModel{generateFunc: func(context.Context, []*schema.Message) (*schema.Message, error) {
			return &schema.Message{Role: schema.Assistant, Content: "R"}, nil
		}},
		enableMemory:   true,
		contextBuilder: &fakeContextBuilder{},
		memoryService:  mem,
	}
	ctx := domainagent.WithSessionID(context.Background(), "s1")
	resp, err := a.Chat(ctx, "hi")
	require.NoError(t, err)
	assert.Equal(t, "R", resp.Content)
	assert.Equal(t, true, resp.Metadata["with_memory"])
	// 用户消息（原话）在 buildInitialMessages 中同步落库。
	assert.Contains(t, mem.savedRoles(), "user:hi")
	// 助手响应异步落库（愈合：记忆激活即持久化助手响应）。
	assert.Eventually(t, func() bool {
		for _, s := range mem.savedRoles() {
			if s == "assistant:R" {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond, "助手响应应被异步落库")
}

// TestMatrix_Buffered_Mem_Tools 有记忆有工具（愈合关键项）：
// 旧变体在记忆+工具组合下行为分叉；统一后必须同时 with_memory + with_tools 且工具真的执行。
func TestMatrix_Buffered_Mem_Tools(t *testing.T) {
	var order []string
	rt := &recordingTool{name: "query", calls: &order}
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		{Role: schema.Assistant, Content: "done"},
	}}
	mem := newRecordingMemory()
	a := &agentImpl{
		name:           "m",
		toolModel:      tm,
		prompt:         "p",
		tools:          []tool.BaseTool{rt},
		maxIter:        5,
		enableMemory:   true,
		contextBuilder: &fakeContextBuilder{},
		memoryService:  mem,
	}
	ctx := domainagent.WithSessionID(context.Background(), "s1")
	resp, err := a.Chat(ctx, "hi")
	require.NoError(t, err)
	assert.Equal(t, "done", resp.Content)
	assert.Equal(t, []string{"query"}, order, "记忆+工具组合下工具必须真的被执行")
	assert.Equal(t, true, resp.Metadata["with_memory"])
	assert.Equal(t, true, resp.Metadata["with_tools"])
	assert.Contains(t, mem.savedRoles(), "user:hi")
}

// TestMatrix_Buffered_Tools_MaxIter wind-down 在（无记忆）工具路径统一生效的锚点，
// 与后续 streaming 版比对：达 maxIter → terminated_by=max_iter + partial + 非空收尾。
func TestMatrix_Buffered_Tools_MaxIter(t *testing.T) {
	var order []string
	rt := &recordingTool{name: "query", calls: &order}
	tm := &scriptedToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		toolCallMsg("2", "query"),
	}}
	a := &agentImpl{name: "m", toolModel: tm, prompt: "p", tools: []tool.BaseTool{rt}, maxIter: 2}

	resp, err := a.Chat(context.Background(), "loop")
	require.NoError(t, err)
	assert.Len(t, order, 2)
	assert.Equal(t, TerminatedByMaxIter, resp.Metadata["terminated_by"])
	assert.Equal(t, true, resp.Metadata["partial"])
	assert.NotEmpty(t, resp.Content, "wind-down 应给出非空部分结论")
}

// ---- streaming（Stream）四组合 ----

// TestMatrix_Stream_NoMem_NoTools 流式无记忆无工具：内容分块拼接 + end 事件。
func TestMatrix_Stream_NoMem_NoTools(t *testing.T) {
	a := &agentImpl{name: "m", model: &errStreamModel{chunks: []string{"你好", "世界"}}}

	ch, err := a.Stream(context.Background(), "hi")
	require.NoError(t, err)
	var content string
	var chunks []*Chunk
	for c := range ch {
		chunks = append(chunks, c)
		content += c.Content
	}
	assert.Equal(t, "你好世界", content)
	ev, last := lastEvent(t, chunks)
	assert.Equal(t, string(EventEnd), ev)
	assert.True(t, last.Done)
}

// TestMatrix_Stream_Mem_NoTools 流式有记忆无工具（愈合）：走 BuildForCollaboration/Build 并落库。
func TestMatrix_Stream_Mem_NoTools(t *testing.T) {
	mem := newRecordingMemory()
	a := &agentImpl{
		name:           "m",
		model:          &errStreamModel{chunks: []string{"答", "案"}},
		enableMemory:   true,
		contextBuilder: &fakeContextBuilder{},
		memoryService:  mem,
	}
	ctx := domainagent.WithSessionID(context.Background(), "s1")
	ch, err := a.Stream(ctx, "hi")
	require.NoError(t, err)
	var content string
	var chunks []*Chunk
	for c := range ch {
		chunks = append(chunks, c)
		content += c.Content
	}
	assert.Equal(t, "答案", content)
	ev, _ := lastEvent(t, chunks)
	assert.Equal(t, string(EventEnd), ev)
	// 愈合：流式+记忆同样构建上下文并同步落库用户原话。
	assert.Contains(t, mem.savedRoles(), "user:hi")
}

// TestMatrix_Stream_NoMem_Tools 流式无记忆有工具：ReAct 工具循环在流式下同样执行，
// 发出 tool_call/tool_result 事件，末轮自然收尾拼接内容。
func TestMatrix_Stream_NoMem_Tools(t *testing.T) {
	var order []string
	rt := &recordingTool{name: "query", calls: &order}
	sm := &scriptedStreamToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		{Role: schema.Assistant, Content: "done"},
	}}
	a := &agentImpl{name: "m", toolModel: sm, prompt: "p", tools: []tool.BaseTool{rt}, maxIter: 5}

	ch, err := a.Stream(context.Background(), "hi")
	require.NoError(t, err)
	var content string
	var events []string
	var chunks []*Chunk
	for c := range ch {
		chunks = append(chunks, c)
		content += c.Content
		if ev, ok := c.Metadata["event"].(string); ok {
			events = append(events, ev)
		}
	}
	assert.Equal(t, []string{"query"}, order, "流式路径工具必须真的被执行")
	assert.Equal(t, "done", content)
	assert.Contains(t, events, string(EventToolCall))
	assert.Contains(t, events, string(EventToolResult))
	ev, last := lastEvent(t, chunks)
	assert.Equal(t, string(EventEnd), ev)
	assert.True(t, last.Done)
}

// TestMatrix_Stream_Mem_Tools 流式 + 记忆 + 工具（八组合中最全的一项）：
// 三组件全激活——构建上下文、执行工具、流式下发、落库；元数据无回退。
func TestMatrix_Stream_Mem_Tools(t *testing.T) {
	var order []string
	rt := &recordingTool{name: "query", calls: &order}
	sm := &scriptedStreamToolModel{script: []*schema.Message{
		toolCallMsg("1", "query"),
		{Role: schema.Assistant, Content: "done"},
	}}
	mem := newRecordingMemory()
	a := &agentImpl{
		name:           "m",
		toolModel:      sm,
		prompt:         "p",
		tools:          []tool.BaseTool{rt},
		maxIter:        5,
		enableMemory:   true,
		contextBuilder: &fakeContextBuilder{},
		memoryService:  mem,
	}
	ctx := domainagent.WithSessionID(context.Background(), "s1")
	ch, err := a.Stream(ctx, "hi")
	require.NoError(t, err)
	var content string
	for c := range ch {
		content += c.Content
	}
	assert.Equal(t, []string{"query"}, order)
	assert.Equal(t, "done", content)
	assert.Contains(t, mem.savedRoles(), "user:hi")
}
