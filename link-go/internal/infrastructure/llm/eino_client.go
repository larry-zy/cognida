// Package llm 提供 LLM 基础设施层实现。
//
// eino_client.go 把 eino 的 DSML 归一化 ToolCallingChatModel 适配成 domainllm.LLMClient，
// 作为 OpenAI 兼容 provider（OpenAI/Aliyun/DeepSeek/LKEAP/Qwen/Generic）基础对话路径的唯一
// 客户端实现，取代已弃用的 openaiChatRepo。相较后者，本客户端复用 chat 包 openaiClient，
// 因而透明获得：
//   - DeepSeek 系原生 <｜｜DSML｜｜tool_calls> 归一化（见 chat/dsml.go、chat/dsml_model.go），
//     绑定工具时不再把工具调用泄漏进正文；
//   - thinking 模式 reasoning_content 回传规则（needsReasoningRoundTrip）；
//   - 采样参数真正 apply（model.GetCommonOptions）与流末 usage 上报。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"link/internal/infrastructure/llm/chat"
	"link/internal/model/common"
	domainllm "link/internal/model/llm"
)

// einoLLMClient 将 eino model.ToolCallingChatModel 适配为 domainllm.LLMClient。
// model 字段持有的是 DSML 归一化装饰后的底层客户端（见 chat.NewToolCallingChatModel）。
type einoLLMClient struct {
	model     model.ToolCallingChatModel
	provider  domainllm.Provider
	modelName string
}

// NewEinoLLMClient 用 domainllm.ModelConfig 构造基于 eino 的 LLMClient。
// 仅做客户端装配，不发起网络 I/O；APIKey 缺失时立即报错（与旧 openaiChatRepo 一致）。
func NewEinoLLMClient(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("api_key is required for provider %s", config.Provider)
	}

	provider := config.Provider
	if provider == "" {
		provider = domainllm.ProviderOpenAI
	}

	chatCfg := &chat.ChatConfig{
		Source:    common.ModelSourceRemote,
		BaseURL:   config.BaseURL,
		ModelName: config.ModelName,
		APIKey:    config.APIKey,
		Provider:  string(provider),
	}

	// context.Background 仅用于构造期；NewToolCallingChatModel 内部不做网络调用，
	// 真正的请求上下文由 Chat/ChatStream 逐次传入。
	m, err := chat.NewToolCallingChatModel(context.Background(), chatCfg)
	if err != nil {
		return nil, err
	}

	return &einoLLMClient{
		model:     m,
		provider:  provider,
		modelName: config.ModelName,
	}, nil
}

// Chat 执行单次非流式聊天。
func (c *einoLLMClient) Chat(ctx context.Context, req *domainllm.ChatRequest) (*domainllm.ChatResponse, error) {
	m, msgs, opts, err := c.prepare(req)
	if err != nil {
		return nil, err
	}

	out, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, c.translateErr(err)
	}

	return c.toChatResponse(out), nil
}

// translateErr 把 eino 客户端的错误翻译成类型化 *domainllm.APIError，
// 让弹性装饰器（llm/resilience）能据 StatusCode/Class 精确决策重试/降级/熔断。
// 底层 chat.openaiClient 的 HTTP 状态错误经此还原为 429/5xx 的可重试分级，
// 传输/取消错误则按其性质（net→transient、ctx.Canceled→canceled）归类。
func (c *einoLLMClient) translateErr(err error) error {
	if err == nil {
		return nil
	}
	// 已是类型化 APIError（例如底层链路已构建）则原样沿用其判定。
	var apiErr *domainllm.APIError
	if errors.As(err, &apiErr) {
		return err
	}
	// 非 2xx HTTP 状态：依状态码 + Retry-After 头恢复分级。
	var httpErr *chat.HTTPStatusError
	if errors.As(err, &httpErr) {
		return apiErrorFromStatus(c.provider, c.modelName, httpErr.StatusCode, httpErr.RetryAfter, httpErr.Body)
	}
	// 传输层/取消/其它：net 错误→transient，ctx.Canceled→canceled，余→terminal。
	return apiErrorFromTransport(c.provider, c.modelName, err)
}

