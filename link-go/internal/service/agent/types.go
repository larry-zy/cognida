// Package agent provides Agent service implementations
package agent

// ========================================
// AgenticRAG 相关 DTO
// ========================================

// AgenticRAGRequest AgenticRAG 请求
type AgenticRAGRequest struct {
	AgentID   string                 `json:"agent_id,omitempty"` // Agent ID，默认为 "default"
	Query     string                 `json:"query" binding:"required"`
	SessionID string                 `json:"session_id,omitempty"`
	Options   *AgentOptions          `json:"options,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// AgentOptions Agent 选项
type AgentOptions struct {
	Stream        bool     `json:"stream"`
	MaxIterations int      `json:"max_iterations"`
	Temperature   float64  `json:"temperature"`
	Tools         []string `json:"tools"`
}

// AgenticRAGResponse AgenticRAG 响应
type AgenticRAGResponse struct {
	Answer     string                 `json:"answer"`
	Success    bool                   `json:"success"`
	Error      string                 `json:"error,omitempty"`
	ToolCalls  []*ToolCallRecordDTO    `json:"tool_calls,omitempty"`
	Sources    []string               `json:"sources,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Conclusion *ConclusionDTO         `json:"conclusion,omitempty"` // 数据结论
}

// ToolCallRecordDTO 工具调用记录DTO
type ToolCallRecordDTO struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Input    string                 `json:"input"`
	Output   string                 `json:"output,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Duration int64                  `json:"duration"`
}

// QueryAnalysisDTO 查询分析DTO
type QueryAnalysisDTO struct {
	Type           string   `json:"type"`
	NeedRetrieval  bool     `json:"need_retrieval"`
	SuggestedTools []string `json:"suggested_tools"`
	NeedWebSearch  bool     `json:"need_web_search"`
	RetrievalMode  string   `json:"retrieval_mode"`
	Complexity     int      `json:"complexity"`
}

// ResultEvaluationDTO 结果评估DTO
type ResultEvaluationDTO struct {
	Confidence    float64 `json:"confidence"`
	IsSufficient  bool    `json:"is_sufficient"`
	MissingInfo   []string `json:"missing_info,omitempty"`
	Suggestions   []string `json:"suggestions,omitempty"`
	ToolCount     int     `json:"tool_count"`
	HasWebSearch  bool    `json:"has_web_search"`
}

// ChatChunkDTO 流式聊天分块DTO
type ChatChunkDTO struct {
	Content  string                 `json:"content"`
	Done     bool                   `json:"done"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ========================================
// Deep Research 相关 DTO
// ========================================

// DeepResearchRequest 深度研究请求
type DeepResearchRequest struct {
	Query    string                `json:"query" binding:"required"`
	SessionID string                `json:"session_id,omitempty"`
	Options   *DeepResearchOptions  `json:"options,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// DeepResearchOptions 深度研究选项
type DeepResearchOptions struct {
	Stream                bool `json:"stream"`
	MaxSubQuestions       int  `json:"max_sub_questions"`
	MaxConcurrency        int  `json:"max_concurrency"`
	EnableFactCheck       bool `json:"enable_fact_check"`
	MaxFactsToValidate    int  `json:"max_facts_to_validate"`
	EnableCrossValidation bool `json:"enable_cross_validation"`
}

// DeepResearchResponse 深度研究响应
type DeepResearchResponse struct {
	Query            string                  `json:"query"`
	ExecutiveSummary string                  `json:"executive_summary"`
	DetailedAnalysis string                  `json:"detailed_analysis"`
	KeyFindings      []string                `json:"key_findings"`
	Sources          []ReportSourceDTO       `json:"sources"`
	VerifiedFacts    []*ValidationResultDTO `json:"verified_facts,omitempty"`
	Metadata         ReportMetadataDTO        `json:"metadata"`
	Success          bool                    `json:"success"`
	Error            string                  `json:"error,omitempty"`
}

// ReportSourceDTO 报告来源DTO
type ReportSourceDTO struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Excerpt string `json:"excerpt"`
}

// ValidationResultDTO 验证结果DTO
type ValidationResultDTO struct {
	Fact           *ExtractedFactDTO    `json:"fact,omitempty"`
	IsVerified     bool                  `json:"is_verified"`
	Confidence     float64               `json:"confidence"`
	Consensus      string                `json:"consensus,omitempty"`
	Contradictions []string              `json:"contradictions,omitempty"`
	Sources        []ValidationSourceDTO `json:"sources,omitempty"`
	MissingInfo    []string              `json:"missing_info,omitempty"`
}

// ExtractedFactDTO 提取的事实DTO
type ExtractedFactDTO struct {
	Fact       string   `json:"fact"`
	FactType   string   `json:"fact_type"`
	Confidence float64  `json:"confidence"`
	SourceRefs []int    `json:"source_refs,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
}

// ValidationSourceDTO 验证来源DTO
type ValidationSourceDTO struct {
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Content   string  `json:"content"`
	Supports  bool    `json:"supports"`
	Relevance float64 `json:"relevance"`
	Authority float64 `json:"authority"`
}

