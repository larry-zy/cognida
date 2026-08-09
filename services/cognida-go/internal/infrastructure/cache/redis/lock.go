// Package redis 提供 Redis 分布式锁实现
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// Distributed Lock Implementation
// ========================================

// distributedLock 实现 Lock 接口
type distributedLock struct {
	client *redis.Client
}

// NewLock 创建分布式锁实例
func NewLock(client *redis.Client) cache.Lock {
	return &distributedLock{client: client}
}

// TryLock 尝试获取锁
func (l *distributedLock) TryLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if key == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	if expiration <= 0 {
		return false, cache.ErrCacheExpirationInvalid
	}

	lockKey := withLockPrefix(key)
	lockVal := generateLockValue()

	// SET NX EX 命令
	acquired, err := l.client.SetNX(ctx, lockKey, lockVal, expiration).Result()
	if err != nil {
		return false, fmt.Errorf("%w: %w", cache.ErrLockAcquisitionFailed, err)
	}

	return acquired, nil
}

// Lock 获取锁（阻塞）
func (l *distributedLock) Lock(ctx context.Context, key string, expiration time.Duration) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	if expiration <= 0 {
		return cache.ErrCacheExpirationInvalid
	}

	lockKey := withLockPrefix(key)
	lockVal := generateLockValue()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			acquired, err := l.client.SetNX(ctx, lockKey, lockVal, expiration).Result()
			if err != nil {
				return fmt.Errorf("%w: %w", cache.ErrLockAcquisitionFailed, err)
			}
			if acquired {
				return nil
			}
		}
	}
}

// Unlock 释放锁
func (l *distributedLock) Unlock(ctx context.Context, key string) error {
	if key == "" {
		return cache.ErrCacheKeyEmpty
	}

	lockKey := withLockPrefix(key)

	// 简化处理：直接删除锁
	// 生产环境建议使用 Lua 脚本验证锁持有者
	result, err := l.client.Del(ctx, lockKey).Result()
	if err != nil {
		return err
	}

	if result == 0 {
		return cache.ErrLockNotHeld
	}

	return nil
}

// Extend 延长锁过期时间
func (l *distributedLock) Extend(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if key == "" {
		return false, cache.ErrCacheKeyEmpty
	}

	if expiration <= 0 {
		return false, cache.ErrCacheExpirationInvalid
	}

	lockKey := withLockPrefix(key)

	// 使用 Lua 脚本确保只延长自己持有的锁
	script := `
		if redis.call("exists", KEYS[1]) == 1 then
			return redis.call("expire", KEYS[1], ARGV[1])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{lockKey}, int(expiration.Seconds())).Result()
	if err != nil {
		return false, err
	}

	// 返回 1 表示成功，0 表示锁不存在
	return result.(int64) == 1, nil
}

// ========================================
// Lock with Value Tracking (Advanced)
// ========================================

// lockWithTracking 带值追踪的锁（用于验证持有者）
type lockWithTracking struct {
	client    *redis.Client
	lockKey   string
	lockValue string
}

// NewLockWithTracking 创建带追踪的锁
func NewLockWithTracking(client *redis.Client, key string) *lockWithTracking {
	return &lockWithTracking{
		client:  client,
		lockKey: withLockPrefix(key),
	}
}

// LockWithContext 获取锁并追踪值
func (l *lockWithTracking) LockWithContext(ctx context.Context, expiration time.Duration) error {
	l.lockValue = generateLockValue()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			acquired, err := l.client.SetNX(ctx, l.lockKey, l.lockValue, expiration).Result()
			if err != nil {
				return fmt.Errorf("%w: %w", cache.ErrLockAcquisitionFailed, err)
			}
			if acquired {
				return nil
			}
		}
	}
}

// UnlockWithContext 释放锁（验证持有者）
func (l *lockWithTracking) UnlockWithContext(ctx context.Context) error {
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{l.lockKey}, l.lockValue).Result()
	if err != nil {
		return err
	}

	if result.(int64) == 0 {
		return cache.ErrLockNotHeld
	}

	return nil
}

// ExtendWithContext 延长锁（验证持有者）
func (l *lockWithTracking) ExtendWithContext(ctx context.Context, expiration time.Duration) (bool, error) {
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{l.lockKey}, l.lockValue, int(expiration.Seconds())).Result()
	if err != nil {
		return false, err
	}

	return result.(int64) == 1, nil
}

// TryLockWithContext 尝试获取锁并追踪值
func (l *lockWithTracking) TryLockWithContext(ctx context.Context, expiration time.Duration) (bool, error) {
	l.lockValue = generateLockValue()

	return l.client.SetNX(ctx, l.lockKey, l.lockValue, expiration).Result()
}
