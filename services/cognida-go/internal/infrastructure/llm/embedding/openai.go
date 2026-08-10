package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"cognida/internal/config"
	"cognida/internal/infrastructure/llm/httpx"
	"github.com/cloudwego/eino/components/embedding"
)

// OpenAIEmbedder OpenAI（及兼容 API）向量化实现
type OpenAIEmbedder struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIEmbedder 创建 OpenAI Embedder
func NewOpenAIEmbedder(cfg *config.EmbeddingConfig) *OpenAIEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1/embeddings"
	}

	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	return &OpenAIEmbedder{
		apiKey:  cfg.APIKey,
		model:   model,
		baseURL: baseURL,
		client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: httpx.SharedTransport(),
		},
	}
}

// EmbedStrings 批量向量化文本（实现 eino Embedder 接口）
func (e *OpenAIEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}
	if e.apiKey == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is not configured")
	}

	_ = opts

	reqBody := openAIEmbeddingRequest{
		Model: e.model,
		Input: texts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	var result openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body: %s", err, string(respBody))
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no data in response")
	}

	// 按 index 排序，保证与输入顺序一致
	embeddings := make([][]float64, len(texts))
	for _, data := range result.Data {
		if data.Index < 0 || data.Index >= len(embeddings) {
			continue
		}
		embeddings[data.Index] = data.Embedding
	}

	return embeddings, nil
}

// NewOpenAIEmbedderWrapper 创建 OpenAI Embedder（实现 Eino 接口）
func NewOpenAIEmbedderWrapper(cfg *config.EmbeddingConfig) (embedding.Embedder, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	return NewOpenAIEmbedder(cfg), nil
}

// ========================================
// OpenAI Embedding API Types
// ========================================

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []openAIEmbeddingData `json:"data"`
	Model  string                `json:"model"`
	Usage  openAIEmbeddingUsage  `json:"usage"`
}

type openAIEmbeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type openAIEmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
