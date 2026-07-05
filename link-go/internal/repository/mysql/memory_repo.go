// Package mysql 提供 Memory 模块的 MySQL 持久化实现
package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"link/internal/model/memory"
)

// ========================================
// MySQL 实体模型
// ========================================

// ConversationMessageEntity 会话消息实体
type ConversationMessageEntity struct {
	ID           string     `gorm:"primaryKey;column:id"`
	RequestID    string     `gorm:"column:request_id"`
	SessionID    string     `gorm:"column:session_id;not null;index:idx_session_id"`
	TenantID     int64      `gorm:"column:tenant_id;not null;index:idx_tenant_user"`
	UserID       int64      `gorm:"column:user_id;not null;index:idx_tenant_user"`
	MessageType  string     `gorm:"column:message_type;not null"`
	Role         string     `gorm:"column:role;not null"`
	Content      string     `gorm:"type:text;not null"`
	Tokens       int        `gorm:"column:tokens;default:0"`
	IsIntermediate bool     `gorm:"column:is_intermediate;default:false"`
	Iteration    int        `gorm:"column:iteration;default:0"`
	ToolCalls    string     `gorm:"type:json;column:tool_calls"`

	// 压缩相关
	CompressedAt *time.Time `gorm:"column:compressed_at;index:idx_compressed"`
	SummaryID    *string    `gorm:"column:summary_id"`

	// 追踪字段
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
	DeletedAt    *time.Time `gorm:"column:deleted_at;index:idx_deleted_at"`
}

// TableName 指定表名
func (ConversationMessageEntity) TableName() string {
	return "conversation_messages"
}

