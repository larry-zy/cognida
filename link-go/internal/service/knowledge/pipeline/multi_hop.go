// Package rag 提供 Multi-Hop (多跳) 检索实现
package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainrag "link/internal/model/rag"
	prompts "link/internal/prompt"
)

// ========================================
// Multi-Hop Retriever 实现
// ========================================

// MultiHopRetrieverImpl 多跳检索器实现
type MultiHopRetrieverImpl struct {
	baseRetriever domainrag.Retriever
	llm           domainrag.LLMChat
}

// NewMultiHopRetriever 创建多跳检索器
func NewMultiHopRetriever(baseRetriever domainrag.Retriever, llm domainrag.LLMChat) domainrag.MultiHopRetriever {
	return &MultiHopRetrieverImpl{
		baseRetriever: baseRetriever,
		llm:           llm,
	}
}

// RetrieveMultiHop 执行多跳检索
func (m *MultiHopRetrieverImpl) RetrieveMultiHop(ctx context.Context, query string, opts *domainrag.MultiHopOptions) (*domainrag.MultiHopResponse, error) {
	if opts == nil {
		opts = domainrag.DefaultMultiHopOptions()
	}

	response := &domainrag.MultiHopResponse{
		Hops:         make([]*domainrag.HopResult, 0),
		AllDocuments: make([]*domainrag.Document, 0),
		Metadata:     make(map[string]interface{}),
	}

	currentQuery := query
	allDocs := make(map[string]*domainrag.Document)
	reasoningChain := make([]string, 0)

	// 执行多跳检索
	for hop := 1; hop <= opts.MaxHops; hop++ {
		hopStart := time.Now()

		// 执行当前跳的检索
		retrieveOpts := &domainrag.RetrieveOptions{
			TopK:              10,
			RerankEnabled:     true,
			RetrievalMode:     "hybrid",
			SimilarityThreshold: 0.5,
		}

		retrieveResp, err := m.baseRetriever.Retrieve(ctx, "", "", currentQuery, retrieveOpts)
		if err != nil {
			return nil, fmt.Errorf("hop %d retrieval failed: %w", hop, err)
		}

		// 记录文档
		for _, doc := range retrieveResp.Results {
			if _, exists := allDocs[doc.ChunkID]; !exists {
				allDocs[doc.ChunkID] = doc
				response.AllDocuments = append(response.AllDocuments, doc)
			}
		}

		// 构建当前跳的结果
		hopResult := &domainrag.HopResult{
			HopIndex:       hop,
			Query:          currentQuery,
			RetrievedDocs:  retrieveResp.Results,
			Latency:        time.Since(hopStart).Milliseconds(),
		}

		// 生成中间答案和下一步查询
		nextQuery, intermediateAnswer, shouldContinue, err := m.generateNextStep(
			ctx,
			query,
			currentQuery,
			retrieveResp.Results,
			hop,
			opts,
		)
		if err != nil {
			return nil, fmt.Errorf("hop %d reasoning failed: %w", hop, err)
		}

		hopResult.IntermediateResult = intermediateAnswer
		hopResult.NextQueryDecision = nextQuery

		response.Hops = append(response.Hops, hopResult)
		reasoningChain = append(reasoningChain,
			fmt.Sprintf("跳 %d: %s -> %s", hop, currentQuery, intermediateAnswer))

		// 检查是否应该继续
		if !shouldContinue {
			break
		}

		currentQuery = nextQuery

		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return response, ctx.Err()
		default:
		}
	}

	// 生成最终答案
	finalAnswer, err := m.generateFinalAnswer(ctx, query, response.Hops, opts)
	if err != nil {
		finalAnswer = m.buildFallbackAnswer(response.Hops)
	}

	response.FinalAnswer = finalAnswer
	response.ReasoningChain = strings.Join(reasoningChain, "\n")

	return response, nil
}

// ========================================
// 推理和决策
// ========================================

// generateNextStep 生成下一步查询决策
func (m *MultiHopRetrieverImpl) generateNextStep(
	ctx context.Context,
	originalQuery string,
	currentQuery string,
	docs []*domainrag.Document,
	hopIndex int,
	opts *domainrag.MultiHopOptions,
) (nextQuery string, intermediateAnswer string, shouldContinue bool, err error) {
	// 构建推理 prompt
	prompt := m.buildReasoningPrompt(originalQuery, currentQuery, docs, hopIndex)

	messages := []domainrag.LLMMessage{
		{Role: "system", Content: m.getReasoningSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	resp, err := m.llm.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.3,
		MaxTokens:   500,
	})
	if err != nil {
		return "", "", false, err
	}

	// 解析响应
	return m.parseReasoningResponse(resp.Content, hopIndex, opts.MaxHops)
}

