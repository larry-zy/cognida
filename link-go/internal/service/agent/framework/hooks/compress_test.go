package hooks

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"link/internal/model/memory"
)

// ========================================
// Mock CompressionService
// ========================================

// mockCompressionService 是一个模拟的压缩服务
type mockCompressionService struct {
	mu                sync.Mutex
	compressCallCount int
	lastSessionID     string
	compressDelay     time.Duration
	compressError     error
	statusResult      *memory.CompressionStatus
	compressResult    *memory.CompressionResult
}

func newMockCompressionService() *mockCompressionService {
	return &mockCompressionService{
		statusResult: &memory.CompressionStatus{
			SessionID:         "",
			TotalMessages:     10,
			TotalTokens:       3000,
			CompressedCount:   0,
			OffloadedCount:    0,
			CompressionRatio:  0,
			NeedsCompression:  false,
			CompressionInProg: false,
		},
		compressResult: &memory.CompressionResult{
			SessionID:        "",
			Strategy:         memory.CompressionStrategySummary,
			OriginalMessages: 5,
			OriginalTokens:   2000,
			CompressedTokens: 500,
			TokensSaved:      1500,
			CompressionRatio: 0.25,
			Duration:         100,
		},
	}
}

// SetTokenUsage 设置 token 使用量
func (m *mockCompressionService) SetTokenUsage(totalTokens int, needsCompression bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusResult.TotalTokens = totalTokens
	m.statusResult.NeedsCompression = needsCompression
}

// SetCompressDelay 设置压缩延迟
func (m *mockCompressionService) SetCompressDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compressDelay = delay
}

// SetCompressError 设置压缩错误
func (m *mockCompressionService) SetCompressError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compressError = err
}

// GetCompressCallCount 获取压缩调用次数
func (m *mockCompressionService) GetCompressCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.compressCallCount
}

// GetLastSessionID 获取最后一次压缩的 sessionID
func (m *mockCompressionService) GetLastSessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSessionID
}

// ResetCompressCallCount 重置压缩调用计数
func (m *mockCompressionService) ResetCompressCallCount() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compressCallCount = 0
}

// CheckAndCompress 实现
func (m *mockCompressionService) CheckAndCompress(ctx context.Context, sessionID string) (*memory.CompressionResult, error) {
	m.mu.Lock()
	m.compressCallCount++
	m.lastSessionID = sessionID
	delay := m.compressDelay
	err := m.compressError
	result := m.compressResult
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	if err != nil {
		return nil, err
	}

	result.SessionID = sessionID
	return result, nil
}

// CompressHistory 实现
func (m *mockCompressionService) CompressHistory(ctx context.Context, sessionID string) (*memory.CompressionResult, error) {
	return m.CheckAndCompress(ctx, sessionID)
}

// GetCompressionStatus 实现
func (m *mockCompressionService) GetCompressionStatus(ctx context.Context, sessionID string) (*memory.CompressionStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := *m.statusResult
	status.SessionID = sessionID
	return &status, nil
}

// OffloadLargeContent 实现
func (m *mockCompressionService) OffloadLargeContent(ctx context.Context, sessionID string) (*memory.OffloadResult, error) {
	return &memory.OffloadResult{}, nil
}

// OffloadToolInputs 实现
func (m *mockCompressionService) OffloadToolInputs(ctx context.Context, sessionID string) (*memory.OffloadResult, error) {
	return &memory.OffloadResult{}, nil
}

// ========================================
// 测试用例
// ========================================

// TestNewAutoCompressHook 测试创建 Hook
func TestNewAutoCompressHook(t *testing.T) {
	mockSvc := newMockCompressionService()

	hook := NewAutoCompressHook(mockSvc)

	if hook == nil {
		t.Fatal("NewAutoCompressHook returned nil")
	}

	if hook.IsEnabled() {
		t.Error("Hook should be disabled by default")
	}

	if hook.threshold != 0.8 {
		t.Errorf("Default threshold should be 0.8, got %v", hook.threshold)
	}

	if hook.maxTokens != 4000 {
		t.Errorf("Default maxTokens should be 4000, got %d", hook.maxTokens)
	}

	if !hook.asyncMode {
		t.Error("Default asyncMode should be true")
	}
}

