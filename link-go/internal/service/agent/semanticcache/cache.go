// Package semanticcache 实现指标语义层的 Verified/Golden Query 缓存。
//
// 缓存键 = 租户 + 语义模型名 + 模型版本 + 结构化取数请求的规范化签名。
// 因版本进入键，任何语义变更 bump 版本后旧键自然失效（键不再命中）；TTL 兜底陈旧。
// 命中返回「受信」的治理 SQL（Verified 为引擎生成、Golden 为人工策展且优先），
// 后续仍由 sql_execute 沿用 result_id 信封执行取数。
package semanticcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"link/internal/service/agent/metricsql"
)

// DefaultTTL 受信查询缓存默认存活时长（远长于 Result Store，语义稳定时可复用）。
const DefaultTTL = 24 * time.Hour

// Entry 是一条受信查询记录。
type Entry struct {
	// SQL 治理口径下的 SQL（引擎生成或人工策展）。
	SQL string `json:"sql"`
	// ModelVersion 生成该条记录时的语义模型版本（与键中版本冗余，便于观测）。
	ModelVersion int `json:"model_version"`
	// Verified 经引擎按治理口径生成即为受信。
	Verified bool `json:"verified"`
	// Golden 人工策展的黄金查询，优先于引擎输出。
	Golden bool `json:"golden,omitempty"`
	// CreatedAt Unix 秒。
	CreatedAt int64 `json:"created_at"`
}

// Cache 是受信查询缓存读写接口。
type Cache interface {
	Get(ctx context.Context, key string) (*Entry, bool, error)
	Put(ctx context.Context, key string, e *Entry, ttl time.Duration) error
}

// BuildKey 构造缓存键：tenant + model + version + 规范化请求签名。
func BuildKey(tenantID int64, model string, version int, q metricsql.Query) string {
	sig := signature(q)
	return fmt.Sprintf("%d:%s:v%d:%s", tenantID, strings.ToLower(strings.TrimSpace(model)), version, sig)
}

// canonicalQuery 是用于签名的规范化视图：名称统一 trim+lower（过滤/排序值保持原样，
// 因其为字面数据；SELECT 列顺序有意义，故不排序 metrics/dimensions）。
type canonicalQuery struct {
	Metrics    []string             `json:"m"`
	Dimensions []string             `json:"d"`
	Filters    []metricsql.Filter   `json:"f,omitempty"`
	OrderBy    []metricsql.OrderKey `json:"o,omitempty"`
	Limit      int                  `json:"l,omitempty"`
}

// signature 返回请求的稳定 SHA-256 签名。
func signature(q metricsql.Query) string {
	cq := canonicalQuery{
		Metrics:    normNames(q.Metrics),
		Dimensions: normNames(q.Dimensions),
		OrderBy:    make([]metricsql.OrderKey, 0, len(q.OrderBy)),
		Limit:      q.Limit,
	}
	for _, f := range q.Filters {
		cq.Filters = append(cq.Filters, metricsql.Filter{
			Field:  norm(f.Field),
			Op:     f.Op,
			Values: f.Values,
		})
	}
	for _, o := range q.OrderBy {
		cq.OrderBy = append(cq.OrderBy, metricsql.OrderKey{Field: norm(o.Field), Desc: o.Desc})
	}
	data, _ := json.Marshal(cq)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, norm(s))
	}
	return out
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ========================================
// Redis 后端
// ========================================

const redisKeyPrefix = "agent:semanticcache:"

// RedisCache 以 Redis 为后端。
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache 构造 Redis 后端；client 为 nil 时返回 nil（调用方据此降级）。
func NewRedisCache(client *redis.Client) *RedisCache {
	if client == nil {
		return nil
	}
	return &RedisCache{client: client}
}

func (c *RedisCache) redisKey(key string) string { return redisKeyPrefix + key }

// Get 实现 Cache。
func (c *RedisCache) Get(ctx context.Context, key string) (*Entry, bool, error) {
	data, err := c.client.Get(ctx, c.redisKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get: %w", err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false, fmt.Errorf("unmarshal entry: %w", err)
	}
	return &e, true, nil
}

// Put 实现 Cache。
func (c *RedisCache) Put(ctx context.Context, key string, e *Entry, ttl time.Duration) error {
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	if err := c.client.Set(ctx, c.redisKey(key), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// ========================================
// 内存后端（Redis 不可用时降级 / 单测用）
// ========================================

type memoryEntry struct {
	entry     *Entry
	expiresAt time.Time
}

// MemoryCache 是进程内后端，带 TTL 惰性过期。
type MemoryCache struct {
	mu   sync.Mutex
	data map[string]memoryEntry
}

// NewMemoryCache 构造内存后端。
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{data: make(map[string]memoryEntry)}
}

// Get 实现 Cache（惰性过期）。
func (c *MemoryCache) Get(ctx context.Context, key string) (*Entry, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	me, ok := c.data[key]
	if !ok {
		return nil, false, nil
	}
	if time.Now().After(me.expiresAt) {
		delete(c.data, key)
		return nil, false, nil
	}
	return me.entry, true, nil
}

// Put 实现 Cache。
func (c *MemoryCache) Put(ctx context.Context, key string, e *Entry, ttl time.Duration) error {
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = memoryEntry{entry: e, expiresAt: time.Now().Add(ttl)}
	return nil
}
