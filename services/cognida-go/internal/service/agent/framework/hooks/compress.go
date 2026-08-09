// Package hooks provides Agent lifecycle hook implementations.
package hooks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cognida/internal/model/memory"
)

// ========================================
// CompressionService 压缩服务接口
// ========================================

// CompressionService defines the interface for memory compression.
type CompressionService interface {
	CheckAndCompress(ctx context.Context, sessionID string) (*memory.CompressionResult, error)
	GetCompressionStatus(ctx context.Context, sessionID string) (*memory.CompressionStatus, error)
}

// ========================================
// AutoCompressHook 自动压缩 Hook
// ========================================

// AutoCompressHook 在每次响应后自动检查并压缩会话历史。
// 当 token 数量超过阈值时，自动执行压缩策略。
type AutoCompressHook struct {
	*BaseHook
	compressionService CompressionService
	threshold          float64 // 压缩阈值（0-1），相对于 maxTokens
	maxTokens          int     // 最大 token 数量
	mu                 sync.Mutex
	compressingMap     map[string]bool // 正在压缩的会话
	asyncMode          bool // 是否异步执行压缩
	compressInterval   time.Duration // 最小压缩间隔
	lastCompressTime   map[string]time.Time // 每个会话上次压缩时间
}

// NewAutoCompressHook 创建自动压缩 Hook
func NewAutoCompressHook(service CompressionService) *AutoCompressHook {
	return &AutoCompressHook{
		BaseHook:           NewBaseHook("auto_compress", nil), // 不需要 LLM
		compressionService: service,
		threshold:          0.8, // 默认 80% 触发压缩
		maxTokens:          4000, // 默认最大 token 数
		compressingMap:     make(map[string]bool),
		asyncMode:          true, // 默认异步模式
		compressInterval:   30 * time.Second, // 默认最小间隔 30 秒
		lastCompressTime:   make(map[string]time.Time),
	}
}

// Enable 启用 Hook
func (h *AutoCompressHook) Enable() *AutoCompressHook {
	h.BaseHook.Enable()
	return h
}

// Disable 禁用 Hook
func (h *AutoCompressHook) Disable() *AutoCompressHook {
	h.BaseHook.Disable()
	return h
}

// WithThreshold 设置压缩阈值
func (h *AutoCompressHook) WithThreshold(threshold float64) *AutoCompressHook {
	if threshold > 0 && threshold <= 1 {
		h.threshold = threshold
	}
	return h
}

// WithMaxTokens 设置最大 token 数量
func (h *AutoCompressHook) WithMaxTokens(maxTokens int) *AutoCompressHook {
	if maxTokens > 0 {
		h.maxTokens = maxTokens
	}
	return h
}

// WithAsyncMode 设置是否异步执行压缩
// 异步模式下，压缩不会阻塞响应返回
func (h *AutoCompressHook) WithAsyncMode(async bool) *AutoCompressHook {
	h.asyncMode = async
	return h
}

// WithCompressInterval 设置最小压缩间隔
func (h *AutoCompressHook) WithCompressInterval(interval time.Duration) *AutoCompressHook {
	h.compressInterval = interval
	return h
}

// Before implements domain.HookService.Before.
// AutoCompressHook 不需要 Before 钩子
func (h *AutoCompressHook) Before(ctx context.Context, message string) (context.Context, string, error) {
	if !h.IsEnabled() {
		return ctx, message, nil
	}
	return ctx, message, nil
}

// After implements domain.HookService.After.
// 检查会话是否需要压缩，如果需要则执行压缩
func (h *AutoCompressHook) After(ctx context.Context, resp interface{}) error {
	if !h.IsEnabled() {
		return nil
	}

	// 从 context 中提取 sessionID
	sessionID := extractSessionID(ctx)
	if sessionID == "" {
		// 没有 sessionID，跳过压缩
		return nil
	}

	// 检查是否已在压缩中
	h.mu.Lock()
	if h.compressingMap[sessionID] {
		h.mu.Unlock()
		return nil // 已在压缩中，跳过
	}

	// 检查压缩间隔
	if lastTime, ok := h.lastCompressTime[sessionID]; ok {
		if time.Since(lastTime) < h.compressInterval {
			h.mu.Unlock()
			return nil // 距上次压缩时间太短，跳过
		}
	}
	h.mu.Unlock()

	// 获取压缩状态
	status, err := h.compressionService.GetCompressionStatus(ctx, sessionID)
	if err != nil {
		// 获取状态失败，记录但不中断
		return fmt.Errorf("failed to get compression status: %w", err)
	}

	// 检查是否需要压缩
	if !status.NeedsCompression {
		return nil
	}

	// 检查 token 使用量
	usageRatio := float64(status.TotalTokens) / float64(h.maxTokens)
	if usageRatio < h.threshold {
		return nil // 未达到阈值
	}

	// 执行压缩
	if h.asyncMode {
		// 异步模式：不阻塞响应
		go func() {
			_ = h.executeCompress(context.Background(), sessionID) // 异步压缩失败保持静默，不影响主流程
		}()
		return nil
	}

	// 同步模式：等待压缩完成
	return h.executeCompress(ctx, sessionID)
}

// executeCompress 执行压缩
func (h *AutoCompressHook) executeCompress(ctx context.Context, sessionID string) error {
	// 标记为正在压缩
	h.mu.Lock()
	if h.compressingMap[sessionID] {
		h.mu.Unlock()
		return nil
	}
	h.compressingMap[sessionID] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.compressingMap, sessionID)
		h.lastCompressTime[sessionID] = time.Now()
		h.mu.Unlock()
	}()

	// 执行压缩
	result, err := h.compressionService.CheckAndCompress(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("compression failed: %w", err)
	}

	// 记录压缩结果（可以改为使用 logger）
	if result != nil && result.Strategy != memory.CompressionStrategyNone {
		_ = fmt.Sprintf("compressed session %s: strategy=%s, saved %d tokens",
			sessionID, result.Strategy, result.TokensSaved)
	}

	return nil
}

// Hook 返回 AfterHook 函数（兼容通用接口）
func (h *AutoCompressHook) Hook() func(ctx context.Context, resp interface{}) error {
	return func(ctx context.Context, resp interface{}) error {
		return h.SafeExecute(ctx, func() error {
			return h.After(ctx, resp)
		})
	}
}

// ========================================
// 辅助函数
// ========================================

// sessionKey 是 context 中存储 sessionID 的 key 类型
type sessionKey struct{}

// ContextWithSessionID 将 sessionID 添加到 context
func ContextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey{}, sessionID)
}

// extractSessionID 从 context 中提取 sessionID
func extractSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value(sessionKey{}).(string); ok {
		return sessionID
	}
	return ""
}
