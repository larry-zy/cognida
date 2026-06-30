// Package rag 提供 RAG 领域实体定义
package rag

import (
	"time"
)

// ========================================
// Pipeline 配置实体
// ========================================

// PipelineConfig RAG 处理管道配置
type PipelineConfig struct {
	// 检索配置
	TopK                int     // 返回结果数量
	SimilarityThreshold float64 // 相似度阈值
	RetrievalMode       string  // 检索模式: vector/bm25/hybrid/graph

	// 重排配置
	EnableRerank   bool    // 是否启用重排
	RerankStrategy string  // 重排策略: rrf/weighted/weighted_rrf/model
	Alpha          float64 // 混合检索权重 (向量: 1-Alpha, BM25: Alpha)

	// 图谱配置
	GraphEnabled bool   // 是否启用图谱检索
	GraphDepth   int    // 图谱检索深度
	GraphTopK    int    // 图谱返回数量

	// 查询增强配置
	EnableQueryRewrite bool   // 是否启用查询重写
	EnableQuerySplit   bool   // 是否启用查询拆分
}

// DefaultPipelineConfig 默认管道配置
func DefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		TopK:                10,
		SimilarityThreshold: 0.5,
		RetrievalMode:       "hybrid",
		EnableRerank:        true,
		RerankStrategy:      "rrf",
		Alpha:               0.5,
		GraphEnabled:        false,
		GraphDepth:          2,
		GraphTopK:           5,
		EnableQueryRewrite:  false,
		EnableQuerySplit:    false,
	}
}

// ========================================
// 检索选项实体
// ========================================

// RetrieveOptions 检索选项
type RetrieveOptions struct {
	TopK                int     // 返回结果数量
	SimilarityThreshold float64 // 相似度阈值
	RerankEnabled       bool    // 是否启用重排
	GraphEnabled        bool    // 是否启用图谱
	Alpha               float64 // 混合检索权重
	RetrievalMode       string  // 检索模式
}

// DefaultRetrieveOptions 默认检索选项
func DefaultRetrieveOptions() *RetrieveOptions {
	return &RetrieveOptions{
		TopK:                10,
		SimilarityThreshold: 0.5,
		RerankEnabled:       true,
		GraphEnabled:        false,
		Alpha:               0.5,
		RetrievalMode:       "hybrid",
	}
}

// ========================================
// 检索结果实体
// ========================================

// Document 文档
type Document struct {
	ChunkID      string  // 文档块ID
	KnowledgeID  string  // 知识条目ID
	KnowledgeBaseID         string  // 知识库ID
	Content      string  // 文档内容
	Score        float32 // 相似度分数
	MatchType    string  // 匹配类型: vector/bm25/graph/hybrid
	ChunkIndex   int     // 文档块索引
	Metadata     map[string]interface{} // 元数据
}

// RetrieveResponse 检索响应
type RetrieveResponse struct {
	Results        []*Document  // 检索结果
	Query          string       // 查询内容
	ProcessedQuery string       // 处理后的查询（重写后）
	TotalCount     int          // 总结果数
	HasMore        bool         // 是否有更多结果
	Latency        int64        // 延迟（毫秒）
	SearchTrace    *SearchTrace // 检索追踪信息
	Answer         string       // 生成的答案（如果有）
}

// SearchTrace 检索追踪信息
type SearchTrace struct {
	VectorResultCount int              // 向量检索结果数
	BM25ResultCount   int              // BM25检索结果数
	GraphResultCount  int              // 图谱检索结果数
	RerankedCount     int              // 重排后结果数
	VectorLatency     int64            // 向量检索延迟
	BM25Latency       int64            // BM25检索延迟
	GraphLatency      int64            // 图谱检索延迟
	RerankLatency     int64            // 重排延迟
	RetrievalDetails  []RetrievalStep  // 检索步骤详情

	// HyDE 相关
	HyDEUsed        bool   // 是否使用了 HyDE
	HypotheticalDoc string // 假设文档内容

	// Multi-Hop 相关
	MultiHopUsed   bool   // 是否使用了多跳检索
	HopCount       int    // 跳数
	ReasoningChain string // 推理链
}

// RetrievalStep 检索步骤详情
type RetrievalStep struct {
	StepType    string        // 步骤类型
	Description string        // 描述
	Latency     int64         // 延迟
	ResultCount int           // 结果数
	Details     map[string]interface{} // 详细信息
}

// ========================================
// RAG 上下文实体
// ========================================

// RAGContext RAG 上下文
type RAGContext struct {
	Query              string       // 原始查询
	RewrittenQuery     string       // 重写后的查询
	SubQueries         []string     // 拆分的子查询
	Documents          []*Document  // 检索到的文档
	RetrievalMetadata  *SearchTrace // 检索元数据
	ConversationHistory []ConversationMessage // 对话历史
	ProcessingTime     int64        // 处理时间（毫秒）
	Timestamp          time.Time    // 时间戳
}

// ConversationMessage 对话消息
type ConversationMessage struct {
	Role    string // 角色: user/assistant/system
	Content string // 内容
	Time    time.Time // 时间
}

// ========================================
// 查询增强结果实体
// ========================================

// StrengthenedQuery 增强后的查询
type StrengthenedQuery struct {
	OriginalQuery   string   // 原始查询
	RewrittenQuery  string   // 重写后的查询
	SubQueries      []string // 拆分的子查询
	RewriteApplied  bool     // 是否应用了重写
	SplitApplied    bool     // 是否应用了拆分
	ProcessingTime  int64    // 处理耗时（毫秒）
}

// GetQueriesForRetrieve 获取用于检索的查询列表
func (sq *StrengthenedQuery) GetQueriesForRetrieve() []string {
	var queries []string

	// 如果有拆分的子查询，使用子查询
	if len(sq.SubQueries) > 0 {
		queries = append(queries, sq.SubQueries...)
	}
	// 如果有重写后的查询，添加重写后的查询
	if sq.RewrittenQuery != "" {
		queries = append(queries, sq.RewrittenQuery)
	}
	// 始终包含原查询
	queries = append(queries, sq.OriginalQuery)

	return deduplicateQueries(queries)
}

// ========================================
// 图谱实体
// ========================================

// GraphNode 图谱节点
type GraphNode struct {
	ID         string            // 节点ID
	Name       string            // 节点名称
	EntityType string            // 实体类型
	Attributes []string          // 属性列表
	Chunks     []string          // 关联的文档块ID列表
	Properties map[string]string // 属性键值对
}

// GraphRelation 图谱关系
type GraphRelation struct {
	ID          string            // 关系ID
	Source      string            // 源节点名称
	Target      string            // 目标节点名称
	Type        string            // 关系类型
	Strength    float64           // 关系强度 (1-10)
	Weight      float64           // 关系权重
	ChunkIDs    []string          // 关联的文档块ID列表
	Properties  map[string]string // 属性键值对
	CombinedDegree int            // 组合度数
}

// GraphData 图谱数据
type GraphData struct {
	Node     []*GraphNode     // 节点列表
	Relation []*GraphRelation // 关系列表
}

// ========================================
// 辅助函数
// ========================================

// deduplicateQueries 去重查询列表
func deduplicateQueries(queries []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, q := range queries {
		q := trimSpace(q)
		if q != "" && !seen[q] {
			seen[q] = true
			result = append(result, q)
		}
	}

	return result
}

// trimSpace 去除字符串首尾空格
func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}

	if start >= end {
		return ""
	}

	return s[start:end]
}
