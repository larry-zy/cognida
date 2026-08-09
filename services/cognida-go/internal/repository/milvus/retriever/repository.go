package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"cognida/internal/model/knowledge"
	"cognida/internal/repository/milvus"
)

// VectorRetriever 向量检索器
type VectorRetriever struct {
	client   *milvusclient.Client
	embedder embedding.Embedder
}

// NewVectorRetriever 创建向量检索器
func NewVectorRetriever(embedder embedding.Embedder) (*VectorRetriever, error) {
	if embedder == nil {
		return nil, fmt.Errorf("embedder is required")
	}

	cli := milvus.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("milvus client not initialized")
	}

	return &VectorRetriever{
		client:   cli,
		embedder: embedder,
	}, nil
}

// ========================================
// 知识库 (Collection) 管理
// ========================================

// CreateKnowledgeBaseOptions 创建知识库选项
type CreateKnowledgeBaseOptions struct {
	Dimension     int               // 向量维度
	IndexType     IndexType         // 索引类型
	MetricType    entity.MetricType // 距离度量类型
	AutoID        bool              // 是否自动生成ID
	EnableDynamic bool              // 是否启用动态字段
	Fields        []*entity.Field   // 字段定义
	Description   string            // 描述
	EnableBM25    bool              // 是否启用 BM25 全文搜索
	BM25K1        float64           // BM25 k1 参数（词频饱和度，范围 [1.2, 2.0]，默认 1.2）
	BM25B         float64           // BM25 b 参数（文档长度归一化，范围 [0, 1]，默认 0.75）
}

// IndexType 索引类型
type IndexType string

const (
	IndexTypeFlat           IndexType = "FLAT"
	IndexTypeIvfFlat        IndexType = "IVF_FLAT"
	IndexTypeIvfSq8         IndexType = "IVF_SQ8"
	IndexTypeIvfPq          IndexType = "IVF_PQ"
	IndexTypeHnsw           IndexType = "HNSW"
	IndexTypeDiskAnn        IndexType = "DISKANN"
	IndexTypeAutoIndex      IndexType = "AUTOINDEX"
	IndexTypeScalar         IndexType = "SCALAR"
	IndexTypeSparseInverted IndexType = "SPARSE_INVERTED" // 稀疏向量倒排索引
)

// CreateKnowledgeBase 创建知识库 (Collection)
// 注意：统一使用 "link" collection，kbID 参数仅为兼容性保留
func (r *VectorRetriever) CreateKnowledgeBase(ctx context.Context, kbID int64, opts *CreateKnowledgeBaseOptions) error {
	// 检查 collection 是否已存在
	collectionName := r.getCollectionName(0)
	hasOpt := milvusclient.NewHasCollectionOption(collectionName)
	exists, err := r.client.HasCollection(ctx, hasOpt)
	if err != nil {
		return fmt.Errorf("check collection exists failed: %w", err)
	}
	if exists {
		log.Printf("[Milvus] Collection 'link' already exists, skipping creation")
		return nil
	}

	schema := r.buildSchema(0, opts)

	// 使用新 SDK Option API 创建 collection
	createOpt := milvusclient.NewCreateCollectionOption(collectionName, schema)
	err = r.client.CreateCollection(ctx, createOpt)
	if err != nil {
		return fmt.Errorf("create collection failed: %w", err)
	}

	log.Printf("[Milvus] Collection 'link' created successfully")
	return nil
}

