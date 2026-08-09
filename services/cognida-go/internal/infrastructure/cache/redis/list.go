// Package redis 提供 Redis 列表操作实现
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// List Cache Implementation
// ========================================

// listCache 实现 ListCache 接口
type listCache struct {
	client *redis.Client
}

// NewListCache 创建列表缓存实例
func NewListCache(client *redis.Client) cache.ListCache {
	return &listCache{client: client}
}

// LPush 从列表左侧插入值
func (c *listCache) LPush(ctx context.Context, key string, values ...interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.LPush(ctx, key, values...).Err()
}

// RPush 从列表右侧插入值
func (c *listCache) RPush(ctx context.Context, key string, values ...interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.RPush(ctx, key, values...).Err()
}

// LPop 从列表左侧弹出值
func (c *listCache) LPop(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.LPop(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", cache.ErrCacheNotFound
		}
		return "", err
	}

	return val, nil
}

// RPop 从列表右侧弹出值
func (c *listCache) RPop(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.RPop(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", cache.ErrCacheNotFound
		}
		return "", err
	}

	return val, nil
}

// LLen 获取列表长度
func (c *listCache) LLen(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.LLen(ctx, key).Result()
}

// LRange 获取列表指定范围元素
func (c *listCache) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.LRange(ctx, key, start, stop).Result()
}

// LTrim 修剪列表，只保留指定范围
func (c *listCache) LTrim(ctx context.Context, key string, start, stop int64) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.LTrim(ctx, key, start, stop).Err()
}

// LIndex 获取列表指定索引元素
func (c *listCache) LIndex(ctx context.Context, key string, index int64) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.LIndex(ctx, key, index).Result()
	if err != nil {
		if err == redis.Nil {
			return "", cache.ErrCacheNotFound
		}
		return "", err
	}

	return val, nil
}

// LSet 设置列表指定索引元素
func (c *listCache) LSet(ctx context.Context, key string, index int64, value interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.LSet(ctx, key, index, value).Err()
}

// LInsert 在列表指定位置插入元素
func (c *listCache) LInsert(ctx context.Context, key, op string, pivot, value interface{}) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	if op == "before" {
		return c.client.LInsertBefore(ctx, key, pivot, value).Result()
	}
	return c.client.LInsertAfter(ctx, key, pivot, value).Result()
}

// LRem 移除列表中指定值的元素
func (c *listCache) LRem(ctx context.Context, key string, count int64, value interface{}) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.LRem(ctx, key, count, value).Result()
}

// LPopN 从列表左侧弹出多个值
func (c *listCache) LPopN(ctx context.Context, key string, count int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.LPopCount(ctx, key, int(count)).Result()
}

// RPopN 从列表右侧弹出多个值
func (c *listCache) RPopN(ctx context.Context, key string, count int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.RPopCount(ctx, key, int(count)).Result()
}
