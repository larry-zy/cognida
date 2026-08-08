// Package rag 提供 RAG 检索优化服务
package knowledge

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"

	domainrag "link/internal/model/rag"
	"link/internal/service/knowledge/pipeline"
)

// ========================================
// Retrieval Optimization Service 检索优化服务
// ========================================

// Optimizer 检索优化服务
type Optimizer struct {
	hydeGenerator      domainrag.HyDEGenerator
	queryRewriter      domainrag.QueryRewriter
	multiHopRetriever  domainrag.MultiHopRetriever
	baseRetriever      domainrag.Retriever
	optimizer          *rag.RetrievalOptimizer
}

// NewOptimizer 创建检索优化服务
func NewOptimizer(
	llm model.ChatModel,
	baseRetriever domainrag.Retriever,
) *Optimizer {
	// 创建各个优化组件
	hydeGen := rag.NewHyDEGenerator(llm)
	queryRewriter := rag.NewQueryRewriter(llm)
	multiHop := rag.NewMultiHopRetriever(baseRetriever, nil) // LLM chat 传入 nil 表示不使用

	optimizer := rag.NewRetrievalOptimizer(hydeGen, queryRewriter, multiHop, baseRetriever)

	return &Optimizer{
		hydeGenerator:     hydeGen,
		queryRewriter:     queryRewriter,
		multiHopRetriever: multiHop,
		baseRetriever:     baseRetriever,
		optimizer:         optimizer,
	}
}

// ========================================
// HyDE 检索
// ========================================

// HyDERetrieveRequest HyDE 检索请求
type HyDERetrieveRequest struct {
	TenantID         string
	KnowledgeBaseID  string
	Query            string
	EnableMultiDoc   bool
	DocCount         int
	Temperature      float64
	MaxTokens        int
	Domain           string
}

// HyDERetrieveResponse HyDE 检索响应
type HyDERetrieveResponse struct {
	Documents       []*domainrag.Document
	HypotheticalDoc string
	Query           string
	TotalCount      int
	Latency         int64
	UsedHyDE        bool
}

// HyDERetrieve 执行 HyDE 检索
func (s *Optimizer) HyDERetrieve(ctx context.Context, req *HyDERetrieveRequest) (*HyDERetrieveResponse, error) {
	opts := &domainrag.HyDEOptions{
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Domain:      req.Domain,
	}

	// 生成假设文档
	var hypotheticalDoc string
	var docs []*domainrag.Document
	var err error

	if req.EnableMultiDoc && req.DocCount > 1 {
		// 多文档 HyDE
		// 注意：这里必须用 `=` 复用外层 err，不能 `:=` 另建局部 err——否则
		// GenerateMultiple / retrieveWithMultipleDocs 的错误只落到影子变量里，
		// 外层 err 恒为 nil，下面的「降级到普通检索」判定永远失效。
		var docsList []string
		docsList, err = s.hydeGenerator.GenerateMultiple(ctx, req.Query, req.DocCount, opts)
		if err == nil && len(docsList) > 0 {
			hypotheticalDoc = docsList[0]
			// 使用多个假设文档进行检索
			docs, err = s.retrieveWithMultipleDocs(ctx, req, docsList)
		}
	} else {
		// 单文档 HyDE
		hypotheticalDoc, err = s.hydeGenerator.GenerateHypotheticalDoc(ctx, req.Query, opts)
		if err == nil {
			// 使用假设文档进行检索
			docs, err = s.retrieveWithHyDE(ctx, req, hypotheticalDoc)
		}
	}

	// 降级到普通检索
	if err != nil || len(docs) == 0 {
		retrieveResp, err := s.baseRetriever.Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, req.Query, &domainrag.RetrieveOptions{
			TopK:                10,
			RerankEnabled:       true,
			SimilarityThreshold: 0.5,
			RetrievalMode:       "hybrid",
		})
		if err != nil {
			return nil, err
		}
		docs = retrieveResp.Results
		hypotheticalDoc = ""
	}

	return &HyDERetrieveResponse{
		Documents:       docs,
		HypotheticalDoc: hypotheticalDoc,
		Query:           req.Query,
		TotalCount:      len(docs),
		UsedHyDE:        hypotheticalDoc != "",
	}, nil
}

