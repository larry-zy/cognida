// Package redis 提供 Memory 模块的 Redis 缓存实现
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"link/internal/model/memory"
)

// ========================================
// Redis 键前缀常量
// ========================================

const (
	// SessionKeyPrefix 会话消息缓存键前缀
	SessionKeyPrefix = "session:msg:"

	// SessionListKey 会话消息列表键
	SessionListKey = "session:msg:list:"

	// SessionMetaKey 会话元数据键
	SessionMetaKey = "session:meta:"

	// CacheKeyPattern 缓存键模式
	CacheKeyPattern = "%s%s"
)

// ========================================
// RedisMemoryStore Redis 内存存储
// ========================================

// RedisMemoryStore Redis 内存存储实现
type RedisMemoryStore struct {
	client *redis.Client
	ttl    time.Duration // 默认 TTL
}

// NewRedisMemoryStore 创建 Redis 内存存储
func NewRedisMemoryStore(client *redis.Client, defaultTTL time.Duration) *RedisMemoryStore {
	if defaultTTL == 0 {
		defaultTTL = 24 * time.Hour // 默认 24 小时
	}
	return &RedisMemoryStore{
		client: client,
		ttl:    defaultTTL,
	}
}

// ========================================
// 消息缓存操作
// ========================================

// SaveMessage 保存单条消息到缓存
func (s *RedisMemoryStore) SaveMessage(ctx context.Context, msg *memory.Message) error {
	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 存储消息
	key := s.buildMessageKey(msg.SessionID, msg.ID)
	pipe := s.client.Pipeline()

	// 设置消息（带 TTL）
	pipe.Set(ctx, key, data, s.ttl)

	// 添加到列表（用于按顺序获取）
	listKey := s.buildListKey(msg.SessionID)
	pipe.ZAdd(ctx, listKey, redis.Z{
		Score:  float64(msg.CreatedAt.UnixMilli()),
		Member: msg.ID,
	})
	pipe.Expire(ctx, listKey, s.ttl)

	// 更新元数据
	metaKey := s.buildMetaKey(msg.SessionID)
	pipe.HIncrBy(ctx, metaKey, "count", 1)
	pipe.HIncrBy(ctx, metaKey, "tokens", int64(msg.Tokens))
	pipe.Expire(ctx, metaKey, s.ttl)

	// 执行管道
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save message to redis: %w", err)
	}

	return nil
}

// SaveMessageBatch 批量保存消息
func (s *RedisMemoryStore) SaveMessageBatch(ctx context.Context, msgs []*memory.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()
	sessionID := msgs[0].SessionID

	// 批量保存消息
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		key := s.buildMessageKey(msg.SessionID, msg.ID)
		pipe.Set(ctx, key, data, s.ttl)

		listKey := s.buildListKey(msg.SessionID)
		pipe.ZAdd(ctx, listKey, redis.Z{
			Score:  float64(msg.CreatedAt.UnixMilli()),
			Member: msg.ID,
		})
	}

	// 设置列表过期时间
	listKey := s.buildListKey(sessionID)
	pipe.Expire(ctx, listKey, s.ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save message batch to redis: %w", err)
	}

	return nil
}

