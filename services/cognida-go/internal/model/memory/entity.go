// Package memory 提供内存管理和上下文工程的领域实体
package memory

import "time"

// ========================================
// MessageType 消息类型枚举
// ========================================

// MessageType 消息类型
type MessageType string

const (
	MessageTypeSystem     MessageType = "system"
	MessageTypeUser       MessageType = "user"
	MessageTypeAssistant  MessageType = "assistant"
	MessageTypeToolCall   MessageType = "tool_call"   // 中间过程
	MessageTypeToolResult MessageType = "tool_result" // 中间过程
	MessageTypeSummary    MessageType = "summary"
)

// String 实现 Stringer 接口
func (t MessageType) String() string {
	return string(t)
}

// IsIntermediate 判断是否为中间过程消息
func (t MessageType) IsIntermediate() bool {
	return t == MessageTypeToolCall || t == MessageTypeToolResult
}

// IsContextMessage 判断是否为需要展示给 LLM 的上下文消息
func (t MessageType) IsContextMessage() bool {
	switch t {
	case MessageTypeSystem, MessageTypeUser, MessageTypeAssistant, MessageTypeSummary:
		return true
	default:
		return false
	}
}

// ========================================
// Message 消息实体
// ========================================

// Message 消息实体
type Message struct {
	ID           string
	SessionID    string
	TenantID     int64
	UserID       int64
	Type         MessageType
	Role         string // system/user/assistant/tool
	Content      string
	Tokens       int
	IsIntermediate bool
	Iteration    int // 多步迭代计数

	// 工具调用相关（当 Type 为 tool_call 时）
	ToolCalls    []ToolCallInfo

	// 压缩相关
	CompressedAt *time.Time
	SummaryID    *string

	// 追踪字段
	RequestID    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// ToolCallInfo 工具调用信息
type ToolCallInfo struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Arguments string                `json:"arguments"`
	Result   string                 `json:"result"`
	Error    string                 `json:"error,omitempty"`
	Duration int64                  `json:"duration"` // 毫秒
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// IsCompressed 判断消息是否已被压缩
func (m *Message) IsCompressed() bool {
	return m.CompressedAt != nil
}

// ShouldIncludeInContext 判断消息是否应包含在上下文中
func (m *Message) ShouldIncludeInContext(includeIntermediate bool) bool {
	if m.DeletedAt != nil {
		return false
	}
	if m.IsCompressed() {
		return false
	}
	if !includeIntermediate && m.IsIntermediate {
		return false
	}
	return true
}

// ========================================
// Summary 摘要实体
// ========================================

// Summary 对话摘要实体
type Summary struct {
	ID                string
	SessionID         string
	TenantID          int64

	// 摘要内容
	Content           string

	// 时间范围
	TimeRangeStart    time.Time
	TimeRangeEnd      time.Time

	// 统计信息
	OriginalMessageCount int
	OriginalTokens       int
	CompressedTokens     int

	// 压缩率
	CompressionRatio float64

	// 元数据
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CalculateCompressionRatio 计算压缩率
func (s *Summary) CalculateCompressionRatio() float64 {
	if s.OriginalTokens == 0 {
		return 0
	}
	return float64(s.CompressedTokens) / float64(s.OriginalTokens)
}

// ========================================
// LongTermMemory 长期记忆实体
// ========================================

// LongTermMemory 长期记忆实体
type LongTermMemory struct {
	ID          string
	TenantID    int64
	UserID      *int64 // 可选，为空表示全局记忆

	// 记忆内容
	Content     string
	Category    string // 类别（如 user_preferences, project_context 等）

	// 语义检索
	Embedding   []float32 // 向量嵌入

	// 重要性评分
	Importance  float64 // 0-1

	// 访问统计
	AccessCount int
	LastAccessAt *time.Time

	// 追踪字段
	Metadata    map[string]interface{}
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// IsGlobal 判断是否为全局记忆
func (m *LongTermMemory) IsGlobal() bool {
	return m.UserID == nil
}

// RecordAccess 记录访问
func (m *LongTermMemory) RecordAccess() {
	m.AccessCount++
	now := time.Now()
	m.LastAccessAt = &now
}

// ========================================
// ContextBuildMetadata 上下文构建元数据
// ========================================

// ContextBuildMetadata 上下文构建元数据
type ContextBuildMetadata struct {
	// Token 统计
	SystemPromptTokens  int
	FewShotTokens       int
	SummaryTokens       int
	HistoryTokens       int
	CurrentMessageTokens int
	TotalTokens         int

	// 消息统计
	SystemMessageCount  int
	UserMessageCount    int
	AssistantMessageCount int
	SummaryMessageCount int
	ExcludedIntermediateCount int

	// 性能统计
	BuildTime           time.Duration
	CacheLoadTime       time.Duration
	DBLoadTime          time.Duration
	CompressionTime     time.Duration

	// 压缩信息
	CompressionApplied  bool
	CompressionStrategy string
	TokensSaved         int
}

// AddSystemTokens 添加系统 prompt tokens
func (m *ContextBuildMetadata) AddSystemTokens(tokens int) {
	m.SystemPromptTokens = tokens
	m.RecalculateTotal()
}

// AddFewShotTokens 添加 few-shot tokens
func (m *ContextBuildMetadata) AddFewShotTokens(tokens int) {
	m.FewShotTokens = tokens
	m.RecalculateTotal()
}

// AddSummaryTokens 添加摘要 tokens
func (m *ContextBuildMetadata) AddSummaryTokens(tokens int) {
	m.SummaryTokens = tokens
	m.RecalculateTotal()
}

// AddHistoryTokens 添加历史消息 tokens
func (m *ContextBuildMetadata) AddHistoryTokens(tokens int) {
	m.HistoryTokens = tokens
	m.RecalculateTotal()
}

// AddCurrentMessageTokens 添加当前消息 tokens
func (m *ContextBuildMetadata) AddCurrentMessageTokens(tokens int) {
	m.CurrentMessageTokens = tokens
	m.RecalculateTotal()
}

// RecalculateTotal 重新计算总 tokens
func (m *ContextBuildMetadata) RecalculateTotal() {
	m.TotalTokens = m.SystemPromptTokens +
		m.FewShotTokens +
		m.SummaryTokens +
		m.HistoryTokens +
		m.CurrentMessageTokens
}