// retrieveWithHyDE 使用假设文档检索
func (s *Optimizer) retrieveWithHyDE(ctx context.Context, req *HyDERetrieveRequest, hypotheticalDoc string) ([]*domainrag.Document, error) {
	// 组合原始查询和假设文档
	// 这里简化处理，实际应该对两者分别检索后融合
	retrieveResp, err := s.baseRetriever.Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, hypotheticalDoc, &domainrag.RetrieveOptions{
		TopK:                10,
		RerankEnabled:       true,
		SimilarityThreshold: 0.5,
		RetrievalMode:       "hybrid",
	})
	if err != nil {
		return nil, err
	}
	return retrieveResp.Results, nil
}

// retrieveWithMultipleDocs 使用多个假设文档检索
func (s *Optimizer) retrieveWithMultipleDocs(ctx context.Context, req *HyDERetrieveRequest, docsList []string) ([]*domainrag.Document, error) {
	// 对每个假设文档进行检索，然后合并结果
	allDocs := make(map[string]*domainrag.Document)

	for _, doc := range docsList {
		retrieveResp, err := s.baseRetriever.Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, doc, &domainrag.RetrieveOptions{
			TopK:                5,
			RerankEnabled:       false,
			SimilarityThreshold: 0.5,
			RetrievalMode:       "hybrid",
		})
		if err == nil {
			for _, d := range retrieveResp.Results {
				if _, exists := allDocs[d.ChunkID]; !exists {
					allDocs[d.ChunkID] = d
				}
			}
		}
	}

	// 转换为切片
	result := make([]*domainrag.Document, 0, len(allDocs))
	for _, doc := range allDocs {
		result = append(result, doc)
	}

	return result, nil
}

// ========================================
// Query Rewrite 查询重写
// ========================================

// QueryRewriteRequest 查询重写请求
type QueryRewriteRequest struct {
	Query               string
	ConversationHistory []string
	Domain              string
	EnableExpansion     bool
	ExpansionCount      int
	EnableDecomposition bool
}

// QueryRewriteResponse 查询重写响应
type QueryRewriteResponse struct {
	OriginalQuery       string
	RewrittenQuery      string
	ExpandedQueries     []string
	SubQueries          []*domainrag.SubQuery
	Keywords            []string
	Intent              string
}

// RewriteQuery 重写查询
func (s *Optimizer) RewriteQuery(ctx context.Context, req *QueryRewriteRequest) (*QueryRewriteResponse, error) {
	opts := &domainrag.RewriteOptions{
		PreserveIntent:      true,
		Domain:              req.Domain,
		ConversationHistory: req.ConversationHistory,
	}

	result, err := s.queryRewriter.RewriteQuery(ctx, req.Query, opts)
	if err != nil {
		return nil, fmt.Errorf("query rewrite failed: %w", err)
	}

	response := &QueryRewriteResponse{
		OriginalQuery:  result.Original,
		RewrittenQuery: result.Rewritten,
		Keywords:       result.Keywords,
		Intent:         result.Intent,
	}

	// 扩展查询
	if req.EnableExpansion && req.ExpansionCount > 0 {
		expanded, err := s.queryRewriter.ExpandQuery(ctx, req.Query, req.ExpansionCount, opts)
		if err == nil {
			response.ExpandedQueries = expanded
		}
	}

	// 分解查询
	if req.EnableDecomposition {
		subQueries, err := s.queryRewriter.DecomposeQuery(ctx, req.Query, opts)
		if err == nil {
			response.SubQueries = subQueries
		}
	}

	return response, nil
}

// ========================================
// Multi-Hop Retrieval 多跳检索
// ========================================

// MultiHopRetrieveRequest 多跳检索请求
type MultiHopRetrieveRequest struct {
	TenantID      string
	KnowledgeBaseID string
	Query         string
	MaxHops       int
	HopStrategy   string
}

