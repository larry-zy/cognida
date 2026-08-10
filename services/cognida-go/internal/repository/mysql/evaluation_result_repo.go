// Package persistence MySQL 评测结果仓储实现
package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"cognida/internal/model/evaluation"
	"cognida/internal/pkg/pagination"
)

var (
	_ evaluation.EvaluationResultRepository = (*EvaluationResultRepository)(nil)
)

// EvaluationResultRepository 评测结果仓储实现
type EvaluationResultRepository struct {
	db *gorm.DB
}

// NewEvaluationResultRepository 创建评测结果仓储
func NewEvaluationResultRepository(db *gorm.DB) *EvaluationResultRepository {
	return &EvaluationResultRepository{db: db}
}

// Create 创建评测结果
func (r *EvaluationResultRepository) Create(ctx context.Context, result *evaluation.EvaluationResult) error {
	model := FromDomainEvaluationResult(result)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return nil
}

// CreateBatch 批量创建评测结果
func (r *EvaluationResultRepository) CreateBatch(ctx context.Context, results []*evaluation.EvaluationResult) error {
	if len(results) == 0 {
		return nil
	}

	models := make([]*EvaluationResultModel, len(results))
	for i, result := range results {
		models[i] = FromDomainEvaluationResult(result)
	}

	// 使用批量插入
	if err := r.db.WithContext(ctx).CreateInBatches(models, 100).Error; err != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return nil
}

// FindByTaskID 根据任务 ID 查找所有结果
func (r *EvaluationResultRepository) FindByTaskID(ctx context.Context, taskID string) ([]*evaluation.EvaluationResult, error) {
	var models []*EvaluationResultModel
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	return ToDomainEvaluationResultList(models), nil
}

// FindByTaskIDWithPagination 根据任务 ID 查找结果（分页）
func (r *EvaluationResultRepository) FindByTaskIDWithPagination(ctx context.Context, taskID string, page, pageSize int) ([]*evaluation.EvaluationResult, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var models []*EvaluationResultModel
	var total int64

	// 查询总数
	if err := r.db.WithContext(ctx).
		Model(&EvaluationResultModel{}).
		Where("task_id = ?", taskID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	// 查询分页数据
	offset := (page - 1) * pageSize
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("id ASC").
		Limit(pageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	return ToDomainEvaluationResultList(models), total, nil
}

// FindByTaskIDByCursor 游标（keyset）分页〔M5〕：按 id 升序（与 FindByTaskIDWithPagination 一致），
// 以 cursor 为锚点取下一页。id 为自增主键、天然单调唯一，直接以 id 作 keyset 锚点即可稳定翻页。
// 多取一条判断是否还有下一页，据此产出 nextCursor（空=末页）。
func (r *EvaluationResultRepository) FindByTaskIDByCursor(ctx context.Context, taskID string, cursor string, limit int) ([]*evaluation.EvaluationResult, string, error) {
	limit = pagination.NormalizeLimit(limit)

	db := r.db.WithContext(ctx).Where("task_id = ?", taskID)

	cur, err := pagination.Decode(cursor)
	if err != nil {
		// 畸形游标属客户端错误：保留 ErrInvalidCursor 链（handler 据此回 400），
		// 不套 ErrRepository（否则会被误判为服务器内部错误 500）。
		return nil, "", fmt.Errorf("解析游标失败: %w", err)
	}
	if !cur.IsZero() {
		// 升序按主键 id 翻页：取锚点之后（id 更大）的一侧。
		db = db.Where("id > ?", cur.ID)
	}

	var models []*EvaluationResultModel
	if err := db.Order("id ASC").Limit(limit + 1).Find(&models).Error; err != nil {
		return nil, "", fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	nextCursor := ""
	if len(models) > limit {
		last := models[limit-1] // 本页末行即下一页锚点
		nextCursor = pagination.Cursor{ID: last.ID}.Encode()
		models = models[:limit]
	}

	return ToDomainEvaluationResultList(models), nextCursor, nil
}

// DeleteByTaskID 根据任务 ID 删除所有结果
func (r *EvaluationResultRepository) DeleteByTaskID(ctx context.Context, taskID string) error {
	result := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Delete(&EvaluationResultModel{})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	return nil
}

// ReplaceByTaskID 在单个事务内先按 taskID 删旧结果、再批量插入新结果，保证幂等替换的原子性：
// 中途崩溃时事务回滚，不会出现「只删不插」导致整批结果行丢失。results 为空时仅删除旧行。
func (r *EvaluationResultRepository) ReplaceByTaskID(ctx context.Context, taskID string, results []*evaluation.EvaluationResult) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", taskID).Delete(&EvaluationResultModel{}).Error; err != nil {
			return err
		}
		if len(results) == 0 {
			return nil
		}
		models := make([]*EvaluationResultModel, len(results))
		for i, result := range results {
			models[i] = FromDomainEvaluationResult(result)
		}
		return tx.CreateInBatches(models, 100).Error
	})
	if err != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return nil
}
