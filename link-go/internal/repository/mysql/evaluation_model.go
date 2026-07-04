// Package persistence MySQL 持久化模型
package mysql

import (
	"encoding/json"
	"time"

	"link/internal/model/evaluation"
)

// ========================================
// EvaluationTaskModel 评测任务模型
// ========================================

// EvaluationTaskModel 评测任务 GORM 模型
type EvaluationTaskModel struct {
	ID           string    `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	TenantID     int64     `gorm:"column:tenant_id;not null;index:idx_tenant_status" json:"tenant_id"`
	UserID       int64     `gorm:"column:user_id;not null;index" json:"user_id"`
	DatasetID    string    `gorm:"column:dataset_id;not null;type:varchar(64);index" json:"dataset_id"`
	Type         string    `gorm:"column:type;not null;type:varchar(32);index:idx_type_status" json:"type"`
	KnowledgeBaseID string    `gorm:"column:knowledge_base_id;type:varchar(64)" json:"knowledge_base_id,omitempty"`
	AgentID      string    `gorm:"column:agent_id;type:varchar(64)" json:"agent_id,omitempty"`
	ModelID      string    `gorm:"column:model_id;type:varchar(64)" json:"model_id,omitempty"`
	Config       []byte    `gorm:"column:config;type:json" json:"config,omitempty"`
	Status       string    `gorm:"column:status;not null;type:varchar(32);index:idx_status,idx_tenant_status,idx_type_status" json:"status"`
	ErrorMessage string    `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	TotalCount   int       `gorm:"column:total_count;not null;default:0" json:"total_count"`
	SuccessCount int       `gorm:"column:success_count;not null;default:0" json:"success_count"`
	FailureCount int       `gorm:"column:failure_count;not null;default:0" json:"failure_count"`
	Metrics      []byte    `gorm:"column:metrics;type:json" json:"metrics,omitempty"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;index:idx_created_at" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
	DeletedAt    *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// TableName 指定表名
func (EvaluationTaskModel) TableName() string {
	return "evaluation_tasks"
}

// ToDomain 转换为领域实体
func (m *EvaluationTaskModel) ToDomain() *evaluation.EvaluationTask {
	var config json.RawMessage
	if len(m.Config) > 0 {
		config = json.RawMessage(m.Config)
	}

	var metrics *evaluation.TaskMetrics
	if len(m.Metrics) > 0 {
		var parsed evaluation.TaskMetrics
		if err := json.Unmarshal(m.Metrics, &parsed); err == nil {
			metrics = &parsed
		}
	}

	return &evaluation.EvaluationTask{
		ID:           m.ID,
		TenantID:     m.TenantID,
		UserID:       m.UserID,
		DatasetID:    m.DatasetID,
		Type:         evaluation.EvaluationType(m.Type),
		KnowledgeBaseID:         m.KnowledgeBaseID,
		AgentID:      m.AgentID,
		ModelID:      m.ModelID,
		Config:       config,
		Status:       evaluation.TaskStatus(m.Status),
		ErrorMessage: m.ErrorMessage,
		TotalCount:   m.TotalCount,
		SuccessCount: m.SuccessCount,
		FailureCount: m.FailureCount,
		Metrics:      metrics,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		DeletedAt:    m.DeletedAt,
	}
}

// FromDomain 从领域实体转换为模型
func FromDomainEvaluationTask(task *evaluation.EvaluationTask) *EvaluationTaskModel {
	var config []byte
	if task.Config != nil {
		config = []byte(task.Config)
	}

	var metrics []byte
	if task.Metrics != nil {
		if data, err := json.Marshal(task.Metrics); err == nil {
			metrics = data
		}
	}

	return &EvaluationTaskModel{
		ID:           task.ID,
		TenantID:     task.TenantID,
		UserID:       task.UserID,
		DatasetID:    task.DatasetID,
		Type:         string(task.Type),
		KnowledgeBaseID:         task.KnowledgeBaseID,
		AgentID:      task.AgentID,
		ModelID:      task.ModelID,
		Config:       config,
		Status:       string(task.Status),
		ErrorMessage: task.ErrorMessage,
		TotalCount:   task.TotalCount,
		SuccessCount: task.SuccessCount,
		FailureCount: task.FailureCount,
		Metrics:      metrics,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
		DeletedAt:    task.DeletedAt,
	}
}

// ToDomainList 批量转换为领域实体
func ToDomainEvaluationTaskList(models []*EvaluationTaskModel) []*evaluation.EvaluationTask {
	tasks := make([]*evaluation.EvaluationTask, len(models))
	for i, m := range models {
		tasks[i] = m.ToDomain()
	}
	return tasks
}

// ========================================
// EvaluationResultModel 评测结果模型
// ========================================

// EvaluationResultModel 评测结果 GORM 模型
type EvaluationResultModel struct {
	ID                 int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TaskID             string    `gorm:"column:task_id;not null;type:varchar(64);index:idx_task_id" json:"task_id"`
	Question           string    `gorm:"column:question;not null;type:text" json:"question"`
	ReferenceAnswer    string    `gorm:"column:reference_answer;type:text" json:"reference_answer"`
	GeneratedAnswer    string    `gorm:"column:generated_answer;type:text" json:"generated_answer"`
	RetrievedPIDs      string    `gorm:"column:retrieved_pids;type:json" json:"retrieved_pids,omitempty"`
	RelevantPIDs       string    `gorm:"column:relevant_pids;type:json" json:"relevant_pids,omitempty"`
	Success            bool      `gorm:"column:success;not null;default:false" json:"success"`
	Error              string    `gorm:"column:error;type:text" json:"error,omitempty"`

	// 检索指标
	Precision *float64 `gorm:"column:precision;type:double" json:"precision,omitempty"`
	Recall    *float64 `gorm:"column:recall;type:double" json:"recall,omitempty"`
	NDCG      *float64 `gorm:"column:ndcg;type:double" json:"ndcg,omitempty"`
	RR        *float64 `gorm:"column:rr;type:double" json:"rr,omitempty"`

	// 生成指标
	ROUGE1 *float64 `gorm:"column:rouge_1;type:double" json:"rouge_1,omitempty"`
	ROUGE2 *float64 `gorm:"column:rouge_2;type:double" json:"rouge_2,omitempty"`
	ROUGEL *float64 `gorm:"column:rouge_l;type:double" json:"rouge_l,omitempty"`
	BLEU1  *float64 `gorm:"column:bleu_1;type:double" json:"bleu_1,omitempty"`
	BLEU2  *float64 `gorm:"column:bleu_2;type:double" json:"bleu_2,omitempty"`
	BLEU4  *float64 `gorm:"column:bleu_4;type:double" json:"bleu_4,omitempty"`

	// LLM Judge
	LLMScore     *float64 `gorm:"column:llm_score;type:double" json:"llm_score,omitempty"`
	LLMReasoning string   `gorm:"column:llm_reasoning;type:text" json:"llm_reasoning,omitempty"`

	// 语义相似度
	SemanticSimilarity *float64 `gorm:"column:semantic_similarity;type:double" json:"semantic_similarity,omitempty"`

	// 动态指标载体:注册表驱动的 name->value(JSON 列,与上面固定列并存以兼容)
	// 用 []byte 而非 string:空时为 nil 写入 SQL NULL,避免空串 '' 触发 JSON 列非法文本错误(与 Metrics 列一致)。
	Scores []byte `gorm:"column:scores;type:json" json:"scores,omitempty"`

	CreatedAt time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

// TableName 指定表名
func (EvaluationResultModel) TableName() string {
	return "evaluation_qa_results"
}

// ToDomain 转换为领域实体
func (m *EvaluationResultModel) ToDomain() *evaluation.EvaluationResult {
	result := &evaluation.EvaluationResult{
		ID:              m.ID,
		TaskID:          m.TaskID,
		Question:        m.Question,
		ReferenceAnswer: m.ReferenceAnswer,
		GeneratedAnswer: m.GeneratedAnswer,
		Success:         m.Success,
		Error:           m.Error,

		Precision:          m.Precision,
		Recall:             m.Recall,
		NDCG:               m.NDCG,
		RR:                 m.RR,
		ROUGE1:             m.ROUGE1,
		ROUGE2:             m.ROUGE2,
		ROUGEL:             m.ROUGEL,
		BLEU1:              m.BLEU1,
		BLEU2:              m.BLEU2,
		BLEU4:              m.BLEU4,
		LLMScore:           m.LLMScore,
		LLMReasoning:       m.LLMReasoning,
		SemanticSimilarity: m.SemanticSimilarity,
		CreatedAt:          m.CreatedAt,
	}

	// 解析 JSON 字段
	if m.RetrievedPIDs != "" {
		json.Unmarshal([]byte(m.RetrievedPIDs), &result.RetrievedPIDs)
	}
	if m.RelevantPIDs != "" {
		json.Unmarshal([]byte(m.RelevantPIDs), &result.RelevantPIDs)
	}

	// 动态指标载体:NULL/空列兼容为空 map
	result.Scores = map[string]float64{}
	if len(m.Scores) > 0 {
		json.Unmarshal(m.Scores, &result.Scores)
	}

	return result
}

// FromDomain 从领域实体转换为模型
func FromDomainEvaluationResult(result *evaluation.EvaluationResult) *EvaluationResultModel {
	model := &EvaluationResultModel{
		ID:                 result.ID,
		TaskID:             result.TaskID,
		Question:           result.Question,
		ReferenceAnswer:    result.ReferenceAnswer,
		GeneratedAnswer:    result.GeneratedAnswer,
		Success:            result.Success,
		Error:              result.Error,

		Precision:          result.Precision,
		Recall:             result.Recall,
		NDCG:               result.NDCG,
		RR:                 result.RR,
		ROUGE1:             result.ROUGE1,
		ROUGE2:             result.ROUGE2,
		ROUGEL:             result.ROUGEL,
		BLEU1:              result.BLEU1,
		BLEU2:              result.BLEU2,
		BLEU4:              result.BLEU4,
		LLMScore:           result.LLMScore,
		LLMReasoning:       result.LLMReasoning,
		SemanticSimilarity: result.SemanticSimilarity,
		CreatedAt:          result.CreatedAt,
	}

	// 序列化 JSON 字段（空数组也要序列化为 "[]"）
	if data, err := json.Marshal(result.RetrievedPIDs); err == nil {
		model.RetrievedPIDs = string(data)
	}
	if data, err := json.Marshal(result.RelevantPIDs); err == nil {
		model.RelevantPIDs = string(data)
	}

	// 动态指标载体:仅在有值时写入(空则 Scores 保持 nil→SQL NULL,读侧兼容为空 map)
	if len(result.Scores) > 0 {
		if data, err := json.Marshal(result.Scores); err == nil {
			model.Scores = data
		}
	}

	return model
}

// ToDomainList 批量转换为领域实体
func ToDomainEvaluationResultList(models []*EvaluationResultModel) []*evaluation.EvaluationResult {
	results := make([]*evaluation.EvaluationResult, len(models))
	for i, m := range models {
		results[i] = m.ToDomain()
	}
	return results
}
