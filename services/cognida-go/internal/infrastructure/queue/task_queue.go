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

// GetDelayedTasks 获取到期的延迟任务
func (q *RedisTaskQueue) GetDelayedTasks(ctx context.Context, limit int) ([]*domaintask.Task, error) {
	now := float64(time.Now().Unix())

	// 获取已到期的任务
	// ZRangeArgs{ByScore:true} 替代已弃用的 ZRangeByScore（Redis 6.2+）：取到期任务（score ≤ now）。
	results, err := q.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     "task:delayed",
		ByScore: true,
		Start:   "0",
		Stop:    fmt.Sprintf("%f", now),
		Offset:  0,
		Count:   int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}

	tasks := make([]*domaintask.Task, 0, len(results))
	for _, data := range results {
		var task domaintask.Task
		if err := json.Unmarshal([]byte(data), &task); err != nil {
			continue
		}
		tasks = append(tasks, &task)
	}

	// 从延迟队列中移除
	if len(tasks) > 0 {
		for _, task := range tasks {
			data, _ := json.Marshal(task)
			q.client.ZRem(ctx, "task:delayed", data)
		}
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
