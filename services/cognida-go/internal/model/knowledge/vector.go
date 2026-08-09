// Package knowledge provides the knowledge management domain vector entity definitions
package knowledge

// ========================================
// Vector Document Entity
// ========================================

// VectorDocument vector document for semantic search
type VectorDocument struct {
	ID           int64           // Primary key ID
	DenseVector  []float32       // Dense vector
	SparseVector SparseEmbedding // Sparse vector
	ChunkID      string          // Chunk ID
	KnowledgeID  string          // Knowledge entry ID
	KnowledgeBaseID string          // Knowledge base ID
	TenantID     int64           // Tenant ID
	ChunkIndex   int             // Chunk index
	Content      string          // Chunk content
	IsEnabled    bool            // Enabled
	StartAt      int64           // Start position
	EndAt        int64           // End position
	TokenCount   int64           // Token count
	Score        float32         // Similarity score
	Metadata     map[string]interface{} // Metadata
}

// SparseEmbedding sparse vector representation
type SparseEmbedding map[uint32]float32

// ========================================
// Vector Search Options
// ========================================

// VectorSearchOptions vector search options
type VectorSearchOptions struct {
	TopK            int     // Number of results to return
	ScoreThreshold  float32 // Similarity threshold
	Filter          string  // Filter expression
	IncludeContent  bool    // Include content in results
	IncludeMetadata bool    // Include metadata in results
}

// DefaultVectorSearchOptions returns default vector search options
func DefaultVectorSearchOptions() *VectorSearchOptions {
	return &VectorSearchOptions{
		TopK:            10,
		ScoreThreshold:  0.0,
		Filter:          "",
		IncludeContent:  true,
		IncludeMetadata: false,
	}
}

// ========================================
// Vector Collection Options
// ========================================

// CollectionOptions vector collection options
type CollectionOptions struct {
	Description   string // Collection description
	AutoID        bool   // Auto-generate ID
	EnableDynamic bool   // Enable dynamic fields
	ReplicaNumber int    // Replica number for load balancing
}

// DefaultCollectionOptions returns default collection options
func DefaultCollectionOptions() *CollectionOptions {
	return &CollectionOptions{
		Description:   "Knowledge base vector collection",
		AutoID:        false,
		EnableDynamic: true,
		ReplicaNumber: 1,
	}
}

// ========================================
// Index Types
// ========================================

// IndexType vector index type
type IndexType string

const (
	IndexTypeFlat            IndexType = "FLAT"
	IndexTypeIvfFlat         IndexType = "IVF_FLAT"
	IndexTypeIvfSq8          IndexType = "IVF_SQ8"
	IndexTypeIvfPq           IndexType = "IVF_PQ"
	IndexTypeHnsw            IndexType = "HNSW"
	IndexTypeDiskAnn         IndexType = "DISKANN"
	IndexTypeAutoIndex       IndexType = "AUTOINDEX"
	IndexTypeScalar          IndexType = "SCALAR"
	IndexTypeSparseInverted  IndexType = "SPARSE_INVERTED"
	IndexTypeSparseWAND      IndexType = "SPARSE_WAND"
)

// MetricType distance metric type
type MetricType string

const (
	MetricTypeL2     MetricType = "L2"
	MetricTypeIP     MetricType = "IP"
	MetricTypeCOSINE MetricType = "COSINE"
	MetricTypeBM25   MetricType = "BM25"
)

// ========================================
// Vector Collection Stats
// ========================================

// CollectionStats vector collection statistics
type CollectionStats struct {
	Name       string              // Collection name
	FieldCount int                 // Field count
	RowCount   int64               // Row count
	Size       int64               // Collection size in bytes
	Statistics map[string]string   // Additional statistics
}

// ========================================
// Helper Methods
// ========================================

// GetSize returns the size of the dense vector
func (v *VectorDocument) GetSize() int {
	return len(v.DenseVector)
}

// GetSparseSize returns the size of the sparse vector
func (v *VectorDocument) GetSparseSize() int {
	return len(v.SparseVector)
}

// IsEmpty checks if the vector document has no vector data
func (v *VectorDocument) IsEmpty() bool {
	return len(v.DenseVector) == 0 && len(v.SparseVector) == 0
}

// ToMap converts the vector document to a map for insertion
func (v *VectorDocument) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"chunk_id":     v.ChunkID,
		"knowledge_id": v.KnowledgeID,
		"kb_id":        v.KnowledgeBaseID,
		"tenant_id":    v.TenantID,
		"chunk_index":  v.ChunkIndex,
		"content":      v.Content,
		"is_enabled":   v.IsEnabled,
		"start_at":     v.StartAt,
		"end_at":       v.EndAt,
		"token_count":  v.TokenCount,
		"vector":       v.DenseVector,
	}
}
