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

// DashScopeEmbedder DashScope 向量化实现
type DashScopeEmbedder struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewDashScopeEmbedder 创建 DashScope Embedder
func NewDashScopeEmbedder(cfg *config.EmbeddingConfig) *DashScopeEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings"
	}

	return &DashScopeEmbedder{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: baseURL,
		client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: httpx.SharedTransport(),
		},
	}
}

// EmbedStrings 批量向量化文本（实现 eino Embedder 接口）
func (e *DashScopeEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts cannot be empty")
	}

	if e.apiKey == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY is not configured")
	}

	// TODO: 如果需要支持 eino 的选项（如 WithModel），可以在这里处理
	// 目前暂时忽略选项
	_ = opts

	// 构建请求
	reqBody := e.buildRequest(texts)

	// 发送请求
	resp, err := e.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("[DashScope Embedding] API Error (status %d): %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	// 调试：打印响应（仅首次）
	// fmt.Printf("[DEBUG] Response body: %s\n", string(body))

	// 解析响应
	var result dashScopeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w, body: %s", err, string(body))
	}

	// 检查是否有数据
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no data in response")
	}

	// 提取向量
	embeddings := make([][]float64, len(texts))
	for i, data := range result.Data {
		if i >= len(embeddings) {
			break
		}
		embeddings[i] = data.Embedding
	}

	return embeddings, nil
}

// buildRequest 构建请求体
func (e *DashScopeEmbedder) buildRequest(texts []string) dashScopeRequest {
	req := dashScopeRequest{
		Model: e.model,
		Input: texts,
	}

	return req
}

// sendRequest 发送 HTTP 请求
func (e *DashScopeEmbedder) sendRequest(ctx context.Context, reqBody dashScopeRequest) (*http.Response, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 调试：打印请求信息
	fmt.Printf("[DashScope Embedding] API Key: %s..., Model: %s, Texts count: %d, URL: %s\n",
		maskKey(e.apiKey), e.model, len(reqBody.Input), e.baseURL)

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	return e.client.Do(req)
}

// maskKey 掩盖 API Key 用于日志
func maskKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// ========================================
// DashScope API Types
// ========================================

type dashScopeRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"` // 直接使用数组，不是嵌套对象
}

type dashScopeResponse struct {
	Data      []dashScopeData `json:"data"`
	Usage     dashScopeUsage  `json:"usage"`
	RequestId string          `json:"request_id"`
}

type dashScopeData struct {
	Embedding []float64 `json:"embedding"`
}

type dashScopeUsage struct {
	TotalTokens int `json:"total_tokens"`
}
