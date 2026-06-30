// Package mysql provides transaction management for MySQL
package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	domain_knowledge "link/internal/model/knowledge"
)

// ========================================
// TransactionManager 事务管理器实现
// ========================================

// gormTransactionManager implements TransactionManager using GORM
type gormTransactionManager struct {
	db *gorm.DB
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(db *gorm.DB) domain_knowledge.TransactionManager {
	return &gormTransactionManager{db: db}
}

// WithTransaction executes a function within a transaction
func (tm *gormTransactionManager) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return tm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create a new context with the transaction
		txCtx := context.WithValue(ctx, "tx", tx)
		return fn(txCtx)
	})
}

// ========================================
// KnowledgeStatsQuerier 知识库统计查询器实现
// ========================================

// knowledgeStatsQuerier implements KnowledgeStatsQuerier
type knowledgeStatsQuerier struct {
	db *gorm.DB
}

// NewKnowledgeStatsQuerier creates a new knowledge stats querier
func NewKnowledgeStatsQuerier(db *gorm.DB) domain_knowledge.KnowledgeStatsQuerier {
	return &knowledgeStatsQuerier{db: db}
}

// GetStats retrieves statistics for a knowledge base
func (q *knowledgeStatsQuerier) GetStats(ctx context.Context, knowledgeBaseID string) (*domain_knowledge.KnowledgeBaseStats, error) {
	var stats domain_knowledge.KnowledgeBaseStats

	// 统计知识条目数量
	var knowledgeCount int64
	if err := q.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("knowledge_base_id = ? AND deleted_at IS NULL", knowledgeBaseID).
		Count(&knowledgeCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count knowledge: %w", err)
	}

	// 统计分块数量
	var chunkCount int64
	if err := q.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("knowledge_base_id = ? AND deleted_at IS NULL", knowledgeBaseID).
		Count(&chunkCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count chunks: %w", err)
	}

	// 统计总存储大小
	var totalSize int64
	if err := q.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("knowledge_base_id = ? AND deleted_at IS NULL", knowledgeBaseID).
		Select("COALESCE(SUM(LENGTH(content)), 0)").
		Scan(&totalSize).Error; err != nil {
		return nil, fmt.Errorf("failed to calculate storage size: %w", err)
	}

	stats.KnowledgeBaseID = knowledgeBaseID
	stats.KnowledgeCount = knowledgeCount
	stats.ChunkCount = chunkCount
	stats.TotalSize = totalSize

	return &stats, nil
}

// GetKnowledgeList retrieves a paginated list of knowledge items
func (q *knowledgeStatsQuerier) GetKnowledgeList(
	ctx context.Context,
	knowledgeBaseID string,
	page, pageSize int,
	status string,
) ([]*domain_knowledge.Knowledge, int64, error) {
	var models []*KnowledgeModel
	var total int64

	db := q.db.WithContext(ctx).Model(&KnowledgeModel{}).Where("knowledge_base_id = ?", knowledgeBaseID)

	// 添加状态过滤
	if status != "" && status != "all" {
		db = db.Where("parse_status = ?", status)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count knowledge: %w", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to find knowledge: %w", err)
	}

	result := make([]*domain_knowledge.Knowledge, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// GetKnowledgeListWithStatus retrieves a paginated list of knowledge items with multiple statuses
func (q *knowledgeStatsQuerier) GetKnowledgeListWithStatus(
	ctx context.Context,
	knowledgeBaseID string,
	page, pageSize int,
	statuses []string,
) ([]*domain_knowledge.Knowledge, int64, error) {
	var models []*KnowledgeModel
	var total int64

	db := q.db.WithContext(ctx).Model(&KnowledgeModel{}).Where("knowledge_base_id = ?", knowledgeBaseID)

	// 添加多状态过滤
	if len(statuses) > 0 {
		db = db.Where("parse_status IN ?", statuses)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count knowledge: %w", err)
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to find knowledge: %w", err)
	}

	result := make([]*domain_knowledge.Knowledge, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// DeleteKnowledgeWithChunks deletes a knowledge item and its chunks within a transaction
func (q *knowledgeStatsQuerier) DeleteKnowledgeWithChunks(ctx context.Context, knowledgeBaseID, knowledgeID string) error {
	return q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 删除关联的分块（验证 knowledge_base_id）
		if err := tx.Where("knowledge_base_id = ? AND knowledge_id = ?", knowledgeBaseID, knowledgeID).Delete(&ChunkModel{}).Error; err != nil {
			return fmt.Errorf("failed to delete chunks: %w", err)
		}

		// 删除知识条目（验证 knowledge_base_id）
		result := tx.Where("knowledge_base_id = ? AND id = ?", knowledgeBaseID, knowledgeID).Delete(&KnowledgeModel{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete knowledge: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledge not found in this knowledge base")
		}

		return nil
	})
}
