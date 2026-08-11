// Package mysql 提供知识条目的 MySQL 持久化实现
package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domain_knowledge "cognida/internal/model/knowledge"
)

// ========================================
// KnowledgeRepository 知识条目仓储实现
// ========================================

// knowledgeRepository 知识条目仓储实现
type knowledgeRepository struct {
	db *gorm.DB
}

// NewKnowledgeRepository 创建知识条目仓储
func NewKnowledgeRepository(db *gorm.DB) domain_knowledge.KnowledgeRepository {
	return &knowledgeRepository{db: db}
}

// Create 创建知识条目
func (r *knowledgeRepository) Create(ctx context.Context, knowledge *domain_knowledge.Knowledge) error {
	var model KnowledgeModel
	model.FromDomainForCreate(knowledge)
	return r.db.WithContext(ctx).Create(&model).Error
}

// CreateBatch 批量创建知识条目
func (r *knowledgeRepository) CreateBatch(ctx context.Context, knowledgeList []*domain_knowledge.Knowledge) error {
	if len(knowledgeList) == 0 {
		return nil
	}
	models := make([]*KnowledgeModel, len(knowledgeList))
	for i, k := range knowledgeList {
		var model KnowledgeModel
		model.FromDomain(k)
		models[i] = &model
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

// FindByID 根据ID查找知识条目
func (r *knowledgeRepository) FindByID(ctx context.Context, id string) (*domain_knowledge.Knowledge, error) {
	var model KnowledgeModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("知识条目不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询知识条目失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByKnowledgeBaseID 根据知识库ID查找知识条目列表
func (r *knowledgeRepository) FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string, query *domain_knowledge.KnowledgeListQuery) ([]*domain_knowledge.Knowledge, int64, error) {
	var models []*KnowledgeModel
	var total int64

	db := r.db.WithContext(ctx).Model(&KnowledgeModel{}).Where("knowledge_base_id = ?", knowledgeBaseID)

	// 类型过滤
	if query.Type != "" {
		db = db.Where("type = ?", query.Type)
	}
	// 解析状态过滤
	if query.ParseStatus != "" {
		db = db.Where("parse_status = ?", query.ParseStatus)
	}
	// 启用状态过滤
	if query.EnableStatus != "" {
		db = db.Where("enable_status = ?", query.EnableStatus)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计知识条目数量失败: %w", err)
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	err := db.Order("created_at DESC").
		Limit(query.PageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询知识条目列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Knowledge, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindByTenantID 根据租户ID查找知识条目列表
func (r *knowledgeRepository) FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*domain_knowledge.Knowledge, int64, error) {
	var models []*KnowledgeModel
	var total int64

	db := r.db.WithContext(ctx).Model(&KnowledgeModel{}).Where("tenant_id = ?", tenantID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计知识条目数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询知识条目列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Knowledge, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// Update 更新知识条目
func (r *knowledgeRepository) Update(ctx context.Context, knowledge *domain_knowledge.Knowledge) error {
	var model KnowledgeModel
	model.FromDomain(knowledge)
	return r.db.WithContext(ctx).Save(&model).Error
}

// UpdateParseStatus 更新解析状态
func (r *knowledgeRepository) UpdateParseStatus(ctx context.Context, id string, parseStatus string, errorMessage string) error {
	updates := map[string]interface{}{
		"parse_status": parseStatus,
	}
	// 如果状态为 completed，同时设置 processed_at 时间戳
	if parseStatus == domain_knowledge.ParseStatusCompleted {
		updates["processed_at"] = time.Now()
	}
	if errorMessage != "" {
		updates["error_message"] = errorMessage
	}
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateChunkCount 更新分块数量
func (r *knowledgeRepository) UpdateChunkCount(ctx context.Context, id string, chunkCount int) error {
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id = ?", id).
		Update("chunk_count", chunkCount).Error
}

// UpdateEnableStatus 单独更新启用状态，不触碰其它字段。
func (r *knowledgeRepository) UpdateEnableStatus(ctx context.Context, id string, enableStatus string) error {
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id = ?", id).
		Update("enable_status", enableStatus).Error
}

// FilterEnabledKnowledgeIDs 返回入参 id 集合中「启用且未删除」条目的 id->true 映射，供检索后过滤。
// 只回查主键与启用状态，命中量受 top_k 约束，单次 IN 查询即可。
func (r *knowledgeRepository) FilterEnabledKnowledgeIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var enabledIDs []string
	err := r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id IN ? AND enable_status = ? AND deleted_at IS NULL", ids, domain_knowledge.EnableStatusEnabled).
		Pluck("id", &enabledIDs).Error
	if err != nil {
		return nil, fmt.Errorf("查询启用知识条目失败: %w", err)
	}
	for _, id := range enabledIDs {
		result[id] = true
	}
	return result, nil
}

// Delete 删除知识条目（软删除）
func (r *knowledgeRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&KnowledgeModel{}).Error
}

// DeleteBatch 批量删除知识条目（软删除）
func (r *knowledgeRepository) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&KnowledgeModel{}).Error
}

// HardDelete 硬删除知识条目
func (r *knowledgeRepository) HardDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&KnowledgeModel{}).Error
}

// HardDeleteBatch 批量硬删除
func (r *knowledgeRepository) HardDeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&KnowledgeModel{}).Error
}

