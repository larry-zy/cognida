// Package adapters —— 知识块精确拉取适配器：把 Milvus 向量库仓储 *retriever.VectorRetriever
// 适配为 Agent 工具层声明的 tools.ChunkFetcher。
//
// 关键设计（与 RAGRetrieverAdapter 同治理）：租户与 KB 检索范围不由 LLM 决定，而由用户在会话
// 入口选定并经 ctx 透传，适配器在此完成 scope 强制。由于全部知识库共享同一个 Milvus collection、
// 仅靠标量字段（tenant_id / kb_id / …）过滤，若放任 LLM 传入任意 tenant_id 会造成跨租户泄漏；
// 因此本适配器强制 tenant_id 来自 ctx、kb_id 只能在授权范围内收窄，从表达式层面堵死越权。
package adapters

import (
	"context"
	"fmt"
	"strings"

	agentctx "link/internal/model/agent"
	"link/internal/model/knowledge"
	"link/internal/repository/milvus/retriever"
	"link/internal/service/agent/tools"
)

// chunkQuerier 是 ChunkFetchAdapter 对底层向量库仓储的最小依赖面：仅需按标量过滤直取分块。
// *retriever.VectorRetriever 天然满足；抽成接口便于在无 Milvus 环境下以 fake 覆盖治理与映射逻辑。
type chunkQuerier interface {
	Query(ctx context.Context, kbID int64, opts *retriever.QueryOptions) ([]*retriever.DocumentData, error)
}

// ChunkFetchAdapter 将底层向量库仓储适配为 tools.ChunkFetcher。
type ChunkFetchAdapter struct {
	retriever chunkQuerier
	kbRepo    knowledge.KnowledgeBaseRepository
}

// NewChunkFetchAdapter 创建知识块精确拉取适配器。
func NewChunkFetchAdapter(vr *retriever.VectorRetriever, kbRepo knowledge.KnowledgeBaseRepository) *ChunkFetchAdapter {
	// 显式判空后再赋值：typed-nil (*VectorRetriever)(nil) 装进接口会变成非 nil 接口值，
	// 绕过 FetchChunks 里的 a.retriever == nil 判断，故在此保留为真正的 nil 接口。
	if vr == nil {
		return &ChunkFetchAdapter{retriever: nil, kbRepo: kbRepo}
	}
	return &ChunkFetchAdapter{retriever: vr, kbRepo: kbRepo}
}

// emptyChunks 统一的空结果（非 nil 切片，避免上层 JSON 序列化成 null）。
func emptyChunks() *tools.FetchChunksResult {
	return &tools.FetchChunksResult{Chunks: []tools.FetchedChunk{}, Count: 0}
}

// FetchChunks 实现 tools.ChunkFetcher。ctxAny 由工具层透传，断言回 context.Context 读取 scope。
func (a *ChunkFetchAdapter) FetchChunks(ctxAny interface{}, req *tools.FetchChunksRequest) (*tools.FetchChunksResult, error) {
	ctx := asContext(ctxAny)
	if a.retriever == nil {
		return nil, fmt.Errorf("向量检索器未接线")
	}
	if req == nil {
		return emptyChunks(), nil
	}

	tenantID := agentctx.MustGetTenantID(ctx)
	if tenantID <= 0 {
		return emptyChunks(), nil // 缺少有效租户信息时不拉取，避免落到 tenant=0
	}

	// KB 范围：用户选定 ∩ 租户内已启用（与 rag_query 完全一致的授权上界）。
	scope := resolveKBScope(ctx, a.kbRepo, tenantID)
	if len(scope) == 0 {
		return emptyChunks(), nil
	}

	// Agent 可经 kb_ids 进一步收窄，但不得越出授权上界（越权/陌生 ID 一律丢弃）。
	effectiveKBs := scope
	if len(req.KBIDs) > 0 {
		effectiveKBs = intersect(req.KBIDs, scope)
		if len(effectiveKBs) == 0 {
			return emptyChunks(), nil
		}
	}

	filter := buildChunkFilter(tenantID, effectiveKBs, req)

	docs, err := a.retriever.Query(ctx, 0, &retriever.QueryOptions{
		Expr:   []string{filter},
		Limit:  int64(req.Limit),
		Offset: int64(req.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("知识块拉取失败: %w", err)
	}

	chunks := make([]tools.FetchedChunk, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		chunks = append(chunks, tools.FetchedChunk{
			ChunkID:     d.ChunkID,
			KnowledgeID: d.KnowledgeID,
			KBID:        d.KnowledgeBaseID,
			ChunkIndex:  d.ChunkIndex,
			Content:     d.Content,
			IsEnabled:   d.IsEnabled,
			TokenCount:  d.TokenCount,
		})
	}

	return &tools.FetchChunksResult{Chunks: chunks, Count: len(chunks)}, nil
}

// buildChunkFilter 组装 Milvus 标量过滤表达式：tenant_id 与 kb_id 为强制的安全边界，
// chunk_id / knowledge_id / is_enabled 为可选收窄条件；各子句以 and 连接。
func buildChunkFilter(tenantID int64, kbIDs []string, req *tools.FetchChunksRequest) string {
	clauses := []string{
		fmt.Sprintf("tenant_id == %d", tenantID),          // 强制：租户边界
		"kb_id in " + milvusStrList(kbIDs),                // 强制：授权范围内的 KB
	}
	if len(req.ChunkIDs) > 0 {
		clauses = append(clauses, "chunk_id in "+milvusStrList(req.ChunkIDs))
	}
	if len(req.KnowledgeIDs) > 0 {
		clauses = append(clauses, "knowledge_id in "+milvusStrList(req.KnowledgeIDs))
	}
	if req.EnabledOnly {
		clauses = append(clauses, "is_enabled == true")
	}
	return strings.Join(clauses, " and ")
}

// milvusStrList 把字符串切片渲染为 Milvus 表达式的字符串数组字面量，如 ["a", "b"]。
func milvusStrList(vals []string) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, milvusQuote(v))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// milvusQuote 将字符串安全地包成 Milvus 表达式的双引号字面量（转义反斜杠与双引号），
// 防止 ID 里的特殊字符破坏表达式或被用于注入。
func milvusQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
