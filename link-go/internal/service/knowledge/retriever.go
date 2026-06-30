// Package rag provides RAG service implementations
package knowledge

import (
	"context"
	"fmt"

	domainrag "link/internal/model/rag"
)

// ========================================
// Retrieve Service Implementation
// ========================================

// Retriever implements document retrieval
type Retriever struct {
	retriever         domainrag.Retriever
	reranker          domainrag.Reranker
	queryStrengthener domainrag.QueryStrengthener
}

// NewRetriever creates a new retriever service
func NewRetriever(
	retriever domainrag.Retriever,
	reranker domainrag.Reranker,
	queryStrengthener domainrag.QueryStrengthener,
) *Retriever {
	return &Retriever{
		retriever:         retriever,
		reranker:          reranker,
		queryStrengthener: queryStrengthener,
	}
}

// Retrieve executes a retrieval request
func (r *Retriever) Retrieve(ctx context.Context, tenantID int64, req *RetrieveRequest) (*RetrieveResponse, error) {
	opts := &domainrag.RetrieveOptions{
		TopK:                req.TopK,
		SimilarityThreshold: req.SimilarityThreshold,
		RerankEnabled:       req.EnableRerank,
		RetrievalMode:       req.RetrievalMode,
	}

	// Set defaults
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.SimilarityThreshold <= 0 {
		opts.SimilarityThreshold = 0.5
	}
	if opts.RetrievalMode == "" {
		opts.RetrievalMode = "hybrid"
	}

	var resp *domainrag.RetrieveResponse
	var err error

	switch opts.RetrievalMode {
	case "vector":
		resp, err = r.retriever.VectorRetrieve(ctx, formatTenantID(tenantID), req.KnowledgeBaseID, req.Query, opts)
	case "bm25", "keyword":
		resp, err = r.retriever.BM25Retrieve(ctx, formatTenantID(tenantID), req.KnowledgeBaseID, req.Query, opts)
	case "graph":
		resp, err = r.retriever.GraphRetrieve(ctx, formatTenantID(tenantID), req.KnowledgeBaseID, req.Query, opts)
	default:
		resp, err = r.retriever.HybridRetrieve(ctx, formatTenantID(tenantID), req.KnowledgeBaseID, req.Query, opts)
	}

	if err != nil {
		return nil, err
	}

	return &RetrieveResponse{
		Documents:   convertDocuments(resp.Results),
		Query:       resp.Query,
		TotalCount:  resp.TotalCount,
		HasMore:     resp.HasMore,
		Latency:     resp.Latency,
		SearchTrace: convertRetrievalTrace(resp.SearchTrace),
	}, nil
}

// MultiKBRetrieve executes retrieval across multiple knowledge bases
func (r *Retriever) MultiKBRetrieve(ctx context.Context, tenantID int64, req *MultiKBRetrieveRequest) (*MultiKBRetrieveResponse, error) {
	results := make([]KBRetrieveResultDTO, len(req.KnowledgeBaseIDs))
	totalCount := 0

	for i, kbID := range req.KnowledgeBaseIDs {
		retrieveReq := &RetrieveRequest{
			KnowledgeBaseID:       kbID,
			Query:                  req.Query,
			TopK:                   req.TopK,
			SimilarityThreshold:    req.SimilarityThreshold,
			RetrievalMode:          req.RetrievalMode,
			EnableRerank:           req.EnableRerank,
		}

		resp, err := r.Retrieve(ctx, tenantID, retrieveReq)
		if err != nil {
			results[i] = KBRetrieveResultDTO{
				KnowledgeBaseID: kbID,
				Count:           0,
			}
			continue
		}

		results[i] = KBRetrieveResultDTO{
			KnowledgeBaseID: kbID,
			Documents:       resp.Documents,
			Count:           len(resp.Documents),
		}
		totalCount += len(resp.Documents)
	}

	return &MultiKBRetrieveResponse{
		Results:    results,
		Query:      req.Query,
		TotalCount: totalCount,
	}, nil
}

// ========================================
// Query Strengthening Service
// ========================================

// StrengthenQuery enhances a query with rewriting and splitting
func (r *Retriever) StrengthenQuery(ctx context.Context, req *StrengthenQueryRequest) (*StrengthenQueryResponse, error) {
	if r.queryStrengthener == nil {
		return &StrengthenQueryResponse{
			OriginalQuery:      req.Query,
			QueriesForRetrieve: []string{req.Query},
		}, nil
	}

	strengthOpts := domainrag.DefaultStrengthOptions()
	strengthOpts.EnableRewrite = req.EnableRewrite
	strengthOpts.EnableSplit = req.EnableSplit

	enhanced, err := r.queryStrengthener.StrengthenQuery(ctx, req.Query, req.ConversationHistory, strengthOpts)
	if err != nil {
		return nil, err
	}

	return &StrengthenQueryResponse{
		OriginalQuery:      enhanced.OriginalQuery,
		RewrittenQuery:     enhanced.RewrittenQuery,
		SubQueries:         enhanced.SubQueries,
		RewriteApplied:     enhanced.RewriteApplied,
		SplitApplied:       enhanced.SplitApplied,
		ProcessingTime:     enhanced.ProcessingTime,
		QueriesForRetrieve: enhanced.GetQueriesForRetrieve(),
	}, nil
}

// ========================================
// Helper Functions
// ========================================

// formatTenantID formats tenant ID as string
func formatTenantID(tenantID int64) string {
	return fmt.Sprintf("%d", tenantID)
}

// buildConversationHistory builds conversation history string
func buildConversationHistory(messages []ConversationMessage) string {
	if len(messages) == 0 {
		return ""
	}

	history := ""
	for _, msg := range messages {
		history += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}
	return history
}

// convertDocuments converts domain documents to DTOs
func convertDocuments(docs []*domainrag.Document) []DocumentDTO {
	result := make([]DocumentDTO, len(docs))
	for i, doc := range docs {
		result[i] = *convertDocumentDTO(doc)
	}
	return result
}

// convertDocumentDTO converts a single domain document to DTO
func convertDocumentDTO(doc *domainrag.Document) *DocumentDTO {
	return &DocumentDTO{
		ChunkID:          doc.ChunkID,
		KnowledgeID:      doc.KnowledgeID,
		KnowledgeBaseID:  doc.KnowledgeBaseID,
		Content:          doc.Content,
		Score:            float64(doc.Score),
		MatchType:        doc.MatchType,
		ChunkIndex:       doc.ChunkIndex,
		Metadata:         doc.Metadata,
	}
}

// convertRetrievalTrace converts retrieval trace to DTO
func convertRetrievalTrace(trace *domainrag.SearchTrace) *RetrievalTraceDTO {
	if trace == nil {
		return nil
	}

	steps := make([]RetrievalStepDTO, len(trace.RetrievalDetails))
	for i, step := range trace.RetrievalDetails {
		steps[i] = RetrievalStepDTO{
			StepType:    step.StepType,
			Description: step.Description,
			Latency:     step.Latency,
			ResultCount: step.ResultCount,
			Details:     step.Details,
		}
	}

	return &RetrievalTraceDTO{
		VectorLatency: trace.VectorLatency,
		BM25Latency:    trace.BM25Latency,
		GraphLatency:   trace.GraphLatency,
		RerankLatency:  trace.RerankLatency,
		Steps:          steps,
	}
}
