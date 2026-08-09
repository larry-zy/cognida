// Package redis 提供 Redis 有序集合操作实现
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// Sorted Set Cache Implementation
// ========================================

// sortedSetCache 实现 SortedSetCache 接口
type sortedSetCache struct {
	client *redis.Client
}

// NewSortedSetCache 创建有序集合缓存实例
func NewSortedSetCache(client *redis.Client) cache.SortedSetCache {
	return &sortedSetCache{client: client}
}

// ZAdd 向有序集合添加成员
func (c *sortedSetCache) ZAdd(ctx context.Context, key string, score float64, member interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	z := redis.Z{
		Score:  score,
		Member: member,
	}

	return c.client.ZAdd(ctx, key, z).Err()
}

// ZRem 从有序集合移除成员
func (c *sortedSetCache) ZRem(ctx context.Context, key string, members ...interface{}) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	return c.client.ZRem(ctx, key, members...).Err()
}

// ZScore 获取成员分数
func (c *sortedSetCache) ZScore(ctx context.Context, key string, member interface{}) (float64, error) {
	if key == "" {
		return 0, cache.ErrScoreInvalid
	}

	memberStr := fmt.Sprintf("%v", member)
	score, err := c.client.ZScore(ctx, key, memberStr).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, cache.ErrMemberNotExists
		}
		return 0, err
	}

	return score, nil
}

// ZRange 按分数范围获取成员（升序）
func (c *sortedSetCache) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.ZRange(ctx, key, start, stop).Result()
}

// ZRangeByScore 按分数范围获取成员
func (c *sortedSetCache) ZRangeByScore(ctx context.Context, key string, min, max float64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	opt := &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", min),
		Max: fmt.Sprintf("%f", max),
	}

	return c.client.ZRangeByScore(ctx, key, opt).Result()
}

// ZRangeWithScores 按分数范围获取成员及分数（升序）
func (c *sortedSetCache) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]*cache.ScoredMember, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	zs, err := c.client.ZRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}

	result := make([]*cache.ScoredMember, len(zs))
	for i, z := range zs {
		result[i] = &cache.ScoredMember{
			Member: fmt.Sprintf("%v", z.Member),
			Score:  z.Score,
		}
	}

	return result, nil
}

// ZRank 获取成员排名（升序）
func (c *sortedSetCache) ZRank(ctx context.Context, key string, member interface{}) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	memberStr := fmt.Sprintf("%v", member)
	rank, err := c.client.ZRank(ctx, key, memberStr).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, cache.ErrMemberNotExists
		}
		return 0, err
	}

	return rank, nil
}

// ZRevRank 获取成员排名（降序）
func (c *sortedSetCache) ZRevRank(ctx context.Context, key string, member interface{}) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	memberStr := fmt.Sprintf("%v", member)
	rank, err := c.client.ZRevRank(ctx, key, memberStr).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, cache.ErrMemberNotExists
		}
		return 0, err
	}

	return rank, nil
}

// ZCard 获取有序集合成员数量
func (c *sortedSetCache) ZCard(ctx context.Context, key string) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.ZCard(ctx, key).Result()
}

// ZCount 获取分数范围内的成员数量
func (c *sortedSetCache) ZCount(ctx context.Context, key string, min, max float64) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	minStr := fmt.Sprintf("%f", min)
	maxStr := fmt.Sprintf("%f", max)

	return c.client.ZCount(ctx, key, minStr, maxStr).Result()
}

// ZIncrBy 增加成员分数
func (c *sortedSetCache) ZIncrBy(ctx context.Context, key string, increment float64, member interface{}) (float64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	memberStr := fmt.Sprintf("%v", member)
	return c.client.ZIncrBy(ctx, key, increment, memberStr).Result()
}

// ZRevRange 按排名范围获取成员（降序）
func (c *sortedSetCache) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	return c.client.ZRevRange(ctx, key, start, stop).Result()
}

// ZRevRangeWithScores 按排名范围获取成员及分数（降序）
func (c *sortedSetCache) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]*cache.ScoredMember, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	zs, err := c.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}

	result := make([]*cache.ScoredMember, len(zs))
	for i, z := range zs {
		result[i] = &cache.ScoredMember{
			Member: fmt.Sprintf("%v", z.Member),
			Score:  z.Score,
		}
	}

	return result, nil
}

// ZRemRangeByRank 移除指定排名范围内的成员
func (c *sortedSetCache) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	return c.client.ZRemRangeByRank(ctx, key, start, stop).Result()
}

// ZRemRangeByScore 移除指定分数范围内的成员
func (c *sortedSetCache) ZRemRangeByScore(ctx context.Context, key string, min, max float64) (int64, error) {
	if key == "" {
		return 0, cache.ErrCacheKeyEmpty
	}

	minStr := fmt.Sprintf("%f", min)
	maxStr := fmt.Sprintf("%f", max)

	return c.client.ZRemRangeByScore(ctx, key, minStr, maxStr).Result()
}

// ZRangeByScoreWithScores 按分数范围获取成员及分数
func (c *sortedSetCache) ZRangeByScoreWithScores(ctx context.Context, key string, min, max float64) ([]*cache.ScoredMember, error) {
	if key == "" {
		return nil, cache.ErrCacheKeyEmpty
	}

	opt := &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", min),
		Max: fmt.Sprintf("%f", max),
	}

	zs, err := c.client.ZRangeByScoreWithScores(ctx, key, opt).Result()
	if err != nil {
		return nil, err
	}

	result := make([]*cache.ScoredMember, len(zs))
	for i, z := range zs {
		result[i] = &cache.ScoredMember{
			Member: fmt.Sprintf("%v", z.Member),
			Score:  z.Score,
		}
	}

	return result, nil
}

// ZScan 扫描有序集合
func (c *sortedSetCache) ZScan(ctx context.Context, key string, cursor uint64, match string, count int64) ([]string, uint64, error) {
	if key == "" {
		return nil, 0, cache.ErrCacheKeyEmpty
	}

	keys, nextCursor, err := c.client.ZScan(ctx, key, cursor, match, count).Result()
	if err != nil {
		return nil, 0, err
	}

	return keys, nextCursor, nil
}
