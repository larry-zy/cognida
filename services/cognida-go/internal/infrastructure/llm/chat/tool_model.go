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

// chatModelAdapter 将 BaseChatModel 适配为已弃用的 model.ChatModel 接口，
// 供 HyDE / Guardrail 等仅需 Generate 能力的组件使用。
type chatModelAdapter struct {
	model.BaseChatModel
	tools []*schema.ToolInfo
}

// BindTools 实现 model.ChatModel 接口。
// 当前消费方（HyDE 生成、查询改写、内容护栏）仅使用 Generate 能力，
// 这里保存工具信息以满足接口契约。
func (m *chatModelAdapter) BindTools(tools []*schema.ToolInfo) error {
	m.tools = tools
	return nil
}

// NewChatModel 创建兼容已弃用 model.ChatModel 接口的聊天模型。
func NewChatModel(ctx context.Context, config *ChatConfig) (model.ChatModel, error) {
	base, err := NewToolCallingChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return &chatModelAdapter{BaseChatModel: base}, nil
}
