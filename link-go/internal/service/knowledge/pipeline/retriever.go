// Package rag 提供 RAG 基础设施层实现
package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	domainrag "link/internal/model/rag"
	"link/internal/model/knowledge"
)

// ========================================
// Retriever 实现
// ========================================

// RetrieverImpl 检索器实现
type RetrieverImpl struct {
	kbSettingRepo       knowledge.KnowledgeBaseSettingRepository
	chunkRepo           knowledge.ChunkRepository
	embedder            embedding.Embedder
	milvusRetriever     MilvusRetriever
	neo4jRepo           domainrag.GraphRepository
	graphQueryRepo      domainrag.GraphQueryRepository
	reranker            domainrag.Reranker
}

// MilvusRetriever Milvus 检索器接口
type MilvusRetriever interface {
	Search(ctx context.Context, collectionName string, vector []float32, topK int, filter map[string]string) ([]*domainrag.Document, error)
	// FullTextSearch 使用 BM25 全文搜索
	FullTextSearch(ctx context.Context, collectionName string, query string, topK int, filter map[string]string) ([]*domainrag.Document, error)
}

// NewRetriever 创建检索器
func NewRetriever(
	kbSettingRepo knowledge.KnowledgeBaseSettingRepository,
	chunkRepo knowledge.ChunkRepository,
	embedder embedding.Embedder,
	milvusRetriever MilvusRetriever,
	neo4jRepo domainrag.GraphRepository,
	graphQueryRepo domainrag.GraphQueryRepository,
	reranker domainrag.Reranker,
) domainrag.Retriever {
	return &RetrieverImpl{
		kbSettingRepo:   kbSettingRepo,
		chunkRepo:       chunkRepo,
		embedder:        embedder,
		milvusRetriever: milvusRetriever,
		neo4jRepo:       neo4jRepo,
		graphQueryRepo:  graphQueryRepo,
		reranker:        reranker,
	}
}

// Retrieve 执行检索
func (r *RetrieverImpl) Retrieve(ctx context.Context, tenantID, kbID, query string, opts *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultRetrieveOptions()
	}

	switch opts.RetrievalMode {
	case "vector":
		return r.VectorRetrieve(ctx, tenantID, kbID, query, opts)
	case "bm25":
		return r.BM25Retrieve(ctx, tenantID, kbID, query, opts)
	case "graph":
		return r.GraphRetrieve(ctx, tenantID, kbID, query, opts)
	default:
		return r.HybridRetrieve(ctx, tenantID, kbID, query, opts)
	}
}

// RetrieveWithEmbedding 使用提供的向量执行检索
func (r *RetrieverImpl) RetrieveWithEmbedding(ctx context.Context, tenantID, kbID, query string, embedding []float32, opts *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultRetrieveOptions()
	}

	startTime := time.Now()
	results := make([]*domainrag.Document, 0)
	trace := &domainrag.SearchTrace{
		RetrievalDetails: make([]domainrag.RetrievalStep, 0),
	}

	// 向量检索
	if r.milvusRetriever != nil {
		vectorDocs, err := r.milvusRetriever.Search(ctx, kbID, embedding, opts.TopK, map[string]string{
			"tenant_id": tenantID,
		})
		if err == nil && len(vectorDocs) > 0 {
			results = append(results, vectorDocs...)
			trace.RetrievalDetails = append(trace.RetrievalDetails, domainrag.RetrievalStep{
				StepType:    "milvus_vector",
				Description: "Milvus 向量检索",
				Latency:     time.Since(startTime).Milliseconds(),
				ResultCount: len(vectorDocs),
			})
		}
	}

	// 应用相似度阈值
	filtered := r.filterByThreshold(results, opts.SimilarityThreshold)

	// 重排
	if opts.RerankEnabled && r.reranker != nil {
		rerankStart := time.Now()
		filtered = rerankOrKeep(ctx, r.reranker, filtered, query)
		trace.RerankLatency = time.Since(rerankStart).Milliseconds()
		trace.RerankedCount = len(filtered)
	}

	return &domainrag.RetrieveResponse{
		Results:     filtered,
		Query:       query,
		TotalCount:  len(filtered),
		HasMore:     false,
		Latency:     time.Since(startTime).Milliseconds(),
		SearchTrace: trace,
	}, nil
}

