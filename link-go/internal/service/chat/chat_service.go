// Package llm provides LLM service layer implementations
package chat

import (
	"context"
	"fmt"

	"link/internal/model/llm"
)

// ========================================
// Chat Service
// ========================================

// ChatService 聊天服务 - 直接使用 LLM 模型进行对话
type ChatService struct {
	llmClient     llm.LLMClient
	modelRepo    llm.ModelRepository
	modelFactory llm.ModelFactory
}

// NewChatService 创建聊天服务
func NewChatService(
	llmClient llm.LLMClient,
	modelRepo llm.ModelRepository,
	modelFactory llm.ModelFactory,
) *ChatService {
	return &ChatService{
		llmClient:     llmClient,
		modelRepo:    modelRepo,
		modelFactory: modelFactory,
	}
}

// SetChatModel 设置聊天模型
func (s *ChatService) SetChatModel(client llm.LLMClient) {
	s.llmClient = client
}

// Chat 执行聊天请求
func (s *ChatService) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("聊天模型未初始化")
	}

	resp, err := s.llmClient.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("聊天请求失败: %w", err)
	}

	return resp, nil
}

// ChatStream 执行流式聊天请求
func (s *ChatService) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan *llm.ChatChunk, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("聊天模型未初始化")
	}

	return s.llmClient.ChatStream(ctx, req)
}

// GetModelInfo 获取模型信息
func (s *ChatService) GetModelInfo(ctx context.Context) (*llm.ModelInfo, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("聊天模型未初始化")
	}

	return s.llmClient.GetModelInfo(ctx)
}

// SupportsTools 检查是否支持工具调用
func (s *ChatService) SupportsTools() bool {
	if s.llmClient == nil {
		return false
	}
	return s.llmClient.SupportsTools()
}

// SupportsStreaming 检查是否支持流式输出
func (s *ChatService) SupportsStreaming() bool {
	if s.llmClient == nil {
		return false
	}
	return s.llmClient.SupportsStreaming()
}
