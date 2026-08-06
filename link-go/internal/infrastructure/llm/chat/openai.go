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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"link/internal/infrastructure/llm/httpx"
)

// ========================================
// OpenAI Client Implementation
// ========================================

// openaiClient OpenAI客户端
type openaiClient struct {
	baseURL   string
	apiKey    string
	model     string
	client    *http.Client
	toolInfos []*schema.ToolInfo
	// roundTripReasoning 是否在请求中回传 assistant 轮的 reasoning_content。
	// DeepSeek V4 thinking 模式要求"有工具调用的轮次必须原样回传 reasoning_content"，
	// 否则 400；而旧版 deepseek-reasoner 规则相反（回传反而报错）。按模型名判定。
	roundTripReasoning bool
}

// needsReasoningRoundTrip 判定该模型是否需要回传 reasoning_content。
// DeepSeek V4 系列（thinking 模式）在工具调用轮必须回传；
// 旧版 deepseek-reasoner / deepseek-chat 不回传（reasoner 回传会 400，chat 不产生 reasoning）。
// 其它 provider 通常不返回 reasoning_content，回传为空即无副作用，故默认放行。
func needsReasoningRoundTrip(modelName string) bool {
	m := strings.ToLower(modelName)
	if strings.Contains(m, "deepseek-reasoner") || strings.Contains(m, "deepseek-chat") {
		return false
	}
	return true
}

// newOpenAIClient 创建OpenAI客户端
func newOpenAIClient(config *ChatConfig) (*openaiClient, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for openai")
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}

	return &openaiClient{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		apiKey:  config.APIKey,
		model:   config.ModelName,
		// 流式(SSE)不设整体超时，由 ctx 控制；共享调优 transport 提供连接复用与连接阶段超时。
		client:             &http.Client{Transport: httpx.SharedTransport()},
		roundTripReasoning: needsReasoningRoundTrip(config.ModelName),
	}, nil
}

// Generate 非流式生成
func (c *openaiClient) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// 构建请求
	reqBody := c.buildRequest(messages, opts, false)

	// 发送请求
	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
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
	toolCalls := make([]schema.ToolCall, len(choice.Message.ToolCalls))
	if len(toolCalls) > 0 {
		fmt.Printf("🔧 [OpenAI] 工具调用: %d个\n", len(toolCalls))
	}
	for i, tc := range choice.Message.ToolCalls {
		toolCalls[i] = schema.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: schema.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
		fmt.Printf("   → %s(%s)\n", tc.Function.Name, truncateArgs(tc.Function.Arguments))
	}

	return &schema.Message{
		Role:    schema.RoleType(choice.Message.Role),
		Content: choice.Message.Content,
		// 保留 thinking 模式的 reasoning_content，供 Agent 在后续工具调用轮回传（DeepSeek V4 要求）。
		ReasoningContent: choice.Message.ReasoningContent,
		ToolCalls:        toolCalls,
		// 回填本轮 token 用量：此前该字段从未被填充，导致 ReAct 循环里 usageTotalTokens(msg)
		// 恒为 0，Agent 评测的 tokens_used 指标全部落 0。API 已返回 usage，此处透出。
		ResponseMeta: newResponseMeta(openaiResp.Usage),
	}, nil
}

// newResponseMeta 将 OpenAI 兼容响应的 usage 转成 eino schema.ResponseMeta。
// usage 全 0（部分供应商省略）时返回 nil，避免伪造零用量掩盖"未上报"与"确为 0"的区别。
func newResponseMeta(u openaiUsage) *schema.ResponseMeta {
	if u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0 {
		return nil
	}
	return &schema.ResponseMeta{
		Usage: &schema.TokenUsage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		},
	}
}

