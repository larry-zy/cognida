// Package knowledge provides the knowledge management domain error definitions
package knowledge

import "errors"

// ========================================
// Common Errors
// ========================================

var (
	// ErrKnowledgeBaseNotFound knowledge base not found
	ErrKnowledgeBaseNotFound = errors.New("knowledge base not found")

	// ErrKnowledgeNotFound knowledge entry not found
	ErrKnowledgeNotFound = errors.New("knowledge entry not found")

	// ErrChunkNotFound chunk not found
	ErrChunkNotFound = errors.New("chunk not found")

	// ErrTagNotFound tag not found
	ErrTagNotFound = errors.New("tag not found")

	// ErrKnowledgeBaseAlreadyExists knowledge base already exists
	ErrKnowledgeBaseAlreadyExists = errors.New("knowledge base already exists")

	// ErrInvalidKnowledgeBaseName invalid knowledge base name
	ErrInvalidKnowledgeBaseName = errors.New("invalid knowledge base name")

	// ErrInvalidKnowledgeBaseID invalid knowledge base ID
	ErrInvalidKnowledgeBaseID = errors.New("invalid knowledge base ID")

	// ErrKnowledgeBaseInUse knowledge base is in use
	ErrKnowledgeBaseInUse = errors.New("knowledge base is in use")

	// ErrKnowledgeBaseLimitExceeded knowledge base limit exceeded
	ErrKnowledgeBaseLimitExceeded = errors.New("knowledge base limit exceeded")

	// ErrStorageLimitExceeded storage limit exceeded
	ErrStorageLimitExceeded = errors.New("storage limit exceeded")

	// ErrChunkingFailed chunking failed
	ErrChunkingFailed = errors.New("chunking failed")

	// ErrParsingFailed parsing failed
	ErrParsingFailed = errors.New("parsing failed")

	// ErrEmbeddingFailed embedding generation failed
	ErrEmbeddingFailed = errors.New("embedding generation failed")

	// ========================================
	// Validation Errors
	// ========================================

	// ErrInvalidName invalid name
	ErrInvalidName = errors.New("invalid name")

	// ErrInvalidTenantID invalid tenant ID
	ErrInvalidTenantID = errors.New("invalid tenant ID")

	// ErrInvalidUserID invalid user ID
	ErrInvalidUserID = errors.New("invalid user ID")

	// ErrInvalidTitle invalid title
	ErrInvalidTitle = errors.New("invalid title")

	// ErrInvalidKnowledgeID invalid knowledge ID
	ErrInvalidKnowledgeID = errors.New("invalid knowledge ID")

	// ErrInvalidContent invalid content
	ErrInvalidContent = errors.New("invalid content")

	// ErrRetrievalFailed retrieval failed
	ErrRetrievalFailed = errors.New("retrieval failed")

	// ErrNoResultsFound no results found
	ErrNoResultsFound = errors.New("no results found")

	// ErrInvalidRetrievalMode invalid retrieval mode
	ErrInvalidRetrievalMode = errors.New("invalid retrieval mode")

	// ErrCollectionNotFound collection not found
	ErrCollectionNotFound = errors.New("collection not found")

	// ErrVectorSearchFailed vector search failed
	ErrVectorSearchFailed = errors.New("vector search failed")

	// ErrGraphNotFound graph not found
	ErrGraphNotFound = errors.New("graph not found")

	// ErrGraphNodeNotFound graph node not found
	ErrGraphNodeNotFound = errors.New("graph node not found")

	// ErrGraphRelationNotFound graph relation not found
	ErrGraphRelationNotFound = errors.New("graph relation not found")

	// ErrGraphExtractionFailed graph extraction failed
	ErrGraphExtractionFailed = errors.New("graph extraction failed")

	// ErrGraphMergeFailed graph merge failed
	ErrGraphMergeFailed = errors.New("graph merge failed")

	// ErrInvalidGraphData invalid graph data
	ErrInvalidGraphData = errors.New("invalid graph data")

	// ErrQueryTooLong query too long
	ErrQueryTooLong = errors.New("query too long")

	// ErrInvalidDocument invalid document
	ErrInvalidDocument = errors.New("invalid document")

	// ErrInvalidChunkSize invalid chunk size
	ErrInvalidChunkSize = errors.New("invalid chunk size")

	// ErrInvalidDimension invalid vector dimension
	ErrInvalidDimension = errors.New("invalid vector dimension")

	// ErrInvalidIndexType invalid index type
	ErrInvalidIndexType = errors.New("invalid index type")

	// ErrInvalidMetricType invalid metric type
	ErrInvalidMetricType = errors.New("invalid metric type")

	// ErrRerankFailed rerank failed
	ErrRerankFailed = errors.New("rerank failed")

	// ErrQueryEnhancementFailed query enhancement failed
	ErrQueryEnhancementFailed = errors.New("query enhancement failed")
)

// ========================================
// Knowledge Error Types
// ========================================

