// Package chat 提供聊天领域的仓储接口定义
package conversation

import "context"

// ========================================
// SessionRepository 会话仓储接口
// ========================================

// SessionRepository 会话仓储接口
type SessionRepository interface {
	// Create 创建会话
	Create(ctx context.Context, session *Session) error

	// FindByID 根据ID查找会话
	FindByID(ctx context.Context, id string) (*Session, error)

	// FindByUserID 根据用户ID查找会话列表
	FindByUserID(ctx context.Context, userID int64, req *ListSessionsRequest) ([]*Session, int64, error)

	// FindByTenantID 根据租户ID查找会话列表
	FindByTenantID(ctx context.Context, tenantID int64, req *ListSessionsRequest) ([]*Session, int64, error)

	// Update 更新会话
	Update(ctx context.Context, session *Session) error

	// UpdateStatus 更新会话状态
	UpdateStatus(ctx context.Context, id string, status int8) error

	// UpdateMessageCount 更新消息数量
	UpdateMessageCount(ctx context.Context, id string, count int) error

	// Delete 删除会话（软删除）
	Delete(ctx context.Context, id string) error

	// DeleteByUserID 删除用户的所有会话
	DeleteByUserID(ctx context.Context, userID int64) error

	// ArchiveSession 归档会话
	ArchiveSession(ctx context.Context, id string) error

	// ActivateSession 激活会话
	ActivateSession(ctx context.Context, id string) error

	// Exists 检查会话是否存在
	Exists(ctx context.Context, id string) (bool, error)

	// CountByUserID 统计用户的会话数量
	CountByUserID(ctx context.Context, userID int64) (int64, error)
}

// ========================================
// MessageRepository 消息仓储接口
// ========================================

// MessageRepository 消息仓储接口
type MessageRepository interface {
	// Create 创建消息
	Create(ctx context.Context, message *Message) error

	// CreateBatch 批量创建消息
	CreateBatch(ctx context.Context, messages []*Message) error

	// FindByID 根据ID查找消息
	FindByID(ctx context.Context, id string) (*Message, error)

	// FindBySessionID 根据会话ID查找消息列表
	FindBySessionID(ctx context.Context, sessionID string, req *ListMessagesRequest) ([]*Message, int64, error)

	// FindByRequestID 根据请求ID查找消息
	FindByRequestID(ctx context.Context, requestID string) (*Message, error)

	// Update 更新消息
	Update(ctx context.Context, message *Message) error

	// UpdateContent 更新消息内容
	UpdateContent(ctx context.Context, id string, content string) error

	// UpdateCompleted 更新消息完成状态
	UpdateCompleted(ctx context.Context, id string, isCompleted bool) error

	// Delete 删除消息（软删除）
	Delete(ctx context.Context, id string) error

	// DeleteBySessionID 删除会话的所有消息
	DeleteBySessionID(ctx context.Context, sessionID string) error

	// CountBySessionID 统计会话的消息数量
	CountBySessionID(ctx context.Context, sessionID string) (int64, error)

	// GetTokenCountBySessionID 获取会话的 Token 总数
	GetTokenCountBySessionID(ctx context.Context, sessionID string) (int, error)
}

// ========================================
// MessageFeedbackRepository 消息反馈仓储接口
// ========================================

// MessageFeedbackRepository 消息反馈仓储接口
type MessageFeedbackRepository interface {
	// Create 创建反馈
	Create(ctx context.Context, feedback *MessageFeedback) error

	// FindByMessageID 根据消息ID查找反馈
	FindByMessageID(ctx context.Context, messageID string) ([]*MessageFeedback, error)

	// FindByUserID 根据用户ID查找反馈列表
	FindByUserID(ctx context.Context, userID int64, page, pageSize int) ([]*MessageFeedback, int64, error)

	// Update 更新反馈
	Update(ctx context.Context, feedback *MessageFeedback) error

	// Delete 删除反馈
	Delete(ctx context.Context, id int64) error
}

// ========================================
// LLMChatService LLM 聊天服务接口（领域层）
// ========================================

// LLMChatService LLM 聊天服务接口
// 定义应用层所需的 LLM 聊天能力，遵循依赖倒置原则
type LLMChatService interface {
	// Chat 执行非流式聊天
	Chat(ctx context.Context, messages []*ConversationMessage, opts *ConversationOptions) (*ChatResult, error)

	// ChatStream 执行流式聊天
	ChatStream(ctx context.Context, messages []*ConversationMessage, opts *ConversationOptions) (<-chan *ChatStreamChunk, error)

	// GetModelInfo 获取模型信息
	GetModelInfo(ctx context.Context) (*ModelInfo, error)
}

// ChatResult 聊天结果
type ChatResult struct {
	MessageID  string
	Content    string
	Role       string
	TokenCount int
	ToolCalls  []ToolCall
	FinishReason string
}

// ChatStreamChunk 流式聊天分块
type ChatStreamChunk struct {
	Event      string
	Content    string
	MessageID  string
	TokenCount int
	ToolCalls  []ToolCall
	Done       bool
	Error      error
}

// ModelInfo 模型信息
type ModelInfo struct {
	Name        string
	DisplayName string
	Provider    string
	MaxTokens   int
}
