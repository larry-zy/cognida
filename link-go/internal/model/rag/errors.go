// Package rag provides RAG domain-specific error definitions
package rag

import (
	"errors"
	"fmt"
)

var (
	// ErrRetrievalFailed indicates a retrieval operation failed
	ErrRetrievalFailed = errors.New("retrieval failed")

	// ErrNoResultsFound indicates no results were found
	ErrNoResultsFound = errors.New("no results found")

	// ErrInvalidRetrievalMode indicates an invalid retrieval mode
	ErrInvalidRetrievalMode = errors.New("invalid retrieval mode")

	// ErrCollectionNotFound indicates a vector collection was not found
	ErrCollectionNotFound = errors.New("collection not found")

	// ErrEmbeddingFailed indicates an embedding operation failed
	ErrEmbeddingFailed = errors.New("embedding failed")

	// ErrRerankFailed indicates a rerank operation failed
	ErrRerankFailed = errors.New("rerank failed")

	// ErrQueryTooLong indicates the query is too long
	ErrQueryTooLong = errors.New("query too long")

	// ErrInvalidDocument indicates an invalid document
	ErrInvalidDocument = errors.New("invalid document")
)

// RAGError represents a RAG-specific domain error
type RAGError struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface
func (e *RAGError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface
func (e *RAGError) Unwrap() error {
	return e.Err
}

// NewRAGError creates a new RAG error
func NewRAGError(code, message string, err error) *RAGError {
	return &RAGError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Error codes
const (
	ErrorCodeRetrievalFailed     = "RETRIEVAL_FAILED"
	ErrorCodeNoResultsFound      = "NO_RESULTS_FOUND"
	ErrorCodeInvalidRetrievalMode = "INVALID_RETRIEVAL_MODE"
	ErrorCodeCollectionNotFound  = "COLLECTION_NOT_FOUND"
	ErrorCodeEmbeddingFailed     = "EMBEDDING_FAILED"
	ErrorCodeInvalidDocument     = "INVALID_DOCUMENT"
)

// Common error constructors
func RetrievalFailedError(mode string, err error) *RAGError {
	return NewRAGError(ErrorCodeRetrievalFailed, fmt.Sprintf("%s retrieval failed", mode), err)
}

func NoResultsFoundError(query string) *RAGError {
	return NewRAGError(ErrorCodeNoResultsFound, fmt.Sprintf("no results found for query: %s", query), nil)
}

func CollectionNotFoundError(kbID int64) *RAGError {
	return NewRAGError(ErrorCodeCollectionNotFound, fmt.Sprintf("collection for KB %d not found", kbID), nil)
}
