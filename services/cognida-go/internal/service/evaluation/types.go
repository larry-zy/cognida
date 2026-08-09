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
	// EvaluationTypeQA QA 评测（llm 的历史别名，向后兼容）
	EvaluationTypeQA EvaluationType = "qa"
	// EvaluationTypeLLM 大模型/通用 QA 生成评测
	EvaluationTypeLLM EvaluationType = "llm"
	// EvaluationTypeSQL Text2SQL / SQL 生成评测（静态结构指标 + 执行准确率）
	EvaluationTypeSQL EvaluationType = "sql"
)

// IsValid 检查评测类型是否有效
func (t EvaluationType) IsValid() bool {
	switch t {
	case EvaluationTypeAgent, EvaluationTypeRAG, EvaluationTypeQA, EvaluationTypeLLM, EvaluationTypeSQL:
		return true
	}
	return false
}

// Normalize 将评测类型规范化，qa 归一化为 llm（与 Python 侧 normalize_eval_type 对齐）。
// 供拉取可用指标目录、按类型区分指标时统一使用。
func (t EvaluationType) Normalize() EvaluationType {
	if t == EvaluationTypeQA {
		return EvaluationTypeLLM
	}
	return t
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

	// Agent 评测期望标注（仅 Agent 类型使用，QA/RAG 留空）
	ExpectedTools []string `json:"expected_tools,omitempty"` // 期望调用的工具名（tool_selection/tool_order）
	ExpectedSteps []string `json:"expected_steps,omitempty"` // 期望步骤序列（trajectory_match/step_efficiency）

	// SQL 评测金标准 SQL（仅 SQL 类型使用，为空回退 ReferenceAnswer）
	GoldSQL string `json:"gold_sql,omitempty"`
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
	RetrievedChunks  []string `json:"retrieved_chunks,omitempty"` // 检索到的分块正文（用于生成与忠实度类指标）
	RetrievedPIDs    []string `json:"retrieved_pids,omitempty"`   // 检索到的分块ID（用于检索指标：precision/recall/ndcg/mrr）
	RelevantPIDs     []string `json:"relevant_pids,omitempty"`
	Success          bool     `json:"success"`
	Error            string   `json:"error,omitempty"`

	// Agent 评测轨迹与期望标注（仅 Agent 类型填充）
	ToolsUsed     []string `json:"tools_used,omitempty"`     // 实际调用的工具名（按调用顺序）
	Trajectory    []string `json:"trajectory,omitempty"`     // 实际步骤序列
	TotalSteps    int      `json:"total_steps,omitempty"`    // 步骤总数
	ExpectedTools []string `json:"expected_tools,omitempty"` // 期望调用的工具名
	ExpectedSteps []string `json:"expected_steps,omitempty"` // 期望步骤序列

	// Agent 运行时基础指标（由 Go 执行器直接采集，非 Python 计算）
	LatencyMs  int64 `json:"latency_ms,omitempty"`  // 单轮对话墙钟耗时（毫秒）
	TokensUsed int   `json:"tokens_used,omitempty"` // 本轮消耗总 token 数
	LLMCalls   int   `json:"llm_calls,omitempty"`   // 本轮 LLM 调用次数（ReAct 迭代数）

	// RequestID 本条 QA 运行的子 request_id，供前端深链到 trace 瀑布图。
	RequestID string `json:"request_id,omitempty"`

	// SQL 评测运行时字段（仅 SQL 类型填充，透传给 Python 计算结构/执行准确率；瞬态，不落库）
	GeneratedSQL  string          `json:"generated_sql,omitempty"`   // Agent 生成的 SQL
	GoldSQL       string          `json:"gold_sql,omitempty"`        // 金标准 SQL
	ResultSet     [][]interface{} `json:"result_set,omitempty"`      // 生成 SQL 只读执行结果集
	GoldResultSet [][]interface{} `json:"gold_result_set,omitempty"` // 金标准 SQL 只读执行结果集

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

	// 动态指标载体:注册表驱动的 name->value(与上面固定字段并存以兼容)
	Scores map[string]float64 `json:"scores,omitempty"`
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

	// RAG 专属指标聚合（忠实度/上下文相关性/噪声比，仅 RAG 类型有意义）
	Faithfulness     *float64 `json:"faithfulness,omitempty"`
	ContextRelevance *float64 `json:"context_relevance,omitempty"`
	NoiseRatio       *float64 `json:"noise_ratio,omitempty"`

	// 动态聚合指标载体:注册表驱动的 name->value(与上面固定字段并存以兼容)
	Scores map[string]float64 `json:"scores,omitempty"`

	// 统计
	TotalCount   int     `json:"total_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
}

// ========================================
// Grader Catalog Types（可用指标目录，注册表元数据的唯一事实来源在 Python 侧）
// ========================================

// GraderMeta 单个可用指标（grader）的元数据，来源于 Python 注册表。
// 字段与 Python `BaseGrader.get_metadata()` 输出对齐。
type GraderMeta struct {
	Name             string   `json:"name"`
	Label            string   `json:"label"`
	Description      string   `json:"description,omitempty"`
	Mode             string   `json:"mode,omitempty"`
	Group            string   `json:"group"`
	EvalTypes        []string `json:"eval_types"`
	RequiresReference bool    `json:"requires_reference"`
	RequiresContexts  bool    `json:"requires_contexts"`
}

// GraderCatalog 按评测类型过滤后的可用指标目录。
type GraderCatalog struct {
	EvalType string        `json:"eval_type"`
	Graders  []*GraderMeta `json:"graders"`
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
	Question          string   `json:"question"`
	ReferenceAnswer   string   `json:"reference_answer"`
	GeneratedAnswer   string   `json:"generated_answer"`
	RetrievedPIDs     []string `json:"retrieved_pids,omitempty"`     // 检索到的分块ID（检索指标）
	RelevantPIDs      []string `json:"relevant_pids,omitempty"`      // 标注的相关分块ID
	RetrievedContexts []string `json:"retrieved_contexts,omitempty"` // 检索到的分块正文（RAG 忠实度类指标）

	// Agent 评测字段：references(expected_*) + outputs(tools_used/trajectory/total_steps)，命中 compute_agent_metrics
	ExpectedTools []string `json:"expected_tools,omitempty"` // 期望调用的工具名（tool_selection/tool_order）
	ExpectedSteps []string `json:"expected_steps,omitempty"` // 期望步骤序列（trajectory_match/step_efficiency）
	ToolsUsed     []string `json:"tools_used,omitempty"`     // 实际调用的工具名（按调用顺序）
	Trajectory    []string `json:"trajectory,omitempty"`     // 实际步骤序列
	TotalSteps    int      `json:"total_steps,omitempty"`    // 步骤总数

	// SQL 评测字段：结构指标用 gold_sql/generated_sql，执行准确率用双方结果集。
	// 结果集字段刻意不加 omitempty：需向 Python 区分「未执行/执行失败」(nil → JSON null →
	// sql_execution_accuracy 剔除出分母) 与「执行成功但零行」([] → JSON [] → 参与比对)。
	// 若加 omitempty，非 nil 空切片也会被省略，未执行样本会被 Python 当成空==空误判为满分（#1）。
	GoldSQL       string          `json:"gold_sql,omitempty"`      // 金标准 SQL（sql_exact_match/sql_component_match）
	GeneratedSQL  string          `json:"generated_sql,omitempty"` // Agent 生成的 SQL
	ResultSet     [][]interface{} `json:"result_set"`              // 生成 SQL 结果集（sql_execution_accuracy）；nil=未执行
	GoldResultSet [][]interface{} `json:"gold_result_set"`         // 金标准 SQL 结果集；nil=未执行
}

// ComputeMetricsResponse Python 指标计算响应
type ComputeMetricsResponse struct {
	Items     []*ComputeItemResult `json:"items"`
	Aggregate map[string]float64   `json:"aggregate"` // 聚合结果本身即动态 scores map(name->value)
	// Unsupported 请求了但注册表中无对应 grader 的指标名（不静默忽略）
	Unsupported []string `json:"unsupported,omitempty"`
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

	// 动态指标载体:注册表驱动的 name->value(与上面固定字段并存以兼容)
	Scores map[string]float64 `json:"scores,omitempty"`
}
