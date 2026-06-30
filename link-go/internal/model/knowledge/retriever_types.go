package knowledge

import "link/internal/model/common"

// RetrieverType represents the type of rag
type RetrieverType string

// RetrieverType constants
const (
	KeywordsRetrieverType  RetrieverType = "keywords"  // Keywords rag
	VectorRetrieverType    RetrieverType = "vector"    // Vector rag
	WebSearchRetrieverType RetrieverType = "websearch" // Web search rag
)

type RetrieveParams struct {
	// Query text
	Query string
	// Query embedding (used for vector retrieval)
	Embedding []float32
	// Knowledge base IDs
	KnowledgeBaseIDs []string
	// Knowledge IDs
	KnowledgeIDs []string
	// Tag IDs for filtering (used for FAQ priority filtering)
	TagIDs []string
	// Excluded knowledge IDs
	ExcludeKnowledgeIDs []string
	// Excluded chunk IDs
	ExcludeChunkIDs []string
	// Number of results to return
	TopK int
	// Similarity threshold
	Threshold float64
	// Knowledge type (e.g., "faq", "manual") - determines which index to use
	KnowledgeType string
	// Additional parameters, different retrievers may require different parameters
	AdditionalParams map[string]interface{}
	// Retriever type
	RetrieverType RetrieverType // Retriever type
}

// RetrieverEngineParams represents the parameters for rag engine
type RetrieverEngineParams struct {
	// Retriever type
	RetrieverType RetrieverType `yaml:"retriever_type"        json:"retriever_type"`
}

// IndexWithScore represents the index with score
type IndexWithScore struct {
	// ID
	ID string
	// Content
	Content string
	// Source ID
	SourceID string
	// Source type
	SourceType common.SourceType
	// Chunk ID
	ChunkID string
	// Knowledge ID
	KnowledgeID string
	// Knowledge base ID
	KnowledgeBaseID string
	// Tag ID
	TagID string
	// Score
	Score float64
	// Match type
	MatchType common.MatchType
	// IsEnabled
	IsEnabled bool
}

// GetScore returns the score for ScoreComparable interface
func (i *IndexWithScore) GetScore() float64 {
	return i.Score
}

// RetrieveResult represents the result of retrieval
type RetrieveResult struct {
	Results       []*IndexWithScore // Retrieval results
	RetrieverType RetrieverType     // Retrieval type
	Error         error             // Retrieval error
}
