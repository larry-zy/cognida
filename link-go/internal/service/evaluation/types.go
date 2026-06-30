// Package evaluation 提供评测系统应用层类型定义
package evaluation

import "time"

// ========================================
// Evaluation Type Constants
// ========================================

// EvaluationType 评测类型
type EvaluationType string

const (
	// EvaluationTypeAgent Agent 评测
	EvaluationTypeAgent EvaluationType = "agent"
	// EvaluationTypeRAG RAG 评测
	EvaluationTypeRAG EvaluationType = "rag"
	// EvaluationTypeQA QA 评测
	EvaluationTypeQA EvaluationType = "qa"
)

// IsValid 检查评测类型是否有效
func (t EvaluationType) IsValid() bool {
	switch t {
	case EvaluationTypeAgent, EvaluationTypeRAG, EvaluationTypeQA:
		return true
	}
	return false
}

// ========================================
// Dataset Types
// ========================================

// DatasetType 数据集类型
type DatasetType string

const (
	// DatasetTypeFile 文件系统数据集
	DatasetTypeFile DatasetType = "file"
	// DatasetTypeDatabase 数据库数据集
	DatasetTypeDatabase DatasetType = "database"
)

// DatasetMetadata 数据集元数据
type DatasetMetadata struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        DatasetType    `json:"type"`
	EvalType    EvaluationType `json:"evaluation_type"`
	QACount     int            `json:"qa_count"`
	ModifiedAt  time.Time      `json:"modified_at"`
}

// QAPair QA 对
type QAPair struct {
	Question        string   `json:"question"`
	ReferenceAnswer string   `json:"reference_answer"`
	RelevantPIDs    []string `json:"relevant_pids,omitempty"` // 相关文档ID（用于检索评测）
	Context         string   `json:"context,omitempty"`       // 额外上下文
}

// Dataset 数据集
type Dataset struct {
	Metadata *DatasetMetadata `json:"metadata"`
	QAPairs  []*QAPair        `json:"qa_pairs"`
}

// ========================================
// Evaluation Task Types
// ========================================

// EvaluationTaskConfig 评测任务配置
type EvaluationTaskConfig struct {
	DatasetID string                 `json:"dataset_id"`
	Type      EvaluationType         `json:"type"`
	KnowledgeBaseID      string                 `json:"kb_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	ModelID   string                 `json:"model_id,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
}

// EvaluationConfig 评测配置
type EvaluationConfig struct {
	TopK            int                      `json:"top_k,omitempty"`            // 检索 top-k
	Graders         []string                 `json:"graders,omitempty"`          // 评分器列表
	LLMJudgeConfig  map[string]interface{}   `json:"llm_judge_config,omitempty"` // LLM 裁判配置
	TimeoutPerQA    int                      `json:"timeout_per_qa,omitempty"`   // 单 QA 超时（秒）
}

// ========================================
// Evaluation Result Types
// ========================================

// QAResult QA 评测结果
type QAResult struct {
	Question         string   `json:"question"`
	ReferenceAnswer  string   `json:"reference_answer"`
	GeneratedAnswer  string   `json:"generated_answer"`
	RetrievedChunks  []string `json:"retrieved_chunks,omitempty"`
	RelevantPIDs     []string `json:"relevant_pids,omitempty"`
	Success          bool     `json:"success"`
	Error            string   `json:"error,omitempty"`

	// 检索指标
	Precision *float64 `json:"precision,omitempty"`
	Recall    *float64 `json:"recall,omitempty"`
	NDCG      *float64 `json:"ndcg,omitempty"`
	RR        *float64 `json:"rr,omitempty"` // Reciprocal Rank

	// 生成指标
	ROUGE1 *float64 `json:"rouge_1,omitempty"`
	ROUGE2 *float64 `json:"rouge_2,omitempty"`
	ROUGEL *float64 `json:"rouge_l,omitempty"`
	BLEU1  *float64 `json:"bleu_1,omitempty"`
	BLEU2  *float64 `json:"bleu_2,omitempty"`
	BLEU4  *float64 `json:"bleu_4,omitempty"`

	// LLM Judge
	LLMScore     *float64 `json:"llm_score,omitempty"`
	LLMReasoning string   `json:"llm_reasoning,omitempty"`

	// 语义相似度
	SemanticSimilarity *float64 `json:"semantic_similarity,omitempty"`
}

