// Package knowledge provides transaction management interfaces for the knowledge domain
package knowledge

import "context"

// TransactionManager defines the interface for managing database transactions.
// This abstracts the underlying transaction implementation (GORM, sql.Tx, etc.)
type TransactionManager interface {
	// WithTransaction executes a function within a transaction.
	// If the function returns an error, the transaction is rolled back.
	// Otherwise, it is committed.
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// KnowledgeStatsQuerier defines methods for querying knowledge statistics.
// This separates statistics queries from the main repository.
type KnowledgeStatsQuerier interface {
	// GetStats retrieves statistics for a knowledge base.
	GetStats(ctx context.Context, kbID string) (*KnowledgeBaseStats, error)

	// GetKnowledgeList retrieves a paginated list of knowledge items.
	GetKnowledgeList(ctx context.Context, kbID string, page, pageSize int, status string) ([]*Knowledge, int64, error)

	// GetKnowledgeListWithStatus retrieves a paginated list of knowledge items with multiple statuses.
	GetKnowledgeListWithStatus(ctx context.Context, kbID string, page, pageSize int, statuses []string) ([]*Knowledge, int64, error)

	// DeleteKnowledgeWithChunks deletes a knowledge item and its chunks within a transaction.
	DeleteKnowledgeWithChunks(ctx context.Context, kbID, knowledgeID string) error
}
