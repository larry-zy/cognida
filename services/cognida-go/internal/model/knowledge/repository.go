// Package knowledge provides the knowledge management domain repository interface definitions
package knowledge

import "context"

// ========================================
// KnowledgeBaseRepository 知识库仓储接口
// ========================================

// KnowledgeBaseRepository knowledge base repository interface
type KnowledgeBaseRepository interface {
	// Create create knowledge base
	Create(ctx context.Context, kb *KnowledgeBase) error

	// FindByID find by ID
	FindByID(ctx context.Context, id string) (*KnowledgeBase, error)

	// FindByIDForTenant find by ID scoped to tenant（租户纵深防御：WHERE id=? AND tenant_id=?，不匹配返回 not found）
	FindByIDForTenant(ctx context.Context, id string, tenantID int64) (*KnowledgeBase, error)

	// FindByTenantID find by tenant ID
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*KnowledgeBase, int64, error)

	// FindByUser find by user ID
	FindByUser(ctx context.Context, userID int64, page, pageSize int) ([]*KnowledgeBase, int64, error)

	// Update update knowledge base
	Update(ctx context.Context, kb *KnowledgeBase) error

	// UpdateStats update statistics
	UpdateStats(ctx context.Context, knowledgeBaseID string, documentCount, chunkCount int, storageSize int64) error

	// Delete soft delete
	Delete(ctx context.Context, id string) error

	// HardDelete hard delete knowledge base and all associated data
	HardDelete(ctx context.Context, id string) error

	// Exists check if knowledge base exists
	Exists(ctx context.Context, id string) (bool, error)

	// GetStorageStats get tenant storage statistics
	GetStorageStats(ctx context.Context, tenantID int64) (totalSize, kbCount int64, err error)
}

// ========================================
// KnowledgeRepository 知识条目仓储接口
// ========================================

// KnowledgeRepository knowledge entry repository interface
type KnowledgeRepository interface {
	// Create create knowledge entry
	Create(ctx context.Context, knowledge *Knowledge) error

	// CreateBatch batch create
	CreateBatch(ctx context.Context, knowledgeList []*Knowledge) error

	// FindByID find by ID
	FindByID(ctx context.Context, id string) (*Knowledge, error)

	// FindByKnowledgeBaseID find by knowledge base ID
	FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string, query *KnowledgeListQuery) ([]*Knowledge, int64, error)

	// FindByTenantID find by tenant ID
	FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*Knowledge, int64, error)

	// Update update knowledge entry
	Update(ctx context.Context, knowledge *Knowledge) error

	// UpdateParseStatus update parse status
	UpdateParseStatus(ctx context.Context, id string, parseStatus string, errorMessage string) error

	// UpdateChunkCount update chunk count
	UpdateChunkCount(ctx context.Context, id string, chunkCount int) error

	// Delete soft delete
	Delete(ctx context.Context, id string) error

	// DeleteBatch batch soft delete
	DeleteBatch(ctx context.Context, ids []string) error

	// HardDelete hard delete
	HardDelete(ctx context.Context, id string) error

	// HardDeleteBatch batch hard delete
	HardDeleteBatch(ctx context.Context, ids []string) error

	// FindByStatus find by parse status
	FindByStatus(ctx context.Context, tenantID int64, parseStatus string, limit int) ([]*Knowledge, error)

	// FindByFileHash find by file hash (deduplication)
	FindByFileHash(ctx context.Context, tenantID int64, fileHash string) (*Knowledge, error)

	// FindByFileHashInKB find non-deleted knowledge by file hash within a knowledge base (dedup scope)
	FindByFileHashInKB(ctx context.Context, knowledgeBaseID string, fileHash string) (*Knowledge, error)

	// UpdateTagID update tag ID
	UpdateTagID(ctx context.Context, id string, tagID int64) error

	// RemoveTagID remove tag ID (set to 0)
	RemoveTagID(ctx context.Context, id string) error

	// RemoveTagIDBatch batch remove tag ID
	RemoveTagIDBatch(ctx context.Context, ids []string, tagID int64) error

	// FindByTagID find by tag ID
	FindByTagID(ctx context.Context, tenantID int64, tagID int64, page, pageSize int) ([]*Knowledge, int64, error)

	// AddTagIDBatch batch add tag ID
	AddTagIDBatch(ctx context.Context, ids []string, tagID int64) error
}

// ========================================
// ChunkRepository 文档分块仓储接口
// ========================================

