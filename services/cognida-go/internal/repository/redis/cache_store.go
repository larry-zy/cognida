// Package redis provides Redis storage implementation for semantic cache
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

const (
	// CacheKeyPrefix 缓存键前缀
	CacheKeyPrefix = "semantic_cache:"

	// CacheStatsKey 统计键前缀
	CacheStatsKey = "semantic_cache:stats"

	// CacheSimilarityHistogram 相似度分布直方图键
	CacheSimilarityHistogram = "semantic_cache:similarity_histogram"
)

// ========================================
// Cache Content Repository
// ========================================

// CacheContentRepository 缓存内容仓储实现
type CacheContentRepository struct {
	client *redis.Client
}

// NewCacheContentRepository 创建缓存内容仓储
func NewCacheContentRepository(client *redis.Client) *CacheContentRepository {
	return &CacheContentRepository{
		client: client,
	}
}

// ========================================
// Content Operations
// ========================================

// GetContent 获取缓存内容
func (r *CacheContentRepository) GetContent(ctx context.Context, cacheID string) (*cache.CacheEntry, error) {
	key := r.buildCacheKey(cacheID)

	data, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("cache entry not found")
		}
		return nil, fmt.Errorf("failed to get cache content: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("cache entry not found")
	}

	return r.unmarshalEntry(data)
}

// SetContent 设置缓存内容
func (r *CacheContentRepository) SetContent(ctx context.Context, entry *cache.CacheEntry, ttl time.Duration) error {
	key := r.buildCacheKey(entry.CacheID)

	// 序列化数据
	data, err := r.marshalEntry(entry)
	if err != nil {
		return err
	}

	// 设置 Hash 字段 - 使用 HMSet 兼容 Redis 3.2+
	pipe := r.client.Pipeline()
	pipe.HMSet(ctx, key, data)
	pipe.Expire(ctx, key, ttl)

	// 添加到租户索引
	tenantKey := r.buildTenantKey(entry.TenantID)
	pipe.SAdd(ctx, tenantKey, entry.CacheID)
	pipe.Expire(ctx, tenantKey, ttl)

	// 添加到 Agent 类型索引
	agentKey := r.buildAgentKey(entry.TenantID, entry.AgentType)
	pipe.SAdd(ctx, agentKey, entry.CacheID)
	pipe.Expire(ctx, agentKey, ttl)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set cache content: %w", err)
	}

	return nil
}

// MGetContents 批量获取缓存内容
func (r *CacheContentRepository) MGetContents(ctx context.Context, cacheIDs []string) (map[string]*cache.CacheEntry, error) {
	if len(cacheIDs) == 0 {
		return make(map[string]*cache.CacheEntry), nil
	}

	keys := make([]string, len(cacheIDs))
	for i, id := range cacheIDs {
		keys[i] = r.buildCacheKey(id)
	}

	// 使用 Pipeline 批量获取
	pipe := r.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.HGetAll(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to mget cache contents: %w", err)
	}

	// 解析结果
	results := make(map[string]*cache.CacheEntry)
	for i, cmd := range cmds {
		data, _ := cmd.Result()
		if len(data) == 0 {
			continue
		}

		entry, err := r.unmarshalEntry(data)
		if err != nil {
			continue
		}
		results[cacheIDs[i]] = entry
	}

	return results, nil
}

// Delete 删除缓存内容
func (r *CacheContentRepository) Delete(ctx context.Context, cacheID string) error {
	key := r.buildCacheKey(cacheID)

	// 先获取条目信息以清理索引
	entry, err := r.GetContent(ctx, cacheID)
	if err == nil && entry != nil {
		pipe := r.client.Pipeline()

		// 从租户索引移除
		tenantKey := r.buildTenantKey(entry.TenantID)
		pipe.SRem(ctx, tenantKey, cacheID)

		// 从 Agent 索引移除
		agentKey := r.buildAgentKey(entry.TenantID, entry.AgentType)
		pipe.SRem(ctx, agentKey, cacheID)

		_, _ = pipe.Exec(ctx)
	}

	// 删除主数据
	return r.client.Del(ctx, key).Err()
}

// UpdateHitCount 更新命中计数
func (r *CacheContentRepository) UpdateHitCount(ctx context.Context, cacheID string) error {
	key := r.buildCacheKey(cacheID)
	return r.client.HIncrBy(ctx, key, "hit_count", 1).Err()
}

// ========================================
// Statistics Operations
// ========================================