// LoadHistory 加载会话历史消息
func (s *RedisMemoryStore) LoadHistory(ctx context.Context, sessionID string, limit int) ([]*memory.Message, error) {
	listKey := s.buildListKey(sessionID)

	// 从有序集合获取消息 ID（按时间顺序）
	var messageIDs []string
	var err error

	if limit > 0 {
		messageIDs, err = s.client.ZRevRange(ctx, listKey, 0, int64(limit-1)).Result()
	} else {
		messageIDs, err = s.client.ZRevRange(ctx, listKey, 0, -1).Result()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get message ids: %w", err)
	}

	if len(messageIDs) == 0 {
		return []*memory.Message{}, nil
	}

	// 批量获取消息内容
	keys := make([]string, len(messageIDs))
	for i, id := range messageIDs {
		keys[i] = s.buildMessageKey(sessionID, id)
	}

	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// 反序列化消息
	messages := make([]*memory.Message, 0, len(messageIDs))
	for _, value := range values {
		if value == nil {
			continue
		}
		data, ok := value.(string)
		if !ok {
			continue
		}

		var msg memory.Message
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		messages = append(messages, &msg)
	}

	// 反转顺序（因为我们用了 ZRevRange）
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// GetMessage 获取单条消息
func (s *RedisMemoryStore) GetMessage(ctx context.Context, sessionID, messageID string) (*memory.Message, error) {
	key := s.buildMessageKey(sessionID, messageID)

	data, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, memory.ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	var msg memory.Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// DeleteMessage 删除消息
func (s *RedisMemoryStore) DeleteMessage(ctx context.Context, sessionID, messageID string) error {
	pipe := s.client.Pipeline()

	// 删除消息
	key := s.buildMessageKey(sessionID, messageID)
	pipe.Del(ctx, key)

	// 从列表移除
	listKey := s.buildListKey(sessionID)
	pipe.ZRem(ctx, listKey, messageID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

// ClearSession 清空会话
func (s *RedisMemoryStore) ClearSession(ctx context.Context, sessionID string) error {
	pipe := s.client.Pipeline()

	// 删除列表
	listKey := s.buildListKey(sessionID)
	pipe.Del(ctx, listKey)

	// 删除元数据
	metaKey := s.buildMetaKey(sessionID)
	pipe.Del(ctx, metaKey)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to clear session: %w", err)
	}

	return nil
}

// ========================================
// 会话元数据操作
// ========================================

// GetSessionMeta 获取会话元数据
func (s *RedisMemoryStore) GetSessionMeta(ctx context.Context, sessionID string) (*SessionMeta, error) {
	metaKey := s.buildMetaKey(sessionID)

	data, err := s.client.HGetAll(ctx, metaKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get session meta: %w", err)
	}

	if len(data) == 0 {
		return &SessionMeta{
			SessionID: sessionID,
			Count:     0,
			Tokens:    0,
		}, nil
	}

	meta := &SessionMeta{
		SessionID: sessionID,
	}

	// 解析字段
	if count, ok := data["count"]; ok {
		fmt.Sscanf(count, "%d", &meta.Count)
	}
	if tokens, ok := data["tokens"]; ok {
		fmt.Sscanf(tokens, "%d", &meta.Tokens)
	}

	return meta, nil
}

// UpdateSessionMeta 更新会话元数据
func (s *RedisMemoryStore) UpdateSessionMeta(ctx context.Context, sessionID string, updates map[string]interface{}) error {
	metaKey := s.buildMetaKey(sessionID)

	// 转换为 map[string]interface{}
	args := make([]interface{}, 0, len(updates)*2)
	for k, v := range updates {
		args = append(args, k, v)
	}

	if len(args) == 0 {
		return nil
	}

	err := s.client.HSet(ctx, metaKey, args...).Err()
	if err != nil {
		return fmt.Errorf("failed to update session meta: %w", err)
	}

	// 设置过期时间
	s.client.Expire(ctx, metaKey, s.ttl)

	return nil
}

// ========================================
// 缓存辅助方法
// ========================================

// buildMessageKey 构建消息缓存键
func (s *RedisMemoryStore) buildMessageKey(sessionID, messageID string) string {
	return fmt.Sprintf(CacheKeyPattern, SessionKeyPrefix, sessionID+":"+messageID)
}

// buildListKey 构建列表键
func (s *RedisMemoryStore) buildListKey(sessionID string) string {
	return fmt.Sprintf(CacheKeyPattern, SessionListKey, sessionID)
}

// buildMetaKey 构建元数据键
func (s *RedisMemoryStore) buildMetaKey(sessionID string) string {
	return fmt.Sprintf(CacheKeyPattern, SessionMetaKey, sessionID)
}

// ========================================
// SessionMeta 会话元数据
// ========================================

// SessionMeta 会话元数据
type SessionMeta struct {
	SessionID string
	Count     int    // 消息数量
	Tokens    int    // 总 Token 数
	CreatedAt string // 创建时间
	UpdatedAt string // 更新时间
}

// ========================================
// 缓存统计
// ========================================

// GetCacheStats 获取缓存统计信息
func (s *RedisMemoryStore) GetCacheStats(ctx context.Context, sessionID string) (*CacheStats, error) {
	listKey := s.buildListKey(sessionID)

	count, err := s.client.ZCard(ctx, listKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	ttl, err := s.client.TTL(ctx, listKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get ttl: %w", err)
	}

	return &CacheStats{
		SessionID: sessionID,
		MessageCount: int(count),
		TTL:       ttl,
	}, nil
}

// CacheStats 缓存统计
type CacheStats struct {
	SessionID   string
	MessageCount int
	TTL         time.Duration
}

// ========================================
// 批量操作
// ========================================

// WarmUpCache 预热缓存（从 MySQL 加载到 Redis）
func (s *RedisMemoryStore) WarmUpCache(ctx context.Context, sessionID string, messages []*memory.Message) error {
	if len(messages) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()

	// 批量保存消息
	totalTokens := 0
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		key := s.buildMessageKey(msg.SessionID, msg.ID)
		pipe.Set(ctx, key, data, s.ttl)

		listKey := s.buildListKey(msg.SessionID)
		pipe.ZAdd(ctx, listKey, redis.Z{
			Score:  float64(msg.CreatedAt.UnixMilli()),
			Member: msg.ID,
		})

		totalTokens += msg.Tokens
	}

	// 设置列表过期时间
	listKey := s.buildListKey(sessionID)
	pipe.Expire(ctx, listKey, s.ttl)

	// 设置元数据
	metaKey := s.buildMetaKey(sessionID)
	pipe.HSet(ctx, metaKey, "count", len(messages))
	pipe.HSet(ctx, metaKey, "tokens", totalTokens)
	pipe.Expire(ctx, metaKey, s.ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to warm up cache: %w", err)
	}

	return nil
}

// InvalidateCache 使缓存失效
func (s *RedisMemoryStore) InvalidateCache(ctx context.Context, sessionID string) error {
	listKey := s.buildListKey(sessionID)
	metaKey := s.buildMetaKey(sessionID)

	// 获取所有消息 ID
	messageIDs, err := s.client.ZRange(ctx, listKey, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get message ids: %w", err)
	}

	pipe := s.client.Pipeline()

	// 删除所有消息
	for _, id := range messageIDs {
		key := s.buildMessageKey(sessionID, id)
		pipe.Del(ctx, key)
	}

	// 删除列表和元数据
	pipe.Del(ctx, listKey)
	pipe.Del(ctx, metaKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to invalidate cache: %w", err)
	}

	return nil
}

// ========================================
// 健康检查
// ========================================

// Ping 检查 Redis 连接
func (s *RedisMemoryStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// ========================================
// 通用键值操作
// ========================================

// Set 设置键值（带 TTL）
func (s *RedisMemoryStore) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return s.client.Set(ctx, key, value, expiration).Err()
}

// Get 获取键值
func (s *RedisMemoryStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

// Del 删除键
func (s *RedisMemoryStore) Del(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

// Exists 检查键是否存在
func (s *RedisMemoryStore) Exists(ctx context.Context, keys ...string) (int64, error) {
	return s.client.Exists(ctx, keys...).Result()
}

// Close 关闭连接
func (s *RedisMemoryStore) Close() error {
	return s.client.Close()
}