// ChunkRepository document chunk repository interface
type ChunkRepository interface {
	// Create create chunk
	Create(ctx context.Context, chunk *Chunk) error

	// CreateBatch batch create
	CreateBatch(ctx context.Context, chunks []*Chunk) error

	// FindByID find by ID
	FindByID(ctx context.Context, id string) (*Chunk, error)

	// FindByKnowledgeBaseID find by knowledge base ID
	FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string, query *ChunkListQuery) ([]*Chunk, int64, error)

	// FindByKnowledgeID find by knowledge entry ID
	FindByKnowledgeID(ctx context.Context, knowledgeID string, enabledOnly bool) ([]*Chunk, error)

	// Update update chunk
	Update(ctx context.Context, chunk *Chunk) error

	// UpdateEmbeddingID update embedding ID
	UpdateEmbeddingID(ctx context.Context, id string, embeddingID string) error

	// UpdateBatchStatus batch update enabled status
	UpdateBatchStatus(ctx context.Context, ids []string, isEnabled bool) error

	// Delete soft delete
	Delete(ctx context.Context, id string) error

	// DeleteByKnowledgeID delete all chunks of knowledge entry
	DeleteByKnowledgeID(ctx context.Context, knowledgeID string) error

	// DeleteByKnowledgeBaseID delete all chunks of knowledge base
	DeleteByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) error

	// HardDelete hard delete
	HardDelete(ctx context.Context, id string) error

	// HardDeleteBatch batch hard delete
	HardDeleteBatch(ctx context.Context, ids []string) error

	// FindEnabledChunks find enabled chunks (for retrieval)
	FindEnabledChunks(ctx context.Context, knowledgeBaseID string, limit int) ([]*Chunk, error)

	// CountByKnowledgeBaseID count chunks by knowledge base
	CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int64, error)

	// UpdateTagID update tag ID
	UpdateTagID(ctx context.Context, id string, tagID int64) error

	// RemoveTagID remove tag ID
	RemoveTagID(ctx context.Context, id string) error

	// UpdateTagIDBatch batch update tag ID
	UpdateTagIDBatch(ctx context.Context, ids []string, tagID int64) error

	// RemoveTagIDBatch batch remove tag ID
	RemoveTagIDBatch(ctx context.Context, ids []string, tagID int64) error

	// FindByTagID find by tag ID
	FindByTagID(ctx context.Context, tenantID int64, tagID int64, page, pageSize int) ([]*Chunk, int64, error)

	// AddTagIDBatch batch add tag ID
	AddTagIDBatch(ctx context.Context, ids []string, tagID int64) error

	// FindByGraphNodes find associated chunks by graph node names
	FindByGraphNodes(ctx context.Context, knowledgeBaseID string, nodeNames []string) ([]*Chunk, error)

	// GetGraphStats get graph statistics
	GetGraphStats(ctx context.Context, knowledgeBaseID string) (*GraphStats, error)
}

// ========================================
// KnowledgeBaseSettingRepository 知识库设置仓储接口
// ========================================

// KnowledgeBaseSettingRepository knowledge base setting repository interface
type KnowledgeBaseSettingRepository interface {
	// Create create setting
	Create(ctx context.Context, setting *KnowledgeBaseSetting) error

	// FindByKnowledgeBaseID find by knowledge base ID
	FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (*KnowledgeBaseSetting, error)

	// Update update setting
	Update(ctx context.Context, setting *KnowledgeBaseSetting) error

	// UpdateRetrievalConfig update retrieval config
	UpdateRetrievalConfig(ctx context.Context, knowledgeBaseID string, mode string, threshold float64, topK int) error

	// UpdateModelConfig update model config
	UpdateModelConfig(ctx context.Context, knowledgeBaseID string, embeddingModelID, summaryModelID, rerankModelID string) error

	// UpdateProcessingConfig update processing config
	UpdateProcessingConfig(ctx context.Context, knowledgeBaseID string, chunkingConfig, imageProcessingConfig, cosConfig, vlmConfig, extractConfig string) error

	// Delete delete setting
	Delete(ctx context.Context, knowledgeBaseID string) error

	// Exists check if setting exists
	Exists(ctx context.Context, knowledgeBaseID string) (bool, error)
}

// ========================================
// RetrievalSettingRepository 检索设置仓储接口
// ========================================

