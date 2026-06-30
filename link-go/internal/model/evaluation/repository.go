// Package evaluation 提供评测领域仓储接口定义
package evaluation

import (
	"context"
)

// ========================================
// TaskFilter 任务过滤器
// ========================================

// TaskFilter 任务查询过滤器
type TaskFilter struct {
	TenantID  *int64          `json:"tenant_id,omitempty"`
	UserID    *int64          `json:"user_id,omitempty"`
	Type      *EvaluationType `json:"type,omitempty"`
	Status    *TaskStatus     `json:"status,omitempty"`
	DatasetID *string         `json:"dataset_id,omitempty"`
	DateFrom  *int64          `json:"date_from,omitempty"` // Unix timestamp
	DateTo    *int64          `json:"date_to,omitempty"`   // Unix timestamp
	Page      int             `json:"page"`
	PageSize  int             `json:"page_size"`
}

// ========================================
// EvaluationTaskRepository 评测任务仓储接口
// ========================================

// EvaluationTaskRepository 评测任务仓储接口
type EvaluationTaskRepository interface {
	// Create 创建评测任务
	Create(ctx context.Context, task *EvaluationTask) error

	// FindByID 根据 ID 查找任务
	FindByID(ctx context.Context, id string) (*EvaluationTask, error)

	// UpdateStatus 更新任务状态
	UpdateStatus(ctx context.Context, id string, status TaskStatus) error

	// UpdateProgress 更新任务进度（成功/失败计数）
	UpdateProgress(ctx context.Context, id string, success, failure int) error

	// UpdateError 更新任务错误信息
	UpdateError(ctx context.Context, id string, errorMsg string) error

	// SoftDelete 软删除任务
	SoftDelete(ctx context.Context, id string) error

	// List 根据过滤条件列出任务，返回任务列表和总数
	List(ctx context.Context, filter *TaskFilter) ([]*EvaluationTask, int64, error)

	// WithTransaction 在事务中执行操作
	WithTransaction(ctx context.Context, fn func(context.Context) error) error
}

// ========================================
// EvaluationResultRepository 评测结果仓储接口
// ========================================

// EvaluationResultRepository 评测结果仓储接口
type EvaluationResultRepository interface {
	// Create 创建评测结果
	Create(ctx context.Context, result *EvaluationResult) error

	// CreateBatch 批量创建评测结果
	CreateBatch(ctx context.Context, results []*EvaluationResult) error

	// FindByTaskID 根据任务 ID 查找所有结果
	FindByTaskID(ctx context.Context, taskID string) ([]*EvaluationResult, error)

	// FindByTaskIDWithPagination 根据任务 ID 查找结果（分页）
	FindByTaskIDWithPagination(ctx context.Context, taskID string, page, pageSize int) ([]*EvaluationResult, int64, error)

	// DeleteByTaskID 根据任务 ID 删除所有结果
	DeleteByTaskID(ctx context.Context, taskID string) error
}

// ========================================
// DatasetFilter 数据集过滤器
// ========================================

// DatasetFilter 数据集查询过滤器
type DatasetFilter struct {
	TenantID       *int64        `json:"tenant_id,omitempty"`
	UserID         *int64        `json:"user_id,omitempty"`
	Type           *DatasetType  `json:"type,omitempty"`
	EvaluationType *EvaluationType `json:"evaluation_type,omitempty"`
	Search         string        `json:"search,omitempty"` // 名称搜索
	Page           int           `json:"page"`
	PageSize       int           `json:"page_size"`
}

// ========================================
// DatasetRepository 数据集仓储接口
// ========================================

// DatasetRepository 数据集仓储接口
type DatasetRepository interface {
	// Create 创建数据集
	Create(ctx context.Context, dataset *Dataset) error

	// FindByID 根据 dataset_id 查找数据集
	FindByID(ctx context.Context, datasetID string) (*Dataset, error)

	// FindByIDWithTenant 根据 dataset_id 和 tenant_id 查找数据集
	FindByIDWithTenant(ctx context.Context, datasetID string, tenantID int64) (*Dataset, error)

	// List 根据过滤条件列出数据集
	List(ctx context.Context, filter *DatasetFilter) ([]*Dataset, int64, error)

	// Update 更新数据集元数据
	Update(ctx context.Context, dataset *Dataset) error

	// SoftDelete 软删除数据集
	SoftDelete(ctx context.Context, datasetID string, tenantID int64) error

	// AddSamples 批量添加样本
	AddSamples(ctx context.Context, datasetID string, samples []*DatasetRecord) error

	// ListSamples 查询样本列表（分页）
	ListSamples(ctx context.Context, datasetID string, tenantID int64, page, pageSize int) ([]*DatasetRecord, int64, error)

	// DeleteSample 删除单个样本
	DeleteSample(ctx context.Context, datasetID string, tenantID int64, sampleID int64) error

	// CountSamples 统计样本数量
	CountSamples(ctx context.Context, datasetID string) (int, error)
}
