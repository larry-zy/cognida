// Package cache 提供 Redis 评测进度缓存实现
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	domeval "cognida/internal/model/evaluation"
)

const (
	// ProgressKeyPrefix 进度缓存 key 前缀
	ProgressKeyPrefix = "eval:progress:"
	// ProgressExpiry 进度缓存过期时间
	ProgressExpiry = 1 * time.Hour
)

// 进度类型已下沉至 model/evaluation（领域端口）。此处保留别名，
// 使 infrastructure 内部引用与外部依赖均指向同一领域类型。
type (
	// ProgressStage 评测进度阶段
	ProgressStage = domeval.ProgressStage
	// Progress 评测进度
	Progress = domeval.Progress
)

const (
	StageQueued     = domeval.StageQueued
	StageLoading    = domeval.StageLoading
	StageGeneration = domeval.StageGeneration
	StageEvaluation = domeval.StageEvaluation
	StageCompleted  = domeval.StageCompleted
	StageFailed     = domeval.StageFailed
)

// 编译期断言：*ProgressCache 满足领域端口 ProgressReader / ProgressWriter。
var (
	_ domeval.ProgressReader = (*ProgressCache)(nil)
	_ domeval.ProgressWriter = (*ProgressCache)(nil)
)

// ProgressCache 进度缓存
type ProgressCache struct {
	client *redis.Client
}

// NewProgressCache 创建进度缓存
func NewProgressCache(client *redis.Client) *ProgressCache {
	return &ProgressCache{
		client: client,
	}
}

// getProgressKey 获取进度 key
func (c *ProgressCache) getProgressKey(taskID string) string {
	return ProgressKeyPrefix + taskID
}

// SetProgress 设置进度
func (c *ProgressCache) SetProgress(ctx context.Context, taskID string, progress *Progress) error {
	key := c.getProgressKey(taskID)

	// 计算百分比
	if progress.Total > 0 {
		progress.Percentage = int(float64(progress.Current) / float64(progress.Total) * 100)
	}

	// 使用 HSET 存储字段
	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key,
		"stage", string(progress.Stage), // Convert ProgressStage to string
		"current", progress.Current,
		"total", progress.Total,
		"message", progress.Message,
		"percentage", progress.Percentage,
		"retry_count", progress.RetryCount,
	)
	if progress.Error != "" {
		pipe.HSet(ctx, key, "error", progress.Error)
	} else {
		pipe.HDel(ctx, key, "error")
	}
	pipe.Expire(ctx, key, ProgressExpiry)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set progress failed: %w", err)
	}

	return nil
}

// GetProgress 获取进度
func (c *ProgressCache) GetProgress(ctx context.Context, taskID string) (*Progress, error) {
	key := c.getProgressKey(taskID)

	data, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get progress failed: %w", err)
	}

	if len(data) == 0 {
		return nil, nil // 不存在
	}

	progress := &Progress{
		Stage:   ProgressStage(data["stage"]),
		Message: data["message"],
		Error:   data["error"],
	}

	// 解析数字字段
	if current, ok := data["current"]; ok && current != "" {
		fmt.Sscanf(current, "%d", &progress.Current)
	}
	if total, ok := data["total"]; ok && total != "" {
		fmt.Sscanf(total, "%d", &progress.Total)
	}
	if percentage, ok := data["percentage"]; ok && percentage != "" {
		fmt.Sscanf(percentage, "%d", &progress.Percentage)
	}
	if retryCount, ok := data["retry_count"]; ok && retryCount != "" {
		fmt.Sscanf(retryCount, "%d", &progress.RetryCount)
	}

	return progress, nil
}

// UpdateStage 更新阶段
func (c *ProgressCache) UpdateStage(ctx context.Context, taskID string, stage ProgressStage, message string) error {
	progress, err := c.GetProgress(ctx, taskID)
	if err != nil {
		return err
	}

	if progress == nil {
		progress = &Progress{
			Current: 0,
			Total:   100,
		}
	}

	progress.Stage = stage
	progress.Message = message

	return c.SetProgress(ctx, taskID, progress)
}

// UpdateProgress 更新进度
func (c *ProgressCache) UpdateProgress(ctx context.Context, taskID string, current, total int, message string) error {
	progress, err := c.GetProgress(ctx, taskID)
	if err != nil {
		return err
	}

	if progress == nil {
		progress = &Progress{}
	}

	progress.Current = current
	progress.Total = total
	if message != "" {
		progress.Message = message
	}

	return c.SetProgress(ctx, taskID, progress)
}

// Increment 增加进度
func (c *ProgressCache) Increment(ctx context.Context, taskID string, message string) error {
	progress, err := c.GetProgress(ctx, taskID)
	if err != nil {
		return err
	}

	if progress == nil {
		return fmt.Errorf("progress not found for task: %s", taskID)
	}

	progress.Current++
	if message != "" {
		progress.Message = message
	}

	return c.SetProgress(ctx, taskID, progress)
}

// SetError 设置错误状态
func (c *ProgressCache) SetError(ctx context.Context, taskID string, errMsg string, retryCount int) error {
	progress, err := c.GetProgress(ctx, taskID)
	if err != nil {
		return err
	}

	if progress == nil {
		progress = &Progress{}
	}

	progress.Stage = StageFailed
	progress.Error = errMsg
	progress.RetryCount = retryCount

	return c.SetProgress(ctx, taskID, progress)
}

// Delete 删除进度缓存
func (c *ProgressCache) Delete(ctx context.Context, taskID string) error {
	key := c.getProgressKey(taskID)
	return c.client.Del(ctx, key).Err()
}