// RecordHit 记录命中
func (r *CacheContentRepository) RecordHit(ctx context.Context) error {
	return r.client.Incr(ctx, CacheStatsKey+":hits").Err()
}

// RecordMiss 记录未命中
func (r *CacheContentRepository) RecordMiss(ctx context.Context) error {
	return r.client.Incr(ctx, CacheStatsKey+":misses").Err()
}

// GetStats 获取统计信息
func (r *CacheContentRepository) GetStats(ctx context.Context) (*cache.CacheStats, error) {
	pipe := r.client.Pipeline()

	hitsCmd := pipe.Get(ctx, CacheStatsKey+":hits")
	missesCmd := pipe.Get(ctx, CacheStatsKey+":misses")
	histoCmd := pipe.HGetAll(ctx, CacheSimilarityHistogram)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	// 解析命中/未命中计数
	hits, _ := strconv.ParseInt(hitsCmd.Val(), 10, 64)
	misses, _ := strconv.ParseInt(missesCmd.Val(), 10, 64)

	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	// 解析相似度分布
	similarityDist := make(map[string]int64)
	histoData := histoCmd.Val()
	if len(histoData) > 0 {
		for bucket, countStr := range histoData {
			if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
				similarityDist[bucket] = count
			}
		}
	}

	return &cache.CacheStats{
		Hits:          hits,
		Misses:        misses,
		HitRate:       hitRate,
		TotalSaved:    0, // 需要另外计算
		SimilarityDist: similarityDist,
	}, nil
}

// IncrementSimilarityBucket 增加相似度区间计数
func (r *CacheContentRepository) IncrementSimilarityBucket(ctx context.Context, bucket string) error {
	return r.client.HIncrBy(ctx, CacheSimilarityHistogram, bucket, 1).Err()
}

// ResetStats 重置统计信息
func (r *CacheContentRepository) ResetStats(ctx context.Context) error {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, CacheStatsKey+":hits")
	pipe.Del(ctx, CacheStatsKey+":misses")
	pipe.Del(ctx, CacheSimilarityHistogram)
	_, err := pipe.Exec(ctx)
	return err
}

// ========================================
// Batch Operations
// ========================================

// ClearByTenant 清除租户所有缓存
func (r *CacheContentRepository) ClearByTenant(ctx context.Context, tenantID int64) (int64, error) {
	tenantKey := r.buildTenantKey(tenantID)

	// 获取所有 cache_id
	cacheIDs, err := r.client.SMembers(ctx, tenantKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant cache IDs: %w", err)
	}

	if len(cacheIDs) == 0 {
		return 0, nil
	}

	// 批量删除
	count, err := r.deleteBatch(ctx, cacheIDs)
	if err != nil {
		return count, err
	}

	// 删除租户索引
	_ = r.client.Del(ctx, tenantKey).Err()

	return count, nil
}

// ClearByAgent 清除指定 Agent 类型的所有缓存
func (r *CacheContentRepository) ClearByAgent(ctx context.Context, tenantID int64, agentType string) (int64, error) {
	agentKey := r.buildAgentKey(tenantID, agentType)

	// 获取所有 cache_id
	cacheIDs, err := r.client.SMembers(ctx, agentKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get agent cache IDs: %w", err)
	}

	if len(cacheIDs) == 0 {
		return 0, nil
	}

	// 批量删除
	count, err := r.deleteBatch(ctx, cacheIDs)
	if err != nil {
		return count, err
	}

	// 删除 Agent 索引
	_ = r.client.Del(ctx, agentKey).Err()

	return count, nil
}

// ClearByModel 清除指定模型的所有缓存
func (r *CacheContentRepository) ClearByModel(ctx context.Context, tenantID int64, model string) (int64, error) {
	// 遍历租户所有缓存，按模型过滤
	tenantKey := r.buildTenantKey(tenantID)
	cacheIDs, err := r.client.SMembers(ctx, tenantKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get tenant cache IDs: %w", err)
	}

	if len(cacheIDs) == 0 {
		return 0, nil
	}

	// 筛选匹配模型的缓存 ID
	targetIDs := make([]string, 0)
	for _, cacheID := range cacheIDs {
		entry, err := r.GetContent(ctx, cacheID)
		if err != nil {
			continue
		}
		if entry.Model == model {
			targetIDs = append(targetIDs, cacheID)
		}
	}

	if len(targetIDs) == 0 {
		return 0, nil
	}

	// 批量删除
	return r.deleteBatch(ctx, targetIDs)
}

