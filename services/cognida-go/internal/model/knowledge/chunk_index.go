package knowledge

import "context"

// ChunkIndexQuery reads exact indexed chunks within an already authorized scope.
// Storage-specific filter syntax and vector columns do not cross this port.
type ChunkIndexQuery struct {
	TenantID     int64
	KBIDs        []string
	ChunkIDs     []string
	KnowledgeIDs []string
	EnabledOnly  bool
	Limit        int64
	Offset       int64
}

// IndexedChunk is the scalar content used by exact-reading Agent tools.
type IndexedChunk struct {
	ChunkID         string
	KnowledgeID     string
	KnowledgeBaseID string
	ChunkIndex      int
	Content         string
	IsEnabled       bool
	TokenCount      int64
}

type ChunkIndexReader interface {
	QueryChunks(context.Context, ChunkIndexQuery) ([]*IndexedChunk, error)
}
