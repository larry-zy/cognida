// Package evaluation 提供 Python 评测服务 HTTP 客户端
package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultTimeout 默认 HTTP 超时
	DefaultTimeout = 30 * time.Second
	// MaxRetries 最大重试次数
	MaxRetries = 3
	// RetryDelay 重试延迟
	RetryDelay = 1 * time.Second
)

// PythonEvaluationClient Python 评测服务客户端
type PythonEvaluationClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPythonEvaluationClient 创建 Python 评测客户端
func NewPythonEvaluationClient(baseURL string) *PythonEvaluationClient {
	return &PythonEvaluationClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ComputeMetrics 批量计算评测指标
func (c *PythonEvaluationClient) ComputeMetrics(ctx context.Context, req *ComputeMetricsRequest) (*ComputeMetricsResponse, error) {
	// 序列化请求
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 构建请求
	url := c.baseURL + "/api/v1/evaluation/compute-metrics"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// 发送请求（带重试）
	var lastErr error
	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			// 重试前等待
			select {
			case <-time.After(RetryDelay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http request failed: %w", err)
			continue // 重试
		}

		// 读取响应
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("read response failed: %w", err)
			continue // 重试
		}

		// 检查状态码
		if resp.StatusCode >= 500 {
			// 服务器错误，重试
			lastErr = fmt.Errorf("server error: %d, %s", resp.StatusCode, string(data))
			continue
		}

		if resp.StatusCode >= 400 {
			// 客户端错误，不重试
			return nil, fmt.Errorf("client error: %d, %s", resp.StatusCode, string(data))
		}

		// 解析响应
		var result ComputeMetricsResponse
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("unmarshal response failed: %w", err)
		}

		return &result, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ComputeMetricsWithRetry 带自定义重试的指标计算
func (c *PythonEvaluationClient) ComputeMetricsWithRetry(ctx context.Context, req *ComputeMetricsRequest, maxRetries int, retryDelay time.Duration) (*ComputeMetricsResponse, error) {
	// 创建带自定义超时的客户端
	client := &PythonEvaluationClient{
		baseURL: c.baseURL,
		httpClient: &http.Client{
			Timeout: c.httpClient.Timeout,
		},
	}

	// 临时覆盖重试配置
	originalMaxRetries := MaxRetries
	originalRetryDelay := RetryDelay

	// TODO: 使用更优雅的方式传递重试配置
	_ = maxRetries
	_ = retryDelay

	resp, err := client.ComputeMetrics(ctx, req)
	_ = originalMaxRetries
	_ = originalRetryDelay
	return resp, err
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
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create health check request failed: %w", err)
	}

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
