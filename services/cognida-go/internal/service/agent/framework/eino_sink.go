// 由 eino_agent.go 拆出——同包、行为等价（M1 god-file 拆分）。
package framework

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ========================================
// StreamSink：输出侧差异抽象
// ========================================

// execSink 抽象「一次执行主干」的输出侧差异：
// bufferedSink 累积为 *Response（Chat）；streamSink 即时下发 *Chunk（Stream）。
// 主干 run 只依赖本接口，不再为 chat/stream × memory × tools 维护多份变体。
type execSink interface {
	// start 发出起始信号。streaming 发 start 事件；buffered 空实现。
	// 返回 false 表示下游已断开（streaming），主干应静默终止。
	start(ctx context.Context) bool

	// generate 产出一轮完整的 assistant 消息。
	// streaming 实现边收边下发 content 增量，收满后 ConcatMessages 合并返回；
	// buffered 实现直接 Generate。aborted=true 表示下游断开（streaming）。
	generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, iteration int) (msg *schema.Message, aborted bool, err error)

	// onToolCall / onToolResult 下发工具事件。buffered 空实现返回 true。
	// 返回 false 表示下游断开。
	onToolCall(ctx context.Context, ev *ToolCallInStream) bool
	onToolResult(ctx context.Context, ev *ToolCallInStream, iteration int) bool

	// fail 终态错误。streaming 发 error 事件；buffered 空实现（错误经 run 返回值上抛）。
	fail(ctx context.Context, err error)

	// finish 终态成功。buffered 执行 middleware.After + afterHooks（对 *Response 收尾）；
	// streaming 发 end 事件。
	finish(ctx context.Context, response *Response) error
}

// bufferedSink 是 Chat 的输出侧：累积为完整 *Response，收尾时跑 middleware.After + afterHooks。
type bufferedSink struct{ a *agentImpl }

func (s *bufferedSink) start(context.Context) bool { return true }

func (s *bufferedSink) generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, _ int) (*schema.Message, bool, error) {
	msg, err := m.Generate(ctx, msgs)
	return msg, false, err
}

func (s *bufferedSink) onToolCall(context.Context, *ToolCallInStream) bool        { return true }
func (s *bufferedSink) onToolResult(context.Context, *ToolCallInStream, int) bool { return true }
func (s *bufferedSink) fail(context.Context, error)                               {}

// finish 在 buffered 路径执行 middleware.After（逆序）+ afterHooks。
// 注意：流式路径正文已逐块下发、无完整 *Response 可供事后变换，故 After/afterHooks
// 仅在此收敛（Phase 3 刻意保留的输出侧边界，不做愈合）。
func (s *bufferedSink) finish(ctx context.Context, response *Response) error {
	for i := len(s.a.middleware) - 1; i >= 0; i-- {
		if err := s.a.middleware[i].After(ctx, response); err != nil {
			return err
		}
	}
	for _, hook := range s.a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return err
		}
	}
	return nil
}

// streamSink 是 Stream 的输出侧：把每一步即时下发为 *Chunk 事件。
type streamSink struct {
	a  *agentImpl
	ch chan *Chunk
}

func (s *streamSink) start(ctx context.Context) bool {
	return sendChunk(ctx, s.ch, &Chunk{Metadata: map[string]interface{}{"event": "start"}})
}

// generate 流式生成：内容分块即时下发，收满整条流后 ConcatMessages 合并返回。
func (s *streamSink) generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, iteration int) (*schema.Message, bool, error) {
	reader, err := m.Stream(ctx, msgs)
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()

	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if recvErr != nil {
			// io.EOF 是正常结束；其余是真实错误（网络中断等），必须上报而非静默截断。
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return nil, false, recvErr
		}
		if chunk == nil {
			break
		}
		chunks = append(chunks, chunk)

		// 内容分块即时下发以保证流式 UX；reasoning/tool_call 分块留待整流合并。
		if chunk.Content != "" {
			if !sendChunk(ctx, s.ch, &Chunk{
				Content: chunk.Content,
				Metadata: map[string]interface{}{
					"event":     string(EventContent),
					"iteration": iteration,
				},
			}) {
				return nil, true, nil // 客户端已断开
			}
		}
	}

	if len(chunks) == 0 {
		return &schema.Message{Role: schema.Assistant}, false, nil
	}

	// 收满整条流后再合并，得到完整 content / reasoning_content / tool_calls。
	// eino 流式协议下 tool_calls 以 delta 分块到达（按 Index 合并、参数分片拼接），
	// 绝不能只取首个带 tool_calls 的分块——否则参数残缺、assistant.tool_calls 与随后
	// 追加的 tool 消息数目错位，触发 "insufficient tool messages following tool_calls" 400。
	merged, concatErr := schema.ConcatMessages(chunks)
	if concatErr != nil {
		return nil, false, fmt.Errorf("合并流式响应失败: %w", concatErr)
	}
	return merged, false, nil
}

