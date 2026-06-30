// Package milvus provides Milvus vector storage implementation for semantic cache
package milvus

import (
	"context"
	"fmt"
	"log"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"link/internal/model/cache"
)

const (
	// SemanticCacheCollectionName 语义缓存 Collection 名称
	SemanticCacheCollectionName = "semantic_cache_vectors"

	// SemanticCacheVectorDim 向量维度（text-embedding-3-small）
	SemanticCacheVectorDim = 1536
)

// ========================================
// Cache Vector Repository
// ========================================

// CacheVectorRepository 缓存向量仓储实现
type CacheVectorRepository struct {
	client *milvusclient.Client
}

// NewCacheVectorRepository 创建缓存向量仓储
func NewCacheVectorRepository() (*CacheVectorRepository, error) {
	cli := GetClient()
	if cli == nil {
		return nil, fmt.Errorf("milvus client not initialized")
	}

	return &CacheVectorRepository{
		client: cli,
	}, nil
}

// ========================================
// Collection Management
// ========================================

// CreateCacheCollection 创建语义缓存 Collection
func (r *CacheVectorRepository) CreateCacheCollection(ctx context.Context) error {
	// 构建 schema
	schema := r.buildSchema()

	// 创建 collection
	createOpt := milvusclient.NewCreateCollectionOption(SemanticCacheCollectionName, schema)
	err := r.client.CreateCollection(ctx, createOpt)
	if err != nil {
		return fmt.Errorf("create cache collection failed: %w", err)
	}

	log.Printf("[Milvus] Cache collection created: %s", SemanticCacheCollectionName)
	return nil
}

// buildSchema 构建 Collection Schema
func (r *CacheVectorRepository) buildSchema() *entity.Schema {
	schema := entity.NewSchema().
		WithName(SemanticCacheCollectionName).
		WithDescription("Semantic cache vectors for LLM responses")

	// 主键字段
	schema = schema.WithField(
		entity.NewField().WithName("cache_id").WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(64).WithIsPrimaryKey(true).WithIsAutoID(false),
	)

	// 向量字段
	schema = schema.WithField(
		entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).
			WithDim(SemanticCacheVectorDim),
	)

	// 租户 ID
	schema = schema.WithField(
		entity.NewField().WithName("tenant_id").WithDataType(entity.FieldTypeInt64),
	)

	// Agent 类型
	schema = schema.WithField(
		entity.NewField().WithName("agent_type").WithDataType(entity.FieldTypeVarChar).WithMaxLength(32),
	)

	// 创建时间
	schema = schema.WithField(
		entity.NewField().WithName("created_at").WithDataType(entity.FieldTypeInt64),
	)

	return schema
}

// CreateCacheIndex 创建缓存向量索引
func (r *CacheVectorRepository) CreateCacheIndex(ctx context.Context) error {
	// 使用 HNSW 索引，COSINE 距离度量
	idx := index.NewHNSWIndex(entity.COSINE, 16, 256)

	createIndexOpt := milvusclient.NewCreateIndexOption(
		SemanticCacheCollectionName,
		"vector",
		idx,
	)

	_, err := r.client.CreateIndex(ctx, createIndexOpt)
	if err != nil {
		return fmt.Errorf("create cache index failed: %w", err)
	}

	log.Printf("[Milvus] Cache index created on %s.vector", SemanticCacheCollectionName)
	return nil
}

// LoadCacheCollection 加载缓存 Collection 到内存
func (r *CacheVectorRepository) LoadCacheCollection(ctx context.Context) error {
	loadOpt := milvusclient.NewLoadCollectionOption(SemanticCacheCollectionName)
	task, err := r.client.LoadCollection(ctx, loadOpt)
	if err != nil {
		return fmt.Errorf("load cache collection failed: %w", err)
	}

	// 等待加载完成
	_ = task.Await(ctx)

	log.Printf("[Milvus] Cache collection loaded: %s", SemanticCacheCollectionName)
	return nil
}