// FindByStatus 根据解析状态查找待处理的知识条目
func (r *knowledgeRepository) FindByStatus(ctx context.Context, tenantID int64, parseStatus string, limit int) ([]*domain_knowledge.Knowledge, error) {
	var models []*KnowledgeModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND parse_status = ?", tenantID, parseStatus).
		Limit(limit).
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("查询待处理知识条目失败: %w", err)
	}

	result := make([]*domain_knowledge.Knowledge, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// FindByFileHash 根据文件哈希查找（去重用）
func (r *knowledgeRepository) FindByFileHash(ctx context.Context, tenantID int64, fileHash string) (*domain_knowledge.Knowledge, error) {
	var model KnowledgeModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND file_hash = ?", tenantID, fileHash).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询知识条目失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByFileHashInKB 在指定知识库范围内根据文件哈希查找未删除条目（防止重传相同文件）
func (r *knowledgeRepository) FindByFileHashInKB(ctx context.Context, knowledgeBaseID string, fileHash string) (*domain_knowledge.Knowledge, error) {
	if fileHash == "" {
		return nil, nil
	}
	var model KnowledgeModel
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND file_hash = ?", knowledgeBaseID, fileHash).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询知识条目失败: %w", err)
	}

	return model.ToDomain(), nil
}

// UpdateTagID 更新知识条目的标签ID
func (r *knowledgeRepository) UpdateTagID(ctx context.Context, id string, tagID int64) error {
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id = ?", id).
		Update("tag_id", tagID).Error
}

// RemoveTagID 移除知识条目的标签ID
func (r *knowledgeRepository) RemoveTagID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id = ?", id).
		Update("tag_id", nil).Error
}

// RemoveTagIDBatch 批量移除知识条目的标签ID
func (r *knowledgeRepository) RemoveTagIDBatch(ctx context.Context, ids []string, tagID int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id IN ? AND tag_id = ?", ids, tagID).
		Update("tag_id", nil).Error
}

// FindByTagID 根据标签ID查找知识条目列表
func (r *knowledgeRepository) FindByTagID(ctx context.Context, tenantID int64, tagID int64, page, pageSize int) ([]*domain_knowledge.Knowledge, int64, error) {
	var models []*KnowledgeModel
	var total int64

	db := r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("tenant_id = ? AND tag_id = ?", tenantID, tagID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计知识条目数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询知识条目列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Knowledge, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// AddTagIDBatch 批量为知识条目添加标签ID
func (r *knowledgeRepository) AddTagIDBatch(ctx context.Context, ids []string, tagID int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&KnowledgeModel{}).
		Where("id IN ?", ids).
		Update("tag_id", tagID).Error
}

// CountByKnowledgeBaseID 统计知识库的知识条目数量（过滤条件与 FindByKnowledgeBaseID 空查询一致）
func (r *knowledgeRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&KnowledgeModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Count(&count).Error

	return count, err
}

// CountByTenantID 统计租户的知识条目数量（过滤条件与 FindByTenantID 一致）
func (r *knowledgeRepository) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&KnowledgeModel{}).
		Where("tenant_id = ?", tenantID).
		Count(&count).Error

	return count, err
}

