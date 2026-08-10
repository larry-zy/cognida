// Package redis 提供 Redis 客户端集成测试
package redis

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cognida/internal/config"
	"cognida/internal/model/cache"
)

// getRedisTestConfig 返回测试用 Redis 配置
// 可以通过环境变量覆盖，否则使用默认值
func getRedisTestConfig() *config.RedisConfig {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	password := os.Getenv("TEST_REDIS_PASSWORD")
	db := 0
	if dbStr := os.Getenv("TEST_REDIS_DB"); dbStr != "" {
		if parsed, err := parseTestDB(dbStr); err == nil {
			db = parsed // 使用环境变量指定的数据库
		}
	}

	return &config.RedisConfig{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
	}
}

func parseTestDB(dbStr string) (int, error) {
	var db int
	if _, err := fmt.Sscanf(dbStr, "%d", &db); err != nil {
		return 0, err
	}
	return db, nil
}

// setupIntegrationTestClient 创建用于集成测试的 Redis 客户端
// 跳过测试如果 Redis 不可用
func setupIntegrationTestClient(t *testing.T) (*RedisClient, *redis.Client) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := getRedisTestConfig()

	// 先尝试创建临时客户端检查连接
	testClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := testClient.Ping(ctx).Err(); err != nil {
		testClient.Close()
		t.Skipf("Redis not available at %s: %v", cfg.Addr, err)
	}
	testClient.Close()

	// 创建正式的客户端
	redisClient, err := NewClient(cfg)
	require.NoError(t, err)

	return redisClient, redisClient.GetClient()
}

func TestRedisClient_NewClient(t *testing.T) {
	t.Run("成功创建客户端", func(t *testing.T) {
		cfg := getRedisTestConfig()

		client, err := NewClient(cfg)
		if err != nil {
			t.Skipf("Redis not available: %v", err)
		}
		require.NoError(t, err)
		require.NotNil(t, client)

		defer client.Close()
	})

	t.Run("nil 配置", func(t *testing.T) {
		_, err := NewClient(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "redis config is nil")
	})

	t.Run("无效数据库索引", func(t *testing.T) {
		cfg := &config.RedisConfig{
			Addr: "localhost:6379",
			DB:   16, // 无效索引（0-15）
		}

		_, err := NewClient(cfg)
		assert.ErrorIs(t, err, cache.ErrDatabaseInvalid)
	})
}

func TestRedisClient_CheckHealth(t *testing.T) {
	client, _ := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()

	t.Run("健康检查成功", func(t *testing.T) {
		err := client.CheckHealth(ctx)
		assert.NoError(t, err)
	})
}

func TestRedisClient_Close(t *testing.T) {
	cfg := getRedisTestConfig()

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	require.NoError(t, err)

	t.Run("关闭客户端", func(t *testing.T) {
		err := client.Close()
		assert.NoError(t, err)
	})
}

