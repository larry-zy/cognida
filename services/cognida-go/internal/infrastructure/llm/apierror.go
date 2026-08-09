package llm

import (
	"net/http"
	"time"

	"cognida/internal/infrastructure/llm/resilience"
	domainllm "cognida/internal/model/llm"
)

// retryAfterCap 限制来自 Retry-After 头的等待上限，避免上游给出过长值拖垮请求。
const retryAfterCap = 30 * time.Second

// apiErrorFromResponse 从非 2xx HTTP 响应构建类型化 *APIError，供弹性装饰器分级。
// body 为已读取的响应体（脱敏后作为 Detail）。
func apiErrorFromResponse(provider domainllm.Provider, model string, resp *http.Response, body []byte) *domainllm.APIError {
	return apiErrorFromStatus(provider, model, resp.StatusCode, resp.Header.Get("Retry-After"), string(body))
}

// apiErrorFromStatus 从状态码/Retry-After 头/响应体三元组构建类型化 *APIError。
// 与 apiErrorFromResponse 共享分级逻辑，供无法直接持有 *http.Response 的调用方
// （如 eino 客户端仅拿到 chat.HTTPStatusError）复用，确保 429/5xx 的重试判定一致。
func apiErrorFromStatus(provider domainllm.Provider, model string, statusCode int, retryAfterHeader, body string) *domainllm.APIError {
	class := domainllm.ClassifyHTTPStatus(statusCode)
	if class == "" {
		class = domainllm.ClassTerminal
	}
	var retryAfter time.Duration
	if class == domainllm.ClassRateLimited {
		retryAfter = resilience.ParseRetryAfter(retryAfterHeader, retryAfterCap, time.Now())
	}
	return &domainllm.APIError{
		Provider:   provider,
		Model:      model,
		StatusCode: statusCode,
		Class:      class,
		RetryAfter: retryAfter,
		Detail:     domainllm.SummarizeDetail(body),
	}
}

// apiErrorFromTransport 从传输层/构造错误构建类型化 *APIError（网络错误→transient，取消→canceled，其余→terminal）。
func apiErrorFromTransport(provider domainllm.Provider, model string, err error) *domainllm.APIError {
	return &domainllm.APIError{
		Provider: provider,
		Model:    model,
		Class:    resilience.Classify(0, err),
		Detail:   domainllm.SummarizeDetail(err.Error()),
		Err:      err,
	}
}
