// EOF 语义测试：流式 Recv 的 io.EOF 是正常结束，非 EOF 错误必须上报（3.4.2）。
package framework

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// errStreamModel 流式桩：依次发出 chunks，然后可选注入一个非 EOF 错误；
// 不注入错误时直接 Close（Recv 得到 io.EOF，即正常结束）。
type errStreamModel struct {
	chunks []string
	err    error
}

func (m *errStreamModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "unused"}, nil
}

func (m *errStreamModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](len(m.chunks) + 1)
	go func() {
		defer writer.Close()
		for _, c := range m.chunks {
			writer.Send(&schema.Message{Role: schema.Assistant, Content: c}, nil)
		}
		if m.err != nil {
			writer.Send(nil, m.err)
		}
	}()
	return reader, nil
}

func (m *errStreamModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

// collect 运行流式函数并收集全部 chunk。
func collect(run func(ch chan *Chunk)) []*Chunk {
	ch := make(chan *Chunk, 64)
	go func() {
		run(ch)
		close(ch)
	}()
	var out []*Chunk
	for c := range ch {
		out = append(out, c)
	}
	return out
}

// lastEvent 取最后一个 chunk 的 event 元数据。
func lastEvent(t *testing.T, chunks []*Chunk) (string, *Chunk) {
	t.Helper()
	if len(chunks) == 0 {
		t.Fatal("未收到任何 chunk")
	}
	last := chunks[len(chunks)-1]
	ev, _ := last.Metadata["event"].(string)
	return ev, last
}

// ========================================
// streamWithTools
// ========================================

// TestStreamWithTools_NonEOFErrorReported 注入非 EOF 错误：
// 旧实现把所有 Recv 错误当"流结束"静默截断，新实现必须上报 error 事件。
func TestStreamWithTools_NonEOFErrorReported(t *testing.T) {
	a := &agentImpl{
		name:      "t",
		toolModel: &errStreamModel{chunks: []string{"部分"}, err: fmt.Errorf("network reset")},
		maxIter:   3,
	}

	chunks := collect(func(ch chan *Chunk) {
		// 收敛后经统一主干 run + streamSink 驱动；collect 负责 close(ch)，run 不关闭。
		sink := &streamSink{a: a, ch: ch}
		a.run(context.Background(), runRequest{message: "hi", rawMessage: "hi"}, sink)
	})

	ev, last := lastEvent(t, chunks)
	if ev != string(EventError) {
		t.Fatalf("最后事件 = %q, 期望 %q（非 EOF 错误被静默吞掉）", ev, EventError)
	}
	if !last.Done {
		t.Fatal("error 事件应携带 Done=true")
	}
	errMsg, _ := last.Metadata["error"].(string)
	if !strings.Contains(errMsg, "network reset") {
		t.Fatalf("error 元数据 = %q, 期望包含 network reset", errMsg)
	}
}

// TestStreamWithTools_EOFNormalEnd EOF（writer.Close）→ 正常 end 事件，无 error。
func TestStreamWithTools_EOFNormalEnd(t *testing.T) {
	a := &agentImpl{
		name:      "t",
		toolModel: &errStreamModel{chunks: []string{"你好", "世界"}},
		maxIter:   3,
	}

	chunks := collect(func(ch chan *Chunk) {
		// 收敛后经统一主干 run + streamSink 驱动；collect 负责 close(ch)，run 不关闭。
		sink := &streamSink{a: a, ch: ch}
		a.run(context.Background(), runRequest{message: "hi", rawMessage: "hi"}, sink)
	})

	var content string
	for _, c := range chunks {
		if ev, _ := c.Metadata["event"].(string); ev == string(EventError) {
			t.Fatalf("EOF 正常结束不应产生 error 事件: %v", c.Metadata["error"])
		}
		content += c.Content
	}
	ev, last := lastEvent(t, chunks)
	if ev != string(EventEnd) || !last.Done {
		t.Fatalf("最后事件 = %q Done=%v, 期望 end/true", ev, last.Done)
	}
	if content != "你好世界" {
		t.Fatalf("拼接内容 = %q, 期望 你好世界", content)
	}
}

// ========================================
// streamWithoutTools
// ========================================

// TestStreamWithoutTools_EOFNormalEnd 旧实现把 io.EOF 也当错误上报；
// 新实现 EOF → end 事件。
func TestStreamWithoutTools_EOFNormalEnd(t *testing.T) {
	a := &agentImpl{
		name:  "t",
		model: &errStreamModel{chunks: []string{"答案"}},
	}

	chunks := collect(func(ch chan *Chunk) {
		sink := &streamSink{a: a, ch: ch}
		a.run(context.Background(), runRequest{message: "hi", rawMessage: "hi"}, sink)
	})

	ev, last := lastEvent(t, chunks)
	if ev != "end" || !last.Done {
		t.Fatalf("最后事件 = %q Done=%v, 期望 end/true（EOF 被误报为错误）", ev, last.Done)
	}
	for _, c := range chunks {
		if e, _ := c.Metadata["event"].(string); e == "error" {
			t.Fatalf("EOF 正常结束不应产生 error 事件: %v", c.Metadata["error"])
		}
	}
}

// TestStreamWithoutTools_NonEOFErrorReported 非 EOF 错误仍然上报。
func TestStreamWithoutTools_NonEOFErrorReported(t *testing.T) {
	a := &agentImpl{
		name:  "t",
		model: &errStreamModel{err: fmt.Errorf("upstream 502")},
	}

	chunks := collect(func(ch chan *Chunk) {
		sink := &streamSink{a: a, ch: ch}
		a.run(context.Background(), runRequest{message: "hi", rawMessage: "hi"}, sink)
	})

	ev, last := lastEvent(t, chunks)
	if ev != "error" || !last.Done {
		t.Fatalf("最后事件 = %q Done=%v, 期望 error/true", ev, last.Done)
	}
	errMsg, _ := last.Metadata["error"].(string)
	if !strings.Contains(errMsg, "upstream 502") {
		t.Fatalf("error 元数据 = %q, 期望包含 upstream 502", errMsg)
	}
}
