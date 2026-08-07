package knowledge

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"link/internal/model/rag"
)

// errRetrieverNotWired 底层检索器未注入时的哨兵错误。
var errRetrieverNotWired = errors.New("检索器未接线")

// ========================================
// 知识检索能力封装（单一事实源）
// ========================================
//
// RetrievalCapability 是知识库「检索注入」的唯一能力封装：多库检索 → 跨库去重 →
// 单片截断 → 出处标签 → 质量阈值 → 可插拔重排。此前 Agent 路径（rag_query）把这套治理
// 逻辑写死在工具适配器里、而 REST 路径（/knowledge/search）走裸子串匹配，两路实现质量割裂。
// 现收敛为一路：Agent 适配器与 REST handler 都经本能力检索，只在「检索范围如何确定」上各自
// 负责（Agent 读会话 ctx，REST 按请求 + 租户归属），检索本体与治理完全一致。
//
// 检索范围（哪些知识库）由调用方在传入前完成租户边界强制；本能力只对给定 kbIDs 检索，
// 不再触碰知识库归属仓储，职责单一。

const (
	// capMaxChunkContentChars 单个片段回灌上限（字符）：截断超长文档，保住语义主体又不撑爆上下文。
	capMaxChunkContentChars = 1600
	// capDefaultMaxChunks 未指定 TopK 时的片段数硬上限，兜底防跨库合并后条数失控。
	capDefaultMaxChunks = 20
	// capHybridRelFloor 混合检索的相对质量下限：RRF 融合分与余弦阈值量纲不同（约 0.01~0.03），
	// 无法直接套 SimilarityThreshold。改以「最高分的相对比例」为下限，丢弃 < floor×maxScore 的弱命中，
	// 避免 hybrid 模式把一堆几乎不相关的片段当证据全量回灌。
	capHybridRelFloor = 0.3
)

// GovernedQuery 是一次受治理检索的入参（范围已由调用方按租户边界确定）。
type GovernedQuery struct {
	Query        string
	Mode         string  // vector / bm25 / hybrid（其它值按 hybrid 处理）
	TopK         int     // ≤0 时用 capDefaultMaxChunks 兜底
	MinScore     float64 // 单模式（vector/bm25）的余弦相似度阈值，透传给底层检索器
	Alpha        float64 // 混合检索向量/关键词融合权重，透传
	EnableRerank bool    // 可插拔重排开关，默认关；开启且注入了 reranker 时对合并结果单次重排
}

// GovernedChunk 是受治理后回灌上下文的单个片段（已截断、带出处）。
type GovernedChunk struct {
	Content     string
	Score       float64
	Source      string // 人类可读出处：文档标题→知识条目→片段 ID→知识库 ID
	KnowledgeID string
	ChunkID     string
	ChunkIndex  int
	Metadata    map[string]interface{}
}

// GovernedResult 是一次受治理检索的结果。
type GovernedResult struct {
	Chunks    []GovernedChunk
	HasAnswer bool // 是否有通过质量阈值的命中；false 时调用方应向 LLM 明示「无可靠证据」
}

// RetrievalCapability 封装底层检索器 + 可插拔重排器。
type RetrievalCapability struct {
	retriever rag.Retriever
	reranker  rag.Reranker // 可插拔；为 nil 或 GovernedQuery.EnableRerank=false 时跳过重排
}

// NewRetrievalCapability 创建检索能力封装。reranker 可为 nil（重排能力缺省关闭）。
func NewRetrievalCapability(retriever rag.Retriever, reranker rag.Reranker) *RetrievalCapability {
	return &RetrievalCapability{retriever: retriever, reranker: reranker}
}

