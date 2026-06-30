// Package tool 提供知识库选择和检索工具
package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	domain_knowledge "link/internal/model/knowledge"
)

// ========================================
// 包级依赖管理
// ========================================

var (
	kbRepoInstance domain_knowledge.KnowledgeBaseRepository
	kbRepoOnce     sync.Once
)

// InitKnowledgeBaseTool 初始化知识库工具的依赖
func InitKnowledgeBaseTool(repo domain_knowledge.KnowledgeBaseRepository) {
	kbRepoOnce.Do(func() {
		kbRepoInstance = repo
	})
}

// ========================================
// kb_select - 知识库选择工具
// ========================================

// KbSelectRequest 知识库选择请求
type KbSelectRequest struct {
	// Query 用户查询内容
	Query string `json:"query" jsonschema:"required,description=用户的问题或查询内容"`

	// TopK 最多返回多少个知识库，默认3
	TopK int `json:"top_k" jsonschema:"description=最多返回知识库数量，默认3，范围1-5"`

	// MinScore 最低匹配分数，默认0.1
	MinScore float64 `json:"min_score" jsonschema:"description=最低匹配分数(0-1)，默认0.1"`
}

// KbSelectResult 知识库选择结果
type KbSelectResult struct {
	// Query 原始查询
	Query string `json:"query"`

	// SelectedKBs 选中的知识库列表
	SelectedKBs []SelectedKB `json:"selected_kbs"`

	// TotalKb 可用知识库总数
	TotalKb int `json:"total_kb"`

	// LatencyMs 查询耗时
	LatencyMs int64 `json:"latency_ms"`

	// SelectionLog 选择过程的日志
	SelectionLog string `json:"selection_log"`
}

// SelectedKB 选中的知识库
type SelectedKB struct {
	KnowledgeBaseID       string  `json:"kb_id"`       // 知识库ID
	Name       string  `json:"name"`        // 知识库名称
	MatchScore float64 `json:"match_score"` // 匹配分数(0-1)
	Reason     string  `json:"reason"`      // 选择原因
}

// NewKbSelectTool 创建知识库选择工具
func NewKbSelectTool() *TypedBaseTool[KbSelectRequest, KbSelectResult] {
	return NewTypedBaseTool(
		"kb_select",
		`智能选择知识库工具 - 根据查询内容自动匹配最相关的知识库。

这个工具负责：
1. 获取所有启用的知识库信息
2. 分析查询内容，提取关键词
3. 计算每个知识库与查询的相关度
4. 返回最相关的知识库ID列表

返回的 kb_id 可用于后续的 rag_query 检索。

适用场景：
- 不确定查询应该用哪个知识库
- 需要从多个知识库综合查询
- 需要查看知识库的匹配情况

使用流程：
1. 先调用 kb_select 获取相关知识库ID列表
2. 将返回的 kb_id 传给 rag_query 进行检索

参数说明：
- query: 查询内容（必需）
- top_k: 最多返回知识库数量（可选，默认3）
- min_score: 最低匹配分数（可选，默认0.1）`,
		kbSelect,
	)
}

// kbSelect 执行知识库选择
func kbSelect(ctx context.Context, req *KbSelectRequest) (*KbSelectResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if req.TopK <= 0 {
		req.TopK = 3
	}
	if req.TopK > 5 {
		req.TopK = 5
	}
	if req.MinScore < 0 {
		req.MinScore = 0.1
	}
	if req.MinScore > 1 {
		req.MinScore = 1.0
	}

	// 2. 检查依赖
	if kbRepoInstance == nil {
		return nil, fmt.Errorf("knowledge base repository not initialized")
	}

	// 3. 获取租户ID
	tenantID, err := getTenantIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取租户信息失败: %w", err)
	}

	// 4. 查询所有启用的知识库
	kbs, total, err := kbRepoInstance.FindByTenantID(ctx, tenantID, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("查询知识库列表失败: %w", err)
	}

	// 5. 计算知识库匹配度
	matchedKBs := calculateKBMatches(req.Query, kbs, req.MinScore)

	// 6. 按匹配度排序并取TopK
	sortMatchedKbs(matchedKBs)
	maxCount := req.TopK
	if len(matchedKBs) > maxCount {
		matchedKBs = matchedKBs[:maxCount]
	}

	// 7. 构建返回结果
	selectedKBs := make([]SelectedKB, 0, len(matchedKBs))
	var logBuilder strings.Builder

	logBuilder.WriteString(fmt.Sprintf("查询: %s\n", req.Query))
	logBuilder.WriteString(fmt.Sprintf("可用知识库: %d\n", total))
	logBuilder.WriteString(fmt.Sprintf("匹配阈值: %.2f\n", req.MinScore))

	if len(matchedKBs) == 0 {
		logBuilder.WriteString("未找到匹配的知识库")
	} else {
		logBuilder.WriteString(fmt.Sprintf("选中 %d 个知识库:\n", len(matchedKBs)))
		for i, kb := range matchedKBs {
			logBuilder.WriteString(fmt.Sprintf("  %d. %s (分数: %.2f, 原因: %s)\n",
				i+1, kb.kb.Name, kb.score, kb.reason))
			selectedKBs = append(selectedKBs, SelectedKB{
			 KnowledgeBaseID:       kb.kb.ID,
				Name:       kb.kb.Name,
				MatchScore: kb.score,
				Reason:     kb.reason,
			})
		}
	}

	return &KbSelectResult{
		Query:        req.Query,
		SelectedKBs:  selectedKBs,
		TotalKb:      int(total),
		LatencyMs:    time.Since(startTime).Milliseconds(),
		SelectionLog: logBuilder.String(),
	}, nil
}

