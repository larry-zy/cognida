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

	"cognida/internal/infrastructure/llm/httpx"
)

// HTTPStatusError 表示上游返回了非 2xx HTTP 状态。它保留状态码与原始 Retry-After 头，
// 供上层弹性装饰器（llm/resilience）精确分级——把可重试的 429/5xx 与终止的 4xx 区分开，
// 避免在只拿到格式化字符串时误判为终态而不重试/降级。Error() 保持原有文案以兼容既有断言。
type HTTPStatusError struct {
	StatusCode int
	RetryAfter string // 原始 Retry-After 头值，可空
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Body)
}

// ========================================
// OpenAI Client Implementation
// ========================================

// openaiClient OpenAI客户端
// defaultMaxOutputTokens 是调用方未显式指定 max_tokens 时的兜底输出上限。
// 取 DeepSeek 单轮生成上限 8192：thinking 模型的 reasoning_content 计入输出预算，
// 依赖 provider 较小的默认上限会在 tool_call 发出前被长思维链耗尽（finish_reason=length），
// 显式抬到 8192 给 thinking + 正文 + tool_call 留足空间，避免截断导致 agent"卡住"。
const defaultMaxOutputTokens = 8192

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
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After"), Body: string(body)}
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
		// 同时透出 finish_reason，供上层识别 length 截断（thinking 链撑爆输出预算）。
		ResponseMeta: newResponseMeta(openaiResp.Usage, choice.FinishReason),
	}, nil
}

// newResponseMeta 将 OpenAI 兼容响应的 usage + finish_reason 转成 eino schema.ResponseMeta。
// usage 全 0（部分供应商省略）且 finishReason 为空时返回 nil，避免伪造零用量掩盖"未上报"与"确为 0"
// 的区别；但只要 finishReason 非空就必须返回非 nil，否则 ReAct 循环无从判定截断（finish_reason=length）。
func newResponseMeta(u openaiUsage, finishReason string) *schema.ResponseMeta {
	hasUsage := u.PromptTokens != 0 || u.CompletionTokens != 0 || u.TotalTokens != 0
	if !hasUsage && finishReason == "" {
		return nil
	}
	rm := &schema.ResponseMeta{FinishReason: finishReason}
	if hasUsage {
		rm.Usage = &schema.TokenUsage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		}
	}
	return rm
}

// Stream 流式生成
func (c *openaiClient) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// 构建请求
	reqBody := c.buildRequest(messages, opts, true)

	// 发送请求
	//nolint:bodyclose // 流式响应：body 交由下方读取 goroutine 的 defer resp.Body.Close() 在流生命周期结束时关闭；此处提前关闭会截断流。bodyclose 无法跨 goroutine 追踪该关闭。
	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After"), Body: string(body)}
	}

	// 创建流式读取器
	reader, writer := schema.Pipe[*schema.Message](10)

	go func() {
		defer writer.Close()
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// 单条 SSE data 行可能超过 bufio.Scanner 默认 64KB 上限（大 tool_call 参数分片 / 大 content delta），
		// 抬高到 1MB，避免 ErrTooLong 让流被下面的循环静默截断。
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		hasSentData := false
		var streamUsage *openaiUsage // 流末 usage chunk 携带的 token 用量（若服务端上报）
		var streamFinishReason string // 本轮结束原因（length=截断/tool_calls/stop），透出供上层判截断
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
					if rm := newResponseMeta(*streamUsage, streamFinishReason); rm != nil {
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
					streamFinishReason = *choice.FinishReason

					// 收敛待发的工具调用（若有）。
					var toolCalls []schema.ToolCall
					if len(pendingToolCalls) > 0 {
						toolCalls = make([]schema.ToolCall, 0, len(pendingToolCalls))
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
					}

					// 始终下发一条携带 finish_reason 的收尾消息：即便本轮没有工具调用（如 thinking 链
					// 撑爆输出预算导致 finish_reason=length、正文被半途截断），也要把结束原因透出到合并后的
					// 最终消息，供 ReAct 循环识别截断、避免把半句正文误当最终答案而让前端空轮询"卡住"。
					// ConcatMessages 取最后一个非空 FinishReason，故此消息即结束原因的可靠载体。
					hasSentData = true
					writer.Send(&schema.Message{
						Role:             schema.Assistant,
						Content:          "",
						ReasoningContent: reasoningBuilder.String(),
						ToolCalls:        toolCalls,
						ResponseMeta:     &schema.ResponseMeta{FinishReason: *choice.FinishReason},
					}, nil)
					pendingToolCalls = make(map[int]*schema.ToolCall)
					reasoningBuilder.Reset()
				}
			}
		}

		// 循环因读取错误提前结束（连接中断/超时/ErrTooLong）时，Scan() 返回 false 且并非 [DONE] 正常收尾；
		// 必须把错误显式下发给消费者，否则截断流会被当成"干净完整"的成功，resilience 装饰器也无法据此重试/降级。
		if err := scanner.Err(); err != nil {
			writer.Send(nil, fmt.Errorf("openai 流式读取中断: %w", err))
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
	} else {
		// 调用方未显式指定输出上限时兜一个宽松默认（而非依赖 provider 默认）。
		// DeepSeek thinking 模型的 reasoning_content 计入输出预算：重型 agent 首轮的长思维链
		// 会在 provider 较小的默认上限（常见 4096）内、于发出 tool_call 之前就把预算耗尽，
		// finish_reason=length 截断 → 上层把半句正文误当最终答案 → 前端空轮询"卡住"。
		// 给到 DeepSeek 单轮上限 8192，让 thinking + 正文 + tool_call 有足够空间落地。
		req.MaxTokens = defaultMaxOutputTokens
	}

	return req
}

// getOptions 从选项数组中提取 Options。
// 用 eino 官方 model.GetCommonOptions 真正 apply 传入的 opts（Temperature/TopP/MaxTokens 等），
// 原实现返回空 Options，导致上层设置的采样参数全被丢弃、从不生效。
func getOptions(opts []model.Option) *model.Options {
	return model.GetCommonOptions(&model.Options{}, opts...)
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
