// Package cache 提供缓存领域的仓储接口定义
package cache

import (
	"context"
	"time"
)

// ========================================
// Cache 缓存接口
// ========================================

// Cache 缓存仓储接口
type Cache interface {
	// Get 获取缓存值
	Get(ctx context.Context, key string) (string, error)

	// Set 设置缓存值
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error

	// Del 删除缓存
	Del(ctx context.Context, keys ...string) error

	// Exists 检查键是否存在
	Exists(ctx context.Context, key string) (bool, error)

	// Expire 设置过期时间
	Expire(ctx context.Context, key string, expiration time.Duration) error

	// TTL 获取剩余过期时间
	TTL(ctx context.Context, key string) (time.Duration, error)

	// FlushDB 清空当前数据库
	FlushDB(ctx context.Context) error

	// Keys 获取匹配模式的所有键
	Keys(ctx context.Context, pattern string) ([]string, error)

	// MGet 批量获取多个键的值
	MGet(ctx context.Context, keys ...string) ([]interface{}, error)

	// MSet 批量设置多个键值对
	MSet(ctx context.Context, values map[string]interface{}) error

	// Incr 自增
	Incr(ctx context.Context, key string) (int64, error)

	// IncrBy 自增指定值
	IncrBy(ctx context.Context, key string, value int64) (int64, error)

	// Decr 自减
	Decr(ctx context.Context, key string) (int64, error)

	// DecrBy 自减指定值
	DecrBy(ctx context.Context, key string, value int64) (int64, error)

	// SetNX 设置键值（仅当键不存在时）
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)

	// GetSet 设置新值并返回旧值
	GetSet(ctx context.Context, key string, value interface{}) (string, error)

	// Append 追加值到键
	Append(ctx context.Context, key string, value string) (int64, error)

	// GetRange 获取键值的子字符串
	GetRange(ctx context.Context, key string, start, end int64) (string, error)

	// SetRange 设置键值的子字符串
	SetRange(ctx context.Context, key string, offset int64, value string) (int64, error)

	// StrLen 获取键值的长度
	StrLen(ctx context.Context, key string) (int64, error)
}

// ========================================
// HashCache 哈希缓存接口
// ========================================

// HashCache 哈希缓存仓储接口
type HashCache interface {
	// HGet 获取哈希字段值
	HGet(ctx context.Context, key, field string) (string, error)

	// HSet 设置哈希字段值
	HSet(ctx context.Context, key, field string, value interface{}) error

	// HMGet 批量获取哈希字段值
	HMGet(ctx context.Context, key string, fields ...string) ([]interface{}, error)

	// HMSet 批量设置哈希字段值
	HMSet(ctx context.Context, key string, values map[string]interface{}) error

	// HGetAll 获取哈希所有字段值
	HGetAll(ctx context.Context, key string) (map[string]string, error)

	// HDel 删除哈希字段
	HDel(ctx context.Context, key string, fields ...string) error

	// HExists 检查哈希字段是否存在
	HExists(ctx context.Context, key, field string) (bool, error)

	// HKeys 获取哈希所有字段名
	HKeys(ctx context.Context, key string) ([]string, error)

	// HLen 获取哈希字段数量
	HLen(ctx context.Context, key string) (int64, error)

	// HIncrBy 哈希字段值增加指定数值
	HIncrBy(ctx context.Context, key, field string, incr int64) (int64, error)

	// HIncrByFloat 哈希字段值增加浮点数
	HIncrByFloat(ctx context.Context, key, field string, incr float64) (float64, error)

	// HVals 获取哈希所有值
	HVals(ctx context.Context, key string) ([]string, error)

	// HScan 扫描哈希字段
	HScan(ctx context.Context, key string, cursor uint64, match string, count int64) ([]string, uint64, error)
}

// ========================================
// ListCache 列表缓存接口
// ========================================

// ListCache 列表缓存仓储接口
type ListCache interface {
	// LPush 从列表左侧插入值
	LPush(ctx context.Context, key string, values ...interface{}) error

	// RPush 从列表右侧插入值
	RPush(ctx context.Context, key string, values ...interface{}) error

	// LPop 从列表左侧弹出值
	LPop(ctx context.Context, key string) (string, error)

	// RPop 从列表右侧弹出值
	RPop(ctx context.Context, key string) (string, error)

	// LLen 获取列表长度
	LLen(ctx context.Context, key string) (int64, error)

	// LRange 获取列表指定范围元素
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)

	// LTrim 修剪列表，只保留指定范围
	LTrim(ctx context.Context, key string, start, stop int64) error

	// LIndex 获取列表指定索引元素
	LIndex(ctx context.Context, key string, index int64) (string, error)

	// LSet 设置列表指定索引元素
	LSet(ctx context.Context, key string, index int64, value interface{}) error
}

