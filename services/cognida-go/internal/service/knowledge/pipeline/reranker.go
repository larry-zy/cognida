// Package rag 提供 RAG 基础设施层实现 - 重排器
package rag

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	domainrag "cognida/internal/model/rag"
	"cognida/internal/pkg/safego"
)

// ========================================
// Reranker 实现
// ========================================

// RerankerImpl 重排器实现
type RerankerImpl struct {
	strategy   string
	strategies map[string]domainrag.RerankerStrategy
}

// NewReranker 创建重排器
func NewReranker() domainrag.Reranker {
	rr := &RerankerImpl{
		strategy:   "rrf",
		strategies: make(map[string]domainrag.RerankerStrategy),
	}

	// 注册内置策略
	rr.RegisterStrategy("rrf", &RRFStrategy{})
	rr.RegisterStrategy("weighted", &WeightedFusionStrategy{})
	rr.RegisterStrategy("weighted_rrf", &WeightedRRFStrategy{})

	return rr
}

// Rerank 重排检索结果
func (r *RerankerImpl) Rerank(ctx context.Context, results []*domainrag.Document, query string) ([]*domainrag.Document, error) {
	if len(results) == 0 {
		return results, nil
	}

	// 获取当前策略
	strategy, exists := r.strategies[r.strategy]
	if !exists {
		return results, nil
	}

	// 执行重排
	reranked, err := strategy.Rerank(ctx, results, query)
	if err != nil {
		return nil, err
	}

	return reranked, nil
}

// SetStrategy 设置重排策略
func (r *RerankerImpl) SetStrategy(strategy string) error {
	if _, exists := r.strategies[strategy]; !exists {
		return fmt.Errorf("未知的重排策略: %s", strategy)
	}
	r.strategy = strategy
	return nil
}

// GetStrategy 获取当前策略
func (r *RerankerImpl) GetStrategy() string {
	return r.strategy
}

// RegisterStrategy 注册策略
func (r *RerankerImpl) RegisterStrategy(name string, strategy domainrag.RerankerStrategy) {
	r.strategies[name] = strategy
}

// ========================================
// RerankerStrategy 策略接口
// ========================================

// 注意：这个接口已在 domain 层定义，这里只是实现示例

// ========================================
// RRF Strategy - Reciprocal Rank Fusion
// ========================================

// RRFStrategy RRF 融合策略
type RRFStrategy struct {
	K int // RRF 常数，默认60
}

