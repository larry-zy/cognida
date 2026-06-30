// Package memory provides Milvus-based ReflectionMemory implementation.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"link/internal/model/agent/reflection"
	"link/internal/repository/milvus"
)

// ========================================
// MilvusReflectionMemory Milvus 反思记忆实现
// ========================================

// MilvusReflectionMemory 使用 Milvus 存储和检索反思经验
type MilvusReflectionMemory struct {
	client   *milvusclient.Client
	embedder embedding.Embedder
	config   *reflection.MemoryConfig
}

// NewMilvusReflectionMemory 创建 Milvus 反思记忆
func NewMilvusReflectionMemory(embedder embedding.Embedder, config *reflection.MemoryConfig) (*MilvusReflectionMemory, error) {
	client := milvus.GetClient()
	if client == nil {
		return nil, fmt.Errorf("milvus client not initialized")
	}

	mem := &MilvusReflectionMemory{
		client:   client,
		embedder: embedder,
		config:   config,
	}

	// 初始化 collection
	if err := mem.initCollection(context.Background()); err != nil {
		return nil, fmt.Errorf("init collection failed: %w", err)
	}

	return mem, nil
}

// ========================================
// Collection 管理
// ========================================

const (
	collectionName = "reflection_memory"
	vectorFieldName = "vector"
)

// initCollection 初始化 collection
func (m *MilvusReflectionMemory) initCollection(ctx context.Context) error {
	// 检查是否存在
	has, err := m.hasCollection(ctx)
	if err != nil {
		return err
	}

	if has {
		// 加载到内存
		return m.loadCollection(ctx)
	}

	// 创建 collection
	schema := m.buildSchema()
	createOpt := milvusclient.NewCreateCollectionOption(collectionName, schema)
	if err := m.client.CreateCollection(ctx, createOpt); err != nil {
		return fmt.Errorf("create collection failed: %w", err)
	}

	log.Printf("[ReflectionMemory] Collection created: %s", collectionName)

	// 创建索引
	if err := m.createIndex(ctx); err != nil {
		return fmt.Errorf("create index failed: %w", err)
	}

	// 加载到内存
	return m.loadCollection(ctx)
}

// hasCollection 检查 collection 是否存在
func (m *MilvusReflectionMemory) hasCollection(ctx context.Context) (bool, error) {
	hasOpt := milvusclient.NewHasCollectionOption(collectionName)
	has, err := m.client.HasCollection(ctx, hasOpt)
	if err != nil {
		return false, fmt.Errorf("check collection exists failed: %w", err)
	}
	return has, nil
}

// loadCollection 加载 collection 到内存
func (m *MilvusReflectionMemory) loadCollection(ctx context.Context) error {
	loadOpt := milvusclient.NewLoadCollectionOption(collectionName)
	task, err := m.client.LoadCollection(ctx, loadOpt)
	if err != nil {
		return fmt.Errorf("load collection failed: %w", err)
	}
	// 等待加载完成
	_ = task.Await(ctx)
	return nil
}

// buildSchema 构建 collection schema
func (m *MilvusReflectionMemory) buildSchema() *entity.Schema {
	schema := entity.NewSchema().
		WithName(collectionName).
		WithDescription("Agent reflection memory for storing lessons learned").
		WithDynamicFieldEnabled(true)

	// 主键字段
	schema = schema.WithField(
		entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(100).WithIsPrimaryKey(true).WithIsAutoID(false),
	)

	// Agent ID 字段
	schema = schema.WithField(
		entity.NewField().WithName("agent_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(100),
	)

	// 任务字段
	schema = schema.WithField(
		entity.NewField().WithName("task").WithDataType(entity.FieldTypeVarChar).WithMaxLength(4096),
	)

	// 教训字段
	schema = schema.WithField(
		entity.NewField().WithName("lesson").WithDataType(entity.FieldTypeVarChar).WithMaxLength(4096),
	)

	// 向量字段
	schema = schema.WithField(
		entity.NewField().WithName(vectorFieldName).WithDataType(entity.FieldTypeFloatVector).
			WithDim(int64(m.config.VectorDim)),
	)

	// 尝试的输出 (JSON)
	schema = schema.WithField(
		entity.NewField().WithName("attempt").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535),
	)

	// 批评结果 (JSON)
	schema = schema.WithField(
		entity.NewField().WithName("critique").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535),
	)

	// 成功标志
	schema = schema.WithField(
		entity.NewField().WithName("success").WithDataType(entity.FieldTypeBool),
	)

	// 迭代次数
	schema = schema.WithField(
		entity.NewField().WithName("iterations").WithDataType(entity.FieldTypeInt64),
	)

	// 使用次数
	schema = schema.WithField(
		entity.NewField().WithName("used_count").WithDataType(entity.FieldTypeInt64),
	)

	// 创建时间
	schema = schema.WithField(
		entity.NewField().WithName("created_at").WithDataType(entity.FieldTypeInt64),
	)

	// 更新时间
	schema = schema.WithField(
		entity.NewField().WithName("updated_at").WithDataType(entity.FieldTypeInt64),
	)

	// TTL 过期时间
	schema = schema.WithField(
		entity.NewField().WithName("ttl").WithDataType(entity.FieldTypeInt64),
	)

	return schema
}

