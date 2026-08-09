// Package llm 提供 LLM 领域的类型化错误分级。
//
// 各 provider 客户端在失败路径 SHALL 返回 *APIError（而非裸字符串错误），
// 使上层弹性装饰器能据 error_class 决策：重试 / 降级 / 熔断 / 立即失败。
package llm

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrorClass LLM 调用错误分级。
type ErrorClass string

const (
	// ClassRateLimited 限流（HTTP 429）。可重试，且优先遵循 Retry-After。
	ClassRateLimited ErrorClass = "rate_limited"
	// ClassTransient 瞬时错（5xx / 网络抖动 / 可重试超时 / 连接重置）。可重试。
	ClassTransient ErrorClass = "transient"
	// ClassTerminal 终态错（4xx 鉴权/参数/未找到、或不确定错误）。不重试。
	ClassTerminal ErrorClass = "terminal"
	// ClassCanceled 调用方 ctx 取消/超时。不重试、不降级，直接透传。
	ClassCanceled ErrorClass = "canceled"
)

// Retryable 报告该分级是否应重试。
func (c ErrorClass) Retryable() bool {
	return c == ClassTransient || c == ClassRateLimited
}

// Countable 报告该分级是否计入熔断失败计数。
// terminal（配置类错误）与 canceled（调用方主动取消）不代表目标不健康，不计入。
func (c ErrorClass) Countable() bool {
	return c == ClassTransient || c == ClassRateLimited
}

// APIError 类型化的 LLM 调用错误。
type APIError struct {
	Provider   Provider      // 目标 provider
	Model      string        // 目标模型名
	StatusCode int           // HTTP 状态码，0 表示传输层错误
	Class      ErrorClass    // 错误分级
	RetryAfter time.Duration // 仅 rate_limited，解析自 Retry-After，0 表示无
	Detail     string        // 脱敏后的错误摘要（不含 api_key/host）
	Err        error         // 包裹的底层错误（errors.Unwrap 可达）
}

// Error 实现 error 接口，输出脱敏后的摘要。
func (e *APIError) Error() string {
	target := string(e.Provider)
	if e.Model != "" {
		target += "/" + e.Model
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("llm %s: %s (status %d): %s", target, e.Class, e.StatusCode, e.Detail)
	}
	return fmt.Sprintf("llm %s: %s: %s", target, e.Class, e.Detail)
}

// Unwrap 使 errors.Is/As 可回溯到底层错误。
func (e *APIError) Unwrap() error { return e.Err }

// Retryable 报告该错误是否应重试。
func (e *APIError) Retryable() bool { return e.Class.Retryable() }

// AsAPIError 从错误链中提取 *APIError。
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// ClassifyHTTPStatus 将 HTTP 状态码映射为 ErrorClass（纯函数，不感知网络错误）。
// 返回空字符串表示成功或无法据 status 判定（需结合传输层错误）。
func ClassifyHTTPStatus(code int) ErrorClass {
	switch {
	case code == 429:
		return ClassRateLimited
	case code == 408: // 请求超时，视为可重试
		return ClassTransient
	case code >= 500:
		return ClassTransient
	case code >= 400:
		return ClassTerminal
	default:
		return ""
	}
}

// bearerPattern 匹配 Authorization Bearer token，用于脱敏。
var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]+`)

// skKeyPattern 匹配常见的 sk- 前缀 API key。
var skKeyPattern = regexp.MustCompile(`sk-[A-Za-z0-9]{8,}`)

// RedactSecrets 脱敏错误摘要中的敏感信息（api_key/bearer token）。
func RedactSecrets(s string) string {
	s = bearerPattern.ReplaceAllString(s, "Bearer [REDACTED]")
	s = skKeyPattern.ReplaceAllString(s, "sk-[REDACTED]")
	return s
}

// SummarizeDetail 生成脱敏且长度受限的错误摘要。
func SummarizeDetail(s string) string {
	s = RedactSecrets(strings.TrimSpace(s))
	const maxLen = 512
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}
