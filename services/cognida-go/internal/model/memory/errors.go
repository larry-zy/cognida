// Package memory 领域错误定义
package memory

import "errors"

// ========================================
// Memory 相关错误
// ========================================

var (
	// Message 错误
	ErrMessageNotFound      = errors.New("message not found")
	ErrMessageAlreadyDeleted = errors.New("message already deleted")
	ErrInvalidMessageType   = errors.New("invalid message type")
	ErrInvalidSessionID     = errors.New("invalid session id")

	// Summary 错误
	ErrSummaryNotFound      = errors.New("summary not found")
	ErrSummaryGeneration    = errors.New("summary generation failed")
	ErrInvalidTimeRange     = errors.New("invalid time range for summary")

	// Long Term Memory 错误
	ErrMemoryNotFound       = errors.New("long term memory not found")
	ErrInvalidMemoryContent = errors.New("invalid memory content")
	ErrInvalidCategory      = errors.New("invalid memory category")
	ErrEmbeddingFailed      = errors.New("embedding generation failed")

	// Compression 错误
	ErrCompressionFailed    = errors.New("compression failed")
	ErrCompressionInProgress = errors.New("compression already in progress")
	ErrInvalidCompressionStrategy = errors.New("invalid compression strategy")
	ErrTokenCountFailed     = errors.New("token count failed")

	// Context Builder 错误
	ErrContextTooLarge      = errors.New("context exceeds maximum token limit")
	ErrTemplateNotFound     = errors.New("template not found")
	ErrTemplateVariable     = errors.New("template variable error")

	// Storage 错误
	ErrStorageUnavailable   = errors.New("storage unavailable")
	ErrCacheUnavailable     = errors.New("cache unavailable")
	ErrTenantIsolation      = errors.New("tenant isolation violation")

	// Repository 错误
	ErrRepository           = errors.New("repository error")
	ErrTransactionFailed    = errors.New("transaction failed")
)
