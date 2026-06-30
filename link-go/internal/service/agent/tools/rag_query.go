// Package tools 提供 RAG 查询工具
package tools

import (
	"context"
	"fmt"
	"time"
)

// ========================================
// RAG 查询工具
// ========================================

// 全局 RAG 服务实例
var ragService RAGQueryService

// InitRAGQueryTool 初始化 RAG 查询工具
func InitRAGQueryTool(service RAGQueryService) {
	ragService = service
}

// SetRAGService 设置 RAG 服务（用于测试）
func SetRAGService(service RAGQueryService) {
	ragService = service
}

// RAGQueryRequest RAG 查询请求
type RAGQueryRequest struct {
	// Query 查询内容
	Query string `json:"query" jsonschema:"required,description=用户的问题或查询内容"`

	// KnowledgeBaseID 知识库ID，空字符串表示查询所有启用的知识库
	KnowledgeBaseID string `json:"kb_id" jsonschema:"description=知识库ID，空字符串或不传表示查询所有启用的知识库"`

	// TopK 返回结果数量，默认5
	TopK int `json:"top_k" jsonschema:"description=返回结果数量，默认5，范围1-20"`

	// RetrievalMode 检索模式：vector(向量)、bm25(关键词)、hybrid(混合)
	RetrievalMode string `json:"retrieval_mode" jsonschema:"description=检索模式：vector/bm25/hybrid，默认hybrid"`

	// MinScore 最小相似度阈值，默认0.7
	MinScore float64 `json:"min_score" jsonschema:"description=最小相似度阈值，范围0-1，默认0.7"`

	// EnableRerank 是否启用重排序，默认false
	EnableRerank bool `json:"enable_rerank" jsonschema:"description=是否启用重排序，默认false"`

	// EnableHyDE 是否启用 HyDE（假设文档嵌入），默认false
	EnableHyDE bool `json:"enable_hyde" jsonschema:"description=是否启用HyDE假设文档检索，适合复杂查询，默认false"`

	// HyDECount HyDE 生成的假设文档数量，默认1
	HyDECount int `json:"hyde_count" jsonschema:"description=HyDE生成假设文档数量，默认1，范围1-3"`

	// EnableQueryRewrite 是否启用查询重写，默认false
	EnableQueryRewrite bool `json:"enable_query_rewrite" jsonschema:"description=是否启用查询重写，解决指代消解，默认false"`

	// EnableQueryExpansion 是否启用查询扩展，默认false
	EnableQueryExpansion bool `json:"enable_query_expansion" jsonschema:"description=是否启用查询扩展，生成多个查询变体，默认false"`

	// ExpansionCount 查询扩展的变体数量，默认3
	ExpansionCount int `json:"expansion_count" jsonschema:"description=查询扩展变体数量，默认3，范围2-5"`

	// EnableMultiHop 是否启用多跳检索，默认false
	EnableMultiHop bool `json:"enable_multi_hop" jsonschema:"description=是否启用多跳检索，适合复杂推理问题，默认false"`

	// MaxHops 多跳检索的最大跳数，默认3
	MaxHops int `json:"max_hops" jsonschema:"description=多跳检索最大跳数，默认3，范围2-5"`

	// Domain 知识域，用于优化查询和假设文档生成
	Domain string `json:"domain" jsonschema:"description=知识域，用于优化检索效果，如：medical/legal/finance"`

	// ConversationHistory 对话历史，用于查询重写的上下文理解
	ConversationHistory []string `json:"conversation_history" jsonschema:"description=对话历史，用于查询重写上下文理解，可选"`
}

// RAGQueryResult RAG 查询结果
type RAGQueryResult struct {
	// Answer 基于检索内容生成的答案
	Answer string `json:"answer"`

	// Chunks 检索到的文档片段
	Chunks []DocumentChunk `json:"chunks"`

	// Count 检索到的片段数量
	Count int `json:"count"`

	// Query 原始查询
	Query string `json:"query"`

	// KnowledgeBaseID 使用的知识库ID
	KnowledgeBaseID string `json:"kb_id"`

	// Latency 查询耗时（毫秒）
	Latency int64 `json:"latency_ms"`

	// RetrievalMode 实际使用的检索模式
	RetrievalMode string `json:"retrieval_mode"`

	// HasAnswer 是否有答案
	HasAnswer bool `json:"has_answer"`

	// OptimizationApplied 应用的优化措施
	OptimizationApplied map[string]bool `json:"optimization_applied,omitempty"`
}