// VectorRetrieve 向量检索
func (r *RetrieverImpl) VectorRetrieve(ctx context.Context, tenantID, kbID, query string, opts *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultRetrieveOptions()
	}

	startTime := time.Now()
	results := make([]*domainrag.Document, 0)
	trace := &domainrag.SearchTrace{
		RetrievalDetails: make([]domainrag.RetrievalStep, 0),
	}

	// 生成向量
	var embeddingVec []float32
	if r.embedder != nil {
		vecs, err := r.embedder.EmbedStrings(ctx, []string{query})
		if err == nil && len(vecs) > 0 {
			// 将 float64 转换为 float32
			for _, v := range vecs[0] {
				embeddingVec = append(embeddingVec, float32(v))
			}
		}
	}

	// Milvus 向量检索
	if r.milvusRetriever != nil && len(embeddingVec) > 0 {
		vectorDocs, err := r.milvusRetriever.Search(ctx, kbID, embeddingVec, opts.TopK, map[string]string{
			"tenant_id": tenantID,
		})
		if err == nil && len(vectorDocs) > 0 {
			for _, doc := range vectorDocs {
				doc.MatchType = "vector"
			}
			results = append(results, vectorDocs...)
		}
		trace.VectorLatency = time.Since(startTime).Milliseconds()
		trace.VectorResultCount = len(vectorDocs)
		trace.RetrievalDetails = append(trace.RetrievalDetails, domainrag.RetrievalStep{
			StepType:    "vector",
			Description: "向量检索",
			Latency:     trace.VectorLatency,
			ResultCount: trace.VectorResultCount,
		})
	}

	// 内存向量检索（降级方案）
	if len(results) == 0 && r.chunkRepo != nil {
		memoryDocs, err := r.vectorRetrieveWithEmbedding(ctx, tenantID, kbID, query, embeddingVec, opts)
		if err == nil && len(memoryDocs) > 0 {
			for _, doc := range memoryDocs {
				doc.MatchType = "vector"
			}
			results = append(results, memoryDocs...)
		}
	}

	// 应用相似度阈值
	filtered := r.filterByThreshold(results, opts.SimilarityThreshold)

	// 重排
	if opts.RerankEnabled && r.reranker != nil {
		rerankStart := time.Now()
		filtered = rerankOrKeep(ctx, r.reranker, filtered, query)
		trace.RerankLatency = time.Since(rerankStart).Milliseconds()
		trace.RerankedCount = len(filtered)
	}

	return &domainrag.RetrieveResponse{
		Results:     filtered,
		Query:       query,
		TotalCount:  len(filtered),
		HasMore:     false,
		Latency:     time.Since(startTime).Milliseconds(),
		SearchTrace: trace,
	}, nil
}

// BM25Retrieve BM25 关键词检索
func (r *RetrieverImpl) BM25Retrieve(ctx context.Context, tenantID, kbID, query string, opts *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultRetrieveOptions()
	}

	startTime := time.Now()
	results := make([]*domainrag.Document, 0)
	trace := &domainrag.SearchTrace{
		RetrievalDetails: make([]domainrag.RetrievalStep, 0),
	}

	// 优先使用 Milvus BM25 全文搜索
	if r.milvusRetriever != nil {
		bm25Docs, err := r.milvusRetriever.FullTextSearch(ctx, kbID, query, opts.TopK, map[string]string{
			"tenant_id": tenantID,
		})
		if err == nil && len(bm25Docs) > 0 {
			for _, doc := range bm25Docs {
				doc.MatchType = "bm25"
				results = append(results, doc)
			}
		}
		// 如果 Milvus 检索失败或无结果，fallback 到数据库检索
	}

	// Fallback: 从数据库检索（如果 Milvus 未启用或无结果）。
	// 按查询词的词项重合度打分排序，而非固定分数，保证降级结果仍按相关性排序。
	if len(results) == 0 && r.chunkRepo != nil {
		chunks, err := r.chunkRepo.FindEnabledChunks(ctx, kbID, opts.TopK*10)
		if err == nil {
			for _, chunk := range chunks {
				score := keywordScore(chunk.Content, query)
				if score <= 0 {
					continue
				}
				results = append(results, &domainrag.Document{
					ChunkID:         chunk.ID,
					KnowledgeID:     chunk.KnowledgeID,
					KnowledgeBaseID: chunk.KnowledgeBaseID,
					Content:         chunk.Content,
					Score:           score,
					MatchType:       "bm25",
					ChunkIndex:      chunk.ChunkIndex,
				})
			}
			sortDocsByScoreDesc(results)
			if len(results) > opts.TopK {
				results = results[:opts.TopK]
			}
		}
	}

	trace.BM25Latency = time.Since(startTime).Milliseconds()
	trace.BM25ResultCount = len(results)
	trace.RetrievalDetails = append(trace.RetrievalDetails, domainrag.RetrievalStep{
		StepType:    "bm25",
		Description: "BM25 全文检索",
		Latency:     trace.BM25Latency,
		ResultCount: trace.BM25ResultCount,
	})

	// 应用相似度阈值
	filtered := r.filterByThreshold(results, opts.SimilarityThreshold)

	return &domainrag.RetrieveResponse{
		Results:     filtered,
		Query:       query,
		TotalCount:  len(filtered),
		HasMore:     false,
		Latency:     time.Since(startTime).Milliseconds(),
		SearchTrace: trace,
	}, nil
}

