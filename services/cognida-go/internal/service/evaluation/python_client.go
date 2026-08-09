// Package evaluation 提供 Python 评测服务 HTTP 客户端
package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"

	"cognida/internal/infrastructure/reliability"
	agentctx "cognida/internal/model/agent"
)

// setTraceHeaders 将 request_id 透传到 Python 评测服务，实现跨进程链路追踪。
func setTraceHeaders(ctx context.Context, req *http.Request) {
	if rid, ok := agentctx.GetRequestID(ctx); ok && rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}
}

const (
	// MaxRetries 最大重试次数
	MaxRetries = 3
	// RetryDelay 重试延迟
	RetryDelay = 1 * time.Second

	// 各端点超时改由 context 逐次控制，不再用 http.Client.Timeout 一刀切
	// （后者会把慢的批量指标计算与快的健康检查/目录查询一起卡在同一上限）。

	// HealthTimeout 健康检查超时
	HealthTimeout = 5 * time.Second
	// GraderCatalogTimeout 指标目录查询超时
	GraderCatalogTimeout = 15 * time.Second
	// ComputeBaseTimeout 批量指标计算的基础超时
	ComputeBaseTimeout = 60 * time.Second
	// ComputePerItemTimeout 批量指标计算每条追加的超时预算。
	// LLM 裁判逐条调大模型（可含多维度），80 条可达数分钟，固定 30s 必然超时。
	ComputePerItemTimeout = 8 * time.Second
	// ComputeMaxTimeout 批量指标计算超时上限
	ComputeMaxTimeout = 20 * time.Minute
)

// computeTimeout 按待评条数伸缩批量指标计算的超时：基础 + 每条预算，封顶上限。
func computeTimeout(itemCount int) time.Duration {
	d := ComputeBaseTimeout + time.Duration(itemCount)*ComputePerItemTimeout
	if d > ComputeMaxTimeout {
		return ComputeMaxTimeout
	}
	return d
}

// PythonEvaluationClient Python 评测服务客户端
type PythonEvaluationClient struct {
	baseURL    string
	httpClient *http.Client
	// breaker 与 gRPC 通路同源的统一熔断（〔X-4〕）：批量指标计算在 Python 持续不可用时
	// 快速失败，避免逐条 8s 预算堆到 20min 上限空等。仅护重的 compute 路径，健康检查/目录查询
	// 不熔断（健康检查是恢复探测手段，熔断它会掩盖恢复）。
	breaker *reliability.Breaker
}

// NewPythonEvaluationClient 创建 Python 评测客户端
func NewPythonEvaluationClient(baseURL string) *PythonEvaluationClient {
	return &PythonEvaluationClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			// 不设 http.Client.Timeout（0=不限）：整体超时由各方法用 context 分端点控制，
			// 使慢的批量指标计算不被健康检查那种短上限误杀。
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		breaker: reliability.NewRegistry(reliability.DefaultConfig()).Get(baseURL),
	}
}

// ComputeMetrics 批量计算评测指标（使用默认重试配置）
func (c *PythonEvaluationClient) ComputeMetrics(ctx context.Context, req *ComputeMetricsRequest) (*ComputeMetricsResponse, error) {
	return c.computeMetrics(ctx, req, MaxRetries, RetryDelay)
}

// ComputeMetricsWithRetry 带自定义重试配置的指标计算
func (c *PythonEvaluationClient) ComputeMetricsWithRetry(ctx context.Context, req *ComputeMetricsRequest, maxRetries int, retryDelay time.Duration) (*ComputeMetricsResponse, error) {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if retryDelay < 0 {
		retryDelay = 0
	}
	return c.computeMetrics(ctx, req, maxRetries, retryDelay)
}

