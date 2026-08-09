// Package redis 提供 Redis 字符串操作实现
package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// String Cache Implementation
// ========================================

// StringCache 实现 Cache 接口
type StringCache struct {
	client *redis.Client
}

// NewStringCache 创建字符串缓存实例
func NewStringCache(client *redis.Client) cache.Cache {
	return &StringCache{client: client}
}

// Get 获取缓存值
func (c *StringCache) Get(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", cache.ErrCacheNotFound
		}
		return "", err
	}

	return val, nil
}

// Set 设置缓存值
func (c *StringCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.Set(ctx, key, value, expiration).Err()
}

// Del 删除缓存
func (c *StringCache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	return c.client.Del(ctx, keys...).Err()
}

// Exists 检查键是否存在
func (c *StringCache) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// Expire 设置过期时间
func (c *StringCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	if expiration <= 0 {
		return cache.ErrCacheExpirationInvalid
	}

	return c.client.Expire(ctx, key, expiration).Err()
}

// TTL 获取剩余过期时间
func (c *StringCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	ttl, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// redis.Returns -2 for keys that don't exist, -1 for keys with no expiry
	if ttl < -1 {
		return 0, cache.ErrCacheKeyNotFound
	}

	return ttl, nil
}

// FlushDB 清空当前数据库
func (c *StringCache) FlushDB(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// Keys 获取匹配模式的所有键
func (c *StringCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	if pattern == "" {
		return nil, cache.ErrPatternInvalid
	}

	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	return keys, nil
}

// ========================================
// Additional Utility Methods
// ========================================

// MGet 批量获取多个键的值
func (c *StringCache) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	if len(keys) == 0 {
		return []interface{}{}, nil
	}

	return c.client.MGet(ctx, keys...).Result()
}

// MSet 批量设置多个键值对
func (c *StringCache) MSet(ctx context.Context, values map[string]interface{}) error {
	if len(values) == 0 {
		return nil
	}

	pairs := make([]interface{}, 0, len(values)*2)
	for k, v := range values {
		pairs = append(pairs, k, v)
	}

	return c.client.MSet(ctx, pairs...).Err()
}

// Incr 自增
func (c *StringCache) Incr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.Incr(ctx, key).Result()
}

// IncrBy 自增指定值
func (c *StringCache) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.IncrBy(ctx, key, value).Result()
}

// Decr 自减
func (c *StringCache) Decr(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.Decr(ctx, key).Result()
}

// DecrBy 自减指定值
func (c *StringCache) DecrBy(ctx context.Context, key string, value int64) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.DecrBy(ctx, key, value).Result()
}

// SetNX 设置键值（仅当键不存在时）
func (c *StringCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	if key == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	return setNX(ctx, c.client, key, value, expiration)
}

// GetSet 设置新值并返回旧值
func (c *StringCache) GetSet(ctx context.Context, key string, value interface{}) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	// SetArgs{Get:true} 等价于已弃用的 GETSET：写入新值并返回旧值，键不存在时返回 redis.Nil。
	val, err := c.client.SetArgs(ctx, key, value, redis.SetArgs{Get: true}).Result()
	if err == redis.Nil {
		return "", nil // 返回空字符串表示键不存在
	}
	if err != nil {
		return "", err
	}

	return val, nil
}

// Append 追加值到键
func (c *StringCache) Append(ctx context.Context, key string, value string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.Append(ctx, key, value).Result()
}

// GetRange 获取键值的子字符串
func (c *StringCache) GetRange(ctx context.Context, key string, start, end int64) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	return c.client.GetRange(ctx, key, start, end).Result()
}

// SetRange 设置键值的子字符串
func (c *StringCache) SetRange(ctx context.Context, key string, offset int64, value string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.SetRange(ctx, key, offset, value).Result()
}

// StrLen 获取键值的长度
func (c *StringCache) StrLen(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.StrLen(ctx, key).Result()
}