// Retrieve 在给定知识库范围内执行受治理检索。kbIDs 须已由调用方完成租户边界强制。
func (c *RetrievalCapability) Retrieve(ctx context.Context, tenantID int64, kbIDs []string, q GovernedQuery) (*GovernedResult, error) {
	if c.retriever == nil {
		return nil, errRetrieverNotWired
	}
	if len(kbIDs) == 0 || strings.TrimSpace(q.Query) == "" {
		return &GovernedResult{Chunks: []GovernedChunk{}, HasAnswer: false}, nil
	}
	tenantStr := strconv.FormatInt(tenantID, 10)

	// TopK 兜底集中在此处：≤0 一律回落硬上限，既用于底层逐库检索的 opts.TopK（否则
	// BM25 的 results[:0] / 向量库 topK=0 会直接返回空），也用于最终跨库合并后的截断。
	limit := q.TopK
	if limit <= 0 {
		limit = capDefaultMaxChunks
	}

	// 底层逐库检索强制关闭重排：重排改由本能力在跨库合并后单次执行（对完整候选集重排，
	// 而非各库各自重排后再合并），既是「可插拔」的统一落点，也避免双重重排。
	opts := &rag.RetrieveOptions{
		TopK:                limit,
		SimilarityThreshold: q.MinScore,
		RerankEnabled:       false,
		RetrievalMode:       q.Mode,
		Alpha:               q.Alpha,
	}

	// 跨库合并 + 去重：同一片段可能被多库/多模式重复命中，按 ChunkID（退回 Content）去重保留最高分。
	var merged []*rag.Document
	seen := make(map[string]int) // 去重键 -> merged 下标
	for _, kbID := range kbIDs {
		resp, err := c.retrieveByMode(ctx, tenantStr, kbID, q.Query, q.Mode, opts)
		if err != nil || resp == nil {
			continue // 单库失败不阻断整体
		}
		for _, d := range resp.Results {
			if d == nil {
				continue
			}
			key := d.ChunkID
			if strings.TrimSpace(key) == "" {
				key = d.Content
			}
			if idx, dup := seen[key]; dup {
				if d.Score > merged[idx].Score { // 命中重复：保留更高分
					merged[idx] = d
				}
				continue
			}
			seen[key] = len(merged)
			merged = append(merged, d)
		}
	}

	// 混合模式相对质量下限：丢弃 RRF 量纲下明显弱相关的命中（vector/bm25 已在底层按余弦阈值过滤）。
	merged = applyHybridFloor(merged, q.Mode)

	// 排序：默认按分数降序；重排开启且注入了 reranker 时改由重排决定顺序（对完整候选集单次重排）。
	if q.EnableRerank && c.reranker != nil && len(merged) > 0 {
		if reranked, err := c.reranker.Rerank(ctx, merged, q.Query); err == nil && reranked != nil {
			merged = reranked
		}
	} else {
		sort.SliceStable(merged, func(i, j int) bool { return merged[i].Score > merged[j].Score })
	}

	// 取 TopK（limit 已在入口按硬上限兜底）。
	if len(merged) > limit {
		merged = merged[:limit]
	}

	chunks := make([]GovernedChunk, 0, len(merged))
	for _, d := range merged {
		chunks = append(chunks, GovernedChunk{
			Content:     truncateRunes(d.Content, capMaxChunkContentChars),
			Score:       float64(d.Score),
			Source:      chunkSourceLabel(d),
			KnowledgeID: d.KnowledgeID,
			ChunkID:     d.ChunkID,
			ChunkIndex:  d.ChunkIndex,
			Metadata:    d.Metadata,
		})
	}
	return &GovernedResult{Chunks: chunks, HasAnswer: len(chunks) > 0}, nil
}

// retrieveByMode 按检索模式选择底层检索器方法（未知模式按 hybrid）。
func (c *RetrievalCapability) retrieveByMode(ctx context.Context, tenantID, kbID, query, mode string, opts *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	switch mode {
	case "vector":
		return c.retriever.VectorRetrieve(ctx, tenantID, kbID, query, opts)
	case "bm25":
		return c.retriever.BM25Retrieve(ctx, tenantID, kbID, query, opts)
	default:
		return c.retriever.HybridRetrieve(ctx, tenantID, kbID, query, opts)
	}
}

// applyHybridFloor 对混合模式的合并结果套相对质量下限；非混合模式原样返回。
func applyHybridFloor(docs []*rag.Document, mode string) []*rag.Document {
	if mode == "vector" || mode == "bm25" || len(docs) == 0 {
		return docs
	}
	var maxScore float32
	for _, d := range docs {
		if d.Score > maxScore {
			maxScore = d.Score
		}
	}
	if maxScore <= 0 {
		return docs // 分数量纲异常（全 0/负）时不过滤，避免误删全部命中
	}
	floor := maxScore * float32(capHybridRelFloor)
	kept := docs[:0]
	for _, d := range docs {
		if d.Score >= floor {
			kept = append(kept, d)
		}
	}
	return kept
}

// truncateRunes 按 rune 截断，避免切断多字节 CJK；截断时追加省略标记。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "…（内容已截断）"
}

// chunkSourceLabel 推导人类可读出处：优先文档标题，退回知识条目/片段 ID，最后知识库 ID。
func chunkSourceLabel(d *rag.Document) string {
	if d.Metadata != nil {
		for _, k := range []string{"title", "document_title", "doc_title", "file_name", "filename", "source", "name"} {
			if v, ok := d.Metadata[k]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
	}
	if strings.TrimSpace(d.KnowledgeID) != "" {
		return d.KnowledgeID
	}
	if strings.TrimSpace(d.ChunkID) != "" {
		return d.ChunkID
	}
	return d.KnowledgeBaseID
}
