// Package cache 提供缓存领域的错误定义
package cache

import "errors"

var (
	// ErrCacheNotFound 缓存未找到
	ErrCacheNotFound = errors.New("cache not found")

	// ErrCacheKeyEmpty 缓存键为空
	ErrCacheKeyEmpty = errors.New("cache key is empty")

	// ErrCacheKeyNotFound 缓存键不存在
	ErrCacheKeyNotFound = errors.New("cache key not found")

	// ErrCacheTypeMismatch 缓存类型不匹配
	ErrCacheTypeMismatch = errors.New("cache type mismatch")

	// ErrCacheValueInvalid 缓存值无效
	ErrCacheValueInvalid = errors.New("cache value is invalid")

	// ErrCacheExpirationInvalid 缓存过期时间无效
	ErrCacheExpirationInvalid = errors.New("cache expiration is invalid")

	// ErrLockNotHeld 锁未被持有
	ErrLockNotHeld = errors.New("lock is not held")

	// ErrLockAcquisitionFailed 锁获取失败
	ErrLockAcquisitionFailed = errors.New("failed to acquire lock")

	// ErrTransactionFailed 事务执行失败
	ErrTransactionFailed = errors.New("transaction failed")

	// ErrConnectionFailed Redis 连接失败
	ErrConnectionFailed = errors.New("redis connection failed")

	// ErrDatabaseInvalid 数据库索引无效
	ErrDatabaseInvalid = errors.New("invalid database index")

	// ErrPatternInvalid 匹配模式无效
	ErrPatternInvalid = errors.New("invalid pattern")

	// ErrScoreInvalid 分数无效
	ErrScoreInvalid = errors.New("score is invalid")

	// ErrMemberNotExists 成员不存在
	ErrMemberNotExists = errors.New("member does not exist")

	// ErrChannelEmpty 频道名为空
	ErrChannelEmpty = errors.New("channel name is empty")
)
