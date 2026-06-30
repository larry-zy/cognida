package chat

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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"link/internal/infrastructure/llm/httpx"
)

// ========================================
// Ollama Chat Implementation
// ========================================

// ollamaChat Ollama聊天实现（本地模型）
type ollamaChat struct {
	base   *baseChat
	client *ollamaClient
}

// NewOllamaChat 创建Ollama聊天实例
func NewOllamaChat(config *ChatConfig) (Chat, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model_name is required for ollama")
	}

	// 默认baseURL
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}

	client, err := newOllamaClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	return &ollamaChat{
		base: &baseChat{
			config:  config,
			modelID: config.ModelID,
		},
		client: client,
	}, nil
}

// Chat 进行非流式聊天
func (c *ollamaChat) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*ChatResponse, error) {
	einoMessages := c.base.convertMessages(messages)
	modelOpts := c.base.convertOptions(opts)

	resp, err := c.client.Generate(ctx, einoMessages, modelOpts...)
	if err != nil {
		return nil, fmt.Errorf("ollama chat failed: %w", err)
	}

	return c.base.convertResponse(resp), nil
}

// ChatStream 进行流式聊天
func (c *ollamaChat) ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan StreamResponse, error) {
	einoMessages := c.base.convertMessages(messages)
	modelOpts := c.base.convertOptions(opts)

	streamReader, err := c.client.Stream(ctx, einoMessages, modelOpts...)
	if err != nil {
		return nil, fmt.Errorf("ollama stream chat failed: %w", err)
	}

	return recvStream(ctx, streamReader)
}

// GetModelName 获取模型名称
func (c *ollamaChat) GetModelName() string {
	return c.base.config.ModelName
}

// GetModelID 获取模型ID
func (c *ollamaChat) GetModelID() string {
	return c.base.modelID
}

// ========================================
// Ollama Client Implementation
// ========================================

// ollamaClient Ollama客户端，调用本地 Ollama 原生 /api/chat 接口
type ollamaClient struct {
	baseURL   string
	model     string
	client    *http.Client
	toolInfos []*schema.ToolInfo
}

// newOllamaClient 创建Ollama客户端
func newOllamaClient(config *ChatConfig) (*ollamaClient, error) {
	if config.ModelName == "" {
		return nil, fmt.Errorf("model_name is required for ollama")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	return &ollamaClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   config.ModelName,
		// 流式不设超时，由 ctx 控制；共享调优 transport 提供连接复用与连接阶段超时。
		client: &http.Client{Timeout: 0, Transport: httpx.SharedTransport()},
	}, nil
}

// Generate 非流式生成
func (c *ollamaClient) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	reqBody := c.buildRequest(messages, opts, false)

	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	toolCalls := convertOllamaToolCalls(ollamaResp.Message.ToolCalls)
	if len(toolCalls) > 0 {
		fmt.Printf("🔧 [Ollama] 工具调用: %d个\n", len(toolCalls))
		for _, tc := range toolCalls {
			fmt.Printf("   → %s(%s)\n", tc.Function.Name, truncateArgs(tc.Function.Arguments))
		}
	}

	return &schema.Message{
		Role:      roleOrAssistant(ollamaResp.Message.Role),
		Content:   ollamaResp.Message.Content,
		ToolCalls: toolCalls,
	}, nil
}

