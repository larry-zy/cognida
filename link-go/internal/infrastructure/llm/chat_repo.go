// Package llm 提供 LLM 基础设施层的聊天仓储实现
// @Deprecated: This package is deprecated. Use Eino framework (infrastructure/llm/chat/) instead.
// The direct HTTP implementation has been replaced by the Eino-based implementation.
package llm

import (
	"bufio"
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

// ========================================
// OpenAI Chat Repository (DEPRECATED)
// ========================================

// openaiChatRepo OpenAI 聊天仓储实现
type openaiChatRepo struct {
	provider domainllm.Provider
	baseURL  string
	apiKey   string
	model    string
	client   *http.Client
	tools    []*domainllm.Tool
}

// NewOpenAIChatRepo 创建 OpenAI LLM 客户端。
//
// Deprecated: 已退出 live 路径，OpenAI 兼容 provider 统一改用 NewEinoLLMClient（基于 eino
// 的 DSML 归一化客户端）。本实现缺少 DeepSeek 原生 tool_calls 归一化，仅保留供既有单测覆盖，
// 请勿在新代码中引用。
func NewOpenAIChatRepo(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for openai")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	provider := config.Provider
	if provider == "" {
		provider = domainllm.ProviderOpenAI
	}

	return &openaiChatRepo{
		provider: provider,
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		apiKey:   config.APIKey,
		model:    config.ModelName,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// Chat 执行非流式聊天
func (r *openaiChatRepo) Chat(ctx context.Context, req *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	// 构建请求
	reqBody := r.buildRequest(req, false)

	// 发送请求
	resp, err := r.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, apiErrorFromTransport(r.provider, r.model, err)
	}
	defer resp.Body.Close()

	// 解析响应
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiErrorFromResponse(r.provider, r.model, resp, body)
	}

	var openaiResp openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	choice := openaiResp.Choices[0]

	// 转换 ToolCalls
	toolCalls := make([]*domainllm.ToolCall, len(choice.Message.ToolCalls))
	for i, tc := range choice.Message.ToolCalls {
		toolCalls[i] = &domainllm.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: &domainllm.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}

	// 构建 Message
	message := &domainllm.Message{
		Role:      choice.Message.Role,
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
	}

	return &domainllm.ChatResponse{
		Message:      message,
		Content:      choice.Message.Content,
		ToolCalls:    toolCalls,
		Usage:        convertUsage(openaiResp.Usage),
		FinishReason: choice.FinishReason,
		Metadata: map[string]interface{}{
			"model":  openaiResp.Model,
			"id":     openaiResp.ID,
			"object": openaiResp.Object,
		},
	}, nil
}

// ChatStream 执行流式聊天
func (r *openaiChatRepo) ChatStream(ctx context.Context, req *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	// 构建请求
	reqBody := r.buildRequest(req, true)

	// 发送请求
	resp, err := r.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, apiErrorFromTransport(r.provider, r.model, err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, apiErrorFromResponse(r.provider, r.model, resp, body)
	}

	// 创建流式通道
	resultChan := make(chan *domainllm.ChatChunk, 10)

	go func() {
		defer close(resultChan)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		pendingToolCalls := make(map[int]*domainllm.ToolCall)

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// 发送任何待处理的工具调用
				if len(pendingToolCalls) > 0 {
					toolCalls := make([]*domainllm.ToolCall, 0, len(pendingToolCalls))
					for i := 0; i < len(pendingToolCalls); i++ {
						if tc := pendingToolCalls[i]; tc != nil {
							toolCalls = append(toolCalls, tc)
						}
					}
					resultChan <- &domainllm.ChatChunk{
						ToolCalls: toolCalls,
						Done:      true,
					}
					pendingToolCalls = make(map[int]*domainllm.ToolCall)
				}

				// 发送结束标记
				resultChan <- &domainllm.ChatChunk{
					Done: true,
				}
				return
			}

			var chunk openaiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				delta := choice.Delta

				// 处理内容
				if delta.Content != "" {
					resultChan <- &domainllm.ChatChunk{
						Content: delta.Content,
						Role:    delta.Role,
					}
				}

				// 处理工具调用
				if len(delta.ToolCalls) > 0 {
					for _, tc := range delta.ToolCalls {
						index := tc.Index
						if pendingToolCalls[index] == nil {
							pendingToolCalls[index] = &domainllm.ToolCall{
								Function: &domainllm.FunctionCall{},
							}
						}

						if tc.ID != "" {
							pendingToolCalls[index].ID = tc.ID
						}
						if tc.Type != "" {
							pendingToolCalls[index].Type = tc.Type
						}
						if tc.Function.Name != "" {
							pendingToolCalls[index].Function.Name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							pendingToolCalls[index].Function.Arguments += tc.Function.Arguments
						}
					}
				}

				// 检查是否完成
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					if len(pendingToolCalls) > 0 {
						toolCalls := make([]*domainllm.ToolCall, 0, len(pendingToolCalls))
						for i := 0; i < len(pendingToolCalls); i++ {
							if tc := pendingToolCalls[i]; tc != nil {
								toolCalls = append(toolCalls, tc)
							}
						}
						resultChan <- &domainllm.ChatChunk{
							ToolCalls: toolCalls,
						}
						pendingToolCalls = make(map[int]*domainllm.ToolCall)
					}
				}
			}
		}
	}()

	return resultChan, nil
}

