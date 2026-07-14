package llm

import (
	"net/http"
	"time"

	"link/internal/infrastructure/llm/resilience"
	domainllm "link/internal/model/llm"
)

// retryAfterCap 限制来自 Retry-After 头的等待上限，避免上游给出过长值拖垮请求。
const retryAfterCap = 30 * time.Second

// apiErrorFromResponse 从非 2xx HTTP 响应构建类型化 *APIError，供弹性装饰器分级。
// body 为已读取的响应体（脱敏后作为 Detail）。
func apiErrorFromResponse(provider domainllm.Provider, model string, resp *http.Response, body []byte) *domainllm.APIError {
	class := domainllm.ClassifyHTTPStatus(resp.StatusCode)
	if class == "" {
		class = domainllm.ClassTerminal
	}
	var retryAfter time.Duration
	if class == domainllm.ClassRateLimited {
		retryAfter = resilience.ParseRetryAfter(resp.Header.Get("Retry-After"), retryAfterCap, time.Now())
	}
	return &domainllm.APIError{
		Provider:   provider,
		Model:      model,
		StatusCode: resp.StatusCode,
		Class:      class,
		RetryAfter: retryAfter,
		Detail:     domainllm.SummarizeDetail(string(body)),
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