// computeMetrics 批量计算评测指标的内部实现，重试次数与延迟可配置
func (c *PythonEvaluationClient) computeMetrics(ctx context.Context, req *ComputeMetricsRequest, maxRetries int, retryDelay time.Duration) (*ComputeMetricsResponse, error) {
	// 序列化请求
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	url := c.baseURL + "/api/v1/evaluation/compute-metrics"

	// 单次尝试的超时按条数伸缩（LLM 裁判逐条调大模型，批量可达数分钟）。
	timeout := computeTimeout(len(req.Items))

	// 熔断快速失败：Python 持续不可用时不再堆积重试与长超时。
	if !c.breaker.Allow() {
		return nil, fmt.Errorf("python 评测服务熔断打开(%s)，暂拒请求以快速失败", c.baseURL)
	}

	// 发送请求（带重试）
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// 重试前等待
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				c.breaker.OnResult(false, false) // 调用方取消，非目标不健康，不计入熔断
				return nil, ctx.Err()
			}
		}

		result, retryable, err := c.computeAttempt(ctx, url, body, timeout)
		if err == nil {
			c.breaker.OnResult(true, false)
			return result, nil
		}
		lastErr = err
		if !retryable {
			c.breaker.OnResult(false, false) // 4xx/解析错误：客户端/契约问题，不计入熔断
			return nil, err                  // 客户端错误/解析错误，不重试
		}
	}

	// 重试耗尽（网络/5xx）：目标不健康，计入熔断。
	c.breaker.OnResult(false, true)
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// computeAttempt 执行一次指标计算请求。retryable 标识该错误是否值得重试
// （网络/超时、5xx 可重试；4xx、解析失败不可重试）。每次尝试独立 context 超时。
func (c *PythonEvaluationClient) computeAttempt(ctx context.Context, url string, body []byte, timeout time.Duration) (result *ComputeMetricsResponse, retryable bool, err error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 每次尝试构建新的请求（body 为一次性 Reader，重试需重新构建）
	httpReq, err := http.NewRequestWithContext(attemptCtx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setTraceHeaders(ctx, httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, true, fmt.Errorf("http request failed: %w", err) // 网络/超时，可重试
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("server error: %d, %s", resp.StatusCode, string(data))
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("client error: %d, %s", resp.StatusCode, string(data))
	}

	var out ComputeMetricsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, false, fmt.Errorf("unmarshal response failed: %w", err)
	}
	return &out, false, nil
}

// ListGraders 拉取按评测类型过滤的可用指标目录（注册表元数据的唯一事实来源在 Python 侧）。
// evalType 未知时 Python 返回 400，本方法据此返回错误，不做静默全量回退。
func (c *PythonEvaluationClient) ListGraders(ctx context.Context, evalType string) (*GraderCatalog, error) {
	url := fmt.Sprintf("%s/api/v1/evaluation/graders?eval_type=%s", c.baseURL, neturl.QueryEscape(evalType))

	ctx, cancel := context.WithTimeout(ctx, GraderCatalogTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	setTraceHeaders(ctx, httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("grader catalog error: %d, %s", resp.StatusCode, string(data))
	}

	var result GraderCatalog
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response failed: %w", err)
	}
	return &result, nil
}

// SetTimeout 设置 HTTP 超时
func (c *PythonEvaluationClient) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}

// SetHTTPClient 设置自定义 HTTP 客户端
func (c *PythonEvaluationClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// HealthCheck 健康检查
func (c *PythonEvaluationClient) HealthCheck(ctx context.Context) error {
	url := c.baseURL + "/health"
	ctx, cancel := context.WithTimeout(ctx, HealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create health check request failed: %w", err)
	}
	setTraceHeaders(ctx, req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status: %d", resp.StatusCode)
	}

	return nil
}

// ========================================
// Error Types
// ========================================

// HTTPError HTTP 错误
type HTTPError struct {
	StatusCode int
	Message    string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 网络错误可重试
	if IsNetworkError(err) {
		return true
	}

	// HTTP 5xx 错误可重试
	if httpErr, ok := err.(*HTTPError); ok && httpErr.StatusCode >= 500 {
		return true
	}

	return false
}

// IsNetworkError 判断是否为网络错误
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 检查常见的网络错误字符串
	errStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"no such host",
		"network is unreachable",
	}

	for _, msg := range networkErrors {
		if contains(errStr, msg) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