// ========================================
// ChunkRepository 分块仓储实现
// ========================================

// chunkRepository 分块仓储实现
type chunkRepository struct {
	db *gorm.DB
}

// NewChunkRepository 创建分块仓储
func NewChunkRepository(db *gorm.DB) domain_knowledge.ChunkRepository {
	return &chunkRepository{db: db}
}

// Create 创建分块
func (r *chunkRepository) Create(ctx context.Context, chunk *domain_knowledge.Chunk) error {
	var model ChunkModel
	model.FromDomainForCreate(chunk)
	return r.db.WithContext(ctx).Create(&model).Error
}

// CreateBatch 批量创建分块
func (r *chunkRepository) CreateBatch(ctx context.Context, chunks []*domain_knowledge.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	models := make([]*ChunkModel, len(chunks))
	for i, c := range chunks {
		var model ChunkModel
		model.FromDomainForCreate(c)
		models[i] = &model
	}
	// 分批插入，每次插入1000条
	batchSize := 1000
	for i := 0; i < len(models); i += batchSize {
		end := i + batchSize
		if end > len(models) {
			end = len(models)
		}
		if err := r.db.WithContext(ctx).Create(models[i:end]).Error; err != nil {
			return fmt.Errorf("批量插入分块失败: %w", err)
		}
	}
	return nil
}

// FindByID 根据ID查找分块
func (r *chunkRepository) FindByID(ctx context.Context, id string) (*domain_knowledge.Chunk, error) {
	var model ChunkModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("分块不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询分块失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByKnowledgeBaseID 根据知识库ID查找分块列表
func (r *chunkRepository) FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string, query *domain_knowledge.ChunkListQuery) ([]*domain_knowledge.Chunk, int64, error) {
	var models []*ChunkModel
	var total int64

	db := r.db.WithContext(ctx).Model(&ChunkModel{}).Where("knowledge_base_id = ?", knowledgeBaseID)

	// 是否启用过滤
	if query.IsEnabled != nil {
		db = db.Where("is_enabled = ?", *query.IsEnabled)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计分块数量失败: %w", err)
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	err := db.Order("chunk_index ASC").
		Limit(query.PageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询分块列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Chunk, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindByKnowledgeID 根据知识条目ID查找分块列表
func (r *chunkRepository) FindByKnowledgeID(ctx context.Context, knowledgeID string, enabledOnly bool) ([]*domain_knowledge.Chunk, error) {
	var models []*ChunkModel

	db := r.db.WithContext(ctx).Where("knowledge_id = ?", knowledgeID)

	if enabledOnly {
		db = db.Where("is_enabled = ?", true)
	}

	err := db.Order("chunk_index ASC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("查询分块列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Chunk, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// Update 更新分块
func (r *chunkRepository) Update(ctx context.Context, chunk *domain_knowledge.Chunk) error {
	var model ChunkModel
	model.FromDomain(chunk)
	return r.db.WithContext(ctx).Save(&model).Error
}

// UpdateEmbeddingID 更新向量ID
func (r *chunkRepository) UpdateEmbeddingID(ctx context.Context, id string, embeddingID string) error {
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id = ?", id).
		Update("embedding_id", embeddingID).Error
}

// UpdateBatchStatus 批量更新启用状态
func (r *chunkRepository) UpdateBatchStatus(ctx context.Context, ids []string, isEnabled bool) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id IN ?", ids).
		Update("is_enabled", isEnabled).Error
}

// Delete 删除分块（软删除）
func (r *chunkRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&ChunkModel{}).Error
}

// DeleteByKnowledgeID 删除知识条目的所有分块
func (r *chunkRepository) DeleteByKnowledgeID(ctx context.Context, knowledgeID string) error {
	return r.db.WithContext(ctx).Where("knowledge_id = ?", knowledgeID).Delete(&ChunkModel{}).Error
}

// DeleteByKnowledgeBaseID 删除知识库的所有分块
func (r *chunkRepository) DeleteByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) error {
	return r.db.WithContext(ctx).Where("knowledge_base_id = ?", knowledgeBaseID).Delete(&ChunkModel{}).Error
}

// HardDelete 硬删除分块
func (r *chunkRepository) HardDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&ChunkModel{}).Error
}

// HardDeleteBatch 批量硬删除
func (r *chunkRepository) HardDeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&ChunkModel{}).Error
}

