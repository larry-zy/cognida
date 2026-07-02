// Package persistence MySQL 评测任务仓储实现
package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"link/internal/model/evaluation"
)

var (
	_ evaluation.EvaluationTaskRepository = (*EvaluationTaskRepository)(nil)
)

// EvaluationTaskRepository 评测任务仓储实现
type EvaluationTaskRepository struct {
	db *gorm.DB
}

// NewEvaluationTaskRepository 创建评测任务仓储
func NewEvaluationTaskRepository(db *gorm.DB) *EvaluationTaskRepository {
	return &EvaluationTaskRepository{db: db}
}

// Create 创建评测任务
func (r *EvaluationTaskRepository) Create(ctx context.Context, task *evaluation.EvaluationTask) error {
	model := FromDomainEvaluationTask(task)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return nil
}

// FindByID 根据 ID 查找任务
func (r *EvaluationTaskRepository) FindByID(ctx context.Context, id string) (*evaluation.EvaluationTask, error) {
	var model EvaluationTaskModel
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, evaluation.ErrTaskNotFound
		}
		return nil, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}
	return model.ToDomain(), nil
}

// UpdateStatus 更新任务状态
func (r *EvaluationTaskRepository) UpdateStatus(ctx context.Context, id string, status evaluation.TaskStatus) error {
	result := r.db.WithContext(ctx).
		Model(&EvaluationTaskModel{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"status":     string(status),
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrTaskNotFound
	}
	return nil
}

// UpdateProgress 更新任务进度（成功/失败计数）
func (r *EvaluationTaskRepository) UpdateProgress(ctx context.Context, id string, success, failure int) error {
	result := r.db.WithContext(ctx).
		Model(&EvaluationTaskModel{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"success_count": success,
			"failure_count": failure,
			"updated_at":    time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrTaskNotFound
	}
	return nil
}

// UpdateError 更新任务错误信息
func (r *EvaluationTaskRepository) UpdateError(ctx context.Context, id string, errorMsg string) error {
	result := r.db.WithContext(ctx).
		Model(&EvaluationTaskModel{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"status":        string(evaluation.TaskStatusFailed),
			"error_message": errorMsg,
			"updated_at":    time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrTaskNotFound
	}
	return nil
}

// UpdateMetrics 更新任务级聚合指标
func (r *EvaluationTaskRepository) UpdateMetrics(ctx context.Context, id string, metrics *evaluation.TaskMetrics) error {
	var data []byte
	if metrics != nil {
		var err error
		data, err = json.Marshal(metrics)
		if err != nil {
			return fmt.Errorf("%w: marshal metrics: %v", evaluation.ErrRepository, err)
		}
	}

	result := r.db.WithContext(ctx).
		Model(&EvaluationTaskModel{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"metrics":    data,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrTaskNotFound
	}
	return nil
}

// SoftDelete 软删除任务
func (r *EvaluationTaskRepository) SoftDelete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&EvaluationTaskModel{})

	if result.Error != nil {
		return fmt.Errorf("%w: %v", evaluation.ErrRepository, result.Error)
	}
	if result.RowsAffected == 0 {
		return evaluation.ErrTaskNotFound
	}
	return nil
}

// List 根据过滤条件列出任务，返回任务列表和总数
func (r *EvaluationTaskRepository) List(ctx context.Context, filter *evaluation.TaskFilter) ([]*evaluation.EvaluationTask, int64, error) {
	query := r.db.WithContext(ctx).Model(&EvaluationTaskModel{}).Where("deleted_at IS NULL")

	// 应用过滤条件
	if filter.TenantID != nil {
		query = query.Where("tenant_id = ?", *filter.TenantID)
	}
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.Type != nil {
		query = query.Where("type = ?", string(*filter.Type))
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.DatasetID != nil {
		query = query.Where("dataset_id = ?", *filter.DatasetID)
	}
	if filter.DateFrom != nil {
		query = query.Where("created_at >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("created_at <= ?", *filter.DateTo)
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	// 分页和排序
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	offset := (filter.Page - 1) * filter.PageSize

	var models []*EvaluationTaskModel
	err := query.Order("created_at DESC").
		Limit(filter.PageSize).
		Offset(offset).
		Find(&models).Error

	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", evaluation.ErrRepository, err)
	}

	return ToDomainEvaluationTaskList(models), total, nil
}

// WithTransaction 在事务中执行操作
func (r *EvaluationTaskRepository) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, "tx", tx)
		return fn(txCtx)
	})
}
