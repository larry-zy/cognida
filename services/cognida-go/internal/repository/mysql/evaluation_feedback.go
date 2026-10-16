package mysql

import (
	"cognida/internal/model/evaluation"
	"context"
	"time"
)

// ClaimFeedbackEvaluation is a single-use fence. Queue redelivery and process
// overlap cannot replay a paid fixed-validation run.
func (r *EvaluationTaskRepository) ClaimFeedbackEvaluation(ctx context.Context, id string) (bool, error) {
	result := r.getDB(ctx).Model(&EvaluationTaskModel{}).Where("id=? AND status=? AND deleted_at IS NULL AND JSON_EXTRACT(snapshot,'$.feedback_view') IS NOT NULL", id, string(evaluation.TaskStatusPending)).Updates(map[string]any{"status": string(evaluation.TaskStatusRunning), "updated_at": time.Now().UTC()})
	return result.RowsAffected == 1, result.Error
}
