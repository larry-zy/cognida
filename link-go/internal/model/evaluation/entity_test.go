// Package evaluation 评测领域实体测试
package evaluation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEvaluationTask(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		tenantID  int64
		userID    int64
		datasetID string
		evalType  EvaluationType
		totalCount int
		wantErr   bool
	}{
		{
			name:      "valid task",
			id:        "test-123",
			tenantID:  1,
			userID:    10,
			datasetID: "ds-001",
			evalType:  EvaluationTypeRAG,
			totalCount: 100,
			wantErr:   false,
		},
		{
			name:      "agent evaluation type",
			id:        "test-456",
			tenantID:  2,
			userID:    20,
			datasetID: "ds-002",
			evalType:  EvaluationTypeAgent,
			totalCount: 50,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := NewEvaluationTask(tt.id, tt.tenantID, tt.userID, tt.datasetID, tt.evalType, tt.totalCount)

			if task == nil {
				t.Fatal("NewEvaluationTask returned nil")
			}

			if task.ID != tt.id {
				t.Errorf("ID = %v, want %v", task.ID, tt.id)
			}
			if task.TenantID != tt.tenantID {
				t.Errorf("TenantID = %v, want %v", task.TenantID, tt.tenantID)
			}
			if task.UserID != tt.userID {
				t.Errorf("UserID = %v, want %v", task.UserID, tt.userID)
			}
			if task.DatasetID != tt.datasetID {
				t.Errorf("DatasetID = %v, want %v", task.DatasetID, tt.datasetID)
			}
			if task.Type != tt.evalType {
				t.Errorf("Type = %v, want %v", task.Type, tt.evalType)
			}
			if task.Status != TaskStatusPending {
				t.Errorf("Status = %v, want %v", task.Status, TaskStatusPending)
			}
			if task.TotalCount != tt.totalCount {
				t.Errorf("TotalCount = %v, want %v", task.TotalCount, tt.totalCount)
			}
			if task.SuccessCount != 0 {
				t.Errorf("SuccessCount = %v, want 0", task.SuccessCount)
			}
			if task.FailureCount != 0 {
				t.Errorf("FailureCount = %v, want 0", task.FailureCount)
			}
		})
	}
}

func TestEvaluationTask_SetStatus(t *testing.T) {
	task := NewEvaluationTask("test", 1, 1, "ds-001", EvaluationTypeRAG, 10)
	originalUpdatedAt := task.UpdatedAt
	time.Sleep(1 * time.Millisecond) // Ensure time difference

	task.SetStatus(TaskStatusRunning)

	if task.Status != TaskStatusRunning {
		t.Errorf("Status = %v, want %v", task.Status, TaskStatusRunning)
	}
	if !task.UpdatedAt.After(originalUpdatedAt) {
		t.Error("UpdatedAt should be updated after SetStatus")
	}
}

func TestEvaluationTask_SetError(t *testing.T) {
	task := NewEvaluationTask("test", 1, 1, "ds-001", EvaluationTypeRAG, 10)
	originalUpdatedAt := task.UpdatedAt
	time.Sleep(1 * time.Millisecond)

	errorMsg := "retrieval failed"
	task.SetError(errorMsg)

	if task.Status != TaskStatusFailed {
		t.Errorf("Status = %v, want %v", task.Status, TaskStatusFailed)
	}
	if task.ErrorMessage != errorMsg {
		t.Errorf("ErrorMessage = %v, want %v", task.ErrorMessage, errorMsg)
	}
	if !task.UpdatedAt.After(originalUpdatedAt) {
		t.Error("UpdatedAt should be updated after SetError")
	}
}

func TestEvaluationTask_IncrementProgress(t *testing.T) {
	task := NewEvaluationTask("test", 1, 1, "ds-001", EvaluationTypeRAG, 10)

	task.IncrementProgress()
	if task.SuccessCount != 1 {
		t.Errorf("SuccessCount = %v, want 1", task.SuccessCount)
	}

	task.IncrementProgress()
	if task.SuccessCount != 2 {
		t.Errorf("SuccessCount = %v, want 2", task.SuccessCount)
	}
}

func TestEvaluationTask_IncrementFailure(t *testing.T) {
	task := NewEvaluationTask("test", 1, 1, "ds-001", EvaluationTypeRAG, 10)

	task.IncrementFailure()
	if task.FailureCount != 1 {
		t.Errorf("FailureCount = %v, want 1", task.FailureCount)
	}

	task.IncrementFailure()
	if task.FailureCount != 2 {
		t.Errorf("FailureCount = %v, want 2", task.FailureCount)
	}
}