// FindEnabledChunks 查找启用的分块（用于检索）
func (r *chunkRepository) FindEnabledChunks(ctx context.Context, knowledgeBaseID string, limit int) ([]*domain_knowledge.Chunk, error) {
	var models []*ChunkModel
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND is_enabled = ?", knowledgeBaseID, true).
		Limit(limit).
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("查询启用分块失败: %w", err)
	}

	result := make([]*domain_knowledge.Chunk, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// CountByKnowledgeBaseID 统计知识库的分块数量
func (r *chunkRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&ChunkModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Count(&count).Error

	return count, err
}

// UpdateTagID 更新分块的标签ID
func (r *chunkRepository) UpdateTagID(ctx context.Context, id string, tagID int64) error {
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id = ?", id).
		Update("tag_id", tagID).Error
}

// RemoveTagID 移除分块的标签ID
func (r *chunkRepository) RemoveTagID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id = ?", id).
		Update("tag_id", nil).Error
}

// UpdateTagIDBatch 批量更新分块的标签ID
func (r *chunkRepository) UpdateTagIDBatch(ctx context.Context, ids []string, tagID int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id IN ?", ids).
		Update("tag_id", tagID).Error
}

// RemoveTagIDBatch 批量移除分块的标签ID
func (r *chunkRepository) RemoveTagIDBatch(ctx context.Context, ids []string, tagID int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id IN ? AND tag_id = ?", ids, tagID).
		Update("tag_id", nil).Error
}