// MultiHopRetrieveResponse 多跳检索响应
type MultiHopRetrieveResponse struct {
	FinalAnswer     string
	Hops            []*HopInfo
	AllDocuments    []*domainrag.Document
	ReasoningChain  string
	HopCount        int
}

// HopInfo 单跳信息
type HopInfo struct {
	HopIndex         int
	Query            string
	IntermediateResult string
	RetrievedCount   int
	Latency          int64
}

// MultiHopRetrieve 执行多跳检索
func (s *Optimizer) MultiHopRetrieve(ctx context.Context, req *MultiHopRetrieveRequest) (*MultiHopRetrieveResponse, error) {
	opts := &domainrag.MultiHopOptions{
		MaxHops:      req.MaxHops,
		HopStrategy:  req.HopStrategy,
		EnableLog:    true,
	}

	result, err := s.multiHopRetriever.RetrieveMultiHop(ctx, req.Query, opts)
	if err != nil {
		return nil, fmt.Errorf("multi-hop retrieve failed: %w", err)
	}

	// 转换响应
	hops := make([]*HopInfo, len(result.Hops))
	for i, hop := range result.Hops {
		hops[i] = &HopInfo{
			HopIndex:           hop.HopIndex,
			Query:              hop.Query,
			IntermediateResult: hop.IntermediateResult,
			RetrievedCount:     len(hop.RetrievedDocs),
			Latency:           hop.Latency,
		}
	}

	return &MultiHopRetrieveResponse{
		FinalAnswer:    result.FinalAnswer,
		Hops:           hops,
		AllDocuments:   result.AllDocuments,
		ReasoningChain: result.ReasoningChain,
		HopCount:       len(result.Hops),
	}, nil
}

// ========================================
// 综合优化检索
// ========================================

// OptimizedRetrieveRequest 优化检索请求
type OptimizedRetrieveRequest struct {
	TenantID      string
	KnowledgeBaseID string
	Query         string
	Config        *domainrag.RetrievalOptimizationConfig
}

// OptimizedRetrieveResponse 优化检索响应
type OptimizedRetrieveResponse struct {
	Documents      []*domainrag.Document
	Query          string
	ProcessedQuery string
	TotalCount     int
	Answer         string
	TechniquesUsed []string
	Trace          *domainrag.SearchTrace
}

