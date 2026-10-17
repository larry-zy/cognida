// Package adapters —— 知识块精确拉取适配器：把领域 ChunkIndexReader
// 适配为 Agent 工具层声明的 tools.ChunkFetcher。
//
// 关键设计（与 RAGRetrieverAdapter 同治理）：租户与 KB 检索范围不由 LLM 决定，而由用户在会话
// 入口选定并经 ctx 透传，适配器在此完成 scope 强制。由于全部知识库共享同一个 Milvus collection、
// 仅靠标量字段（tenant_id / kb_id / …）过滤，若放任 LLM 传入任意 tenant_id 会造成跨租户泄漏；
// 因此本适配器强制 tenant_id 来自 ctx、kb_id 只能在授权范围内收窄，从表达式层面堵死越权。
package adapters

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/knowledge"
	"cognida/internal/model/rag"
	"cognida/internal/service/agent/tools"
)

// ChunkFetchAdapter 将底层向量库仓储适配为 tools.ChunkFetcher。
type ChunkFetchAdapter struct {
	retriever knowledge.ChunkIndexReader
	kbRepo    knowledge.KnowledgeBaseRepository
	guard     rag.ConsumptionGuard
}

func (a *ChunkFetchAdapter) WithConsumptionGuard(guard rag.ConsumptionGuard) *ChunkFetchAdapter {
	a.guard = guard
	return a
}

// NewChunkFetchAdapter 创建知识块精确拉取适配器。
func NewChunkFetchAdapter(reader knowledge.ChunkIndexReader, kbRepo knowledge.KnowledgeBaseRepository) *ChunkFetchAdapter {
	// An absent pointer implementation must remain unwired, not a non-nil
	// interface that can panic when its method dereferences the receiver.
	if reader != nil {
		value := reflect.ValueOf(reader)
		if value.Kind() == reflect.Ptr && value.IsNil() {
			reader = nil
		}
	}
	return &ChunkFetchAdapter{retriever: reader, kbRepo: kbRepo}
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

	docs, err := a.retriever.QueryChunks(ctx, knowledge.ChunkIndexQuery{
		TenantID:     tenantID,
		KBIDs:        effectiveKBs,
		ChunkIDs:     req.ChunkIDs,
		KnowledgeIDs: req.KnowledgeIDs,
		EnabledOnly:  req.EnabledOnly,
		Limit:        int64(req.Limit),
		Offset:       int64(req.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("知识块拉取失败: %w", err)
	}
	if a.guard != nil {
		candidates := make([]*rag.Document, 0, len(docs))
		for _, doc := range docs {
			if doc != nil {
				candidates = append(candidates, &rag.Document{ChunkID: doc.ChunkID, KnowledgeID: doc.KnowledgeID, KnowledgeBaseID: doc.KnowledgeBaseID, Content: doc.Content})
			}
		}
		allowed, err := a.guard.Filter(ctx, strconv.FormatInt(tenantID, 10), "", candidates)
		if err != nil {
			return nil, err
		}
		// A shared chunk ID can have an old and a current index projection.
		// Only the exact content and ownership tuple approved by the guard is visible.
		type chunkIdentity struct{ chunkID, knowledgeID, kbID, content string }
		identities := map[chunkIdentity]bool{}
		for _, doc := range allowed {
			if doc != nil {
				identities[chunkIdentity{doc.ChunkID, doc.KnowledgeID, doc.KnowledgeBaseID, doc.Content}] = true
			}
		}
		visible := docs[:0]
		for _, doc := range docs {
			if doc != nil && identities[chunkIdentity{doc.ChunkID, doc.KnowledgeID, doc.KnowledgeBaseID, doc.Content}] {
				visible = append(visible, doc)
			}
		}
		docs = visible
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

	// Milvus query 不保证返回顺序，按 (knowledge_id, chunk_index, chunk_id) 稳定排序，
	// 使同一请求可复现、同文档分块按序阅读；深翻页的跨页一致性另由工具层的窗口封顶约束兜底。
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].KnowledgeID != chunks[j].KnowledgeID {
			return chunks[i].KnowledgeID < chunks[j].KnowledgeID
		}
		if chunks[i].ChunkIndex != chunks[j].ChunkIndex {
			return chunks[i].ChunkIndex < chunks[j].ChunkIndex
		}
		return chunks[i].ChunkID < chunks[j].ChunkID
	})

	return &tools.FetchChunksResult{Chunks: chunks, Count: len(chunks)}, nil
}
