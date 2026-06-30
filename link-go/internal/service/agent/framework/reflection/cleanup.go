// Package reflection provides background cleanup for reflection memory.
package reflection

import (
	"context"
	"log"
	"time"

	"link/internal/model/agent/reflection"
)

// CleanupScheduler 反思记忆清理调度器
type CleanupScheduler struct {
	memory  reflection.ReflectionMemory
	ticker  *time.Ticker
	stopCh  chan struct{}
}

// NewCleanupScheduler 创建清理调度器
func NewCleanupScheduler(memory reflection.ReflectionMemory) *CleanupScheduler {
	return &CleanupScheduler{
		memory: memory,
		stopCh: make(chan struct{}),
	}
}

// Start 启动定期清理任务
func (s *CleanupScheduler) Start(interval time.Duration) {
	s.ticker = time.NewTicker(interval)

	go func() {
		log.Printf("[ReflectionCleanup] Started cleanup scheduler with interval: %v", interval)

		for {
			select {
			case <-s.ticker.C:
				s.cleanup()
			case <-s.stopCh:
				s.ticker.Stop()
				log.Printf("[ReflectionCleanup] Stopped cleanup scheduler")
				return
			}
		}
	}()
}

// Stop 停止清理任务
func (s *CleanupScheduler) Stop() {
	close(s.stopCh)
}

// cleanup 执行一次清理
func (s *CleanupScheduler) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := s.memory.Cleanup(ctx); err != nil {
		log.Printf("[ReflectionCleanup] Cleanup failed: %v", err)
	} else {
		log.Printf("[ReflectionCleanup] Cleanup completed successfully")
	}
}

// StartDefaultCleanupScheduler 使用默认配置启动清理调度器
// 默认每天凌晨 2 点执行一次清理
func StartDefaultCleanupScheduler(memory reflection.ReflectionMemory) *CleanupScheduler {
	scheduler := NewCleanupScheduler(memory)
	// 计算到下一个凌晨 2 点的时间
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	if next.Before(now) {
		next = next.Add(24 * time.Hour)
	}
	initialDelay := next.Sub(now)

	// 启动初始延迟
	time.AfterFunc(initialDelay, func() {
		scheduler.cleanup()
		// 之后每 24 小时执行一次
		scheduler.Start(24 * time.Hour)
	})

	log.Printf("[ReflectionCleanup] Scheduled first cleanup at: %v", next.Format(time.RFC3339))
	return scheduler
}