// ConversationSummaryEntity 会话摘要实体
type ConversationSummaryEntity struct {
	ID                 string    `gorm:"primaryKey;column:id"`
	SessionID          string    `gorm:"column:session_id;not null;index:idx_session_id"`
	TenantID           int64     `gorm:"column:tenant_id;not null"`
	Content            string    `gorm:"type:text;not null"`
	TimeRangeStart     time.Time `gorm:"column:time_range_start;not null;index:idx_time_range"`
	TimeRangeEnd       time.Time `gorm:"column:time_range_end;not null;index:idx_time_range"`
	OriginalMessageCount int     `gorm:"column:original_message_count;not null"`
	OriginalTokens     int       `gorm:"column:original_tokens;not null"`
	CompressedTokens   int       `gorm:"column:compressed_tokens;not null"`
	CompressionRatio   float64   `gorm:"column:compression_ratio;not null"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

// TableName 指定表名
func (ConversationSummaryEntity) TableName() string {
	return "conversation_summaries"
}

// LongTermMemoryEntity 长期记忆实体
type LongTermMemoryEntity struct {
	ID          string        `gorm:"primaryKey;column:id"`
	TenantID    int64         `gorm:"column:tenant_id;not null;index:idx_tenant_user"`
	UserID      *int64        `gorm:"column:user_id;index:idx_tenant_user"`
	Content     string        `gorm:"type:text;not null"`
	Category    string        `gorm:"column:category;index:idx_category"`
	Embedding   string        `gorm:"type:json;column:embedding"` // JSON 数组
	Importance  float64       `gorm:"column:importance;default:0.5"`
	AccessCount int           `gorm:"column:access_count;default:0"`
	LastAccessAt *time.Time   `gorm:"column:last_access_at"`
	Metadata    string        `gorm:"type:json;column:metadata"`
	CreatedAt   time.Time     `gorm:"column:created_at"`
	UpdatedAt   time.Time     `gorm:"column:updated_at"`
	DeletedAt   *time.Time    `gorm:"column:deleted_at"`
}

// TableName 指定表名
func (LongTermMemoryEntity) TableName() string {
	return "long_term_memories"
}

// ========================================
// MemoryRepository 实现
// ========================================

// MemoryRepository MySQL 实现
type MemoryRepository struct {
	db *gorm.DB
}

// NewMemoryRepository 创建 MemoryRepository
func NewMemoryRepository(db *gorm.DB) memory.MemoryRepository {
	return &MemoryRepository{db: db}
}

// ========================================
// 消息操作
// ========================================

// SaveMessage 保存单条消息
func (r *MemoryRepository) SaveMessage(ctx context.Context, msg *memory.Message) error {
	entity := r.toMessageEntity(msg)

	// 使用 ON DUPLICATE KEY UPDATE 处理已存在的记录
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "tokens", "updated_at"}),
		}).
		Create(entity).Error

	if err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	return nil
}

// SaveMessageBatch 批量保存消息
func (r *MemoryRepository) SaveMessageBatch(ctx context.Context, msgs []*memory.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	entities := make([]*ConversationMessageEntity, len(msgs))
	for i, msg := range msgs {
		entities[i] = r.toMessageEntity(msg)
	}

	err := r.db.WithContext(ctx).CreateInBatches(entities, 100).Error
	if err != nil {
		return fmt.Errorf("failed to save messages batch: %w", err)
	}

	return nil
}

// LoadHistory 加载会话历史消息
func (r *MemoryRepository) LoadHistory(ctx context.Context, sessionID string, includeCompressed bool) ([]*memory.Message, error) {
	var entities []*ConversationMessageEntity

	query := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Where("deleted_at IS NULL").
		Order("created_at ASC")

	if !includeCompressed {
		query = query.Where("compressed_at IS NULL")
	}

	err := query.Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	messages := make([]*memory.Message, len(entities))
	for i, entity := range entities {
		msg, err := r.toDomainMessage(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		messages[i] = msg
	}

	return messages, nil
}

// LoadHistoryWithLimit 加载指定数量的最近消息
func (r *MemoryRepository) LoadHistoryWithLimit(ctx context.Context, sessionID string, limit int, includeCompressed bool) ([]*memory.Message, error) {
	var entities []*ConversationMessageEntity

	query := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit)

	if !includeCompressed {
		query = query.Where("compressed_at IS NULL")
	}

	err := query.Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load history with limit: %w", err)
	}

	// 反转顺序（因为用的是 DESC）
	for i, j := 0, len(entities)-1; i < j; i, j = i+1, j-1 {
		entities[i], entities[j] = entities[j], entities[i]
	}

	messages := make([]*memory.Message, len(entities))
	for i, entity := range entities {
		msg, err := r.toDomainMessage(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		messages[i] = msg
	}

	return messages, nil
}

// LoadHistoryByTimeRange 按时间范围加载消息
func (r *MemoryRepository) LoadHistoryByTimeRange(ctx context.Context, sessionID string, start, end int64) ([]*memory.Message, error) {
	var entities []*ConversationMessageEntity

	startTime := time.Unix(0, start*1000000) // 毫秒转时间
	endTime := time.Unix(0, end*1000000)

	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Where("deleted_at IS NULL").
		Order("created_at ASC").
		Find(&entities).Error

	if err != nil {
		return nil, fmt.Errorf("failed to load history by time range: %w", err)
	}

	messages := make([]*memory.Message, len(entities))
	for i, entity := range entities {
		msg, err := r.toDomainMessage(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		messages[i] = msg
	}

	return messages, nil
}

// GetMessageByID 根据 ID 获取消息
func (r *MemoryRepository) GetMessageByID(ctx context.Context, id string) (*memory.Message, error) {
	var entity ConversationMessageEntity

	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		First(&entity).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, memory.ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to get message by id: %w", err)
	}

	return r.toDomainMessage(&entity)
}

// UpdateMessage 更新消息
func (r *MemoryRepository) UpdateMessage(ctx context.Context, msg *memory.Message) error {
	entity := r.toMessageEntity(msg)

	err := r.db.WithContext(ctx).
		Model(&ConversationMessageEntity{}).
		Where("id = ?", msg.ID).
		Updates(entity).Error

	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	return nil
}

// MarkMessagesAsCompressed 标记消息为已压缩
func (r *MemoryRepository) MarkMessagesAsCompressed(ctx context.Context, sessionID string, summaryID string, messageIDs []string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).
		Model(&ConversationMessageEntity{}).
		Where("session_id = ?", sessionID).
		Where("id IN ?", messageIDs).
		Updates(map[string]interface{}{
			"compressed_at": &now,
			"summary_id":    &summaryID,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to mark messages as compressed: %w", err)
	}

	return nil
}

// DeleteMessage 删除消息（软删除）
func (r *MemoryRepository) DeleteMessage(ctx context.Context, id string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).
		Model(&ConversationMessageEntity{}).
		Where("id = ?", id).
		Update("deleted_at", &now).Error

	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

// ClearSession 清空会话消息（软删除）
func (r *MemoryRepository) ClearSession(ctx context.Context, sessionID string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).
		Model(&ConversationMessageEntity{}).
		Where("session_id = ?", sessionID).
		Update("deleted_at", &now).Error

	if err != nil {
		return fmt.Errorf("failed to clear session: %w", err)
	}

	return nil
}

// ========================================
// 摘要操作
// ========================================

// SaveSummary 保存摘要
func (r *MemoryRepository) SaveSummary(ctx context.Context, summary *memory.Summary) error {
	entity := r.toSummaryEntity(summary)

	err := r.db.WithContext(ctx).Create(entity).Error
	if err != nil {
		return fmt.Errorf("failed to save summary: %w", err)
	}

	return nil
}

// LoadSummary 加载摘要
func (r *MemoryRepository) LoadSummary(ctx context.Context, id string) (*memory.Summary, error) {
	var entity ConversationSummaryEntity

	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&entity).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, memory.ErrSummaryNotFound
		}
		return nil, fmt.Errorf("failed to load summary: %w", err)
	}

	return r.toDomainSummary(&entity), nil
}

// LoadSummariesBySession 加载会话的所有摘要
func (r *MemoryRepository) LoadSummariesBySession(ctx context.Context, sessionID string) ([]*memory.Summary, error) {
	var entities []*ConversationSummaryEntity

	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("time_range_start ASC").
		Find(&entities).Error

	if err != nil {
		return nil, fmt.Errorf("failed to load summaries by session: %w", err)
	}

	summaries := make([]*memory.Summary, len(entities))
	for i, entity := range entities {
		summaries[i] = r.toDomainSummary(entity)
	}

	return summaries, nil
}

// LoadLatestSummary 加载最新摘要
func (r *MemoryRepository) LoadLatestSummary(ctx context.Context, sessionID string) (*memory.Summary, error) {
	var entity ConversationSummaryEntity

	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("time_range_end DESC").
		First(&entity).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, memory.ErrSummaryNotFound
		}
		return nil, fmt.Errorf("failed to load latest summary: %w", err)
	}

	return r.toDomainSummary(&entity), nil
}

// UpdateSummary 更新摘要
func (r *MemoryRepository) UpdateSummary(ctx context.Context, summary *memory.Summary) error {
	entity := r.toSummaryEntity(summary)

	err := r.db.WithContext(ctx).
		Model(&ConversationSummaryEntity{}).
		Where("id = ?", summary.ID).
		Updates(entity).Error

	if err != nil {
		return fmt.Errorf("failed to update summary: %w", err)
	}

	return nil
}

// DeleteSummary 删除摘要
func (r *MemoryRepository) DeleteSummary(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).
		Delete(&ConversationSummaryEntity{}, "id = ?", id).Error

	if err != nil {
		return fmt.Errorf("failed to delete summary: %w", err)
	}

	return nil
}

// ========================================
// Token 统计
// ========================================

// GetTokenUsage 获取会话的 token 使用情况
func (r *MemoryRepository) GetTokenUsage(ctx context.Context, sessionID string) (*memory.TokenUsage, error) {
	var result struct {
		TotalMessages  int64
		TotalTokens    int64
		CompressedCount int64
	}

	// 统计总消息数和总 tokens
	err := r.db.WithContext(ctx).
		Model(&ConversationMessageEntity{}).
		Where("session_id = ?", sessionID).
		Where("deleted_at IS NULL").
		Select("COUNT(*) as total_messages, COALESCE(SUM(tokens), 0) as total_tokens").
		Scan(&result).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get token usage: %w", err)
	}

	// 统计已压缩的消息数
	var compressedCount int64
	err = r.db.WithContext(ctx).
		Model(&ConversationMessageEntity{}).
		Where("session_id = ?", sessionID).
		Where("compressed_at IS NOT NULL").
		Count(&compressedCount).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get compressed count: %w", err)
	}

	return &memory.TokenUsage{
		SessionID:       sessionID,
		TotalMessages:   int(result.TotalMessages),
		TotalTokens:     int(result.TotalTokens),
		CompressedCount: int(compressedCount),
		OffloadedCount:  0, // TODO: 实现卸载统计
	}, nil
}

// ========================================
// 实体转换方法
// ========================================

// toMessageEntity 领域消息转实体
func (r *MemoryRepository) toMessageEntity(msg *memory.Message) *ConversationMessageEntity {
	entity := &ConversationMessageEntity{
		ID:            msg.ID,
		RequestID:     msg.RequestID,
		SessionID:     msg.SessionID,
		TenantID:      msg.TenantID,
		UserID:        msg.UserID,
		MessageType:   string(msg.Type),
		Role:          msg.Role,
		Content:       msg.Content,
		Tokens:        msg.Tokens,
		IsIntermediate: msg.IsIntermediate,
		Iteration:     msg.Iteration,
		CompressedAt:  msg.CompressedAt,
		SummaryID:     msg.SummaryID,
		CreatedAt:     msg.CreatedAt,
		UpdatedAt:     msg.UpdatedAt,
		DeletedAt:     msg.DeletedAt,
	}

	// 序列化工具调用
	if len(msg.ToolCalls) > 0 {
		toolCallsJSON, err := json.Marshal(msg.ToolCalls)
		if err == nil {
			entity.ToolCalls = string(toolCallsJSON)
		}
	}

	return entity
}

// toDomainMessage 实体转领域消息
func (r *MemoryRepository) toDomainMessage(entity *ConversationMessageEntity) (*memory.Message, error) {
	msg := &memory.Message{
		ID:            entity.ID,
		RequestID:     entity.RequestID,
		SessionID:     entity.SessionID,
		TenantID:      entity.TenantID,
		UserID:        entity.UserID,
		Type:          memory.MessageType(entity.MessageType),
		Role:          entity.Role,
		Content:       entity.Content,
		Tokens:        entity.Tokens,
		IsIntermediate: entity.IsIntermediate,
		Iteration:     entity.Iteration,
		CompressedAt:  entity.CompressedAt,
		SummaryID:     entity.SummaryID,
		CreatedAt:     entity.CreatedAt,
		UpdatedAt:     entity.UpdatedAt,
		DeletedAt:     entity.DeletedAt,
	}

	// 反序列化工具调用
	if entity.ToolCalls != "" {
		var toolCalls []memory.ToolCallInfo
		if err := json.Unmarshal([]byte(entity.ToolCalls), &toolCalls); err == nil {
			msg.ToolCalls = toolCalls
		}
	}

	return msg, nil
}

// toSummaryEntity 领域摘要转实体
func (r *MemoryRepository) toSummaryEntity(summary *memory.Summary) *ConversationSummaryEntity {
	return &ConversationSummaryEntity{
		ID:                   summary.ID,
		SessionID:            summary.SessionID,
		TenantID:             summary.TenantID,
		Content:              summary.Content,
		TimeRangeStart:       summary.TimeRangeStart,
		TimeRangeEnd:         summary.TimeRangeEnd,
		OriginalMessageCount: summary.OriginalMessageCount,
		OriginalTokens:       summary.OriginalTokens,
		CompressedTokens:     summary.CompressedTokens,
		CompressionRatio:     summary.CompressionRatio,
		CreatedAt:            summary.CreatedAt,
		UpdatedAt:            summary.UpdatedAt,
	}
}

// toDomainSummary 实体转领域摘要
func (r *MemoryRepository) toDomainSummary(entity *ConversationSummaryEntity) *memory.Summary {
	return &memory.Summary{
		ID:                   entity.ID,
		SessionID:            entity.SessionID,
		TenantID:             entity.TenantID,
		Content:              entity.Content,
		TimeRangeStart:       entity.TimeRangeStart,
		TimeRangeEnd:         entity.TimeRangeEnd,
		OriginalMessageCount: entity.OriginalMessageCount,
		OriginalTokens:       entity.OriginalTokens,
		CompressedTokens:     entity.CompressedTokens,
		CompressionRatio:     entity.CompressionRatio,
		CreatedAt:            entity.CreatedAt,
		UpdatedAt:            entity.UpdatedAt,
	}
}

// ========================================
// LongTermMemoryRepository 实现
// ========================================

// LongTermMemoryRepository MySQL 实现
type LongTermMemoryRepository struct {
	db *gorm.DB
}

// NewLongTermMemoryRepository 创建 LongTermMemoryRepository
func NewLongTermMemoryRepository(db *gorm.DB) memory.LongTermMemoryRepository {
	return &LongTermMemoryRepository{db: db}
}

// Store 存储长期记忆
func (r *LongTermMemoryRepository) Store(ctx context.Context, mem *memory.LongTermMemory) error {
	entity := r.toMemoryEntity(mem)

	err := r.db.WithContext(ctx).Create(entity).Error
	if err != nil {
		return fmt.Errorf("failed to store long term memory: %w", err)
	}

	return nil
}

// Retrieve 根据 ID 获取记忆
func (r *LongTermMemoryRepository) Retrieve(ctx context.Context, id string) (*memory.LongTermMemory, error) {
	var entity LongTermMemoryEntity

	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		First(&entity).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, memory.ErrMemoryNotFound
		}
		return nil, fmt.Errorf("failed to retrieve memory: %w", err)
	}

	return r.toDomainMemory(&entity)
}

// Search 搜索记忆（语义检索）
// 注意：这里实现基础搜索，实际向量搜索需要在 Milvus 中实现
func (r *LongTermMemoryRepository) Search(ctx context.Context, query *memory.MemorySearchQuery) ([]*memory.LongTermMemory, error) {
	var entities []*LongTermMemoryEntity

	q := r.db.WithContext(ctx).
		Where("tenant_id = ?", query.TenantID).
		Where("deleted_at IS NULL")

	if query.UserID != nil {
		q = q.Where("user_id = ?", *query.UserID)
	} else {
		q = q.Where("user_id IS NULL")
	}

	if query.Category != "" {
		q = q.Where("category = ?", query.Category)
	}

	if query.MinImportance > 0 {
		q = q.Where("importance >= ?", query.MinImportance)
	}

	err := q.
		Order("importance DESC, created_at DESC").
		Limit(query.MaxResults).
		Find(&entities).Error

	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	memories := make([]*memory.LongTermMemory, len(entities))
	for i, entity := range entities {
		mem, err := r.toDomainMemory(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert memory: %w", err)
		}
		memories[i] = mem
	}

	return memories, nil
}

// Update 更新记忆
func (r *LongTermMemoryRepository) Update(ctx context.Context, mem *memory.LongTermMemory) error {
	entity := r.toMemoryEntity(mem)

	err := r.db.WithContext(ctx).
		Model(&LongTermMemoryEntity{}).
		Where("id = ?", mem.ID).
		Updates(entity).Error

	if err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	return nil
}

// RecordAccess 原子记录一次访问：数据库侧自增，不经过 read-modify-write，
// 避免并发丢计数，也避免调用方在 goroutine 里改写已返回的实体（data race）。
func (r *LongTermMemoryRepository) RecordAccess(ctx context.Context, id string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).
		Model(&LongTermMemoryEntity{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"access_count":   gorm.Expr("access_count + 1"),
			"last_access_at": &now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to record memory access: %w", err)
	}

	return nil
}

// Delete 删除记忆（软删除）
func (r *LongTermMemoryRepository) Delete(ctx context.Context, id string) error {
	now := time.Now()

	err := r.db.WithContext(ctx).
		Model(&LongTermMemoryEntity{}).
		Where("id = ?", id).
		Update("deleted_at", &now).Error

	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	return nil
}

// ListByCategory 按类别列出记忆
func (r *LongTermMemoryRepository) ListByCategory(ctx context.Context, tenantID int64, category string, limit int) ([]*memory.LongTermMemory, error) {
	var entities []*LongTermMemoryEntity

	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("category = ?", category).
		Where("deleted_at IS NULL").
		Order("importance DESC, created_at DESC").
		Limit(limit).
		Find(&entities).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list memories by category: %w", err)
	}

	memories := make([]*memory.LongTermMemory, len(entities))
	for i, entity := range entities {
		mem, err := r.toDomainMemory(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert memory: %w", err)
		}
		memories[i] = mem
	}

	return memories, nil
}

// ListByUser 列出用户的记忆
func (r *LongTermMemoryRepository) ListByUser(ctx context.Context, tenantID, userID int64, limit int) ([]*memory.LongTermMemory, error) {
	var entities []*LongTermMemoryEntity

	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Where("user_id = ?", userID).
		Where("deleted_at IS NULL").
		Order("importance DESC, created_at DESC").
		Limit(limit).
		Find(&entities).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list memories by user: %w", err)
	}

	memories := make([]*memory.LongTermMemory, len(entities))
	for i, entity := range entities {
		mem, err := r.toDomainMemory(entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert memory: %w", err)
		}
		memories[i] = mem
	}

	return memories, nil
}

// ========================================
// 长期记忆实体转换
// ========================================

// toMemoryEntity 领域记忆转实体
func (r *LongTermMemoryRepository) toMemoryEntity(mem *memory.LongTermMemory) *LongTermMemoryEntity {
	entity := &LongTermMemoryEntity{
		ID:          mem.ID,
		TenantID:    mem.TenantID,
		UserID:      mem.UserID,
		Content:     mem.Content,
		Category:    mem.Category,
		Importance:  mem.Importance,
		AccessCount: mem.AccessCount,
		LastAccessAt: mem.LastAccessAt,
		CreatedAt:   mem.CreatedAt,
		UpdatedAt:   mem.UpdatedAt,
		DeletedAt:   mem.DeletedAt,
	}

	// 序列化向量
	if len(mem.Embedding) > 0 {
		embeddingJSON, err := json.Marshal(mem.Embedding)
		if err == nil {
			entity.Embedding = string(embeddingJSON)
		}
	}

	// 序列化元数据
	if mem.Metadata != nil {
		metadataJSON, err := json.Marshal(mem.Metadata)
		if err == nil {
			entity.Metadata = string(metadataJSON)
		}
	}

	return entity
}

// toDomainMemory 实体转领域记忆
func (r *LongTermMemoryRepository) toDomainMemory(entity *LongTermMemoryEntity) (*memory.LongTermMemory, error) {
	mem := &memory.LongTermMemory{
		ID:          entity.ID,
		TenantID:    entity.TenantID,
		UserID:      entity.UserID,
		Content:     entity.Content,
		Category:    entity.Category,
		Importance:  entity.Importance,
		AccessCount: entity.AccessCount,
		LastAccessAt: entity.LastAccessAt,
		Metadata:    make(map[string]interface{}),
		CreatedAt:   entity.CreatedAt,
		UpdatedAt:   entity.UpdatedAt,
		DeletedAt:   entity.DeletedAt,
	}

	// 反序列化向量
	if entity.Embedding != "" {
		var embedding []float32
		if err := json.Unmarshal([]byte(entity.Embedding), &embedding); err == nil {
			mem.Embedding = embedding
		}
	}

	// 反序列化元数据
	if entity.Metadata != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(entity.Metadata), &metadata); err == nil {
			mem.Metadata = metadata
		}
	}

	return mem, nil
}

// ========================================
// SQL NULL 处理辅助函数
// ========================================