// EvaluationResult 评测结果汇总
type EvaluationResult struct {
	TaskID    string              `json:"task_id"`
	DatasetID string              `json:"dataset_id"`
	KnowledgeBaseID      string              `json:"kb_id"`
	ModelID   string              `json:"model_id"`
	QAResults []*QAResult        `json:"qa_results"`

	// 聚合指标
	Precision  *float64            `json:"precision,omitempty"`
	Recall     *float64            `json:"recall,omitempty"`
	NDCG       *float64            `json:"ndcg,omitempty"`
	MRR        *float64            `json:"mrr,omitempty"` // Mean Reciprocal Rank
	MAP        *float64            `json:"map,omitempty"` // Mean Average Precision

	// ROUGE 聚合
	ROUGE1 *float64 `json:"rouge_1,omitempty"`
	ROUGE2 *float64 `json:"rouge_2,omitempty"`
	ROUGEL *float64 `json:"rouge_l,omitempty"`

	// BLEU 聚合
	BLEU1 *float64 `json:"bleu_1,omitempty"`
	BLEU2 *float64 `json:"bleu_2,omitempty"`
	BLEU4 *float64 `json:"bleu_4,omitempty"`

	// LLM Judge 聚合
	LLMJudgeScore    *float64 `json:"llm_judge_score,omitempty"`
	LLMJudgeReasoning string  `json:"llm_judge_reasoning,omitempty"`

	// 语义相似度聚合
	SemanticSimilarity *float64 `json:"semantic_similarity,omitempty"`

	// 统计
	TotalCount   int     `json:"total_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
}

// ComputeMetricsRequest Python 指标计算请求
type ComputeMetricsRequest struct {
	Items     []*ComputeItem         `json:"items"`
	Graders   []string               `json:"graders"`
	LLMJudge  map[string]interface{} `json:"llm_judge,omitempty"`
	Reference map[string]interface{} `json:"reference,omitempty"`
}

// ComputeItem 单个计算项
type ComputeItem struct {
	Question         string   `json:"question"`
	ReferenceAnswer  string   `json:"reference_answer"`
	GeneratedAnswer  string   `json:"generated_answer"`
	RetrievedPIDs    []string `json:"retrieved_pids,omitempty"`
	RelevantPIDs     []string `json:"relevant_pids,omitempty"`
}

// ComputeMetricsResponse Python 指标计算响应
type ComputeMetricsResponse struct {
	Items     []*ComputeItemResult `json:"items"`
	Aggregate map[string]float64   `json:"aggregate"`
}

// ComputeItemResult 单项计算结果
type ComputeItemResult struct {
	Index int `json:"index"`

	// 检索指标
	Precision *float64 `json:"precision,omitempty"`
	Recall    *float64 `json:"recall,omitempty"`
	NDCG      *float64 `json:"ndcg,omitempty"`
	RR        *float64 `json:"rr,omitempty"`

	// 生成指标
	ROUGE1 *float64 `json:"rouge_1,omitempty"`
	ROUGE2 *float64 `json:"rouge_2,omitempty"`
	ROUGEL *float64 `json:"rouge_l,omitempty"`
	BLEU1  *float64 `json:"bleu_1,omitempty"`
	BLEU2  *float64 `json:"bleu_2,omitempty"`
	BLEU4  *float64 `json:"bleu_4,omitempty"`

	// LLM Judge
	LLMScore     *float64 `json:"llm_score,omitempty"`
	LLMReasoning string   `json:"llm_reasoning,omitempty"`

	// 语义相似度
	SemanticSimilarity *float64 `json:"semantic_similarity,omitempty"`
}
