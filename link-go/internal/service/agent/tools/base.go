// Package tools provides base tool class for agent tools
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino/components/tool"
)

// ========================================
// Base Tool - 工具基类
// ========================================

// BaseTool 工具基类，提供通用功能
type BaseTool struct {
	// Name 工具名称
	Name string

	// Description 工具描述
	Description string

	// ParamsOneOf 可选：机器可读的参数 schema（下发给 LLM 的 function parameters）。
	// 为 nil 时工具仅凭 Description 文本约束入参；非 nil 时 Info() 会带上，
	// 让参数形状在格式层被 LLM 遵循（推荐用 eino utils.GoStruct2ParamsOneOf 从入参 struct 反射生成）。
	ParamsOneOf *schema.ParamsOneOf

	// Handler 工具处理函数
	Handler func(ctx context.Context, args string) (string, error)

	// ParseArgs 可选：自定义参数解析函数
	// 如果不提供，使用默认的 JSON 解析
	ParseArgs func(args string) (interface{}, error)

	// FormatResult 可选：自定义结果格式化函数
	// 如果不提供，使用默认的 JSON 格式化
	FormatResult func(result interface{}) (string, error)

	// Validate 可选：参数验证函数
	Validate func(args interface{}) error

	// Init 可选：初始化函数
	Init func() error

	// ========================================
	// 通用维护能力
	// ========================================

	// Metrics 指标收集器
	Metrics *ToolMetrics

	// Cache 结果缓存
	Cache *ToolCache

	// Audit 审计日志
	Audit *ToolAudit

	// RateLimiter 速率限制器
	RateLimiter *ToolRateLimiter
}

// InvokableRun 实现 tool.InvokableTool 接口
func (t *BaseTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	startTime := time.Now()

	// 执行初始化（如果需要）
	if t.Init != nil {
		if err := t.Init(); err != nil {
			t.recordMetrics(false, time.Since(startTime), err)
			return "", fmt.Errorf("tool initialization failed: %w", err)
		}
		// 初始化后清除，避免重复执行
		t.Init = nil
	}

	// 速率限制检查
	if t.RateLimiter != nil && !t.RateLimiter.Allow() {
		t.recordMetrics(false, time.Since(startTime), fmt.Errorf("rate limit exceeded"))
		return "", fmt.Errorf("rate limit exceeded for tool %s", t.Name)
	}

	// 生成缓存键
	cacheKey := t.generateCacheKey(args)

	// 尝试从缓存获取
	if t.Cache != nil {
		if cached, found := t.Cache.Get(cacheKey); found {
			t.recordMetrics(true, time.Since(startTime), nil)
			t.logAudit(ctx, args, cached, true, "", time.Since(startTime))
			return cached, nil
		}
	}

	// 解析参数
	parsedArgs := args
	if t.ParseArgs != nil {
		parsed, err := t.ParseArgs(args)
		if err != nil {
			t.recordMetrics(false, time.Since(startTime), err)
			return "", fmt.Errorf("failed to parse args: %w", err)
		}
		if t.Validate != nil {
			if err := t.Validate(parsed); err != nil {
				t.recordMetrics(false, time.Since(startTime), err)
				return "", fmt.Errorf("args validation failed: %w", err)
			}
		}
		// 重新序列化为 JSON 传递给 handler
		jsonBytes, err := json.Marshal(parsed)
		if err != nil {
			t.recordMetrics(false, time.Since(startTime), err)
			return "", err
		}
		parsedArgs = string(jsonBytes)
	}

	// 调用处理函数
	result, err := t.Handler(ctx, parsedArgs)
	latency := time.Since(startTime)

	if err != nil {
		t.recordMetrics(false, latency, err)
		t.logAudit(ctx, args, "", false, err.Error(), latency)
		return "", err
	}

	// 格式化结果
	if t.FormatResult != nil {
		formatted, err := t.FormatResult(result)
		if err != nil {
			t.recordMetrics(false, latency, err)
			t.logAudit(ctx, args, result, false, err.Error(), latency)
			return "", fmt.Errorf("failed to format result: %w", err)
		}
		result = formatted
	}

	// 缓存结果
	if t.Cache != nil {
		t.Cache.Set(cacheKey, result)
	}

	// 记录指标和审计
	t.recordMetrics(true, latency, nil)
	t.logAudit(ctx, args, result, true, "", latency)

	return result, nil
}

// recordMetrics 记录指标
func (t *BaseTool) recordMetrics(success bool, latency time.Duration, err error) {
	if t.Metrics == nil {
		return
	}
	t.Metrics.Record(success, latency.Milliseconds(), err)
}