// kbMatchScore 知识库匹配分数
type kbMatchScore struct {
	kb     *domain_knowledge.KnowledgeBase
	score  float64
	reason string
}

// calculateKBMatches 计算知识库匹配度
func calculateKBMatches(query string, kbs []*domain_knowledge.KnowledgeBase, minScore float64) []kbMatchScore {
	// 提取查询关键词
	keywords := extractKeywordsForKB(query)

	var matched []kbMatchScore

	for _, kb := range kbs {
		// 跳过已删除和禁用的知识库
		if kb.DeletedAt != nil || kb.Status == 0 {
			continue
		}

		// 计算匹配分数和原因
		score, reason := calculateKBMatchScoreDetailed(query, keywords, kb)

		// 过滤低于阈值的知识库
		if score >= minScore {
			matched = append(matched, kbMatchScore{
				kb:     kb,
				score:  score,
				reason: reason,
			})
		}
	}

	return matched
}

// calculateKBMatchScoreDetailed 详细计算知识库匹配分数
func calculateKBMatchScoreDetailed(query string, keywords []string, kb *domain_knowledge.KnowledgeBase) (float64, string) {
	score := 0.0
	reasons := []string{}

	kbName := strings.ToLower(kb.Name)
	queryLower := strings.ToLower(query)

	// 1. 名称完全匹配
	if kbName == queryLower {
		score += 1.0
		reasons = append(reasons, "名称完全匹配")
	} else if strings.Contains(kbName, queryLower) {
		score += 0.5
		reasons = append(reasons, "名称包含查询词")
	} else if strings.Contains(queryLower, kbName) {
		score += 0.3
		reasons = append(reasons, "查询词包含知识库名")
	}

	// 2. 描述匹配
	if kb.Description != "" {
		desc := strings.ToLower(kb.Description)
		matchedInDesc := false
		for _, keyword := range keywords {
			if strings.Contains(desc, keyword) {
				score += 0.2
				matchedInDesc = true
			}
		}
		if matchedInDesc {
			reasons = append(reasons, "描述包含关键词")
		}
	}

	// 3. 关键词匹配知识库名
	for _, keyword := range keywords {
		if strings.Contains(kbName, keyword) {
			score += 0.3
			reasons = append(reasons, fmt.Sprintf("关键词'%s'匹配名称", keyword))
		}
	}

	// 构建原因字符串
	reasonStr := strings.Join(reasons, "；")
	if reasonStr == "" {
		reasonStr = "基础匹配"
	}

	return score, reasonStr
}

