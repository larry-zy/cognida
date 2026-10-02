// Package queue contains compatibility adapters for the legacy task queue.
//
// Deprecated: new background work must depend on infrastructure/job directly.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"cognida/internal/infrastructure/job"
	domaintask "cognida/internal/model/task"
)

// ========================================
// RedisTaskQueue Redis任务队列实现
// ========================================

// RedisTaskQueue adapts TaskQueue to generic durable jobs. Task state remains
// a domain concern; this adapter owns no status transitions.
//
// Deprecated: inject job.Queue and a domain-specific Job handler in new code.
type RedisTaskQueue struct {
	client *redis.Client
	mu     sync.Mutex
	queues map[string]*job.RedisQueue
}

// NewRedisTaskQueue 创建 Redis 任务队列
func NewRedisTaskQueue(client *redis.Client) domaintask.TaskQueue {
	return &RedisTaskQueue{client: client, queues: make(map[string]*job.RedisQueue)}
}

func normalizedType(taskType string) string {
	if taskType == "" {
		return "default"
	}
	return taskType
}

func (q *RedisTaskQueue) queue(taskType string) *job.RedisQueue {
	taskType = normalizedType(taskType)
	q.mu.Lock()
	defer q.mu.Unlock()
	if existing := q.queues[taskType]; existing != nil {
		return existing
	}
	delivery := job.NewRedisQueueWithPrefix(q.client, "legacy:task:"+taskType)
	q.queues[taskType] = delivery
	return delivery
}

func asJob(task *domaintask.Task) (job.Job, error) {
	if task == nil {
		return job.Job{}, fmt.Errorf("task is required")
	}
	// TaskQueue historically permits duplicate delivery of the same Task.ID.
	return job.New(uuid.NewString(), "legacy.task."+normalizedType(task.Type), task)
}

// Enqueue 将任务加入队列
func (q *RedisTaskQueue) Enqueue(ctx context.Context, task *domaintask.Task) error {
	j, err := asJob(task)
	if err != nil {
		return err
	}
	return q.queue(task.Type).Enqueue(ctx, j)
}

var priority = []string{
	domaintask.TaskTypeEvaluation,
	domaintask.TaskTypeDocumentParse,
	domaintask.TaskTypeKBIndex,
	domaintask.TaskTypeMLInference,
	domaintask.TaskTypePipelineRun,
	domaintask.TaskTypeAgentSubtask,
	"default",
}

// Dequeue retains legacy priority. The delivery is acknowledged immediately
// because TaskQueue exposes no acknowledgement handle; consumers requiring
// at-least-once processing must use job.Dispatcher directly.
func (q *RedisTaskQueue) Dequeue(ctx context.Context, timeout int) (*domaintask.Task, error) {
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	if timeout <= 0 {
		deadline = time.Now().Add(30 * time.Second)
	}
	for {
		for _, taskType := range priority {
			delivery := q.queue(taskType)
			j, err := delivery.TryClaim(ctx)
			if err != nil {
				return nil, err
			}
			if j == nil {
				continue
			}
			return decodeAndAck(ctx, delivery, *j)
		}
		if !time.Now().Before(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func decodeAndAck(ctx context.Context, delivery *job.RedisQueue, j job.Job) (*domaintask.Task, error) {
	var task domaintask.Task
	if err := json.Unmarshal(j.Payload, &task); err != nil {
		_ = delivery.Ack(context.Background(), j)
		return nil, fmt.Errorf("decode task job: %w", err)
	}
	if err := delivery.Ack(ctx, j); err != nil {
		return nil, fmt.Errorf("ack task job: %w", err)
	}
	return &task, nil
}

// EnqueueDelay 延迟加入队列
func (q *RedisTaskQueue) EnqueueDelay(ctx context.Context, task *domaintask.Task, delaySeconds int) error {
	j, err := asJob(task)
	if err != nil {
		return err
	}
	return q.queue(task.Type).Schedule(ctx, j, time.Duration(delaySeconds)*time.Second)
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

// GetDelayedTasks is a deprecated inspection-and-claim helper. New code should
// let job.Dispatcher promote and dispatch delayed work asynchronously.
func (q *RedisTaskQueue) GetDelayedTasks(ctx context.Context, limit int) ([]*domaintask.Task, error) {
	if limit <= 0 {
		limit = 100
	}
	tasks := make([]*domaintask.Task, 0, limit)
	for _, taskType := range priority {
		delivery := q.queue(taskType)
		if _, err := delivery.PromoteDue(ctx, limit-len(tasks)); err != nil {
			return nil, err
		}
		for len(tasks) < limit {
			j, err := delivery.TryClaim(ctx)
			if err != nil {
				return nil, err
			}
			if j == nil {
				break
			}
			task, err := decodeAndAck(ctx, delivery, *j)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
		}
		if len(tasks) == limit {
			break
		}
	}
	return tasks, nil
}

// GetQueueSize 获取队列大小
func (q *RedisTaskQueue) GetQueueSize(ctx context.Context, taskType string) (int64, error) {
	return q.queue(taskType).ReadySize(ctx)
}

// ClearQueue 清空队列
func (q *RedisTaskQueue) ClearQueue(ctx context.Context, taskType string) error {
	return q.queue(taskType).Purge(ctx)
}
