// Package collaboration retains result aggregation contracts.
// Planning and durable dispatch are provided by service/taskcontrol; the former
// unused decomposer/dispatcher must not create a parallel orchestration path.
package collaboration

import (
	"time"

	infraagent "cognida/internal/service/agent/framework"
)

// SubTaskResult represents the result of a sub-task.
type SubTaskResult struct {
	Content     string                 `json:"content"`
	ToolCalls   []*infraagent.ToolCall `json:"tool_calls,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Duration    time.Duration          `json:"duration"`
	CompletedAt time.Time              `json:"completed_at"`
}

// ExecutionResult represents the result of a collaborative execution.
type ExecutionResult struct {
	PlanID      string                    `json:"plan_id"`
	Status      string                    `json:"status"` // completed, failed, partial
	SubTasks    map[string]*SubTaskResult `json:"sub_tasks"`
	TotalTasks  int                       `json:"total_tasks"`
	FailedTasks int                       `json:"failed_tasks"`
	StartedAt   time.Time                 `json:"started_at"`
	FinishedAt  time.Time                 `json:"finished_at"`
	Metadata    map[string]interface{}    `json:"metadata"`
}

// GetSucceededResults returns all succeeded task results.
func (r *ExecutionResult) GetSucceededResults() []*SubTaskResult {
	results := make([]*SubTaskResult, 0)
	for _, result := range r.SubTasks {
		if result.Error == "" {
			results = append(results, result)
		}
	}
	return results
}

// GetFailedResults returns all failed task results.
func (r *ExecutionResult) GetFailedResults() []*SubTaskResult {
	results := make([]*SubTaskResult, 0)
	for _, result := range r.SubTasks {
		if result.Error != "" {
			results = append(results, result)
		}
	}
	return results
}