// HasCacheCollection 检查缓存 Collection 是否存在
func (r *CacheVectorRepository) HasCacheCollection(ctx context.Context) (bool, error) {
	hasOpt := milvusclient.NewHasCollectionOption(SemanticCacheCollectionName)
	has, err := r.client.HasCollection(ctx, hasOpt)
	if err != nil {
		return false, fmt.Errorf("check cache collection exists failed: %w", err)
	}
	return has, nil
}

// ========================================
// Vector Operations
// ========================================

// Search 搜索相似向量
func (r *CacheVectorRepository) Search(ctx context.Context, vector []float32, tenantID int64, agentType string, topK int) ([]*cache.VectorSearchResult, error) {
	// 构建搜索向量
	vectors := []entity.Vector{entity.FloatVector(vector)}

	// 构建搜索参数（HNSW）
	annParam := index.NewHNSWAnnParam(64)

	// 构建过滤表达式
	expr := r.buildFilterExpr(tenantID, agentType)

	// 执行搜索
	searchOpt := milvusclient.NewSearchOption(SemanticCacheCollectionName, topK, vectors)
	searchOpt.WithANNSField("vector")
	searchOpt.WithFilter(expr)
	searchOpt.WithOutputFields("cache_id")
	searchOpt.WithAnnParam(annParam)

	searchResult, err := r.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 解析结果
	results := make([]*cache.VectorSearchResult, 0)
	for _, resultSet := range searchResult {
		for i := 0; i < resultSet.ResultCount; i++ {
			// 提取 cache_id
			col := resultSet.GetColumn("cache_id")
			if col == nil {
				continue
			}

			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				cacheID, _ := varcharCol.Value(i)
				results = append(results, &cache.VectorSearchResult{
					CacheID:    cacheID,
					Similarity: resultSet.Scores[i],
				})
			}
		}
	}

	return results, nil
}

// Insert 插入向量
func (r *CacheVectorRepository) Insert(ctx context.Context, entry *cache.CacheEntry) error {
	// 构建列数据
	cacheIDs := []string{entry.CacheID}
	vectors := [][]float32{entry.Vector}
	tenantIDs := []int64{entry.TenantID}
	agentTypes := []string{entry.AgentType}
_createdAt := []int64{entry.CreatedAt}

	columns := []column.Column{
		column.NewColumnVarChar("cache_id", cacheIDs),
		column.NewColumnFloatVector("vector", SemanticCacheVectorDim, vectors),
		column.NewColumnInt64("tenant_id", tenantIDs),
		column.NewColumnVarChar("agent_type", agentTypes),
		column.NewColumnInt64("created_at", _createdAt),
	}

	// 插入数据
	insertOpt := milvusclient.NewColumnBasedInsertOption(SemanticCacheCollectionName, columns...)
	_, err := r.client.Insert(ctx, insertOpt)
	if err != nil {
		return fmt.Errorf("insert cache vector failed: %w", err)
	}

	// 刷新以确保可搜索
	flushOpt := milvusclient.NewFlushOption(SemanticCacheCollectionName)
	_, err = r.client.Flush(ctx, flushOpt)
	if err != nil {
		return fmt.Errorf("flush cache collection failed: %w", err)
	}

	log.Printf("[Milvus] Cache vector inserted: %s", entry.CacheID)
	return nil
}

// Delete 删除向量
func (r *CacheVectorRepository) Delete(ctx context.Context, cacheID string) error {
	expr := fmt.Sprintf("cache_id == '%s'", cacheID)

	deleteOpt := milvusclient.NewDeleteOption(SemanticCacheCollectionName)
	deleteOpt.WithExpr(expr)

	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete cache vector failed: %w", err)
	}

	log.Printf("[Milvus] Cache vector deleted: %s", cacheID)
	return nil
}

// DeleteByTenant 删除租户所有向量
func (r *CacheVectorRepository) DeleteByTenant(ctx context.Context, tenantID int64) error {
	expr := fmt.Sprintf("tenant_id == %d", tenantID)

	deleteOpt := milvusclient.NewDeleteOption(SemanticCacheCollectionName)
	deleteOpt.WithExpr(expr)

	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete tenant cache vectors failed: %w", err)
	}

	log.Printf("[Milvus] Tenant cache vectors deleted: tenant_id=%d", tenantID)
	return nil
}