// Stream 流式生成
func (c *openaiClient) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// 构建请求
	reqBody := c.buildRequest(messages, opts, true)

	// 发送请求
	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(body))
	}

	// 创建流式读取器
	reader, writer := schema.Pipe[*schema.Message](10)

	go func() {
		defer writer.Close()
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		hasSentData := false
		var streamUsage *openaiUsage // 流末 usage chunk 携带的 token 用量（若服务端上报）
		toolCallCount := 0
		pendingToolCalls := make(map[int]*schema.ToolCall)
		// 累积 thinking 模式的 reasoning_content 分片，附加到带 tool_calls 的 assistant 消息上，
		// 以满足 DeepSeek V4 "工具调用轮必须回传 reasoning_content" 的要求。
		var reasoningBuilder strings.Builder

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// 发送任何待处理的工具调用
				if len(pendingToolCalls) > 0 {
					toolCalls := make([]schema.ToolCall, 0, len(pendingToolCalls))
					for _, tc := range pendingToolCalls {
						toolCalls = append(toolCalls, *tc)
					}
					if toolCallCount == 0 {
						toolCallCount = len(toolCalls)
						fmt.Printf("🔧 [OpenAI] 工具调用: %d个\n", toolCallCount)
					}
					for _, tc := range toolCalls {
						fmt.Printf("   → %s(%s)\n", tc.Function.Name, truncateArgs(tc.Function.Arguments))
					}
					hasSentData = true
					writer.Send(&schema.Message{
						Role:             schema.Assistant,
						Content:          "",
						ReasoningContent: reasoningBuilder.String(),
						ToolCalls:        toolCalls,
					}, nil)
					pendingToolCalls = make(map[int]*schema.ToolCall)
					reasoningBuilder.Reset()
				}

				// 如果流结束但没有发送任何数据，发送一个空消息
				if !hasSentData {
					writer.Send(&schema.Message{
						Role:    schema.Assistant,
						Content: "",
					}, nil)
				}

				// 追发一条仅携带 usage 的结尾消息，供 eino 拼接时并入 token 用量
				// （ConcatMessages 会对各分片的 Usage 取最大值合并）。
				if streamUsage != nil {
					if rm := newResponseMeta(*streamUsage); rm != nil {
						writer.Send(&schema.Message{
							Role:         schema.Assistant,
							Content:      "",
							ResponseMeta: rm,
						}, nil)
					}
				}
				return
			}

			var chunk openaiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// usage chunk 通常 choices 为空、单独在流末到达，需在 choices 分支外捕获。
			if chunk.Usage != nil {
				streamUsage = chunk.Usage
			}

			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				delta := choice.Delta

				// 处理有内容的情况
				if delta.Content != "" {
					hasSentData = true
					writer.Send(&schema.Message{
						Role:    schema.RoleType(delta.Role),
						Content: delta.Content,
					}, nil)
				}

				// 累积 reasoning_content（thinking 模式下与 content 并行分片到达，不直接下发给用户）
				if delta.ReasoningContent != "" {
					reasoningBuilder.WriteString(delta.ReasoningContent)
				}

				// 处理工具调用（在流中，工具调用在 delta.tool_calls 中）
				if len(delta.ToolCalls) > 0 {
					for _, tc := range delta.ToolCalls {
						// 获取或创建该 index 的工具调用
						index := tc.Index
						if pendingToolCalls[index] == nil {
							pendingToolCalls[index] = &schema.ToolCall{
								Function: schema.FunctionCall{
									Arguments: "",
								},
							}
						}

						// 更新工具调用信息
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

				// 检查是否完成（finish_reason 不为空）
				if choice.FinishReason != nil && *choice.FinishReason != "" {
					// 发送待处理的工具调用
					if len(pendingToolCalls) > 0 {
						toolCalls := make([]schema.ToolCall, 0, len(pendingToolCalls))
						// 按 index 排序
						for i := 0; i < len(pendingToolCalls); i++ {
							if tc := pendingToolCalls[i]; tc != nil {
								toolCalls = append(toolCalls, *tc)
							}
						}

						if toolCallCount == 0 {
							toolCallCount = len(toolCalls)
							fmt.Printf("🔧 [OpenAI] 工具调用: %d个\n", toolCallCount)
						}
						for _, tc := range toolCalls {
							fmt.Printf("   → %s(%s)\n", tc.Function.Name, truncateArgs(tc.Function.Arguments))
						}

						hasSentData = true
						writer.Send(&schema.Message{
							Role:      schema.Assistant,
							Content:   "",
							ToolCalls: toolCalls,
						}, nil)
						pendingToolCalls = make(map[int]*schema.ToolCall)
					}
				}
			}
		}
	}()

	return reader, nil
}

// truncateArgs 截断参数用于日志显示
func truncateArgs(args string) string {
	if len(args) <= 100 {
		return args
	}
	return args[:100] + "..."
}

