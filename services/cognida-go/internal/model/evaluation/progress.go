// Package evaluation 定义评测领域的实体、接口与端口
package evaluation

import "context"

// ProgressStage 评测进度阶段
type ProgressStage string

const (
	// StageQueued 排队中
	StageQueued ProgressStage = "queued"
	// StageLoading 加载中
	StageLoading ProgressStage = "loading"
	// StageGeneration 生成中
	StageGeneration ProgressStage = "generation"
	// StageEvaluation 评测中
	StageEvaluation ProgressStage = "evaluation"
	// StageCompleted 已完成
	StageCompleted ProgressStage = "completed"
	// StageFailed 已失败
	StageFailed ProgressStage = "failed"
)

// Progress 评测进度
type Progress struct {
	Stage      ProgressStage `json:"stage"`
	Current    int           `json:"current"`
	Total      int           `json:"total"`
	Message    string        `json:"message"`
	Percentage int           `json:"percentage"`
	RetryCount int           `json:"retry_count"`
	Error      string        `json:"error,omitempty"`
}

// ProgressReader 进度读取端口（消费端接口）
// Handler 经此接口依赖进度缓存，避免直接耦合 infrastructure 实现。
type ProgressReader interface {
	GetProgress(ctx context.Context, taskID string) (*Progress, error)
}

// ProgressWriter 进度写入端口（消费端接口）
// Worker 经此接口上报进度与错误状态，避免直接耦合 infrastructure 实现。
type ProgressWriter interface {
	SetProgress(ctx context.Context, taskID string, progress *Progress) error
	SetError(ctx context.Context, taskID string, errMsg string, retryCount int) error
}
