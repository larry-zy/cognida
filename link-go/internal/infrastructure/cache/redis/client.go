// Package redis 提供 Redis 客户端实现
package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"link/internal/model/cache"
	"link/internal/infrastructure/config"
)

var (
	// Client 全局 Redis 客户端（单例）
	Client *redis.Client
)

// ========================================
// Redis Client
// ========================================

// RedisClient Redis 客户端封装
type RedisClient struct {
	client *redis.Client
	config *config.RedisConfig
}

// NewClient 创建 Redis 客户端
func NewClient(cfg *config.RedisConfig) (*RedisClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("redis config is nil")
	}

	// 验证数据库索引
	if cfg.DB < 0 || cfg.DB > 15 {
		return nil, cache.ErrDatabaseInvalid
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", cache.ErrConnectionFailed, err)
	}

	Client = client
	log.Printf("✅ Redis connected: %s, DB: %d\n", cfg.Addr, cfg.DB)

	return &RedisClient{
		client: client,
		config: cfg,
	}, nil
}

// GetClient 获取原生 Redis 客户端
func (c *RedisClient) GetClient() *redis.Client {
	return c.client
}

// CheckHealth 检查 Redis 连接健康状态
func (c *RedisClient) CheckHealth(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: %w", cache.ErrConnectionFailed, err)
	}
	return nil
}

// Close 关闭 Redis 连接
func (c *RedisClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// ========================================
// Helper Functions
// ========================================

// withLockPrefix 添加锁前缀
func withLockPrefix(key string) string {
	return "lock:" + key
}

// generateLockValue 生成锁的唯一值
func generateLockValue() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), "lock")
}
