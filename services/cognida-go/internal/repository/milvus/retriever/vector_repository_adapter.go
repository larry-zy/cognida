// 由 repository.go 拆出——同包、行为等价（M2 god-file 拆分）。
package retriever

import (
	"cognida/internal/model/knowledge"
	"context"
	"fmt"

	"github.com/milvus-io/milvus/client/v2/entity"
)

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
	enableBM25 := false
	bm25K1, bm25B := 1.2, 0.75
	if opts != nil {
		autoID = opts.AutoID
		enableDynamic = opts.EnableDynamic
		enableBM25 = opts.EnableBM25
		if opts.BM25K1 > 0 {
			bm25K1 = opts.BM25K1
		}
		if opts.BM25B > 0 {
			bm25B = opts.BM25B
		}
	}

	createOpts := &CreateKnowledgeBaseOptions{
		Dimension:     dimension,
		IndexType:     IndexTypeAutoIndex,
		MetricType:    entity.L2,
		AutoID:        autoID,
		EnableDynamic: enableDynamic,
		EnableBM25:    enableBM25,
		BM25K1:        bm25K1,
		BM25B:         bm25B,
	}
	if err := a.retriever.CreateKnowledgeBase(ctx, kbID, createOpts); err != nil {
		return err
	}

	// 启用 BM25 时，建表后立即在 sparse 字段上创建 BM25 稀疏倒排索引，
	// 使服务端 Function 生成的 sparse 向量可被 FullTextSearch 检索（否则查询报索引缺失）。
	if enableBM25 {
		if err := a.retriever.CreateBM25Index(ctx, kbID, &BM25IndexOptions{K1: bm25K1, B: bm25B}); err != nil {
			return err
		}
	}
	return nil
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
			ID:              doc.ID,
			DenseVector:     doc.DenseVector,
			SparseVector:    sparseVec,
			Text:            doc.Text, // BM25 全文检索分词源
			ChunkID:         doc.ChunkID,
			KnowledgeID:     doc.KnowledgeID,
			KnowledgeBaseID: doc.KnowledgeBaseID,
			TenantID:        doc.TenantID,
			ChunkIndex:      doc.ChunkIndex,
			Content:         doc.Content,
			IsEnabled:       doc.IsEnabled,
			StartAt:         doc.StartAt,
			EndAt:           doc.EndAt,
			TokenCount:      doc.TokenCount,
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
			ID:              r.ID,
			Score:           r.Score,
			ChunkID:         r.ChunkID,
			KnowledgeID:     r.KnowledgeID,
			KnowledgeBaseID: r.KnowledgeBaseID,
			TenantID:        r.TenantID,
			ChunkIndex:      r.ChunkIndex,
			Content:         r.Content,
			IsEnabled:       r.IsEnabled,
			StartAt:         r.StartAt,
			EndAt:           r.EndAt,
			TokenCount:      r.TokenCount,
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