// Rerank 执行 RRF 重排
func (s *RRFStrategy) Rerank(ctx context.Context, results []*domainrag.Document, query string) ([]*domainrag.Document, error) {
	if s.K <= 0 {
		s.K = 60
	}

	// 按原始分数排序
	sorted := make([]*domainrag.Document, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// 计算 RRF 分数
	for i, doc := range sorted {
		rrfScore := 1.0 / float64(s.K+i+1)
		// 结合原始分数和 RRF 分数
		doc.Score = float32(0.7*float64(doc.Score) + 0.3*rrfScore)
	}

	// 重新排序
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	return sorted, nil
}

// ========================================
// WeightedFusionStrategy 加权融合策略
// ========================================

// WeightedFusionStrategy 加权融合策略
type WeightedFusionStrategy struct {
	VectorWeight float64 // 向量分数权重
	BM25Weight   float64 // BM25 分数权重
	GraphWeight  float64 // 图谱分数权重
}

// Rerank 执行加权融合重排
func (s *WeightedFusionStrategy) Rerank(ctx context.Context, results []*domainrag.Document, query string) ([]*domainrag.Document, error) {
	// 设置默认权重
	if s.VectorWeight <= 0 && s.BM25Weight <= 0 && s.GraphWeight <= 0 {
		s.VectorWeight = 0.5
		s.BM25Weight = 0.3
		s.GraphWeight = 0.2
	}

	// 按匹配类型分组
	vectorDocs := make([]*domainrag.Document, 0)
	bm25Docs := make([]*domainrag.Document, 0)
	graphDocs := make([]*domainrag.Document, 0)
	otherDocs := make([]*domainrag.Document, 0)

	for _, doc := range results {
		switch doc.MatchType {
		case "vector":
			vectorDocs = append(vectorDocs, doc)
		case "bm25":
			bm25Docs = append(bm25Docs, doc)
		case "graph":
			graphDocs = append(graphDocs, doc)
		default:
			otherDocs = append(otherDocs, doc)
		}
	}

	// 归一化分数
	normalizeScores(vectorDocs)
	normalizeScores(bm25Docs)
	normalizeScores(graphDocs)
	normalizeScores(otherDocs)

	// 合并结果并应用权重
	scoreMap := make(map[string]*domainrag.Document)
	weightMap := make(map[string]float64)

	// 处理向量结果
	totalWeight := s.VectorWeight + s.BM25Weight + s.GraphWeight
	for _, doc := range vectorDocs {
		key := doc.ChunkID
		if _, exists := scoreMap[key]; !exists {
			scoreMap[key] = doc
		}
		weightMap[key] += float64(doc.Score) * (s.VectorWeight / totalWeight)
	}

	// 处理 BM25 结果
	for _, doc := range bm25Docs {
		key := doc.ChunkID
		if _, exists := scoreMap[key]; !exists {
			scoreMap[key] = doc
		}
		weightMap[key] += float64(doc.Score) * (s.BM25Weight / totalWeight)
	}

	// 处理图谱结果
	for _, doc := range graphDocs {
		key := doc.ChunkID
		if _, exists := scoreMap[key]; !exists {
			scoreMap[key] = doc
		}
		weightMap[key] += float64(doc.Score) * (s.GraphWeight / totalWeight)
	}

	// 处理其他结果
	for _, doc := range otherDocs {
		key := doc.ChunkID
		if _, exists := scoreMap[key]; !exists {
			scoreMap[key] = doc
		}
		weightMap[key] += float64(doc.Score) * 0.1 // 低权重
	}

	// 应用融合分数
	for _, doc := range scoreMap {
		doc.Score = float32(weightMap[doc.ChunkID])
	}

	// 排序
	fused := make([]*domainrag.Document, 0, len(scoreMap))
	for _, doc := range scoreMap {
		fused = append(fused, doc)
	}
	sort.Slice(fused, func(i, j int) bool {
		return fused[i].Score > fused[j].Score
	})

	return fused, nil
}

// ========================================
// WeightedRRFStrategy 加权 RRF 策略
// ========================================

// WeightedRRFStrategy 加权 RRF 策略
type WeightedRRFStrategy struct {
	K           int     // RRF 常数
	VectorAlpha float64 // 向量权重
	BM25Alpha   float64 // BM25 权重
	GraphAlpha  float64 // 图谱权重
}

// Rerank 执行加权 RRF 重排
func (s *WeightedRRFStrategy) Rerank(ctx context.Context, results []*domainrag.Document, query string) ([]*domainrag.Document, error) {
	if s.K <= 0 {
		s.K = 60
	}
	if s.VectorAlpha <= 0 {
		s.VectorAlpha = 0.5
	}
	if s.BM25Alpha <= 0 {
		s.BM25Alpha = 0.3
	}
	if s.GraphAlpha <= 0 {
		s.GraphAlpha = 0.2
	}

	// 按匹配类型分组并排序
	vectorDocs := s.groupAndSortByType(results, "vector")
	bm25Docs := s.groupAndSortByType(results, "bm25")
	graphDocs := s.groupAndSortByType(results, "graph")

	// 计算 RRF 分数
	scoreMap := make(map[string]float64)
	docMap := make(map[string]*domainrag.Document)

	// 向量 RRF 分数
	totalAlpha := s.VectorAlpha + s.BM25Alpha + s.GraphAlpha
	for i, doc := range vectorDocs {
		key := doc.ChunkID
		docMap[key] = doc
		rrfScore := 1.0 / float64(s.K+i+1)
		scoreMap[key] += rrfScore * (s.VectorAlpha / totalAlpha)
	}

	// BM25 RRF 分数
	for i, doc := range bm25Docs {
		key := doc.ChunkID
		if _, exists := docMap[key]; !exists {
			docMap[key] = doc
		}
		rrfScore := 1.0 / float64(s.K+i+1)
		scoreMap[key] += rrfScore * (s.BM25Alpha / totalAlpha)
	}

	// 图谱 RRF 分数
	for i, doc := range graphDocs {
		key := doc.ChunkID
		if _, exists := docMap[key]; !exists {
			docMap[key] = doc
		}
		rrfScore := 1.0 / float64(s.K+i+1)
		scoreMap[key] += rrfScore * (s.GraphAlpha / totalAlpha)
	}

	// 应用融合分数
	for _, doc := range docMap {
		doc.Score = float32(scoreMap[doc.ChunkID])
	}

	// 排序
	fused := make([]*domainrag.Document, 0, len(docMap))
	for _, doc := range docMap {
		fused = append(fused, doc)
	}
	sort.Slice(fused, func(i, j int) bool {
		return fused[i].Score > fused[j].Score
	})

	return fused, nil
}

func (s *WeightedRRFStrategy) groupAndSortByType(docs []*domainrag.Document, matchType string) []*domainrag.Document {
	filtered := make([]*domainrag.Document, 0)
	for _, doc := range docs {
		if doc.MatchType == matchType {
			filtered = append(filtered, doc)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})
	return filtered
}

// ========================================
// 辅助函数
// ========================================

// normalizeScores 归一化分数到 [0, 1]
func normalizeScores(docs []*domainrag.Document) {
	if len(docs) == 0 {
		return
	}

	// 找到最大和最小分数
	maxScore := float32(0)
	minScore := float32(math.MaxFloat32)

	for _, doc := range docs {
		if doc.Score > maxScore {
			maxScore = doc.Score
		}
		if doc.Score < minScore {
			minScore = doc.Score
		}
	}

	// 归一化
	if maxScore > minScore {
		for _, doc := range docs {
			doc.Score = (doc.Score - minScore) / (maxScore - minScore)
		}
	} else if maxScore > 0 {
		// 所有分数相同
		for _, doc := range docs {
			doc.Score = 1.0
		}
	}
}

// ========================================
// ModelRerankStrategy 模型重排策略
// ========================================

// ModelRerankStrategy 模型重排策略
type ModelRerankStrategy struct {
	llmChat domainrag.LLMChat
}

// NewModelRerankStrategy 创建模型重排策略
func NewModelRerankStrategy(llmChat domainrag.LLMChat) *ModelRerankStrategy {
	return &ModelRerankStrategy{
		llmChat: llmChat,
	}
}

// Rerank 使用模型重排
func (s *ModelRerankStrategy) Rerank(ctx context.Context, results []*domainrag.Document, query string) ([]*domainrag.Document, error) {
	if s.llmChat == nil {
		// 降级到简单排序
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
		return results, nil
	}

	// 构建重排提示词
	prompt := s.buildRerankPrompt(query, results)

	// 调用 LLM
	messages := []domainrag.LLMMessage{
		{Role: "user", Content: prompt},
	}

	resp, err := s.llmChat.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2000,
	})
	if err != nil {
		// 降级到简单排序
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
		return results, nil
	}

	// 解析 LLM 响应并重排
	reranked := s.parseRerankResponse(results, resp.Content)

	return reranked, nil
}

