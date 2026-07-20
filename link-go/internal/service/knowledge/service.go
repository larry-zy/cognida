// Package knowledge provides Knowledge service implementations
package knowledge

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cloudwego/eino/components/model"

	domainrag "link/internal/model/rag"
	domain_knowledge "link/internal/model/knowledge"
)

// RAGService provides a unified interface for backward compatibility
// It combines the individual service components
type RAGService struct {
	pipeline        *Pipeline
	retriever       *Retriever
	optimizer       *Optimizer
	graphService    *GraphService
}

// NewRAGService creates a new RAG service with all components
func NewRAGService(
	retriever domainrag.Retriever,
	reranker domainrag.Reranker,
	queryStrengthener domainrag.QueryStrengthener,
	graphRepo domain_knowledge.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
	llmChat domainrag.LLMChat,
	graphExtractor domain_knowledge.GraphExtractor,
) *RAGService {
	return &RAGService{
		pipeline: NewPipeline(retriever, reranker, queryStrengthener, llmChat),
		retriever: NewRetriever(retriever, reranker, queryStrengthener),
		graphService: NewGraphServiceWithQuery(graphRepo, graphQueryRepo, graphExtractor),
	}
}

// NewRAGServiceWithOptimizer creates a new RAG service with optimizer support
func NewRAGServiceWithOptimizer(
	baseRetriever domainrag.Retriever,
	reranker domainrag.Reranker,
	queryStrengthener domainrag.QueryStrengthener,
	graphRepo domain_knowledge.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
	llmChat domainrag.LLMChat,
	llmModel model.ChatModel, // eino.ChatModel for optimizer
	graphExtractor domain_knowledge.GraphExtractor,
) *RAGService {
	return &RAGService{
		pipeline:     NewPipeline(baseRetriever, reranker, queryStrengthener, llmChat),
		retriever:    NewRetriever(baseRetriever, reranker, queryStrengthener),
		optimizer:    NewOptimizer(llmModel, baseRetriever),
		graphService: NewGraphServiceWithQuery(graphRepo, graphQueryRepo, graphExtractor),
	}
}

// Chat delegates to Pipeline
func (s *RAGService) Chat(ctx context.Context, tenantID int64, req *ChatRequest) (*ChatResponse, error) {
	return s.pipeline.Chat(ctx, tenantID, req)
}

// ChatStream delegates to Pipeline
func (s *RAGService) ChatStream(ctx context.Context, tenantID int64, req *ChatRequest) (<-chan *StreamEvent, error) {
	return s.pipeline.ChatStream(ctx, tenantID, req)
}

// Retrieve delegates to Retriever
func (s *RAGService) Retrieve(ctx context.Context, tenantID int64, req *RetrieveRequest) (*RetrieveResponse, error) {
	return s.retriever.Retrieve(ctx, tenantID, req)
}

// ExtractGraph delegates to GraphService
// 从单个文档块抽取图谱（节点+关系），不落库，仅返回抽取结果 DTO。
func (s *RAGService) ExtractGraph(ctx context.Context, tenantID int64, req *GraphExtractRequest) (*GraphExtractResponse, error) {
	if s.graphService == nil {
		return nil, fmt.Errorf("graph service not initialized")
	}

	mode := ToDomainExtractionMode(req.Mode)
	input := &domain_knowledge.ChunkExtractionInput{
		KnowledgeBaseID: req.KnowledgeBaseID,
		ChunkID:         req.ChunkID,
		Document:        req.Document,
		Query:           req.Query,
		Mode:            mode,
	}

	data, err := s.graphService.ExtractGraphFromChunksWithMode(ctx, []*domain_knowledge.ChunkExtractionInput{input}, mode)
	if err != nil {
		return nil, fmt.Errorf("extract graph failed: %w", err)
	}

	nodes, relations := FromDomainGraphData(data)
	return &GraphExtractResponse{
		ChunkID:   req.ChunkID,
		Nodes:     nodes,
		Relations: relations,
	}, nil
}

// QueryGraph delegates to GraphService
// 按给定节点名在租户/知识库命名空间下检索图谱子图。
func (s *RAGService) QueryGraph(ctx context.Context, tenantID int64, req *GraphQueryRequest) (*GraphQueryResponse, error) {
	if s.graphService == nil {
		return nil, fmt.Errorf("graph service not initialized")
	}

	namespace := ToDomainNameSpace(req.KnowledgeBaseID, strconv.FormatInt(tenantID, 10))
	data, err := s.graphService.SearchNode(ctx, namespace, req.Nodes)
	if err != nil {
		return nil, fmt.Errorf("query graph failed: %w", err)
	}

	nodes, relations := FromDomainGraphData(data)
	return &GraphQueryResponse{
		Nodes:     nodes,
		Relations: relations,
	}, nil
}

// StrengthenQuery delegates to Retriever
func (s *RAGService) StrengthenQuery(ctx context.Context, req *StrengthenQueryRequest) (*StrengthenQueryResponse, error) {
	return s.retriever.StrengthenQuery(ctx, req)
}

// MultiKBRetrieve uses Retriever for multi-KB retrieval
func (s *RAGService) MultiKBRetrieve(ctx context.Context, tenantID int64, req *MultiKBRetrieveRequest) (*MultiKBRetrieveResponse, error) {
	return s.retriever.MultiKBRetrieve(ctx, tenantID, req)
}

// HyDERetrieve delegates to Optimizer
func (s *RAGService) HyDERetrieve(ctx context.Context, req *HyDERetrieveRequest) (*HyDERetrieveResponse, error) {
	if s.optimizer == nil {
		return nil, fmt.Errorf("optimizer not initialized")
	}
	return s.optimizer.HyDERetrieve(ctx, req)
}

// RewriteQuery delegates to Optimizer
func (s *RAGService) RewriteQuery(ctx context.Context, req *QueryRewriteRequest) (*QueryRewriteResponse, error) {
	if s.optimizer == nil {
		return nil, fmt.Errorf("optimizer not initialized")
	}
	return s.optimizer.RewriteQuery(ctx, req)
}

// MultiHopRetrieve delegates to Optimizer
func (s *RAGService) MultiHopRetrieve(ctx context.Context, req *MultiHopRetrieveRequest) (*MultiHopRetrieveResponse, error) {
	if s.optimizer == nil {
		return nil, fmt.Errorf("optimizer not initialized")
	}
	return s.optimizer.MultiHopRetrieve(ctx, req)
}

// OptimizedRetrieve delegates to Optimizer
func (s *RAGService) OptimizedRetrieve(ctx context.Context, req *OptimizedRetrieveRequest) (*OptimizedRetrieveResponse, error) {
	if s.optimizer == nil {
		return nil, fmt.Errorf("optimizer not initialized")
	}
	return s.optimizer.OptimizedRetrieve(ctx, req)
}