func (s *streamSink) onToolCall(ctx context.Context, ev *ToolCallInStream) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolCall),
			"tool_call": ev,
		},
	})
}

func (s *streamSink) onToolResult(ctx context.Context, ev *ToolCallInStream, iteration int) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolResult),
			"tool_call": ev,
			"iteration": iteration,
		},
	})
}

func (s *streamSink) fail(ctx context.Context, err error) {
	sendChunk(ctx, s.ch, &Chunk{
		Done: true,
		Metadata: map[string]interface{}{
			"event": string(EventError),
			"error": err.Error(),
		},
	})
}

// finish 发送 end 事件，并从 response.Metadata 透传 iterations/terminated_by/partial。
func (s *streamSink) finish(ctx context.Context, response *Response) error {
	meta := map[string]interface{}{"event": string(EventEnd)}
	if it, ok := response.Metadata["iterations"]; ok {
		meta["iterations"] = it
	}
	if tb, ok := response.Metadata["terminated_by"]; ok {
		meta["terminated_by"] = tb
		if tb == TerminatedByMaxIter {
			meta["max_reached"] = true
		}
	}
	if p, ok := response.Metadata["partial"]; ok {
		meta["partial"] = p
	}
	sendChunk(ctx, s.ch, &Chunk{Done: true, Metadata: meta})
	return nil
}

// bufferedStreamSink 是「启用输出护栏的流式会话」的输出侧：正文按缓冲方式产出（不逐块下发
// content），工具事件仍即时下发以保留进度 UX；finish 时先跑 middleware.After + afterHooks
// （含输出护栏脱敏/替换），再把最终安全内容作为单个 content 分块下发，最后发 end 事件。
// 这样兼顾「流式接口契约（仍是 chunk 流）」与「输出护栏必须作用于完整回答」两个约束。
type bufferedStreamSink struct {
	a  *agentImpl
	ch chan *Chunk
}

func (s *bufferedStreamSink) start(ctx context.Context) bool {
	return sendChunk(ctx, s.ch, &Chunk{Metadata: map[string]interface{}{"event": "start"}})
}

// generate 缓冲生成：直接 Generate 出完整消息，不逐块下发 content（护栏需作用于完整回答）。
func (s *bufferedStreamSink) generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, _ int) (*schema.Message, bool, error) {
	msg, err := m.Generate(ctx, msgs)
	return msg, false, err
}

// onToolCall / onToolResult 与 streamSink 一致：工具进度仍即时下发。
func (s *bufferedStreamSink) onToolCall(ctx context.Context, ev *ToolCallInStream) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolCall),
			"tool_call": ev,
		},
	})
}

func (s *bufferedStreamSink) onToolResult(ctx context.Context, ev *ToolCallInStream, iteration int) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolResult),
			"tool_call": ev,
			"iteration": iteration,
		},
	})
}

func (s *bufferedStreamSink) fail(ctx context.Context, err error) {
	sendChunk(ctx, s.ch, &Chunk{
		Done: true,
		Metadata: map[string]interface{}{
			"event": string(EventError),
			"error": err.Error(),
		},
	})
}

// finish 先跑 middleware.After（逆序）+ afterHooks（含输出护栏），对完整 *Response 收尾脱敏，
// 再一次性下发最终安全内容与 end 事件。
func (s *bufferedStreamSink) finish(ctx context.Context, response *Response) error {
	for i := len(s.a.middleware) - 1; i >= 0; i-- {
		if err := s.a.middleware[i].After(ctx, response); err != nil {
			return err
		}
	}
	for _, hook := range s.a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return err
		}
	}

	// 下发最终（已过护栏）内容为单个 content 分块。
	if response.Content != "" {
		if !sendChunk(ctx, s.ch, &Chunk{
			Content: response.Content,
			Metadata: map[string]interface{}{
				"event":    string(EventContent),
				"buffered": true,
			},
		}) {
			return nil // 客户端已断开
		}
	}

	// end 事件：透传与 streamSink.finish 一致的元数据。
	meta := map[string]interface{}{"event": string(EventEnd)}
	if it, ok := response.Metadata["iterations"]; ok {
		meta["iterations"] = it
	}
	if tb, ok := response.Metadata["terminated_by"]; ok {
		meta["terminated_by"] = tb
		if tb == TerminatedByMaxIter {
			meta["max_reached"] = true
		}
	}
	if p, ok := response.Metadata["partial"]; ok {
		meta["partial"] = p
	}
	sendChunk(ctx, s.ch, &Chunk{Done: true, Metadata: meta})
	return nil
}