// extractKeywordsForKB 从查询中提取关键词
func extractKeywordsForKB(query string) []string {
	query = strings.ToLower(query)

	// 移除停用词
	stopWords := map[string]bool{
		"的": true, "了": true, "是": true, "在": true, "有": true,
		"和": true, "与": true, "或": true, "但": true, "而": true,
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"什么": true, "怎么": true, "如何": true, "哪些": true,
	}

	// 分割并过滤
	words := strings.Fields(query)
	var keywords []string
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:，。！？；：、")
		if len(word) > 1 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// sortMatchedKbs 按匹配分数降序排序
func sortMatchedKbs(kbs []kbMatchScore) {
	sort.Slice(kbs, func(i, j int) bool {
		return kbs[i].score > kbs[j].score
	})
}

// getTenantIDFromContext 从上下文获取租户ID
func getTenantIDFromContext(ctx context.Context) (int64, error) {
	// 尝试从context获取tenant_id（由middleware设置）
	if tid := ctx.Value("tenant_id"); tid != nil {
		if tidInt, ok := tid.(int64); ok {
			return tidInt, nil
		}
	}
	// 默认返回租户ID 1（用于开发/测试）
	return 1, nil
}

// ========================================
// rag_query_multi - 支持多知识库的检索工具
// ========================================

// RAGQueryMultiRequest RAG 多知识库检索请求
type RAGQueryMultiRequest struct {
	// Query 查询内容
	Query string `json:"query" jsonschema:"required,description=用户的问题或查询内容"`

	// KnowledgeBaseIDs 知识库ID列表，支持多个知识库并发检索
	KnowledgeBaseIDs []string `json:"kb_ids" jsonschema:"required,description=知识库ID列表，支持多个知识库并发检索"`

	// TopK 每个知识库返回的片段数，默认5
	TopK int `json:"top_k" jsonschema:"description=每个知识库返回的片段数，默认5，范围1-20"`

	// RetrievalMode 检索模式：vector/bm25/hybrid
	RetrievalMode string `json:"retrieval_mode" jsonschema:"description=检索模式：vector/bm25/hybrid，默认hybrid"`

	// MinScore 最小相似度阈值，默认0.5
	MinScore float64 `json:"min_score" jsonschema:"description=最小相似度阈值(0-1)，默认0.5"`
}

// RAGQueryMultiResult RAG 多知识库检索结果
type RAGQueryMultiResult struct {
	// Query 原始查询
	Query string `json:"query"`

	// KbResults 各知识库的检索结果
	KbResults []KbResult `json:"kb_results"`

	// TotalChunks 总片段数
	TotalChunks int `json:"total_chunks"`

	// TopChunks 合并后的top片段
	TopChunks []DocumentChunk `json:"top_chunks"`

	// Summary 检索摘要
	Summary string `json:"summary"`

	// LatencyMs 查询耗时
	LatencyMs int64 `json:"latency_ms"`
}

// KbResult 单个知识库的检索结果
type KbResult struct {
	KnowledgeBaseID   string          `json:"kb_id"`
	Name   string          `json:"name"`
	Chunks []DocumentChunk `json:"chunks"`
	Count  int             `json:"count"`
}

// NewRAGQueryMultiTool 创建多知识库检索工具
func NewRAGQueryMultiTool() *TypedBaseTool[RAGQueryMultiRequest, RAGQueryMultiResult] {
	return NewTypedBaseTool(
		"rag_query_multi",
		`多知识库并发检索工具 - 同时从多个知识库检索信息并合并结果。

这是 rag_query 的多知识库版本，支持：
1. 同时查询多个知识库
2. 并发执行，提高检索速度
3. 自动合并和去重结果
4. 按相关度排序返回

适用场景：
- 需要从多个知识库综合查询
- 知识库内容有重叠，需要去重合并
- 需要快速检索多个知识库

使用流程：
1. 先调用 kb_select 获取相关知识库ID
2. 将返回的 kb_id 列表传给此工具
3. 工具会并发查询所有指定的知识库
4. 返回合并排序后的结果

参数说明：
- query: 查询内容（必需）
- kb_ids: 知识库ID列表，如 ["1", "2", "3"]（必需）
- top_k: 每个知识库返回的片段数（可选，默认5）
- retrieval_mode: 检索模式（可选，默认hybrid）
- min_score: 最小相似度阈值（可选，默认0.5）`,
		ragQueryMulti,
	)
}

// ragQueryMulti 执行多知识库检索
func ragQueryMulti(ctx context.Context, req *RAGQueryMultiRequest) (*RAGQueryMultiResult, error) {
	startTime := time.Now()

	// 1. 参数验证
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if len(req.KnowledgeBaseIDs) == 0 {
		return nil, fmt.Errorf("kb_ids cannot be empty")
	}

	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.TopK > 20 {
		req.TopK = 20
	}

	if req.MinScore <= 0 {
		req.MinScore = 0.5
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

	// 2. 检查 RAG 服务
	if ragService == nil {
		return mockRAGQueryMulti(ctx, req, startTime)
	}

	// 3. 并发查询所有知识库
	type kbResult struct {
		kbID   string
		name   string
		chunks []DocumentChunk
		err    error
	}

	resultChan := make(chan kbResult, len(req.KnowledgeBaseIDs))

	for _, kbID := range req.KnowledgeBaseIDs {
		go func(id string) {
			// 获取知识库名称
			kbName := getKBNameByID(ctx, id)

			// 执行检索 - 注意：这里使用字符串ID
			// 如果RAG服务需要int64，需要在服务层做转换
			chunks := queryKBForChunks(ctx, id, req.Query, req.TopK, req.RetrievalMode, req.MinScore)

			resultChan <- kbResult{
				kbID:   id,
				name:   kbName,
				chunks: chunks,
			}
		}(kbID)
	}

	// 4. 收集结果
	kbResults := make([]KbResult, 0, len(req.KnowledgeBaseIDs))
	allChunks := make([]DocumentChunk, 0)

	for i := 0; i < len(req.KnowledgeBaseIDs); i++ {
		result := <-resultChan
		if result.err != nil {
			// 记录错误但继续
			continue
		}
		kbResults = append(kbResults, KbResult{
		 KnowledgeBaseID:   result.kbID,
			Name:   result.name,
			Chunks: result.chunks,
			Count:  len(result.chunks),
		})
		allChunks = append(allChunks, result.chunks...)
	}

	// 5. 合并去重并排序
	topChunks := mergeAndRankChunksMulti(allChunks, req.TopK*2)

	// 6. 生成摘要
	summary := generateMultiKBSummary(req.Query, kbResults, topChunks)

	return &RAGQueryMultiResult{
		Query:       req.Query,
		KbResults:   kbResults,
		TotalChunks: len(allChunks),
		TopChunks:   topChunks,
		Summary:     summary,
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}, nil
}

// getKBNameByID 根据ID获取知识库名称
func getKBNameByID(ctx context.Context, kbID string) string {
	if kbRepoInstance == nil {
		return fmt.Sprintf("KB-%s", kbID)
	}

	kb, err := kbRepoInstance.FindByID(ctx, kbID)
	if err != nil || kb == nil {
		return fmt.Sprintf("KB-%s", kbID)
	}
	return kb.Name
}

// queryKBForChunks 查询单个知识库的片段
func queryKBForChunks(ctx context.Context, kbID string, query string, topK int, mode string, minScore float64) []DocumentChunk {
	if ragService == nil {
		// 返回模拟数据
		return generateMockChunksForKB(query, kbID, topK)
	}

	// 调用RAG服务
	req := &RAGQueryRequest{
		Query:         query,
	 KnowledgeBaseID:          kbID,
		TopK:          topK,
		RetrievalMode: mode,
		MinScore:      minScore,
	}

	result, err := ragService.Query(ctx, req)
	if err != nil {
		return []DocumentChunk{}
	}

	return result.Chunks
}

// mergeAndRankChunksMulti 合并和排序片段（多知识库版本）
func mergeAndRankChunksMulti(chunks []DocumentChunk, topN int) []DocumentChunk {
	if len(chunks) == 0 {
		return chunks
	}

	// 按相似度排序
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].Score > chunks[j].Score
	})

	// 去重（基于内容）
	uniqueChunks := make([]DocumentChunk, 0)
	seen := make(map[string]bool)

	for _, chunk := range chunks {
		// 使用内容的前100个字符作为去重标识
		key := chunk.Content
		if len(key) > 100 {
			key = key[:100]
		}
		key = strings.TrimSpace(key)

		if !seen[key] {
			seen[key] = true
			uniqueChunks = append(uniqueChunks, chunk)
		}

		// 达到目标数量后停止
		if len(uniqueChunks) >= topN {
			break
		}
	}

	return uniqueChunks
}