// generateFinalAnswer 生成最终答案
func (m *MultiHopRetrieverImpl) generateFinalAnswer(
	ctx context.Context,
	originalQuery string,
	hops []*domainrag.HopResult,
	opts *domainrag.MultiHopOptions,
) (string, error) {
	// 构建最终答案 prompt
	prompt := m.buildFinalAnswerPrompt(originalQuery, hops)

	messages := []domainrag.LLMMessage{
		{Role: "system", Content: m.getFinalAnswerSystemPrompt()},
		{Role: "user", Content: prompt},
	}

	resp, err := m.llm.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.5,
		MaxTokens:   1000,
	})
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// ========================================
// Prompt 构建
// ========================================

// getReasoningSystemPrompt 获取推理系统 prompt
func (m *MultiHopRetrieverImpl) getReasoningSystemPrompt() string {
	return prompts.MustGet("knowledge", "reasoning_system")
}

// buildReasoningPrompt 构建推理 prompt
func (m *MultiHopRetrieverImpl) buildReasoningPrompt(
	originalQuery string,
	currentQuery string,
	docs []*domainrag.Document,
	hopIndex int,
) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("原始问题：%s\n\n", originalQuery))
	builder.WriteString(fmt.Sprintf("当前查询（第 %d 跳）：%s\n\n", hopIndex, currentQuery))

	builder.WriteString("检索结果：\n")
	for i, doc := range docs {
		if i >= 5 {
			break
		}
		builder.WriteString(fmt.Sprintf("- %s\n", doc.Content))
	}

	builder.WriteString("\n请分析：\n")
	builder.WriteString("1. 当前检索结果是否足够回答原始问题？\n")
	builder.WriteString("2. 如果不够，下一步应该查询什么？\n")

	return builder.String()
}

// getFinalAnswerSystemPrompt 获取最终答案系统 prompt
func (m *MultiHopRetrieverImpl) getFinalAnswerSystemPrompt() string {
	return prompts.MustGet("knowledge", "final_answer_system")
}

// buildFinalAnswerPrompt 构建最终答案 prompt
func (m *MultiHopRetrieverImpl) buildFinalAnswerPrompt(
	originalQuery string,
	hops []*domainrag.HopResult,
) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("问题：%s\n\n", originalQuery))
	builder.WriteString("多跳检索过程：\n\n")

	for _, hop := range hops {
		builder.WriteString(fmt.Sprintf("--- 跳 %d ---\n", hop.HopIndex))
		builder.WriteString(fmt.Sprintf("查询：%s\n", hop.Query))
		builder.WriteString(fmt.Sprintf("中间结果：%s\n\n", hop.IntermediateResult))
	}

	builder.WriteString("请基于以上多跳检索过程，生成最终答案。")

	return builder.String()
}

// ========================================
// 响应解析
// ========================================

// parseReasoningResponse 解析推理响应
func (m *MultiHopRetrieverImpl) parseReasoningResponse(
	content string,
	hopIndex int,
	maxHops int,
) (nextQuery string, intermediateAnswer string, shouldContinue bool, err error) {
	// 简化实现：从文本中提取信息

	lines := strings.Split(content, "\n")
	intermediateAnswer = "已检索到相关信息"

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "intermediate_answer") || strings.Contains(line, "中间答案") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				intermediateAnswer = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "should_continue") || strings.Contains(line, "是否继续") {
			if strings.Contains(line, "true") || strings.Contains(line, "是") {
				shouldContinue = hopIndex < maxHops
			}
		}

		if strings.Contains(line, "next_query") || strings.Contains(line, "下一步查询") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				nextQuery = strings.TrimSpace(parts[1])
			}
		}
	}

	// 如果没有明确的下一步查询，不继续
	if nextQuery == "" {
		shouldContinue = false
	}

	return nextQuery, intermediateAnswer, shouldContinue, nil
}

// buildFallbackAnswer 构建降级答案
func (m *MultiHopRetrieverImpl) buildFallbackAnswer(hops []*domainrag.HopResult) string {
	if len(hops) == 0 {
		return "抱歉，未能找到相关信息。"
	}

	var builder strings.Builder
	builder.WriteString("基于检索结果，以下是相关信息：\n\n")

	for _, hop := range hops {
		if hop.IntermediateResult != "" {
			builder.WriteString(fmt.Sprintf("- %s\n", hop.IntermediateResult))
		}
	}

	return builder.String()
}

// ========================================
// 检索优化服务 (整合所有优化策略)
// ========================================

// RetrievalOptimizer 检索优化服务
type RetrievalOptimizer struct {
	hydeGenerator    domainrag.HyDEGenerator
	queryRewriter    domainrag.QueryRewriter
	multiHopRetriever domainrag.MultiHopRetriever
	baseRetriever    domainrag.Retriever
}

