// Package task provides task-related domain entities
package task

import "time"

// ========================================
// Task Status Constants
// ========================================

const (
	// TaskStatusPending 等待执行
	TaskStatusPending = "pending"
	// TaskStatusProcessing 执行中
	TaskStatusProcessing = "processing"
	// TaskStatusCompleted 执行完成
	TaskStatusCompleted = "completed"
	// TaskStatusFailed 执行失败
	TaskStatusFailed = "failed"
	// TaskStatusCancelled 已取消
	TaskStatusCancelled = "cancelled"
)

// ========================================
// Task Type Constants
// ========================================

const (
	// TaskTypeEvaluation 评测任务
	TaskTypeEvaluation = "evaluation"
	// TaskTypeDocumentParse 文档解析
	TaskTypeDocumentParse = "document_parse"
	// TaskTypeMLInference ML推理
	TaskTypeMLInference = "ml_inference"
	// TaskTypeKBIndex 知识库索引
	TaskTypeKBIndex = "kb_index"
	// TaskTypePipelineRun Pipeline执行
	TaskTypePipelineRun = "pipeline_run"
	// TaskTypeAgentSubtask Agent子任务
	TaskTypeAgentSubtask = "agent_subtask"
)

// ========================================
// Task Entity
// ========================================

// Task 任务实体
type Task struct {
	ID            string                 `json:"id" gorm:"primaryKey;size:36"`
	TenantID      int64                  `json:"tenant_id" gorm:"not null;index:idx_tenant_id"`
	UserID        int64                  `json:"user_id,omitempty" gorm:"index:idx_user_id"`
	Type          string                 `json:"type" gorm:"not null;size:50;index:idx_type"`
	TargetID      string                 `json:"target_id,omitempty" gorm:"size:100;index:idx_target_id"`
	Payload       map[string]interface{} `json:"payload,omitempty" gorm:"type:json"`
	Status        string                 `json:"status" gorm:"not null;size:20;default:pending;index:idx_status"`
	Result        map[string]interface{} `json:"result,omitempty" gorm:"type:json"`
	ErrorMessage  string                 `json:"error_message,omitempty" gorm:"type:text"`
	RetryCount    int                    `json:"retry_count" gorm:"not null;default:0"`
	MaxRetries    int                    `json:"max_retries" gorm:"not null;default:3"`
	TimeoutSeconds int                   `json:"timeout_seconds,omitempty"`
	ParentID      string                 `json:"parent_id,omitempty" gorm:"size:36;index:idx_parent_id"`
	WorkerID      string                 `json:"worker_id,omitempty" gorm:"size:100"`
	StartedAt     *time.Time             `json:"started_at,omitempty"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	CreatedAt     time.Time              `json:"created_at" gorm:"autoCreateTime;index:idx_created_at"`
	UpdatedAt     time.Time              `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt     *time.Time             `json:"deleted_at,omitempty" gorm:"index:idx_deleted_at"`
}

// IsPending 是否等待中
func (t *Task) IsPending() bool {
	return t.Status == TaskStatusPending
}

// IsProcessing 是否执行中
func (t *Task) IsProcessing() bool {
	return t.Status == TaskStatusProcessing
}

// IsCompleted 是否已完成
func (t *Task) IsCompleted() bool {
	return t.Status == TaskStatusCompleted
}

// IsFailed 是否失败
func (t *Task) IsFailed() bool {
	return t.Status == TaskStatusFailed
}

// CanRetry 是否可以重试
func (t *Task) CanRetry() bool {
	return t.IsFailed() && t.RetryCount < t.MaxRetries
}

// MarkProcessing 标记为处理中
func (t *Task) MarkProcessing() {
	t.Status = TaskStatusProcessing
	now := time.Now()
	t.StartedAt = &now
}

// MarkCompleted 标记为完成
func (t *Task) MarkCompleted(result map[string]interface{}) {
	t.Status = TaskStatusCompleted
	t.Result = result
	now := time.Now()
	t.CompletedAt = &now
}

// MarkFailed 标记为失败
func (t *Task) MarkFailed(errMsg string) {
	t.Status = TaskStatusFailed
	t.ErrorMessage = errMsg
	t.RetryCount++
	now := time.Now()
	t.CompletedAt = &now
}

// MarkCancelled 标记为取消
func (t *Task) MarkCancelled() {
	t.Status = TaskStatusCancelled
	now := time.Now()
	t.CompletedAt = &now
}
