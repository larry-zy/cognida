// Package research provides Deep Research Agent type definitions
package research

import "time"

// ========================================
// Deep Research 类型定义
// ========================================

// SubQuestion 子问题
type SubQuestion struct {
	Question   string   `json:"question"`              // 问题内容
	Priority   int      `json:"priority"`              // 优先级 1=最高
	QueryTerms []string `json:"query_terms,omitempty"` // 查询关键词
	Status     string   `json:"status"`                // pending/in_progress/completed
}

// DecompositionResult 问题拆解结果
type DecompositionResult struct {
	OriginalQuery string        `json:"original_query"`
	SubQuestions  []SubQuestion `json:"sub_questions"`
	Method        string        `json:"method"`     // llm/simple
	Complexity    int           `json:"complexity"` // 1-5
	LatencyMs     int64         `json:"latency_ms"`
	Timestamp     int64         `json:"timestamp"`
}

// ========================================
// 研究执行结果
// ========================================

// ResearchResult 研究结果
type ResearchResult struct {
	SubQuestion  *SubQuestion      `json:"sub_question"`
	RAGResults   []RAGResultItem   `json:"rag_results,omitempty"`
	WebResults   []WebResultItem   `json:"web_results,omitempty"`
	GraphResults []GraphResultItem `json:"graph_results,omitempty"`
	Answer       string            `json:"answer"` // Agent 生成的简要答案
	CompletedAt  time.Time         `json:"completed_at"`
	Error        string            `json:"error,omitempty"`
}

// RAGResultItem RAG 检索结果项
type RAGResultItem struct {
	Content    string                 `json:"content"`
	Score      float64                `json:"score"`
	Source     string                 `json:"source"`
	DocumentID string                 `json:"document_id,omitempty"`
	ChunkIndex int                    `json:"chunk_index,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// WebResultItem 网络搜索结果项
type WebResultItem struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Snippet       string  `json:"snippet"`
	Score         float64 `json:"score"`
	PublishedDate string  `json:"published_date,omitempty"`
	Source        string  `json:"source"`
}

// GraphResultItem 图谱查询结果项
type GraphResultItem struct {
	Entity     string                 `json:"entity"`
	Relation   string                 `json:"relation"`
	Target     string                 `json:"target"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Confidence float64                `json:"confidence"`
}

// ========================================
// 事实提取
// ========================================

// ExtractedFact 提取的事实
type ExtractedFact struct {
	Fact       string   `json:"fact"`                  // 事实陈述
	FactType   string   `json:"fact_type"`             // statistic/date/claim/definition/entity
	Confidence float64  `json:"confidence"`            // 可信度 0-1
	SourceRefs []int    `json:"source_refs,omitempty"` // 来源结果索引
	Keywords   []string `json:"keywords,omitempty"`    // 关键词
}

// FactExtractionResult 事实提取结果
type FactExtractionResult struct {
	Facts      []ExtractedFact `json:"facts"`
	Query      string          `json:"query"`
	TotalFacts int             `json:"total_facts"`
	Method     string          `json:"method"` // llm/fallback
	LatencyMs  int64           `json:"latency_ms"`
}

// ========================================
// 交叉验证
// ========================================

// ValidationResult 验证结果
type ValidationResult struct {
	Fact           *ExtractedFact     `json:"fact"`
	IsVerified     bool               `json:"is_verified"`
	Confidence     float64            `json:"confidence"`
	Consensus      string             `json:"consensus"` // 共识描述
	Contradictions []string           `json:"contradictions,omitempty"`
	Sources        []ValidationSource `json:"sources,omitempty"`
	MissingInfo    []string           `json:"missing_info,omitempty"`
}

// ValidationSource 验证来源
type ValidationSource struct {
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Content   string  `json:"content"`
	Supports  bool    `json:"supports"`  // 是否支持该事实
	Relevance float64 `json:"relevance"` // 相关性 0-1
	Authority float64 `json:"authority"` // 权威性 0-1
}

// ========================================
// 研究报告
// ========================================

// ResearchReport 研究报告
type ResearchReport struct {
	Query            string              `json:"query"`
	ExecutiveSummary string              `json:"executive_summary"`        // 核心答案
	DetailedAnalysis string              `json:"detailed_analysis"`        // 详细分析
	KeyFindings      []string            `json:"key_findings"`             // 关键发现
	Sources          []ReportSource      `json:"sources"`                  // 参考资料
	VerifiedFacts    []*ValidationResult `json:"verified_facts,omitempty"` // 已验证事实
	Metadata         ReportMetadata      `json:"metadata"`
}

// ReportSource 报告来源
type ReportSource struct {
	Type    string `json:"type"` // rag/web/graph
	Title   string `json:"title"`
	URL     string `json:"url,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// ReportMetadata 报告元数据
type ReportMetadata struct {
	SubQuestionCount  int       `json:"sub_question_count"`
	TotalSources      int       `json:"total_sources"`
	ExecutionTimeMs   int64     `json:"execution_time_ms"`
	FactChecked       bool      `json:"fact_checked"`
	VerifiedFactCount int       `json:"verified_fact_count"`
	Timestamp         time.Time `json:"timestamp"`
}

// ========================================
// Agent 配置
// ========================================

// DeepResearchConfig Deep Research Agent 配置
type DeepResearchConfig struct {
	// 基本配置
	Name        string // Agent 名称
	Description string // Agent 描述

	// 拆解配置
	MaxSubQuestions  int // 最大子问题数量
	SimpleQueryLimit int // 简单问题阈值（复杂度<=此值不拆解）

	// 执行配置
	MaxConcurrency int // 最大并发执行数
	MaxToolRounds  int // 每个子问题最大工具调用轮数

	// 事实验证配置
	EnableFactCheck    bool    // 启用事实验证
	MaxFactsToValidate int     // 最大验证事实数
	MinFactConfidence  float64 // 事实提取最小可信度

	// 验证配置
	EnableCrossValidation bool    // 启用交叉验证
	MinValidationSources  int     // 最小验证来源数
	ConsensusThreshold    float64 // 共识阈值
}

// DefaultDeepResearchConfig 默认配置
func DefaultDeepResearchConfig() *DeepResearchConfig {
	return &DeepResearchConfig{
		Name:                  "DeepResearch",
		Description:           "深度研究助手 - Plan → Execute → Synthesize",
		MaxSubQuestions:       5,
		SimpleQueryLimit:      2,
		MaxConcurrency:        3,
		MaxToolRounds:         5,
		EnableFactCheck:       true,
		MaxFactsToValidate:    5,
		MinFactConfidence:     0.7,
		EnableCrossValidation: true,
		MinValidationSources:  3,
		ConsensusThreshold:    0.6,
	}
}

// ========================================
// 进度事件
// ========================================

// ProgressEvent 进度事件
type ProgressEvent struct {
	Stage    string      `json:"stage"` // decompose/execute/synthesize
	Message  string      `json:"message"`
	Progress float64     `json:"progress"` // 0-1
	Data     interface{} `json:"data,omitempty"`
}

// ProgressCallback 进度回调函数
type ProgressCallback func(event *ProgressEvent)

// ========================================
// 搜索结果
// ========================================

// SearchResult 通用搜索结果
type SearchResult struct {
	Content string  `json:"content"`         // 结果内容
	Source  string  `json:"source"`          // 结果来源
	Score   float64 `json:"score"`           // 相关性评分
	URL     string  `json:"url,omitempty"`   // 链接地址
	Title   string  `json:"title,omitempty"` // 结果标题
}
