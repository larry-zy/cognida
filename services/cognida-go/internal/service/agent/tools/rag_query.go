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

// RAGQueryRequest RAG 查询请求
type RAGQueryRequest struct {
	// Query 查询内容
	Query string `json:"query" jsonschema:"required,description=用户的问题或查询内容"`

	// TopK 返回结果数量，默认5
	TopK int `json:"top_k" jsonschema:"description=返回结果数量，默认5，范围1-20"`

	// RetrievalMode 检索模式：vector(向量)、bm25(关键词)、hybrid(混合)
	RetrievalMode string `json:"retrieval_mode" jsonschema:"description=检索模式：vector/bm25/hybrid，默认hybrid"`

	// MinScore 最小相似度阈值，默认0.5（与系统检索默认一致）。
	// 用指针区分「未传」与「显式 0」：nil=未传→回落 0.5；显式 0=主动关闭分数过滤，不再被静默改成 0.5。
	MinScore *float64 `json:"min_score" jsonschema:"description=最小相似度阈值（余弦），范围0-1，默认0.5；显式传0关闭分数过滤"`

	// EnableRerank 是否启用重排序，默认false
	EnableRerank bool `json:"enable_rerank" jsonschema:"description=是否启用重排序，默认false"`
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
// 使用基类 TypedBaseTool 实现类型安全；svc RAG 服务（可为 nil）经参数注入。
func NewRAGQueryTool(svc RAGQueryService) *TypedBaseTool[RAGQueryRequest, RAGQueryResult] {
	handler := func(ctx context.Context, req *RAGQueryRequest) (*RAGQueryResult, error) {
		return ragQuery(ctx, req, svc)
	}
	return NewTypedBaseTool("rag_query",
		`使用 RAG（检索增强生成）技术从知识库文档中查询信息。

这是文档内容查询的主要工具，能够：
1. 从用户上传的文档中检索相关内容
2. 使用向量相似度、关键词匹配等检索方式
3. 基于检索结果生成准确的答案
4. 标注信息来源，便于验证

【检索模式】
- vector: 向量检索，基于语义相似度，适合概念性查询
- bm25: 关键词检索，基于精确匹配，适合事实性查询
- hybrid: 混合检索（推荐），结合向量和关键词的优势

适用场景：
- 概念解释、操作指南、文档摘要、技术对比、上下文查询

检索范围（哪些知识库）由用户在会话入口选定，或在结合/智能模式下由你经 kb_route 聚焦；
系统始终在允许范围内强制，无需也无法在本工具参数中指定 kb_id。

参数说明：
- query: 查询内容（必需）
- top_k: 返回结果数量（默认5，范围1-20）
- retrieval_mode: 检索模式（默认hybrid）
- min_score: 最小相似度阈值（默认0.5）
- enable_rerank: 是否启用重排序（默认false）`,
		handler,
	)
}

// ragQuery 执行 RAG 查询
func ragQuery(ctx context.Context, req *RAGQueryRequest, ragService RAGQueryService) (*RAGQueryResult, error) {
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
	// 缺省（nil）回落系统默认 0.5；显式传入的值（含 0，即关闭分数过滤）一律尊重，不再覆盖。
	if req.MinScore == nil {
		def := 0.5
		req.MinScore = &def
	}
	if req.RetrievalMode == "" {
		req.RetrievalMode = "hybrid"
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
	result.RetrievalMode = req.RetrievalMode

	return result, nil
}
