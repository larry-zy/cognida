// 由 repository.go 拆出——同包、行为等价（M2 god-file 拆分）。
package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

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