// FindByTagID 根据标签ID查找分块列表
func (r *chunkRepository) FindByTagID(ctx context.Context, tenantID int64, tagID int64, page, pageSize int) ([]*domain_knowledge.Chunk, int64, error) {
	var models []*ChunkModel
	var total int64

	db := r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("tenant_id = ? AND tag_id = ?", tenantID, tagID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计分块数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询分块列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Chunk, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// AddTagIDBatch 批量为分块添加标签ID
func (r *chunkRepository) AddTagIDBatch(ctx context.Context, ids []string, tagID int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&ChunkModel{}).
		Where("id IN ?", ids).
		Update("tag_id", tagID).Error
}

// FindByGraphNodes 根据图谱节点名称查找关联的分片
func (r *chunkRepository) FindByGraphNodes(ctx context.Context, knowledgeBaseID string, nodeNames []string) ([]*domain_knowledge.Chunk, error) {
	var models []*ChunkModel

	// 查找包含任一节点名称的分块
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND is_enabled = ?", knowledgeBaseID, true).
		Where("JSON_SEARCH(metadata, 'all', ?) IS NOT NULL", nodeNames).
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("查询图谱关联分块失败: %w", err)
	}

	result := make([]*domain_knowledge.Chunk, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, nil
}

// GetGraphStats 获取图谱关联的统计信息
func (r *chunkRepository) GetGraphStats(ctx context.Context, knowledgeBaseID string) (*domain_knowledge.GraphStats, error) {
	stats := &domain_knowledge.GraphStats{}

	// 统计节点数量
	if err := r.db.WithContext(ctx).
		Model(&ChunkModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Select("COUNT(DISTINCT JSON_EXTRACT(metadata, '$.nodes')) as node_count").
		Scan(&stats.NodeCount).Error; err != nil {
		return nil, fmt.Errorf("统计节点数量失败: %w", err)
	}

	// 统计分块数量
	chunkCount, err := r.CountByKnowledgeBaseID(ctx, knowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("统计分块数量失败: %w", err)
	}
	stats.ChunkCount = chunkCount

	return stats, nil
}

// ========================================
// KnowledgeBaseSettingRepository 知识库设置仓储实现
// ========================================

// kbSettingRepository 知识库设置仓储实现
type kbSettingRepository struct {
	db *gorm.DB
}

// NewKnowledgeBaseSettingRepository 创建知识库设置仓储
func NewKnowledgeBaseSettingRepository(db *gorm.DB) domain_knowledge.KnowledgeBaseSettingRepository {
	return &kbSettingRepository{db: db}
}

// Create 创建设置
func (r *kbSettingRepository) Create(ctx context.Context, setting *domain_knowledge.KnowledgeBaseSetting) error {
	var model KnowledgeBaseSettingModel
	model.FromDomain(setting)
	return r.db.WithContext(ctx).Create(&model).Error
}

// FindByKnowledgeBaseID 根据知识库ID查找设置
func (r *kbSettingRepository) FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (*domain_knowledge.KnowledgeBaseSetting, error) {
	var model KnowledgeBaseSettingModel
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("知识库设置不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询知识库设置失败: %w", err)
	}

	return model.ToDomain(), nil
}

// Update 更新设置
func (r *kbSettingRepository) Update(ctx context.Context, setting *domain_knowledge.KnowledgeBaseSetting) error {
	var model KnowledgeBaseSettingModel
	model.FromDomain(setting)
	return r.db.WithContext(ctx).Save(&model).Error
}

// UpdateRetrievalConfig 更新检索配置
func (r *kbSettingRepository) UpdateRetrievalConfig(ctx context.Context, knowledgeBaseID string, mode string, threshold float64, topK int) error {
	// 将配置更新到 settings_json 字段
	return r.db.WithContext(ctx).Model(&KnowledgeBaseSettingModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Updates(map[string]interface{}{
			"settings_json": fmt.Sprintf(`{"retrieval_mode":"%s","similarity_threshold":%f,"top_k":%d}`, mode, threshold, topK),
		}).Error
}

// UpdateModelConfig 更新模型配置
func (r *kbSettingRepository) UpdateModelConfig(ctx context.Context, knowledgeBaseID string, embeddingModelID, summaryModelID, rerankModelID string) error {
	updates := make(map[string]interface{})
	if embeddingModelID != "" {
		updates["embedding_model_id"] = embeddingModelID
	}
	if summaryModelID != "" {
		updates["summary_model_id"] = summaryModelID
	}
	if rerankModelID != "" {
		updates["rerank_model_id"] = rerankModelID
	}

	return r.db.WithContext(ctx).Model(&KnowledgeBaseSettingModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Updates(updates).Error
}

// UpdateProcessingConfig 更新处理配置
func (r *kbSettingRepository) UpdateProcessingConfig(ctx context.Context, knowledgeBaseID string, chunkingConfig, imageProcessingConfig, cosConfig, vlmConfig, extractConfig string) error {
	updates := make(map[string]interface{})
	if chunkingConfig != "" {
		updates["chunking_config"] = chunkingConfig
	}
	if imageProcessingConfig != "" {
		updates["image_processing_config"] = imageProcessingConfig
	}
	if cosConfig != "" {
		updates["cos_config"] = cosConfig
	}
	if vlmConfig != "" {
		updates["vlm_config"] = vlmConfig
	}
	if extractConfig != "" {
		updates["extract_config"] = extractConfig
	}

	return r.db.WithContext(ctx).Model(&KnowledgeBaseSettingModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Updates(updates).Error
}

// Delete 删除设置
func (r *kbSettingRepository) Delete(ctx context.Context, knowledgeBaseID string) error {
	return r.db.WithContext(ctx).Where("knowledge_base_id = ?", knowledgeBaseID).Delete(&KnowledgeBaseSettingModel{}).Error
}

// Exists 检查设置是否存在
func (r *kbSettingRepository) Exists(ctx context.Context, knowledgeBaseID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&KnowledgeBaseSettingModel{}).
		Where("knowledge_base_id = ?", knowledgeBaseID).
		Count(&count).Error

	return count > 0, err
}

// ========================================
// RetrievalSettingRepository 检索设置仓储实现
// ========================================

// retrievalSettingRepository 检索设置仓储实现
type retrievalSettingRepository struct {
	db *gorm.DB
}

// NewRetrievalSettingRepository 创建检索设置仓储
func NewRetrievalSettingRepository(db *gorm.DB) domain_knowledge.RetrievalSettingRepository {
	return &retrievalSettingRepository{db: db}
}

// Create 创建检索设置
func (r *retrievalSettingRepository) Create(ctx context.Context, setting *domain_knowledge.RetrievalSetting) error {
	var model RetrievalSettingModel
	model.FromDomain(setting)
	return r.db.WithContext(ctx).Create(&model).Error
}

// FindByID 根据ID查找检索设置
func (r *retrievalSettingRepository) FindByID(ctx context.Context, id int64) (*domain_knowledge.RetrievalSetting, error) {
	var model RetrievalSettingModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("检索设置不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询检索设置失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindBySessionID 根据会话ID查找检索设置
func (r *retrievalSettingRepository) FindBySessionID(ctx context.Context, sessionID string) (*domain_knowledge.RetrievalSetting, error) {
	var model RetrievalSettingModel
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		// 未找到检索设置是正常情况，返回 nil 而不是错误
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询检索设置失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindBySessionIDs 批量按会话ID查询检索设置，一次 IN 查询返回 sessionID→设置 映射（避免 ListSessions N+1）
func (r *retrievalSettingRepository) FindBySessionIDs(ctx context.Context, sessionIDs []string) (map[string]*domain_knowledge.RetrievalSetting, error) {
	result := make(map[string]*domain_knowledge.RetrievalSetting, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return result, nil
	}

	var models []RetrievalSettingModel
	if err := r.db.WithContext(ctx).
		Where("session_id IN ?", sessionIDs).
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("批量查询检索设置失败: %w", err)
	}

	for i := range models {
		if models[i].SessionID == nil {
			continue
		}
		result[*models[i].SessionID] = models[i].ToDomain()
	}
	return result, nil
}

// Update 更新检索设置
func (r *retrievalSettingRepository) Update(ctx context.Context, setting *domain_knowledge.RetrievalSetting) error {
	var model RetrievalSettingModel
	model.FromDomain(setting)
	return r.db.WithContext(ctx).Save(&model).Error
}

// UpsertBySessionID 根据会话ID创建或更新检索设置
func (r *retrievalSettingRepository) UpsertBySessionID(ctx context.Context, sessionID string, tenantID int64, ragConfig interface{}) error {
	// 使用 GORM 的 Clauses 实现 upsert
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			UpdateAll: true,
		}).
		Create(&RetrievalSettingModel{
			SessionID: &sessionID,
			TenantID:  tenantID,
		}).Error
}

// ========================================
// TagRepository 标签仓储实现
// ========================================

// tagRepository 标签仓储实现
type tagRepository struct {
	db *gorm.DB
}

// NewTagRepository 创建标签仓储
func NewTagRepository(db *gorm.DB) domain_knowledge.TagRepository {
	return &tagRepository{db: db}
}

// Create 创建标签
func (r *tagRepository) Create(ctx context.Context, tag *domain_knowledge.Tag) error {
	var model TagModel
	model.FromDomain(tag)
	return r.db.WithContext(ctx).Create(&model).Error
}

// CreateBatch 批量创建标签
func (r *tagRepository) CreateBatch(ctx context.Context, tags []*domain_knowledge.Tag) error {
	if len(tags) == 0 {
		return nil
	}
	models := make([]*TagModel, len(tags))
	for i, t := range tags {
		var model TagModel
		model.FromDomain(t)
		models[i] = &model
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

// FindByID 根据ID查找标签
func (r *tagRepository) FindByID(ctx context.Context, id int64) (*domain_knowledge.Tag, error) {
	var model TagModel
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("标签不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询标签失败: %w", err)
	}

	return model.ToDomain(), nil
}

// FindByKnowledgeBaseID 根据知识库ID查找标签列表
func (r *tagRepository) FindByKnowledgeBaseID(ctx context.Context, tenantID string, kbID int64, query *domain_knowledge.TagListQuery) ([]*domain_knowledge.Tag, int64, error) {
	var models []*TagModel
	var total int64

	db := r.db.WithContext(ctx).Model(&TagModel{}).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID)

	// 名称过滤
	if query.Name != "" {
		db = db.Where("name LIKE ?", "%"+query.Name+"%")
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计标签数量失败: %w", err)
	}

	// 分页查询
	offset := (query.Page - 1) * query.PageSize
	err := db.Order("sort_order ASC, created_at DESC").
		Limit(query.PageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询标签列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Tag, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindByTenantID 根据租户ID查找标签列表
func (r *tagRepository) FindByTenantID(ctx context.Context, tenantID string, page, pageSize int) ([]*domain_knowledge.Tag, int64, error) {
	var models []*TagModel
	var total int64

	db := r.db.WithContext(ctx).Model(&TagModel{}).Where("tenant_id = ?", tenantID)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计标签数量失败: %w", err)
	}

	offset := (page - 1) * pageSize
	err := db.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("查询标签列表失败: %w", err)
	}

	result := make([]*domain_knowledge.Tag, len(models))
	for i, m := range models {
		result[i] = m.ToDomain()
	}

	return result, total, nil
}

// FindByName 根据名称查找标签
func (r *tagRepository) FindByName(ctx context.Context, tenantID string, kbID int64, name string) (*domain_knowledge.Tag, error) {
	var model TagModel
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ? AND name = ?", tenantID, kbID, name).
		First(&model).Error

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("标签不存在")
	}
	if err != nil {
		return nil, fmt.Errorf("查询标签失败: %w", err)
	}

	return model.ToDomain(), nil
}

// Update 更新标签
func (r *tagRepository) Update(ctx context.Context, tag *domain_knowledge.Tag) error {
	var model TagModel
	model.FromDomain(tag)
	return r.db.WithContext(ctx).Save(&model).Error
}

// Delete 删除标签（软删除）
func (r *tagRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&TagModel{}).Error
}

// DeleteBatch 批量删除标签（软删除）
func (r *tagRepository) DeleteBatch(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&TagModel{}).Error
}

// DeleteByKnowledgeBaseID 删除知识库的所有标签
func (r *tagRepository) DeleteByKnowledgeBaseID(ctx context.Context, tenantID string, kbID int64) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).
		Delete(&TagModel{}).Error
}

// Exists 检查标签是否存在
func (r *tagRepository) Exists(ctx context.Context, tenantID string, kbID int64, name string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&TagModel{}).
		Where("tenant_id = ? AND knowledge_base_id = ? AND name = ?", tenantID, kbID, name).
		Count(&count).Error

	return count > 0, err
}

// CountByKnowledgeBaseID 统计知识库的标签数量
func (r *tagRepository) CountByKnowledgeBaseID(ctx context.Context, tenantID string, kbID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&TagModel{}).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).
		Count(&count).Error

	return count, err
}

// UpdateSortOrder 批量更新排序
func (r *tagRepository) UpdateSortOrder(ctx context.Context, tagOrders []domain_knowledge.TagSortOrder) error {
	if len(tagOrders) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, to := range tagOrders {
			if err := tx.Model(&TagModel{}).
				Where("id = ?", to.ID).
				Update("sort_order", to.SortOrder).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