// buildSchema 构建集合 Schema（支持稠密向量和稀疏向量）
// 统一使用 "link" collection
func (r *VectorRetriever) buildSchema(kbID int64, opts *CreateKnowledgeBaseOptions) *entity.Schema {
	collectionName := r.getCollectionName(0)

	// 使用 entity.NewSchema() 创建 schema
	schema := entity.NewSchema().WithName(collectionName).WithDescription(opts.Description)

	// 1. 主键字段
	schema = schema.WithField(
		entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(opts.AutoID),
	)

	// 2. 稠密向量字段（Dense Vector）- 用于语义检索
	schema = schema.WithField(
		entity.NewField().WithName("dense_vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(opts.Dimension)),
	)

	// 3. BM25 全文搜索字段（如果启用）
	if opts.EnableBM25 {
		// text 字段：存储原始文本，用于 BM25 分词
		// 注意：enable_analyzer 需要通过 Milvus 服务端配置，SDK 暂不支持直接设置
		schema = schema.WithField(
			entity.NewField().WithName("text").WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(65535),
		)

		// sparse 字段：BM25 生成的稀疏向量
		schema = schema.WithField(
			entity.NewField().WithName("sparse").WithDataType(entity.FieldTypeSparseVector),
		)
	}

	// 4. 稀疏向量字段（Sparse Vector）- 用于自定义 BM25 关键词匹配（保留兼容性）
	schema = schema.WithField(
		entity.NewField().WithName("sparse_vector").WithDataType(entity.FieldTypeSparseVector),
	)

	// 4. 元数据字段 - chunk_id (UUID string，对应 MySQL chunks.id)
	schema = schema.WithField(
		entity.NewField().WithName("chunk_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(36),
	)

	// 5. 元数据字段 - knowledge_id (UUID string，对应 MySQL chunks.knowledge_id)
	schema = schema.WithField(
		entity.NewField().WithName("knowledge_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(36),
	)

	// 6. 元数据字段 - kb_id (UUID string，对应 MySQL chunks.kb_id)
	schema = schema.WithField(
		entity.NewField().WithName("kb_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(36),
	)

	// 7. 元数据字段 - tenant_id (int64，对应 MySQL chunks.tenant_id)
	schema = schema.WithField(
		entity.NewField().WithName("tenant_id").WithDataType(entity.FieldTypeInt64),
	)

	// 8. 分块索引
	schema = schema.WithField(
		entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64),
	)

	// 9. 分块内容
	schema = schema.WithField(
		entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535),
	)

	// 10. 启用状态
	schema = schema.WithField(
		entity.NewField().WithName("is_enabled").WithDataType(entity.FieldTypeBool),
	)

	// 11. 位置信息
	schema = schema.WithField(
		entity.NewField().WithName("start_at").WithDataType(entity.FieldTypeInt64),
	)
	schema = schema.WithField(
		entity.NewField().WithName("end_at").WithDataType(entity.FieldTypeInt64),
	)

	// 12. Token 数量
	schema = schema.WithField(
		entity.NewField().WithName("token_count").WithDataType(entity.FieldTypeInt64),
	)

	// 13. 动态字段（可选，稀疏向量可作为动态字段添加）
	if opts.EnableDynamic {
		schema = schema.WithDynamicFieldEnabled(true)
	}

	// 注意：BM25 Function 需要在 Milvus 服务端通过 REST API 或配置添加
	// SDK 当前版本暂不支持直接创建 Function
	// 用户需要手动在 Milvus 中配置 BM25 Function 或使用稀疏向量索引

	return schema
}

// HasKnowledgeBase 检查知识库是否存在
func (r *VectorRetriever) HasKnowledgeBase(ctx context.Context, kbID int64) (bool, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	hasOpt := milvusclient.NewHasCollectionOption(collectionName)
	has, err := r.client.HasCollection(ctx, hasOpt)
	if err != nil {
		return false, fmt.Errorf("check collection exists failed: %w", err)
	}

	return has, nil
}

// GetKnowledgeBaseInfo 获取知识库信息
func (r *VectorRetriever) GetKnowledgeBaseInfo(ctx context.Context, kbID int64) (*entity.Collection, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	descOpt := milvusclient.NewDescribeCollectionOption(collectionName)
	coll, err := r.client.DescribeCollection(ctx, descOpt)
	if err != nil {
		return nil, fmt.Errorf("describe collection failed: %w", err)
	}

	return coll, nil
}

// ListKnowledgeBase 列出所有知识库
func (r *VectorRetriever) ListKnowledgeBase(ctx context.Context) ([]*entity.Collection, error) {
	// 新 SDK 的 ListCollections 返回 []string（集合名称），
	// 需要逐个 DescribeCollection 转换为 []*entity.Collection
	names, err := r.client.ListCollections(ctx, milvusclient.NewListCollectionOption())
	if err != nil {
		return nil, fmt.Errorf("list collections failed: %w", err)
	}

	collections := make([]*entity.Collection, 0, len(names))
	for _, name := range names {
		descOpt := milvusclient.NewDescribeCollectionOption(name)
		coll, err := r.client.DescribeCollection(ctx, descOpt)
		if err != nil {
			// 单个集合描述失败不应中断整体列表，记录后跳过
			log.Printf("[Milvus] describe collection %q failed: %v", name, err)
			continue
		}
		collections = append(collections, coll)
	}

	return collections, nil
}

// ========================================
// 索引管理
// ========================================

// IndexOptions 索引选项
type IndexOptions struct {
	IndexType   IndexType         // 索引类型
	MetricType  entity.MetricType // 距离度量类型
	IndexParams map[string]string // 索引参数
	FieldName   string            // 字段名称
}

// CreateIndex 创建索引
func (r *VectorRetriever) CreateIndex(ctx context.Context, kbID int64, opts *IndexOptions) error {
	collectionName := r.getCollectionName(kbID)

	// 设置默认值
	if opts.FieldName == "" {
		opts.FieldName = "dense_vector"
	}
	if opts.MetricType == "" {
		opts.MetricType = entity.L2
	}

	// 构建索引 - 使用新 SDK index 包
	var idx index.Index
	var err error

	switch opts.IndexType {
	case IndexTypeFlat, "":
		idx = index.NewFlatIndex(opts.MetricType)
	case IndexTypeIvfFlat:
		nlist := 128
		if val, ok := opts.IndexParams["nlist"]; ok {
			fmt.Sscanf(val, "%d", &nlist)
		}
		idx = index.NewIvfFlatIndex(opts.MetricType, nlist)
	case IndexTypeIvfSq8:
		nlist := 128
		if val, ok := opts.IndexParams["nlist"]; ok {
			fmt.Sscanf(val, "%d", &nlist)
		}
		idx = index.NewIvfSQ8Index(opts.MetricType, nlist)
	case IndexTypeHnsw:
		M := 16
		efConstruction := 256
		if val, ok := opts.IndexParams["M"]; ok {
			fmt.Sscanf(val, "%d", &M)
		}
		if val, ok := opts.IndexParams["efConstruction"]; ok {
			fmt.Sscanf(val, "%d", &efConstruction)
		}
		idx = index.NewHNSWIndex(opts.MetricType, M, efConstruction)
	case IndexTypeSparseInverted:
		dropRatio := 0.2
		if val, ok := opts.IndexParams["drop_ratio"]; ok {
			fmt.Sscanf(val, "%f", &dropRatio)
		}
		idx = index.NewSparseInvertedIndex(opts.MetricType, dropRatio)
	default:
		// 默认使用 IVF_FLAT
		nlist := 128
		if val, ok := opts.IndexParams["nlist"]; ok {
			fmt.Sscanf(val, "%d", &nlist)
		}
		idx = index.NewIvfFlatIndex(opts.MetricType, nlist)
	}

	// 使用新 SDK Option API 创建索引
	createIndexOpt := milvusclient.NewCreateIndexOption(collectionName, opts.FieldName, idx)
	_, err = r.client.CreateIndex(ctx, createIndexOpt)
	if err != nil {
		return fmt.Errorf("create index failed: %w", err)
	}

	log.Printf("[Milvus] Index created on %s.%s", collectionName, opts.FieldName)
	return nil
}

// DropIndex 删除索引
func (r *VectorRetriever) DropIndex(ctx context.Context, kbID int64, fieldName string) error {
	collectionName := r.getCollectionName(kbID)

	if fieldName == "" {
		fieldName = "dense_vector"
	}

	// 使用新 SDK Option API
	dropIndexOpt := milvusclient.NewDropIndexOption(collectionName, fieldName)
	err := r.client.DropIndex(ctx, dropIndexOpt)
	if err != nil {
		return fmt.Errorf("drop index failed: %w", err)
	}

	log.Printf("[Milvus] Index dropped on %s.%s", collectionName, fieldName)
	return nil
}

// DescribeIndex 描述索引
func (r *VectorRetriever) DescribeIndex(ctx context.Context, kbID int64, fieldName string) (milvusclient.IndexDescription, error) {
	collectionName := r.getCollectionName(kbID)

	if fieldName == "" {
		fieldName = "dense_vector"
	}

	// 使用新 SDK Option API
	descIndexOpt := milvusclient.NewDescribeIndexOption(collectionName, fieldName)
	indexDesc, err := r.client.DescribeIndex(ctx, descIndexOpt)
	if err != nil {
		return milvusclient.IndexDescription{}, fmt.Errorf("describe index failed: %w", err)
	}

	return indexDesc, nil
}

// ========================================
// BM25 索引管理
// ========================================

// BM25IndexOptions BM25 索引选项
type BM25IndexOptions struct {
	K1              float64 // k1 参数：词频饱和度，范围 [1.2, 2.0]，默认 1.2
	B               float64 // b 参数：文档长度归一化，范围 [0, 1]，默认 0.75
	DropRatioSearch float64 // drop_ratio_search 参数：忽略低权重词比例，范围 [0, 1]，默认 0.2
	InvertedAlgo    string  // inverted_index_algo: DAAT_MAXSCORE（高 k 值优化）或 DAAT_WAND（低 k 值优化）
}

// CreateBM25Index 创建 BM25 稀疏向量索引
func (r *VectorRetriever) CreateBM25Index(ctx context.Context, kbID int64, opts *BM25IndexOptions) error {
	collectionName := r.getCollectionName(kbID)

	// 设置默认值
	if opts == nil {
		opts = &BM25IndexOptions{}
	}
	if opts.K1 == 0 {
		opts.K1 = 1.2 // 默认 k1
	}
	if opts.B == 0 {
		opts.B = 0.75 // 默认 b
	}
	if opts.DropRatioSearch == 0 {
		opts.DropRatioSearch = 0.2 // 默认 drop_ratio
	}
	if opts.InvertedAlgo == "" {
		opts.InvertedAlgo = "DAAT_MAXSCORE" // 默认使用 DAAT_MAXSCORE
	}

	// 验证参数范围
	if opts.K1 < 1.2 || opts.K1 > 2.0 {
		return fmt.Errorf("bm25_k1 参数必须在 [1.2, 2.0] 范围内，当前值: %.2f", opts.K1)
	}
	if opts.B < 0 || opts.B > 1 {
		return fmt.Errorf("bm25_b 参数必须在 [0, 1] 范围内，当前值: %.2f", opts.B)
	}
	if opts.DropRatioSearch < 0 || opts.DropRatioSearch > 1 {
		return fmt.Errorf("drop_ratio_search 参数必须在 [0, 1] 范围内，当前值: %.2f", opts.DropRatioSearch)
	}

	// 创建 SPARSE_INVERTED_INDEX 索引，使用 BM25 作为 metric_type
	// 注意：BM25 作为字符串传递，因为 SDK 暂未定义常量
	idx := index.NewSparseInvertedIndex(entity.MetricType("BM25"), opts.DropRatioSearch)

	// 使用新 SDK Option API 创建索引（在 sparse 字段上）
	createIndexOpt := milvusclient.NewCreateIndexOption(collectionName, "sparse", idx)
	_, err := r.client.CreateIndex(ctx, createIndexOpt)
	if err != nil {
		return fmt.Errorf("create BM25 index failed: %w", err)
	}

	log.Printf("[Milvus] BM25 Index created on %s.sparse (k1=%.2f, b=%.2f, drop_ratio=%.2f, algo=%s)",
		collectionName, opts.K1, opts.B, opts.DropRatioSearch, opts.InvertedAlgo)
	return nil
}

// ========================================
// BM25 全文检索
// ========================================

// FullTextSearch 使用 BM25 全文搜索（新 SDK v2.6+）
func (r *VectorRetriever) FullTextSearch(ctx context.Context, kbID int64, query string, topK int, filter map[string]string) ([]*SearchResult, error) {
	collectionName := r.getCollectionName(kbID)

	// 设置默认值
	if topK <= 0 {
		topK = 10
	}

	// 构建过滤表达式
	expr := r.buildFilterExpression(filter)

	// 创建 AnnRequest - 使用 entity.Text() 进行全文搜索
	// 在 sparse 字段上执行 BM25 搜索
	annReq := milvusclient.NewAnnRequest("sparse", topK, entity.Text(query))

	// 添加过滤表达式
	if expr != "" {
		annReq.WithFilter(expr)
	}

	// 创建搜索参数 - BM25 全文搜索参数
	annSearchParams := index.NewCustomAnnParam()
	// drop_ratio 参数可以在创建索引时配置
	annReq.WithAnnParam(annSearchParams)

	// 使用 HybridSearch API 执行 BM25 全文搜索
	hybridSearchOpt := milvusclient.NewHybridSearchOption(collectionName, topK, annReq)
	resultSets, err := r.client.HybridSearch(ctx, hybridSearchOpt)
	if err != nil {
		// 如果 BM25 搜索失败，可能是 Milvus 版本不支持或索引未创建
		// 返回空结果而不是错误，让上层可以 fallback
		log.Printf("[Milvus] BM25 full-text search failed (possibly not supported or index not created): %v", err)
		return []*SearchResult{}, nil
	}

	// 解析结果 - 新 SDK 返回的是 []ResultSet
	results := make([]*SearchResult, 0)
	for _, resultSet := range resultSets {
		for i := 0; i < resultSet.ResultCount; i++ {
			result := &SearchResult{
				Score: resultSet.Scores[i], // BM25 分数
			}

			// 提取字段值 - 新 SDK 使用 GetColumn() 方法
			// 输出字段列表
			outputFields := []string{"chunk_id", "knowledge_id", "kb_id", "tenant_id", "chunk_index", "content", "is_enabled", "start_at", "end_at", "token_count"}
			for _, fieldName := range outputFields {
				col := resultSet.GetColumn(fieldName)
				if col == nil {
					continue
				}

				// 根据字段类型提取值 - 使用新 SDK column API
				switch fieldName {
				case "chunk_id", "knowledge_id", "kb_id":
					if varcharCol, ok := col.(*column.ColumnVarChar); ok {
						val, _ := varcharCol.Value(i)
						switch fieldName {
						case "chunk_id":
							result.ChunkID = val
						case "knowledge_id":
							result.KnowledgeID = val
						case "kb_id":
							result.KnowledgeBaseID = val
						}
					}
				case "tenant_id", "chunk_index", "start_at", "end_at", "token_count":
					if intCol, ok := col.(*column.ColumnInt64); ok {
						val, _ := intCol.Value(i)
						switch fieldName {
						case "tenant_id":
							result.TenantID = val
						case "chunk_index":
							result.ChunkIndex = int(val)
						case "start_at":
							result.StartAt = val
						case "end_at":
							result.EndAt = val
						case "token_count":
							result.TokenCount = val
						}
					}
				case "content":
					if varcharCol, ok := col.(*column.ColumnVarChar); ok {
						val, _ := varcharCol.Value(i)
						result.Content = val
					}
				case "is_enabled":
					if boolCol, ok := col.(*column.ColumnBool); ok {
						val, _ := boolCol.Value(i)
						result.IsEnabled = val
					}
				}
			}

			results = append(results, result)
		}
	}
	log.Printf("[Milvus] BM25 full-text search completed: %d results for query '%s'", len(results), query)
	return results, nil
}

// buildFilterExpression 构建过滤表达式
func (r *VectorRetriever) buildFilterExpression(filter map[string]string) string {
	if len(filter) == 0 {
		return ""
	}

	expr := ""
	for k, v := range filter {
		if expr != "" {
			expr += " && "
		}

		// 根据字段类型构建表达式
		switch k {
		case "tenant_id":
			expr += fmt.Sprintf("%s == %s", k, v)
		case "kb_id", "knowledge_id", "chunk_id":
			expr += fmt.Sprintf("%s == '%s'", k, v)
		case "is_enabled":
			if v == "true" {
				expr += "is_enabled == true"
			} else {
				expr += "is_enabled == false"
			}
		default:
			// 默认字符串相等
			expr += fmt.Sprintf("%s == '%s'", k, v)
		}
	}

	return expr
}

// ========================================
// 向量查询
// ========================================

// SearchResult 搜索结果（适配新 schema）
type SearchResult struct {
	ID          int64
	Score       float32
	ChunkID     string // 分块 ID (MySQL chunks.id)
	KnowledgeID string // 知识条目 ID
	KnowledgeBaseID        string // 知识库 ID
	TenantID    int64  // 租户 ID
	ChunkIndex  int    // 分块索引
	Content     string // 分块内容
	IsEnabled   bool   // 是否启用
	StartAt     int64  // 起始位置
	EndAt       int64  // 结束位置
	TokenCount  int64  // Token 数量
}

// SearchOptions 搜索选项（适配新 schema）
type SearchOptions struct {
	TopK             int                     // 返回结果数量
	ScoreThreshold   float32                 // 相似度阈值
	MetricType       entity.MetricType       // 距离度量类型
	Expr             string                  // 过滤表达式
	OutputFields     []string                // 输出字段
	ConsistencyLevel entity.ConsistencyLevel // 一致性级别
	VectorFieldName  string                  // 向量字段名称（默认 "dense_vector"）
	SearchParams     map[string]interface{}  // 搜索参数
	IndexType        IndexType               // 索引类型
}

// Search 向量搜索
func (r *VectorRetriever) Search(ctx context.Context, kbID int64, queryText string, opts *SearchOptions) ([]*SearchResult, error) {
	// 1. 生成查询向量
	embeddings, err := r.embedder.EmbedStrings(ctx, []string{queryText})
	if err != nil {
		return nil, fmt.Errorf("embed query failed: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings generated")
	}

	// 2. 转换为 float32
	vectors := make([][]float32, len(embeddings))
	for i, emb := range embeddings {
		vectors[i] = make([]float32, len(emb))
		for j, val := range emb {
			vectors[i][j] = float32(val)
		}
	}

	// 3. 执行搜索
	return r.SearchVectors(ctx, kbID, vectors[0], opts)
}

// SearchVectors 直接使用向量搜索（适配新 schema）
func (r *VectorRetriever) SearchVectors(ctx context.Context, kbID int64, vector []float32, opts *SearchOptions) ([]*SearchResult, error) {
	collectionName := r.getCollectionName(kbID)

	// 设置默认值
	if opts == nil {
		opts = &SearchOptions{}
	}
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.MetricType == "" {
		opts.MetricType = entity.L2
	}
	if opts.VectorFieldName == "" {
		opts.VectorFieldName = "dense_vector" // 默认使用稠密向量
	}
	if len(opts.OutputFields) == 0 {
		opts.OutputFields = []string{"chunk_id", "knowledge_id", "kb_id", "tenant_id", "chunk_index", "content", "is_enabled", "start_at", "end_at", "token_count"}
	}

	// 构建搜索向量
	vectors := []entity.Vector{
		entity.FloatVector(vector),
	}

	// 构建搜索参数 - 使用新 SDK index.AnnParam
	var annParam index.AnnParam
	switch opts.IndexType {
	case IndexTypeIvfFlat, IndexTypeIvfSq8:
		nprobe := 64
		if val, ok := opts.SearchParams["nprobe"]; ok {
			if v, ok := val.(float64); ok {
				nprobe = int(v)
			}
		}
		annParam = index.NewIvfAnnParam(nprobe)
	case IndexTypeHnsw:
		ef := 64
		if val, ok := opts.SearchParams["ef"]; ok {
			if v, ok := val.(float64); ok {
				ef = int(v)
			}
		}
		annParam = index.NewHNSWAnnParam(ef)
	default:
		// 使用默认搜索参数
		annParam = index.NewCustomAnnParam()
	}

	// 使用新 SDK Option API 执行搜索
	searchOpt := milvusclient.NewSearchOption(collectionName, opts.TopK, vectors)
	searchOpt.WithANNSField(opts.VectorFieldName)
	if opts.Expr != "" {
		searchOpt.WithFilter(opts.Expr)
	}
	searchOpt.WithOutputFields(opts.OutputFields...)
	searchOpt.WithAnnParam(annParam)

	searchResult, err := r.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// 解析结果 - 新 SDK 返回 []ResultSet
	results := make([]*SearchResult, 0)
	for _, resultSet := range searchResult {
		for i := 0; i < resultSet.ResultCount; i++ {
			// 应用阈值过滤
			if opts.ScoreThreshold > 0 && resultSet.Scores[i] < opts.ScoreThreshold {
				continue
			}

			result := &SearchResult{
				Score: resultSet.Scores[i],
			}

			// 提取字段值 - 新 SDK 使用 GetColumn() 方法
			for _, fieldName := range opts.OutputFields {
				col := resultSet.GetColumn(fieldName)
				if col == nil {
					continue
				}

				// 根据字段类型提取值 - 使用新 SDK column API
				switch fieldName {
				case "id":
					if intCol, ok := col.(*column.ColumnInt64); ok {
						val, _ := intCol.Value(i)
						result.ID = val
					}
				case "chunk_id", "knowledge_id", "kb_id":
					if varcharCol, ok := col.(*column.ColumnVarChar); ok {
						val, _ := varcharCol.Value(i)
						switch fieldName {
						case "chunk_id":
							result.ChunkID = val
						case "knowledge_id":
							result.KnowledgeID = val
						case "kb_id":
							result.KnowledgeBaseID = val
						}
					}
				case "tenant_id", "chunk_index", "start_at", "end_at", "token_count":
					if intCol, ok := col.(*column.ColumnInt64); ok {
						val, _ := intCol.Value(i)
						switch fieldName {
						case "tenant_id":
							result.TenantID = val
						case "chunk_index":
							result.ChunkIndex = int(val)
						case "start_at":
							result.StartAt = val
						case "end_at":
							result.EndAt = val
						case "token_count":
							result.TokenCount = val
						}
					}
				case "content":
					if varcharCol, ok := col.(*column.ColumnVarChar); ok {
						val, _ := varcharCol.Value(i)
						result.Content = val
					}
				case "is_enabled":
					if boolCol, ok := col.(*column.ColumnBool); ok {
						val, _ := boolCol.Value(i)
						result.IsEnabled = val
					}
				}
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// SearchBatchOptions 批量搜索选项
type SearchBatchOptions struct {
	TopK             int                     // 返回结果数量
	ScoreThreshold   float32                 // 相似度阈值
	MetricType       entity.MetricType       // 距离度量类型
	Expr             string                  // 过滤表达式
	OutputFields     []string                // 输出字段
	ConsistencyLevel entity.ConsistencyLevel // 一致性级别
	VectorFieldName  string                  // 向量字段名称
	SearchParams     map[string]interface{}  // 搜索参数
	IndexType        IndexType               // 索引类型
}

// BatchSearch 批量向量搜索
func (r *VectorRetriever) BatchSearch(ctx context.Context, kbID int64, queryTexts []string, opts *SearchBatchOptions) ([][]*SearchResult, error) {
	if len(queryTexts) == 0 {
		return nil, fmt.Errorf("query texts cannot be empty")
	}

	// 1. 生成查询向量
	embeddings, err := r.embedder.EmbedStrings(ctx, queryTexts)
	if err != nil {
		return nil, fmt.Errorf("embed queries failed: %w", err)
	}

	// 2. 转换为 float32
	vectors := make([][]float32, len(embeddings))
	for i, emb := range embeddings {
		vectors[i] = make([]float32, len(emb))
		for j, val := range emb {
			vectors[i][j] = float32(val)
		}
	}

	// 3. 执行批量搜索
	return r.BatchSearchVectors(ctx, kbID, vectors, opts)
}

// BatchSearchVectors 批量向量搜索（直接使用向量，适配新 schema）
func (r *VectorRetriever) BatchSearchVectors(ctx context.Context, kbID int64, vectors [][]float32, opts *SearchBatchOptions) ([][]*SearchResult, error) {
	collectionName := r.getCollectionName(kbID)

	// 设置默认值
	if opts == nil {
		opts = &SearchBatchOptions{}
	}
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.MetricType == "" {
		opts.MetricType = entity.L2
	}
	if opts.VectorFieldName == "" {
		opts.VectorFieldName = "dense_vector" // 默认使用稠密向量
	}
	if len(opts.OutputFields) == 0 {
		opts.OutputFields = []string{"chunk_id", "knowledge_id", "kb_id", "tenant_id", "chunk_index", "content", "is_enabled", "start_at", "end_at", "token_count"}
	}

	// 构建搜索向量
	searchVectors := make([]entity.Vector, len(vectors))
	for i, vec := range vectors {
		searchVectors[i] = entity.FloatVector(vec)
	}

	// 构建搜索参数 - 使用新 SDK index.AnnParam
	var annParam index.AnnParam
	switch opts.IndexType {
	case IndexTypeIvfFlat, IndexTypeIvfSq8:
		nprobe := 64
		if val, ok := opts.SearchParams["nprobe"]; ok {
			if v, ok := val.(float64); ok {
				nprobe = int(v)
			}
		}
		annParam = index.NewIvfAnnParam(nprobe)
	case IndexTypeHnsw:
		ef := 64
		if val, ok := opts.SearchParams["ef"]; ok {
			if v, ok := val.(float64); ok {
				ef = int(v)
			}
		}
		annParam = index.NewHNSWAnnParam(ef)
	default:
		// 使用默认搜索参数
		annParam = index.NewCustomAnnParam()
	}

	// 使用新 SDK Option API 执行批量搜索
	searchOpt := milvusclient.NewSearchOption(collectionName, opts.TopK, searchVectors)
	searchOpt.WithANNSField(opts.VectorFieldName)
	if opts.Expr != "" {
		searchOpt.WithFilter(opts.Expr)
	}
	searchOpt.WithOutputFields(opts.OutputFields...)
	searchOpt.WithAnnParam(annParam)

	searchResults, err := r.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("batch search failed: %w", err)
	}

	// 解析结果 - 新 SDK 返回 []ResultSet
	allResults := make([][]*SearchResult, len(vectors))
	for queryIdx, resultSet := range searchResults {
		results := make([]*SearchResult, 0)
		for i := 0; i < resultSet.ResultCount; i++ {
			// 应用阈值过滤
			if opts.ScoreThreshold > 0 && resultSet.Scores[i] < opts.ScoreThreshold {
				continue
			}

			result := &SearchResult{
				Score: resultSet.Scores[i],
			}

			// 提取字段值 - 新 SDK 使用 GetColumn() 方法
			for _, fieldName := range opts.OutputFields {
				col := resultSet.GetColumn(fieldName)
				if col == nil {
					continue
				}

				// 根据字段类型提取值 - 使用新 SDK column API
				switch fieldName {
				case "id":
					if intCol, ok := col.(*column.ColumnInt64); ok {
						val, _ := intCol.Value(i)
						result.ID = val
					}
				case "chunk_id", "knowledge_id", "kb_id":
					if varcharCol, ok := col.(*column.ColumnVarChar); ok {
						val, _ := varcharCol.Value(i)
						switch fieldName {
						case "chunk_id":
							result.ChunkID = val
						case "knowledge_id":
							result.KnowledgeID = val
						case "kb_id":
							result.KnowledgeBaseID = val
						}
					}
				case "tenant_id", "chunk_index", "start_at", "end_at", "token_count":
					if intCol, ok := col.(*column.ColumnInt64); ok {
						val, _ := intCol.Value(i)
						switch fieldName {
						case "tenant_id":
							result.TenantID = val
						case "chunk_index":
							result.ChunkIndex = int(val)
						case "start_at":
							result.StartAt = val
						case "end_at":
							result.EndAt = val
						case "token_count":
							result.TokenCount = val
						}
					}
				case "content":
					if varcharCol, ok := col.(*column.ColumnVarChar); ok {
						val, _ := varcharCol.Value(i)
						result.Content = val
					}
				case "is_enabled":
					if boolCol, ok := col.(*column.ColumnBool); ok {
						val, _ := boolCol.Value(i)
						result.IsEnabled = val
					}
				}
			}

			results = append(results, result)
		}
		allResults[queryIdx] = results
	}

	return allResults, nil
}

// ========================================
// 数据管理
// ========================================

// InsertData 插入数据
func (r *VectorRetriever) InsertData(ctx context.Context, kbID int64, docs []*DocumentData) error {
	if len(docs) == 0 {
		return fmt.Errorf("documents cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)

	// 构建列数据
	columns := r.buildColumns(docs)

	// 使用新 SDK Option API 插入数据
	insertOpt := milvusclient.NewColumnBasedInsertOption(collectionName, columns...)
	_, err := r.client.Insert(ctx, insertOpt)
	if err != nil {
		return fmt.Errorf("insert data failed: %w", err)
	}

	// 使用新 SDK Option API 刷新数据以确保可搜索
	flushOpt := milvusclient.NewFlushOption(collectionName)
	_, err = r.client.Flush(ctx, flushOpt)
	if err != nil {
		return fmt.Errorf("flush collection failed: %w", err)
	}

	log.Printf("[Milvus] Inserted %d documents into %s", len(docs), collectionName)
	return nil
}

// DocumentData 文档数据（支持稠密向量和稀疏向量）
type DocumentData struct {
	ID           int64                  // 主键 ID（可选，AutoID 时可省略）
	DenseVector  []float32              // 稠密向量（语义检索）
	SparseVector entity.SparseEmbedding // 稀疏向量（BM25 关键词匹配）
	Text         string                 // 原始文本（用于 BM25 全文搜索）
	ChunkID      string                 // 对应 MySQL chunks.id
	KnowledgeID  string                 // 对应 MySQL chunks.knowledge_id
	KnowledgeBaseID         string                 // 对应 MySQL chunks.kb_id
	TenantID     int64                  // 对应 MySQL chunks.tenant_id
	ChunkIndex   int                    // 分块索引
	Content      string                 // 分块内容
	IsEnabled    bool                   // 是否启用
	StartAt      int64                  // 起始位置
	EndAt        int64                  // 结束位置
	TokenCount   int64                  // Token 数量
}

// buildColumns 构建插入数据的列（支持稠密向量和稀疏向量）
func (r *VectorRetriever) buildColumns(docs []*DocumentData) []column.Column {
	ids := make([]int64, len(docs))
	denseVectors := make([][]float32, len(docs))
	sparseVectors := make([]entity.SparseEmbedding, len(docs))
	texts := make([]string, len(docs)) // BM25 全文搜索字段
	chunkIDs := make([]string, len(docs))
	knowledgeIDs := make([]string, len(docs))
	kbIDs := make([]string, len(docs))
	tenantIDs := make([]int64, len(docs))
	chunkIndexes := make([]int64, len(docs))
	contents := make([]string, len(docs))
	isEnableds := make([]bool, len(docs))
	startAts := make([]int64, len(docs))
	endAts := make([]int64, len(docs))
	tokenCounts := make([]int64, len(docs))

	for i, doc := range docs {
		ids[i] = doc.ID
		denseVectors[i] = doc.DenseVector
		sparseVectors[i] = doc.SparseVector
		texts[i] = doc.Text // BM25 文本字段
		chunkIDs[i] = doc.ChunkID
		knowledgeIDs[i] = doc.KnowledgeID
		kbIDs[i] = doc.KnowledgeBaseID
		tenantIDs[i] = doc.TenantID
		chunkIndexes[i] = int64(doc.ChunkIndex)
		contents[i] = doc.Content
		isEnableds[i] = doc.IsEnabled
		startAts[i] = doc.StartAt
		endAts[i] = doc.EndAt
		tokenCounts[i] = doc.TokenCount
	}

	dim := len(denseVectors[0])
	columns := []column.Column{
		column.NewColumnInt64("id", ids),
		column.NewColumnFloatVector("dense_vector", dim, denseVectors),
		column.NewColumnVarChar("text", texts),                                   // BM25 文本字段
		column.NewColumnSparseVectors("sparse", sparseVectors),                   // BM25 稀疏向量字段（由 Function 自动生成）
		column.NewColumnSparseVectors("sparse_vector", sparseVectors),             // 保留兼容性
		column.NewColumnVarChar("chunk_id", chunkIDs),
		column.NewColumnVarChar("knowledge_id", knowledgeIDs),
		column.NewColumnVarChar("kb_id", kbIDs),
		column.NewColumnInt64("tenant_id", tenantIDs),
		column.NewColumnInt64("chunk_index", chunkIndexes),
		column.NewColumnVarChar("content", contents),
		column.NewColumnBool("is_enabled", isEnableds),
		column.NewColumnInt64("start_at", startAts),
		column.NewColumnInt64("end_at", endAts),
		column.NewColumnInt64("token_count", tokenCounts),
	}
	return columns
}

// DeleteData 删除数据
func (r *VectorRetriever) DeleteData(ctx context.Context, kbID int64, ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("ids cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)

	// 构建删除表达式
	idStr := ""
	for i, id := range ids {
		if i > 0 {
			idStr += ", "
		}
		idStr += fmt.Sprintf("%d", id)
	}
	expr := fmt.Sprintf("id in [%s]", idStr)

	// 使用新 SDK Option API 执行删除
	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete data failed: %w", err)
	}

	log.Printf("[Milvus] Deleted %d records from %s", len(ids), collectionName)
	return nil
}

// DeleteByExpr 按表达式删除数据
func (r *VectorRetriever) DeleteByExpr(ctx context.Context, kbID int64, expr string) error {
	if expr == "" {
		return fmt.Errorf("expression cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete by expression failed: %w", err)
	}

	log.Printf("[Milvus] Deleted by expr '%s' from %s", expr, collectionName)
	return nil
}

// DeleteByChunkID 按 chunk_id 删除向量数据
func (r *VectorRetriever) DeleteByChunkID(ctx context.Context, kbID int64, chunkID string) error {
	if chunkID == "" {
		return fmt.Errorf("chunk_id cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)
	// chunk_id 是 VarChar 字段，使用字符串相等匹配
	expr := fmt.Sprintf("chunk_id == '%s'", chunkID)

	// 使用新 SDK Option API
	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete by chunk_id failed: %w", err)
	}

	log.Printf("[Milvus] Deleted chunk_id='%s' from %s", chunkID, collectionName)
	return nil
}

// DeleteByKnowledgeID 按 knowledge_id 删除所有相关向量数据
func (r *VectorRetriever) DeleteByKnowledgeID(ctx context.Context, kbID int64, knowledgeID string) error {
	if knowledgeID == "" {
		return fmt.Errorf("knowledge_id cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)
	// knowledge_id 是 VarChar 字段，使用字符串相等匹配
	expr := fmt.Sprintf("knowledge_id == '%s'", knowledgeID)

	// 使用新 SDK Option API
	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete by knowledge_id failed: %w", err)
	}

	log.Printf("[Milvus] Deleted knowledge_id='%s' from %s", knowledgeID, collectionName)
	return nil
}

// DeleteByTenantID 按租户ID删除所有向量数据（用于租户删除场景）
func (r *VectorRetriever) DeleteByTenantID(ctx context.Context, tenantID int64) error {
	if tenantID <= 0 {
		return fmt.Errorf("invalid tenant_id: %d", tenantID)
	}

	// 使用新 SDK Option API 获取所有集合
	listOpt := milvusclient.NewListCollectionOption()
	collectionNames, err := r.client.ListCollections(ctx, listOpt)
	if err != nil {
		return fmt.Errorf("list collections failed: %w", err)
	}

	deletedCount := 0
	for _, collName := range collectionNames {
		// 使用新 SDK Option API 获取集合信息
		descOpt := milvusclient.NewDescribeCollectionOption(collName)
		collInfo, err := r.client.DescribeCollection(ctx, descOpt)
		if err != nil {
			log.Printf("[Milvus] Warning: failed to describe collection %s: %v", collName, err)
			continue
		}

		// 检查是否包含 tenant_id 字段
		hasTenantIDField := false
		for _, field := range collInfo.Schema.Fields {
			if field.Name == "tenant_id" {
				hasTenantIDField = true
				break
			}
		}

		if !hasTenantIDField {
			continue
		}

		// 使用新 SDK Option API 删除该租户的数据
		expr := fmt.Sprintf("tenant_id == %d", tenantID)
		deleteOpt := milvusclient.NewDeleteOption(collName)
		deleteOpt.WithExpr(expr)
		_, err = r.client.Delete(ctx, deleteOpt)
		if err != nil {
			log.Printf("[Milvus] Warning: failed to delete tenant_id=%d from %s: %v", tenantID, collName, err)
			continue
		}
		deletedCount++
		log.Printf("[Milvus] Deleted tenant_id=%d data from collection %s", tenantID, collName)
	}

	log.Printf("[Milvus] Deleted tenant_id=%d data from %d collections", tenantID, deletedCount)
	return nil
}

// DeleteByChunkIDs 批量按 chunk_id 删除向量数据
func (r *VectorRetriever) DeleteByChunkIDs(ctx context.Context, kbID int64, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return fmt.Errorf("chunk_ids cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)

	// 构建删除表达式（批量）
	var chunkIDExprs []string
	for _, chunkID := range chunkIDs {
		chunkIDExprs = append(chunkIDExprs, fmt.Sprintf("'%s'", chunkID))
	}
	expr := fmt.Sprintf("chunk_id in [%s]", strings.Join(chunkIDExprs, ", "))

	// 使用新 SDK Option API
	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete by chunk_ids failed: %w", err)
	}

	log.Printf("[Milvus] Deleted %d chunk_ids from %s", len(chunkIDs), collectionName)
	return nil
}

// DeleteByKnowledgeIDs 批量按 knowledge_id 删除向量数据
func (r *VectorRetriever) DeleteByKnowledgeIDs(ctx context.Context, kbID int64, knowledgeIDs []string) error {
	if len(knowledgeIDs) == 0 {
		return fmt.Errorf("knowledge_ids cannot be empty")
	}

	collectionName := r.getCollectionName(kbID)

	// 构建删除表达式（批量）
	var knowledgeIDExprs []string
	for _, knowledgeID := range knowledgeIDs {
		knowledgeIDExprs = append(knowledgeIDExprs, fmt.Sprintf("'%s'", knowledgeID))
	}
	expr := fmt.Sprintf("knowledge_id in [%s]", strings.Join(knowledgeIDExprs, ", "))

	// 使用新 SDK Option API
	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := r.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("delete by knowledge_ids failed: %w", err)
	}

	log.Printf("[Milvus] Deleted %d knowledge_ids from %s", len(knowledgeIDs), collectionName)
	return nil
}

// GetDeleteStats 获取删除统计信息（用于验证删除操作）
func (r *VectorRetriever) GetDeleteStats(ctx context.Context, kbID int64) (map[string]int64, error) {
	collectionName := r.getCollectionName(kbID)

	// 获取集合统计信息 - 使用新 SDK GetCollectionStats API
	statsOpt := milvusclient.NewGetCollectionStatsOption(collectionName)
	stats, err := r.client.GetCollectionStats(ctx, statsOpt)
	if err != nil {
		return nil, fmt.Errorf("get collection statistics failed: %w", err)
	}

	result := make(map[string]int64)
	// 解析统计信息（GetCollectionStats 返回 map[string]string）
	for k, v := range stats {
		// 尝试将字符串转换为 int64
		var count int64
		_, err := fmt.Sscanf(v, "%d", &count)
		if err == nil {
			result[k] = count
		}
	}

	return result, nil
}

// QueryOptions 查询选项
type QueryOptions struct {
	Expr         []string // 过滤表达式（取 Expr[0] 作为 filter）
	OutputFields []string // 输出字段
	Limit        int64    // 限制数量
	Offset       int64    // 偏移量
}

// Query 查询数据
func (r *VectorRetriever) Query(ctx context.Context, kbID int64, opts *QueryOptions) ([]*DocumentData, error) {
	collectionName := r.getCollectionName(kbID)

	// 设置默认值
	if opts == nil {
		opts = &QueryOptions{}
	}
	if len(opts.Expr) == 0 {
		opts.Expr = []string{"id >= 0"} // 匹配所有
	}
	if len(opts.OutputFields) == 0 {
		// 与写入 schema 保持一致的真实字段（原默认含 document_id/metadata 两个本集合并不存在的字段，查询必空）。
		opts.OutputFields = []string{
			"id", "chunk_id", "knowledge_id", "kb_id", "tenant_id",
			"chunk_index", "content", "is_enabled", "start_at", "end_at", "token_count",
		}
	}

	// 使用新 SDK Option API 执行查询
	queryOpt := milvusclient.NewQueryOption(collectionName)
	queryOpt.WithFilter(opts.Expr[0])
	queryOpt.WithOutputFields(opts.OutputFields...)
	if opts.Limit > 0 {
		queryOpt.WithLimit(int(opts.Limit))
	}
	if opts.Offset > 0 {
		queryOpt.WithOffset(int(opts.Offset))
	}
	queryResult, err := r.client.Query(ctx, queryOpt)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// 解析结果 - 新 SDK 返回单个 ResultSet，按列(GetColumn)+行下标(Value)取值，
	// 字段类型映射与 Search 解析保持一致，避免出错时返回空 docs 造成静默失效。
	rowCount := queryResult.Len()
	docs := make([]*DocumentData, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		doc := &DocumentData{}
		for _, fieldName := range opts.OutputFields {
			col := queryResult.GetColumn(fieldName)
			if col == nil {
				continue
			}
			switch fieldName {
			case "id":
				if intCol, ok := col.(*column.ColumnInt64); ok {
					val, _ := intCol.Value(i)
					doc.ID = val
				}
			case "chunk_id", "knowledge_id", "kb_id":
				if varcharCol, ok := col.(*column.ColumnVarChar); ok {
					val, _ := varcharCol.Value(i)
					switch fieldName {
					case "chunk_id":
						doc.ChunkID = val
					case "knowledge_id":
						doc.KnowledgeID = val
					case "kb_id":
						doc.KnowledgeBaseID = val
					}
				}
			case "tenant_id", "chunk_index", "start_at", "end_at", "token_count":
				if intCol, ok := col.(*column.ColumnInt64); ok {
					val, _ := intCol.Value(i)
					switch fieldName {
					case "tenant_id":
						doc.TenantID = val
					case "chunk_index":
						doc.ChunkIndex = int(val)
					case "start_at":
						doc.StartAt = val
					case "end_at":
						doc.EndAt = val
					case "token_count":
						doc.TokenCount = val
					}
				}
			case "text":
				if varcharCol, ok := col.(*column.ColumnVarChar); ok {
					val, _ := varcharCol.Value(i)
					doc.Text = val
				}
			case "content":
				if varcharCol, ok := col.(*column.ColumnVarChar); ok {
					val, _ := varcharCol.Value(i)
					doc.Content = val
				}
			case "is_enabled":
				if boolCol, ok := col.(*column.ColumnBool); ok {
					val, _ := boolCol.Value(i)
					doc.IsEnabled = val
				}
			}
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// ========================================
// 字段管理
// ========================================

// AddField 添加字段
// 注意：Milvus 不支持直接向已有 collection 添加字段
func (r *VectorRetriever) AddField(ctx context.Context, kbID int64, fieldName string, fieldType entity.FieldType) error {
	return fmt.Errorf("Milvus does not support adding fields to existing collection. Use dynamic fields or recreate collection")
}

// UpdateField 更新字段
// 注意：Milvus 不支持修改字段
func (r *VectorRetriever) UpdateField(ctx context.Context, kbID int64, fieldName string) error {
	return fmt.Errorf("Milvus does not support updating field definitions")
}

// DropField 删除字段
// 注意：Milvus 不支持删除字段
func (r *VectorRetriever) DropField(ctx context.Context, kbID int64, fieldName string) error {
	return fmt.Errorf("Milvus does not support dropping fields. Use dynamic fields or recreate collection")
}

// ========================================
// 加载和释放
// ========================================

// LoadKnowledgeBase 加载知识库到内存
func (r *VectorRetriever) LoadKnowledgeBase(ctx context.Context, kbID int64, async bool) error {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	loadOpt := milvusclient.NewLoadCollectionOption(collectionName)
	task, err := r.client.LoadCollection(ctx, loadOpt)
	if err != nil {
		return fmt.Errorf("load collection failed: %w", err)
	}

	// 如果不是异步模式，等待加载完成
	if !async {
		// 新 SDK 的 LoadTask 可用于等待加载完成
		_ = task.Await(ctx)
	}

	log.Printf("[Milvus] Collection loaded: %s", collectionName)
	return nil
}

// ReleaseKnowledgeBase 释放知识库内存
func (r *VectorRetriever) ReleaseKnowledgeBase(ctx context.Context, kbID int64) error {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	releaseOpt := milvusclient.NewReleaseCollectionOption(collectionName)
	err := r.client.ReleaseCollection(ctx, releaseOpt)
	if err != nil {
		return fmt.Errorf("release collection failed: %w", err)
	}

	log.Printf("[Milvus] Collection released: %s", collectionName)
	return nil
}

// GetLoadingProgress 获取加载进度
func (r *VectorRetriever) GetLoadingProgress(ctx context.Context, kbID int64) (int64, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API - GetLoadState 返回 LoadState 结构
	loadStateOpt := milvusclient.NewGetLoadStateOption(collectionName)
	loadState, err := r.client.GetLoadState(ctx, loadStateOpt)
	if err != nil {
		return 0, fmt.Errorf("get loading progress failed: %w", err)
	}

	return loadState.Progress, nil
}

// GetLoadState 获取加载状态
func (r *VectorRetriever) GetLoadState(ctx context.Context, kbID int64) (entity.LoadState, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	loadStateOpt := milvusclient.NewGetLoadStateOption(collectionName)
	loadState, err := r.client.GetLoadState(ctx, loadStateOpt)
	if err != nil {
		// 返回空的 LoadState 而不是 LoadStateNotExist
		return entity.LoadState{}, fmt.Errorf("get load state failed: %w", err)
	}

	return loadState, nil
}

// ========================================
// Partition 管理
// ========================================

// CreatePartition 创建分区
func (r *VectorRetriever) CreatePartition(ctx context.Context, kbID int64, partitionName string) error {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	createPartOpt := milvusclient.NewCreatePartitionOption(collectionName, partitionName)
	err := r.client.CreatePartition(ctx, createPartOpt)
	if err != nil {
		return fmt.Errorf("create partition failed: %w", err)
	}

	log.Printf("[Milvus] Partition created: %s.%s", collectionName, partitionName)
	return nil
}

// DropPartition 删除分区
func (r *VectorRetriever) DropPartition(ctx context.Context, kbID int64, partitionName string) error {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	dropPartOpt := milvusclient.NewDropPartitionOption(collectionName, partitionName)
	err := r.client.DropPartition(ctx, dropPartOpt)
	if err != nil {
		return fmt.Errorf("drop partition failed: %w", err)
	}

	log.Printf("[Milvus] Partition dropped: %s.%s", collectionName, partitionName)
	return nil
}

// ShowPartitions 显示分区列表
func (r *VectorRetriever) ShowPartitions(ctx context.Context, kbID int64) ([]string, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API - ListPartitions 返回分区名称列表
	listPartOpt := milvusclient.NewListPartitionOption(collectionName)
	partitionNames, err := r.client.ListPartitions(ctx, listPartOpt)
	if err != nil {
		return nil, fmt.Errorf("list partitions failed: %w", err)
	}

	return partitionNames, nil
}

// HasPartition 检查分区是否存在
func (r *VectorRetriever) HasPartition(ctx context.Context, kbID int64, partitionName string) (bool, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API
	hasPartOpt := milvusclient.NewHasPartitionOption(collectionName, partitionName)
	has, err := r.client.HasPartition(ctx, hasPartOpt)
	if err != nil {
		return false, fmt.Errorf("check partition exists failed: %w", err)
	}

	return has, nil
}

// ========================================
// 辅助方法
// ========================================

// getCollectionName 获取集合名称（统一使用 link collection）
func (r *VectorRetriever) getCollectionName(kbID int64) string {
	return "link"
}

// GetStats 获取集合统计信息
func (r *VectorRetriever) GetStats(ctx context.Context, kbID int64) (map[string]interface{}, error) {
	collectionName := r.getCollectionName(kbID)

	// 使用新 SDK Option API 获取 collection 信息
	descOpt := milvusclient.NewDescribeCollectionOption(collectionName)
	coll, err := r.client.DescribeCollection(ctx, descOpt)
	if err != nil {
		return nil, fmt.Errorf("describe collection failed: %w", err)
	}

	stats := map[string]interface{}{
		"name":        coll.Name,
		"field_count": len(coll.Schema.Fields),
	}

	// 获取实体数量
	statsOpt := milvusclient.NewGetCollectionStatsOption(collectionName)
	entities, err := r.client.GetCollectionStats(ctx, statsOpt)
	if err != nil {
		return nil, fmt.Errorf("get collection statistics failed: %w", err)
	}
	stats["statistics"] = entities

	return stats, nil
}

// CompactCollection 压缩集合（Milvus 会自动处理）
func (r *VectorRetriever) CompactCollection(ctx context.Context, kbID int64) error {
	// Milvus 会自动进行数据压缩
	// 这个方法保留为兼容性接口
	log.Printf("[Milvus] Collection auto-compaction enabled: %s", r.getCollectionName(kbID))
	return nil
}

// ========================================
// 工具函数
// ========================================

// MetadataToJSON 将 metadata 转为 JSON 字符串
func MetadataToJSON(metadata map[string]interface{}) string {
	if metadata == nil {
		return "{}"
	}
	data, _ := json.Marshal(metadata)
	return string(data)
}

// JSONToMetadata 将 JSON 字符串转为 metadata
func JSONToMetadata(jsonStr string) map[string]interface{} {
	if jsonStr == "" {
		return nil
	}
	var metadata map[string]interface{}
	_ = json.Unmarshal([]byte(jsonStr), &metadata)
	return metadata
}

// ========================================
// 健康检查
// ========================================

// CheckHealth 检查 Milvus 连接健康状态
func (r *VectorRetriever) CheckHealth(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 使用新 SDK Option API 尝试列出集合
	listOpt := milvusclient.NewListCollectionOption()
	_, err := r.client.ListCollections(checkCtx, listOpt)
	if err != nil {
		return fmt.Errorf("milvus health check failed: %w", err)
	}

	return nil
}

// GetServerVersion 获取 Milvus 服务器版本
func (r *VectorRetriever) GetServerVersion(ctx context.Context) (string, error) {
	versionOpt := milvusclient.NewGetServerVersionOption()
	return r.client.GetServerVersion(ctx, versionOpt)
}

// ========================================
// VectorRepository interface implementation
// Adapter methods to match the interface
// ========================================

// VectorRepositoryAdapter wraps VectorRetriever to implement domain_knowledge.VectorRepository
type VectorRepositoryAdapter struct {
	retriever *VectorRetriever
}

// NewVectorRepositoryAdapter creates a new adapter
func NewVectorRepositoryAdapter(retriever *VectorRetriever) *VectorRepositoryAdapter {
	return &VectorRepositoryAdapter{retriever: retriever}
}

// CreateCollection implements VectorRepository.CreateCollection
func (a *VectorRepositoryAdapter) CreateCollection(ctx context.Context, kbID int64, dimension int, opts *knowledge.CollectionOptions) error {
	// Convert options to CreateKnowledgeBaseOptions
	autoID := false
	enableDynamic := false
	if opts != nil {
		autoID = opts.AutoID
		enableDynamic = opts.EnableDynamic
	}

	createOpts := &CreateKnowledgeBaseOptions{
		Dimension:     dimension,
		IndexType:     IndexTypeAutoIndex,
		MetricType:    entity.L2,
		AutoID:        autoID,
		EnableDynamic: enableDynamic,
	}
	return a.retriever.CreateKnowledgeBase(ctx, kbID, createOpts)
}

// HasCollection implements VectorRepository.HasCollection
func (a *VectorRepositoryAdapter) HasCollection(ctx context.Context, kbID int64) (bool, error) {
	return a.retriever.HasKnowledgeBase(ctx, kbID)
}

// Insert implements VectorRepository.Insert
func (a *VectorRepositoryAdapter) Insert(ctx context.Context, kbID int64, docs []*knowledge.VectorDocument) error {
	// Convert VectorDocument to DocumentData
	dataDocs := make([]*DocumentData, len(docs))
	for i, doc := range docs {
		// Convert sparse vector from knowledge.SparseEmbedding (map) to entity.SparseEmbedding (interface)
		var sparseVec entity.SparseEmbedding
		if len(doc.SparseVector) > 0 {
			positions := make([]uint32, 0, len(doc.SparseVector))
			values := make([]float32, 0, len(doc.SparseVector))
			for k, v := range doc.SparseVector {
				positions = append(positions, k)
				values = append(values, v)
			}
			sparseVec, _ = entity.NewSliceSparseEmbedding(positions, values)
		}

		dataDocs[i] = &DocumentData{
			ID:           doc.ID,
			DenseVector:  doc.DenseVector,
			SparseVector: sparseVec,
			ChunkID:      doc.ChunkID,
			KnowledgeID:  doc.KnowledgeID,
			KnowledgeBaseID: doc.KnowledgeBaseID,
			TenantID:     doc.TenantID,
			ChunkIndex:   doc.ChunkIndex,
			Content:      doc.Content,
			IsEnabled:    doc.IsEnabled,
			StartAt:      doc.StartAt,
			EndAt:        doc.EndAt,
			TokenCount:   doc.TokenCount,
		}
	}
	return a.retriever.InsertData(ctx, kbID, dataDocs)
}

// Search implements VectorRepository.Search
func (a *VectorRepositoryAdapter) Search(ctx context.Context, kbID int64, vector []float32, opts *knowledge.VectorSearchOptions) ([]*knowledge.VectorDocument, error) {
	// Convert options to SearchOptions
	searchOpts := &SearchOptions{
		TopK: 10,
	}
	if opts != nil {
		searchOpts.TopK = opts.TopK
		searchOpts.ScoreThreshold = opts.ScoreThreshold
		searchOpts.Expr = opts.Filter
	}

	results, err := a.retriever.SearchVectors(ctx, kbID, vector, searchOpts)
	if err != nil {
		return nil, err
	}

	// Convert SearchResult to VectorDocument
	docs := make([]*knowledge.VectorDocument, len(results))
	for i, r := range results {
		docs[i] = &knowledge.VectorDocument{
			ID:             r.ID,
			Score:          r.Score,
			ChunkID:        r.ChunkID,
			KnowledgeID:    r.KnowledgeID,
			KnowledgeBaseID: r.KnowledgeBaseID,
			TenantID:       r.TenantID,
			ChunkIndex:     r.ChunkIndex,
			Content:        r.Content,
			IsEnabled:      r.IsEnabled,
			StartAt:        r.StartAt,
			EndAt:          r.EndAt,
			TokenCount:     r.TokenCount,
		}
	}
	return docs, nil
}

// Delete implements VectorRepository.Delete
func (a *VectorRepositoryAdapter) Delete(ctx context.Context, kbID int64, ids []int64) error {
	return a.retriever.DeleteData(ctx, kbID, ids)
}

// DeleteByChunkID implements VectorRepository.DeleteByChunkID
func (a *VectorRepositoryAdapter) DeleteByChunkID(ctx context.Context, kbID int64, chunkID string) error {
	return a.retriever.DeleteByChunkID(ctx, kbID, chunkID)
}

// DeleteByKnowledgeID implements VectorRepository.DeleteByKnowledgeID
func (a *VectorRepositoryAdapter) DeleteByKnowledgeID(ctx context.Context, kbID int64, knowledgeID string) error {
	return a.retriever.DeleteByKnowledgeID(ctx, kbID, knowledgeID)
}

// CreateIndex implements VectorRepository.CreateIndex
func (a *VectorRepositoryAdapter) CreateIndex(ctx context.Context, kbID int64, fieldName string, indexType knowledge.IndexType, metricType knowledge.MetricType, params map[string]string) error {
	// Convert knowledge types to retriever types
	var idxType IndexType
	switch indexType {
	case knowledge.IndexTypeFlat:
		idxType = IndexTypeFlat
	case knowledge.IndexTypeIvfFlat:
		idxType = IndexTypeIvfFlat
	case knowledge.IndexTypeIvfSq8:
		idxType = IndexTypeIvfSq8
	case knowledge.IndexTypeHnsw:
		idxType = IndexTypeHnsw
	case knowledge.IndexTypeAutoIndex:
		idxType = IndexTypeAutoIndex
	default:
		idxType = IndexTypeAutoIndex
	}

	var metric entity.MetricType
	switch metricType {
	case knowledge.MetricTypeL2:
		metric = entity.L2
	case knowledge.MetricTypeIP:
		metric = entity.IP
	case knowledge.MetricTypeCOSINE:
		metric = entity.COSINE
	default:
		metric = entity.L2
	}

	return a.retriever.CreateIndex(ctx, kbID, &IndexOptions{
		IndexType:   idxType,
		MetricType:  metric,
		FieldName:   fieldName,
		IndexParams: params,
	})
}

// LoadCollection implements VectorRepository.LoadCollection
func (a *VectorRepositoryAdapter) LoadCollection(ctx context.Context, kbID int64, async bool) error {
	return a.retriever.LoadKnowledgeBase(ctx, kbID, async)
}

// ReleaseCollection implements VectorRepository.ReleaseCollection
func (a *VectorRepositoryAdapter) ReleaseCollection(ctx context.Context, kbID int64) error {
	return a.retriever.ReleaseKnowledgeBase(ctx, kbID)
}

// GetStats implements VectorRepository.GetStats
func (a *VectorRepositoryAdapter) GetStats(ctx context.Context, kbID int64) (*knowledge.CollectionStats, error) {
	stats, err := a.retriever.GetStats(ctx, kbID)
	if err != nil {
		return nil, err
	}

	// Convert map[string]interface{} to CollectionStats
	var rowCount int64
	var size int64
	fieldCount := 0

	// Extract values from stats map
	if val, ok := stats["field_count"].(int); ok {
		fieldCount = val
	}
	if statistics, ok := stats["statistics"].(map[string]interface{}); ok {
		if rowCountVal, ok := statistics["row_count"]; ok {
			switch v := rowCountVal.(type) {
			case int64:
				rowCount = v
			case int:
				rowCount = int64(v)
			case float64:
				rowCount = int64(v)
			}
		}
	}

	return &knowledge.CollectionStats{
		Name:       fmt.Sprintf("kb_%d", kbID),
		FieldCount: fieldCount,
		RowCount:   rowCount,
		Size:       size,
		Statistics: make(map[string]string),
	}, nil
}

// CheckHealth implements VectorRepository.CheckHealth
func (a *VectorRepositoryAdapter) CheckHealth(ctx context.Context) error {
	return a.retriever.CheckHealth(ctx)
}