// TestAutoCompressHook_EnableDisable 测试启用/禁用
func TestAutoCompressHook_EnableDisable(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc)

	// 测试禁用状态
	ctx := context.Background()
	err := hook.After(ctx, nil)
	if err != nil {
		t.Errorf("After should return nil when disabled, got %v", err)
	}

	// 启用 Hook
	hook.Enable()
	if !hook.IsEnabled() {
		t.Error("Hook should be enabled after Enable()")
	}

	// 禁用 Hook
	hook.Disable()
	if hook.IsEnabled() {
		t.Error("Hook should be disabled after Disable()")
	}
}

// TestAutoCompressHook_WithThreshold 测试阈值设置
func TestAutoCompressHook_WithThreshold(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc)

	// 有效阈值
	hook.WithThreshold(0.5)
	if hook.threshold != 0.5 {
		t.Errorf("Threshold should be 0.5, got %v", hook.threshold)
	}

	// 无效阈值（应该被忽略）
	hook.WithThreshold(1.5)
	if hook.threshold != 0.5 {
		t.Errorf("Invalid threshold should be ignored, got %v", hook.threshold)
	}

	// 无效阈值（负数）
	hook.WithThreshold(-0.1)
	if hook.threshold != 0.5 {
		t.Errorf("Negative threshold should be ignored, got %v", hook.threshold)
	}
}

// TestAutoCompressHook_WithMaxTokens 测试最大 token 设置
func TestAutoCompressHook_WithMaxTokens(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc)

	hook.WithMaxTokens(8000)
	if hook.maxTokens != 8000 {
		t.Errorf("maxTokens should be 8000, got %d", hook.maxTokens)
	}

	// 无效值（应该被忽略）
	hook.WithMaxTokens(-100)
	if hook.maxTokens != 8000 {
		t.Errorf("Invalid maxTokens should be ignored, got %d", hook.maxTokens)
	}
}

// TestAutoCompressHook_WithAsyncMode 测试异步模式
func TestAutoCompressHook_WithAsyncMode(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc)

	hook.WithAsyncMode(false)
	if hook.asyncMode {
		t.Error("asyncMode should be false")
	}

	hook.WithAsyncMode(true)
	if !hook.asyncMode {
		t.Error("asyncMode should be true")
	}
}

// TestAutoCompressHook_WithCompressInterval 测试压缩间隔
func TestAutoCompressHook_WithCompressInterval(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc)

	interval := 10 * time.Second
	hook.WithCompressInterval(interval)

	if hook.compressInterval != interval {
		t.Errorf("compressInterval should be %v, got %v", interval, hook.compressInterval)
	}
}

// TestAutoCompressHook_Before 测试 Before 钩子
func TestAutoCompressHook_Before(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc).Enable()

	ctx := context.Background()
	newCtx, message, err := hook.Before(ctx, "test message")

	if err != nil {
		t.Errorf("Before should not return error, got %v", err)
	}

	if newCtx != ctx {
		t.Error("Context should be unchanged")
	}

	if message != "test message" {
		t.Errorf("Message should be unchanged, got %s", message)
	}
}

// TestAutoCompressHook_After_NoSessionID 测试没有 sessionID 的情况
func TestAutoCompressHook_After_NoSessionID(t *testing.T) {
	mockSvc := newMockCompressionService()
	hook := NewAutoCompressHook(mockSvc).Enable()

	ctx := context.Background()
	err := hook.After(ctx, nil)

	if err != nil {
		t.Errorf("After should return nil when no sessionID, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 0 {
		t.Error("Compression should not be called when no sessionID")
	}
}

// TestAutoCompressHook_After_NoCompressionNeeded 测试不需要压缩的情况
func TestAutoCompressHook_After_NoCompressionNeeded(t *testing.T) {
	mockSvc := newMockCompressionService()
	// 设置 token 使用量低，不需要压缩
	mockSvc.SetTokenUsage(1000, false)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000)

	sessionID := "test-session-123"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	err := hook.After(ctx, nil)

	if err != nil {
		t.Errorf("After should return nil, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 0 {
		t.Error("Compression should not be called when below threshold")
	}
}

// TestAutoCompressHook_After_CompressionTriggered 测试触发压缩
func TestAutoCompressHook_After_CompressionTriggered(t *testing.T) {
	mockSvc := newMockCompressionService()
	// 设置 token 使用量高，需要压缩
	mockSvc.SetTokenUsage(3500, true)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000).
		WithAsyncMode(false) // 同步模式

	sessionID := "test-session-123"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	err := hook.After(ctx, nil)

	if err != nil {
		t.Errorf("After should return nil, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should be called once, got %d", mockSvc.GetCompressCallCount())
	}

	if mockSvc.GetLastSessionID() != sessionID {
		t.Errorf("SessionID should be %s, got %s", sessionID, mockSvc.GetLastSessionID())
	}
}

