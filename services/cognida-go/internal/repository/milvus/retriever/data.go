// 由 repository.go 拆出——同包、行为等价（M2 god-file 拆分）。
package retriever

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

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
	ID              int64                  // 主键 ID（可选，AutoID 时可省略）
	DenseVector     []float32              // 稠密向量（语义检索）
	SparseVector    entity.SparseEmbedding // 稀疏向量（BM25 关键词匹配）
	Text            string                 // 原始文本（用于 BM25 全文搜索）
	ChunkID         string                 // 对应 MySQL chunks.id
	KnowledgeID     string                 // 对应 MySQL chunks.knowledge_id
	KnowledgeBaseID string                 // 对应 MySQL chunks.kb_id
	TenantID        int64                  // 对应 MySQL chunks.tenant_id
	ChunkIndex      int                    // 分块索引
	Content         string                 // 分块内容
	IsEnabled       bool                   // 是否启用
	StartAt         int64                  // 起始位置
	EndAt           int64                  // 结束位置
	TokenCount      int64                  // Token 数量
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
		column.NewColumnVarChar("text", texts),                        // BM25 文本字段
		column.NewColumnSparseVectors("sparse", sparseVectors),        // BM25 稀疏向量字段（由 Function 自动生成）
		column.NewColumnSparseVectors("sparse_vector", sparseVectors), // 保留兼容性
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
