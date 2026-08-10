// Package queue 提供任务队列实现
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	domaintask "cognida/internal/model/task"
)

// ========================================
// RedisTaskQueue Redis任务队列实现
// ========================================

// RedisTaskQueue Redis 任务队列实现
type RedisTaskQueue struct {
	client *redis.Client
}

// NewRedisTaskQueue 创建 Redis 任务队列
func NewRedisTaskQueue(client *redis.Client) domaintask.TaskQueue {
	return &RedisTaskQueue{client: client}
}

// getQueueKey 获取队列key
func (q *RedisTaskQueue) getQueueKey(taskType string) string {
	if taskType == "" {
		return "task:queue:default"
	}
	return fmt.Sprintf("task:queue:%s", taskType)
}

// Enqueue 将任务加入队列
func (q *RedisTaskQueue) Enqueue(ctx context.Context, task *domaintask.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task failed: %w", err)
	}

	key := q.getQueueKey(task.Type)
	return q.client.LPush(ctx, key, data).Err()
}

// Dequeue 从队列取出任务（阻塞）
func (q *RedisTaskQueue) Dequeue(ctx context.Context, timeout int) (*domaintask.Task, error) {
	// 从多个队列中按优先级获取
	queues := []string{
		"task:queue:evaluation",
		"task:queue:document_parse",
		"task:queue:kb_index",
		"task:queue:ml_inference",
		"task:queue:pipeline_run",
		"task:queue:agent_subtask",
		"task:queue:default",
	}

	// BRPOP 阻塞式弹出
	duration := time.Duration(timeout) * time.Second
	if timeout <= 0 {
		duration = 30 * time.Second // 默认30秒
	}

	result, err := q.client.BRPop(ctx, duration, queues...).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 超时
		}
		return nil, fmt.Errorf("dequeue failed: %w", err)
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("invalid dequeue result")
	}

	var task domaintask.Task
	if err := json.Unmarshal([]byte(result[1]), &task); err != nil {
		return nil, fmt.Errorf("unmarshal task failed: %w", err)
	}

	return &task, nil
}

// EnqueueDelay 延迟加入队列
func (q *RedisTaskQueue) EnqueueDelay(ctx context.Context, task *domaintask.Task, delaySeconds int) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task failed: %w", err)
	}

	// 使用 ZSet 实现延迟队列
	executeAt := float64(time.Now().Unix()) + float64(delaySeconds)
	return q.client.ZAdd(ctx, "task:delayed", redis.Z{
		Score:  executeAt,
		Member: data,
	}).Err()
}

// Complete 标记任务完成
func (q *RedisTaskQueue) Complete(ctx context.Context, taskID string) error {
	// 任务完成由 TaskRepository 处理
	// 这里只负责从队列中移除（如果需要）
	return nil
}

// Fail 标记任务失败
func (q *RedisTaskQueue) Fail(ctx context.Context, taskID string, errMsg string) error {
	// 任务失败由 TaskRepository 处理
	// 这里可以放入重试队列
	return nil
}

// ========================================
// 辅助方法
// ========================================

// claimDelayedScript 原子领取到期延迟任务：
// 在一次 Lua 调用内 ZRANGEBYSCORE 取出 score ≤ now 的成员并逐个 ZREM，返回被本次成功移除的原始成员。
// 这样保证——① 每个成员只会被一个 worker 领取（读取与移除原子，杜绝多 worker 双执行）；
// ② 移除用的是 ZSet 中的原始 JSON 字节，而非重新 Marshal 的结果（旧实现重新 Marshal 可能因字段序/空白不一致导致 ZRem 失配、任务残留被反复执行）。
const claimDelayedScript = `
local due = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, ARGV[2])
for i = 1, #due do
	redis.call('ZREM', KEYS[1], due[i])
end
return due
`

// GetDelayedTasks 获取并领取到期的延迟任务（原子：取出即从队列移除）
func (q *RedisTaskQueue) GetDelayedTasks(ctx context.Context, limit int) ([]*domaintask.Task, error) {
	now := time.Now().Unix()

	raw, err := q.client.Eval(ctx, claimDelayedScript, []string{"task:delayed"},
		fmt.Sprintf("%d", now), limit).Result()
	if err != nil {
		return nil, err
	}

	members, ok := raw.([]interface{})
	if !ok {
		return nil, nil
	}

	tasks := make([]*domaintask.Task, 0, len(members))
	for _, m := range members {
		data, ok := m.(string)
		if !ok {
			continue
		}
		var task domaintask.Task
		if err := json.Unmarshal([]byte(data), &task); err != nil {
			// 成员已被原子领取移除；反序列化失败则只能丢弃，无法回补。
			continue
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// GetQueueSize 获取队列大小
func (q *RedisTaskQueue) GetQueueSize(ctx context.Context, taskType string) (int64, error) {
	key := q.getQueueKey(taskType)
	return q.client.LLen(ctx, key).Result()
}

// ClearQueue 清空队列
func (q *RedisTaskQueue) ClearQueue(ctx context.Context, taskType string) error {
	key := q.getQueueKey(taskType)
	return q.client.Del(ctx, key).Err()
}