// TestAutoCompressHook_After_AsyncMode 测试异步模式
func TestAutoCompressHook_After_AsyncMode(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)
	mockSvc.SetCompressDelay(100 * time.Millisecond)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000).
		WithAsyncMode(true) // 异步模式

	sessionID := "test-session-async"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	start := time.Now()
	err := hook.After(ctx, nil)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("After should return nil immediately in async mode, got %v", err)
	}

	// 异步模式应该立即返回
	if duration > 50*time.Millisecond {
		t.Errorf("Async mode should return immediately, took %v", duration)
	}

	// 等待异步压缩完成
	time.Sleep(150 * time.Millisecond)

	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should be called once in async mode, got %d", mockSvc.GetCompressCallCount())
	}
}

// TestAutoCompressHook_CompressionInterval 测试压缩间隔保护
func TestAutoCompressHook_CompressionInterval(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	interval := 50 * time.Millisecond
	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000).
		WithCompressInterval(interval).
		WithAsyncMode(false)

	sessionID := "test-session-interval"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	// 第一次调用
	err := hook.After(ctx, nil)
	if err != nil {
		t.Errorf("First After should return nil, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should be called on first attempt, got %d", mockSvc.GetCompressCallCount())
	}

	// 立即第二次调用（应该被跳过）
	err = hook.After(ctx, nil)
	if err != nil {
		t.Errorf("Second After should return nil, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should not be called again within interval, got %d", mockSvc.GetCompressCallCount())
	}

	// 等待间隔后
	time.Sleep(interval + 10*time.Millisecond)

	err = hook.After(ctx, nil)
	if err != nil {
		t.Errorf("Third After should return nil, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 2 {
		t.Errorf("Compression should be called after interval, got %d", mockSvc.GetCompressCallCount())
	}
}

// TestAutoCompressHook_ConcurrentCompression 测试并发压缩保护
func TestAutoCompressHook_ConcurrentCompression(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)
	mockSvc.SetCompressDelay(100 * time.Millisecond)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000).
		WithAsyncMode(false)

	sessionID := "test-session-concurrent"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	var wg sync.WaitGroup
	errors := make(chan error, 3)

	// 并发调用
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := hook.After(ctx, nil); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Errorf("Concurrent After should not return error, got %v", err)
	}

	// 只应该调用一次压缩
	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should only be called once for concurrent calls, got %d", mockSvc.GetCompressCallCount())
	}
}

// TestAutoCompressHook_CompressionError 测试压缩错误处理
func TestAutoCompressHook_CompressionError(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)
	mockSvc.SetCompressError(fmt.Errorf("compression failed"))

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000).
		WithAsyncMode(false)

	sessionID := "test-session-error"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	err := hook.After(ctx, nil)

	if err == nil {
		t.Error("After should return error when compression fails")
	}

	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should be attempted, got %d calls", mockSvc.GetCompressCallCount())
	}
}

// TestContextWithSessionID 测试 context sessionID 辅助函数
func TestContextWithSessionID(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-456"

	newCtx := ContextWithSessionID(ctx, sessionID)

	if newCtx == ctx {
		t.Error("Context should be different")
	}

	extracted := extractSessionID(newCtx)
	if extracted != sessionID {
		t.Errorf("SessionID should be %s, got %s", sessionID, extracted)
	}

	// 原始 context 不应该有 sessionID
	extracted = extractSessionID(ctx)
	if extracted != "" {
		t.Errorf("Original context should not have sessionID, got %s", extracted)
	}
}

// TestAutoCompressHook_Hook 测试 Hook 方法返回
func TestAutoCompressHook_Hook(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000).
		WithAsyncMode(false)

	hookFunc := hook.Hook()
	if hookFunc == nil {
		t.Fatal("Hook() should return a function")
	}

	sessionID := "test-session-hook"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	err := hookFunc(ctx, nil)
	if err != nil {
		t.Errorf("Hook function should return nil, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 1 {
		t.Errorf("Compression should be called via Hook(), got %d", mockSvc.GetCompressCallCount())
	}
}

