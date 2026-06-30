// Package evaluation 定义评测领域的实体、接口与端口
package evaluation

import "context"

// TaskEnqueuer 评测任务入队端口（消费端接口）
// Service 经此接口依赖评测队列，避免直接耦合 infrastructure 实现。
type TaskEnqueuer interface {
	Enqueue(ctx context.Context, taskID string) error
}

// TaskQueue 评测任务队列端口（消费端接口）
// Worker 经此接口依赖队列的入队/出队与并发槽位控制，避免直接耦合 infrastructure 实现。
type TaskQueue interface {
	TaskEnqueuer
	// Dequeue 从队列取出任务（阻塞）
	Dequeue(ctx context.Context) (string, error)
	// AcquireSlot 获取并发槽位
	AcquireSlot(ctx context.Context) (bool, error)
	// ReleaseSlot 释放并发槽位
	ReleaseSlot(ctx context.Context) error
}