// ReportMetadataDTO 报告元数据DTO
type ReportMetadataDTO struct {
	SubQuestionCount   int    `json:"sub_question_count"`
	TotalSources       int    `json:"total_sources"`
	ExecutionTimeMs    int64  `json:"execution_time_ms"`
	FactChecked        bool   `json:"fact_checked"`
	VerifiedFactCount   int    `json:"verified_fact_count"`
	Timestamp           int64  `json:"timestamp"`
}

// ========================================
// 进度推送相关 DTO
// ========================================

// ResearchProgressDTO 研究进度DTO
type ResearchProgressDTO struct {
	Stage        string          `json:"stage"`
	Progress     float64         `json:"progress"`
	Detail       string          `json:"detail"`
	SubStage     string          `json:"sub_stage,omitempty"`
	TotalSteps   int             `json:"total_steps"`
	CurrentStep  int             `json:"current_step"`
	ElapsedTime  int64           `json:"elapsed_ms"`
	Estimated    int64           `json:"estimated_ms,omitempty"`
	ResultsCount int             `json:"results_count"`
	SubQuestions []string        `json:"sub_questions,omitempty"`
	Sources      []SourceInfoDTO `json:"sources,omitempty"`
	Error        string          `json:"error,omitempty"`
	Timestamp    int64           `json:"timestamp"`
}

// SourceInfoDTO 来源信息DTO
type SourceInfoDTO struct {
	Query string `json:"query"`
	URL   string `json:"url,omitempty"`
	Title string `json:"title,omitempty"`
	Count int    `json:"count"`
}

// ========================================
// 结论相关 DTO
// ========================================

// ConclusionDTO 数据结论DTO
type ConclusionDTO struct {
	Summary     string   `json:"summary"`
	KeyPoints   []string `json:"key_points"`
	DataSources []string `json:"data_sources"`
	Confidence  float64  `json:"confidence"`
}

// ========================================
// Clarification 相关 DTO
// ========================================

// ClarificationQuestionDTO 澄清问题DTO
type ClarificationQuestionDTO struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	Multiple bool     `json:"multiple_select"`
	Required bool     `json:"required"`
}

// ClarificationResponseDTO 澄清响应DTO
type ClarificationResponseDTO struct {
	NeedsClarification bool                       `json:"needs_clarification"`
	Questions          []*ClarificationQuestionDTO `json:"questions"`
	OriginalQuery      string                      `json:"original_query"`
	CurrentAnswers     map[string]string           `json:"current_answers,omitempty"`
	Round              int                         `json:"round"`
	MaxRounds          int                         `json:"max_rounds"`
}

// ========================================
// 配置相关 DTO
// ========================================

// AgentConfigDTO Agent配置DTO
type AgentConfigDTO struct {
	Type                  string                 `json:"type"`
	Name                  string                 `json:"name"`
	Description           string                 `json:"description"`
	MaxIterations         int                    `json:"max_iterations"`
	EnableSmartRetrieval  bool                   `json:"enable_smart_retrieval"`
	DefaultTopK           int                    `json:"default_top_k"`
	EnableQueryRewrite    bool                   `json:"enable_query_rewrite"`
	EnableResultEval      bool                   `json:"enable_result_eval"`
	MinConfidence         float64                `json:"min_confidence"`
	Extra                 map[string]interface{} `json:"extra,omitempty"`
}

// SearchConfigDTO 搜索配置DTO
type SearchConfigDTO struct {
	Enabled    bool   `json:"enabled"`
	Engine     string `json:"engine"`
	MaxResults int    `json:"max_results"`
}

// HookConfigDTO Hook配置DTO
type HookConfigDTO struct {
	EnableConclusion     bool     `json:"enable_conclusion"`
	DataTools           []string `json:"data_tools"`
	EnableClarification bool     `json:"enable_clarification"`
	BusinessContext     string   `json:"business_context"`
	MaxRounds           int      `json:"max_rounds"`
	EnableMemoryWrite   bool     `json:"enable_memory_write"`
	EnableAutoCompress  bool     `json:"enable_auto_compress"`
	EnableLongTermMemory bool    `json:"enable_long_term_memory"`
}

// MemoryConfigDTO 记忆配置DTO
type MemoryConfigDTO struct {
	StorageBackend     string                 `json:"storage_backend"`
	ContextConfig      map[string]interface{} `json:"context_config"`
	CompressionConfig  map[string]interface{} `json:"compression_config"`
}

// ========================================
// Progress 相关 DTO
// ========================================

// SessionProgress 会话进度（用于 Service 层）
type SessionProgress struct {
	SessionID   string  `json:"session_id"`
	Stage       string  `json:"stage"`
	Progress    float64 `json:"progress"`
	Detail      string  `json:"detail"`
	IsCompleted bool    `json:"is_completed"`
	Error       string  `json:"error,omitempty"`
}

// SimilarityRequest 相似度计算请求
type SimilarityRequest struct {
	Text1  string `json:"text1"`
	Text2  string `json:"text2"`
	Method string `json:"method,omitempty"` // jaccard, cosine, etc.
}

// SimilarityResponse 相似度计算响应
type SimilarityResponse struct {
	Similarity float64 `json:"similarity"`
	Method     string  `json:"method"`
}
