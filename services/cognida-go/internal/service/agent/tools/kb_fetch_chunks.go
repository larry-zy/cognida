// Package tools 提供知识块精确拉取工具（kb_fetch_chunks）。
package tools

import (
	"context"
	"fmt"
)

// ========================================
// 知识块精确拉取工具
// ========================================

// milvusWindowCap 是 Milvus query 的 offset+limit 窗口硬上限（超过即报错），用于翻页深度封顶。
const milvusWindowCap = 16384

// FetchChunksRequest 知识块精确拉取请求。
//
// 与 rag_query 的语义检索互补：本工具不做向量相似度，而是按标量字段精确过滤直取原文，
// 适合「已知 chunk_id / 文档ID，需拉取原始分块全文」或「按知识库遍历分块」的确定性场景。
//
// 安全约束：租户由系统从会话上下文强制注入，不接受也不信任调用方传入的租户；
// kb_ids 只能进一步收窄到「你被授权的检索范围」，越权/陌生 ID 会被静默丢弃。
type FetchChunksRequest struct {
	// ChunkIDs 按分块ID精确拉取（对应 chunks.id），可多值；留空表示不按此过滤。
	ChunkIDs []string `json:"chunk_ids" jsonschema:"description=按分块ID精确拉取（chunks.id），可传多个；留空表示不按此过滤"`

	// KnowledgeIDs 按文档ID拉取该文档下全部分块（对应 chunks.knowledge_id），可多值。
	KnowledgeIDs []string `json:"knowledge_ids" jsonschema:"description=按文档ID拉取该文档下的全部分块（chunks.knowledge_id），可传多个"`

	// KBIDs 限定知识库ID，须落在你被授权的检索范围内（越权ID将被忽略）；留空表示当前允许范围内全部。
	KBIDs []string `json:"kb_ids" jsonschema:"description=限定知识库ID（须在你被授权的检索范围内，越权ID将被忽略），可传多个；留空表示当前允许范围内全部"`

	// EnabledOnly 仅返回已启用的分块，默认 false（返回全部，含被禁用）。
	EnabledOnly bool `json:"enabled_only" jsonschema:"description=仅返回已启用的分块，默认false"`

	// Limit 最多返回条数，默认20，范围1-200。
	Limit int `json:"limit" jsonschema:"description=最多返回条数，默认20，范围1-200"`

	// Offset 分页偏移，默认0。
	Offset int `json:"offset" jsonschema:"description=分页偏移量，用于翻页，默认0"`
}

// FetchedChunk 单个知识块（精确拉取结果项）。
type FetchedChunk struct {
	// ChunkID 分块ID（chunks.id）。
	ChunkID string `json:"chunk_id"`
	// KnowledgeID 所属文档ID（chunks.knowledge_id）。
	KnowledgeID string `json:"knowledge_id"`
	// KBID 所属知识库ID（chunks.kb_id）。
	KBID string `json:"kb_id"`
	// ChunkIndex 分块在文档内的序号。
	ChunkIndex int `json:"chunk_index"`
	// Content 分块原文内容。
	Content string `json:"content"`
	// IsEnabled 是否启用。
	IsEnabled bool `json:"is_enabled"`
	// TokenCount 分块 token 数。
	TokenCount int64 `json:"token_count"`
}

// FetchChunksResult 知识块精确拉取结果。
type FetchChunksResult struct {
	// Chunks 命中的分块列表。
	Chunks []FetchedChunk `json:"chunks"`
	// Count 命中数量。
	Count int `json:"count"`
}

// NewKbFetchChunksTool 创建知识块精确拉取工具。
// fetcher 可为 nil（向量库未就绪）；此时工具仍注册，Run 时返回降级提示（宁拒不闯）。
func NewKbFetchChunksTool(fetcher ChunkFetcher) *TypedBaseTool[FetchChunksRequest, FetchChunksResult] {
	handler := func(ctx context.Context, req *FetchChunksRequest) (*FetchChunksResult, error) {
		// 至少需要一个过滤维度，避免无意义地全库拉取（且防止把整库分块喂进上下文）。
		if len(req.ChunkIDs) == 0 && len(req.KnowledgeIDs) == 0 && len(req.KBIDs) == 0 {
			return nil, fmt.Errorf("至少需指定 chunk_ids / knowledge_ids / kb_ids 之一，避免无边界拉取")
		}

		// 默认值与边界（在委派前收敛，适配器只需信任已归一化的值）。
		if req.Limit <= 0 {
			req.Limit = 20
		}
		if req.Limit > 200 {
			req.Limit = 200
		}
		if req.Offset < 0 {
			req.Offset = 0
		}
		// Milvus 硬约束：offset + limit ≤ 16384。超限直接 fail-closed 并给出可执行指引，
		// 避免深翻页落到底层驱动才报错（宁拒不闯）；深翻页请改用 knowledge_ids/chunk_ids 精确收窄。
		if req.Offset+req.Limit > milvusWindowCap {
			return nil, fmt.Errorf("翻页过深：offset(%d)+limit(%d) 超过上限 %d；请用 knowledge_ids/chunk_ids 精确收窄，或减小 offset",
				req.Offset, req.Limit, milvusWindowCap)
		}

		if fetcher == nil {
			return nil, fmt.Errorf("知识块拉取端口未接线（向量库/嵌入器未就绪），请改用 rag_query 语义检索")
		}
		return fetcher.FetchChunks(ctx, req)
	}

	return NewTypedBaseTool("kb_fetch_chunks",
		`按条件从知识库中精确拉取原始知识块（分块）内容，不做语义相似度计算。

与 rag_query 的区别：
- rag_query：用自然语言问题做向量/关键词检索，返回最相关的若干片段（适合「找答案」）。
- kb_fetch_chunks：用已知的标量条件（分块ID / 文档ID / 知识库ID）精确直取分块原文（适合「按ID取原文」「遍历某文档的全部分块」「翻页浏览」）。

适用场景：
- 已从检索结果或引用中拿到 chunk_id，需要拉取该分块的完整原文；
- 已知某文档ID（knowledge_id），需取回该文档被切分出的全部分块并按序阅读；
- 需要在某知识库内分页遍历分块做核对/统计。

检索范围治理：
- 租户由系统从会话上下文强制注入，你无法也无需传入；
- kb_ids 仅能在你被授权的检索范围内进一步收窄，越权/陌生 ID 会被忽略。

参数说明：
- chunk_ids: 分块ID列表（精确匹配 chunks.id）
- knowledge_ids: 文档ID列表（取这些文档下全部分块）
- kb_ids: 知识库ID列表（限定范围，须在授权范围内）
- enabled_only: 仅返回已启用分块（默认false）
- limit: 最多返回条数（默认20，范围1-200）
- offset: 分页偏移（默认0）

注意：chunk_ids / knowledge_ids / kb_ids 至少提供一个，否则拒绝执行。`,
		handler,
	)
}
