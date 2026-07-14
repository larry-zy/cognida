// Package resilience 提供 LLM 调用的统一弹性能力：类型化错误分级、
// 请求级退避重试、per-target 熔断、有序模型降级/故障转移链。
//
// 装饰对象是领域接口 domainllm.LLMClient / EmbeddingRepository / RerankRepository，
// 对上层调用方透明，返回类型不变。
package resilience

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	domainllm "link/internal/model/llm"
)

// Classify 统一裁决错误分级。各 provider 客户端 MUST NOT 各写一套分级逻辑。
//
// 优先级：调用方 ctx 取消 > HTTP 状态 > 传输层网络错误 > 保守归 terminal。
//   - context.Canceled                → canceled（调用方主动取消）
//   - context.DeadlineExceeded        → transient（每次尝试子 ctx 超时，可换目标/重试）
//   - HTTP status（见 ClassifyHTTPStatus）
//   - net 连接重置/拒绝/EOF/超时       → transient
//   - 其它无法识别                     → terminal（不确定不重试）
func Classify(statusCode int, err error) domainllm.ErrorClass {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return domainllm.ClassCanceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return domainllm.ClassTransient
		}
	}
	if statusCode > 0 {
		if c := domainllm.ClassifyHTTPStatus(statusCode); c != "" {
			return c
		}
	}
	if err != nil && isTransientNetErr(err) {
		return domainllm.ClassTransient
	}
	if statusCode > 0 {
		// 有明确状态码但落到此处（如 3xx/2xx 被当作错误）——保守归终态。
		return domainllm.ClassTerminal
	}
	if err != nil {
		return domainllm.ClassTerminal
	}
	return domainllm.ClassTerminal
}

// isTransientNetErr 判定是否为可重试的传输层网络错误。
func isTransientNetErr(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		// 连接超时/临时网络错误均视为可重试。
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected eof",
		"no such host",
		"i/o timeout",
		"tls handshake timeout",
		"server closed",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// ParseRetryAfter 解析 Retry-After 头（秒数或 HTTP-date 两种格式），并按 cap 封顶。
// 无法解析或为空返回 0。now 用于 HTTP-date 差值计算（可注入以便测试）。
func ParseRetryAfter(header string, cap time.Duration, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	// 形式一：整数秒
	if secs, err := strconv.Atoi(header); err == nil {
		return capDuration(time.Duration(secs)*time.Second, cap)
	}
	// 形式二：HTTP-date
	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return capDuration(d, cap)
	}
	return 0
}

// capDuration 将 d 封顶到 cap（cap<=0 表示不封顶）。
func capDuration(d, cap time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if cap > 0 && d > cap {
		return cap
	}
	return d
}
