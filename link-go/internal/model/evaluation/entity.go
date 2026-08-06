// Package evaluation 提供评测领域实体定义
package evaluation

import (
	"encoding/json"
	"time"
)

// ========================================
// QAPair QA 对
// ========================================

// QAPair QA 对（用于评测输入）
type QAPair struct {
	Question        string   `json:"question"`
	ReferenceAnswer string   `json:"reference_answer"`
	RelevantPIDs    []string `json:"relevant_pids,omitempty"` // 相关文档ID（用于检索评测）
	Context         string   `json:"context,omitempty"`       // 额外上下文

	// Agent 评测期望标注（仅 Agent 类型使用，QA/RAG 留空）
	ExpectedTools []string `json:"expected_tools,omitempty"` // 期望调用的工具名（tool_selection/tool_order）
	ExpectedSteps []string `json:"expected_steps,omitempty"` // 期望步骤序列（trajectory_match/step_efficiency）

	// SQL 评测金标准 SQL（仅 SQL 类型使用）。为空时回退到 ReferenceAnswer——
	// sql 数据集的金标准 SQL 直接存在 reference_answer 列，不新增数据集列。
	GoldSQL string `json:"gold_sql,omitempty"`
}

// ========================================
// QAResult QA 评测结果（简化版，用于执行器）
// ========================================

// QAResult QA 评测结果（用于执行器返回）
type QAResult struct {
	Question         string   `json:"question"`
	ReferenceAnswer  string   `json:"reference_answer"`
	GeneratedAnswer  string   `json:"generated_answer"`
	RetrievedChunks  []string `json:"retrieved_chunks,omitempty"` // 检索到的分块正文（用于生成与忠实度类指标）
	RetrievedPIDs    []string `json:"retrieved_pids,omitempty"`   // 检索到的分块ID（用于检索指标：precision/recall/ndcg/mrr）
	RelevantPIDs     []string `json:"relevant_pids,omitempty"`
	Success          bool     `json:"success"`
	Error            string   `json:"error,omitempty"`

	// Agent 评测轨迹（仅 Agent 类型填充，供 tool_selection/trajectory/step_efficiency）
	ToolsUsed  []string `json:"tools_used,omitempty"`  // 实际调用的工具名（按调用顺序）
	Trajectory []string `json:"trajectory,omitempty"`  // 实际步骤序列
	TotalSteps int      `json:"total_steps,omitempty"` // 步骤总数

	// Agent 运行时基础指标（由 Go 执行器直接采集，非 Python 计算）
	LatencyMs  int64 `json:"latency_ms,omitempty"`  // 单轮对话墙钟耗时（毫秒）
	TokensUsed int   `json:"tokens_used,omitempty"` // 本轮消耗总 token 数
	LLMCalls   int   `json:"llm_calls,omitempty"`   // 本轮 LLM 调用次数（ReAct 迭代数）

	// RequestID 本条 QA 运行的子 request_id（<任务级 rid>#<序号>），指向该轮 Agent 运行落成的
	// trace_spans 调用链。前端评测详情据此深链到既有 trace 瀑布图，逐条 debug 工具入参/出参/绕路，
	// 无需在评测结果里重复存一份结构化轨迹。
	RequestID string `json:"request_id,omitempty"`

	// Agent 评测期望标注（从 QAPair 透传，供 Worker 组装 references；QA/RAG 留空）
	ExpectedTools []string `json:"expected_tools,omitempty"` // 期望调用的工具名
	ExpectedSteps []string `json:"expected_steps,omitempty"` // 期望步骤序列

	// SQL 评测运行时字段（仅 SQL 类型填充，透传给 Python 计算结构/执行准确率；均为瞬态，不落库）
	GeneratedSQL  string          `json:"generated_sql,omitempty"`   // Agent 生成的 SQL（取最后一次 sql_execute 调用）
	GoldSQL       string          `json:"gold_sql,omitempty"`        // 金标准 SQL（GoldSQL 或 ReferenceAnswer）
	ResultSet     [][]interface{} `json:"result_set,omitempty"`      // 生成 SQL 只读执行的完整结果集（供执行准确率比对）
	GoldResultSet [][]interface{} `json:"gold_result_set,omitempty"` // 金标准 SQL 只读执行的完整结果集

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

	// 动态指标载体:注册表驱动的 name->value(与上面固定字段并存以兼容)
	Scores map[string]float64 `json:"scores,omitempty"`
}

// ========================================
// EvaluationTaskConfig 评测任务配置
// ========================================

// EvaluationTaskConfig 评测任务配置
type EvaluationTaskConfig struct {
	DatasetID string                 `json:"dataset_id"`
	Type      EvaluationType         `json:"type"`
	KnowledgeBaseID string                 `json:"knowledge_base_id,omitempty"`
	AgentID   string                 `json:"agent_id,omitempty"`
	ModelID   string                 `json:"model_id,omitempty"`
	Config    map[string]interface{} `json:"config,omitempty"`
}

// Validate 验证配置
func (c *EvaluationTaskConfig) Validate() error {
	if c.DatasetID == "" {
		return ErrInvalidConfig
	}
	if !c.Type.IsValid() {
		return ErrInvalidEvalType
	}
	return nil
}

