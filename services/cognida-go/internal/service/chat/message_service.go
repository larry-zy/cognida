// Package chat provides message service implementation
package chat

import (
	"context"
	"errors"
	"fmt"

	"cognida/internal/model/conversation"
)

// MessageService 消息服务实现
type MessageService struct {
	messageRepo conversation.MessageRepository
}

// NewMessageService 创建消息服务实例
func NewMessageService(messageRepo conversation.MessageRepository) *MessageService {
	return &MessageService{
		messageRepo: messageRepo,
	}
}

// CreateMessage 创建消息
func (s *MessageService) CreateMessage(ctx context.Context, req *CreateMessageRequest) (*MessageResponse, error) {
	// 验证角色
	if req.Role != "system" && req.Role != "user" && req.Role != "assistant" && req.Role != "tool" {
		return nil, errors.New("无效的消息角色")
	}

	// 验证内容
	if req.Content == "" {
		return nil, errors.New("消息内容不能为空")
	}

	// 构建领域实体
	message := &conversation.Message{
		SessionID:           req.SessionID,
		Role:                req.Role,
		Content:             req.Content,
		KnowledgeReferences: req.KnowledgeReferences,
		AgentSteps:          req.AgentSteps,
		ToolCalls:           req.ToolCalls,
		TokenCount:          req.TokenCount,
	}

	// 调用仓储创建消息
	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, fmt.Errorf("创建消息失败: %w", err)
	}

	// 返回响应
	return s.toMessageResponse(message), nil
}

// GetMessageByID 根据ID获取消息
func (s *MessageService) GetMessageByID(ctx context.Context, id string) (*MessageResponse, error) {
	message, err := s.messageRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toMessageResponse(message), nil
}

// ListMessages 查询消息列表
func (s *MessageService) ListMessages(ctx context.Context, req *ListMessagesRequest) (*MessageListResponse, error) {
	// 设置默认分页参数
	page := req.Page
	if page == 0 {
		page = 1
	}
	size := req.Size
	if size == 0 {
		size = 20
	}

	// 构建查询请求
	listReq := &conversation.ListMessagesRequest{
		SessionID: req.SessionID,
		Page:      page,
		Size:      size,
		Role:      req.Role,
	}

	messages, total, err := s.messageRepo.FindBySessionID(ctx, req.SessionID, listReq)
	if err != nil {
		return nil, fmt.Errorf("查询消息列表失败: %w", err)
	}

	// 转换为响应格式
	messageResponses := make([]*MessageResponse, 0, len(messages))
	for _, msg := range messages {
		messageResponses = append(messageResponses, s.toMessageResponse(msg))
	}

	return &MessageListResponse{
		Page:     page,
		Size:     size,
		Total:    total,
		Messages: messageResponses,
	}, nil
}

// UpdateMessage 更新消息
func (s *MessageService) UpdateMessage(ctx context.Context, id string, req *UpdateMessageRequest) (*MessageResponse, error) {
	// 查找消息
	message, err := s.messageRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 更新字段
	if req.Content != nil {
		if err := s.messageRepo.UpdateContent(ctx, id, *req.Content); err != nil {
			return nil, fmt.Errorf("更新消息内容失败: %w", err)
		}
	}
	if req.IsCompleted != nil {
		if err := s.messageRepo.UpdateCompleted(ctx, id, *req.IsCompleted); err != nil {
			return nil, fmt.Errorf("更新消息完成状态失败: %w", err)
		}
		message.IsCompleted = *req.IsCompleted
	}
	if req.KnowledgeReferences != nil {
		message.KnowledgeReferences = *req.KnowledgeReferences
	}
	if req.AgentSteps != nil {
		message.AgentSteps = *req.AgentSteps
	}
	if req.TokenCount != nil {
		message.TokenCount = *req.TokenCount
	}

	// 重新获取更新后的消息
	message, err = s.messageRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toMessageResponse(message), nil
}

// DeleteMessage 删除消息
func (s *MessageService) DeleteMessage(ctx context.Context, id string) error {
	err := s.messageRepo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("删除消息失败: %w", err)
	}
	return nil
}

// DeleteMessagesBySessionID 删除会话的所有消息
func (s *MessageService) DeleteMessagesBySessionID(ctx context.Context, sessionID string) error {
	err := s.messageRepo.DeleteBySessionID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("删除会话消息失败: %w", err)
	}
	return nil
}

// toMessageResponse 转换为消息响应格式
func (s *MessageService) toMessageResponse(message *conversation.Message) *MessageResponse {
	return &MessageResponse{
		ID:                  message.ID,
		RequestID:           message.RequestID,
		SessionID:           message.SessionID,
		Role:                message.Role,
		Content:             message.Content,
		KnowledgeReferences: message.KnowledgeReferences,
		AgentSteps:          message.AgentSteps,
		ToolCalls:           message.ToolCalls,
		IsCompleted:         message.IsCompleted,
		TokenCount:          message.TokenCount,
		CreatedAt:           message.CreatedAt,
	}
}