// HybridRetrieve 混合检索（向量+BM25）
func (r *RetrieverImpl) HybridRetrieve(ctx context.Context, tenantID, kbID, query string, opts *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultRetrieveOptions()
	}

	startTime := time.Now()
	trace := &domainrag.SearchTrace{
		RetrievalDetails: make([]domainrag.RetrievalStep, 0),
	}

	// 并发执行向量检索和 BM25 检索
	var (
		vectorResults []*domainrag.Document
		bm25Results   []*domainrag.Document
		wg            sync.WaitGroup
		mu            sync.Mutex
	)

	// 向量检索
	wg.Add(1)
	go func() {
		defer wg.Done()
		vectorResp, err := r.VectorRetrieve(ctx, tenantID, kbID, query, &domainrag.RetrieveOptions{
			TopK: opts.TopK * 2,
			// 相似度阈值只对向量分量有意义（余弦分数量纲）；在此分量侧过滤，
			// 融合后的 RRF 分数量纲不同（约 0.01~0.03），不能再套用该阈值。
			SimilarityThreshold: opts.SimilarityThreshold,
			RetrievalMode:       "vector",
			RerankEnabled:       false,
		})
		if err == nil {
			mu.Lock()
			vectorResults = vectorResp.Results
			mu.Unlock()
		}
	}()

	// BM25 检索
	wg.Add(1)
	go func() {
		defer wg.Done()
		bm25Resp, err := r.BM25Retrieve(ctx, tenantID, kbID, query, &domainrag.RetrieveOptions{
			TopK:          opts.TopK * 2,
			RetrievalMode: "bm25",
			RerankEnabled: false,
		})
		if err == nil {
			mu.Lock()
			bm25Results = bm25Resp.Results
			mu.Unlock()
		}
	}()

	wg.Wait()

	// RRF 融合
	alpha := opts.Alpha
	if alpha <= 0 {
		alpha = 0.5
	}

	fusedResults := r.rRFusion(vectorResults, bm25Results, opts.TopK, alpha)

	for _, doc := range fusedResults {
		doc.MatchType = "hybrid"
	}

	trace.RetrievalDetails = append(trace.RetrievalDetails, domainrag.RetrievalStep{
		StepType:    "fusion",
		Description: fmt.Sprintf("混合检索融合 (alpha=%.2f)", alpha),
		Latency:     time.Since(startTime).Milliseconds(),
		ResultCount: len(fusedResults),
	})

	// 注意：不再对融合结果套用 SimilarityThreshold。RRF 分数量纲（约 0.01~0.03）
	// 与余弦相似度阈值（0.5~0.7）不同，若在此过滤会误删全部命中导致检索恒为空。
	// 阈值已在向量分量侧按余弦量纲正确应用。
	filtered := fusedResults

	// 重排
	if opts.RerankEnabled && r.reranker != nil {
		rerankStart := time.Now()
		filtered = rerankOrKeep(ctx, r.reranker, filtered, query)
		trace.RerankLatency = time.Since(rerankStart).Milliseconds()
		trace.RerankedCount = len(filtered)
	}

	return &domainrag.RetrieveResponse{
		Results:     filtered,
		Query:       query,
		TotalCount:  len(filtered),
		HasMore:     false,
		Latency:     time.Since(startTime).Milliseconds(),
		SearchTrace: trace,
	}, nil
}

// GraphRetrieve 图谱检索
func (r *RetrieverImpl) GraphRetrieve(ctx context.Context, tenantID, kbID, query string, opts *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultRetrieveOptions()
	}

	startTime := time.Now()
	results := make([]*domainrag.Document, 0)
	trace := &domainrag.SearchTrace{
		RetrievalDetails: make([]domainrag.RetrievalStep, 0),
	}

	if r.graphQueryRepo != nil {
		// 使用图谱查询检索
		// 这里简化处理，实际应该从图谱中提取实体名称进行查询
		docs, err := r.graphQueryRepo.GetChunksByGraphNodes(ctx, kbID, []string{query})
		if err == nil {
			for _, doc := range docs {
				doc.MatchType = "graph"
				results = append(results, doc)
			}
		}
	}

	trace.GraphLatency = time.Since(startTime).Milliseconds()
	trace.GraphResultCount = len(results)
	trace.RetrievalDetails = append(trace.RetrievalDetails, domainrag.RetrievalStep{
		StepType:    "graph",
		Description: "图谱检索",
		Latency:     trace.GraphLatency,
		ResultCount: trace.GraphResultCount,
	})

	// 应用相似度阈值
	filtered := r.filterByThreshold(results, opts.SimilarityThreshold)

	return &domainrag.RetrieveResponse{
		Results:     filtered,
		Query:       query,
		TotalCount:  len(filtered),
		HasMore:     false,
		Latency:     time.Since(startTime).Milliseconds(),
		SearchTrace: trace,
	}, nil
}

