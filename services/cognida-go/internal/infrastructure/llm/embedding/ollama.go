package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"cognida/internal/infrastructure/config"
	"cognida/internal/infrastructure/llm/httpx"
)

// OllamaEmbedder Ollama 本地向量化实现
type OllamaEmbedder struct {
	model   string
	baseURL string
	client  *http.Client
}

// NewOllamaEmbedder 创建 Ollama Embedder
func NewOllamaEmbedder(cfg *config.EmbeddingConfig) *OllamaEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	// 允许配置完整地址或仅主机地址
	baseURL = strings.TrimSuffix(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/api/embed")

	model := cfg.Model
	if model == "" {
		model = "nomic-embed-text"
	}

	return &OllamaEmbedder{
		model:   model,
		baseURL: baseURL,
		client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: httpx.SharedTransport(),
		},
	}
}

// EmbedStrings 批量向量化文本（实现 eino Embedder 接口）
func (e *OllamaEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	_ = opts

	reqBody := ollamaEmbeddingRequest{
		Model: e.model,
		Input: texts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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

	var result ollamaEmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body: %s", err, string(respBody))
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings in response")
	}

	return result.Embeddings, nil
}

// NewOllamaEmbedderWrapper 创建 Ollama Embedder（实现 Eino 接口）
func NewOllamaEmbedderWrapper(cfg *config.EmbeddingConfig) (embedding.Embedder, error) {
	return NewOllamaEmbedder(cfg), nil
}

// ========================================
// Ollama Embedding API Types
// ========================================

type ollamaEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbeddingResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float64 `json:"embeddings"`
}