// logAudit 记录审计日志
func (t *BaseTool) logAudit(ctx context.Context, input, output string, success bool, errMsg string, latency time.Duration) {
	if t.Audit == nil {
		return
	}

	userID := extractUserID(ctx)
	sessionID := extractSessionID(ctx)

	t.Audit.Log(AuditRecord{
		Time:      time.Now(),
		ToolName:  t.Name,
		Input:     truncateString(input, 1000),
		Output:    truncateString(output, 1000),
		Success:   success,
		Error:     errMsg,
		LatencyMs: latency.Milliseconds(),
		UserID:    userID,
		SessionID: sessionID,
	})
}

// generateCacheKey 生成缓存键
func (t *BaseTool) generateCacheKey(args string) string {
	return fmt.Sprintf("%s:%s", t.Name, args)
}

// extractUserID 从上下文提取用户ID
func extractUserID(ctx context.Context) string {
	if uid := ctx.Value("user_id"); uid != nil {
		if uidStr, ok := uid.(string); ok {
			return uidStr
		}
	}
	return ""
}

// extractSessionID 从上下文提取会话ID
func extractSessionID(ctx context.Context) string {
	if sid := ctx.Value("session_id"); sid != nil {
		if sidStr, ok := sid.(string); ok {
			return sidStr
		}
	}
	return ""
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Info 实现 tool.InvokableTool 接口
func (t *BaseTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        t.Name,
		Desc:        t.Description,
		ParamsOneOf: t.ParamsOneOf, // 为 nil 时等价于无参数 schema（纯描述驱动）
	}, nil
}

// ========================================
// 工具指标收集
// ========================================

// ToolMetrics 工具指标
type ToolMetrics struct {
	mu               sync.RWMutex
	totalCalls       int64
	successCalls     int64
	failureCalls     int64
	totalLatencyMs   int64
	minLatencyMs     int64
	maxLatencyMs     int64
	lastCallTime     time.Time
	lastError       string
	callHistory      []CallRecord // 最近的调用记录
	historyLimit     int
}

// CallRecord 调用记录
type CallRecord struct {
	Time     time.Time `json:"time"`
	Success  bool      `json:"success"`
	LatencyMs int64     `json:"latency_ms"`
	Error    string    `json:"error,omitempty"`
}

// NewToolMetrics 创建工具指标收集器
func NewToolMetrics(historyLimit int) *ToolMetrics {
	if historyLimit <= 0 {
		historyLimit = 100 // 默认保留最近100条
	}
	return &ToolMetrics{
		minLatencyMs: int64(^uint64(0) >> 1), // 最大值
		maxLatencyMs: 0,
		callHistory:  make([]CallRecord, 0, historyLimit),
		historyLimit: historyLimit,
	}
}

// Record 记录一次调用
func (m *ToolMetrics) Record(success bool, latencyMs int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.totalCalls++
	m.totalLatencyMs += latencyMs
	m.lastCallTime = now

	if success {
		m.successCalls++
	} else {
		m.failureCalls++
		if err != nil {
			m.lastError = err.Error()
		}
	}

	// 更新最小/最大延迟
	if latencyMs < m.minLatencyMs {
		m.minLatencyMs = latencyMs
	}
	if latencyMs > m.maxLatencyMs {
		m.maxLatencyMs = latencyMs
	}

	// 添加调用历史
	m.callHistory = append(m.callHistory, CallRecord{
		Time:      now,
		Success:   success,
		LatencyMs: latencyMs,
	})
	if len(m.callHistory) > m.historyLimit {
		m.callHistory = m.callHistory[1:]
	}
}

// Stats 获取统计信息
func (m *ToolMetrics) Stats() MetricsStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var avgLatency float64
	if m.totalCalls > 0 {
		avgLatency = float64(m.totalLatencyMs) / float64(m.totalCalls)
	}

	return MetricsStats{
		TotalCalls:     m.totalCalls,
		SuccessCalls:   m.successCalls,
		FailureCalls:   m.failureCalls,
		SuccessRate:    float64(m.successCalls) / float64(m.totalCalls),
		AvgLatencyMs:   avgLatency,
		MinLatencyMs:   m.minLatencyMs,
		MaxLatencyMs:   m.maxLatencyMs,
		LastCallTime:   m.lastCallTime,
		LastError:      m.lastError,
		RecentCalls:    len(m.callHistory),
	}
}