// ChatStream 执行流式聊天。返回的通道以一个 Done=true 的终止块收尾；
// 中途非 EOF 错误也以 Done=true（并在 Metadata["error"] 附因）收尾，
// 与 domainllm.ChatChunk 无独立错误通道的契约保持一致。
func (c *einoLLMClient) ChatStream(ctx context.Context, req *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error) {
	m, msgs, opts, err := c.prepare(req)
	if err != nil {
		return nil, err
	}

	reader, err := m.Stream(ctx, msgs, opts...)
	if err != nil {
		return nil, c.translateErr(err)
	}

	resultChan := make(chan *domainllm.ChatChunk, 16)
	go func() {
		defer close(resultChan)
		defer reader.Close()

		for {
			msg, recvErr := reader.Recv()
			if recvErr != nil {
				chunk := &domainllm.ChatChunk{Done: true}
				if !errors.Is(recvErr, io.EOF) {
					chunk.Metadata = map[string]interface{}{"error": recvErr.Error()}
				}
				select {
				case resultChan <- chunk:
				case <-ctx.Done():
				}
				return
			}

			chunk := &domainllm.ChatChunk{
				Content: msg.Content,
				Role:    string(msg.Role),
			}
			if len(msg.ToolCalls) > 0 {
				chunk.ToolCalls = fromSchemaToolCalls(msg.ToolCalls)
			}
			// 跳过既无内容也无工具调用的空心跳块（openaiClient 流末会先推 tool_calls 块再 EOF）。
			if chunk.Content == "" && len(chunk.ToolCalls) == 0 {
				continue
			}

			select {
			case resultChan <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return resultChan, nil
}

// GetModelInfo 返回模型基础信息。
func (c *einoLLMClient) GetModelInfo(ctx context.Context) (*domainllm.ModelInfo, error) {
	return &domainllm.ModelInfo{
		Name:     c.modelName,
		Provider: c.provider,
		Types:    []domainllm.ModelType{domainllm.ModelTypeChat},
		Features: []string{"chat", "stream", "tools"},
	}, nil
}

// SupportsTools 报告是否支持工具调用（OpenAI 兼容 provider 均支持）。
func (c *einoLLMClient) SupportsTools() bool { return true }

// SupportsStreaming 报告是否支持流式输出。
func (c *einoLLMClient) SupportsStreaming() bool { return true }

// prepare 把 domainllm 请求翻译为 eino 调用三元组：可能绑定工具后的模型、schema 消息、模型选项。
func (c *einoLLMClient) prepare(req *domainllm.ChatRequest) (model.ToolCallingChatModel, []*schema.Message, []model.Option, error) {
	// 仅拒绝 nil 请求；空消息列表按老 openaiChatRepo 的契约原样转发给底层客户端
	// （由服务端裁决），避免改变可观测行为、破坏弹性 E2E 探针。
	if req == nil {
		return nil, nil, nil, fmt.Errorf("nil chat request")
	}

	msgs := toSchemaMessages(req.Messages)
	m := c.model
	var opts []model.Option

	if req.Options != nil {
		opts = toModelOptions(req.Options)

		if len(req.Options.Tools) > 0 {
			toolInfos, err := toToolInfos(req.Options.Tools)
			if err != nil {
				return nil, nil, nil, err
			}
			bound, err := m.WithTools(toolInfos)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("bind tools failed: %w", err)
			}
			m = bound
		}
	}

	return m, msgs, opts, nil
}

// toChatResponse 把 eino 生成的 schema.Message 转成 domainllm.ChatResponse。
func (c *einoLLMClient) toChatResponse(msg *schema.Message) *domainllm.ChatResponse {
	toolCalls := fromSchemaToolCalls(msg.ToolCalls)

	resp := &domainllm.ChatResponse{
		Message: &domainllm.Message{
			Role:      string(msg.Role),
			Content:   msg.Content,
			ToolCalls: toolCalls,
		},
		Content:   msg.Content,
		ToolCalls: toolCalls,
		Thinking:  msg.ReasoningContent,
	}

	if msg.ResponseMeta != nil {
		resp.FinishReason = msg.ResponseMeta.FinishReason
		if u := msg.ResponseMeta.Usage; u != nil {
			resp.Usage = &domainllm.Usage{
				PromptTokens:     u.PromptTokens,
				CompletionTokens: u.CompletionTokens,
				TotalTokens:      u.TotalTokens,
			}
		}
	}

	return resp
}

// toSchemaMessages 把 domainllm 消息映射为 eino schema 消息（含 assistant 工具调用与 tool 结果）。
func toSchemaMessages(in []*domainllm.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(in))
	for _, m := range in {
		if m == nil {
			continue
		}
		sm := &schema.Message{
			Role:    schema.RoleType(m.Role),
			Content: m.Content,
			Name:    m.Name,
		}
		if m.Role == domainllm.RoleTool {
			sm.ToolCallID = m.ToolID
		}
		for _, tc := range m.ToolCalls {
			if tc == nil {
				continue
			}
			fn := schema.FunctionCall{}
			if tc.Function != nil {
				fn.Name = tc.Function.Name
				fn.Arguments = tc.Function.Arguments
			}
			sm.ToolCalls = append(sm.ToolCalls, schema.ToolCall{ID: tc.ID, Type: tc.Type, Function: fn})
		}
		out = append(out, sm)
	}
	return out
}

// fromSchemaToolCalls 把 eino 工具调用映射回 domainllm 形态。
func fromSchemaToolCalls(in []schema.ToolCall) []*domainllm.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]*domainllm.ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, &domainllm.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: &domainllm.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return out
}

// toModelOptions 把 domainllm.ChatOptions 中受支持的采样参数转成 eino model.Option。
func toModelOptions(o *domainllm.ChatOptions) []model.Option {
	var opts []model.Option
	if o.Temperature > 0 {
		opts = append(opts, model.WithTemperature(float32(o.Temperature)))
	}
	if o.TopP > 0 {
		opts = append(opts, model.WithTopP(float32(o.TopP)))
	}
	if o.MaxTokens > 0 {
		opts = append(opts, model.WithMaxTokens(o.MaxTokens))
	}
	if len(o.Stop) > 0 {
		opts = append(opts, model.WithStop(o.Stop))
	}
	return opts
}

// toToolInfos 把 domainllm.Tool 转成 eino schema.ToolInfo：
// Parameters（原始 JSON Schema map）经 marshal→unmarshal 灌入 *jsonschema.Schema，
// 再由 NewParamsOneOfByJSONSchema 包成 ParamsOneOf（openaiClient 序列化时会 ToJSONSchema 还原）。
func toToolInfos(tools []*domainllm.Tool) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		if t == nil || t.Function == nil {
			continue
		}
		info := &schema.ToolInfo{
			Name: t.Function.Name,
			Desc: t.Function.Description,
		}
		if len(t.Function.Parameters) > 0 {
			raw, err := json.Marshal(t.Function.Parameters)
			if err != nil {
				return nil, fmt.Errorf("marshal tool %q parameters failed: %w", t.Function.Name, err)
			}
			js := &jsonschema.Schema{}
			if err := json.Unmarshal(raw, js); err != nil {
				return nil, fmt.Errorf("parse tool %q parameters failed: %w", t.Function.Name, err)
			}
			info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
		}
		infos = append(infos, info)
	}
	return infos, nil
}
