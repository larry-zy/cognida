// 由 repository.go 拆出——同包、行为等价（M2 god-file 拆分）。
package retriever

import (
	"context"
	"fmt"
	"log"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

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
			_, _ = fmt.Sscanf(val, "%d", &nlist) // 解析失败保持零值
		}
		idx = index.NewIvfFlatIndex(opts.MetricType, nlist)
	case IndexTypeIvfSq8:
		nlist := 128
		if val, ok := opts.IndexParams["nlist"]; ok {
			_, _ = fmt.Sscanf(val, "%d", &nlist) // 解析失败保持零值
		}
		idx = index.NewIvfSQ8Index(opts.MetricType, nlist)
	case IndexTypeHnsw:
		M := 16
		efConstruction := 256
		if val, ok := opts.IndexParams["M"]; ok {
			_, _ = fmt.Sscanf(val, "%d", &M) // 解析失败保持零值
		}
		if val, ok := opts.IndexParams["efConstruction"]; ok {
			_, _ = fmt.Sscanf(val, "%d", &efConstruction) // 解析失败保持零值
		}
		idx = index.NewHNSWIndex(opts.MetricType, M, efConstruction)
	case IndexTypeSparseInverted:
		dropRatio := 0.2
		if val, ok := opts.IndexParams["drop_ratio"]; ok {
			_, _ = fmt.Sscanf(val, "%f", &dropRatio) // 解析失败保持零值
		}
		idx = index.NewSparseInvertedIndex(opts.MetricType, dropRatio)
	default:
		// 默认使用 IVF_FLAT
		nlist := 128
		if val, ok := opts.IndexParams["nlist"]; ok {
			_, _ = fmt.Sscanf(val, "%d", &nlist) // 解析失败保持零值
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
