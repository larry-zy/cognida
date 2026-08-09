// Package memory 提供 Memory 模块的 Repository 接口定义
package memory

import "context"

// ========================================
// MemoryRepository 消息存储接口
// ========================================

// MemoryRepository 消息存储接口
// 负责消息和摘要的持久化操作
type MemoryRepository interface {
	// ========================================
	// 消息操作
	// ========================================

	// SaveMessage 保存单条消息
	SaveMessage(ctx context.Context, msg *Message) error

	// SaveMessageBatch 批量保存消息
	SaveMessageBatch(ctx context.Context, msgs []*Message) error

	// LoadHistory 加载会话历史消息
	// includeCompressed: 是否包含已被压缩的消息
	LoadHistory(ctx context.Context, sessionID string, includeCompressed bool) ([]*Message, error)

	// LoadHistoryWithLimit 加载指定数量的最近消息
	LoadHistoryWithLimit(ctx context.Context, sessionID string, limit int, includeCompressed bool) ([]*Message, error)

	// LoadHistoryByTimeRange 按时间范围加载消息
	LoadHistoryByTimeRange(ctx context.Context, sessionID string, start, end int64) ([]*Message, error)

	// GetMessageByID 根据 ID 获取消息
	GetMessageByID(ctx context.Context, id string) (*Message, error)

	// UpdateMessage 更新消息
	UpdateMessage(ctx context.Context, msg *Message) error

	// MarkMessagesAsCompressed 标记消息为已压缩
	MarkMessagesAsCompressed(ctx context.Context, sessionID string, summaryID string, messageIDs []string) error

	// DeleteMessage 删除消息（软删除）
	DeleteMessage(ctx context.Context, id string) error

	// ClearSession 清空会话消息（软删除）
	ClearSession(ctx context.Context, sessionID string) error

	// ========================================
	// 摘要操作
	// ========================================

	// SaveSummary 保存摘要
	SaveSummary(ctx context.Context, summary *Summary) error

	// LoadSummary 加载摘要
	LoadSummary(ctx context.Context, id string) (*Summary, error)

	// LoadSummariesBySession 加载会话的所有摘要
	LoadSummariesBySession(ctx context.Context, sessionID string) ([]*Summary, error)

	// LoadLatestSummary 加载最新摘要
	LoadLatestSummary(ctx context.Context, sessionID string) (*Summary, error)

	// UpdateSummary 更新摘要
	UpdateSummary(ctx context.Context, summary *Summary) error

	// DeleteSummary 删除摘要
	DeleteSummary(ctx context.Context, id string) error

	// ========================================
	// Token 统计
	// ========================================

	// GetTokenUsage 获取会话的 token 使用情况
	GetTokenUsage(ctx context.Context, sessionID string) (*TokenUsage, error)
}

// ========================================
// LongTermMemoryRepository 长期记忆接口
// ========================================

// LongTermMemoryRepository 长期记忆存储接口
type LongTermMemoryRepository interface {
	// Store 存储长期记忆
	Store(ctx context.Context, memory *LongTermMemory) error

	// Retrieve 根据 ID 获取记忆
	Retrieve(ctx context.Context, id string) (*LongTermMemory, error)

	// Search 搜索记忆（语义检索）
	Search(ctx context.Context, query *MemorySearchQuery) ([]*LongTermMemory, error)

	// Update 更新记忆
	Update(ctx context.Context, memory *LongTermMemory) error

	// RecordAccess 原子记录一次访问（access_count 自增 + last_access_at 刷新）。
	// 由存储层做原子 UPDATE，调用方不得改写已返回给使用者的实体（避免 data race）
	RecordAccess(ctx context.Context, id string) error

	// Delete 删除记忆（软删除）
	Delete(ctx context.Context, id string) error

	// ListByCategory 按类别列出记忆
	ListByCategory(ctx context.Context, tenantID int64, category string, limit int) ([]*LongTermMemory, error)

	// ListByUser 列出用户的记忆
	ListByUser(ctx context.Context, tenantID, userID int64, limit int) ([]*LongTermMemory, error)
}

// MemorySearchQuery 记忆搜索查询
type MemorySearchQuery struct {
	TenantID      int64
	UserID        *int64  // 可选，为空则搜索全局记忆
	Category      string  // 可选，类别过滤
	QueryEmbedding []float32 // 查询向量
	MaxResults    int     // 最大返回数量
	MinImportance float64 // 最小重要性
}

// ========================================
// TokenUsage Token 使用统计
// ========================================

// TokenUsage Token 使用统计
type TokenUsage struct {
	SessionID       string
	TotalMessages   int
	TotalTokens     int
	CompressedCount int // 已压缩的消息数
	OffloadedCount  int // 已卸载的消息数
}