// ========================================
// SetCache 集合缓存接口
// ========================================

// SetCache 集合缓存仓储接口
type SetCache interface {
	// SAdd 向集合添加成员
	SAdd(ctx context.Context, key string, members ...interface{}) error

	// SRem 从集合移除成员
	SRem(ctx context.Context, key string, members ...interface{}) error

	// SMembers 获取集合所有成员
	SMembers(ctx context.Context, key string) ([]string, error)

	// SIsMember 检查成员是否在集合中
	SIsMember(ctx context.Context, key string, member interface{}) (bool, error)

	// SCard 获取集合成员数量
	SCard(ctx context.Context, key string) (int64, error)

	// SDiff 获取集合差集
	SDiff(ctx context.Context, keys ...string) ([]string, error)

	// SInter 获取集合交集
	SInter(ctx context.Context, keys ...string) ([]string, error)

	// SUnion 获取集合并集
	SUnion(ctx context.Context, keys ...string) ([]string, error)

	// SPop 随机弹出集合成员
	SPop(ctx context.Context, key string) (string, error)

	// SPopN 随机弹出多个集合成员
	SPopN(ctx context.Context, key string, count int64) ([]string, error)
}

// ========================================
// SortedSetCache 有序集合缓存接口
// ========================================

// SortedSetCache 有序集合缓存仓储接口
type SortedSetCache interface {
	// ZAdd 向有序集合添加成员
	ZAdd(ctx context.Context, key string, score float64, member interface{}) error

	// ZRem 从有序集合移除成员
	ZRem(ctx context.Context, key string, members ...interface{}) error

	// ZScore 获取成员分数
	ZScore(ctx context.Context, key string, member interface{}) (float64, error)

	// ZRange 按分数范围获取成员（升序）
	ZRange(ctx context.Context, key string, start, stop int64) ([]string, error)

	// ZRangeByScore 按分数范围获取成员
	ZRangeByScore(ctx context.Context, key string, min, max float64) ([]string, error)

	// ZRangeWithScores 按分数范围获取成员及分数（升序）
	ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]*ScoredMember, error)

	// ZRank 获取成员排名（升序）
	ZRank(ctx context.Context, key string, member interface{}) (int64, error)

	// ZRevRank 获取成员排名（降序）
	ZRevRank(ctx context.Context, key string, member interface{}) (int64, error)

	// ZCard 获取有序集合成员数量
	ZCard(ctx context.Context, key string) (int64, error)

	// ZCount 获取分数范围内的成员数量
	ZCount(ctx context.Context, key string, min, max float64) (int64, error)

	// ZIncrBy 增加成员分数
	ZIncrBy(ctx context.Context, key string, increment float64, member interface{}) (float64, error)
}

// ScoredMember 带分数的成员
type ScoredMember struct {
	Member string
	Score  float64
}

// ========================================
// PubSub 发布订阅接口
// ========================================

// PubSub 发布订阅仓储接口
type PubSub interface {
	// Publish 发布消息到频道
	Publish(ctx context.Context, channel string, message interface{}) error

	// Subscribe 订阅频道
	Subscribe(ctx context.Context, channels ...string) (<-chan *Message, error)

	// Unsubscribe 取消订阅频道
	Unsubscribe(ctx context.Context, channels ...string) error

	// PSubscribe 模式订阅
	PSubscribe(ctx context.Context, patterns ...string) (<-chan *Message, error)

	// PUnsubscribe 取消模式订阅
	PUnsubscribe(ctx context.Context, patterns ...string) error

	// PubSubChannels 获取活跃频道列表
	PubSubChannels(ctx context.Context, pattern string) ([]string, error)

	// PubSubNumSub 获取频道订阅者数量
	PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error)

	// PubSubNumPat 获取模式订阅数量
	PubSubNumPat(ctx context.Context) (int64, error)
}

// Message 发布订阅消息
type Message struct {
	Channel string
	Payload string
}

// ========================================
// Lock 分布式锁接口
// ========================================

// Lock 分布式锁仓储接口
type Lock interface {
	// TryLock 尝试获取锁
	TryLock(ctx context.Context, key string, expiration time.Duration) (bool, error)

	// Lock 获取锁（阻塞）
	Lock(ctx context.Context, key string, expiration time.Duration) error

	// Unlock 释放锁
	Unlock(ctx context.Context, key string) error

	// Extend 延长锁过期时间
	Extend(ctx context.Context, key string, expiration time.Duration) (bool, error)
}