// GetModelInfo 获取模型信息
func (r *openaiChatRepo) GetModelInfo(ctx context.Context) (*domainllm.ModelInfo, error) {
	return &domainllm.ModelInfo{
		Name:        r.model,
		DisplayName: r.model,
		Provider:    domainllm.ProviderOpenAI,
		Types:       []domainllm.ModelType{domainllm.ModelTypeChat},
		Features:    []string{"stream", "tools"},
	}, nil
}

// SupportsTools 检查是否支持工具调用
func (r *openaiChatRepo) SupportsTools() bool {
	return true
}

// SupportsStreaming 检查是否支持流式输出
func (r *openaiChatRepo) SupportsStreaming() bool {
	return true
}

// buildRequest 构建请求体
func (r *openaiChatRepo) buildRequest(req *domainllm.ChatRequest, stream bool) openaiRequest {
	messages := make([]openaiMessage, len(req.Messages))
	for i, msg := range req.Messages {
		oaiMsg := openaiMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		// 处理工具调用
		if msg.Role == domainllm.RoleAssistant && len(msg.ToolCalls) > 0 {
			oaiMsg.ToolCalls = make([]openaiToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				oaiMsg.ToolCalls[j] = openaiToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: openaiToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		// 处理工具返回结果
		if msg.Role == domainllm.RoleTool {
			oaiMsg.ToolCallID = msg.ToolID
			oaiMsg.Content = msg.Content
		}

		messages[i] = oaiMsg
	}

	oaiReq := openaiRequest{
		Model:    r.model,
		Messages: messages,
		Stream:   stream,
	}

	// 添加工具
	if len(r.tools) > 0 {
		oaiReq.Tools = make([]openaiTool, len(r.tools))
		for i, tool := range r.tools {
			oaiReq.Tools[i] = openaiTool{
				Type: tool.Type,
				Function: &openaiToolFunction{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			}
		}
	}

	// 应用选项
	if req.Options != nil {
		if req.Options.Temperature > 0 {
			oaiReq.Temperature = req.Options.Temperature
		}
		if req.Options.TopP > 0 {
			oaiReq.TopP = req.Options.TopP
		}
		if req.Options.MaxTokens > 0 {
			oaiReq.MaxTokens = req.Options.MaxTokens
		}
		if req.Options.Stop != nil {
			oaiReq.Stop = req.Options.Stop
		}
	}

	return oaiReq
}

// sendRequest 发送HTTP请求
func (r *openaiChatRepo) sendRequest(ctx context.Context, reqBody openaiRequest) (*http.Response, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	return r.client.Do(req)
}

// SetTools 设置工具列表
func (r *openaiChatRepo) SetTools(tools []*domainllm.Tool) {
	r.tools = tools
}

// ========================================
// Ollama Chat Repository
// ========================================

// ollamaChatRepo Ollama 聊天仓储实现
type ollamaChatRepo struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaChatRepo 创建 Ollama LLM 客户端
func NewOllamaChatRepo(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model_name is required for ollama")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	return &ollamaChatRepo{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   config.ModelName,
		client:  &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// Chat 执行非流式聊天
func (r *ollamaChatRepo) Chat(ctx context.Context, req *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	resp, err := r.sendRequest(ctx, req, false)
	if err != nil {
		return nil, apiErrorFromTransport(domainllm.ProviderOllama, r.model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, apiErrorFromResponse(domainllm.ProviderOllama, r.model, resp, body)
	}

	var oResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	role := oResp.Message.Role
	if role == "" {
		role = domainllm.RoleAssistant
	}

	return &domainllm.ChatResponse{
		Message: &domainllm.Message{
			Role:    role,
			Content: oResp.Message.Content,
		},
		Content:      oResp.Message.Content,
		Usage:        oResp.usage(),
		FinishReason: oResp.DoneReason,
		Metadata: map[string]interface{}{
			"model": oResp.Model,
		},
	}, nil
}

// ChatStream 执行流式聊天
func (r *ollamaChatRepo) ChatStream(ctx context.Context, req *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	resp, err := r.sendRequest(ctx, req, true)
	if err != nil {
		return nil, apiErrorFromTransport(domainllm.ProviderOllama, r.model, err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, apiErrorFromResponse(domainllm.ProviderOllama, r.model, resp, body)
	}

	resultChan := make(chan *domainllm.ChatChunk, 10)

	go func() {
		defer close(resultChan)
		defer resp.Body.Close()

		// Ollama 流式返回 NDJSON，逐行一个 JSON 对象
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var chunk ollamaChatResponse
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}

			if chunk.Message.Content != "" {
				select {
				case resultChan <- &domainllm.ChatChunk{
					Content: chunk.Message.Content,
					Role:    chunk.Message.Role,
				}:
				case <-ctx.Done():
					return
				}
			}

			if chunk.Done {
				select {
				case resultChan <- &domainllm.ChatChunk{Done: true}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()

	return resultChan, nil
}

// sendRequest 构建并发送 Ollama /api/chat 请求
func (r *ollamaChatRepo) sendRequest(ctx context.Context, req *domainllm.ChatRequest, stream bool) (*http.Response, error) {
	messages := make([]ollamaMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = ollamaMessage{Role: msg.Role, Content: msg.Content}
	}

	oReq := ollamaChatRequest{
		Model:    r.model,
		Messages: messages,
		Stream:   stream,
	}

	if req.Options != nil {
		opts := map[string]interface{}{}
		if req.Options.Temperature > 0 {
			opts["temperature"] = req.Options.Temperature
		}
		if req.Options.TopP > 0 {
			opts["top_p"] = req.Options.TopP
		}
		if req.Options.TopK > 0 {
			opts["top_k"] = req.Options.TopK
		}
		if req.Options.MaxTokens > 0 {
			opts["num_predict"] = req.Options.MaxTokens
		}
		if len(req.Options.Stop) > 0 {
			opts["stop"] = req.Options.Stop
		}
		if len(opts) > 0 {
			oReq.Options = opts
		}
	}

	body, err := json.Marshal(oReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", r.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	return r.client.Do(httpReq)
}

// ========================================
// Ollama API Types
// ========================================

type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaMessage        `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

func (o *ollamaChatResponse) usage() *domainllm.Usage {
	return &domainllm.Usage{
		PromptTokens:     o.PromptEvalCount,
		CompletionTokens: o.EvalCount,
		TotalTokens:      o.PromptEvalCount + o.EvalCount,
	}
}

// GetModelInfo 获取模型信息
func (r *ollamaChatRepo) GetModelInfo(ctx context.Context) (*domainllm.ModelInfo, error) {
	return &domainllm.ModelInfo{
		Name:        r.model,
		DisplayName: r.model,
		Provider:    domainllm.ProviderOllama,
		Types:       []domainllm.ModelType{domainllm.ModelTypeChat},
		Features:    []string{"stream"},
	}, nil
}

// SupportsTools 检查是否支持工具调用
func (r *ollamaChatRepo) SupportsTools() bool {
	return false // Ollama 工具调用支持待实现
}

// SupportsStreaming 检查是否支持流式输出
func (r *ollamaChatRepo) SupportsStreaming() bool {
	return true
}

// ========================================
// OpenAI API Types
// ========================================

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []openaiTool    `json:"tools,omitempty"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openaiToolCallFunction `json:"function"`
}

type openaiToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiTool struct {
	Type     string              `json:"type"`
	Function *openaiToolFunction `json:"function,omitempty"`
}

type openaiToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openaiChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Index        int           `json:"index"`
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []openaiStreamChoice `json:"choices"`
}

type openaiStreamChoice struct {
	Index        int                    `json:"index"`
	Delta        openaiDelta            `json:"delta"`
	FinishReason *string                `json:"finish_reason"`
	ToolCalls    []openaiStreamToolCall `json:"tool_calls,omitempty"`
}

type openaiDelta struct {
	Role      string                 `json:"role,omitempty"`
	Content   string                 `json:"content,omitempty"`
	ToolCalls []openaiStreamToolCall `json:"tool_calls,omitempty"`
}

type openaiStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ========================================
// 辅助函数
// ========================================

// convertUsage 转换使用量
func convertUsage(u openaiUsage) *domainllm.Usage {
	return &domainllm.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

// ========================================
// 工厂函数
// ========================================

// NewChatRepository 创建 LLM 客户端。
// OpenAI 兼容 provider 统一走基于 eino 的 DSML 归一化客户端（NewEinoLLMClient），
// 与 modelFactory.registerDefaultProviders 保持一致，退休已弃用的 openaiChatRepo。
func NewChatRepository(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
	switch config.Provider {
	case domainllm.ProviderOpenAI, domainllm.ProviderAliyun, domainllm.ProviderDeepSeek,
		domainllm.ProviderLKEAP, domainllm.ProviderQwen, domainllm.ProviderGeneric:
		return NewEinoLLMClient(config)
	case domainllm.ProviderOllama:
		return NewOllamaChatRepo(config)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}
}