// createIndex 创建向量索引
func (m *MilvusReflectionMemory) createIndex(ctx context.Context) error {
	// 创建 HNSW 索引
	idx := index.NewHNSWIndex(entity.COSINE, 16, 100)
	createIdxOpt := milvusclient.NewCreateIndexOption(collectionName, vectorFieldName, idx)
	_, err := m.client.CreateIndex(ctx, createIdxOpt)
	if err != nil {
		return fmt.Errorf("create index failed: %w", err)
	}

	log.Printf("[ReflectionMemory] Index created on %s.%s", collectionName, vectorFieldName)
	return nil
}

// ========================================
// ReflectionMemory 接口实现
// ========================================

// Store 存储反思记录
func (m *MilvusReflectionMemory) Store(ctx context.Context, record *reflection.ReflectionRecord) error {
	if record.ID == "" {
		record.ID = generateID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	record.UpdatedAt = time.Now()

	// 计算 TTL
	ttl := record.CreatedAt.Add(m.config.TTL).Unix()

	// 向量化任务和教训
	text := fmt.Sprintf("任务：%s\n教训：%s", record.Task, record.Lesson)
	vector, err := m.embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embed failed: %w", err)
	}

	// 序列化 JSON
	attemptJSON, _ := json.Marshal(record.Attempt)
	critiqueJSON, _ := json.Marshal(record.Critique)

	// 构建插入列
	columns := []column.Column{
		column.NewColumnVarChar("id", []string{record.ID}),
		column.NewColumnVarChar("agent_id", []string{record.AgentID}),
		column.NewColumnVarChar("task", []string{record.Task}),
		column.NewColumnVarChar("lesson", []string{record.Lesson}),
		column.NewColumnFloatVector(vectorFieldName, m.config.VectorDim, [][]float32{vector}),
		column.NewColumnVarChar("attempt", []string{string(attemptJSON)}),
		column.NewColumnVarChar("critique", []string{string(critiqueJSON)}),
		column.NewColumnBool("success", []bool{record.Success}),
		column.NewColumnInt64("iterations", []int64{int64(record.Iterations)}),
		column.NewColumnInt64("used_count", []int64{int64(record.UsedCount)}),
		column.NewColumnInt64("created_at", []int64{record.CreatedAt.Unix()}),
		column.NewColumnInt64("updated_at", []int64{record.UpdatedAt.Unix()}),
		column.NewColumnInt64("ttl", []int64{ttl}),
	}

	// 插入数据
	insertOpt := milvusclient.NewColumnBasedInsertOption(collectionName, columns...)
	_, err = m.client.Insert(ctx, insertOpt)
	if err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}

	// 刷新数据
	flushOpt := milvusclient.NewFlushOption(collectionName)
	_, err = m.client.Flush(ctx, flushOpt)
	if err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	log.Printf("[ReflectionMemory] Stored record: %s (success=%v)", record.ID, record.Success)
	return nil
}

// Retrieve 检索相关的反思记录
func (m *MilvusReflectionMemory) Retrieve(ctx context.Context, agentID, task string, limit int) ([]*reflection.ReflectionRecord, error) {
	if limit <= 0 {
		limit = 3
	}

	// 向量化查询
	vector, err := m.embed(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("embed failed: %w", err)
	}

	// 构建搜索向量
	vectors := []entity.Vector{entity.FloatVector(vector)}

	// 构建过滤表达式：只返回失败经验和当前 agent 的记录
	now := time.Now().Unix()
	expr := fmt.Sprintf("agent_id == '%s' && success == false && ttl > %d", agentID, now)

	// 执行搜索
	searchOpt := milvusclient.NewSearchOption(collectionName, limit, vectors)
	searchOpt.WithANNSField(vectorFieldName)
	searchOpt.WithFilter(expr)
	searchOpt.WithOutputFields("id", "agent_id", "task", "lesson", "attempt", "critique", "success", "iterations", "used_count", "created_at", "updated_at")

	searchResult, err := m.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// 解析结果
	records := make([]*reflection.ReflectionRecord, 0)
	for _, resultSet := range searchResult {
		for i := 0; i < resultSet.ResultCount; i++ {
			record := m.parseRecord(&resultSet, i)
			if record != nil {
				// 更新使用次数
				record.UsedCount++
				records = append(records, record)
			}
		}
	}

	return records, nil
}