// MetricsStats 指标统计
type MetricsStats struct {
	TotalCalls    int64     `json:"total_calls"`
	SuccessCalls  int64     `json:"success_calls"`
	FailureCalls  int64     `json:"failure_calls"`
	SuccessRate   float64   `json:"success_rate"`
	AvgLatencyMs  float64   `json:"avg_latency_ms"`
	MinLatencyMs  int64     `json:"min_latency_ms"`
	MaxLatencyMs  int64     `json:"max_latency_ms"`
	LastCallTime  time.Time `json:"last_call_time"`
	LastError     string    `json:"last_error,omitempty"`
	RecentCalls   int       `json:"recent_calls"`
}

// ========================================
// 工具结果缓存
// ========================================

// ToolCache 工具结果缓存
type ToolCache struct {
	mu    sync.RWMutex
	store map[string]*CacheEntry
	ttl   time.Duration
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Value      string
	ExpireAt   time.Time
	HitCount   int
	CreateTime time.Time
}

// NewToolCache 创建工具缓存
func NewToolCache(ttl time.Duration) *ToolCache {
	return &ToolCache{
		store: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
}

// Get 获取缓存
func (c *ToolCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.store[key]
	if !exists {
		return "", false
	}

	// 检查是否过期
	if time.Now().After(entry.ExpireAt) {
		return "", false
	}

	entry.HitCount++
	return entry.Value, true
}

// Set 设置缓存
func (c *ToolCache) Set(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store[key] = &CacheEntry{
		Value:      value,
		ExpireAt:   time.Now().Add(c.ttl),
		CreateTime: time.Now(),
	}
}

// Clear 清除缓存
func (c *ToolCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = make(map[string]*CacheEntry)
}

// Stats 缓存统计
func (c *ToolCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalHits := 0
	for _, entry := range c.store {
		if !time.Now().After(entry.ExpireAt) {
			totalHits += entry.HitCount
		}
	}

	return CacheStats{
		Size:     len(c.store),
		TotalHits: totalHits,
	}
}

// CacheStats 缓存统计
type CacheStats struct {
	Size      int `json:"size"`
	TotalHits int `json:"total_hits"`
}

// ========================================
// 工具审计日志
// ========================================

// ToolAudit 工具审计日志
type ToolAudit struct {
	mu      sync.RWMutex
	records []AuditRecord
	limit   int
}

// AuditRecord 审计记录
type AuditRecord struct {
	Time     time.Time `json:"time"`
	ToolName string    `json:"tool_name"`
	Input    string    `json:"input,omitempty"`
	Output   string    `json:"output,omitempty"`
	Success  bool      `json:"success"`
	Error    string    `json:"error,omitempty"`
	LatencyMs int64    `json:"latency_ms"`
	UserID   string    `json:"user_id,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
}

// NewToolAudit 创建审计日志
func NewToolAudit(limit int) *ToolAudit {
	if limit <= 0 {
		limit = 1000
	}
	return &ToolAudit{
		records: make([]AuditRecord, 0, limit),
		limit:   limit,
	}
}

// Log 记录审计日志
func (a *ToolAudit) Log(record AuditRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.records = append(a.records, record)
	if len(a.records) > a.limit {
		a.records = a.records[1:]
	}
}

// List 列出审计记录
func (a *ToolAudit) List(limit int) []AuditRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 || limit > len(a.records) {
		limit = len(a.records)
	}

	start := len(a.records) - limit
	result := make([]AuditRecord, limit)
	copy(result, a.records[start:])
	return result
}

// ========================================
// 工具速率限制
// ========================================

// ToolRateLimiter 工具速率限制器
type ToolRateLimiter struct {
	mu          sync.Mutex
	requests    []time.Time
	maxRequests int
	window      time.Duration
}

// NewToolRateLimiter 创建速率限制器
func NewToolRateLimiter(maxRequests int, window time.Duration) *ToolRateLimiter {
	return &ToolRateLimiter{
		requests:    make([]time.Time, 0, maxRequests),
		maxRequests: maxRequests,
		window:      window,
	}
}

// Allow 检查是否允许请求
func (r *ToolRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// 清除过期请求
	cutoff := now.Add(-r.window)
	validIdx := 0
	for _, t := range r.requests {
		if t.After(cutoff) {
			r.requests[validIdx] = t
			validIdx++
		}
	}
	r.requests = r.requests[:validIdx]

	// 检查是否超过限制
	if len(r.requests) >= r.maxRequests {
		return false
	}

	r.requests = append(r.requests, now)
	return true
}

// Reset 重置速率限制
func (r *ToolRateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = make([]time.Time, 0, r.maxRequests)
}

// ========================================
// Typed Base Tool - 类型安全的工具基类
// ========================================

// TypedBaseTool 类型安全的工具基类
// 使用泛型提供更好的类型安全性
type TypedBaseTool[Request any, Response any] struct {
	*BaseTool
}

// InvokableRun 实现 tool.InvokableTool 接口
// 这个方法是必需的，因为 Go 的泛型类型不会自动提升嵌入字段的方法
func (t *TypedBaseTool[Request, Response]) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	return t.BaseTool.InvokableRun(ctx, args, opts...)
}

// Info 实现 tool.Info 接口
func (t *TypedBaseTool[Request, Response]) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.BaseTool.Info(ctx)
}

// NewTypedBaseTool 创建类型安全的工具
func NewTypedBaseTool[Request any, Response any](
	name string,
	description string,
	handler func(ctx context.Context, req *Request) (*Response, error),
) *TypedBaseTool[Request, Response] {
	base := &BaseTool{
		Name:        name,
		Description: description,
		// 默认启用指标收集
		Metrics: NewToolMetrics(100),
		// 默认不启用缓存
		Cache: nil,
		// 默认不启用审计日志
		Audit: nil,
		// 默认不启用速率限制
		RateLimiter: nil,
	}

	base.Handler = func(ctx context.Context, args string) (string, error) {
		var req Request
		if err := json.Unmarshal([]byte(args), &req); err != nil {
			return "", fmt.Errorf("failed to parse request: %w", err)
		}

		resp, err := handler(ctx, &req)
		if err != nil {
			return "", err
		}

		result, err := json.Marshal(resp)
		if err != nil {
			return "", fmt.Errorf("failed to serialize response: %w", err)
		}

		return string(result), nil
	}

	return &TypedBaseTool[Request, Response]{BaseTool: base}
}

// ========================================
// 工具配置选项
// ========================================

// ToolOption 工具配置选项
type ToolOption func(*BaseTool)

// WithMetrics 启用指标收集
func WithMetrics(historyLimit int) ToolOption {
	return func(t *BaseTool) {
		t.Metrics = NewToolMetrics(historyLimit)
	}
}

// WithCache 启用结果缓存
func WithCache(ttl time.Duration) ToolOption {
	return func(t *BaseTool) {
		t.Cache = NewToolCache(ttl)
	}
}

// WithAudit 启用审计日志
func WithAudit(limit int) ToolOption {
	return func(t *BaseTool) {
		t.Audit = NewToolAudit(limit)
	}
}

// WithRateLimiter 启用速率限制
func WithRateLimiter(maxRequests int, window time.Duration) ToolOption {
	return func(t *BaseTool) {
		t.RateLimiter = NewToolRateLimiter(maxRequests, window)
	}
}

// WithMaintenance 启用所有维护能力
func WithMaintenance() ToolOption {
	return func(t *BaseTool) {
		t.Metrics = NewToolMetrics(100)
		t.Cache = NewToolCache(5 * time.Minute)
		t.Audit = NewToolAudit(1000)
		t.RateLimiter = NewToolRateLimiter(100, time.Minute)
	}
}

// ========================================
// Service Based Tool - 基于服务的工具
// ========================================

// ServiceBasedTool 基于服务的工具基类
// 用于需要外部服务依赖的工具
type ServiceBasedTool struct {
	*BaseTool

	// ServiceName 服务名称（用于依赖注入）
	ServiceName string

	// RequiredServices 必需的服务列表
	RequiredServices []string
}

// NewServiceBasedTool 创建基于服务的工具
func NewServiceBasedTool(name, description, serviceName string, handler func(ctx context.Context, args string) (string, error)) *ServiceBasedTool {
	return &ServiceBasedTool{
		BaseTool: &BaseTool{
			Name:        name,
			Description: description,
			Handler:     handler,
		},
		ServiceName: serviceName,
	}
}

// InvokableRun 实现 tool.InvokableTool 接口
func (t *ServiceBasedTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	return t.BaseTool.InvokableRun(ctx, args, opts...)
}

// Info 实现 tool.Info 接口
func (t *ServiceBasedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.BaseTool.Info(ctx)
}

// ========================================
// Helper Functions
// ========================================

// MustCreate 创建工具，失败时 panic
// 用于简化工具创建
func MustCreate(t tool.InvokableTool, err error) tool.InvokableTool {
	if err != nil {
		panic(fmt.Sprintf("failed to create tool: %v", err))
	}
	return t
}

// ParseJSONArgs JSON 参数解析辅助函数
func ParseJSONArgs(args string, target interface{}) error {
	return json.Unmarshal([]byte(args), target)
}

// FormatJSONResult JSON 结果格式化辅助函数
func FormatJSONResult(result interface{}) (string, error) {
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