// ========================================
// EvaluationTask 实体
// ========================================

// TaskMetrics 任务级聚合指标
//
// 存放整批评测的聚合结果。其中检索/生成/LLM/语义类指标可由逐条 QAResult 求均值得到，
// 但 RAG 专属指标（faithfulness/context_relevance/noise_ratio）与语料级 MAP 只在批级别存在，
// 无法从逐条结果还原，必须在此持久化。
type TaskMetrics struct {
	// 检索指标
	Precision *float64 `json:"precision,omitempty"`
	Recall    *float64 `json:"recall,omitempty"`
	NDCG      *float64 `json:"ndcg,omitempty"`
	MRR       *float64 `json:"mrr,omitempty"`
	MAP       *float64 `json:"map,omitempty"`

	// 生成指标
	ROUGE1 *float64 `json:"rouge_1,omitempty"`
	ROUGE2 *float64 `json:"rouge_2,omitempty"`
	ROUGEL *float64 `json:"rouge_l,omitempty"`
	BLEU1  *float64 `json:"bleu_1,omitempty"`
	BLEU2  *float64 `json:"bleu_2,omitempty"`
	BLEU4  *float64 `json:"bleu_4,omitempty"`

	// LLM Judge
	LLMJudgeScore *float64 `json:"llm_judge_score,omitempty"`

	// 语义相似度
	SemanticSimilarity *float64 `json:"semantic_similarity,omitempty"`

	// RAG 专属指标（仅 RAG 类型有意义）
	Faithfulness     *float64 `json:"faithfulness,omitempty"`
	ContextRelevance *float64 `json:"context_relevance,omitempty"`
	NoiseRatio       *float64 `json:"noise_ratio,omitempty"`

	// 动态聚合指标载体:注册表驱动的 name->value(与上面固定字段并存以兼容)
	Scores map[string]float64 `json:"scores,omitempty"`
}

