// Package evaluation provides evaluation and task application services
package evaluation

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domaintask "cognida/internal/model/task"
)

// ========================================
// Task Service
// ========================================

// TaskService 任务应用服务
type TaskService struct {
	taskRepo  domaintask.TaskRepository
	taskQueue domaintask.TaskQueue
}

// NewTaskService 创建任务服务
func NewTaskService(
	taskRepo domaintask.TaskRepository,
	taskQueue domaintask.TaskQueue,
) *TaskService {
	return &TaskService{
		taskRepo:  taskRepo,
		taskQueue: taskQueue,
	}
}

// CreateWithQueue 创建任务并入队
func (s *TaskService) CreateWithQueue(
	ctx context.Context,
	tenantID int64,
	userID int64,
	taskType string,
	targetID string,
	payload map[string]interface{},
	options ...TaskOption,
) (*domaintask.Task, error) {
	// 创建任务
	task := &domaintask.Task{
		ID:       uuid.New().String(),
		TenantID: tenantID,
		UserID:   userID,
		Type:     taskType,
		TargetID: targetID,
		Payload:  payload,
		Status:   domaintask.TaskStatusPending,
	}

	// 应用选项
	for _, opt := range options {
		opt(task)
	}

	// 保存到数据库
	if err := s.taskRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("创建任务失败: %w", err)
	}

	// 入队
	if err := s.taskQueue.Enqueue(ctx, task); err != nil {
		return nil, fmt.Errorf("任务入队失败: %w", err)
	}

	return task, nil
}

// GetTask 获取任务
func (s *TaskService) GetTask(ctx context.Context, taskID string) (*domaintask.Task, error) {
	return s.taskRepo.FindByID(ctx, taskID)
}

// ListTasks 列出任务
func (s *TaskService) ListTasks(
	ctx context.Context,
	tenantID int64,
	page, pageSize int,
	filters ...TaskFilter,
) ([]*domaintask.Task, int64, error) {
	// 基础查询
	tasks, total, err := s.taskRepo.FindByTenantID(ctx, tenantID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	// 应用过滤（如果需要更复杂的过滤，可以在 repository 层添加方法）
	// 这里简化处理，实际可以在 repository 层添加更多查询方法

	return tasks, total, nil
}

// ListTasksByType 按类型列出任务
func (s *TaskService) ListTasksByType(
	ctx context.Context,
	tenantID int64,
	taskType string,
	status string,
	limit int,
) ([]*domaintask.Task, error) {
	return s.taskRepo.FindByType(ctx, tenantID, taskType, status, limit)
}

// CancelTask 取消任务
func (s *TaskService) CancelTask(ctx context.Context, taskID string) error {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}

	// 只能取消 pending 或 processing 状态的任务
	if !task.IsPending() && !task.IsProcessing() {
		return fmt.Errorf("任务状态为 %s，无法取消", task.Status)
	}

	task.MarkCancelled()
	return s.taskRepo.Update(ctx, task)
}

// UpdateTaskStatus 更新任务状态
func (s *TaskService) UpdateTaskStatus(ctx context.Context, taskID string, status string, errMsg string) error {
	return s.taskRepo.UpdateStatus(ctx, taskID, status, errMsg)
}

// UpdateTaskResult 更新任务结果
func (s *TaskService) UpdateTaskResult(ctx context.Context, taskID string, result map[string]interface{}) error {
	return s.taskRepo.UpdateResult(ctx, taskID, result)
}

// RetryTask 重试失败的任务
func (s *TaskService) RetryTask(ctx context.Context, taskID string) error {
	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		return err
	}

	if !task.CanRetry() {
		return fmt.Errorf("任务无法重试: retry_count=%d, max_retries=%d", task.RetryCount, task.MaxRetries)
	}

	// 重置状态为 pending
	task.Status = domaintask.TaskStatusPending
	task.ErrorMessage = ""
	task.WorkerID = ""
	task.StartedAt = nil

	if err := s.taskRepo.Update(ctx, task); err != nil {
		return err
	}

	// 重新入队
	return s.taskQueue.Enqueue(ctx, task)
}

// ========================================
// 选项和过滤器
// ========================================

// TaskOption 任务选项
type TaskOption func(*domaintask.Task)

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(maxRetries int) TaskOption {
	return func(t *domaintask.Task) {
		t.MaxRetries = maxRetries
	}
}

// WithTimeout 设置超时时间
func WithTimeout(timeoutSeconds int) TaskOption {
	return func(t *domaintask.Task) {
		t.TimeoutSeconds = timeoutSeconds
	}
}

// WithParentID 设置父任务ID
func WithParentID(parentID string) TaskOption {
	return func(t *domaintask.Task) {
		t.ParentID = parentID
	}
}

// TaskFilter 任务过滤器
type TaskFilter struct {
	Type     string
	Status   string
	TargetID string
}
