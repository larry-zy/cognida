// Package redis 提供 Redis 字符串操作单元测试
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

func setupStringCache(t *testing.T) (*miniredis.Miniredis, cache.Cache) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, NewStringCache(client)
}

func TestStringCache_Get(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取存在的键", func(t *testing.T) {
		mr.Set("key1", "value1")
		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("获取不存在的键", func(t *testing.T) {
		_, err := c.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.Get(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_Set(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("设置键值", func(t *testing.T) {
		err := c.Set(ctx, "key1", "value1", 0)
		assert.NoError(t, err)

		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("设置带过期时间的键", func(t *testing.T) {
		err := c.Set(ctx, "key2", "value2", time.Second)
		assert.NoError(t, err)

		// 立即获取应该成功
		val, err := c.Get(ctx, "key2")
		assert.NoError(t, err)
		assert.Equal(t, "value2", val)

		// 快进时间后应该过期
		mr.FastForward(time.Second)
		_, err = c.Get(ctx, "key2")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)
	})

	t.Run("空键", func(t *testing.T) {
		err := c.Set(ctx, "", "value", 0)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_Del(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("删除单个键", func(t *testing.T) {
		mr.Set("key1", "value1")
		err := c.Del(ctx, "key1")
		assert.NoError(t, err)

		_, err = c.Get(ctx, "key1")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)
	})

	t.Run("删除多个键", func(t *testing.T) {
		mr.Set("key1", "value1")
		mr.Set("key2", "value2")
		mr.Set("key3", "value3")

		err := c.Del(ctx, "key1", "key2")
		assert.NoError(t, err)

		_, err = c.Get(ctx, "key1")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)

		_, err = c.Get(ctx, "key2")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)

		val, err := c.Get(ctx, "key3")
		assert.NoError(t, err)
		assert.Equal(t, "value3", val)
	})

	t.Run("删除不存在的键", func(t *testing.T) {
		err := c.Del(ctx, "nonexistent")
		assert.NoError(t, err)
	})

	t.Run("空参数", func(t *testing.T) {
		err := c.Del(ctx)
		assert.NoError(t, err)
	})
}

func TestStringCache_Exists(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("存在的键", func(t *testing.T) {
		mr.Set("key1", "value1")
		exists, err := c.Exists(ctx, "key1")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("不存在的键", func(t *testing.T) {
		exists, err := c.Exists(ctx, "nonexistent")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.Exists(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_Expire(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("设置过期时间", func(t *testing.T) {
		mr.Set("key1", "value1")
		err := c.Expire(ctx, "key1", time.Second)
		assert.NoError(t, err)

		// 立即获取应该成功
		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)

		// 快进时间后应该过期
		mr.FastForward(time.Second)
		_, err = c.Get(ctx, "key1")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)
	})

	t.Run("空键", func(t *testing.T) {
		err := c.Expire(ctx, "", time.Second)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})

	t.Run("无效过期时间", func(t *testing.T) {
		mr.Set("key1", "value1")
		err := c.Expire(ctx, "key1", -1)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)

		err = c.Expire(ctx, "key1", 0)
		assert.ErrorIs(t, err, cache.ErrCacheExpirationInvalid)
	})
}

func TestStringCache_TTL(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取键的TTL", func(t *testing.T) {
		mr.Set("key1", "value1")
		err := c.Set(ctx, "key1", "value1", time.Second*10)
		require.NoError(t, err)

		ttl, err := c.TTL(ctx, "key1")
		assert.NoError(t, err)
		assert.Greater(t, ttl, time.Duration(0))
		assert.LessOrEqual(t, ttl, time.Second*10)
	})

	t.Run("不存在的键", func(t *testing.T) {
		_, err := c.TTL(ctx, "nonexistent")
		assert.ErrorIs(t, err, cache.ErrCacheKeyNotFound)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.TTL(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})

	t.Run("无过期时间的键", func(t *testing.T) {
		mr.Set("key1", "value1")
		ttl, err := c.TTL(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(-1), ttl)
	})
}

func TestStringCache_FlushDB(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("清空数据库", func(t *testing.T) {
		mr.Set("key1", "value1")
		mr.Set("key2", "value2")
		mr.Set("key3", "value3")

		err := c.FlushDB(ctx)
		assert.NoError(t, err)

		_, err = c.Get(ctx, "key1")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)

		_, err = c.Get(ctx, "key2")
		assert.ErrorIs(t, err, cache.ErrCacheNotFound)
	})
}

func TestStringCache_Keys(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取匹配的键", func(t *testing.T) {
		mr.Set("user:1", "value1")
		mr.Set("user:2", "value2")
		mr.Set("session:1", "value3")

		keys, err := c.Keys(ctx, "user:*")
		assert.NoError(t, err)
		assert.Len(t, keys, 2)
		assert.Contains(t, keys, "user:1")
		assert.Contains(t, keys, "user:2")
	})

	t.Run("空模式", func(t *testing.T) {
		_, err := c.Keys(ctx, "")
		assert.ErrorIs(t, err, cache.ErrPatternInvalid)
	})

	t.Run("无匹配", func(t *testing.T) {
		mr.Set("key1", "value1")
		keys, err := c.Keys(ctx, "nonexistent:*")
		assert.NoError(t, err)
		assert.Empty(t, keys)
	})
}

func TestStringCache_MGet(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("批量获取", func(t *testing.T) {
		mr.Set("key1", "value1")
		mr.Set("key2", "value2")
		mr.Set("key3", "value3")

		values, err := c.MGet(ctx, "key1", "key2", "key3")
		assert.NoError(t, err)
		assert.Len(t, values, 3)
		assert.Equal(t, "value1", values[0])
		assert.Equal(t, "value2", values[1])
		assert.Equal(t, "value3", values[2])
	})

	t.Run("部分键不存在", func(t *testing.T) {
		mr.Set("key1", "value1")

		values, err := c.MGet(ctx, "key1", "nonexistent")
		assert.NoError(t, err)
		assert.Len(t, values, 2)
		assert.Equal(t, "value1", values[0])
		assert.Nil(t, values[1])
	})

	t.Run("空参数", func(t *testing.T) {
		values, err := c.MGet(ctx)
		assert.NoError(t, err)
		assert.Empty(t, values)
	})
}

func TestStringCache_MSet(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("批量设置", func(t *testing.T) {
		values := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		err := c.MSet(ctx, values)
		assert.NoError(t, err)

		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("空参数", func(t *testing.T) {
		err := c.MSet(ctx, map[string]interface{}{})
		assert.NoError(t, err)
	})
}

func TestStringCache_Incr(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("自增", func(t *testing.T) {
		mr.Set("counter", "10")

		val, err := c.Incr(ctx, "counter")
		assert.NoError(t, err)
		assert.Equal(t, int64(11), val)

		result, err := c.Get(ctx, "counter")
		assert.NoError(t, err)
		assert.Equal(t, "11", result)
	})

	t.Run("新键自增", func(t *testing.T) {
		val, err := c.Incr(ctx, "new_counter")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.Incr(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_IncrBy(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("自增指定值", func(t *testing.T) {
		mr.Set("counter", "10")

		val, err := c.IncrBy(ctx, "counter", 5)
		assert.NoError(t, err)
		assert.Equal(t, int64(15), val)
	})

	t.Run("负数递减", func(t *testing.T) {
		mr.Set("counter", "10")

		val, err := c.IncrBy(ctx, "counter", -3)
		assert.NoError(t, err)
		assert.Equal(t, int64(7), val)
	})
}

func TestStringCache_Decr(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("自减", func(t *testing.T) {
		mr.Set("counter", "10")

		val, err := c.Decr(ctx, "counter")
		assert.NoError(t, err)
		assert.Equal(t, int64(9), val)
	})
}

func TestStringCache_DecrBy(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("自减指定值", func(t *testing.T) {
		mr.Set("counter", "10")

		val, err := c.DecrBy(ctx, "counter", 3)
		assert.NoError(t, err)
		assert.Equal(t, int64(7), val)
	})
}

func TestStringCache_SetNX(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("设置不存在的键", func(t *testing.T) {
		set, err := c.SetNX(ctx, "new_key", "value", 0)
		assert.NoError(t, err)
		assert.True(t, set)
	})

	t.Run("设置已存在的键", func(t *testing.T) {
		mr.Set("key1", "old_value")

		set, err := c.SetNX(ctx, "key1", "new_value", 0)
		assert.NoError(t, err)
		assert.False(t, set)

		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "old_value", val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.SetNX(ctx, "", "value", 0)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_GetSet(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("设置新值并返回旧值", func(t *testing.T) {
		mr.Set("key1", "old_value")

		oldVal, err := c.GetSet(ctx, "key1", "new_value")
		assert.NoError(t, err)
		assert.Equal(t, "old_value", oldVal)

		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "new_value", val)
	})

	t.Run("新键", func(t *testing.T) {
		oldVal, err := c.GetSet(ctx, "new_key", "value")
		assert.NoError(t, err)
		assert.Equal(t, "", oldVal)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.GetSet(ctx, "", "value")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_Append(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("追加到存在的键", func(t *testing.T) {
		mr.Set("key1", "hello")

		len, err := c.Append(ctx, "key1", " world")
		assert.NoError(t, err)
		assert.Equal(t, int64(11), len)

		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "hello world", val)
	})

	t.Run("追加到不存在的键", func(t *testing.T) {
		len, err := c.Append(ctx, "new_key", "value")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), len)

		val, err := c.Get(ctx, "new_key")
		assert.NoError(t, err)
		assert.Equal(t, "value", val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.Append(ctx, "", "value")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_GetRange(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取范围", func(t *testing.T) {
		mr.Set("key1", "hello world")

		val, err := c.GetRange(ctx, "key1", 0, 4)
		assert.NoError(t, err)
		assert.Equal(t, "hello", val)
	})

	t.Run("负数索引", func(t *testing.T) {
		mr.Set("key1", "hello world")

		val, err := c.GetRange(ctx, "key1", -5, -1)
		assert.NoError(t, err)
		assert.Equal(t, "world", val) // -5 是 'w'，-1 是 'd'
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.GetRange(ctx, "", 0, 10)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_SetRange(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("设置范围", func(t *testing.T) {
		mr.Set("key1", "hello world")

		len, err := c.SetRange(ctx, "key1", 6, "redis")
		assert.NoError(t, err)
		assert.Equal(t, int64(11), len)

		val, err := c.Get(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, "hello redis", val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.SetRange(ctx, "", 0, "value")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestStringCache_StrLen(t *testing.T) {
	mr, c := setupStringCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取字符串长度", func(t *testing.T) {
		mr.Set("key1", "hello")

		len, err := c.StrLen(ctx, "key1")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), len)
	})

	t.Run("不存在的键", func(t *testing.T) {
		len, err := c.StrLen(ctx, "nonexistent")
		assert.NoError(t, err)
		assert.Equal(t, int64(0), len)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.StrLen(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}
