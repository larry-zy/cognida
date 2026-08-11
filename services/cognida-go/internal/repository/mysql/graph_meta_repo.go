// Package mysql 提供 Graph 领域的 MySQL 仓储实现
package mysql

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	domain_knowledge "cognida/internal/model/knowledge"
)

// ========================================
// GraphQueryRepository 实现
// ========================================

// graphQueryRepository 图谱查询仓储实现
type graphQueryRepository struct {
	db            *gorm.DB
	tenantEnabled bool
	chunkRepo     domain_knowledge.ChunkRepository
}

// NewGraphQueryRepository 创建图谱查询仓储
func NewGraphQueryRepository(db *gorm.DB, tenantEnabled bool) domain_knowledge.GraphQueryRepository {
	return &graphQueryRepository{
		db:            db,
		tenantEnabled: tenantEnabled,
	}
}

// SetChunkRepository 设置 Chunk 仓储（用于依赖注入）
func (r *graphQueryRepository) SetChunkRepository(chunkRepo domain_knowledge.ChunkRepository) {
	r.chunkRepo = chunkRepo
}

// GetChunksByGraphNodes 根据图谱节点名称获取关联的分片
func (r *graphQueryRepository) GetChunksByGraphNodes(ctx context.Context, kbID string, nodeNames []string) ([]*domain_knowledge.Chunk, error) {
	if len(nodeNames) == 0 {
		return []*domain_knowledge.Chunk{}, nil
	}

	if r.chunkRepo == nil {
		return nil, fmt.Errorf("chunk repository not initialized")
	}

	// 使用 Chunk 仓储查找分块
	enabled := true
	chunks, _, err := r.chunkRepo.FindByKnowledgeBaseID(ctx, kbID, &domain_knowledge.ChunkListQuery{
		IsEnabled: &enabled,
		Page:      1,
		PageSize:  500,
	})

	if err != nil {
		return nil, fmt.Errorf("查询分块失败: %w", err)
	}

	// 过滤包含节点名称的分块
	var result []*domain_knowledge.Chunk
	for _, chunk := range chunks {
		if r.containsNodeName(chunk.Content, nodeNames) {
			result = append(result, chunk)
		}
	}

	return result, nil
}

// GetKnowledgeByGraphNodes 根据图谱节点名称获取关联的知识条目
func (r *graphQueryRepository) GetKnowledgeByGraphNodes(ctx context.Context, knowledgeBaseID string, nodeNames []string) (*domain_knowledge.Knowledge, error) {
	if len(nodeNames) == 0 {
		return nil, fmt.Errorf("节点名称列表为空")
	}

	db := r.db.WithContext(ctx)

	// 构造节点名称匹配的 OR 分组：用独立 Session 生成带括号的子条件，
	// 避免 AND/OR 优先级导致 KB/enabled 作用域对 content 匹配失效（跨租户泄漏）。
	orGroup := r.db.Session(&gorm.Session{NewDB: true})
	for i, nodeName := range nodeNames {
		if i == 0 {
			orGroup = orGroup.Where("content LIKE ?", "%"+nodeName+"%")
		} else {
			orGroup = orGroup.Or("content LIKE ?", "%"+nodeName+"%")
		}
	}

	// 通过分块查找知识条目：KB/enabled 作用域对所有 content 匹配都生效
	var knowledgeIDs []string
	err := db.Table("chunks").
		Select("DISTINCT knowledge_id").
		Where("knowledge_base_id = ? AND is_enabled = ?", knowledgeBaseID, true).
		Where(orGroup).
		Pluck("knowledge_id", &knowledgeIDs).Error
	if err != nil {
		return nil, fmt.Errorf("查询知识条目ID失败: %w", err)
	}

	if len(knowledgeIDs) == 0 {
		return nil, fmt.Errorf("未找到关联的知识条目")
	}
	knowledgeID := knowledgeIDs[0]

	// 查询完整知识条目
	var knowledge KnowledgeModel
	err = db.Table("knowledges").
		Where("id = ?", knowledgeID).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		First(&knowledge).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("未找到关联的知识条目")
	}
	if err != nil {
		return nil, fmt.Errorf("查询知识条目失败: %w", err)
	}

	// 转换为领域实体
	return knowledge.ToDomain(), nil
}

// GetChunksByIDs 根据 chunk ID 列表获取分片内容
func (r *graphQueryRepository) GetChunksByIDs(ctx context.Context, kbID string, chunkIDs []string) ([]*domain_knowledge.Chunk, error) {
	if len(chunkIDs) == 0 {
		return []*domain_knowledge.Chunk{}, nil
	}

	if r.chunkRepo == nil {
		return nil, fmt.Errorf("chunk repository not initialized")
	}

	// 使用 Chunk 仓储查找分块
	chunks, err := r.chunkRepo.FindByKnowledgeID(ctx, kbID, false)
	if err != nil {
		return nil, fmt.Errorf("查询分块失败: %w", err)
	}

	// 按ID过滤
	result := make([]*domain_knowledge.Chunk, 0)
	chunkIDMap := make(map[string]bool)
	for _, id := range chunkIDs {
		chunkIDMap[id] = true
	}

	for _, chunk := range chunks {
		if chunkIDMap[chunk.ID] {
			result = append(result, chunk)
		}
	}

	return result, nil
}

// GetGraphStats 获取图谱统计信息
func (r *graphQueryRepository) GetGraphStats(ctx context.Context, knowledgeBaseID string) (*domain_knowledge.DetailedGraphStats, error) {
	db := r.db.WithContext(ctx)

	// 统计关联的分块数量
	var chunkCount int64
	err := db.Table("chunks").
		Where("knowledge_base_id = ? AND is_enabled = ?", knowledgeBaseID, true).
		Count(&chunkCount).Error
	if err != nil {
		return nil, fmt.Errorf("统计分块数量失败: %w", err)
	}

	// 计算基本统计数据
	stats := &domain_knowledge.DetailedGraphStats{
		NodeCount:      0, // 从 Neo4j 获取
		RelationCount:  0, // 从 Neo4j 获取
		AvgDegree:      0, // 从 Neo4j 获取
		AvgStrength:    0, // 从 Neo4j 获取
		AvgWeight:      0, // 从 Neo4j 获取
		MaxDegree:      0, // 从 Neo4j 获取
		IsolatedNodes:  0, // 从 Neo4j 获取
		ComponentCount: 1, // 至少有一个连通分量
	}

	_ = chunkCount // 用于将来扩展统计

	return stats, nil
}

// ========================================
// 辅助方法
// ========================================

// containsNodeName 检查内容是否包含任意节点名称
func (r *graphQueryRepository) containsNodeName(content string, nodeNames []string) bool {
	for _, nodeName := range nodeNames {
		if strings.Contains(content, nodeName) {
			return true
		}
	}
	return false
}

// 注意：本文件仅提供图谱「查询辅助」（GraphQueryRepository），从 chunks/knowledges
// 表按节点名回捞证据。图谱本体读写以 Neo4j 为唯一真源（见〔GO-3〕），
// 原 MySQL 版完整 GraphRepository 实现（graph_store.go）已删除。
