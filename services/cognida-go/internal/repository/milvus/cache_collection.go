// Package milvus provides Milvus vector storage implementation for semantic cache
package milvus

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"cognida/internal/model/cache"
)

const (
	// SemanticCacheCollectionName 语义缓存 Collection 名称
	SemanticCacheCollectionName = "semantic_cache_vectors"

	// SemanticCacheVectorDim 向量维度默认值（探测失败时的回退；实际维度按 embedder 探测传入）。
	SemanticCacheVectorDim = 1536

	// loadCollectionAwaitCap 加载 Collection 时等待就绪的最长时间上限。
	// 本地 Milvus 偶发无法完成加载（etcd/segment 未就绪），无上界的 Await 会永久挂起
	// （曾致集成测试卡死 17min+）。此上限把"卡死"降级为可观测的超时错误。
	loadCollectionAwaitCap = 90 * time.Second
)

// ========================================
// Cache Vector Repository
// ========================================

// CacheVectorRepository 缓存向量仓储实现
type CacheVectorRepository struct {
	client *milvusclient.Client

	// dim 向量维度。由构造方按 embedder 实际输出探测传入（DashScope text-embedding-v3
	// 默认 1024，而非早期硬编码的 1536）。<=0 时回退到 SemanticCacheVectorDim。
	dim int
}

// effectiveDim 返回生效的向量维度：探测值优先，未设置（<=0）时回退默认常量。
// buildSchema/Insert 统一经此取维度，避免建表维度与写入维度不一致。
func (r *CacheVectorRepository) effectiveDim() int {
	if r.dim > 0 {
		return r.dim
	}
	return SemanticCacheVectorDim
}

// NewCacheVectorRepository 创建缓存向量仓储。
// dim 为向量维度（按 embedder 实际输出探测传入）；传 <=0 时回退 SemanticCacheVectorDim。
func NewCacheVectorRepository(dim int) (*CacheVectorRepository, error) {
	cli := GetClient()
	if cli == nil {
		return nil, fmt.Errorf("milvus client not initialized")
	}

	return &CacheVectorRepository{
		client: cli,
		dim:    dim,
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

	// 向量字段（维度按 embedder 探测生效，见 effectiveDim）
	schema = schema.WithField(
		entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).
			WithDim(int64(r.effectiveDim())),
	)

	// 租户 ID
	schema = schema.WithField(
		entity.NewField().WithName("tenant_id").WithDataType(entity.FieldTypeInt64),
	)

	// Agent 类型
	schema = schema.WithField(
		entity.NewField().WithName("agent_type").WithDataType(entity.FieldTypeVarChar).WithMaxLength(32),
	)

	// KB 归属范围（哨兵逗号包裹格式 ",kb1,kb2,"，为空写 ""）。
	// 用于按知识库失效硬缓存：DeleteByKB 以 like "%,kbID,%" 精确匹配单个 kb_id，
	// 逗号包裹可避免 "kb1" 误匹配 "kb12"。
	schema = schema.WithField(
		entity.NewField().WithName("kb_scope").WithDataType(entity.FieldTypeVarChar).WithMaxLength(1024),
	)

	// 创建时间
	schema = schema.WithField(
		entity.NewField().WithName("created_at").WithDataType(entity.FieldTypeInt64),
	)

	return schema
}

// encodeKBScope 把 KB id 列表编码为哨兵逗号包裹字符串：["kb1","kb2"] -> ",kb1,kb2,"。
// 空列表返回 ""。与 DeleteByKB 的 like "%,kbID,%" 匹配约定配套。
func encodeKBScope(kbIDs []string) string {
	cleaned := make([]string, 0, len(kbIDs))
	for _, id := range kbIDs {
		if s := strings.TrimSpace(id); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	sort.Strings(cleaned) // 稳定顺序，便于排查
	return "," + strings.Join(cleaned, ",") + ","
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

	// 等待加载完成——加上界，防止本地 Milvus 加载卡死时无界阻塞（曾致集成测试挂 17min+）。
	awaitCtx, cancel := context.WithTimeout(ctx, loadCollectionAwaitCap)
	defer cancel()
	if err := task.Await(awaitCtx); err != nil {
		return fmt.Errorf("await cache collection load (cap %s): %w", loadCollectionAwaitCap, err)
	}

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
	kbScopes := []string{encodeKBScope(entry.KBScope)}
	createdAts := []int64{entry.CreatedAt}

	columns := []column.Column{
		column.NewColumnVarChar("cache_id", cacheIDs),
		column.NewColumnFloatVector("vector", r.effectiveDim(), vectors),
		column.NewColumnInt64("tenant_id", tenantIDs),
		column.NewColumnVarChar("agent_type", agentTypes),
		column.NewColumnVarChar("kb_scope", kbScopes),
		column.NewColumnInt64("created_at", createdAts),
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

// DeleteByKB 删除某租户下归属于指定知识库(kbID)的所有缓存向量。
// 供 RAG 硬缓存的按库失效使用：知识库文档写入/删除后调用，避免命中陈旧答案。
// 依赖写入时 kb_scope 的哨兵逗号包裹格式，用 like "%,kbID,%" 精确匹配单个 kb_id。
func (r *CacheVectorRepository) DeleteByKB(ctx context.Context, tenantID int64, kbID string) error {
	kbID = strings.TrimSpace(kbID)
	if kbID == "" {
		return fmt.Errorf("delete cache by kb: empty kbID")
	}
	// kb id 由业务生成（UUID/雪花），不含引号；此处仍拒绝含引号者以防表达式破坏。
	if strings.ContainsAny(kbID, `"'%`) {
		return fmt.Errorf("delete cache by kb: illegal kbID %q", kbID)
	}

	expr := fmt.Sprintf("tenant_id == %d && kb_scope like \"%%,%s,%%\"", tenantID, kbID)

	deleteOpt := milvusclient.NewDeleteOption(SemanticCacheCollectionName)
	deleteOpt.WithExpr(expr)

	if _, err := r.client.Delete(ctx, deleteOpt); err != nil {
		return fmt.Errorf("delete cache vectors by kb failed: %w", err)
	}

	log.Printf("[Milvus] KB cache vectors deleted: tenant_id=%d, kb_id=%s", tenantID, kbID)
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

// InitializeCacheCollection 初始化缓存 Collection。
// dim 为向量维度（按 embedder 探测传入）；<=0 回退 SemanticCacheVectorDim。
func InitializeCacheCollection(ctx context.Context, dim int) error {
	repo, err := NewCacheVectorRepository(dim)
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