// DeleteByAgent 删除指定 Agent 类型的所有向量
func (r *CacheVectorRepository) DeleteByAgent(ctx context.Context, tenantID int64, agentType string) error {
	expr := fmt.Sprintf("tenant_id == %d && agent_type == '%s'", tenantID, agentType)

	deleteOpt := milvusclient.NewDeleteOption(SemanticCacheCollectionName)
	deleteOpt.WithExpr(expr)

	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete agent cache vectors failed: %w", err)
	}

	log.Printf("[Milvus] Agent cache vectors deleted: tenant_id=%d, agent_type=%s", tenantID, agentType)
	return nil
}

// ========================================
// Helper Methods
// ========================================

// buildFilterExpr 构建过滤表达式
func (r *CacheVectorRepository) buildFilterExpr(tenantID int64, agentType string) string {
	expr := fmt.Sprintf("tenant_id == %d", tenantID)
	if agentType != "" {
		expr += fmt.Sprintf(" && agent_type == '%s'", agentType)
	}
	return expr
}

// ========================================
// Cleanup Operations
// ========================================

// DeleteOrphanVectors 删除孤儿向量（Redis 中已删除的）
func (r *CacheVectorRepository) DeleteOrphanVectors(ctx context.Context, validCacheIDs []string) error {
	// 获取所有向量
	queryOpt := milvusclient.NewQueryOption(SemanticCacheCollectionName)
	queryOpt.WithOutputFields("cache_id")
	queryOpt.WithFilter("cache_id != ''") // 匹配所有

	queryResult, err := r.client.Query(ctx, queryOpt)
	if err != nil {
		return fmt.Errorf("query cache vectors failed: %w", err)
	}

	// 构建有效 ID 集合
	validSet := make(map[string]bool)
	for _, id := range validCacheIDs {
		validSet[id] = true
	}

	// 找出孤儿向量
	orphanIDs := make([]string, 0)
	col := queryResult.GetColumn("cache_id")
	if col != nil {
		if varcharCol, ok := col.(*column.ColumnVarChar); ok {
			for i := 0; i < queryResult.Len(); i++ {
				cacheID, _ := varcharCol.Value(i)
				if !validSet[cacheID] {
					orphanIDs = append(orphanIDs, cacheID)
				}
			}
		}
	}

	// 批量删除孤儿向量
	if len(orphanIDs) > 0 {
		for _, cacheID := range orphanIDs {
			_ = r.Delete(ctx, cacheID)
		}
		log.Printf("[Milvus] Deleted %d orphan vectors", len(orphanIDs))
	}

	return nil
}

// GetCacheCollectionStats 获取缓存 Collection 统计信息
func (r *CacheVectorRepository) GetCacheCollectionStats(ctx context.Context) (map[string]interface{}, error) {
	statsOpt := milvusclient.NewGetCollectionStatsOption(SemanticCacheCollectionName)
	stats, err := r.client.GetCollectionStats(ctx, statsOpt)
	if err != nil {
		return nil, fmt.Errorf("get cache collection stats failed: %w", err)
	}

	return map[string]interface{}{
		"row_count": stats["row_count"],
	}, nil
}

// ========================================
// Initialization Helper
// ========================================

// InitializeCacheCollection 初始化缓存 Collection
func InitializeCacheCollection(ctx context.Context) error {
	repo, err := NewCacheVectorRepository()
	if err != nil {
		return err
	}

	// 检查是否已存在
	has, err := repo.HasCacheCollection(ctx)
	if err != nil {
		return err
	}

	if has {
		log.Printf("[Milvus] Cache collection already exists: %s", SemanticCacheCollectionName)
		// 确保已加载
		return repo.LoadCacheCollection(ctx)
	}

	// 创建 Collection
	if err := repo.CreateCacheCollection(ctx); err != nil {
		return err
	}

	// 创建索引
	if err := repo.CreateCacheIndex(ctx); err != nil {
		return err
	}

	// 加载到内存
	if err := repo.LoadCacheCollection(ctx); err != nil {
		return err
	}

	log.Printf("[Milvus] Cache collection initialized successfully")
	return nil
}
