// Package redis 提供 Redis 哈希操作单元测试
package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cognida/internal/model/cache"
)

func setupHashCache(t *testing.T) (*miniredis.Miniredis, cache.HashCache) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, NewHashCache(client)
}

func TestHashCache_HGet(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取存在的字段", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "field1", "value1")
		require.NoError(t, err)

		val, err := c.HGet(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("获取不存在的字段", func(t *testing.T) {
		val, err := c.HGet(ctx, "hash1", "nonexistent")
		assert.NoError(t, err)
		assert.Equal(t, "", val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HGet(ctx, "", "field1")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HSet(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("设置字段", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "field1", "value1")
		assert.NoError(t, err)

		val, err := c.HGet(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("更新字段", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "field1", "value1")
		require.NoError(t, err)

		err = c.HSet(ctx, "hash1", "field1", "value2")
		assert.NoError(t, err)

		val, err := c.HGet(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.Equal(t, "value2", val)
	})

	t.Run("空键", func(t *testing.T) {
		err := c.HSet(ctx, "", "field1", "value1")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HMGet(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("批量获取字段", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		values, err := c.HMGet(ctx, "hash1", "field1", "field2", "field3")
		assert.NoError(t, err)
		assert.Len(t, values, 3)
		assert.Equal(t, "value1", values[0])
		assert.Equal(t, "value2", values[1])
		assert.Equal(t, "value3", values[2])
	})

	t.Run("部分字段不存在", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "field1", "value1")
		require.NoError(t, err)

		values, err := c.HMGet(ctx, "hash1", "field1", "nonexistent")
		assert.NoError(t, err)
		assert.Len(t, values, 2)
		assert.Equal(t, "value1", values[0])
		assert.Nil(t, values[1])
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HMGet(ctx, "", "field1")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HMSet(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("批量设置字段", func(t *testing.T) {
		values := map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		}

		err := c.HMSet(ctx, "hash1", values)
		assert.NoError(t, err)

		val, err := c.HGet(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)
	})

	t.Run("空键", func(t *testing.T) {
		err := c.HMSet(ctx, "", map[string]interface{}{"field1": "value1"})
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HGetAll(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取所有字段", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		result, err := c.HGetAll(ctx, "hash1")
		assert.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, "value1", result["field1"])
		assert.Equal(t, "value2", result["field2"])
		assert.Equal(t, "value3", result["field3"])
	})

	t.Run("空哈希", func(t *testing.T) {
		result, err := c.HGetAll(ctx, "empty_hash")
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HGetAll(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HDel(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("删除单个字段", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "field1", "value1")
		require.NoError(t, err)

		err = c.HDel(ctx, "hash1", "field1")
		assert.NoError(t, err)

		val, err := c.HGet(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.Equal(t, "", val)
	})

	t.Run("删除多个字段", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		err = c.HDel(ctx, "hash1", "field1", "field2")
		assert.NoError(t, err)

		val, err := c.HGet(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.Equal(t, "", val)

		val, err = c.HGet(ctx, "hash1", "field3")
		assert.NoError(t, err)
		assert.Equal(t, "value3", val)
	})

	t.Run("删除不存在的字段", func(t *testing.T) {
		err := c.HDel(ctx, "hash1", "nonexistent")
		assert.NoError(t, err)
	})

	t.Run("空键", func(t *testing.T) {
		err := c.HDel(ctx, "", "field1")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HExists(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("字段存在", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "field1", "value1")
		require.NoError(t, err)

		exists, err := c.HExists(ctx, "hash1", "field1")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("字段不存在", func(t *testing.T) {
		exists, err := c.HExists(ctx, "hash1", "nonexistent")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HExists(ctx, "", "field1")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HKeys(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取所有字段名", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		keys, err := c.HKeys(ctx, "hash1")
		assert.NoError(t, err)
		assert.Len(t, keys, 3)
		assert.Contains(t, keys, "field1")
		assert.Contains(t, keys, "field2")
		assert.Contains(t, keys, "field3")
	})

	t.Run("空哈希", func(t *testing.T) {
		keys, err := c.HKeys(ctx, "empty_hash")
		assert.NoError(t, err)
		assert.Empty(t, keys)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HKeys(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HLen(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取字段数量", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		len, err := c.HLen(ctx, "hash1")
		assert.NoError(t, err)
		assert.Equal(t, int64(3), len)
	})

	t.Run("空哈希", func(t *testing.T) {
		len, err := c.HLen(ctx, "empty_hash")
		assert.NoError(t, err)
		assert.Equal(t, int64(0), len)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HLen(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HIncrBy(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("字段值增加", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "counter", "10")
		require.NoError(t, err)

		val, err := c.HIncrBy(ctx, "hash1", "counter", 5)
		assert.NoError(t, err)
		assert.Equal(t, int64(15), val)

		result, err := c.HGet(ctx, "hash1", "counter")
		assert.NoError(t, err)
		assert.Equal(t, "15", result)
	})

	t.Run("新字段自增", func(t *testing.T) {
		val, err := c.HIncrBy(ctx, "hash1", "new_counter", 1)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), val)
	})

	t.Run("负数递减", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "counter", "10")
		require.NoError(t, err)

		val, err := c.HIncrBy(ctx, "hash1", "counter", -3)
		assert.NoError(t, err)
		assert.Equal(t, int64(7), val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HIncrBy(ctx, "", "counter", 1)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HIncrByFloat(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("浮点数增加", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "price", "10.5")
		require.NoError(t, err)

		val, err := c.HIncrByFloat(ctx, "hash1", "price", 2.3)
		assert.NoError(t, err)
		assert.Equal(t, float64(12.8), val)
	})

	t.Run("新字段自增", func(t *testing.T) {
		val, err := c.HIncrByFloat(ctx, "hash1", "new_field", 1.5)
		assert.NoError(t, err)
		assert.Equal(t, float64(1.5), val)
	})

	t.Run("负数递减", func(t *testing.T) {
		err := c.HSet(ctx, "hash1", "price", "10.5")
		require.NoError(t, err)

		val, err := c.HIncrByFloat(ctx, "hash1", "price", -2.5)
		assert.NoError(t, err)
		assert.Equal(t, float64(8.0), val)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HIncrByFloat(ctx, "", "field", 1.5)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HVals(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("获取所有值", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
		})
		require.NoError(t, err)

		vals, err := c.HVals(ctx, "hash1")
		assert.NoError(t, err)
		assert.Len(t, vals, 3)
		assert.Contains(t, vals, "value1")
		assert.Contains(t, vals, "value2")
		assert.Contains(t, vals, "value3")
	})

	t.Run("空哈希", func(t *testing.T) {
		vals, err := c.HVals(ctx, "empty_hash")
		assert.NoError(t, err)
		assert.Empty(t, vals)
	})

	t.Run("空键", func(t *testing.T) {
		_, err := c.HVals(ctx, "")
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}

func TestHashCache_HScan(t *testing.T) {
	mr, c := setupHashCache(t)
	defer mr.Close()

	ctx := context.Background()

	t.Run("扫描哈希", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
			"field3": "value3",
			"field4": "value4",
		})
		require.NoError(t, err)

		keys, cursor, err := c.HScan(ctx, "hash1", 0, "", 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(keys), 4)
		assert.Equal(t, uint64(0), cursor)
	})

	t.Run("带模式匹配", func(t *testing.T) {
		err := c.HMSet(ctx, "hash1", map[string]interface{}{
			"user:1": "value1",
			"user:2": "value2",
			"session:1": "value3",
		})
		require.NoError(t, err)

		keys, cursor, err := c.HScan(ctx, "hash1", 0, "user:*", 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(keys), 2)
		assert.Equal(t, uint64(0), cursor)
	})

	t.Run("空键", func(t *testing.T) {
		_, _, err := c.HScan(ctx, "", 0, "", 10)
		assert.ErrorIs(t, err, cache.ErrCacheKeyEmpty)
	})
}
