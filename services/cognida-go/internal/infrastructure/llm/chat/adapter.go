// Package chat provides LLM chat infrastructure implementation
package chat

import (
	"context"
	"fmt"

	"cognida/internal/pkg/safego"

	domain_conversation "cognida/internal/model/conversation"
)

// ========================================
// Domain LLMChatService Adapter
// ========================================

// ChatServiceAdapter 适配器，将基础设施层的 Chat 实现适配为领域层的 LLMChatService 接口
type ChatServiceAdapter struct {
	chat Chat
}

// NewChatServiceAdapter 创建聊天服务适配器
func NewChatServiceAdapter(chat Chat) *ChatServiceAdapter {
	return &ChatServiceAdapter{
		chat: chat,
	}
}

// Chat 执行非流式聊天
func (a *ChatServiceAdapter) Chat(ctx context.Context, messages []*domain_conversation.ConversationMessage, opts *domain_conversation.ConversationOptions) (*domain_conversation.ChatResult, error) {
	// 转换消息格式
	chatMessages := make([]Message, len(messages))
	for i, msg := range messages {
		chatMessages[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 转换选项
	chatOpts := &ChatOptions{}
	if opts != nil {
		chatOpts = &ChatOptions{
			Temperature:      opts.Temperature,
			TopP:             opts.TopP,
			MaxTokens:        opts.MaxTokens,
			FrequencyPenalty: opts.FrequencyPenalty,
			PresencePenalty:  opts.PresencePenalty,
		}
		if opts.Thinking {
			chatOpts.Thinking = &opts.Thinking
		}
	}

	// 调用基础设施层
	resp, err := a.chat.Chat(ctx, chatMessages, chatOpts)
	if err != nil {
		return nil, fmt.Errorf("chat failed: %w", err)
	}

	// 转换响应
	toolCalls := make([]domain_conversation.ToolCall, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		toolCalls[i] = domain_conversation.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: domain_conversation.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}

	return &domain_conversation.ChatResult{
		MessageID:    resp.MessageID,
		Content:      resp.Content,
		Role:         resp.Role,
		TokenCount:   resp.TokenCount,
		ToolCalls:    toolCalls,
		FinishReason: resp.FinishReason,
	}, nil
}

// ChatStream 执行流式聊天
func (a *ChatServiceAdapter) ChatStream(ctx context.Context, messages []*domain_conversation.ConversationMessage, opts *domain_conversation.ConversationOptions) (<-chan *domain_conversation.ChatStreamChunk, error) {
	// 转换消息格式
	chatMessages := make([]Message, len(messages))
	for i, msg := range messages {
		chatMessages[i] = Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 转换选项
	chatOpts := &ChatOptions{}
	if opts != nil {
		chatOpts = &ChatOptions{
			Temperature:      opts.Temperature,
			TopP:             opts.TopP,
			MaxTokens:        opts.MaxTokens,
			FrequencyPenalty: opts.FrequencyPenalty,
			PresencePenalty:  opts.PresencePenalty,
		}
		if opts.Thinking {
			thinking := opts.Thinking
			chatOpts.Thinking = &thinking
		}
	}

	// 调用基础设施层
	respChan, err := a.chat.ChatStream(ctx, chatMessages, chatOpts)
	if err != nil {
		return nil, fmt.Errorf("chat stream failed: %w", err)
	}

	// 转换响应通道
	resultChan := make(chan *domain_conversation.ChatStreamChunk, 10)
	go func() {
		defer safego.Recover("chat-adapter-stream")
		defer close(resultChan)
		for resp := range respChan {
			toolCalls := make([]domain_conversation.ToolCall, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				toolCalls[i] = domain_conversation.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: domain_conversation.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}

			// 将 Event 映射到 Done
			done := resp.Event == "end"

			var err error
			if resp.Error != "" {
				err = fmt.Errorf("chat stream error: %s", resp.Error)
			}

			resultChan <- &domain_conversation.ChatStreamChunk{
				Event:      resp.Event,
				Content:    resp.Content,
				MessageID:  resp.MessageID,
				TokenCount: resp.TokenCount,
				ToolCalls:  toolCalls,
				Done:       done,
				Error:      err,
			}
		}
	}()

	return resultChan, nil
}

// GetModelInfo 获取模型信息
func (a *ChatServiceAdapter) GetModelInfo(ctx context.Context) (*domain_conversation.ModelInfo, error) {
	return &domain_conversation.ModelInfo{
		Name:        a.chat.GetModelName(),
		DisplayName: a.chat.GetModelName(),
		Provider:    "llm",
		MaxTokens:   4096, // 默认值
	}, nil
}