// RetrievalSettingRepository retrieval setting repository interface
type RetrievalSettingRepository interface {
	// Create create retrieval setting
	Create(ctx context.Context, setting *RetrievalSetting) error

	// FindByKnowledgeBaseID find by knowledge base ID
	FindByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (*RetrievalSetting, error)

	// FindByID find by ID
	FindByID(ctx context.Context, id int64) (*RetrievalSetting, error)

	// FindBySessionID find by session ID
	FindBySessionID(ctx context.Context, sessionID string) (*RetrievalSetting, error)

	// Update update retrieval setting
	Update(ctx context.Context, setting *RetrievalSetting) error

	// Delete delete retrieval setting
	Delete(ctx context.Context, knowledgeBaseID string) error

	// UpsertBySessionID create or update by session ID
	UpsertBySessionID(ctx context.Context, sessionID string, tenantID int64, ragConfig interface{}) error

	// UpdateVectorConfig update vector retrieval config
	UpdateVectorConfig(ctx context.Context, kbID string, topK int, threshold float64, modelID string) error

	// UpdateBM25Config update BM25 retrieval config
	UpdateBM25Config(ctx context.Context, kbID string, topK int) error

	// UpdateGraphConfig update graph retrieval config
	UpdateGraphConfig(ctx context.Context, kbID string, enabled bool, topK int, minStrength float64) error

	// UpdateHybridConfig update hybrid retrieval config
	UpdateHybridConfig(ctx context.Context, kbID string, alpha float64, rerankEnabled bool) error

	// UpdateWebConfig update web search config
	UpdateWebConfig(ctx context.Context, kbID string, enabled bool, topK int, engine, apiKey string, searchDepth int) error

	// UpdateRerankConfig update rerank config
	UpdateRerankConfig(ctx context.Context, kbID string, enabled bool, modelID string) error

	// UpdateDefaultMode update default retrieval mode
	UpdateDefaultMode(ctx context.Context, kbID string, mode string, availableModes []string) error

	// Exists check if setting exists
	Exists(ctx context.Context, knowledgeBaseID string) (bool, error)
}

// ========================================
// TagRepository 知识标签仓储接口
// ========================================

// TagRepository knowledge tag repository interface
type TagRepository interface {
	// Create create tag
	Create(ctx context.Context, tag *Tag) error

	// CreateBatch batch create
	CreateBatch(ctx context.Context, tags []*Tag) error

	// FindByID find by ID
	FindByID(ctx context.Context, id int64) (*Tag, error)

	// FindByKnowledgeBaseID find by knowledge base ID
	FindByKnowledgeBaseID(ctx context.Context, tenantID string, kbID int64, query *TagListQuery) ([]*Tag, int64, error)

	// FindByTenantID find by tenant ID
	FindByTenantID(ctx context.Context, tenantID string, page, pageSize int) ([]*Tag, int64, error)

	// FindByName find by name
	FindByName(ctx context.Context, tenantID string, kbID int64, name string) (*Tag, error)

	// Update update tag
	Update(ctx context.Context, tag *Tag) error

	// Delete soft delete
	Delete(ctx context.Context, id int64) error

	// DeleteBatch batch soft delete
	DeleteBatch(ctx context.Context, ids []int64) error

	// DeleteByKnowledgeBaseID delete all tags of knowledge base
	DeleteByKnowledgeBaseID(ctx context.Context, tenantID string, kbID int64) error

	// Exists check if tag exists
	Exists(ctx context.Context, tenantID string, kbID int64, name string) (bool, error)

	// CountByKnowledgeBaseID count tags by knowledge base
	CountByKnowledgeBaseID(ctx context.Context, tenantID string, kbID int64) (int64, error)

	// UpdateSortOrder batch update sort order
	UpdateSortOrder(ctx context.Context, tagOrders []TagSortOrder) error
}

// TagSortOrder tag sort order info
type TagSortOrder struct {
	ID        int64
	SortOrder int
}

// ========================================
// VectorRepository 向量仓储接口
// ========================================

// VectorRepository vector repository interface
type VectorRepository interface {
	// CreateCollection create collection
	CreateCollection(ctx context.Context, kbID int64, dimension int, opts *CollectionOptions) error

	// HasCollection check if collection exists
	HasCollection(ctx context.Context, kbID int64) (bool, error)

	// Insert insert vector documents
	Insert(ctx context.Context, kbID int64, docs []*VectorDocument) error

	// Search vector search
	Search(ctx context.Context, kbID int64, vector []float32, opts *VectorSearchOptions) ([]*VectorDocument, error)

	// Delete delete by IDs
	Delete(ctx context.Context, kbID int64, ids []int64) error

	// DeleteByChunkID delete by chunk ID
	DeleteByChunkID(ctx context.Context, kbID int64, chunkID string) error

	// DeleteByKnowledgeID delete by knowledge ID
	DeleteByKnowledgeID(ctx context.Context, kbID int64, knowledgeID string) error

	// CreateIndex create index
	CreateIndex(ctx context.Context, kbID int64, fieldName string, indexType IndexType, metricType MetricType, params map[string]string) error

	// LoadCollection load collection to memory
	LoadCollection(ctx context.Context, kbID int64, async bool) error

	// ReleaseCollection release collection memory
	ReleaseCollection(ctx context.Context, kbID int64) error

	// GetStats get collection statistics
	GetStats(ctx context.Context, kbID int64) (*CollectionStats, error)

	// CheckHealth check connection health
	CheckHealth(ctx context.Context) error
}

