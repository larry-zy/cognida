package pendingaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "agent:pendingaction:"

// RedisStore 以 Redis 为后端的待确认操作存储（多实例可用）。
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

func (s *RedisStore) key(id string) string {
	return redisKeyPrefix + id
}

// Put 暂存待确认操作，TTL 由 Redis 过期承担。
func (s *RedisStore) Put(ctx context.Context, action *PendingAction, ttl time.Duration) (string, error) {
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

	data, err := json.Marshal(action)
	if err != nil {
		return "", fmt.Errorf("marshal pending action: %w", err)
	}
	if err := s.client.Set(ctx, s.key(action.ID), data, ttl).Err(); err != nil {
		return "", fmt.Errorf("redis set: %w", err)
	}
	return action.ID, nil
}

// consumeScript 原子取出并删除：
// 归属不符时不删除（视同不存在，不泄露存在性）；归属相符则无论 token
// 是否匹配都删除（token 校验在 Go 侧，不匹配即已消费失效，防暴力尝试）。
var consumeScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if not v then return nil end
local obj = cjson.decode(v)
if obj.owner ~= ARGV[1] then return '__OWNER_MISMATCH__' end
redis.call('DEL', KEYS[1])
return v
`)

// Consume 以 id + token 原子取出并删除。
func (s *RedisStore) Consume(ctx context.Context, owner, id, token string) (*PendingAction, error) {
	res, err := consumeScript.Run(ctx, s.client, []string{s.key(id)}, owner).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis consume: %w", err)
	}
	raw, ok := res.(string)
	if !ok || raw == "__OWNER_MISMATCH__" {
		return nil, ErrNotFound
	}
	var action PendingAction
	if err := json.Unmarshal([]byte(raw), &action); err != nil {
		return nil, fmt.Errorf("unmarshal pending action: %w", err)
	}
	if !tokenEqual(action.Token, token) {
		// 键已删除 = 已消费失效
		return nil, ErrTokenMismatch
	}
	return &action, nil
}
