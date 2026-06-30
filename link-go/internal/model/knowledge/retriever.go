// Package knowledge provides the knowledge management domain retriever interface definitions
package knowledge

import "context"

// ========================================
// Retriever 检索器接口
// ========================================

// Retriever knowledge retriever interface
type Retriever interface {
	// Retrieve execute retrieval
	Retrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrievalOptions) (*RetrievalResponse, error)

	// RetrieveWithEmbedding execute retrieval with provided embedding vector
	RetrieveWithEmbedding(ctx context.Context, tenantID, kbID, query string, embedding []float32, opts *RetrievalOptions) (*RetrievalResponse, error)

	// VectorRetrieve vector retrieval
	VectorRetrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrievalOptions) (*RetrievalResponse, error)

	// BM25Retrieve BM25 keyword retrieval
	BM25Retrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrievalOptions) (*RetrievalResponse, error)

	// HybridRetrieve hybrid retrieval (vector + BM25)
	HybridRetrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrievalOptions) (*RetrievalResponse, error)

	// GraphRetrieve graph retrieval
	GraphRetrieve(ctx context.Context, tenantID, kbID, query string, opts *RetrievalOptions) (*RetrievalResponse, error)
}

// ========================================
// Retrieval Options
// ========================================

// RetrievalOptions retrieval options
type RetrievalOptions struct {
	TopK                int     // Number of results
	SimilarityThreshold float64 // Similarity threshold
	RerankEnabled       bool    // Enable reranking
	GraphEnabled        bool    // Enable graph retrieval
	Alpha               float64 // Hybrid retrieval weight (vector: 1-Alpha, BM25: Alpha)
	RetrievalMode       string  // Retrieval mode: vector/bm25/hybrid/graph
}

// DefaultRetrievalOptions returns default retrieval options
func DefaultRetrievalOptions() *RetrievalOptions {
	return &RetrievalOptions{
		TopK:                10,
		SimilarityThreshold: 0.5,
		RerankEnabled:       true,
		GraphEnabled:        false,
		Alpha:               0.5,
		RetrievalMode:       "hybrid",
	}
}

// ========================================
// Retrieval Result
// ========================================

// RetrievalDocument retrieval document
type RetrievalDocument struct {
	ChunkID      string  // Document chunk ID
	KnowledgeID  string  // Knowledge entry ID
	KnowledgeBaseID         string  // Knowledge base ID
	Content      string  // Document content
	Score        float32 // Similarity score
	MatchType    string  // Match type: vector/bm25/graph/hybrid
	ChunkIndex   int     // Document chunk index
	Metadata     map[string]interface{} // Metadata
}

// RetrievalResponse retrieval response
type RetrievalResponse struct {
	Results     []*RetrievalDocument // Retrieval results
	Query       string               // Query content
	TotalCount  int                  // Total results
	HasMore     bool                 // Has more results
	Latency     int64                // Latency (ms)
	SearchTrace *RetrievalTrace      // Retrieval trace
}

// RetrievalTrace retrieval trace information
type RetrievalTrace struct {
	VectorResultCount int              // Vector retrieval result count
	BM25ResultCount   int              // BM25 retrieval result count
	GraphResultCount  int              // Graph retrieval result count
	RerankedCount     int              // Reranked result count
	VectorLatency     int64            // Vector retrieval latency
	BM25Latency       int64            // BM25 retrieval latency
	GraphLatency      int64            // Graph retrieval latency
	RerankLatency     int64            // Rerank latency
	RetrievalDetails  []RetrievalStep  // Retrieval step details
}

// RetrievalStep retrieval step detail
type RetrievalStep struct {
	StepType    string                 // Step type
	Description string                 // Description
	Latency     int64                  // Latency
	ResultCount int                    // Result count
	Details     map[string]interface{} // Details
}

// ========================================
// Reranker Interface
// ========================================

// RerankerStrategy rerank strategy interface
type RerankerStrategy interface {
	// Rerank execute reranking
	Rerank(ctx context.Context, results []*RetrievalDocument, query string) ([]*RetrievalDocument, error)
}

// Reranker reranker interface
type Reranker interface {
	// Rerank rerank retrieval results
	Rerank(ctx context.Context, results []*RetrievalDocument, query string) ([]*RetrievalDocument, error)

	// SetStrategy set rerank strategy
	SetStrategy(strategy string) error

	// GetStrategy get current strategy
	GetStrategy() string
}

// ========================================
// Query Strengthener Interface
// ========================================

// QueryStrengthener query strengthener interface
type QueryStrengthener interface {
	// StrengthenQuery enhance query
	StrengthenQuery(ctx context.Context, query string, conversationHistory string, opts *StrengthOptions) (*StrengthenedQuery, error)

	// RewriteQuery rewrite query
	RewriteQuery(ctx context.Context, query string, conversationHistory string, opts *StrengthOptions) (string, error)

	// SplitQuery split query
	SplitQuery(ctx context.Context, query string, conversationHistory string, opts *StrengthOptions) ([]string, error)
}

// StrengthOptions query enhancement options
type StrengthOptions struct {
	EnableRewrite bool    // Enable query rewrite
	EnableSplit   bool    // Enable query split
	Temperature   float64 // LLM temperature parameter
	MaxTokens     int     // Max token count
}

// DefaultStrengthOptions returns default strength options
func DefaultStrengthOptions() *StrengthOptions {
	return &StrengthOptions{
		EnableRewrite: true,
		EnableSplit:   true,
		Temperature:   0.1,
		MaxTokens:     2000,
	}
}

// ========================================
// Query Enhancement Result
// ========================================

// StrengthenedQuery enhanced query result
type StrengthenedQuery struct {
	OriginalQuery  string   // Original query
	RewrittenQuery string   // Rewritten query
	SubQueries     []string // Split sub-queries
	RewriteApplied bool    // Whether rewrite was applied
	SplitApplied   bool    // Whether split was applied
	ProcessingTime int64   // Processing time (ms)
}

// GetQueriesForRetrieve gets queries for retrieval
func (sq *StrengthenedQuery) GetQueriesForRetrieve() []string {
	var queries []string

	// If split sub-queries exist, use them
	if len(sq.SubQueries) > 0 {
		queries = append(queries, sq.SubQueries...)
	}
	// If rewritten query exists, add it
	if sq.RewrittenQuery != "" {
		queries = append(queries, sq.RewrittenQuery)
	}
	// Always include original query
	queries = append(queries, sq.OriginalQuery)

	return deduplicateQueries(queries)
}

// deduplicateQueries deduplicates query list
func deduplicateQueries(queries []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, q := range queries {
		q := trimSpace(q)
		if q != "" && !seen[q] {
			seen[q] = true
			result = append(result, q)
		}
	}

	return result
}

// trimSpace trims leading and trailing spaces
func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}

	if start >= end {
		return ""
	}

	return s[start:end]
}
