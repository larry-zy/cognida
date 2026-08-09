// Package llm 提供 LLM 基础设施层的重排仓储实现
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domainllm "cognida/internal/model/llm"
)

// rerankMaxDocs 单次重排支持的最大文档数（保守上限）
const rerankMaxDocs = 1000

// rerankRepo 是 domain RerankRepository 的实现，调用 Jina/Cohere/DashScope
// 兼容的 /rerank 接口。
type rerankRepo struct {
	provider domainllm.Provider
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// newRerankRepo 根据模型配置创建重排仓储。
func newRerankRepo(config *domainllm.ModelConfig) (domainllm.RerankRepository, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model_name is required for rerank model")
	}

	base := strings.TrimSuffix(config.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	endpoint := base
	if !strings.HasSuffix(base, "/rerank") {
		endpoint = base + "/rerank"
	}

	return &rerankRepo{
		provider: config.Provider,
		apiKey:   config.APIKey,
		model:    config.ModelName,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// GetMaxDocs 获取支持的最大文档数
func (r *rerankRepo) GetMaxDocs() int { return rerankMaxDocs }

// Rerank 重排文档列表
func (r *rerankRepo) Rerank(ctx context.Context, req *domainllm.RerankRequest) (*domainllm.RerankResponse, error) {
	if req == nil || req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if len(req.Docs) == 0 {
		return &domainllm.RerankResponse{Results: []*domainllm.RerankResult{}, Model: r.model}, nil
	}
	if r.apiKey == "" {
		return nil, fmt.Errorf("api_key is required for rerank")
	}

	payload := map[string]interface{}{
		"model":     r.model,
		"query":     req.Query,
		"documents": req.Docs,
	}
	if req.Options != nil && req.Options.TopK > 0 {
		payload["top_n"] = req.Options.TopK
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", r.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, apiErrorFromTransport(r.provider, r.model, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apiErrorFromTransport(r.provider, r.model, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiErrorFromResponse(r.provider, r.model, resp, data)
	}

	var result struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body: %s", err, string(data))
	}

	results := make([]*domainllm.RerankResult, 0, len(result.Results))
	for _, item := range result.Results {
		doc := ""
		if item.Index >= 0 && item.Index < len(req.Docs) {
			doc = req.Docs[item.Index]
		}
		results = append(results, &domainllm.RerankResult{
			Index:    item.Index,
			Score:    item.RelevanceScore,
			Document: doc,
		})
	}

	return &domainllm.RerankResponse{Results: results, Model: r.model}, nil
}