// ========================================
// GraphRepository 图谱仓储接口
// ========================================

// GraphRepository unified graph repository interface
type GraphRepository interface {
	// ========================================
	// Basic Operations
	// ========================================

	// InitIndexes initialize indexes idempotently
	InitIndexes(ctx context.Context) error

	// AddGraph add graph data (with single transaction and rollback support)
	AddGraph(ctx context.Context, namespace NameSpace, graphs []*GraphData) error

	// AddRelation add single relation
	AddRelation(ctx context.Context, namespace NameSpace, relation *GraphRelation) (*GraphRelation, error)

	// AddNode add single node
	AddNode(ctx context.Context, namespace NameSpace, node *GraphNode) error

	// UpdateNode update node attributes
	UpdateNode(ctx context.Context, namespace NameSpace, node *GraphNode) error

	// UpdateRelation update relation attributes
	UpdateRelation(ctx context.Context, namespace NameSpace, relation *GraphRelation) (*GraphRelation, error)

	// DeleteNode delete single node
	DeleteNode(ctx context.Context, namespace NameSpace, nodeID string) error

	// DeleteRelation delete single relation
	DeleteRelation(ctx context.Context, namespace NameSpace, relationID string) error

	// DeleteByScope delete by scope (kb_id or knowledge_id)
	DeleteByScope(ctx context.Context, scopeType string, scopeID string) error

	// DeleteByChunkID delete by chunk ID
	DeleteByChunkID(ctx context.Context, namespace NameSpace, chunkID string) error

	// DeleteByKnowledgeID delete by knowledge ID
	DeleteByKnowledgeID(ctx context.Context, namespace NameSpace, knowledgeID string) error

	// DeleteGraph delete graph data
	DeleteGraph(ctx context.Context, namespaces []NameSpace) error

	// ========================================
	// Query Operations
	// ========================================

	// GetGraph get complete graph data
	GetGraph(ctx context.Context, namespace NameSpace) (*GraphData, error)

	// SearchNodes search nodes with full-text index support
	SearchNodes(ctx context.Context, namespace NameSpace, query string, opts *NodeQueryOptions) ([]*GraphNode, error)

	// GetNeighbors get neighbor nodes with filters
	GetNeighbors(ctx context.Context, namespace NameSpace, nodeName string, opts *RelationQueryOptions) (*NodeQueryResult, error)

	// FindShortestPath find shortest path between nodes
	FindShortestPath(ctx context.Context, namespace NameSpace, startNode, endNode string, opts *PathQueryOptions) (*PathQueryResult, error)

	// FindKShortestPaths find K shortest paths between nodes
	FindKShortestPaths(ctx context.Context, namespace NameSpace, startNode, endNode string, k int, opts *PathQueryOptions) ([]*PathQueryResult, error)

	// FindPathWithTypes find path with relation type constraints
	FindPathWithTypes(ctx context.Context, namespace NameSpace, startNode, endNode string, relationTypes []string, maxDepth int) ([]*PathQueryResult, error)

	// GetNodeCommunity get community membership for a node
	GetNodeCommunity(ctx context.Context, namespace NameSpace, nodeName string) (*Community, error)

	// GetEntityContext get existing entities for incremental extraction
	GetEntityContext(ctx context.Context, namespace NameSpace) (*ExtractionContext, error)

	// ========================================
	// Statistics Operations
	// ========================================

	// GetGraphStats get basic graph statistics
	GetGraphStats(ctx context.Context, namespace NameSpace) (*GraphStats, error)

	// GetDegreeStats get degree statistics
	GetDegreeStats(ctx context.Context, namespace NameSpace) (*DegreeStats, error)

	// GetTypeDistribution get type distribution statistics
	GetTypeDistribution(ctx context.Context, namespace NameSpace) (*TypeDistribution, error)

	// GetDensityMetrics get density metrics
	GetDensityMetrics(ctx context.Context, namespace NameSpace) (*DensityMetrics, error)

	// GetCentralitySummaries get centrality summaries (pagerank, betweenness)
	GetCentralitySummaries(ctx context.Context, namespace NameSpace, centralityType string) (*CentralitySummary, error)

	// GetCommunityStats get community detection statistics
	GetCommunityStats(ctx context.Context, namespace NameSpace) (*CommunityStats, error)

	// GetStatsByEntityType get statistics filtered by entity type
	GetStatsByEntityType(ctx context.Context, namespace NameSpace, entityType string) (*GraphStats, error)

	// ========================================
	// Analytics Result Storage
	// ========================================

	// StoreCommunity store community detection results
	StoreCommunity(ctx context.Context, namespace NameSpace, community *Community) error

	// StoreCommunityMembers store community member relationships
	StoreCommunityMembers(ctx context.Context, namespace NameSpace, members []*CommunityMember) error

	// UpdateCentralityScores update centrality scores for entity nodes
	UpdateCentralityScores(ctx context.Context, namespace NameSpace, scores map[string]float64, scoreType string) error

	// ========================================
	// Health & Connection
	// ========================================

	// CheckHealth check graph service health
	CheckHealth(ctx context.Context) error

	// Close close connection
	Close(ctx context.Context) error
}

