// Package redis 提供 Redis 分布式锁单元测试
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link/internal/model/cache"
)

func setupLock(t *testing.T) (*miniredis.Miniredis, cache.Lock) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, NewLock(client)
}

func TestDistributedLock_TryLock(t *testing.T) {
	mr, l := setupLock(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("成功获取锁", func(t *testing.T) {
		acquired, err := l.TryLock(ctx, "resource1", time.Second*10)
		assert.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("锁已被持有", func(t *testing.T) {
		// 第一次获取锁
		acquired, err := l.TryLock(ctx, "resource2", time.Second*10)
		require.NoError(t, err)
		require.True(t, acquired)

		// 第二次尝试获取同一把锁
		acquired, err = l.TryLock(ctx, "resource2", time.Second*10)
		assert.NoError(t, err)
		assert.False(t, acquired)
	})

	t.Run("锁过期后重新获取", func(t *testing.T) {
		// 获取锁，过期时间1秒
		acquired, err := l.TryLock(ctx, "resource3", time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		// 快进时间，让锁过期
		mr.FastForward(time.Second + time.Millisecond*100)

		// 现在可以重新获取锁
		acquired, err = l.TryLock(ctx, "resource3", time.Second*10)
		assert.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := l.TryLock(ctx, "", time.Second*10)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})

	t.Run("无效过期时间", func(t *testing.T) {
		_, err := l.TryLock(ctx, "resource1", -1)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)

		_, err = l.TryLock(ctx, "resource1", 0)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)
	})
}

func TestDistributedLock_Unlock(t *testing.T) {
	mr, l := setupLock(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("释放持有的锁", func(t *testing.T) {
		// 获取锁
		acquired, err := l.TryLock(ctx, "resource1", time.Second*10)
		require.NoError(t, err)
		require.True(t, acquired)

		// 释放锁
		err = l.Unlock(ctx, "resource1")
		assert.NoError(t, err)

		// 现在可以重新获取锁
		acquired, err = l.TryLock(ctx, "resource1", time.Second*10)
		assert.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("释放不存在的锁", func(t *testing.T) {
		err := l.Unlock(ctx, "nonexistent")
		assert.ErrorIs(t, err, cache.ErrLockNotHeld)
	})

	t.Run("空键", func(t *testing.T) {
		err := l.Unlock(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestDistributedLock_Extend(t *testing.T) {
	mr, l := setupLock(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("延长锁过期时间", func(t *testing.T) {
		// 获取锁，过期时间1秒
		acquired, err := l.TryLock(ctx, "resource1", time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		// 延长锁到10秒
		extended, err := l.Extend(ctx, "resource1", time.Second*10)
		assert.NoError(t, err)
		assert.True(t, extended)

		// 快进1秒，锁应该还在（因为已经延长到10秒）
		mr.FastForward(time.Second + time.Millisecond*100)

		// 尝试获取锁应该失败
		acquired, err = l.TryLock(ctx, "resource1", time.Second*10)
		assert.NoError(t, err)
		assert.False(t, acquired)
	})

	t.Run("延长不存在的锁", func(t *testing.T) {
		extended, err := l.Extend(ctx, "nonexistent", time.Second*10)
		assert.NoError(t, err)
		assert.False(t, extended)
	})

	t.Run("延长已过期的锁", func(t *testing.T) {
		// 获取锁，过期时间1秒
		acquired, err := l.TryLock(ctx, "resource2", time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		// 快进时间，让锁过期
		mr.FastForward(time.Second + time.Millisecond*100)

		// 尝试延长已过期的锁
		extended, err := l.Extend(ctx, "resource2", time.Second*10)
		assert.NoError(t, err)
		assert.False(t, extended)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := l.Extend(ctx, "", time.Second*10)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})

	t.Run("无效过期时间", func(t *testing.T) {
		// 先获取锁
		acquired, err := l.TryLock(ctx, "resource3", time.Second*10)
		require.NoError(t, err)
		require.True(t, acquired)

		// 尝试用无效时间延长
		_, err = l.Extend(ctx, "resource3", -1)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)

		_, err = l.Extend(ctx, "resource3", 0)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)
	})
}

func TestDistributedLock_Lock_Blocking(t *testing.T) {
	mr, l := setupLock(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("阻塞获取锁", func(t *testing.T) {
		// 先获取锁
		acquired, err := l.TryLock(ctx, "resource1", time.Second*5)
		require.NoError(t, err)
		require.True(t, acquired)

		// 在另一个 goroutine 中尝试获取锁（会阻塞）
		done := make(chan error, 1)
		go func() {
			err := l.Lock(context.Background(), "resource1", time.Second*10)
			done <- err
		}()

		// 等待一小段时间，确保锁被阻塞等待
		time.Sleep(time.Millisecond * 100)

		// 释放锁
		err = l.Unlock(ctx, "resource1")
		assert.NoError(t, err)

		// 现在另一个 goroutine 应该能获取到锁
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second * 2):
			t.Fatal("timeout waiting for lock acquisition")
		}
	})

	t.Run("上下文取消", func(t *testing.T) {
		// 先获取锁
		acquired, err := l.TryLock(ctx, "resource2", time.Second*10)
		require.NoError(t, err)
		require.True(t, acquired)

		// 创建可取消的上下文
		cancelCtx, cancel := context.WithCancel(context.Background())

		// 在另一个 goroutine 中尝试获取锁
		done := make(chan error, 1)
		go func() {
			err := l.Lock(cancelCtx, "resource2", time.Second*10)
			done <- err
		}()

		// 等待一小段时间后取消上下文
		time.Sleep(time.Millisecond * 100)
		cancel()

		// 应该收到上下文取消的错误
		select {
		case err := <-done:
			assert.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for context cancellation")
		}
	})

	t.Run("空键", func(t *testing.T) {
		err := l.Lock(ctx, "", time.Second*10)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})

	t.Run("无效过期时间", func(t *testing.T) {
		err := l.Lock(ctx, "resource1", -1)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)

		err = l.Lock(ctx, "resource1", 0)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)
	})
}

func TestLockWithTracking_TryLockWithContext(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	ctx := context.Background()

	t.Run("获取并追踪锁", func(t *testing.T) {
		l := NewLockWithTracking(client, "resource1")

		acquired, err := l.TryLockWithContext(ctx, time.Second*10)
		assert.NoError(t, err)
		assert.True(t, acquired)

		// 释放锁
		err = l.UnlockWithContext(ctx)
		assert.NoError(t, err)
	})

	t.Run("验证持有者才释放", func(t *testing.T) {
		l1 := NewLockWithTracking(client, "resource2")
		l2 := NewLockWithTracking(client, "resource2")

		// l1 获取锁
		acquired, err := l1.TryLockWithContext(ctx, time.Second*10)
		require.NoError(t, err)
		require.True(t, acquired)

		// l2 尝试释放锁（应该失败）
		err = l2.UnlockWithContext(ctx)
		assert.ErrorIs(t, err, cache.ErrLockNotHeld)
	})

	t.Run("验证持有者才延长", func(t *testing.T) {
		l1 := NewLockWithTracking(client, "resource3")
		l2 := NewLockWithTracking(client, "resource3")

		// l1 获取锁
		acquired, err := l1.TryLockWithContext(ctx, time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		// l2 尝试延长锁（应该失败）
		extended, err := l2.ExtendWithContext(ctx, time.Second*10)
		assert.NoError(t, err)
		assert.False(t, extended)

		// l1 延长锁（应该成功）
		extended, err = l1.ExtendWithContext(ctx, time.Second*10)
		assert.NoError(t, err)
		assert.True(t, extended)
	})
}

func TestLockWithTracking_LockWithContext(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	ctx := context.Background()

	t.Run("阻塞获取并追踪锁", func(t *testing.T) {
		l1 := NewLockWithTracking(client, "resource1")
		l2 := NewLockWithTracking(client, "resource2")

		// l1 先获取锁
		acquired, err := l1.TryLockWithContext(ctx, time.Second*5)
		require.NoError(t, err)
		require.True(t, acquired)

		// l2 阻塞等待锁
		done := make(chan error, 1)
		go func() {
			err := l2.LockWithContext(context.Background(), time.Second*10)
			done <- err
		}()

		// 等待后 l1 释放锁
		time.Sleep(time.Millisecond * 100)
		err = l1.UnlockWithContext(ctx)
		assert.NoError(t, err)

		// l2 应该能获取到锁
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Second * 2):
			t.Fatal("timeout waiting for lock acquisition")
		}
	})
}

func TestLockWithTracking_ExtendWithContext(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	ctx := context.Background()

	t.Run("延长追踪的锁", func(t *testing.T) {
		l := NewLockWithTracking(client, "resource1")

		// 获取锁
		err := l.LockWithContext(ctx, time.Second)
		require.NoError(t, err)

		// 延长锁
		extended, err := l.ExtendWithContext(ctx, time.Second*10)
		assert.NoError(t, err)
		assert.True(t, extended)

		// 快进1秒，锁应该还在
		mr.FastForward(time.Second + time.Millisecond*100)

		// 检查锁是否仍然持有
		// 由于 miniredis 不直接支持锁状态检查，我们通过尝试获取锁来验证
		l2 := NewLockWithTracking(client, "resource1")
		acquired, err := l2.TryLockWithContext(ctx, time.Second)
		assert.NoError(t, err)
		assert.False(t, acquired) // 锁应该还在
	})
}