// buildRerankPrompt 构建重排提示词
func (s *ModelRerankStrategy) buildRerankPrompt(query string, docs []*domainrag.Document) string {
	prompt := fmt.Sprintf("请根据以下查询，对检索结果进行重排序，只返回排序后的文档索引（从0开始），用逗号分隔：\n\n查询：%s\n\n文档：\n", query)

	for i, doc := range docs {
		prompt += fmt.Sprintf("[%d] %s\n", i, truncateContent(doc.Content, 200))
	}

	prompt += "\n\n请直接返回排序后的索引，如：3,1,0,2,4"
	return prompt
}

// parseRerankResponse 解析重排响应
func (s *ModelRerankStrategy) parseRerankResponse(docs []*domainrag.Document, response string) []*domainrag.Document {
	// 简单解析：提取数字
	indices := extractIndices(response)

	if len(indices) == 0 {
		// 解析失败，返回原始排序
		return docs
	}

	// 根据索引重新排序
	reranked := make([]*domainrag.Document, 0, len(docs))
	indexSet := make(map[int]bool)

	for _, idx := range indices {
		if idx >= 0 && idx < len(docs) && !indexSet[idx] {
			reranked = append(reranked, docs[idx])
			indexSet[idx] = true
		}
	}

	// 添加未包含的文档
	for i, doc := range docs {
		if !indexSet[i] {
			reranked = append(reranked, doc)
		}
	}

	return reranked
}