// ========================================
// GraphQueryRepository 图谱查询仓储接口
// ========================================

// GraphQueryRepository graph query repository interface
type GraphQueryRepository interface {
	// GetChunksByGraphNodes get associated chunks by graph node names
	GetChunksByGraphNodes(ctx context.Context, kbID string, nodeNames []string) ([]*Chunk, error)

	// GetChunksByIDs get chunks by chunk IDs
	GetChunksByIDs(ctx context.Context, kbID string, chunkIDs []string) ([]*Chunk, error)

	// GetKnowledgeByGraphNodes get associated knowledge entries by graph node names
	GetKnowledgeByGraphNodes(ctx context.Context, kbID string, nodeNames []string) (*Knowledge, error)

	// GetGraphStats get graph statistics
	GetGraphStats(ctx context.Context, knowledgeBaseID string) (*DetailedGraphStats, error)
}

// ========================================
// Neo4jGraphRepository Neo4j graph repository interface
// ========================================

// Neo4jGraphRepository Neo4j graph repository (specialized for Neo4j operations)
// DEPRECATED: Use GraphRepository instead. This interface is kept for backward compatibility.
type Neo4jGraphRepository interface {
	// AddGraph add graph data
	AddGraph(ctx context.Context, namespace NameSpace, graphs []*GraphData) error

	// AddRelation add single relation
	AddRelation(ctx context.Context, namespace NameSpace, relation *GraphRelation) (*GraphRelation, error)

	// AddNode add single node
	AddNode(ctx context.Context, namespace NameSpace, node *GraphNode) error

	// DeleteNode delete single node
	DeleteNode(ctx context.Context, namespace NameSpace, nodeID string) error

	// DeleteRelation delete single relation
	DeleteRelation(ctx context.Context, namespace NameSpace, relationID string) error

	// DeleteByChunkID delete by chunk ID
	DeleteByChunkID(ctx context.Context, namespace NameSpace, chunkID string) error

	// DeleteByKnowledgeID delete by knowledge ID
	DeleteByKnowledgeID(ctx context.Context, namespace NameSpace, knowledgeID string) error

	// DeleteGraph delete graph data
	DeleteGraph(ctx context.Context, namespaces []NameSpace) error

	// GetGraph get complete graph data
	GetGraph(ctx context.Context, namespace NameSpace) (*GraphData, error)

	// SearchNode search nodes
	SearchNode(ctx context.Context, namespace NameSpace, nodes []string) (*GraphData, error)

	// SearchNodeV2 search nodes (improved - returns node names directly)
	SearchNodeV2(ctx context.Context, namespace NameSpace, nodes []string) (*GraphData, error)

	// SearchPath search path
	SearchPath(ctx context.Context, namespace NameSpace, startNode, endNode string, maxDepth int) ([]*GraphData, error)

	// CheckHealth check Neo4j connection health
	CheckHealth(ctx context.Context) error

	// UpdateNode update node attributes
	UpdateNode(ctx context.Context, namespace NameSpace, node *GraphNode) error

	// UpdateRelation update relation attributes
	UpdateRelation(ctx context.Context, namespace NameSpace, relation *GraphRelation) (*GraphRelation, error)

	// Close close connection
	Close(ctx context.Context) error
}
