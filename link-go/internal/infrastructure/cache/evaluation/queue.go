// Package cache 提供 Redis 评测队列实现
package cache

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	domeval "link/internal/model/evaluation"
)

// 编译期断言：*EvaluationQueue 满足领域端口 TaskEnqueuer / TaskQueue。
var (
	_ domeval.TaskEnqueuer = (*EvaluationQueue)(nil)
	_ domeval.TaskQueue    = (*EvaluationQueue)(nil)
)

const (
	// QueueKey 评测队列 key
	QueueKey = "eval:queue"
	// CountKey 并发计数 key
	CountKey = "eval:count"
	// DefaultLimit 默认并发限制
	DefaultLimit = 3
	// DefaultTimeout 默认出队超时
	DefaultTimeout = 30 * time.Second
)

// EvaluationQueue 评测队列
type EvaluationQueue struct {
	client *redis.Client
	limit  int
}

// NewQueue 创建评测队列
func NewQueue(client *redis.Client, limit int) *EvaluationQueue {
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &EvaluationQueue{
		client: client,
		limit:  limit,
	}
}

// Enqueue 将任务加入队列
func (q *EvaluationQueue) Enqueue(ctx context.Context, taskID string) error {
	log.Printf("[Queue] Enqueuing task %s to %s", taskID, QueueKey)
	err := q.client.LPush(ctx, QueueKey, taskID).Err()
	if err != nil {
		log.Printf("[Queue] Failed to enqueue task %s: %v", taskID, err)
		return err
	}
	// 验证队列长度
	size, _ := q.client.LLen(ctx, QueueKey).Result()
	log.Printf("[Queue] Task %s enqueued, queue size: %d", taskID, size)
	return nil
}

// Dequeue 从队列取出任务（阻塞）
func (q *EvaluationQueue) Dequeue(ctx context.Context) (string, error) {
	log.Printf("[Queue] Waiting for task from %s (timeout: %v)", QueueKey, DefaultTimeout)
	result, err := q.client.BRPop(ctx, DefaultTimeout, QueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			log.Printf("[Queue] Dequeue timeout (no tasks in queue)")
			return "", nil // 超时
		}
		log.Printf("[Queue] Dequeue error: %v", err)
		return "", fmt.Errorf("dequeue failed: %w", err)
	}

	if len(result) < 2 {
		log.Printf("[Queue] Invalid dequeue result: %v", result)
		return "", fmt.Errorf("invalid dequeue result")
	}

	taskID := result[1]
	log.Printf("[Queue] Dequeued task: %s", taskID)
	return taskID, nil
}

// acquireSlotScript Lua 脚本：原子性地获取槽位
// 返回值：0=失败(已满), 1=成功, -1=错误
const acquireSlotScript = `
	local key = KEYS[1]
	local limit = tonumber(ARGV[1])
	local count = tonumber(redis.call("INCR", key))
	if count > limit then
		redis.call("DECR", key)
		return 0
	end
	return 1
`

// AcquireSlot 获取并发槽位（原子操作）
func (q *EvaluationQueue) AcquireSlot(ctx context.Context) (bool, error) {
	result, err := q.client.Eval(ctx, acquireSlotScript, []string{CountKey}, q.limit).Result()
	if err != nil {
		log.Printf("[Queue] AcquireSlot failed: %v", err)
		return false, fmt.Errorf("acquire slot failed: %w", err)
	}

	success, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected result type: %T", result)
	}

	if success == 1 {
		count, _ := q.client.Get(ctx, CountKey).Int64()
		log.Printf("[Queue] Slot acquired, count=%d/%d", count, q.limit)
		return true, nil
	}

	return false, nil
}

// releaseSlotScript Lua 脚本：原子性地释放槽位，防止负数
const releaseSlotScript = `
	local key = KEYS[1]
	local count = tonumber(redis.call("GET", key))
	if count == nil or count <= 0 then
		redis.call("SET", key, 0)
		return 0
	end
	redis.call("DECR", key)
	return 1
`

// ReleaseSlot 释放并发槽位
func (q *EvaluationQueue) ReleaseSlot(ctx context.Context) error {
	_, err := q.client.Eval(ctx, releaseSlotScript, []string{CountKey}).Result()
	if err != nil {
		log.Printf("[Queue] ReleaseSlot error: %v", err)
		return err
	}
	return nil
}

// GetCount 获取当前并发数
func (q *EvaluationQueue) GetCount(ctx context.Context) (int64, error) {
	count, err := q.client.Get(ctx, CountKey).Int64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, fmt.Errorf("get count failed: %w", err)
	}
	return count, nil
}

// GetSize 获取队列大小
func (q *EvaluationQueue) GetSize(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, QueueKey).Result()
}

// PendingIDs 返回当前仍在队列中（尚未出队）的任务 ID 快照。
// 供 Worker 启动恢复去重使用——LRANGE 只读，不影响队列。
func (q *EvaluationQueue) PendingIDs(ctx context.Context) ([]string, error) {
	ids, err := q.client.LRange(ctx, QueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("list pending failed: %w", err)
	}
	return ids, nil
}

// ResetSlots 将并发槽位计数清零（删除 eval:count）。
// 仅在 Worker 启动、本进程尚无任务持槽时调用，回收上次进程被杀泄漏的计数。
func (q *EvaluationQueue) ResetSlots(ctx context.Context) error {
	return q.client.Del(ctx, CountKey).Err()
}

// Clear 清空队列
func (q *EvaluationQueue) Clear(ctx context.Context) error {
	return q.client.Del(ctx, QueueKey).Err()
}
