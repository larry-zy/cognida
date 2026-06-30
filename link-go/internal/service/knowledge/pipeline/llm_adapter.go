// Package rag 提供 RAG 基础设施层实现 - LLM 适配器
package rag

import (
	"context"
	"fmt"
	"log"

	"link/internal/model/llm"
	domainrag "link/internal/model/rag"
)

// ========================================
// LLMChat 适配器
// ========================================

// LLMChatAdapter 将 llm.LLMClient 适配为 domainrag.LLMChat
type LLMChatAdapter struct {
	llmClient llm.LLMClient
}

// NewLLMChatAdapter 创建 LLMChat 适配器（带降级处理）
func NewLLMChatAdapter(_ interface{}) domainrag.LLMChat {
	log.Printf("[RAG][LLMAdapter] WARNING: Creating adapter without llmClient, using fallback")
	return &LLMChatAdapter{
		llmClient: nil,
	}
}

// NewLLMChatAdapterWithRepo 使用 LLMClient 创建适配器
// @Deprecated: 使用 NewLLMChatAdapterWithClient
func NewLLMChatAdapterWithRepo(chatRepo llm.ChatRepository) domainrag.LLMChat {
	return NewLLMChatAdapterWithClient(chatRepo)
}

// NewLLMChatAdapterWithClient 使用 LLMClient 创建适配器
func NewLLMChatAdapterWithClient(llmClient llm.LLMClient) domainrag.LLMChat {
	if llmClient == nil {
		log.Printf("[RAG][LLMAdapter] WARNING: llmClient is nil in NewLLMChatAdapterWithClient")
	} else {
		log.Printf("[RAG][LLMAdapter] SUCCESS: Created adapter with valid llmClient")
	}
	return &LLMChatAdapter{
		llmClient: llmClient,
	}
}

// Chat 执行非流式聊天
func (a *LLMChatAdapter) Chat(ctx context.Context, messages []domainrag.LLMMessage, opts *domainrag.ChatOptions) (*domainrag.LLMResponse, error) {
	if a.llmClient == nil {
		// 降级处理：返回基于简单规则生成的响应
		return a.fallbackChat(ctx, messages, opts)
	}

	// 转换消息格式
	llmMessages := a.convertMessages(messages)
	req := &llm.ChatRequest{
		Messages: llmMessages,
		Options: &llm.ChatOptions{
			Temperature: opts.Temperature,
			MaxTokens:   opts.MaxTokens,
			TopP:        opts.TopP,
		},
	}

	// 调用底层聊天服务
	resp, err := a.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// 转换响应格式
	messageID := ""
	if resp.Metadata != nil {
		if id, ok := resp.Metadata["id"].(string); ok {
			messageID = id
		}
	}
	return &domainrag.LLMResponse{
		Content:      resp.Content,
		MessageID:    messageID,
		TokenCount:   resp.Usage.TotalTokens,
		FinishReason: resp.FinishReason,
	}, nil
}

// ChatStream 执行流式聊天
func (a *LLMChatAdapter) ChatStream(ctx context.Context, messages []domainrag.LLMMessage, opts *domainrag.ChatOptions) (<-chan *domainrag.LLMStreamEvent, error) {
	if a.llmClient == nil {
		return a.fallbackChatStream(ctx, messages, opts)
	}

	// 转换消息格式
	llmMessages := a.convertMessages(messages)
	req := &llm.ChatRequest{
		Messages: llmMessages,
		Options: &llm.ChatOptions{
			Temperature: opts.Temperature,
			MaxTokens:   opts.MaxTokens,
			TopP:        opts.TopP,
		},
	}

	// 调用底层流式聊天服务
	streamChan, err := a.llmClient.ChatStream(ctx, req)
	if err != nil {
		return nil, err
	}

	// 转换流式事件
	resultChan := make(chan *domainrag.LLMStreamEvent, 10)

	go func() {
		defer close(resultChan)
		for chunk := range streamChan {
			select {
			case <-ctx.Done():
				return
			default:
			}

			resultChan <- &domainrag.LLMStreamEvent{
				Content: chunk.Content,
				Done:    chunk.Done,
				Error:   nil,
			}

			if chunk.Done {
				return
			}
		}
	}()

	return resultChan, nil
}

// ========================================
// 辅助方法
// ========================================

// convertMessages 转换消息格式 (domainrag.LLMMessage -> llm.Message)
func (a *LLMChatAdapter) convertMessages(messages []domainrag.LLMMessage) []*llm.Message {
	result := make([]*llm.Message, len(messages))
	for i, msg := range messages {
		result[i] = &llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	return result
}

// ========================================
// 降级处理方法
// ========================================

// fallbackChat 当 chatRepo 不可用时的降级处理
func (a *LLMChatAdapter) fallbackChat(ctx context.Context, messages []domainrag.LLMMessage, opts *domainrag.ChatOptions) (*domainrag.LLMResponse, error) {
	log.Printf("[RAG][LLMAdapter] Using fallback chat (LLM not configured)")

	// 从最后一条用户消息中提取内容
	userQuery := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userQuery = messages[i].Content
			break
		}
	}

	// 生成简单的响应
	fallbackResponse := `抱歉，当前LLM服务未配置或不可用。
请检查以下配置：
1. 确保在配置文件中设置了有效的 API Key
2. 检查 LLM 服务地址是否正确
3. 验证网络连接是否正常

如果您看到此消息，请联系管理员配置 LLM 服务。`

	// 如果有用户查询，提供更具体的响应
	if userQuery != "" {
		fallbackResponse = fmt.Sprintf(`我收到了您的问题：%s

但是，抱歉，当前LLM服务未配置或不可用。
请检查以下配置：
1. 确保在配置文件中设置了有效的 API Key
2. 检查 LLM 服务地址是否正确
3. 验证网络连接是否正常

请联系管理员配置 LLM 服务。`, userQuery)
	}

	return &domainrag.LLMResponse{
		Content:      fallbackResponse,
		MessageID:    "fallback_" + fmt.Sprintf("%d", ctx.Value("timestamp")),
		TokenCount:   0,
		FinishReason: "fallback",
	}, nil
}

// fallbackChatStream 当 chatRepo 不可用时的降级流式处理
func (a *LLMChatAdapter) fallbackChatStream(ctx context.Context, messages []domainrag.LLMMessage, opts *domainrag.ChatOptions) (<-chan *domainrag.LLMStreamEvent, error) {
	log.Printf("[RAG][LLMAdapter] Using fallback stream (LLM not configured)")

	resultChan := make(chan *domainrag.LLMStreamEvent, 5)

	go func() {
		defer close(resultChan)

		// 获取降级响应
		resp, err := a.fallbackChat(ctx, messages, opts)
		if err != nil {
			resultChan <- &domainrag.LLMStreamEvent{
				Error: fmt.Errorf("fallback failed: %w", err),
				Done:  true,
			}
			return
		}

		// 模拟流式输出（分块发送）
		content := resp.Content
		chunkSize := 50

		for i := 0; i < len(content); i += chunkSize {
			select {
			case <-ctx.Done():
				return
			default:
			}

			end := i + chunkSize
			if end > len(content) {
				end = len(content)
			}

			resultChan <- &domainrag.LLMStreamEvent{
				Content: content[i:end],
				Done:    false,
			}
		}

		// 发送完成标记
		resultChan <- &domainrag.LLMStreamEvent{
			Content: "",
			Done:    true,
		}
	}()

	return resultChan, nil
}
