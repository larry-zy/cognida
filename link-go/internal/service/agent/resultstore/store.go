package resultstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DefaultTTL 结果集在后端的默认存活时长（超时自动回收）。
const DefaultTTL = 30 * time.Minute

var (
	// ErrNotFound result_id 不存在或已过期。
	ErrNotFound = errors.New("result not found or expired")
	// ErrUnauthorized 调用方与结果归属不一致（跨会话读取）。
	ErrUnauthorized = errors.New("result access denied: owner mismatch")
)

// Store 是 data-by-reference 的读写接口，供全部数据工具复用。
type Store interface {
	// Put 持久化完整结果集并返回其 result_id。
	Put(ctx context.Context, r *Result, ttl time.Duration) (string, error)
	// Get 按 result_id 取回完整结果集，并校验 owner 归属一致。
	Get(ctx context.Context, owner, resultID string) (*Result, error)
}

// NewResultID 生成 result_id（crypto 随机，无时间/顺序泄漏）。
func NewResultID() string {
	return "rs_" + uuid.NewString()
}

// ========================================
// Redis 后端
// ========================================

const redisKeyPrefix = "agent:resultstore:"

// RedisStore 以 Redis 为后端，键为 result_id，带 TTL 与归属校验。
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore 构造 Redis 后端；client 为 nil 时返回 nil（调用方据此降级）。
func NewRedisStore(client *redis.Client) *RedisStore {
	if client == nil {
		return nil
	}
	return &RedisStore{client: client}
}

func (s *RedisStore) key(resultID string) string {
	return redisKeyPrefix + resultID
}

// Put 实现 Store。
func (s *RedisStore) Put(ctx context.Context, r *Result, ttl time.Duration) (string, error) {
	if r.ResultID == "" {
		r.ResultID = NewResultID()
	}
	if r.CreatedAt == 0 {
		r.CreatedAt = nowUnix()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	data, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	if err := s.client.Set(ctx, s.key(r.ResultID), data, ttl).Err(); err != nil {
		return "", fmt.Errorf("redis set: %w", err)
	}
	return r.ResultID, nil
}

// Get 实现 Store，含 owner 归属校验。
func (s *RedisStore) Get(ctx context.Context, owner, resultID string) (*Result, error) {
	data, err := s.client.Get(ctx, s.key(resultID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}
	if r.Owner != owner {
		return nil, ErrUnauthorized
	}
	return &r, nil
}

// ========================================
// 内存后端（Redis 不可用时降级 / 单测用）
// ========================================

type memoryEntry struct {
	result    *Result
	expiresAt time.Time
}

// MemoryStore 是进程内后端，带 TTL 惰性过期；用于降级与测试。
type MemoryStore struct {
	mu   sync.Mutex
	data map[string]memoryEntry
}

// NewMemoryStore 构造内存后端。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]memoryEntry)}
}

// Put 实现 Store。
func (s *MemoryStore) Put(ctx context.Context, r *Result, ttl time.Duration) (string, error) {
	if r.ResultID == "" {
		r.ResultID = NewResultID()
	}
	if r.CreatedAt == 0 {
		r.CreatedAt = nowUnix()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[r.ResultID] = memoryEntry{
		result:    r,
		expiresAt: time.Unix(r.CreatedAt, 0).Add(ttl),
	}
	return r.ResultID, nil
}

// Get 实现 Store，含 owner 归属校验与惰性过期。
func (s *MemoryStore) Get(ctx context.Context, owner, resultID string) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[resultID]
	if !ok {
		return nil, ErrNotFound
	}
	if time.Unix(nowUnix(), 0).After(entry.expiresAt) {
		delete(s.data, resultID)
		return nil, ErrNotFound
	}
	if entry.result.Owner != owner {
		return nil, ErrUnauthorized
	}
	return entry.result, nil
}
