// Package pendingaction 提供危险操作的人机确认暂存：
// 危险级操作不直接落库，先以 pending_action_id + token 暂存，
// 等待前端确认卡片经携带 token 的 resume 请求后再提交（Phase 5 端点）。
//
// 本包定义 Store 接口与进程内 MemoryStore；Redis 后端随 confirm-resume
// 端点在 Phase 5 落地（任务 6.1）。
package pendingaction

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// DefaultTTL 待确认操作的默认有效期：过期未确认即失效。
const DefaultTTL = 10 * time.Minute

var (
	// ErrNotFound pending action 不存在或已过期/已消费。
	ErrNotFound = errors.New("pendingaction: not found or expired")
	// ErrTokenMismatch resume 携带的 token 不匹配；该 pending action 同时失效。
	ErrTokenMismatch = errors.New("pendingaction: token mismatch")
)

// PendingAction 一个等待人机确认的危险操作。
type PendingAction struct {
	ID        string                 `json:"id"`
	Token     string                 `json:"token"`  // resume 必须携带的一次性凭证
	Owner     string                 `json:"owner"`  // tenant:session（同 resultstore.OwnerKey）
	Kind      string                 `json:"kind"`   // mutate / etl …
	SQL       string                 `json:"sql"`    // 待提交的 SQL
	Params    map[string]interface{} `json:"params"` // 其余参数（幂等键、目标表、影响行数等）
	CreatedAt int64                  `json:"created_at"`
	ExpiresAt int64                  `json:"expires_at"`
}

// Store 待确认操作存储接口。
type Store interface {
	// Put 暂存一个待确认操作，返回 pending_action_id。
	Put(ctx context.Context, action *PendingAction, ttl time.Duration) (string, error)
	// Consume 以 id + token 原子取出并删除（确认即消费，仅一次）。
	// token 不匹配返回 ErrTokenMismatch 且该 action 立即失效；
	// 不存在/已过期返回 ErrNotFound。归属不符视同不存在。
	Consume(ctx context.Context, owner, id, token string) (*PendingAction, error)
}

// NewID 生成 pending_action_id。
func NewID() string {
	return "pa_" + randomHex(16)
}

// NewToken 生成一次性确认 token。
func NewToken() string {
	return randomHex(24)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败属系统级异常；退化为时间熵仅保证可用性
		return hex.EncodeToString([]byte(time.Now().String()))[:n*2]
	}
	return hex.EncodeToString(b)
}

// nowUnix 便于测试注入时间。
var nowUnix = func() int64 { return time.Now().Unix() }

// ========================================
// MemoryStore 进程内实现
// ========================================

// MemoryStore 进程内待确认操作存储（单实例可用；多实例需 Redis 后端）。
type MemoryStore struct {
	mu      sync.Mutex
	actions map[string]*PendingAction
}

// NewMemoryStore 创建进程内存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{actions: make(map[string]*PendingAction)}
}

// Put 暂存待确认操作。
func (s *MemoryStore) Put(ctx context.Context, action *PendingAction, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if action.ID == "" {
		action.ID = NewID()
	}
	if action.Token == "" {
		action.Token = NewToken()
	}
	action.CreatedAt = nowUnix()
	action.ExpiresAt = action.CreatedAt + int64(ttl/time.Second)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions[action.ID] = action
	return action.ID, nil
}

// Consume 以 id + token 原子取出并删除。
func (s *MemoryStore) Consume(ctx context.Context, owner, id, token string) (*PendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	action, ok := s.actions[id]
	if !ok || action.ExpiresAt < nowUnix() {
		delete(s.actions, id)
		return nil, ErrNotFound
	}
	if action.Owner != owner {
		// 归属不符视同不存在，不泄露他人 pending action 的存在性
		return nil, ErrNotFound
	}
	// 无论 token 是否匹配都消费：不匹配即失效，防止暴力尝试
	delete(s.actions, id)
	if !tokenEqual(action.Token, token) {
		return nil, ErrTokenMismatch
	}
	return action, nil
}

// tokenEqual 常量时间比较确认 token（防计时侧信道；消费即失效已限单次尝试，此为纵深）。
func tokenEqual(expected, got string) bool {
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}
