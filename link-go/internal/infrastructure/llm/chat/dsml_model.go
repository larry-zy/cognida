package chat

import (
	"context"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// dsmlNormalizingModel 是一层「透明装饰器」，包裹任意 model.ToolCallingChatModel。
//
// 它把某些 DeepSeek 系模型以自有 DSML 标记内联在正文里的工具调用（详见 dsml.go）归一化回
// 结构化的 schema.ToolCall，并从用户可见正文里剥掉标记文本。装饰器与具体 provider 无关：
// 不管内层是 OpenAI 兼容客户端还是别的实现，只要它把 DSML 泄漏进 content，这一层都会兜住，
// 从而避免在每个 provider 客户端里重复同一套解析逻辑（与 llm/resilience 的装饰器思路一致）。
//
// 仅当内层未给出结构化 tool_calls、且正文中确实含 DSML 起始标记时才介入；正常路径零改动透传。
type dsmlNormalizingModel struct {
	inner model.ToolCallingChatModel
}

// NewDSMLNormalizingModel 用 DSML 归一化装饰器包裹内层模型。
func NewDSMLNormalizingModel(inner model.ToolCallingChatModel) model.ToolCallingChatModel {
	return &dsmlNormalizingModel{inner: inner}
}

// Generate 非流式路径：拿到完整消息后做一次 DSML 归一化。
func (m *dsmlNormalizingModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	msg, err := m.inner.Generate(ctx, messages, opts...)
	if err != nil || msg == nil {
		return msg, err
	}
	return normalizeDSMLMessage(msg), nil
}

// Stream 流式路径：把内层的 content 分片喂过 dsmlFilter，跨分片检测并剥除 DSML 块，
// 流结束时把解析出的工具调用合成为一条最终 assistant 消息下发。
func (m *dsmlNormalizingModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	inner, err := m.inner.Stream(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	reader, writer := schema.Pipe[*schema.Message](10)

	go func() {
		defer writer.Close()
		defer inner.Close()

		filter := &dsmlFilter{}
		sentAny := false
		send := func(m *schema.Message) {
			writer.Send(m, nil)
			sentAny = true
		}

		// 累积透传消息里的 reasoning_content（thinking 分片），在流末附加到合成的工具调用消息上，
		// 以满足 DeepSeek「工具调用轮需回传 reasoning_content」的要求。
		// 注意：当前内层 openaiClient.Stream 在「无结构化 tool_calls」的分支里不会把 reasoning
		// 作为独立消息下发（它内部缓冲、仅附加到结构化 tool_calls 消息上），因此对 DSML 流式路径
		// 这里通常拿不到 reasoning——属内层限制。此处仍做累积：一是对会以独立分片下发 reasoning 的
		// 内层实现前向兼容，二是保持与非流式 Generate 路径（reasoning 已随消息带出）语义一致。
		var reasoning strings.Builder

		for {
			msg, recvErr := inner.Recv()
			if recvErr == io.EOF {
				tail, calls := filter.finish()
				if tail != "" {
					send(&schema.Message{Role: schema.Assistant, Content: tail})
				}
				if len(calls) > 0 {
					send(&schema.Message{
						Role:             schema.Assistant,
						Content:          "",
						ReasoningContent: reasoning.String(),
						ToolCalls:        calls,
					})
				}
				// 保留「至少下发一条」契约：DSML 块被整体剥除且未解析出调用时，
				// 内层已置 hasSentData 而不再补空消息，这里兜一条空 assistant 消息。
				if !sentAny {
					writer.Send(&schema.Message{Role: schema.Assistant, Content: ""}, nil)
				}
				return
			}
			if recvErr != nil {
				writer.Send(nil, recvErr)
				return
			}
			if msg == nil {
				continue
			}

			// 内层已给出结构化工具调用：正常路径，原样透传（其自带的 reasoning 一并保留）。
			if len(msg.ToolCalls) > 0 {
				send(msg)
				continue
			}

			if msg.ReasoningContent != "" {
				reasoning.WriteString(msg.ReasoningContent)
			}

			if msg.Content != "" {
				visible := filter.feed(msg.Content)
				if visible != "" {
					out := *msg
					out.Content = visible
					// reasoning 已单独累积，避免随可见正文重复下发。
					out.ReasoningContent = ""
					send(&out)
				}
				continue
			}

			// 到此：既无正文也无结构化工具调用。仅承载 reasoning 的分片已被累积，直接丢弃避免
			// 重复下发；真正的空结尾消息（reasoning 也为空）才透传，以保留结尾契约。
			if msg.ReasoningContent == "" {
				send(msg)
			}
		}
	}()

	return reader, nil
}

// WithTools 透传绑定工具，并保持外层装饰不变。
func (m *dsmlNormalizingModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &dsmlNormalizingModel{inner: inner}, nil
}

// normalizeDSMLMessage 对单条完整消息做 DSML 归一化：仅当没有结构化工具调用、
// 且正文含 DSML 标记时，解析出 schema.ToolCall 并剥除标记文本。
func normalizeDSMLMessage(msg *schema.Message) *schema.Message {
	if len(msg.ToolCalls) > 0 {
		return msg
	}
	if !containsDSMLToolCalls(msg.Content) {
		return msg
	}
	cleaned, calls, found := parseDSMLToolCalls(msg.Content)
	if !found {
		// 含起始标记但未解析出有效 invoke（截断/畸形块）：仍剥除标记文本，
		// 与流式路径一致，避免把原始 DSML 标记泄漏给用户。
		out := *msg
		out.Content = strings.TrimSpace(stripDSMLBlocks(msg.Content))
		return &out
	}
	out := *msg
	out.Content = cleaned
	out.ToolCalls = calls
	return &out
}