func TestRedisClient_Integration_StringCache(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	stringCache := NewStringCache(rawClient)
	testKey := "test:string:1"

	// 清理
	defer rawClient.Del(ctx, testKey)

	t.Run("设置和获取", func(t *testing.T) {
		err := stringCache.Set(ctx, testKey, "test value", time.Minute)
		require.NoError(t, err)

		val, err := stringCache.Get(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, "test value", val)
	})

	t.Run("检查存在", func(t *testing.T) {
		exists, err := stringCache.Exists(ctx, testKey)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("设置过期时间", func(t *testing.T) {
		err := stringCache.Expire(ctx, testKey, time.Second*10)
		assert.NoError(t, err)

		ttl, err := stringCache.TTL(ctx, testKey)
		assert.NoError(t, err)
		assert.Greater(t, ttl, time.Duration(0))
		assert.LessOrEqual(t, ttl, time.Second*10)
	})

	t.Run("删除键", func(t *testing.T) {
		err := stringCache.Del(ctx, testKey)
		assert.NoError(t, err)

		exists, err := stringCache.Exists(ctx, testKey)
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestRedisClient_Integration_HashCache(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	hashCache := NewHashCache(rawClient)
	testKey := "test:hash:1"

	// 清理
	defer rawClient.Del(ctx, testKey)

	t.Run("哈希操作", func(t *testing.T) {
		// 设置字段
		err := hashCache.HSet(ctx, testKey, "field1", "value1")
		require.NoError(t, err)

		err = hashCache.HSet(ctx, testKey, "field2", "value2")
		require.NoError(t, err)

		// 获取字段
		val, err := hashCache.HGet(ctx, testKey, "field1")
		assert.NoError(t, err)
		assert.Equal(t, "value1", val)

		// 获取所有字段
		all, err := hashCache.HGetAll(ctx, testKey)
		assert.NoError(t, err)
		assert.Len(t, all, 2)
		assert.Equal(t, "value1", all["field1"])
		assert.Equal(t, "value2", all["field2"])

		// 检查字段存在
		exists, err := hashCache.HExists(ctx, testKey, "field1")
		assert.NoError(t, err)
		assert.True(t, exists)

		// 获取字段数量
		len, err := hashCache.HLen(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), len)
	})
}

func TestRedisClient_Integration_ListCache(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	listCache := NewListCache(rawClient)
	testKey := "test:list:1"

	// 清理
	defer rawClient.Del(ctx, testKey)

	t.Run("列表操作", func(t *testing.T) {
		// 右侧插入
		err := listCache.RPush(ctx, testKey, "item1", "item2", "item3")
		require.NoError(t, err)

		// 左侧插入
		err = listCache.LPush(ctx, testKey, "item0")
		require.NoError(t, err)

		// 获取长度
		len, err := listCache.LLen(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, int64(4), len)

		// 获取范围
		items, err := listCache.LRange(ctx, testKey, 0, -1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"item0", "item1", "item2", "item3"}, items)

		// 左侧弹出
		item, err := listCache.LPop(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, "item0", item)

		// 右侧弹出
		item, err = listCache.RPop(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, "item3", item)
	})
}

func TestRedisClient_Integration_SetCache(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	setCache := NewSetCache(rawClient)
	testKey := "test:set:1"

	// 清理
	defer rawClient.Del(ctx, testKey)

	t.Run("集合操作", func(t *testing.T) {
		// 添加成员
		err := setCache.SAdd(ctx, testKey, "member1", "member2", "member3")
		require.NoError(t, err)

		// 获取所有成员
		members, err := setCache.SMembers(ctx, testKey)
		assert.NoError(t, err)
		assert.Len(t, members, 3)
		assert.Contains(t, members, "member1")

		// 检查成员存在
		exists, err := setCache.SIsMember(ctx, testKey, "member1")
		assert.NoError(t, err)
		assert.True(t, exists)

		// 获取成员数量
		count, err := setCache.SCard(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)

		// 移除成员
		err = setCache.SRem(ctx, testKey, "member1")
		assert.NoError(t, err)

		exists, err = setCache.SIsMember(ctx, testKey, "member1")
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestRedisClient_Integration_SortedSetCache(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	zsetCache := NewSortedSetCache(rawClient)
	testKey := "test:zset:1"

	// 清理
	defer rawClient.Del(ctx, testKey)

	t.Run("有序集合操作", func(t *testing.T) {
		// 添加成员
		err := zsetCache.ZAdd(ctx, testKey, 1.0, "member1")
		require.NoError(t, err)

		err = zsetCache.ZAdd(ctx, testKey, 2.0, "member2")
		require.NoError(t, err)

		err = zsetCache.ZAdd(ctx, testKey, 3.0, "member3")
		require.NoError(t, err)

		// 获取分数
		score, err := zsetCache.ZScore(ctx, testKey, "member2")
		assert.NoError(t, err)
		assert.Equal(t, 2.0, score)

		// 获取排名
		rank, err := zsetCache.ZRank(ctx, testKey, "member2")
		assert.NoError(t, err)
		assert.Equal(t, int64(1), rank) // 0-based, member2 是第二个

		// 获取范围
		members, err := zsetCache.ZRange(ctx, testKey, 0, -1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"member1", "member2", "member3"}, members)

		// 获取带分数的成员
		scoredMembers, err := zsetCache.ZRangeWithScores(ctx, testKey, 0, -1)
		assert.NoError(t, err)
		assert.Len(t, scoredMembers, 3)
		assert.Equal(t, "member1", scoredMembers[0].Member)
		assert.Equal(t, 1.0, scoredMembers[0].Score)

		// 获取成员数量
		count, err := zsetCache.ZCard(ctx, testKey)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})
}

func TestRedisClient_Integration_PubSub(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	pubsub := NewPubSub(rawClient)

	t.Run("发布订阅", func(t *testing.T) {
		channel := "test:pubsub:1"

		// 订阅频道
		msgChan, err := pubsub.Subscribe(ctx, channel)
		require.NoError(t, err)

		// 发布消息
		err = pubsub.Publish(ctx, channel, "test message")
		require.NoError(t, err)

		// 接收消息
		select {
		case msg := <-msgChan:
			assert.Equal(t, channel, msg.Channel)
			assert.Equal(t, "test message", msg.Payload)
		case <-time.After(time.Second * 2):
			t.Fatal("timeout waiting for message")
		}
	})
}

func TestRedisClient_Integration_Lock(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(rawClient)
	testKey := "test:lock:1"

	// 清理
	defer rawClient.Del(ctx, "lock:"+testKey)

	t.Run("分布式锁", func(t *testing.T) {
		// 尝试获取锁
		acquired, err := lock.TryLock(ctx, testKey, time.Second*10)
		require.NoError(t, err)
		assert.True(t, acquired)

		// 再次尝试应该失败
		acquired, err = lock.TryLock(ctx, testKey, time.Second*10)
		assert.NoError(t, err)
		assert.False(t, acquired)

		// 释放锁
		err = lock.Unlock(ctx, testKey)
		assert.NoError(t, err)

		// 现在应该可以获取锁
		acquired, err = lock.TryLock(ctx, testKey, time.Second*10)
		assert.NoError(t, err)
		assert.True(t, acquired)

		// 清理
		lock.Unlock(ctx, testKey)
	})

	t.Run("锁续期", func(t *testing.T) {
		// 获取锁
		acquired, err := lock.TryLock(ctx, testKey, time.Second*5)
		require.NoError(t, err)
		assert.True(t, acquired)

		// 延长锁
		extended, err := lock.Extend(ctx, testKey, time.Second*20)
		assert.NoError(t, err)
		assert.True(t, extended)

		// 检查 TTL
		ttl := rawClient.TTL(ctx, "lock:"+testKey).Val()
		assert.Greater(t, ttl, time.Second*15)

		// 清理
		lock.Unlock(ctx, testKey)
	})
}

func TestRedisClient_ConcurrentOperations(t *testing.T) {
	client, rawClient := setupIntegrationTestClient(t)
	defer client.Close()

	ctx := context.Background()
	stringCache := NewStringCache(rawClient)

	t.Run("并发读写", func(t *testing.T) {
		const goroutines = 100
		const operations = 10

		errs := make(chan error, goroutines)
		done := make(chan struct{}, goroutines)

		for i := 0; i < goroutines; i++ {
			go func(n int) {
				defer func() { done <- struct{}{} }()
				for j := 0; j < operations; j++ {
					key := "test:concurrent:" + string(rune('a'+n%26))
					if err := stringCache.Set(ctx, key, "value", time.Minute); err != nil {
						errs <- err
						return
					}
					if _, err := stringCache.Get(ctx, key); err != nil && err != cache.ErrCacheNotFound {
						errs <- err
						return
					}
				}
			}(i)
		}

		// 等待所有 goroutine 完成
		for i := 0; i < goroutines; i++ {
			select {
			case <-done:
			case err := <-errs:
				t.Fatalf("concurrent operation failed: %v", err)
			case <-time.After(time.Second * 10):
				t.Fatal("timeout waiting for concurrent operations")
			}
		}
	})
}