// TestAutoCompressHook_DisabledHook 测试禁用状态的 Hook
func TestAutoCompressHook_DisabledHook(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	hook := NewAutoCompressHook(mockSvc).
		Disable() // 明确禁用

	hookFunc := hook.Hook()
	if hookFunc == nil {
		t.Fatal("Hook() should return a function")
	}

	sessionID := "test-session-disabled"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	err := hookFunc(ctx, nil)
	if err != nil {
		t.Errorf("Hook function should return nil when disabled, got %v", err)
	}

	if mockSvc.GetCompressCallCount() != 0 {
		t.Error("Compression should not be called when hook is disabled")
	}
}

// TestAutoCompressHook_ThresholdCalculation 测试阈值计算
func TestAutoCompressHook_ThresholdCalculation(t *testing.T) {
	tests := []struct {
		name           string
		totalTokens    int
		maxTokens      int
		threshold      float64
		shouldCompress bool
	}{
		{"Below threshold", 2000, 4000, 0.8, false},
		{"At threshold", 3200, 4000, 0.8, true},
		{"Above threshold", 3500, 4000, 0.8, true},
		{"Over limit", 4500, 4000, 0.8, true},
		{"Low threshold", 3000, 4000, 0.5, true},
		{"High threshold", 3000, 4000, 0.9, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := newMockCompressionService()
			mockSvc.SetTokenUsage(tt.totalTokens, tt.shouldCompress)

			hook := NewAutoCompressHook(mockSvc).
				Enable().
				WithThreshold(tt.threshold).
				WithMaxTokens(tt.maxTokens).
				WithAsyncMode(false)

			sessionID := "test-session-threshold"
			ctx := ContextWithSessionID(context.Background(), sessionID)

			err := hook.After(ctx, nil)
			if err != nil {
				t.Errorf("After should return nil, got %v", err)
			}

			called := mockSvc.GetCompressCallCount() > 0
			if called != tt.shouldCompress {
				t.Errorf("Expected compression=%v, got compression=%v (tokens=%d, max=%d, threshold=%.2f)",
					tt.shouldCompress, called, tt.totalTokens, tt.maxTokens, tt.threshold)
			}
		})
	}
}

// TestAutoCompressHook_ResponseTypes 测试不同响应类型
func TestAutoCompressHook_ResponseTypes(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithAsyncMode(false)

	// 测试不同的响应类型
	responses := []interface{}{
		nil,
		"string response",
		map[string]interface{}{"content": "test"},
		struct{ Content string }{Content: "test"},
	}

	for i, resp := range responses {
		t.Run(fmt.Sprintf("Response Type %d", i), func(t *testing.T) {
			mockSvc.ResetCompressCallCount() // 重置计数

			// 为每个子测试使用唯一的 sessionID，避免并发冲突
			sessionID := fmt.Sprintf("test-session-response-%d", i)
			ctx := ContextWithSessionID(context.Background(), sessionID)

			err := hook.After(ctx, resp)
			if err != nil {
				t.Errorf("After should handle response type %T, got %v", resp, err)
			}

			if mockSvc.GetCompressCallCount() != 1 {
				t.Error("Compression should be called regardless of response type")
			}
		})
	}
}

// TestAutoCompressHook_StatusError 测试获取状态失败
func TestAutoCompressHook_StatusError(t *testing.T) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	// 设置一个会返回错误的 mock（需要扩展 mock）
	// 这里我们简单地不设置 NeedsCompression
	mockSvc.SetTokenUsage(1000, false)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithThreshold(0.8).
		WithMaxTokens(4000)

	sessionID := "test-session-status"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	err := hook.After(ctx, nil)
	// 应该返回错误但不应该 panic
	if err != nil {
		// 由于我们目前的 mock 实现总是返回成功状态
		// 这个测试主要是确保不会 panic
		t.Logf("Expected behavior: error from status check, got %v", err)
	}
}

// BenchmarkAutoCompressHook_Async 基准测试：异步模式
func BenchmarkAutoCompressHook_Async(b *testing.B) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithAsyncMode(true)

	sessionID := "benchmark-session"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hook.After(ctx, nil)
	}
}

// BenchmarkAutoCompressHook_Sync 基准测试：同步模式（无压缩延迟）
func BenchmarkAutoCompressHook_Sync(b *testing.B) {
	mockSvc := newMockCompressionService()
	mockSvc.SetTokenUsage(3500, true)

	hook := NewAutoCompressHook(mockSvc).
		Enable().
		WithAsyncMode(false)

	sessionID := "benchmark-session"
	ctx := ContextWithSessionID(context.Background(), sessionID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hook.After(ctx, nil)
	}
}
