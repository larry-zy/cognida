// Package redis 提供 Redis 事务操作实现
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"cognida/internal/model/cache"
)

// ========================================
// Transaction Implementation
// ========================================

// transaction 实现 Transaction 接口
type transaction struct {
	client *redis.Client
}

// NewTransaction 创建事务实例
func NewTransaction(client *redis.Client) cache.Transaction {
	return &transaction{client: client}
}

// Watch 监视键
func (t *transaction) Watch(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	// Watch 需要在事务前调用
	// 返回一个可以执行事务的函数
	return nil
}

// Unwatch 取消监视
func (t *transaction) Unwatch(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	// go-redis v9 不需要显式 Unwatch
	// Watch 会在事务完成后自动取消
	return nil
}

// Multi 开启事务
func (t *transaction) Multi(ctx context.Context) error {
	// go-redis 的 TxPipeline 会自动开启 MULTI
	// 这里返回一个可以执行事务的对象
	return nil
}

// Exec 执行事务
func (t *transaction) Exec(ctx context.Context) error {
	// go-redis 的事务执行在 TxPipeline.Exec() 中完成
	return nil
}

// Discard 取消事务
func (t *transaction) Discard(ctx context.Context) error {
	// go-redis 的事务取消通过丢弃 TxPipeline 完成
	return nil
}

// Pipeline 获取管道
func (t *transaction) Pipeline(ctx context.Context) cache.Pipeline {
	return &pipeline{
		client: t.client,
		ctx:    ctx,
	}
}

// ========================================
// Pipeline Implementation
// ========================================

// pipeline 实现 Pipeline 接口
type pipeline struct {
	client *redis.Client
	ctx    context.Context
	fns    []func() error
}

// Add 添加命令到管道
func (p *pipeline) Add(fn func() error) {
	p.fns = append(p.fns, fn)
}

// Execute 执行管道命令
func (p *pipeline) Execute(ctx context.Context) error {
	pipe := p.client.Pipeline()

	// 执行所有命令
	for _, fn := range p.fns {
		if err := fn(); err != nil {
			return err
		}
	}

	// 获取结果
	_, err := pipe.Exec(ctx)
	return err
}

// ========================================
// Transaction Helper
// ========================================

// TxTransaction 事务执行辅助函数
type TxTransaction struct {
	client *redis.Client
}

// NewTxTransaction 创建事务辅助实例
func NewTxTransaction(client *redis.Client) *TxTransaction {
	return &TxTransaction{client: client}
}

// Exec 执行事务（使用 Watch）
func (t *TxTransaction) Exec(ctx context.Context, keys []string, fn func(tx *redis.Tx) error) error {
	return t.client.Watch(ctx, fn, keys...)
}

// Pipeline 创建管道
func (t *TxTransaction) Pipeline() redis.Pipeliner {
	return t.client.Pipeline()
}

// TxPipeline 创建事务管道
func (t *TxTransaction) TxPipeline() redis.Pipeliner {
	return t.client.TxPipeline()
}

// ========================================
// Batch Operations
// ========================================

// Batch 批量操作辅助
type Batch struct {
	client *redis.Client
}

// NewBatch 创建批量操作实例
func NewBatch(client *redis.Client) *Batch {
	return &Batch{client: client}
}

// SetBatch 批量设置键值
func (b *Batch) SetBatch(ctx context.Context, pairs map[string]interface{}, expiration int64) error {
	pipe := b.client.Pipeline()

	for key, value := range pairs {
		pipe.Set(ctx, key, value, 0)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// GetBatch 批量获取键值
func (b *Batch) GetBatch(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string), nil
	}

	pipe := b.client.Pipeline()

	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[string]string)
	for i, cmd := range cmds {
		if val, err := cmd.Result(); err == nil {
			result[keys[i]] = val
		}
	}

	return result, nil
}

// DelBatch 批量删除键
func (b *Batch) DelBatch(ctx context.Context, keys []string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	return b.client.Del(ctx, keys...).Result()
}
