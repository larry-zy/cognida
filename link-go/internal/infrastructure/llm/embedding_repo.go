// Package llm 提供 LLM 基础设施层的嵌入向量仓储实现
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

	domainllm "link/internal/model/llm"
)

// embeddingRepo 是 domain EmbeddingRepository 的 OpenAI 兼容实现（含 Ollama 分支）。
type embeddingRepo struct {
	provider  domainllm.Provider
	apiKey    string
	model     string
	endpoint  string
	client    *http.Client
	dimension int // 缓存的向量维度，0 表示未知
}

// newEmbeddingRepo 根据模型配置创建嵌入向量仓储。
func newEmbeddingRepo(config *domainllm.ModelConfig) (domainllm.EmbeddingRepository, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model_name is required for embedding model")
	}

	base := strings.TrimSuffix(config.BaseURL, "/")
	var endpoint string
	if config.Provider == domainllm.ProviderOllama {
		if base == "" {
			base = "http://localhost:11434"
		}
		endpoint = base + "/api/embeddings"
	} else {
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		endpoint = base + "/embeddings"
	}

	return &embeddingRepo{
		provider: config.Provider,
		apiKey:   config.APIKey,
		model:    config.ModelName,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// Embed 生成文本嵌入向量
func (e *embeddingRepo) Embed(ctx context.Context, req *domainllm.EmbeddingRequest) (*domainllm.EmbeddingResponse, error) {
	if req == nil || len(req.Texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}
	if e.provider == domainllm.ProviderOllama {
		return e.embedOllama(ctx, req)
	}
	return e.embedOpenAI(ctx, req)
}

// EmbedBatch 批量生成嵌入向量（OpenAI 接口本身即支持批量输入）
func (e *embeddingRepo) EmbedBatch(ctx context.Context, req *domainllm.EmbeddingRequest) (*domainllm.EmbeddingResponse, error) {
	return e.Embed(ctx, req)
}

// GetDimension 获取向量维度（首次调用探测并缓存）
func (e *embeddingRepo) GetDimension(ctx context.Context) (int, error) {
	if e.dimension > 0 {
		return e.dimension, nil
	}
	resp, err := e.Embed(ctx, &domainllm.EmbeddingRequest{Texts: []string{"dimension probe"}})
	if err != nil {
		return 0, err
	}
	if len(resp.Embeddings) == 0 || len(resp.Embeddings[0]) == 0 {
		return 0, fmt.Errorf("empty embedding returned")
	}
	e.dimension = len(resp.Embeddings[0])
	return e.dimension, nil
}

// embedOpenAI 调用 OpenAI 兼容的 /embeddings 接口（支持批量输入）
func (e *embeddingRepo) embedOpenAI(ctx context.Context, req *domainllm.EmbeddingRequest) (*domainllm.EmbeddingResponse, error) {
	payload := map[string]interface{}{
		"model": e.model,
		"input": req.Texts,
	}
	if req.Options != nil && req.Options.Dimensions > 0 {
		payload["dimensions"] = req.Options.Dimensions
	}

	body, err := e.postJSON(ctx, payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body: %s", err, string(body))
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no data in embedding response")
	}

	// 按 index 回填，保证与输入顺序一致
	embeddings := make([][]float32, len(req.Texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(embeddings) {
			continue
		}
		embeddings[d.Index] = d.Embedding
	}

	return &domainllm.EmbeddingResponse{
		Embeddings: embeddings,
		Model:      result.Model,
		Usage: &domainllm.Usage{
			PromptTokens: result.Usage.PromptTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
	}, nil
}

// embedOllama 调用 Ollama /api/embeddings 接口（单条 prompt，逐条请求）
func (e *embeddingRepo) embedOllama(ctx context.Context, req *domainllm.EmbeddingRequest) (*domainllm.EmbeddingResponse, error) {
	embeddings := make([][]float32, len(req.Texts))
	for i, text := range req.Texts {
		body, err := e.postJSON(ctx, map[string]interface{}{
			"model":  e.model,
			"prompt": text,
		})
		if err != nil {
			return nil, err
		}
		var result struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decode response failed: %w", err)
		}
		embeddings[i] = result.Embedding
	}
	return &domainllm.EmbeddingResponse{Embeddings: embeddings, Model: e.model}, nil
}

// postJSON 发送 JSON POST 请求并返回响应体
func (e *embeddingRepo) postJSON(ctx context.Context, payload interface{}) ([]byte, error) {
	if e.apiKey == "" && e.provider != domainllm.ProviderOllama {
		return nil, fmt.Errorf("api_key is required for %s embedding", e.provider)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, apiErrorFromTransport(e.provider, e.model, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apiErrorFromTransport(e.provider, e.model, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, apiErrorFromResponse(e.provider, e.model, resp, data)
	}
	return data, nil
}