// generateMultiKBSummary 生成多知识库检索摘要
func generateMultiKBSummary(query string, kbResults []KbResult, chunks []DocumentChunk) string {
	var summary strings.Builder

	summary.WriteString(fmt.Sprintf("## 多知识库检索摘要\n\n"))
	summary.WriteString(fmt.Sprintf("**查询**: %s\n\n", query))

	// 各知识库检索结果
	summary.WriteString("**各知识库检索结果**:\n\n")
	for _, result := range kbResults {
		summary.WriteString(fmt.Sprintf("- **%s** (ID: %s): %d 个片段\n",
			result.Name, result.KnowledgeBaseID, result.Count))
	}

	summary.WriteString(fmt.Sprintf("\n**总计**: %d 个片段 (去重后)\n\n", len(chunks)))

	// 关键内容预览
	if len(chunks) > 0 {
		summary.WriteString("**关键内容预览**:\n\n")
		for i, chunk := range chunks {
			if i >= 3 {
				break
			}
			preview := chunk.Content
			if len(preview) > 150 {
				preview = preview[:150] + "..."
			}
			summary.WriteString(fmt.Sprintf("%d. [%.2f] %s\n", i+1, chunk.Score, preview))
		}
	}

	return summary.String()
}

// mockRAGQueryMulti 模拟多知识库检索
func mockRAGQueryMulti(ctx context.Context, req *RAGQueryMultiRequest, startTime time.Time) (*RAGQueryMultiResult, error) {
	// 生成模拟结果
	kbResults := make([]KbResult, 0, len(req.KnowledgeBaseIDs))
	allChunks := make([]DocumentChunk, 0)

	for _, kbID := range req.KnowledgeBaseIDs {
		// 为每个知识库生成模拟片段
		chunks := generateMockChunksForKB(req.Query, kbID, 3)
		kbResults = append(kbResults, KbResult{
		 KnowledgeBaseID:   kbID,
			Name:   fmt.Sprintf("知识库-%s", kbID),
			Chunks: chunks,
			Count:  len(chunks),
		})
		allChunks = append(allChunks, chunks...)
	}

	// 排序并去重
	topChunks := mergeAndRankChunksMulti(allChunks, req.TopK*2)

	return &RAGQueryMultiResult{
		Query:       req.Query,
		KbResults:   kbResults,
		TotalChunks: len(allChunks),
		TopChunks:   topChunks,
		Summary:     fmt.Sprintf("模拟检索了 %d 个知识库，共 %d 个片段", len(req.KnowledgeBaseIDs), len(topChunks)),
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}, nil
}

