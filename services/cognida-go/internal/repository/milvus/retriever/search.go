// 由 repository.go 拆出——同包、行为等价（M2 god-file 拆分）。
package retriever

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

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

	// 输出字段：不显式指定则 Milvus 只回主键+分数，content/chunk_id 等全为空
	//（此前用 HybridSearch 时漏设 WithOutputFields，正是命中结果 content 恒空的根因）。
	outputFields := []string{"chunk_id", "knowledge_id", "kb_id", "tenant_id", "chunk_index", "content", "is_enabled", "start_at", "end_at", "token_count"}

	// 单路稀疏向量检索：在 sparse 字段上以 entity.Text(query) 触发服务端 BM25。
	// 用 client.Search 而非 HybridSearch——本查询只有一路 ANN，无需结果融合；而
	// HybridSearch 会对唯一子请求强加 RRF 重排，把真实 BM25 相关性分替换成「名次倒数」
	// 1/(60+rank)（表现为不同 query 却得到雷同的 0.0164/0.0161/0.0159），既失真又与
	// query 无关。改用 Search 后 Scores 即服务端原始 BM25 分，可用于阈值与排序。
	searchOpt := milvusclient.NewSearchOption(collectionName, topK, []entity.Vector{entity.Text(query)})
	searchOpt.WithANNSField("sparse")
	if expr != "" {
		searchOpt.WithFilter(expr)
	}
	searchOpt.WithOutputFields(outputFields...)
	searchOpt.WithAnnParam(index.NewCustomAnnParam())

	resultSets, err := r.client.Search(ctx, searchOpt)
	if err != nil {
		// BM25 失败通常意味着 Milvus 版本不支持、BM25 Function/索引未在服务端配置。
		// 如实返回错误而非静默空结果：调用方（BM25Retrieve）在 err != nil 时会回退到
		// 数据库关键词检索，同时错误得以被日志/监控观测，避免"BM25 恒空却伪装成正常"的坏账。
		log.Printf("[Milvus] BM25 full-text search failed (possibly not supported or index not created): %v", err)
		return nil, fmt.Errorf("BM25 full-text search failed: %w", err)
	}

	// 解析结果 - 新 SDK 返回的是 []ResultSet
	results := make([]*SearchResult, 0)
	for _, resultSet := range resultSets {
		for i := 0; i < resultSet.ResultCount; i++ {
			result := &SearchResult{
				Score: resultSet.Scores[i], // 服务端原始 BM25 分数
			}

			// 提取字段值 - 新 SDK 使用 GetColumn() 方法
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
		clause := ""
		// 根据字段类型构建表达式
		switch k {
		case "tenant_id":
			// 数值字段：必须为合法整数，否则跳过该条件，杜绝表达式注入
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				log.Printf("[Milvus] 跳过非法 tenant_id 过滤值: %q", v)
				continue
			}
			clause = fmt.Sprintf("tenant_id == %d", id)
		case "is_enabled":
			if v == "true" {
				clause = "is_enabled == true"
			} else {
				clause = "is_enabled == false"
			}
		case "kb_id", "knowledge_id", "chunk_id":
			clause = fmt.Sprintf("%s == '%s'", k, escapeMilvusStringLiteral(v))
		default:
			// 默认字符串相等，值需转义防止越过引号边界
			clause = fmt.Sprintf("%s == '%s'", k, escapeMilvusStringLiteral(v))
		}

		if clause == "" {
			continue
		}
		if expr != "" {
			expr += " && "
		}
		expr += clause
	}

	return expr
}

// escapeMilvusStringLiteral 转义单引号字符串字面量中的特殊字符，
// 防止过滤值携带引号/反斜杠越出字面量边界导致表达式注入。
func escapeMilvusStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// ========================================
// 向量查询
// ========================================

// SearchResult 搜索结果（适配新 schema）
type SearchResult struct {
	ID              int64
	Score           float32
	ChunkID         string // 分块 ID (MySQL chunks.id)
	KnowledgeID     string // 知识条目 ID
	KnowledgeBaseID string // 知识库 ID
	TenantID        int64  // 租户 ID
	ChunkIndex      int    // 分块索引
	Content         string // 分块内容
	IsEnabled       bool   // 是否启用
	StartAt         int64  // 起始位置
	EndAt           int64  // 结束位置
	TokenCount      int64  // Token 数量
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
