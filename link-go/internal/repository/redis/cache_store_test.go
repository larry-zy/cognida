// Package redis provides unit tests for cache store
package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link/internal/model/cache"
)

// ========================================
// Test Helper
// ========================================

// setupTestRedis 设置测试用 Redis
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

// ========================================
// Unit Tests
// ========================================

// TestNewCacheContentRepository 测试仓储创建
func TestNewCacheContentRepository(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	assert.NotNil(t, repo)
}

// TestSetContentAndGetContent 测试内容存取
func TestSetContentAndGetContent(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	entry := &cache.CacheEntry{
		CacheID:    "test_001",
		Query:      "测试查询",
		Response:   "测试响应",
		Model:      "gpt-4o",
		AgentType:  "rag_agent",
		TenantID:   123,
		PromptHash: "abc123",
		Vector:     []float32{0.1, 0.2, 0.3},
		CreatedAt:  time.Now().Unix(),
		HitCount:   0,
	}

	ttl := 10 * time.Second

	// 设置内容
	err := repo.SetContent(ctx, entry, ttl)
	require.NoError(t, err)

	// 获取内容
	retrieved, err := repo.GetContent(ctx, "test_001")
	require.NoError(t, err)

	assert.Equal(t, entry.CacheID, retrieved.CacheID)
	assert.Equal(t, entry.Query, retrieved.Query)
	assert.Equal(t, entry.Response, retrieved.Response)
	assert.Equal(t, entry.Model, retrieved.Model)
	assert.Equal(t, entry.AgentType, retrieved.AgentType)
	assert.Equal(t, entry.TenantID, retrieved.TenantID)
	assert.Equal(t, entry.PromptHash, retrieved.PromptHash)

	// 检查 TTL
	key := CacheKeyPrefix + "test_001"
	ttlValue := mr.TTL(key)
	assert.Greater(t, ttlValue, time.Second*9)
	assert.LessOrEqual(t, ttlValue, time.Second*10)
}

// TestMGetContents 测试批量获取
func TestMGetContents(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	// 设置多个条目
	entries := []*cache.CacheEntry{
		{
			CacheID:   "test_001",
			Query:     "查询1",
			Response:  "响应1",
			Model:     "gpt-4o",
			AgentType: "rag_agent",
			TenantID:  123,
			Vector:    make([]float32, 10),
			CreatedAt: time.Now().Unix(),
		},
		{
			CacheID:   "test_002",
			Query:     "查询2",
			Response:  "响应2",
			Model:     "gpt-4o",
			AgentType: "rag_agent",
			TenantID:  123,
			Vector:    make([]float32, 10),
			CreatedAt: time.Now().Unix(),
		},
	}

	for _, entry := range entries {
		err := repo.SetContent(ctx, entry, time.Minute)
		require.NoError(t, err)
	}

	// 批量获取
	cacheIDs := []string{"test_001", "test_002", "test_003"}
	results, err := repo.MGetContents(ctx, cacheIDs)
	require.NoError(t, err)

	assert.Len(t, results, 2)
	assert.Contains(t, results, "test_001")
	assert.Contains(t, results, "test_002")
	assert.NotContains(t, results, "test_003")
}

// TestUpdateHitCount 测试命中计数更新
func TestUpdateHitCount(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	entry := &cache.CacheEntry{
		CacheID:   "test_001",
		Query:     "查询",
		Response:  "响应",
		Model:     "gpt-4o",
		AgentType: "rag_agent",
		TenantID:  123,
		Vector:    make([]float32, 10),
		CreatedAt: time.Now().Unix(),
		HitCount:  5,
	}

	err := repo.SetContent(ctx, entry, time.Minute)
	require.NoError(t, err)

	// 更新命中计数
	err = repo.UpdateHitCount(ctx, "test_001")
	require.NoError(t, err)

	// 验证计数增加
	retrieved, err := repo.GetContent(ctx, "test_001")
	require.NoError(t, err)
	assert.Equal(t, int64(6), retrieved.HitCount)
}

// TestDelete 测试删除
func TestDelete(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	entry := &cache.CacheEntry{
		CacheID:   "test_001",
		Query:     "查询",
		Response:  "响应",
		Model:     "gpt-4o",
		AgentType: "rag_agent",
		TenantID:  123,
		Vector:    make([]float32, 10),
		CreatedAt: time.Now().Unix(),
	}

	err := repo.SetContent(ctx, entry, time.Minute)
	require.NoError(t, err)

	// 删除
	err = repo.Delete(ctx, "test_001")
	require.NoError(t, err)

	// 验证已删除
	_, err = repo.GetContent(ctx, "test_001")
	assert.Error(t, err)
}

// TestStatistics 测试统计功能
func TestStatistics(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	// 记录命中
	err := repo.RecordHit(ctx)
	require.NoError(t, err)

	err = repo.RecordHit(ctx)
	require.NoError(t, err)

	// 记录未命中
	err = repo.RecordMiss(ctx)
	require.NoError(t, err)

	// 获取统计
	stats, err := repo.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.InDelta(t, 0.666, stats.HitRate, 0.01)
}