// GetAgentCacheIDs 获取指定 Agent 类型的所有缓存 ID
func (r *CacheContentRepository) GetAgentCacheIDs(ctx context.Context, tenantID int64, agentType string) ([]string, error) {
	agentKey := r.buildAgentKey(tenantID, agentType)
	return r.client.SMembers(ctx, agentKey).Result()
}

// ========================================
// Helper Methods
// ========================================

// buildCacheKey 构建缓存键
func (r *CacheContentRepository) buildCacheKey(cacheID string) string {
	return CacheKeyPrefix + cacheID
}

// buildTenantKey 构建租户索引键
func (r *CacheContentRepository) buildTenantKey(tenantID int64) string {
	return fmt.Sprintf("tenant:%d:caches", tenantID)
}

// buildAgentKey 构建 Agent 索引键
func (r *CacheContentRepository) buildAgentKey(tenantID int64, agentType string) string {
	return fmt.Sprintf("tenant:%d:agent:%s:caches", tenantID, agentType)
}

// marshalEntry 序列化缓存条目
func (r *CacheContentRepository) marshalEntry(entry *cache.CacheEntry) (map[string]interface{}, error) {
	// 向量转为 JSON 字符串
	vectorJSON, err := json.Marshal(entry.Vector)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vector: %w", err)
	}

	// 元数据转为 JSON 字符串
	metadataJSON := "{}"
	if entry.Metadata != nil {
		metadataJSONBytes, err := json.Marshal(entry.Metadata)
		if err == nil {
			metadataJSON = string(metadataJSONBytes)
		}
	}

	return map[string]interface{}{
		"cache_id":     entry.CacheID,
		"query":        entry.Query,
		"response":     entry.Response,
		"model":        entry.Model,
		"agent_type":   entry.AgentType,
		"tenant_id":    entry.TenantID,
		"prompt_hash":  entry.PromptHash,
		"vector":       vectorJSON,
		"created_at":   entry.CreatedAt,
		"updated_at":   entry.UpdatedAt,
		"hit_count":    entry.HitCount,
		"metadata":     metadataJSON,
	}, nil
}

// unmarshalEntry 反序列化缓存条目
func (r *CacheContentRepository) unmarshalEntry(data map[string]string) (*cache.CacheEntry, error) {
	entry := &cache.CacheEntry{
		CacheID:    data["cache_id"],
		Query:      data["query"],
		Response:   data["response"],
		Model:      data["model"],
		AgentType:  data["agent_type"],
		PromptHash: data["prompt_hash"],
		Metadata:   make(map[string]string),
	}

	// 解析 tenant_id
	if tenantIDStr := data["tenant_id"]; tenantIDStr != "" {
		_, _ = fmt.Sscanf(tenantIDStr, "%d", &entry.TenantID) // 解析失败保持零值
	}

	// 解析向量
	if vectorJSON := data["vector"]; vectorJSON != "" {
		if err := json.Unmarshal([]byte(vectorJSON), &entry.Vector); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vector: %w", err)
		}
	}

	// 解析时间戳
	if createdAtStr := data["created_at"]; createdAtStr != "" {
		_, _ = fmt.Sscanf(createdAtStr, "%d", &entry.CreatedAt) // 解析失败保持零值
	}
	if updatedAtStr := data["updated_at"]; updatedAtStr != "" {
		_, _ = fmt.Sscanf(updatedAtStr, "%d", &entry.UpdatedAt) // 解析失败保持零值
	}

	// 解析命中计数
	if hitCountStr := data["hit_count"]; hitCountStr != "" {
		_, _ = fmt.Sscanf(hitCountStr, "%d", &entry.HitCount) // 解析失败保持零值
	}

	// 解析元数据
	if metadataJSON := data["metadata"]; metadataJSON != "" && metadataJSON != "{}" {
		_ = json.Unmarshal([]byte(metadataJSON), &entry.Metadata) // 解析失败保持零值
	}

	return entry, nil
}

// deleteBatch 批量删除缓存
func (r *CacheContentRepository) deleteBatch(ctx context.Context, cacheIDs []string) (int64, error) {
	if len(cacheIDs) == 0 {
		return 0, nil
	}

	keys := make([]string, len(cacheIDs))
	for i, id := range cacheIDs {
		keys[i] = r.buildCacheKey(id)
	}

	// 批量删除主数据
	count, err := r.client.Del(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to delete batch: %w", err)
	}

	return count, nil
}

// ========================================
// Health Check
// ========================================

// Ping 检查 Redis 连接
func (r *CacheContentRepository) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
