// Package evaluation 提供评测领域枚举定义
package evaluation

// ========================================
// EvaluationType 评测类型
// ========================================

// EvaluationType 评测类型枚举
type EvaluationType string

const (
	// EvaluationTypeAgent Agent 评测
	EvaluationTypeAgent EvaluationType = "agent"
	// EvaluationTypeRAG RAG 评测
	EvaluationTypeRAG EvaluationType = "rag"
	// EvaluationTypeQA QA 评测
	EvaluationTypeQA EvaluationType = "qa"
)

// String 返回字符串表示
func (t EvaluationType) String() string {
	return string(t)
}

// IsValid 检查评测类型是否有效
func (t EvaluationType) IsValid() bool {
	switch t {
	case EvaluationTypeAgent, EvaluationTypeRAG, EvaluationTypeQA:
		return true
	}
	return false
}

// ========================================
// TaskStatus 任务状态
// ========================================

// TaskStatus 任务状态枚举
type TaskStatus string

const (
	// TaskStatusPending 等待中
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusRunning 运行中
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusCompleted 已完成
	TaskStatusCompleted TaskStatus = "completed"
	// TaskStatusFailed 已失败
	TaskStatusFailed TaskStatus = "failed"
)

// String 返回字符串表示
func (s TaskStatus) String() string {
	return string(s)
}

// IsValid 检查状态是否有效
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusPending, TaskStatusRunning, TaskStatusCompleted, TaskStatusFailed:
		return true
	}
	return false
}

// CanTransitionTo 检查是否可以转换到目标状态
func (s TaskStatus) CanTransitionTo(target TaskStatus) bool {
	// 定义允许的状态转换
	transitions := map[TaskStatus][]TaskStatus{
		TaskStatusPending: {TaskStatusRunning, TaskStatusFailed},
		TaskStatusRunning: {TaskStatusCompleted, TaskStatusFailed},
		TaskStatusCompleted: {},
		TaskStatusFailed:   {},
	}

	allowedTargets, ok := transitions[s]
	if !ok {
		return false
	}

	for _, allowed := range allowedTargets {
		if allowed == target {
			return true
		}
	}
	return false
}