// Stream 流式生成
func (c *ollamaClient) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reqBody := c.buildRequest(messages, opts, true)

	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	reader, writer := schema.Pipe[*schema.Message](10)

	go func() {
		defer writer.Close()
		defer resp.Body.Close()

		// Ollama 流式返回 newline-delimited JSON（非 SSE）
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		hasSentData := false

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
				hasSentData = true
				writer.Send(&schema.Message{
					Role:    roleOrAssistant(chunk.Message.Role),
					Content: chunk.Message.Content,
				}, nil)
			}

			// 工具调用（Ollama 在最终块中一次性返回）
			if len(chunk.Message.ToolCalls) > 0 {
				toolCalls := convertOllamaToolCalls(chunk.Message.ToolCalls)
				fmt.Printf("🔧 [Ollama] 工具调用: %d个\n", len(toolCalls))
				for _, tc := range toolCalls {
					fmt.Printf("   → %s(%s)\n", tc.Function.Name, truncateArgs(tc.Function.Arguments))
				}
				hasSentData = true
				writer.Send(&schema.Message{
					Role:      schema.Assistant,
					Content:   "",
					ToolCalls: toolCalls,
				}, nil)
			}

			if chunk.Done {
				if !hasSentData {
					writer.Send(&schema.Message{
						Role:    schema.Assistant,
						Content: "",
					}, nil)
				}
				return
			}
		}
	}()

	return reader, nil
}

// buildRequest 构建请求体
func (c *ollamaClient) buildRequest(messages []*schema.Message, opts []model.Option, stream bool) ollamaRequest {
	ollamaMessages := make([]ollamaMessage, len(messages))
	for i, msg := range messages {
		om := ollamaMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}

		// assistant 消息中的工具调用
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
			om.ToolCalls = make([]ollamaToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				var args map[string]interface{}
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
				}
				om.ToolCalls = append(om.ToolCalls, ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: args,
					},
				})
			}
		}

		// tool 消息（工具返回结果）
		if msg.Role == schema.Tool {
			om.Content = msg.Content
		}

		ollamaMessages[i] = om
	}

	req := ollamaRequest{
		Model:    c.model,
		Messages: ollamaMessages,
		Stream:   stream,
	}

	// 工具定义
	if len(c.toolInfos) > 0 {
		req.Tools = make([]ollamaTool, 0, len(c.toolInfos))
		for _, toolInfo := range c.toolInfos {
			tool := ollamaTool{
				Type: "function",
				Function: ollamaToolFunction{
					Name:        toolInfo.Name,
					Description: toolInfo.Desc,
				},
			}
			if toolInfo.ParamsOneOf != nil {
				if jsonSchema, err := toolInfo.ParamsOneOf.ToJSONSchema(); err == nil {
					schemaBytes, _ := json.Marshal(jsonSchema)
					var params map[string]interface{}
					if err := json.Unmarshal(schemaBytes, &params); err == nil {
						tool.Function.Parameters = params
					}
				}
			}
			req.Tools = append(req.Tools, tool)
		}
	}

	// 应用选项
	options := getOptions(opts)
	ollamaOpts := ollamaOptions{}
	hasOpts := false
	if options.Temperature != nil {
		ollamaOpts.Temperature = float64(*options.Temperature)
		hasOpts = true
	}
	if options.TopP != nil {
		ollamaOpts.TopP = float64(*options.TopP)
		hasOpts = true
	}
	if options.MaxTokens != nil {
		ollamaOpts.NumPredict = *options.MaxTokens
		hasOpts = true
	}
	if hasOpts {
		req.Options = &ollamaOpts
	}

	return req
}

// sendRequest 发送HTTP请求
func (c *ollamaClient) sendRequest(ctx context.Context, reqBody ollamaRequest) (*http.Response, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return c.client.Do(req)
}

// ========================================
// Helper Functions
// ========================================

// convertOllamaToolCalls 转换 Ollama 工具调用为 eino 格式
func convertOllamaToolCalls(calls []ollamaToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]schema.ToolCall, 0, len(calls))
	for i, tc := range calls {
		// Ollama 的 arguments 是对象，转为 JSON 字符串以对齐内部格式
		argBytes, _ := json.Marshal(tc.Function.Arguments)
		result = append(result, schema.ToolCall{
			ID:   fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: string(argBytes),
			},
		})
	}
	return result
}

// roleOrAssistant 返回消息角色，缺省为 assistant
func roleOrAssistant(role string) schema.RoleType {
	if role == "" {
		return schema.Assistant
	}
	return schema.RoleType(role)
}

// ========================================
// Ollama API Types
// ========================================

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	CreatedAt       string        `json:"created_at"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}
