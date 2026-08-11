// Package retriever 提供 Milvus 检索器的适配器实现
package retriever

import (
	"context"
	"fmt"

	domainrag "cognida/internal/model/rag"
)

// MilvusAdapter 将 VectorRetriever 适配为 domainrag.MilvusRetriever 接口
type MilvusAdapter struct {
	retriever *VectorRetriever
}

// NewMilvusAdapter 创建 Milvus 适配器
func NewMilvusAdapter(retriever *VectorRetriever) *MilvusAdapter {
	return &MilvusAdapter{
		retriever: retriever,
	}
}

// Search 向量搜索 - 实现 MilvusRetriever 接口
func (a *MilvusAdapter) Search(ctx context.Context, collectionName string, vector []float32, topK int, filter map[string]string) ([]*domainrag.Document, error) {
	// 从 filter 中提取 kb_id，用于在统一 collection 中过滤
	// 所有知识库现在使用统一的 "link" collection
	var kbID int64
	if kbIDStr, ok := filter["kb_id"]; ok {
		_, err := fmt.Sscanf(kbIDStr, "%d", &kbID)
		if err != nil {
			return nil, fmt.Errorf("invalid kb_id format: %s", kbIDStr)
		}
	}

	// 使用 VectorRetriever 进行向量搜索（统一使用 "link" collection）
	results, err := a.retriever.SearchVectors(ctx, kbID, vector, &SearchOptions{
		TopK:            topK,
		VectorFieldName: "dense_vector",
		Expr:            buildFilterExpr(filter),
		OutputFields:    []string{"chunk_id", "knowledge_id", "kb_id", "tenant_id", "chunk_index", "content", "is_enabled", "start_at", "end_at", "token_count"},
	})
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 转换为 domainrag.Document
	docs := make([]*domainrag.Document, 0, len(results))
	for _, r := range results {
		docs = append(docs, &domainrag.Document{
			ChunkID:     r.ChunkID,
			KnowledgeID: r.KnowledgeID,
		 KnowledgeBaseID:        r.KnowledgeBaseID,
			Content:     r.Content,
			// dense 检索走 L2 度量（距离越小越相似），而领域检索链路统一按
			// 「分数越大越相关」排序/阈值/RRF/重排。此处在适配边界把 L2 距离转成
			// 单调相似度 1/(1+d)，与 BM25/RRF 的语义对齐，避免最佳匹配被降序排到末位。
			Score:      l2DistanceToSimilarity(r.Score),
			ChunkIndex: r.ChunkIndex,
			Metadata: map[string]interface{}{
				"tenant_id":   r.TenantID,
				"is_enabled":  r.IsEnabled,
				"start_at":    r.StartAt,
				"end_at":      r.EndAt,
				"token_count": r.TokenCount,
			},
		})
	}

	return docs, nil
}

// l2DistanceToSimilarity 将 Milvus L2 距离（[0,∞)，越小越相似）映射为
// 单调递减的相似度分（(0,1]，距离 0→1.0），使 dense 结果与领域层
// 「分数越大越相关」的排序/阈值/融合语义一致。
func l2DistanceToSimilarity(distance float32) float32 {
	if distance < 0 {
		distance = 0
	}
	return 1.0 / (1.0 + distance)
}

// FullTextSearch BM25 全文搜索 - 实现 MilvusRetriever 接口
func (a *MilvusAdapter) FullTextSearch(ctx context.Context, collectionName string, query string, topK int, filter map[string]string) ([]*domainrag.Document, error) {
	// 从 filter 中提取 kb_id，用于在统一 collection 中过滤
	// 所有知识库现在使用统一的 "link" collection
	var kbID int64
	if kbIDStr, ok := filter["kb_id"]; ok {
		_, err := fmt.Sscanf(kbIDStr, "%d", &kbID)
		if err != nil {
			return nil, fmt.Errorf("invalid kb_id format: %s", kbIDStr)
		}
	}

	// 使用 VectorRetriever 进行 BM25 全文搜索（统一使用 "link" collection）
	results, err := a.retriever.FullTextSearch(ctx, kbID, query, topK, filter)
	if err != nil {
		return nil, fmt.Errorf("BM25 full-text search failed: %w", err)
	}

	// 转换为 domainrag.Document
	docs := make([]*domainrag.Document, 0, len(results))
	for _, r := range results {
		docs = append(docs, &domainrag.Document{
			ChunkID:     r.ChunkID,
			KnowledgeID: r.KnowledgeID,
		 KnowledgeBaseID:        r.KnowledgeBaseID,
			Content:     r.Content,
			Score:       r.Score,
			ChunkIndex:  r.ChunkIndex,
			Metadata: map[string]interface{}{
				"tenant_id":   r.TenantID,
				"is_enabled":  r.IsEnabled,
				"start_at":    r.StartAt,
				"end_at":      r.EndAt,
				"token_count": r.TokenCount,
			},
		})
	}

	return docs, nil
}

// buildFilterExpr 构建过滤表达式
func buildFilterExpr(filter map[string]string) string {
	if len(filter) == 0 {
		return ""
	}

	expr := ""
	for k, v := range filter {
		if expr != "" {
			expr += " && "
		}

		// 根据字段类型构建表达式
		switch k {
		case "tenant_id":
			expr += fmt.Sprintf("%s == %s", k, v)
		case "kb_id", "knowledge_id", "chunk_id":
			expr += fmt.Sprintf("%s == '%s'", k, v)
		case "is_enabled":
			if v == "true" {
				expr += "is_enabled == true"
			} else {
				expr += "is_enabled == false"
			}
		default:
			// 默认字符串相等
			expr += fmt.Sprintf("%s == '%s'", k, v)
		}
	}

	return expr
}
