// Package persistence MySQL 评测结果仓储实现
package mysql

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"link/internal/model/evaluation"
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
