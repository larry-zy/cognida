// Package conversation provides the conversation domain entity definitions
package conversation

import "time"

// ========================================
// Session 会话实体
// ========================================

// Session 会话实体
// 对应 sessions 表
type Session struct {
	ID           string
	TenantID     int64
	UserID       int64
	AgentType    string  // Agent类型: rag, text2sql, default
	Title        string
	Description  string
	Status       int8
	MessageCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// ========================================
// Session Business Behavior
// ========================================

// Session status constants
const (
	SessionStatusActive   int8 = 1
	SessionStatusArchived int8 = 0
)

// IsActive checks if the session is active
func (s *Session) IsActive() bool {
	return s.Status == SessionStatusActive
}

// Archive archives the session
func (s *Session) Archive() {
	s.Status = SessionStatusArchived
}

// Activate activates the session
func (s *Session) Activate() {
	s.Status = SessionStatusActive
}

// IncrementMessageCount increments the message count
func (s *Session) IncrementMessageCount() {
	s.MessageCount++
}

// DecrementMessageCount decrements the message count
func (s *Session) DecrementMessageCount() {
	if s.MessageCount > 0 {
		s.MessageCount--
	}
}

// HasMessages checks if the session has messages
func (s *Session) HasMessages() bool {
	return s.MessageCount > 0
}

// IsOwnedBy checks if the session is owned by the given user
func (s *Session) IsOwnedBy(userID int64) bool {
	return s.UserID == userID
}

// BelongsToTenant checks if the session belongs to the given tenant
func (s *Session) BelongsToTenant(tenantID int64) bool {
	return s.TenantID == tenantID
}

// SetTitle sets the session title
func (s *Session) SetTitle(title string) {
	s.Title = title
}

// ========================================
// Message 消息实体
// ========================================

// Message 消息实体
// 对应 messages 表
type Message struct {
	ID                  string
	RequestID           string
	SessionID           string
	Role                string
	Content             string
	KnowledgeReferences string
	AgentSteps          string
	ToolCalls           string
	IsCompleted         bool
	TokenCount          int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

// ========================================
// Message Business Behavior
// ========================================

// Role constants
const (
	RoleUser      string = "user"
	RoleAssistant string = "assistant"
	RoleSystem    string = "system"
)

// IsUser checks if this is a user message
func (m *Message) IsUser() bool {
	return m.Role == RoleUser
}

// IsAssistant checks if this is an assistant message
func (m *Message) IsAssistant() bool {
	return m.Role == RoleAssistant
}

// IsSystem checks if this is a system message
func (m *Message) IsSystem() bool {
	return m.Role == RoleSystem
}

// IsCompletedCheck checks if the message is completed
func (m *Message) IsCompletedCheck() bool {
	return m.IsCompleted
}

// MarkCompleted marks the message as completed
func (m *Message) MarkCompleted() {
	m.IsCompleted = true
}

// MarkIncomplete marks the message as incomplete
func (m *Message) MarkIncomplete() {
	m.IsCompleted = false
}

// HasToolCalls checks if the message has tool calls
func (m *Message) HasToolCalls() bool {
	return m.ToolCalls != ""
}

// HasKnowledgeReferences checks if the message has knowledge references
func (m *Message) HasKnowledgeReferences() bool {
	return m.KnowledgeReferences != ""
}

// HasAgentSteps checks if the message has agent steps
func (m *Message) HasAgentSteps() bool {
	return m.AgentSteps != ""
}

// GetContentLength returns the content length
func (m *Message) GetContentLength() int {
	return len(m.Content)
}

// BelongsToSession checks if the message belongs to the given session
func (m *Message) BelongsToSession(sessionID string) bool {
	return m.SessionID == sessionID
}

// BelongsToRequest checks if the message belongs to the given request
func (m *Message) BelongsToRequest(requestID string) bool {
	return m.RequestID == requestID
}

// SetTokenCount sets the token count
func (m *Message) SetTokenCount(count int) {
	m.TokenCount = count
}

// GetTokenCount returns the token count
func (m *Message) GetTokenCount() int {
	return m.TokenCount
}

// ========================================
// MessageFeedback 消息反馈
// ========================================

// MessageFeedback 消息反馈
type MessageFeedback struct {
	ID        int64
	MessageID string
	UserID    int64
	Rating    int
	Comment   string
	CreatedAt time.Time
}

// ========================================
// RAG 相关类型
// ========================================

// RAGConfig RAG 检索配置
type RAGConfig struct {
	// 是否启用 RAG
	Enabled bool

	// 知识库 ID
	KnowledgeBaseID string

	// 检索模式（可多选，向量检索必选）
	RetrievalModes []string

	// 检索参数
	VectorTopK          int
	KeywordTopK         int
	GraphTopK           int
	SimilarityThreshold float64
	Alpha               float32
}

// DefaultRAGConfig 默认 RAG 配置
func DefaultRAGConfig() *RAGConfig {
	return &RAGConfig{
		Enabled:             false,
		RetrievalModes:      []string{"vector"},
		VectorTopK:          15,
		KeywordTopK:         15,
		GraphTopK:           10,
		SimilarityThreshold: 0.0,
		Alpha:               0.6,
	}
}

// RAGContext RAG 检索结果上下文
type RAGContext struct {
	Query             string
	FinalQuery        string
	Contexts          []string
	ContextsWithScore []map[string]interface{}
	SourceTypes       []string
	RetrievedCount    int
	Stages            map[string]interface{}
}

// ========================================
// Conversation 相关类型
// ========================================

// Message 聊天消息
type ConversationMessage struct {
	Role    string
	Content string
}

// ConversationOptions 聊天选项
type ConversationOptions struct {
	Temperature      float64
	MaxTokens        int
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
	Thinking         bool
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string
	Type     string
	Function FunctionCall
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string
	Arguments string
}

// ========================================
// 查询参数
// ========================================

// ListSessionsRequest 查询会话列表请求
type ListSessionsRequest struct {
	Page   int
	Size   int
	Status *int8
}

// ListMessagesRequest 查询消息列表请求
type ListMessagesRequest struct {
	SessionID string
	Page      int
	Size      int
	Role      string
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	Title   string `json:"title"`
	AgentID string `json:"agent_id,omitempty"`
}

// UpdateSessionRequest 更新会话请求
type UpdateSessionRequest struct {
	Title  string `json:"title,omitempty"`
	Status *int8  `json:"status,omitempty"`
}

// SessionDetailResponse 会话详情响应
type SessionDetailResponse struct {
	Session  *Session
	Messages []*Message
}

// MessageResponse 消息响应
type MessageResponse struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageListResponse 消息列表响应
type MessageListResponse struct {
	Page     int               `json:"page"`
	Size     int               `json:"size"`
	Total    int64             `json:"total"`
	Messages []*MessageResponse `json:"messages"`
}