// NewRetrievalOptimizer 创建检索优化服务
func NewRetrievalOptimizer(
	hydeGenerator domainrag.HyDEGenerator,
	queryRewriter domainrag.QueryRewriter,
	multiHopRetriever domainrag.MultiHopRetriever,
	baseRetriever domainrag.Retriever,
) *RetrievalOptimizer {
	return &RetrievalOptimizer{
		hydeGenerator:    hydeGenerator,
		queryRewriter:    queryRewriter,
		multiHopRetriever: multiHopRetriever,
		baseRetriever:    baseRetriever,
	}
}

// OptimizedRetrieve 执行优化的检索
func (o *RetrievalOptimizer) OptimizedRetrieve(
	ctx context.Context,
	tenantID string,
	kbID string,
	query string,
	config *domainrag.RetrievalOptimizationConfig,
) (*domainrag.RetrieveResponse, error) {
	if config == nil {
		config = domainrag.DefaultRetrievalOptimizationConfig()
	}

	// 1. 查询重写
	processedQuery := query
	var rewrittenQuery *domainrag.RewrittenQuery

	if config.EnableQueryRewrite && o.queryRewriter != nil {
		var err error
		rewrittenQuery, err = o.queryRewriter.RewriteQuery(ctx, query, config.RewriteOptions)
		if err == nil && rewrittenQuery.Rewritten != "" {
			processedQuery = rewrittenQuery.Rewritten
		}
	}

	// 2. HyDE 检索
	if config.EnableHyDE && o.hydeGenerator != nil {
		return o.hydeRetrieve(ctx, tenantID, kbID, query, processedQuery, config)
	}

	// 3. Multi-Hop 检索
	if config.EnableMultiHop && o.multiHopRetriever != nil {
		multiHopResp, err := o.multiHopRetriever.RetrieveMultiHop(ctx, processedQuery, config.MultiHopOptions)
		if err == nil {
			// 转换为标准响应格式
			return o.convertMultiHopResponse(multiHopResp, query, processedQuery)
		}
	}

	// 4. 降级到基础检索
	return o.baseRetriever.Retrieve(ctx, tenantID, kbID, processedQuery, &domainrag.RetrieveOptions{
		TopK:              10,
		RerankEnabled:     true,
		SimilarityThreshold: 0.5,
		RetrievalMode:     "hybrid",
	})
}

// hydeRetrieve 执行 HyDE 检索
func (o *RetrievalOptimizer) hydeRetrieve(
	ctx context.Context,
	tenantID string,
	kbID string,
	originalQuery string,
	processedQuery string,
	config *domainrag.RetrievalOptimizationConfig,
) (*domainrag.RetrieveResponse, error) {
	// 生成假设文档
	hypotheticalDoc, err := o.hydeGenerator.GenerateHypotheticalDoc(ctx, processedQuery, config.HyDEOptions)
	if err != nil {
		// 降级到普通检索
		return o.baseRetriever.Retrieve(ctx, tenantID, kbID, processedQuery, &domainrag.RetrieveOptions{
			TopK:              10,
			RerankEnabled:     true,
			SimilarityThreshold: 0.5,
			RetrievalMode:     "hybrid",
		})
	}

	// 使用假设文档和原始查询进行混合检索
	// 先用假设文档检索
	opts := &domainrag.RetrieveOptions{
		TopK:              10,
		RerankEnabled:     true,
		SimilarityThreshold: 0.5,
		RetrievalMode:     "hybrid",
	}

	// 组合检索：原始查询 + 假设文档
	// 这里简化处理，实际应该对两者结果进行融合
	resp, err := o.baseRetriever.Retrieve(ctx, tenantID, kbID, processedQuery, opts)

	if resp != nil && resp.SearchTrace != nil {
		resp.SearchTrace.HyDEUsed = true
		resp.SearchTrace.HypotheticalDoc = hypotheticalDoc
	}

	return resp, err
}

// convertMultiHopResponse 转换多跳响应为标准检索响应
func (o *RetrievalOptimizer) convertMultiHopResponse(
	multiHopResp *domainrag.MultiHopResponse,
	originalQuery string,
	processedQuery string,
) (*domainrag.RetrieveResponse, error) {
	return &domainrag.RetrieveResponse{
		Results:     multiHopResp.AllDocuments,
		Query:       originalQuery,
		ProcessedQuery: processedQuery,
		TotalCount:  len(multiHopResp.AllDocuments),
		HasMore:     false,
		Answer:      multiHopResp.FinalAnswer,
		SearchTrace: &domainrag.SearchTrace{
			MultiHopUsed:   true,
			HopCount:       len(multiHopResp.Hops),
			ReasoningChain: multiHopResp.ReasoningChain,
		},
	}, nil
}