// ========================================
// 辅助方法
// ========================================

// vectorRetrieveWithEmbedding 是无独立向量索引（Milvus 不可用）时的降级检索。
// chunk 表只存 EmbeddingID 引用、不内联向量，因此这里无法计算真实余弦相似度；
// 退化为按查询词的词项重合度做词法排序，保证降级结果按相关性而非固定分数返回。
// 真正的向量相似度由 Milvus 路径负责。embedding 参数保留以兼容签名。
func (r *RetrieverImpl) vectorRetrieveWithEmbedding(ctx context.Context, tenantID, kbID, query string, embedding []float32, opts *domainrag.RetrieveOptions) ([]*domainrag.Document, error) {
	chunks, err := r.chunkRepo.FindEnabledChunks(ctx, kbID, 1000)
	if err != nil {
		return nil, err
	}

	results := make([]*domainrag.Document, 0)
	for _, chunk := range chunks {
		score := keywordScore(chunk.Content, query)
		if score <= 0 {
			continue
		}
		results = append(results, &domainrag.Document{
			ChunkID:         chunk.ID,
			KnowledgeID:     chunk.KnowledgeID,
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			Content:         chunk.Content,
			Score:           score,
			MatchType:       "vector",
			ChunkIndex:      chunk.ChunkIndex,
		})
	}

	sortDocsByScoreDesc(results)
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}

	return results, nil
}

// filterByThreshold 按相似度阈值过滤
func (r *RetrieverImpl) filterByThreshold(docs []*domainrag.Document, threshold float64) []*domainrag.Document {
	if threshold <= 0 {
		return docs
	}

	filtered := make([]*domainrag.Document, 0)
	for _, doc := range docs {
		if doc.Score >= float32(threshold) {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}

// r RF Fusion 融合算法
func (r *RetrieverImpl) rRFusion(vectorDocs, bm25Docs []*domainrag.Document, topK int, alpha float64) []*domainrag.Document {
	// 构建文档映射
	docMap := make(map[string]*domainrag.Document)
	scoreMap := make(map[string]float64)

	k := 60 // RRF 常数

	// 处理向量结果
	for i, doc := range vectorDocs {
		key := doc.ChunkID
		if _, exists := docMap[key]; !exists {
			docMap[key] = doc
		}
		rrfScore := 1.0 / float64(k+i+1)
		scoreMap[key] += rrfScore * (1 - alpha)
	}

	// 处理 BM25 结果
	for i, doc := range bm25Docs {
		key := doc.ChunkID
		if _, exists := docMap[key]; !exists {
			docMap[key] = doc
		}
		rrfScore := 1.0 / float64(k+i+1)
		scoreMap[key] += rrfScore * alpha
	}

	// 按融合分数排序
	type scoredDoc struct {
		doc   *domainrag.Document
		score float64
	}
	scoredDocs := make([]scoredDoc, 0, len(docMap))
	for key, doc := range docMap {
		scoredDocs = append(scoredDocs, scoredDoc{
			doc:   doc,
			score: scoreMap[key],
		})
	}

	// 按融合分数降序排序（O(n log n)）
	sort.Slice(scoredDocs, func(i, j int) bool {
		return scoredDocs[i].score > scoredDocs[j].score
	})

	// 取 topK
	result := make([]*domainrag.Document, 0, min(topK, len(scoredDocs)))
	for i := 0; i < min(topK, len(scoredDocs)); i++ {
		doc := scoredDocs[i].doc
		doc.Score = float32(scoredDocs[i].score)
		result = append(result, doc)
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// keywordScore 返回 query 与 content 的词项重合度，范围 [0,1]，0 表示不相关。
// 作为无 BM25/向量索引时降级检索的排序依据：对以空格分词的查询按命中词项比例打分；
// 对中文等无空格文本，strings.Fields 退化为整串匹配，按是否为子串给 0 或 1。
func keywordScore(content, query string) float32 {
	content = strings.ToLower(content)
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || content == "" {
		return 0
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0
	}
	matched := 0
	for _, t := range terms {
		if strings.Contains(content, t) {
			matched++
		}
	}
	return float32(matched) / float32(len(terms))
}

// containsKeyword 判断 content 是否与 query 词法相关。
func containsKeyword(content, query string) bool {
	return keywordScore(content, query) > 0
}

// sortDocsByScoreDesc 按 Score 降序就地排序文档。
func sortDocsByScoreDesc(docs []*domainrag.Document) {
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})
}