// truncateContent 截断内容
func truncateContent(content string, maxLen int) string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}

// extractIndices 提取索引
func extractIndices(s string) []int {
	indices := make([]int, 0)
	current := 0

	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			current = current*10 + int(ch-'0')
		} else if current > 0 {
			indices = append(indices, current)
			current = 0
		}
	}

	if current > 0 {
		indices = append(indices, current)
	}

	return indices
}

// ========================================
// 并行重排器
// ========================================

// ParallelReranker 并行重排器
type ParallelReranker struct {
	strategies []domainrag.RerankerStrategy
	workers    int
}

// NewParallelReranker 创建并行重排器
func NewParallelReranker(workers int) *ParallelReranker {
	return &ParallelReranker{
		workers: workers,
	}
}

// AddStrategy 添加策略
func (r *ParallelReranker) AddStrategy(strategy domainrag.RerankerStrategy) {
	r.strategies = append(r.strategies, strategy)
}

// Rerank 并行执行多个策略并融合结果
func (r *ParallelReranker) Rerank(ctx context.Context, results []*domainrag.Document, query string) ([]*domainrag.Document, error) {
	if len(r.strategies) == 0 {
		return results, nil
	}

	// 并发执行所有策略
	type rerankResult struct {
		results []*domainrag.Document
		err     error
	}

	resultChan := make(chan rerankResult, len(r.strategies))
	var wg sync.WaitGroup

	for _, strategy := range r.strategies {
		wg.Add(1)
		go func(s domainrag.RerankerStrategy) {
			defer safego.Recover("reranker-strategy")
			defer wg.Done()
			reranked, err := s.Rerank(ctx, results, query)
			resultChan <- rerankResult{results: reranked, err: err}
		}(strategy)
	}

	wg.Wait()
	close(resultChan)

	// 收集结果
	allResults := make([][]*domainrag.Document, 0)
	for result := range resultChan {
		if result.err == nil {
			allResults = append(allResults, result.results)
		}
	}

	// 融合多个策略的结果
	return r.fuseResults(allResults), nil
}

// fuseResults 融合多个策略的结果
func (r *ParallelReranker) fuseResults(allResults [][]*domainrag.Document) []*domainrag.Document {
	if len(allResults) == 0 {
		return nil
	}
	if len(allResults) == 1 {
		return allResults[0]
	}

	// 使用 Borda 融合
	scoreMap := make(map[string]float64)
	docMap := make(map[string]*domainrag.Document)

	for _, results := range allResults {
		for i, doc := range results {
			key := doc.ChunkID
			if _, exists := docMap[key]; !exists {
				docMap[key] = doc
			}
			// Borda 分数：倒数排名
			scoreMap[key] += float64(len(results) - i)
		}
	}

	// 按融合分数排序
	fused := make([]*domainrag.Document, 0, len(docMap))
	for _, doc := range docMap {
		doc.Score = float32(scoreMap[doc.ChunkID])
		fused = append(fused, doc)
	}
	sort.Slice(fused, func(i, j int) bool {
		return fused[i].Score > fused[j].Score
	})

	return fused
}