// generateMockChunksForKB 生成单个知识库的模拟片段
func generateMockChunksForKB(query string, kbID string, count int) []DocumentChunk {
	chunks := make([]DocumentChunk, 0, count)
	for i := 0; i < count && i < 3; i++ {
		score := 0.95 - float64(i)*0.1
		chunks = append(chunks, DocumentChunk{
			Content:    fmt.Sprintf("[知识库%s] 与查询「%s」相关的文档内容片段 #%d", kbID, query, i+1),
			Score:      score,
			Source:     fmt.Sprintf("知识库-%s", kbID),
			ChunkIndex: i,
		})
	}
	return chunks
}

// ========================================
// kb_list - 知识库列表工具（保留）
// ========================================

// KbListRequest 知识库列表请求
type KbListRequest struct {
	Status string `json:"status" jsonschema:"description=状态筛选：all(全部)/enabled(启用)/disabled(禁用)，默认all"`
}

// KbListResult 知识库列表结果
type KbListResult struct {
	KnowledgeBases []KbInfo `json:"knowledge_bases"`
	Count          int      `json:"count"`
	LatencyMs      int64    `json:"latency_ms"`
}

// KbInfo 知识库信息
type KbInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DocumentCount int64  `json:"document_count"`
	ChunkCount    int64  `json:"chunk_count"`
	Status        string `json:"status"`
}

// NewKbListTool 创建知识库列表工具
func NewKbListTool() *TypedBaseTool[KbListRequest, KbListResult] {
	return NewTypedBaseTool(
		"kb_list",
		`获取当前租户下的所有知识库列表。

适用场景：
- 用户询问"有哪些知识库"
- 查看知识库基本信息
- 手动选择知识库进行查询

参数：
- status: 状态筛选，可选值: all/enabled/disabled，默认 all`,
		kbList,
	)
}

// kbList 执行知识库列表查询
func kbList(ctx context.Context, req *KbListRequest) (*KbListResult, error) {
	startTime := time.Now()

	status := req.Status
	if status == "" {
		status = "all"
	}

	if kbRepoInstance == nil {
		return nil, fmt.Errorf("knowledge base repository not initialized")
	}

	tenantID, err := getTenantIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取租户信息失败: %w", err)
	}

	kbs, _, err := kbRepoInstance.FindByTenantID(ctx, tenantID, 1, 100)
	if err != nil {
		return nil, fmt.Errorf("查询知识库列表失败: %w", err)
	}

	// 转换格式
	var results []KbInfo
	for _, kb := range kbs {
		if kb.DeletedAt != nil {
			continue
		}

		kbStatus := "enabled"
		if kb.Status == 0 {
			kbStatus = "disabled"
		}

		if status != "all" && status != kbStatus {
			continue
		}

		results = append(results, KbInfo{
			ID:            kb.ID,
			Name:          kb.Name,
			Description:   kb.Description,
			DocumentCount: int64(kb.DocumentCount),
			ChunkCount:    int64(kb.ChunkCount),
			Status:        kbStatus,
		})
	}

	return &KbListResult{
		KnowledgeBases: results,
		Count:          len(results),
		LatencyMs:      time.Since(startTime).Milliseconds(),
	}, nil
}
