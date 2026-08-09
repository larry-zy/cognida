// Package mysql 提供 Task 领域的 MySQL 仓储实现
package mysql

import (
	"context"

	"gorm.io/gorm"

	domaintask "cognida/internal/model/task"
)

// ========================================
// TaskRepository 实现
// ========================================

// taskRepository 任务仓储实现
type taskRepository struct {
	db *gorm.DB
}

// NewTaskRepository 创建任务仓储
func NewTaskRepository(db *gorm.DB) domaintask.TaskRepository {
	return &taskRepository{db: db}
}

// Create 创建任务
func (r *taskRepository) Create(ctx context.Context, task *domaintask.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// FindByID 根据ID查找任务
func (r *taskRepository) FindByID(ctx context.Context, id string) (*domaintask.Task, error) {
	var task domaintask.Task
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// FindByTenantID 根据租户ID查找任务列表
func (r *taskRepository) FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*domaintask.Task, int64, error) {
	var tasks []*domaintask.Task
	var total int64

	db := r.db.WithContext(ctx).Model(&domaintask.Task{})

	// 统计总数
	if err := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&tasks).Error

	return tasks, total, err
}

// FindByType 根据任务类型查找任务
func (r *taskRepository) FindByType(ctx context.Context, tenantID int64, taskType string, status string, limit int) ([]*domaintask.Task, error) {
	var tasks []*domaintask.Task
	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND type = ? AND deleted_at IS NULL", tenantID, taskType)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.
		Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// FindByStatus 根据状态查找任务
func (r *taskRepository) FindByStatus(ctx context.Context, status string, limit int) ([]*domaintask.Task, error) {
	var tasks []*domaintask.Task
	err := r.db.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", status).
		Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// FindByTargetID 根据目标资源ID查找任务
func (r *taskRepository) FindByTargetID(ctx context.Context, tenantID int64, targetType, targetID string) ([]*domaintask.Task, error) {
	var tasks []*domaintask.Task
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND type = ? AND target_id = ? AND deleted_at IS NULL", tenantID, targetType, targetID).
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

// FindPendingTasks 查找待处理任务
func (r *taskRepository) FindPendingTasks(ctx context.Context, taskType string, limit int) ([]*domaintask.Task, error) {
	var tasks []*domaintask.Task
	query := r.db.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", domaintask.TaskStatusPending)

	if taskType != "" {
		query = query.Where("type = ?", taskType)
	}

	err := query.
		Order("created_at ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// FindByParentID 根据父任务ID查找子任务
func (r *taskRepository) FindByParentID(ctx context.Context, parentID string) ([]*domaintask.Task, error) {
	var tasks []*domaintask.Task
	err := r.db.WithContext(ctx).
		Where("parent_id = ? AND deleted_at IS NULL", parentID).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// Update 更新任务
func (r *taskRepository) Update(ctx context.Context, task *domaintask.Task) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// UpdateStatus 更新任务状态
func (r *taskRepository) UpdateStatus(ctx context.Context, id string, status string, errMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if status == domaintask.TaskStatusCompleted || status == domaintask.TaskStatusFailed {
		updates["completed_at"] = gorm.Expr("NOW()")
	}
	return r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateResult 更新任务结果
func (r *taskRepository) UpdateResult(ctx context.Context, id string, result map[string]interface{}) error {
	return r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"result":       result,
			"status":       domaintask.TaskStatusCompleted,
			"completed_at": gorm.Expr("NOW()"),
		}).Error
}

// IncrementRetry 增加重试次数
func (r *taskRepository) IncrementRetry(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("id = ?", id).
		Update("retry_count", gorm.Expr("retry_count + 1")).Error
}

// Delete 删除任务（软删除）
func (r *taskRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("id = ?", id).
		Update("deleted_at", gorm.Expr("NOW()")).Error
}

// DeleteByTenantID 删除租户的所有任务
func (r *taskRepository) DeleteByTenantID(ctx context.Context, tenantID int64) error {
	return r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("tenant_id = ?", tenantID).
		Update("deleted_at", gorm.Expr("NOW()")).Error
}

// Exists 检查任务是否存在
func (r *taskRepository) Exists(ctx context.Context, id string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Count(&count).Error
	return count > 0, err
}

// CountByTenantID 统计租户的任务数量
func (r *taskRepository) CountByTenantID(ctx context.Context, tenantID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Count(&count).Error
	return count, err
}

// CountByTypeAndStatus 统计指定类型和状态的任务数量
func (r *taskRepository) CountByTypeAndStatus(ctx context.Context, tenantID int64, taskType, status string) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).
		Model(&domaintask.Task{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

	if taskType != "" {
		query = query.Where("type = ?", taskType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&count).Error
	return count, err
}