// OptimizedRetrieve 执行优化的检索
func (s *Optimizer) OptimizedRetrieve(ctx context.Context, req *OptimizedRetrieveRequest) (*OptimizedRetrieveResponse, error) {
	if req.Config == nil {
		req.Config = domainrag.DefaultRetrievalOptimizationConfig()
	}

	techniquesUsed := make([]string, 0)
	trace := &domainrag.SearchTrace{
		RetrievalDetails: make([]domainrag.RetrievalStep, 0),
	}

	processedQuery := req.Query

	// 1. 查询重写
	if req.Config.EnableQueryRewrite {
		rewriteResult, err := s.queryRewriter.RewriteQuery(ctx, req.Query, req.Config.RewriteOptions)
		if err == nil && rewriteResult.Rewritten != "" {
			processedQuery = rewriteResult.Rewritten
			techniquesUsed = append(techniquesUsed, "query_rewrite")
			trace.RetrievalDetails = append(trace.RetrievalDetails, domainrag.RetrievalStep{
				StepType:    "query_rewrite",
				Description: "查询重写",
				ResultCount: 1,
			})
		}
	}

	// 2. HyDE 检索
	if req.Config.EnableHyDE {
		techniquesUsed = append(techniquesUsed, "hyde")
		trace.HyDEUsed = true

		hydeDoc, err := s.hydeGenerator.GenerateHypotheticalDoc(ctx, processedQuery, req.Config.HyDEOptions)
		if err == nil {
			trace.HypotheticalDoc = hydeDoc
			// 使用假设文档检索
			retrieveResp, err := s.baseRetriever.Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, hydeDoc, &domainrag.RetrieveOptions{
				TopK:                10,
				RerankEnabled:       true,
				SimilarityThreshold: 0.5,
				RetrievalMode:       "hybrid",
			})
			if err == nil {
				return &OptimizedRetrieveResponse{
					Documents:      retrieveResp.Results,
					Query:           req.Query,
					ProcessedQuery:  processedQuery,
					TotalCount:     len(retrieveResp.Results),
					TechniquesUsed: techniquesUsed,
					Trace:          trace,
				}, nil
			}
		}
	}

	// 3. Multi-Hop 检索
	if req.Config.EnableMultiHop {
		techniquesUsed = append(techniquesUsed, "multi_hop")
		trace.MultiHopUsed = true

		multiHopResp, err := s.multiHopRetriever.RetrieveMultiHop(ctx, processedQuery, req.Config.MultiHopOptions)
		if err == nil {
			trace.HopCount = len(multiHopResp.Hops)
			trace.ReasoningChain = multiHopResp.ReasoningChain

			return &OptimizedRetrieveResponse{
				Documents:      multiHopResp.AllDocuments,
				Query:          req.Query,
				ProcessedQuery: processedQuery,
				TotalCount:     len(multiHopResp.AllDocuments),
				Answer:         multiHopResp.FinalAnswer,
				TechniquesUsed: techniquesUsed,
				Trace:          trace,
			}, nil
		}
	}

	// 4. 降级到基础检索
	retrieveResp, err := s.baseRetriever.Retrieve(ctx, req.TenantID, req.KnowledgeBaseID, processedQuery, &domainrag.RetrieveOptions{
		TopK:                10,
		RerankEnabled:       true,
		SimilarityThreshold: 0.5,
		RetrievalMode:       "hybrid",
	})
	if err != nil {
		return nil, err
	}

	return &OptimizedRetrieveResponse{
		Documents:      retrieveResp.Results,
		Query:          req.Query,
		ProcessedQuery: processedQuery,
		TotalCount:     len(retrieveResp.Results),
		TechniquesUsed: techniquesUsed,
		Trace:          retrieveResp.SearchTrace,
	}, nil
}

// ========================================
// 配置辅助
// ========================================

// CreateHyDEConfig 创建 HyDE 配置
func CreateHyDEConfig(temperature float64, maxTokens int, domain string) *domainrag.HyDEOptions {
	return &domainrag.HyDEOptions{
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Domain:      domain,
		TaskType:    "qa",
	}
}

// CreateRewriteConfig 创建查询重写配置
func CreateRewriteConfig(preserveIntent bool, domain string, history []string) *domainrag.RewriteOptions {
	return &domainrag.RewriteOptions{
		PreserveIntent:      preserveIntent,
		Domain:              domain,
		ConversationHistory: history,
	}
}

// CreateMultiHopConfig 创建多跳检索配置
func CreateMultiHopConfig(maxHops int, strategy string) *domainrag.MultiHopOptions {
	return &domainrag.MultiHopOptions{
		MaxHops:      maxHops,
		HopStrategy:  strategy,
		EnableLog:    true,
	}
}

// CreateOptimizerConfig 创建优化器配置
func CreateOptimizerConfig(
	enableHyDE bool,
	enableRewrite bool,
	enableMultiHop bool,
) *domainrag.RetrievalOptimizationConfig {
	return &domainrag.RetrievalOptimizationConfig{
		EnableHyDE:         enableHyDE,
		HyDEOptions:        DefaultHyDEOptions(),
		EnableQueryRewrite: enableRewrite,
		RewriteOptions:     DefaultRewriteOptions(),
		EnableMultiHop:     enableMultiHop,
		MultiHopOptions:    DefaultMultiHopOptions(),
		FallbackStrategy:   "hybrid",
	}
}

// DefaultHyDEOptions 默认 HyDE 配置
func DefaultHyDEOptions() *domainrag.HyDEOptions {
	return domainrag.DefaultHyDEOptions()
}

// DefaultRewriteOptions 默认重写配置
func DefaultRewriteOptions() *domainrag.RewriteOptions {
	return domainrag.DefaultRewriteOptions()
}

// DefaultMultiHopOptions 默认多跳配置
func DefaultMultiHopOptions() *domainrag.MultiHopOptions {
	return domainrag.DefaultMultiHopOptions()
}
