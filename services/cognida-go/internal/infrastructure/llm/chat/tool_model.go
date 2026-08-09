// Package chat 提供 ToolCallingChatModel 的适配器
package chat

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// toolCallingModelAdapter 将 openaiClient 适配为 ToolCallingChatModel
type toolCallingModelAdapter struct {
	client *openaiClient
}

// NewToolCallingChatModel 创建支持工具调用的 ChatModel
func NewToolCallingChatModel(ctx context.Context, config *ChatConfig) (model.ToolCallingChatModel, error) {
	client, err := newOpenAIClient(config)
	if err != nil {
		return nil, err
	}

	// 用 DSML 归一化装饰器透明包裹：把 DeepSeek 系模型内联在正文里的原生工具调用
	// 归一化回结构化 tool_calls（见 dsml_model.go / dsml.go）。与 provider 无关，正常路径零改动透传。
	return NewDSMLNormalizingModel(&toolCallingModelAdapter{client: client}), nil
}

// Generate 实现 BaseChatModel 接口
func (m *toolCallingModelAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.client.Generate(ctx, messages, opts...)
}

// Stream 实现 BaseChatModel 接口
func (m *toolCallingModelAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.client.Stream(ctx, messages, opts...)
}

// WithTools 实现 ToolCallingChatModel 接口
// 返回绑定工具后的新实例
func (m *toolCallingModelAdapter) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	// 创建新实例以支持并发安全
	newClient := &openaiClient{
		baseURL:            m.client.baseURL,
		apiKey:             m.client.apiKey,
		model:              m.client.model,
		client:             m.client.client,
		toolInfos:          tools,
		roundTripReasoning: m.client.roundTripReasoning,
	}

	return &toolCallingModelAdapter{client: newClient}, nil
}

// NewChatModel 创建仅需 Generate 能力的聊天模型（HyDE / Guardrail 等消费方）。
// 返回 BaseChatModel：底层实现（ToolCallingChatModel）天然满足该接口，
// 无需再包一层已弃用的 model.ChatModel（BindTools）适配器。
func NewChatModel(ctx context.Context, config *ChatConfig) (model.BaseChatModel, error) {
	return NewToolCallingChatModel(ctx, config)
}
