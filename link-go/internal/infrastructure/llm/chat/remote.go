package chat

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
)

// ========================================
// Remote Chat Implementation
// ========================================

// remoteChat 远程聊天实现
type remoteChat struct {
	base    *baseChat
	client  *openaiClient
	creator modelCreator
}

// modelCreator 模型创建器接口
type modelCreator interface {
	createClient(config *ChatConfig) (*openaiClient, error)
}

// NewRemoteChat 创建远程聊天实例
func NewRemoteChat(config *ChatConfig) (Chat, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required for remote chat")
	}
	if config.ModelName == "" {
		return nil, fmt.Errorf("model_name is required for remote chat")
	}

	provider := config.Provider
	if provider == "" {
		provider = DetectProvider(config.BaseURL)
	}

	creator, err := getRemoteModelCreator(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get model creator: %w", err)
	}

	client, err := creator.createClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &remoteChat{
		base: &baseChat{
			config:  config,
			modelID: config.ModelID,
		},
		client:  client,
		creator: creator,
	}, nil
}

// Chat 进行非流式聊天
func (c *remoteChat) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*ChatResponse, error) {
	einoMessages := c.base.convertMessages(messages)
	modelOpts := c.base.convertOptions(opts)

	resp, err := c.client.Generate(ctx, einoMessages, modelOpts...)
	if err != nil {
		return nil, fmt.Errorf("remote chat failed: %w", err)
	}

	return c.base.convertResponse(resp), nil
}

// ChatStream 进行流式聊天
func (c *remoteChat) ChatStream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan StreamResponse, error) {
	einoMessages := c.base.convertMessages(messages)
	modelOpts := c.base.convertOptions(opts)

	streamReader, err := c.client.Stream(ctx, einoMessages, modelOpts...)
	if err != nil {
		return nil, fmt.Errorf("remote stream chat failed: %w", err)
	}

	return recvStream(ctx, streamReader)
}

// GetModelName 获取模型名称
func (c *remoteChat) GetModelName() string {
	return c.base.config.ModelName
}

// GetModelID 获取模型ID
func (c *remoteChat) GetModelID() string {
	return c.base.modelID
}

// ========================================
// Model Creators
// ========================================

// getRemoteModelCreator 获取远程模型创建器
func getRemoteModelCreator(provider string) (modelCreator, error) {
	switch provider {
	case ProviderOpenAI:
		return &openaiCreator{}, nil
	case ProviderAliyun:
		return &aliyunCreator{}, nil
	case ProviderDeepSeek:
		return &deepSeekCreator{}, nil
	case ProviderLKEAP:
		return &lkeapCreator{}, nil
	case ProviderQwen:
		return &qwenCreator{}, nil
	case ProviderGeneric:
		return &genericCreator{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// ========================================
// OpenAI Creator
// ========================================

type openaiCreator struct{}

func (c *openaiCreator) createClient(config *ChatConfig) (*openaiClient, error) {
	return newOpenAIClient(config)
}

// ========================================
// Aliyun Creator
// ========================================

type aliyunCreator struct{}

func (c *aliyunCreator) createClient(config *ChatConfig) (*openaiClient, error) {
	// Aliyun使用OpenAI兼容API
	return newOpenAIClient(config)
}

// ========================================
// DeepSeek Creator
// ========================================

type deepSeekCreator struct{}

func (c *deepSeekCreator) createClient(config *ChatConfig) (*openaiClient, error) {
	// DeepSeek使用OpenAI兼容API
	return newOpenAIClient(config)
}

// ========================================
// LKEAP Creator
// ========================================

type lkeapCreator struct{}

func (c *lkeapCreator) createClient(config *ChatConfig) (*openaiClient, error) {
	// LKEAP使用OpenAI兼容API
	return newOpenAIClient(config)
}

// ========================================
// Qwen Creator
// ========================================

type qwenCreator struct{}

func (c *qwenCreator) createClient(config *ChatConfig) (*openaiClient, error) {
	// Qwen使用OpenAI兼容API
	return newOpenAIClient(config)
}

// ========================================
// Generic Creator (vLLM等)
// ========================================

type genericCreator struct{}

func (c *genericCreator) createClient(config *ChatConfig) (*openaiClient, error) {
	// Generic provider使用OpenAI兼容API
	return newOpenAIClient(config)
}

// ========================================
// Legacy Functions (for backward compatibility)
// ========================================

// createRemoteModel 创建远程模型（已弃用，保留用于兼容）
func createRemoteModel(config *ChatConfig, provider string) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}

// createOpenAIModel 创建OpenAI模型（已弃用）
func createOpenAIModel(config *ChatConfig) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}

// createAliyunModel 创建Aliyun模型（已弃用）
func createAliyunModel(config *ChatConfig) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}

// createDeepSeekModel 创建DeepSeek模型（已弃用）
func createDeepSeekModel(config *ChatConfig) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}

// createLKEAPModel 创建LKEAP模型（已弃用）
func createLKEAPModel(config *ChatConfig) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}

// createQwenModel 创建Qwen模型（已弃用）
func createQwenModel(config *ChatConfig) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}

// createGenericModel 创建Generic模型（已弃用）
func createGenericModel(config *ChatConfig) (model.BaseChatModel, error) {
	return nil, fmt.Errorf("deprecated: use NewRemoteChat instead")
}
