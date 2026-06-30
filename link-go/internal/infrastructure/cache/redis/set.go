// Package redis 提供 Redis 集合操作实现
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"link/internal/model/cache"
)

// ========================================
// Set Cache Implementation
// ========================================

// setCache 实现 SetCache 接口
type setCache struct {
	client *redis.Client
}

// NewSetCache 创建集合缓存实例
func NewSetCache(client *redis.Client) cache.SetCache {
	return &setCache{client: client}
}

// SAdd 向集合添加成员
func (c *setCache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.SAdd(ctx, key, members...).Err()
}

// SRem 从集合移除成员
func (c *setCache) SRem(ctx context.Context, key string, members ...interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.SRem(ctx, key, members...).Err()
}

// SMembers 获取集合所有成员
func (c *setCache) SMembers(ctx context.Context, key string) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.SMembers(ctx, key).Result()
}

// SIsMember 检查成员是否在集合中
func (c *setCache) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	if key == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	return c.client.SIsMember(ctx, key, member).Result()
}

// SCard 获取集合成员数量
func (c *setCache) SCard(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.SCard(ctx, key).Result()
}

// SDiff 获取集合差集
func (c *setCache) SDiff(ctx context.Context, keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.SDiff(ctx, keys...).Result()
}

// SInter 获取集合交集
func (c *setCache) SInter(ctx context.Context, keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.SInter(ctx, keys...).Result()
}

// SUnion 获取集合并集
func (c *setCache) SUnion(ctx context.Context, keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.SUnion(ctx, keys...).Result()
}

// SPop 随机弹出集合成员
func (c *setCache) SPop(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.SPop(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", cache.ErrCacheNotFound
		}
		return "", err
	}

	return val, nil
}

// SPopN 随机弹出多个集合成员
func (c *setCache) SPopN(ctx context.Context, key string, count int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	vals, err := c.client.SPopN(ctx, key, count).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, cache.ErrCacheNotFound
		}
		return nil, err
	}

	return vals, nil
}

// SMIsMember 批量检查成员是否在集合中
func (c *setCache) SMIsMember(ctx context.Context, key string, members ...interface{}) ([]bool, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.SMIsMember(ctx, key, members...).Result()
}

// SMove 将成员从一个集合移动到另一个集合
func (c *setCache) SMove(ctx context.Context, source, destination string, member interface{}) (bool, error) {
	if source == "" || destination == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	return c.client.SMove(ctx, source, destination, member).Result()
}

// SRandMember 随机获取集合成员（不移除）
func (c *setCache) SRandMember(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", cache.ErrCacheKeyEmpty
	}

	val, err := c.client.SRandMember(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", cache.ErrCacheNotFound
		}
		return "", err
	}

	return val, nil
}

// SRandMemberN 随机获取多个集合成员（不移除）
func (c *setCache) SRandMemberN(ctx context.Context, key string, count int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.SRandMemberN(ctx, key, count).Result()
}

// SScan 扫描集合成员
func (c *setCache) SScan(ctx context.Context, key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	if key == "" {
		return nil, 0, cache.ErrCacheKeyEmpty
	}

	keys, nextCursor, err := c.client.SScan(ctx, key, cursor, match, count).Result()
	if err != nil {
		return nil, 0, err
	}

	return keys, nextCursor, nil
}