// UpdateSuccess 更新记录为成功状态
func (m *MilvusReflectionMemory) UpdateSuccess(ctx context.Context, id string) error {
	// 由于 Milvus 不支持 UPDATE，这里记录日志
	// 实际应用中可以考虑使用其他方式（如 Redis）存储热数据
	log.Printf("[ReflectionMemory] UpdateSuccess called for %s (Milvus doesn't support direct update)", id)
	return nil
}

// Cleanup 清理过期记录
func (m *MilvusReflectionMemory) Cleanup(ctx context.Context) error {
	now := time.Now().Unix()
	expr := fmt.Sprintf("ttl < %d", now)

	deleteOpt := milvusclient.NewDeleteOption(collectionName)
	deleteOpt.WithExpr(expr)
	_, err := m.client.Delete(ctx, deleteOpt)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	log.Printf("[ReflectionMemory] Cleaned up expired records")
	return nil
}

// ========================================
// 辅助方法
// ========================================

// embed 向量化文本
func (m *MilvusReflectionMemory) embed(ctx context.Context, text string) ([]float32, error) {
	result, err := m.embedder.EmbedStrings(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || len(result[0]) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}

	// 转换为 float32
	vector := make([]float32, len(result[0]))
	for i, val := range result[0] {
		vector[i] = float32(val)
	}
	return vector, nil
}

// parseRecord 解析搜索结果为记录
func (m *MilvusReflectionMemory) parseRecord(resultSet *milvusclient.ResultSet, idx int) *reflection.ReflectionRecord {
	record := &reflection.ReflectionRecord{}

	for _, fieldName := range []string{"id", "agent_id", "task", "lesson", "attempt", "critique", "created_at", "updated_at"} {
		col := resultSet.GetColumn(fieldName)
		if col == nil {
			continue
		}

		switch fieldName {
		case "id":
			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				val, _ := varcharCol.Value(idx)
				record.ID = val
			}
		case "agent_id":
			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				val, _ := varcharCol.Value(idx)
				record.AgentID = val
			}
		case "task":
			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				val, _ := varcharCol.Value(idx)
				record.Task = val
			}
		case "lesson":
			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				val, _ := varcharCol.Value(idx)
				record.Lesson = val
			}
		case "attempt":
			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				val, _ := varcharCol.Value(idx)
				record.Attempt = val
			}
		case "critique":
			if varcharCol, ok := col.(*column.ColumnVarChar); ok {
				val, _ := varcharCol.Value(idx)
				var critique reflection.CritiqueResult
				if err := json.Unmarshal([]byte(val), &critique); err == nil {
					record.Critique = &critique
				}
			}
		case "created_at":
			if intCol, ok := col.(*column.ColumnInt64); ok {
				val, _ := intCol.Value(idx)
				record.CreatedAt = time.Unix(val, 0)
			}
		case "updated_at":
			if intCol, ok := col.(*column.ColumnInt64); ok {
				val, _ := intCol.Value(idx)
				record.UpdatedAt = time.Unix(val, 0)
			}
		}
	}

	// 解析 bool 和 int64 字段
	if col := resultSet.GetColumn("success"); col != nil {
		if boolCol, ok := col.(*column.ColumnBool); ok {
			val, _ := boolCol.Value(idx)
			record.Success = val
		}
	}
	if col := resultSet.GetColumn("iterations"); col != nil {
		if intCol, ok := col.(*column.ColumnInt64); ok {
			val, _ := intCol.Value(idx)
			record.Iterations = int(val)
		}
	}
	if col := resultSet.GetColumn("used_count"); col != nil {
		if intCol, ok := col.(*column.ColumnInt64); ok {
			val, _ := intCol.Value(idx)
			record.UsedCount = int(val)
		}
	}

	return record
}

// generateID 生成唯一 ID
func generateID() string {
	return fmt.Sprintf("ref_%d", time.Now().UnixNano())
}
