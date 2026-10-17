// Package chat provides message service implementation
package chat

import (
	"context"
	"errors"
	"fmt"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/conversation"
)

var ErrMessageForbidden = errors.New("message session is not accessible")

// MessageService 消息服务实现
type MessageService struct {
	messageRepo conversation.MessageRepository
	sessionRepo conversation.SessionRepository
}

// NewMessageService 创建消息服务实例
func NewMessageService(messageRepo conversation.MessageRepository, sessionRepo conversation.SessionRepository) *MessageService {
	return &MessageService{
		messageRepo: messageRepo,
		sessionRepo: sessionRepo,
	}
}

// The HTTP service always checks the session owner. Internal Agent persistence
// continues to use MessageRepository directly, after its own session checks.
func (s *MessageService) authorizeSession(ctx context.Context, id string) error {
	tenant, tok := agentctx.GetTenantID(ctx)
	user, uok := agentctx.GetUserID(ctx)
	if !tok || !uok || tenant <= 0 || user <= 0 || id == "" || s.sessionRepo == nil {
		return ErrMessageForbidden
	}
	session, err := s.sessionRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: session unavailable", ErrMessageForbidden)
	}
	if session == nil || session.DeletedAt != nil || !session.BelongsToTenant(tenant) || !session.IsOwnedBy(user) {
		return ErrMessageForbidden
	}
	return nil
}

func (s *MessageService) authorizedMessage(ctx context.Context, id string) (*conversation.Message, error) {
	tenant, tok := agentctx.GetTenantID(ctx)
	user, uok := agentctx.GetUserID(ctx)
	if !tok || !uok || tenant <= 0 || user <= 0 {
		return nil, ErrMessageForbidden
	}
	message, err := s.messageRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, ErrMessageForbidden
	}
	if err = s.authorizeSession(ctx, message.SessionID); err != nil {
		return nil, err
	}
	return message, nil
}

// CreateMessage 创建消息
func (s *MessageService) CreateMessage(ctx context.Context, req *CreateMessageRequest) (*MessageResponse, error) {
	if err := s.authorizeSession(ctx, req.SessionID); err != nil {
		return nil, err
	}
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
	message, err := s.authorizedMessage(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.toMessageResponse(message), nil
}

// ListMessages 查询消息列表
func (s *MessageService) ListMessages(ctx context.Context, req *ListMessagesRequest) (*MessageListResponse, error) {
	if err := s.authorizeSession(ctx, req.SessionID); err != nil {
		return nil, err
	}
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
	message, err := s.authorizedMessage(ctx, id)
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
	// 知识引用 / Agent 步骤 / Token 计数需真正落库：
	// 否则仅改内存结构体，随后 FindByID 从 DB 重拉会覆盖改动，导致静默 no-op。
	if req.KnowledgeReferences != nil || req.AgentSteps != nil || req.TokenCount != nil {
		if err := s.messageRepo.UpdateAgentData(ctx, id, req.KnowledgeReferences, req.AgentSteps, req.TokenCount); err != nil {
			return nil, fmt.Errorf("更新消息字段失败: %w", err)
		}
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
	if _, err := s.authorizedMessage(ctx, id); err != nil {
		return err
	}
	err := s.messageRepo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("删除消息失败: %w", err)
	}
	return nil
}

// DeleteMessagesBySessionID 删除会话的所有消息
func (s *MessageService) DeleteMessagesBySessionID(ctx context.Context, sessionID string) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}
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
