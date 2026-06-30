// Package llm 提供 LLM 基础设施层的工厂实现
package llm

import (
	"fmt"

	domainllm "link/internal/model/llm"
)

// ========================================
// Model Factory
// ========================================

// modelFactory 模型工厂实现
type modelFactory struct {
	creators map[domainllm.Provider]domainllm.ModelCreator
}

// NewModelFactory 创建模型工厂
func NewModelFactory() domainllm.ModelFactory {
	factory := &modelFactory{
		creators: make(map[domainllm.Provider]domainllm.ModelCreator),
	}

	// 注册默认提供商
	factory.registerDefaultProviders()

	return factory
}

// registerDefaultProviders 注册默认提供商
func (f *modelFactory) registerDefaultProviders() {
	// OpenAI 兼容的提供商
	for _, provider := range []domainllm.Provider{
		domainllm.ProviderOpenAI,
		domainllm.ProviderAliyun,
		domainllm.ProviderDeepSeek,
		domainllm.ProviderLKEAP,
		domainllm.ProviderQwen,
		domainllm.ProviderGeneric,
	} {
		f.RegisterProvider(provider, func(config *domainllm.ModelConfig) (interface{}, error) {
			return NewOpenAIChatRepo(config)
		})
	}

	// Ollama
	f.RegisterProvider(domainllm.ProviderOllama, func(config *domainllm.ModelConfig) (interface{}, error) {
		return NewOllamaChatRepo(config)
	})
}

// CreateChatModel 创建聊天模型客户端
func (f *modelFactory) CreateChatModel(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
	creator, ok := f.creators[config.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider: %s", config.Provider)
	}

	model, err := creator(config)
	if err != nil {
		return nil, err
	}

	llmClient, ok := model.(domainllm.LLMClient)
	if !ok {
		return nil, fmt.Errorf("created model is not a LLMClient")
	}

	return llmClient, nil
}

// CreateEmbeddingModel 创建嵌入向量模型
func (f *modelFactory) CreateEmbeddingModel(config *domainllm.ModelConfig) (domainllm.EmbeddingRepository, error) {
	// TODO: 实现嵌入向量模型创建
	return nil, fmt.Errorf("embedding model not implemented yet")
}

// CreateRerankModel 创建重排模型
func (f *modelFactory) CreateRerankModel(config *domainllm.ModelConfig) (domainllm.RerankRepository, error) {
	// TODO: 实现重排模型创建
	return nil, fmt.Errorf("rerank model not implemented yet")
}

// RegisterProvider 注册提供商
func (f *modelFactory) RegisterProvider(provider domainllm.Provider, creator domainllm.ModelCreator) {
	f.creators[provider] = creator
}

// GetRegisteredProviders 获取已注册的提供商
func (f *modelFactory) GetRegisteredProviders() []domainllm.Provider {
	providers := make([]domainllm.Provider, 0, len(f.creators))
	for provider := range f.creators {
		providers = append(providers, provider)
	}
	return providers
}