// buildRequest 构建请求体
func (c *openaiClient) buildRequest(messages []*schema.Message, opts []model.Option, stream bool) openaiRequest {
	oaiMessages := make([]openaiMessage, len(messages))
	for i, msg := range messages {
		oaiMsg := openaiMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		}

		// 处理 assistant 消息中的 tool_calls
		if msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
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

		// 回传 reasoning_content：DeepSeek V4 thinking 模式要求"有工具调用的 assistant 轮
		// 必须原样带回 reasoning_content"，否则后续请求 400。旧版 reasoner 则相反（已由
		// roundTripReasoning=false 剔除）。仅对 assistant 且非空时回传。
		if msg.Role == schema.Assistant && c.roundTripReasoning && msg.ReasoningContent != "" {
			oaiMsg.ReasoningContent = msg.ReasoningContent
		}

		// 处理 tool 消息（工具返回结果）
		if msg.Role == schema.Tool {
			oaiMsg.ToolCallID = msg.ToolCallID
			oaiMsg.Content = msg.Content
		}

		oaiMessages[i] = oaiMsg
	}

	req := openaiRequest{
		Model:    c.model,
		Messages: oaiMessages,
		Stream:   stream,
	}
	// 流式下显式请求用量上报，否则拿不到 token 统计（tokens_used 会落 0）。
	if stream {
		req.StreamOptions = &openaiStreamOptions{IncludeUsage: true}
	}

	// 添加工具定义（如果有）
	if len(c.toolInfos) > 0 {
		req.Tools = make([]openaiTool, 0, len(c.toolInfos))
		for _, toolInfo := range c.toolInfos {
			tool := openaiTool{
				Type: "function",
				Function: &openaiToolFunction{
					Name:        toolInfo.Name,
					Description: toolInfo.Desc,
				},
			}

			// 处理参数定义
			if toolInfo.ParamsOneOf != nil {
				// 使用 ToJSONSchema 方法获取 JSON Schema
				if jsonSchema, err := toolInfo.ParamsOneOf.ToJSONSchema(); err == nil {
					// 将 JSON Schema 转换为 map
					schemaBytes, _ := json.Marshal(jsonSchema)
					var params map[string]interface{}
					if err := json.Unmarshal(schemaBytes, &params); err == nil {
						// 添加 $schema 字段（OpenAI 要求）
						params["$schema"] = "http://json.org/draft-07/schema#"
						tool.Function.Parameters = params
					}
				}
			}

			req.Tools = append(req.Tools, tool)
		}
	}

	// 应用选项 - 使用GetOptions辅助函数
	options := getOptions(opts)
	if options.Temperature != nil {
		req.Temperature = float64(*options.Temperature)
	}
	if options.TopP != nil {
		req.TopP = float64(*options.TopP)
	}
	if options.MaxTokens != nil {
		req.MaxTokens = *options.MaxTokens
	}

	return req
}

// getOptions 从选项数组中提取Options
func getOptions(opts []model.Option) *model.Options {
	options := &model.Options{}
	// 注意：由于opt.apply是未导出的，我们需要构建选项
	// 这里简化处理，直接创建一个空的Options
	// 实际使用时，应该在调用buildRequest之前就处理选项
	return options
}

// sendRequest 发送HTTP请求
func (c *openaiClient) sendRequest(ctx context.Context, reqBody openaiRequest) (*http.Response, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	return c.client.Do(req)
}

// ========================================
// OpenAI API Types
// ========================================

type openaiRequest struct {
	Model         string               `json:"model"`
	Messages      []openaiMessage      `json:"messages"`
	Temperature   float64              `json:"temperature,omitempty"`
	TopP          float64              `json:"top_p,omitempty"`
	MaxTokens     int                  `json:"max_tokens,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	StreamOptions *openaiStreamOptions `json:"stream_options,omitempty"`
	Tools         []openaiTool         `json:"tools,omitempty"`
}

// openaiStreamOptions 流式选项。include_usage=true 让服务端在流末追发一个仅含 usage 的 chunk，
// 否则流式响应不含 token 用量（OpenAI 兼容接口的默认行为）。
type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"` // For tool response messages
	// ReasoningContent 承载 DeepSeek thinking 模式的思维链，响应中解析、请求中回传（见 buildRequest）。
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// openaiToolCall 工具调用
type openaiToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openaiToolCallFunction `json:"function"`
}

// openaiToolCallFunction 工具调用函数
type openaiToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openaiTool OpenAI 工具定义
type openaiTool struct {
	Type     string              `json:"type"`
	Function *openaiToolFunction `json:"function,omitempty"`
}

// openaiToolFunction 工具函数定义
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
	// Usage 仅在开启 stream_options.include_usage 时、由流末的独立 chunk 携带（choices 为空）。
	Usage *openaiUsage `json:"usage,omitempty"`
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
	// ReasoningContent 流式下 thinking 分片，与 content 并行到达。
	ReasoningContent string `json:"reasoning_content,omitempty"`
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
