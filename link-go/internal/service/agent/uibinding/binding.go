// Package uibinding 维护交互式 UI surface 的绑定状态（Phase 3 任务 4.8）：
// surface ↔ result_id / pending_action_id + token，存 Redis 并施加会话 TTL，
// 支撑前端组件（Filter/Pagination/Confirm）回调路由到正确的数据与暂停动作。
//
// 与 Result Store 的分工：Result Store 存"数据"，本包只存"路由绑定"。
// 绑定超 TTL 后回调返回"会话已过期"，而非错误路由到别人的数据。
package uibinding

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

// DefaultTTL 绑定状态的会话级存活时长（超时回调返回"会话已过期"）。
const DefaultTTL = 24 * time.Hour

// ErrNotFound surface 绑定不存在或已超会话 TTL。
var ErrNotFound = errors.New("ui binding not found or session expired")

// Binding 是一个 UI surface 的回调路由状态。
type Binding struct {
	Surface         string `json:"surface"`                     // surface 标识（render_ui 每次调用独立）
	TenantID        int64  `json:"tenant_id"`                   // 归属租户
	SessionID       string `json:"session_id"`                  // 归属会话
	ResultID        string `json:"result_id,omitempty"`         // 绑定的数据引用（分页/过滤按此取数）
	PendingActionID string `json:"pending_action_id,omitempty"` // 暂停中的危险操作（Phase 4/5 Confirm 用）
	Token           string `json:"token"`                       // 回调防伪 token（随 UISpec 下发前端）
	CreatedAt       int64  `json:"created_at"`
}

// Store 是绑定状态的读写接口。
type Store interface {
	// Put 写入绑定并施加 TTL。
	Put(ctx context.Context, b *Binding, ttl time.Duration) error
	// Get 按 surface 取回绑定；不存在或超 TTL 返回 ErrNotFound。
	Get(ctx context.Context, surface string) (*Binding, error)
}

// NewToken 生成回调防伪 token。
func NewToken() string {
	return "tk_" + uuid.NewString()
}

// 注：绑定存储不再有包级单例。组合根构造后经 tools.ToolDeps.UIBinding 显式注入，
// render_ui 与 handler 回调路由均从注入实例读取（架构加固 Phase 6：去全局化 + AgentState 门面收口）。

// ========================================
// Redis 后端
// ========================================

const redisKeyPrefix = "agent:uibinding:"

// RedisStore 以 Redis 为后端，键为 surface，带会话 TTL。
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

func (s *RedisStore) key(surface string) string {
	return redisKeyPrefix + surface
}

// Put 实现 Store。
func (s *RedisStore) Put(ctx context.Context, b *Binding, ttl time.Duration) error {
	if b.Surface == "" {
		return fmt.Errorf("binding surface 不能为空")
	}
	if b.CreatedAt == 0 {
		b.CreatedAt = nowUnix()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	data, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("marshal binding: %w", err)
	}
	if err := s.client.Set(ctx, s.key(b.Surface), data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Get 实现 Store。
func (s *RedisStore) Get(ctx context.Context, surface string) (*Binding, error) {
	data, err := s.client.Get(ctx, s.key(surface)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var b Binding
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("unmarshal binding: %w", err)
	}
	return &b, nil
}

// ========================================
// 内存后端（Redis 不可用时降级 / 单测用）
// ========================================

type memoryEntry struct {
	binding   *Binding
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
func (s *MemoryStore) Put(ctx context.Context, b *Binding, ttl time.Duration) error {
	if b.Surface == "" {
		return fmt.Errorf("binding surface 不能为空")
	}
	if b.CreatedAt == 0 {
		b.CreatedAt = nowUnix()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[b.Surface] = memoryEntry{
		binding:   b,
		expiresAt: time.Unix(b.CreatedAt, 0).Add(ttl),
	}
	return nil
}

// Get 实现 Store，含惰性过期。
func (s *MemoryStore) Get(ctx context.Context, surface string) (*Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[surface]
	if !ok {
		return nil, ErrNotFound
	}
	if time.Unix(nowUnix(), 0).After(entry.expiresAt) {
		delete(s.data, surface)
		return nil, ErrNotFound
	}
	return entry.binding, nil
}

// nowUnix 便于测试替换（生产用真实时间）。
var nowUnix = func() int64 { return time.Now().Unix() }