// KnowledgeError represents a knowledge-specific domain error
type KnowledgeError struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface
func (e *KnowledgeError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap implements the errors.Unwrap interface
func (e *KnowledgeError) Unwrap() error {
	return e.Err
}

// ========================================
// Error Constructors
// ========================================

// NewKnowledgeError creates a new knowledge error
func NewKnowledgeError(code, message string, err error) *KnowledgeError {
	return &KnowledgeError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// KnowledgeBaseNotFoundError creates a knowledge base not found error
func KnowledgeBaseNotFoundError(id string) *KnowledgeError {
	return NewKnowledgeError("KB_NOT_FOUND", "knowledge base not found: "+id, ErrKnowledgeBaseNotFound)
}

// KnowledgeNotFoundError creates a knowledge entry not found error
func KnowledgeNotFoundError(id string) *KnowledgeError {
	return NewKnowledgeError("KNOWLEDGE_NOT_FOUND", "knowledge entry not found: "+id, ErrKnowledgeNotFound)
}

// ChunkNotFoundError creates a chunk not found error
func ChunkNotFoundError(id string) *KnowledgeError {
	return NewKnowledgeError("CHUNK_NOT_FOUND", "chunk not found: "+id, ErrChunkNotFound)
}

// RetrievalFailedError creates a retrieval failed error
func RetrievalFailedError(mode string, err error) *KnowledgeError {
	return NewKnowledgeError("RETRIEVAL_FAILED", mode+" retrieval failed", err)
}

// NoResultsFoundError creates a no results found error
func NoResultsFoundError(query string) *KnowledgeError {
	return NewKnowledgeError("NO_RESULTS", "no results found for query: "+query, ErrNoResultsFound)
}

// CollectionNotFoundError creates a collection not found error
func CollectionNotFoundError(kbID int64) *KnowledgeError {
	return NewKnowledgeError("COLLECTION_NOT_FOUND", "collection not found for KB "+string(rune(kbID)), ErrCollectionNotFound)
}

// GraphExtractionError creates a graph extraction error
func GraphExtractionError(chunkID string, err error) *KnowledgeError {
	return NewKnowledgeError("GRAPH_EXTRACTION_FAILED", "graph extraction failed for chunk: "+chunkID, err)
}

// InvalidDimensionError creates an invalid dimension error
func InvalidDimensionError(dimension int) *KnowledgeError {
	return NewKnowledgeError("INVALID_DIMENSION", "invalid vector dimension: "+string(rune(dimension)), ErrInvalidDimension)
}

// StorageLimitExceededError creates a storage limit exceeded error
func StorageLimitExceededError(tenantID int64, current, limit int64) *KnowledgeError {
	return NewKnowledgeError("STORAGE_LIMIT", "storage limit exceeded for tenant", ErrStorageLimitExceeded)
}

// ========================================
// Error Codes
// ========================================

const (
	ErrorCodeKBNotFound            = "KB_NOT_FOUND"
	ErrorCodeKnowledgeNotFound      = "KNOWLEDGE_NOT_FOUND"
	ErrorCodeChunkNotFound          = "CHUNK_NOT_FOUND"
	ErrorCodeTagNotFound            = "TAG_NOT_FOUND"
	ErrorCodeKBAlreadyExists        = "KB_ALREADY_EXISTS"
	ErrorCodeInvalidKBName          = "INVALID_KB_NAME"
	ErrorCodeInvalidKnowledgeBaseID            = "INVALID_KB_ID"
	ErrorCodeKBInUse                = "KB_IN_USE"
	ErrorCodeKBLimitExceeded        = "KB_LIMIT_EXCEEDED"
	ErrorCodeStorageLimitExceeded   = "STORAGE_LIMIT_EXCEEDED"
	ErrorCodeChunkingFailed         = "CHUNKING_FAILED"
	ErrorCodeParsingFailed          = "PARSING_FAILED"
	ErrorCodeEmbeddingFailed        = "EMBEDDING_FAILED"
	ErrorCodeRetrievalFailed        = "RETRIEVAL_FAILED"
	ErrorCodeNoResultsFound         = "NO_RESULTS"
	ErrorCodeInvalidRetrievalMode   = "INVALID_RETRIEVAL_MODE"
	ErrorCodeCollectionNotFound     = "COLLECTION_NOT_FOUND"
	ErrorCodeVectorSearchFailed     = "VECTOR_SEARCH_FAILED"
	ErrorCodeGraphNotFound          = "GRAPH_NOT_FOUND"
	ErrorCodeGraphNodeNotFound      = "GRAPH_NODE_NOT_FOUND"
	ErrorCodeGraphRelationNotFound  = "GRAPH_RELATION_NOT_FOUND"
	ErrorCodeGraphExtractionFailed  = "GRAPH_EXTRACTION_FAILED"
	ErrorCodeGraphMergeFailed       = "GRAPH_MERGE_FAILED"
	ErrorCodeInvalidGraphData       = "INVALID_GRAPH_DATA"
	ErrorCodeQueryTooLong           = "QUERY_TOO_LONG"
	ErrorCodeInvalidDocument        = "INVALID_DOCUMENT"
	ErrorCodeInvalidChunkSize       = "INVALID_CHUNK_SIZE"
	ErrorCodeInvalidDimension       = "INVALID_DIMENSION"
	ErrorCodeInvalidIndexType       = "INVALID_INDEX_TYPE"
	ErrorCodeInvalidMetricType      = "INVALID_METRIC_TYPE"
	ErrorCodeRerankFailed           = "RERANK_FAILED"
	ErrorCodeQueryEnhancementFailed = "QUERY_ENHANCEMENT_FAILED"
)
