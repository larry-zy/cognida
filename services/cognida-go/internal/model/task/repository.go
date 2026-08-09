// Package task 提供任务领域的仓储接口定义
package task

import (
	"context"
)

// ========================================
// TaskRepository 任务仓储接口
// ========================================

// TaskRepository 任务仓储接口
type TaskRepository interface {
	// Create 创建任务
	Create(ctx context.Context, task *Task) error

	// FindByID 根据ID查找任务
	FindByID(ctx context.Context, id string) (*Task, error)

	// FindByTenantID 根据租户ID查找任务列表
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*Task, int64, error)

	// FindByType 根据任务类型查找任务
	FindByType(ctx context.Context, tenantID int64, taskType string, status string, limit int) ([]*Task, error)

	// FindByStatus 根据状态查找任务
	FindByStatus(ctx context.Context, status string, limit int) ([]*Task, error)

	// FindByTargetID 根据目标资源ID查找任务
	FindByTargetID(ctx context.Context, tenantID int64, targetType, targetID string) ([]*Task, error)

	// FindPendingTasks 查找待处理任务
	FindPendingTasks(ctx context.Context, taskType string, limit int) ([]*Task, error)

	// FindByParentID 根据父任务ID查找子任务
	FindByParentID(ctx context.Context, parentID string) ([]*Task, error)

	// Update 更新任务
	Update(ctx context.Context, task *Task) error

	// UpdateStatus 更新任务状态
	UpdateStatus(ctx context.Context, id string, status string, errMsg string) error

	// UpdateResult 更新任务结果
	UpdateResult(ctx context.Context, id string, result map[string]interface{}) error

	// IncrementRetry 增加重试次数
	IncrementRetry(ctx context.Context, id string) error

	// Delete 删除任务（软删除）
	Delete(ctx context.Context, id string) error

	// DeleteByTenantID 删除租户的所有任务
	DeleteByTenantID(ctx context.Context, tenantID int64) error

	// Exists 检查任务是否存在
	Exists(ctx context.Context, id string) (bool, error)

	// CountByTenantID 统计租户的任务数量
	CountByTenantID(ctx context.Context, tenantID int64) (int64, error)

	// CountByTypeAndStatus 统计指定类型和状态的任务数量
	CountByTypeAndStatus(ctx context.Context, tenantID int64, taskType, status string) (int64, error)
}

// ========================================
// TaskQueue 任务队列接口
// ========================================

// TaskQueue 任务队列接口
type TaskQueue interface {
	// Enqueue 将任务加入队列
	Enqueue(ctx context.Context, task *Task) error

	// Dequeue 从队列取出任务（阻塞）
	Dequeue(ctx context.Context, timeout int) (*Task, error)

	// EnqueueDelay 延迟加入队列
	EnqueueDelay(ctx context.Context, task *Task, delaySeconds int) error

	// Complete 标记任务完成
	Complete(ctx context.Context, taskID string) error

	// Fail 标记任务失败
	Fail(ctx context.Context, taskID string, errMsg string) error
}