// DocumentChunk 文档片段
type DocumentChunk struct {
	// Content 片段内容
	Content string `json:"content"`

	// Score 相似度分数
	Score float64 `json:"score"`

	// Source 来源文档
	Source string `json:"source"`

	// DocumentID 文档ID
	DocumentID int64 `json:"document_id"`

	// ChunkIndex 片段索引
	ChunkIndex int `json:"chunk_index"`

	// Highlight 高亮内容
	Highlight string `json:"highlight,omitempty"`

	// Metadata 额外元数据
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewRAGQueryTool 创建 RAG 查询工具
// 使用基类 TypedBaseTool 实现类型安全
func NewRAGQueryTool() *TypedBaseTool[RAGQueryRequest, RAGQueryResult] {
	return NewTypedBaseTool("rag_query",
		`使用 RAG（检索增强生成）技术从知识库文档中查询信息。

这是文档内容查询的主要工具，能够：
1. 从用户上传的文档中检索相关内容
2. 使用向量相似度、关键词匹配等检索方式
3. 支持查询优化（HyDE、重写、扩展、多跳检索）
4. 基于检索结果生成准确的答案
5. 标注信息来源，便于验证

【基础检索模式】
- vector: 向量检索，基于语义相似度，适合概念性查询
- bm25: 关键词检索，基于精确匹配，适合事实性查询
- hybrid: 混合检索（推荐），结合向量和关键词的优势

【高级优化功能】
- enable_hyde: 启用 HyDE（假设文档嵌入），适合复杂/模糊查询
- enable_query_rewrite: 启用查询重写，解决指代消解问题
- enable_query_expansion: 启用查询扩展，生成多个查询变体
- enable_multi_hop: 启用多跳检索，适合需要推理的复杂问题

适用场景：
- 概念解释、操作指南、文档摘要、技术对比、上下文查询、复杂推理

参数说明：
- query: 查询内容（必需）
- kb_id: 知识库ID（可选）
- top_k: 返回结果数量（默认5）
- retrieval_mode: 检索模式（默认hybrid）
- enable_hyde: 是否启用 HyDE（默认false）
- enable_query_rewrite: 是否查询重写（默认false）`,
		ragQuery,
	)
}

// ragQuery 执行 RAG 查询
func ragQuery(ctx context.Context, req *RAGQueryRequest) (*RAGQueryResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// 设置默认值
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.TopK > 20 {
		req.TopK = 20
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.7
	}
	if req.RetrievalMode == "" {
		req.RetrievalMode = "hybrid"
	}
	if req.HyDECount <= 0 {
		req.HyDECount = 1
	}
	if req.ExpansionCount <= 0 {
		req.ExpansionCount = 3
	}
	if req.MaxHops <= 0 {
		req.MaxHops = 3
	}

	// 验证检索模式
	validModes := map[string]bool{
		"vector": true,
		"bm25":   true,
		"hybrid": true,
	}
	if !validModes[req.RetrievalMode] {
		return nil, fmt.Errorf("invalid retrieval_mode: %s, must be one of: vector, bm25, hybrid", req.RetrievalMode)
	}

	// 2. 检查 RAG 服务是否已初始化
	if ragService == nil {
		return nil, fmt.Errorf("RAG service not initialized. Please configure RAG service before using rag_query tool")
	}

	// 3. 调用真实的 RAG 服务
	result, err := ragService.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("RAG query failed: %w", err)
	}

	// 4. 更新耗时和元数据
	result.Latency = time.Since(startTime).Milliseconds()
	result.Query = req.Query
	result.KnowledgeBaseID = req.KnowledgeBaseID
	result.RetrievalMode = req.RetrievalMode

	// 5. 记录应用的优化措施
	if result.OptimizationApplied == nil {
		result.OptimizationApplied = make(map[string]bool)
	}
	result.OptimizationApplied["hyde"] = req.EnableHyDE
	result.OptimizationApplied["query_rewrite"] = req.EnableQueryRewrite
	result.OptimizationApplied["query_expansion"] = req.EnableQueryExpansion
	result.OptimizationApplied["multi_hop"] = req.EnableMultiHop

	return result, nil
}

// ========================================
// 工具工厂
// ========================================

// RAGToolFactory RAG 工具工厂
type RAGToolFactory struct {
	service RAGQueryService
}

// NewRAGToolFactory 创建 RAG 工具工厂
func NewRAGToolFactory(service RAGQueryService) *RAGToolFactory {
	return &RAGToolFactory{
		service: service,
	}
}

// CreateTool 创建工具
func (f *RAGToolFactory) CreateTool() *TypedBaseTool[RAGQueryRequest, RAGQueryResult] {
	InitRAGQueryTool(f.service)
	return NewRAGQueryTool()
}
