// 由 repository.go 拆出——同包、行为等价（M2 god-file 拆分）。
package retriever

import (
	"context"
	"fmt"
	"log"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

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