// EvaluationTask 评测任务实体
type EvaluationTask struct {
	ID           string          `json:"id"`
	TenantID     int64           `json:"tenant_id"`
	UserID       int64           `json:"user_id"`
	DatasetID    string          `json:"dataset_id"`
	Type         EvaluationType  `json:"type"`
	KnowledgeBaseID string          `json:"knowledge_base_id,omitempty"`
	AgentID      string          `json:"agent_id,omitempty"`
	ModelID      string          `json:"model_id,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"`
	Status       TaskStatus      `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	TotalCount   int             `json:"total_count"`
	SuccessCount int             `json:"success_count"`
	FailureCount int             `json:"failure_count"`
	Metrics      *TaskMetrics    `json:"metrics,omitempty"` // 任务级聚合指标（完成后写入）
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
}

// NewEvaluationTask 创建新的评测任务
func NewEvaluationTask(id string, tenantID, userID int64, datasetID string, evalType EvaluationType, totalCount int) *EvaluationTask {
	now := time.Now()
	return &EvaluationTask{
		ID:           id,
		TenantID:     tenantID,
		UserID:       userID,
		DatasetID:    datasetID,
		Type:         evalType,
		Status:       TaskStatusPending,
		TotalCount:   totalCount,
		SuccessCount: 0,
		FailureCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// SetStatus 更新任务状态
func (t *EvaluationTask) SetStatus(status TaskStatus) {
	t.Status = status
	t.UpdatedAt = time.Now()
}

// SetError 设置错误信息并将状态设为失败
func (t *EvaluationTask) SetError(msg string) {
	t.Status = TaskStatusFailed
	t.ErrorMessage = msg
	t.UpdatedAt = time.Now()
}

// IncrementProgress 增加成功计数
func (t *EvaluationTask) IncrementProgress() {
	t.SuccessCount++
	t.UpdatedAt = time.Now()
}

// IncrementFailure 增加失败计数
func (t *EvaluationTask) IncrementFailure() {
	t.FailureCount++
	t.UpdatedAt = time.Now()
}

// IsCompleted 检查任务是否完成
func (t *EvaluationTask) IsCompleted() bool {
	return t.Status == TaskStatusCompleted || t.Status == TaskStatusFailed
}

// ========================================
// EvaluationResult 实体
// ========================================

// EvaluationResult 评测结果实体
type EvaluationResult struct {
	ID                 int64     `json:"id"`
	TaskID             string    `json:"task_id"`
	Question           string    `json:"question"`
	ReferenceAnswer    string    `json:"reference_answer"`
	GeneratedAnswer    string    `json:"generated_answer"`
	RetrievedPIDs      []string  `json:"retrieved_pids,omitempty"`
	RelevantPIDs       []string  `json:"relevant_pids,omitempty"`
	Success            bool      `json:"success"`
	Error              string    `json:"error,omitempty"`

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

	// 动态指标载体:注册表驱动的 name->value(与上面固定字段并存以兼容)
	Scores map[string]float64 `json:"scores,omitempty"`

	// RequestID 本条 QA 运行的子 request_id（Agent 类型才有），前端据此深链到 trace 瀑布图。
	RequestID string `json:"request_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// NewEvaluationResult 创建新的评测结果
func NewEvaluationResult(taskID string, index int) *EvaluationResult {
	return &EvaluationResult{
		TaskID:    taskID,
		Success:   false,
		CreatedAt: time.Now(),
	}
}

// ========================================
// Dataset 数据集实体
// ========================================

// Dataset 数据集实体
type Dataset struct {
	ID             int64     `json:"id"`
	DatasetID      string    `json:"dataset_id"`
	TenantID       int64     `json:"tenant_id"`
	UserID         int64     `json:"user_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Type           DatasetType `json:"type"`
	EvaluationType EvaluationType `json:"evaluation_type"`
	QACount        int       `json:"qa_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`

	// 关联的样本数据（非持久化字段）
	QAPairs []*QAPair `json:"qa_pairs,omitempty"`
}

// DatasetType 数据集类型
type DatasetType string

const (
	// DatasetTypeFile 文件系统数据集
	DatasetTypeFile DatasetType = "file"
	// DatasetTypeDatabase 数据库数据集
	DatasetTypeDatabase DatasetType = "database"
)

// NewDataset 创建新数据集
func NewDataset(datasetID string, tenantID, userID int64, name string, evalType EvaluationType) *Dataset {
	now := time.Now()
	return &Dataset{
		DatasetID:      datasetID,
		TenantID:       tenantID,
		UserID:         userID,
		Name:           name,
		Type:           DatasetTypeDatabase,
		EvaluationType: evalType,
		QACount:        0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// UpdateName 更新数据集名称
func (d *Dataset) UpdateName(name string) {
	d.Name = name
	d.UpdatedAt = time.Now()
}

// UpdateDescription 更新数据集描述
func (d *Dataset) UpdateDescription(description string) {
	d.Description = description
	d.UpdatedAt = time.Now()
}

// IncrementQACount 增加样本计数
func (d *Dataset) IncrementQACount(count int) {
	d.QACount += count
	d.UpdatedAt = time.Now()
}

// DecrementQACount 减少样本计数
func (d *Dataset) DecrementQACount(count int) {
	if d.QACount >= count {
		d.QACount -= count
	}
	d.UpdatedAt = time.Now()
}

// IsDeleted 检查是否已删除
func (d *Dataset) IsDeleted() bool {
	return d.DeletedAt != nil
}

// MarkDeleted 标记为已删除
func (d *Dataset) MarkDeleted() {
	now := time.Now()
	d.DeletedAt = &now
	d.UpdatedAt = now
}

// ========================================
// DatasetRecord 数据集样本记录实体
// ========================================

// DatasetRecord 数据集样本记录实体
type DatasetRecord struct {
	ID              int64     `json:"id"`
	DatasetID       string    `json:"dataset_id"`
	TenantID        int64     `json:"tenant_id"`
	Question        string    `json:"question"`
	ReferenceAnswer string    `json:"reference_answer"`
	RelevantPIDs    []string  `json:"relevant_pids,omitempty"`
	Context         string    `json:"context,omitempty"`
	// Agent 评测期望标注（仅 Agent 样本使用，QA/RAG 留空）
	ExpectedTools []string  `json:"expected_tools,omitempty"`
	ExpectedSteps []string  `json:"expected_steps,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// NewDatasetRecord 创建新的样本记录
func NewDatasetRecord(datasetID string, tenantID int64, question, referenceAnswer string) *DatasetRecord {
	return &DatasetRecord{
		DatasetID:       datasetID,
		TenantID:        tenantID,
		Question:        question,
		ReferenceAnswer: referenceAnswer,
		CreatedAt:       time.Now(),
	}
}

// SetRelevantPIDs 设置相关文档ID
func (r *DatasetRecord) SetRelevantPIDs(pids []string) {
	r.RelevantPIDs = pids
}

// SetContext 设置上下文
func (r *DatasetRecord) SetContext(context string) {
	r.Context = context
}

// SetMetrics 设置检索指标
func (r *EvaluationResult) SetRetrievalMetrics(precision, recall, ndcg, rr float64) {
	r.Precision = &precision
	r.Recall = &recall
	r.NDCG = &ndcg
	r.RR = &rr
}

// SetGenerationMetrics 设置生成指标
func (r *EvaluationResult) SetGenerationMetrics(rouge1, rouge2, rougeL, bleu1, bleu2, bleu4 float64) {
	r.ROUGE1 = &rouge1
	r.ROUGE2 = &rouge2
	r.ROUGEL = &rougeL
	r.BLEU1 = &bleu1
	r.BLEU2 = &bleu2
	r.BLEU4 = &bleu4
}

// SetLLMJudge 设置 LLM 裁判结果
func (r *EvaluationResult) SetLLMJudge(score float64, reasoning string) {
	r.LLMScore = &score
	r.LLMReasoning = reasoning
}

// MarkSuccess 标记为成功
func (r *EvaluationResult) MarkSuccess() {
	r.Success = true
}

// MarkError 标记错误
func (r *EvaluationResult) MarkError(err string) {
	r.Success = false
	r.Error = err
}
