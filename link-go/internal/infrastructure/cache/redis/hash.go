// Package redis 提供 Redis 哈希操作实现
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"link/internal/model/cache"
)

// ========================================
// Hash Cache Implementation
// ========================================

// hashCache 实现 HashCache 接口
type hashCache struct {
	client *redis.Client
}

// NewHashCache 创建哈希缓存实例
func NewHashCache(client *redis.Client) cache.HashCache {
	return &hashCache{client: client}
}

// HGet 获取哈希字段值
func (c *hashCache) HGet(ctx context.Context, key, field string) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.HGet(ctx, key, field).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // 返回空字符串表示不存在
		}
		return "", err
	}

	return val, nil
}

// HSet 设置哈希字段值
func (c *hashCache) HSet(ctx context.Context, key, field string, value interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.HSet(ctx, key, field, value).Err()
}

// HMGet 批量获取哈希字段值
func (c *hashCache) HMGet(ctx context.Context, key string, fields ...string) ([]interface{}, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.HMGet(ctx, key, fields...).Result()
}

// HMSet 批量设置哈希字段值
func (c *hashCache) HMSet(ctx context.Context, key string, values map[string]interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.HMSet(ctx, key, values).Err()
}

// HGetAll 获取哈希所有字段值
func (c *hashCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	result, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// HDel 删除哈希字段
func (c *hashCache) HDel(ctx context.Context, key string, fields ...string) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.HDel(ctx, key, fields...).Err()
}

// HExists 检查哈希字段是否存在
func (c *hashCache) HExists(ctx context.Context, key, field string) (bool, error) {
	if key == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	return c.client.HExists(ctx, key, field).Result()
}

// HKeys 获取哈希所有字段名
func (c *hashCache) HKeys(ctx context.Context, key string) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.HKeys(ctx, key).Result()
}

// HLen 获取哈希字段数量
func (c *hashCache) HLen(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.HLen(ctx, key).Result()
}

// HIncrBy 哈希字段值增加指定数值
func (c *hashCache) HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.HIncrBy(ctx, key, field, incr).Result()
}

// HIncrByFloat 哈希字段值增加浮点数
func (c *hashCache) HIncrByFloat(ctx context.Context, key, field string, incr float64) (float64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.HIncrByFloat(ctx, key, field, incr).Result()
}

// HVals 获取哈希所有值
func (c *hashCache) HVals(ctx context.Context, key string) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.HVals(ctx, key).Result()
}

// HScan 扫描哈希字段
func (c *hashCache) HScan(ctx context.Context, key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	if key == "" {
		return nil, 0, cache.ErrCacheKeyEmpty
	}

	keys, nextCursor, err := c.client.HScan(ctx, key, cursor, match, count).Result()
	if err != nil {
		return nil, 0, err
	}

	return keys, nextCursor, nil
}