// TestIncrementSimilarityBucket 测试相似度区间计数
func TestIncrementSimilarityBucket(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	// 增加计数
	buckets := []string{"90-95", "85-90", "95-100"}
	for _, bucket := range buckets {
		err := repo.IncrementSimilarityBucket(ctx, bucket)
		require.NoError(t, err)
	}

	// 验证
	stats, err := repo.GetStats(ctx)
	require.NoError(t, err)

	assert.Len(t, stats.SimilarityDist, 3)
	assert.Equal(t, int64(1), stats.SimilarityDist["90-95"])
	assert.Equal(t, int64(1), stats.SimilarityDist["85-90"])
	assert.Equal(t, int64(1), stats.SimilarityDist["95-100"])
}

// TestClearByTenant 测试按租户清除
func TestClearByTenant(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	// 创建多个条目
	for i := 0; i < 5; i++ {
		entry := &cache.CacheEntry{
			CacheID:   fmt.Sprintf("test_%03d", i),
			Query:     "查询",
			Response:  "响应",
			Model:     "gpt-4o",
			AgentType: "rag_agent",
			TenantID:  123,
			Vector:    make([]float32, 10),
			CreatedAt: time.Now().Unix(),
		}
		err := repo.SetContent(ctx, entry, time.Minute)
		require.NoError(t, err)
	}

	// 清除租户缓存
	count, err := repo.ClearByTenant(ctx, 123)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// 验证已清除
	_, err = repo.GetContent(ctx, "test_000")
	assert.Error(t, err)
}

// TestBuildKey 测试键构建
func TestBuildKey(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)

	tests := []struct {
		name     string
		cacheID  string
		expected string
	}{
		{
			name:     "普通ID",
			cacheID:  "abc123",
			expected: "semantic_cache:abc123",
		},
		{
			name:     "带特殊字符",
			cacheID:  "test-001_xyz",
			expected: "semantic_cache:test-001_xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := repo.buildCacheKey(tt.cacheID)
			assert.Equal(t, tt.expected, key)
		})
	}
}

// TestMarshalUnmarshalEntry 测试序列化反序列化（通过完整流程）
func TestMarshalUnmarshalEntry(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	original := &cache.CacheEntry{
		CacheID:    "test_001",
		Query:      "测试查询",
		Response:   "测试响应",
		Model:      "gpt-4o",
		AgentType:  "rag_agent",
		TenantID:   12345,
		PromptHash: "hash123",
		Vector:     []float32{0.1, 0.2, 0.3, 0.4},
		CreatedAt:  1234567890,
		UpdatedAt:  1234567900,
		HitCount:   42,
		Metadata:   map[string]string{"key1": "value1", "key2": "value2"},
	}

	// 通过 SetContent 和 GetContent 完整测试序列化
	err := repo.SetContent(ctx, original, time.Minute)
	require.NoError(t, err)

	// 获取并验证
	retrieved, err := repo.GetContent(ctx, "test_001")
	require.NoError(t, err)

	assert.Equal(t, original.CacheID, retrieved.CacheID)
	assert.Equal(t, original.Query, retrieved.Query)
	assert.Equal(t, original.Response, retrieved.Response)
	assert.Equal(t, original.Model, retrieved.Model)
	assert.Equal(t, original.AgentType, retrieved.AgentType)
	assert.Equal(t, original.TenantID, retrieved.TenantID)
	assert.Equal(t, original.PromptHash, retrieved.PromptHash)
	assert.Equal(t, original.CreatedAt, retrieved.CreatedAt)
	assert.Equal(t, original.UpdatedAt, retrieved.UpdatedAt)
	assert.Equal(t, original.HitCount, retrieved.HitCount)
	assert.Equal(t, original.Metadata["key1"], retrieved.Metadata["key1"])
}

// ========================================
// Benchmark Tests
// ========================================

// BenchmarkSetContent 性能测试
func BenchmarkSetContent(b *testing.B) {
	mr := miniredis.RunT(b)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	entry := &cache.CacheEntry{
		CacheID:   "bench_001",
		Query:     "基准测试查询",
		Response:  "基准测试响应",
		Model:     "gpt-4o",
		AgentType: "rag_agent",
		TenantID:  1,
		Vector:    make([]float32, 1536),
		CreatedAt: time.Now().Unix(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.CacheID = fmt.Sprintf("bench_%d", i)
		_ = repo.SetContent(ctx, entry, time.Minute)
	}
}

// BenchmarkGetContent 性能测试
func BenchmarkGetContent(b *testing.B) {
	mr := miniredis.RunT(b)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	repo := NewCacheContentRepository(client)
	ctx := context.Background()

	// 预填充
	entry := &cache.CacheEntry{
		CacheID:   "bench_target",
		Query:     "基准测试查询",
		Response:  "基准测试响应",
		Model:     "gpt-4o",
		AgentType: "rag_agent",
		TenantID:  1,
		Vector:    make([]float32, 1536),
		CreatedAt: time.Now().Unix(),
	}
	_ = repo.SetContent(ctx, entry, time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetContent(ctx, "bench_target")
	}
}
