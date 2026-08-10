// Package redis 提供 Redis 分布式锁实现
package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// Distributed Lock Implementation
// ========================================

// distributedLock 实现 Lock 接口。
// Lock 接口的 Unlock/Extend 只接受 key，持有者令牌无法由调用方传入，
// 因此这里用 mu 保护的 values 表按 lockKey 记录本进程持有的令牌，
// 供 Unlock/Extend 做 Lua CAS——只释放/续期自己持有的锁。
type distributedLock struct {
	client *redis.Client
	mu     sync.Mutex
	values map[string]string // lockKey -> owner token
}

// NewLock 创建分布式锁实例
func NewLock(client *redis.Client) cache.Lock {
	return &distributedLock{
		client: client,
		values: make(map[string]string),
	}
}

// storeValue 记录本进程对 lockKey 的持有令牌
func (l *distributedLock) storeValue(lockKey, val string) {
	l.mu.Lock()
	l.values[lockKey] = val
	l.mu.Unlock()
}

// loadValue 读取本进程对 lockKey 的持有令牌
func (l *distributedLock) loadValue(lockKey string) (string, bool) {
	l.mu.Lock()
	val, ok := l.values[lockKey]
	l.mu.Unlock()
	return val, ok
}

// clearValue 清除本进程对 lockKey 的持有记录
func (l *distributedLock) clearValue(lockKey string) {
	l.mu.Lock()
	delete(l.values, lockKey)
	l.mu.Unlock()
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
	acquired, err := setNX(ctx, l.client, lockKey, lockVal, expiration)
	if err != nil {
		return false, fmt.Errorf("%w: %w", cache.ErrLockAcquisitionFailed, err)
	}

	if acquired {
		l.storeValue(lockKey, lockVal)
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
			acquired, err := setNX(ctx, l.client, lockKey, lockVal, expiration)
			if err != nil {
				return fmt.Errorf("%w: %w", cache.ErrLockAcquisitionFailed, err)
			}
			if acquired {
				l.storeValue(lockKey, lockVal)
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

	lockVal, ok := l.loadValue(lockKey)
	if !ok {
		// 本进程没有持有记录——不是自己加的锁，拒绝释放。
		return cache.ErrLockNotHeld
	}

	// Lua CAS：仅当值匹配才删除，避免误删他人（或本进程已过期后被他人重获）的锁。
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{lockKey}, lockVal).Result()
	if err != nil {
		return err
	}

	// 无论 Redis 侧是否仍持有，都清掉本地记录：令牌已失效，不应再用于后续 CAS。
	l.clearValue(lockKey)

	if n, _ := result.(int64); n == 0 {
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

	lockVal, ok := l.loadValue(lockKey)
	if !ok {
		// 本进程没有持有记录，无从续期。
		return false, nil
	}

	// Lua CAS：仅当值匹配（确系本进程持有）才续期。
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{lockKey}, lockVal, int(expiration.Seconds())).Result()
	if err != nil {
		return false, err
	}

	// 返回 1 表示成功续期，0 表示锁已不由本进程持有
	n, _ := result.(int64)
	return n == 1, nil
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
			acquired, err := setNX(ctx, l.client, l.lockKey, l.lockValue, expiration)
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

	return setNX(ctx, l.client, l.lockKey, l.lockValue, expiration)
}

// setNX 用 SET key val NX EX 原子获取锁，替代已弃用的 SetNX（语义等价：
// bool 表示是否获取成功）。SetArgs 在键已存在时返回 redis.Nil，这里归一化为
// (false, nil)——未抢到锁不是错误；仅真实 IO/协议错误才回传 error。
func setNX(ctx context.Context, client *redis.Client, key string, val interface{}, expiration time.Duration) (bool, error) {
	res, err := client.SetArgs(ctx, key, val, redis.SetArgs{Mode: "NX", TTL: expiration}).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return res == "OK", nil
}