func TestEvaluationTask_IsCompleted(t *testing.T) {
	tests := []struct {
		name     string
		status   TaskStatus
		want     bool
	}{
		{
			name: "pending task",
			status: TaskStatusPending,
			want: false,
		},
		{
			name: "running task",
			status: TaskStatusRunning,
			want: false,
		},
		{
			name: "completed task",
			status: TaskStatusCompleted,
			want: true,
		},
		{
			name: "failed task",
			status: TaskStatusFailed,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := NewEvaluationTask("test", 1, 1, "ds-001", EvaluationTypeRAG, 10)
			task.SetStatus(tt.status)

			got := task.IsCompleted()
			if got != tt.want {
				t.Errorf("IsCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluationTaskConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EvaluationTaskConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &EvaluationTaskConfig{
				DatasetID: "ds-001",
				Type:      EvaluationTypeRAG,
				KnowledgeBaseID:      "kb-001",
			},
			wantErr: false,
		},
		{
			name: "valid agent config",
			config: &EvaluationTaskConfig{
				DatasetID: "ds-001",
				Type:      EvaluationTypeAgent,
				AgentID:   "agent-001",
			},
			wantErr: false,
		},
		{
			name: "missing dataset_id",
			config: &EvaluationTaskConfig{
				Type: EvaluationTypeRAG,
				KnowledgeBaseID: "kb-001",
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			config: &EvaluationTaskConfig{
				DatasetID: "ds-001",
				Type:      EvaluationType("invalid"),
			},
			wantErr: true,
		},
		{
			name: "RAG without kb_id",
			config: &EvaluationTaskConfig{
				DatasetID: "ds-001",
				Type:      EvaluationTypeRAG,
			},
			wantErr: false, // kb_id is optional
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluationType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		t    EvaluationType
		want bool
	}{
		{"RAG type", EvaluationTypeRAG, true},
		{"Agent type", EvaluationTypeAgent, true},
		{"QA type", EvaluationTypeQA, true},
		{"Invalid type", EvaluationType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.t.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluationResult_SetRetrievalMetrics(t *testing.T) {
	result := NewEvaluationResult("task-123", 0)

	precision, recall, ndcg, rr := 0.85, 0.90, 0.75, 0.80
	result.SetRetrievalMetrics(precision, recall, ndcg, rr)

	if result.Precision == nil || *result.Precision != precision {
		t.Errorf("Precision = %v, want %v", result.Precision, precision)
	}
	if result.Recall == nil || *result.Recall != recall {
		t.Errorf("Recall = %v, want %v", result.Recall, recall)
	}
	if result.NDCG == nil || *result.NDCG != ndcg {
		t.Errorf("NDCG = %v, want %v", result.NDCG, ndcg)
	}
	if result.RR == nil || *result.RR != rr {
		t.Errorf("RR = %v, want %v", result.RR, rr)
	}
}

func TestEvaluationResult_SetGenerationMetrics(t *testing.T) {
	result := NewEvaluationResult("task-123", 0)

	rouge1, rouge2, rougeL, bleu1, bleu2, bleu4 := 0.5, 0.4, 0.45, 0.6, 0.55, 0.65
	result.SetGenerationMetrics(rouge1, rouge2, rougeL, bleu1, bleu2, bleu4)

	if result.ROUGE1 == nil || *result.ROUGE1 != rouge1 {
		t.Errorf("ROUGE1 = %v, want %v", result.ROUGE1, rouge1)
	}
	if result.ROUGE2 == nil || *result.ROUGE2 != rouge2 {
		t.Errorf("ROUGE2 = %v, want %v", result.ROUGE2, rouge2)
	}
	if result.ROUGEL == nil || *result.ROUGEL != rougeL {
		t.Errorf("ROUGEL = %v, want %v", result.ROUGEL, rougeL)
	}
	if result.BLEU1 == nil || *result.BLEU1 != bleu1 {
		t.Errorf("BLEU1 = %v, want %v", result.BLEU1, bleu1)
	}
	if result.BLEU2 == nil || *result.BLEU2 != bleu2 {
		t.Errorf("BLEU2 = %v, want %v", result.BLEU2, bleu2)
	}
	if result.BLEU4 == nil || *result.BLEU4 != bleu4 {
		t.Errorf("BLEU4 = %v, want %v", result.BLEU4, bleu4)
	}
}

func TestEvaluationResult_SetLLMJudge(t *testing.T) {
	result := NewEvaluationResult("task-123", 0)

	score := 0.9
	reasoning := "Good answer"
	result.SetLLMJudge(score, reasoning)

	if result.LLMScore == nil || *result.LLMScore != score {
		t.Errorf("LLMScore = %v, want %v", result.LLMScore, score)
	}
	if result.LLMReasoning != reasoning {
		t.Errorf("LLMReasoning = %v, want %v", result.LLMReasoning, reasoning)
	}
}

func TestEvaluationResult_MarkSuccess(t *testing.T) {
	result := NewEvaluationResult("task-123", 0)
	result.MarkSuccess()

	if !result.Success {
		t.Error("Success should be true after MarkSuccess")
	}
}

func TestEvaluationResult_MarkError(t *testing.T) {
	result := NewEvaluationResult("task-123", 0)
	errMsg := "generation failed"
	result.MarkError(errMsg)

	if result.Success {
		t.Error("Success should be false after MarkError")
	}
	if result.Error != errMsg {
		t.Errorf("Error = %v, want %v", result.Error, errMsg)
	}
}

func TestEvaluationTaskConfig_JSON(t *testing.T) {
	// Test JSON serialization
	config := &EvaluationTaskConfig{
		DatasetID: "ds-001",
		Type:      EvaluationTypeRAG,
		KnowledgeBaseID:      "kb-001",
		Config: map[string]interface{}{
			"top_k": 5,
			"graders": []string{"rouge", "bleu"},
		},
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal and verify
	var unmarshaled EvaluationTaskConfig
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if unmarshaled.DatasetID != config.DatasetID {
		t.Errorf("DatasetID = %v, want %v", unmarshaled.DatasetID, config.DatasetID)
	}
	if unmarshaled.Type != config.Type {
		t.Errorf("Type = %v, want %v", unmarshaled.Type, config.Type)
	}
	if unmarshaled.KnowledgeBaseID != config.KnowledgeBaseID {
		t.Errorf("KnowledgeBaseID = %v, want %v", unmarshaled.KnowledgeBaseID, config.KnowledgeBaseID)
	}
}
