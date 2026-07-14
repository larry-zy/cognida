// Package llm 提供 LLM 基础设施层的工厂实现
package llm

import (
	"fmt"

	"link/internal/infrastructure/llm/resilience"
	domainllm "link/internal/model/llm"
)

// ========================================
// Model Factory
// ========================================

// modelFactory 模型工厂实现
type modelFactory struct {
	creators   map[domainllm.Provider]domainllm.ModelCreator
	resilience domainllm.ResilienceConfig
}

// Option 配置模型工厂。
type Option func(*modelFactory)

// WithResilience 覆盖工厂的弹性配置。
func WithResilience(cfg domainllm.ResilienceConfig) Option {
	return func(f *modelFactory) { f.resilience = cfg.WithDefaults() }
}

// WithoutResilience 关闭弹性包装（直连底层客户端）。
func WithoutResilience() Option {
	return func(f *modelFactory) { f.resilience.Enabled = false }
}

// NewModelFactory 创建模型工厂。默认启用统一弹性（重试+熔断+降级），
// 对上层透明；可用 WithResilience/WithoutResilience 调整。
func NewModelFactory(opts ...Option) domainllm.ModelFactory {
	factory := &modelFactory{
		creators:   make(map[domainllm.Provider]domainllm.ModelCreator),
		resilience: ResilienceConfigFromEnv(),
	}

	// 注册默认提供商
	factory.registerDefaultProviders()

	for _, opt := range opts {
		opt(factory)
	}

	return factory
}

// NewModelFactoryWithObservability 创建模型工厂，并为弹性层安装进程级默认观测器
// （指标 + 结构化日志）。组合根（wire）用它替代裸 NewModelFactory 以获得可观测性。
func NewModelFactoryWithObservability(opts ...Option) domainllm.ModelFactory {
	resilience.SetObserver(resilience.NewCompositeObserver(
		resilience.NewMetricsObserver(),
		resilience.NewLoggingObserver(nil),
	))
	return NewModelFactory(opts...)
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

// createRawChatModel 构建未包装的底层 chat 客户端。
func (f *modelFactory) createRawChatModel(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
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

// CreateChatModel 创建聊天模型客户端（默认包装单目标弹性）。
func (f *modelFactory) CreateChatModel(config *domainllm.ModelConfig) (domainllm.LLMClient, error) {
	client, err := f.createRawChatModel(config)
	if err != nil {
		return nil, err
	}
	if !f.resilience.Enabled {
		return client, nil
	}
	return resilience.NewResilientClient(f.resilience, nil,
		resilience.ChatTarget{Provider: config.Provider, Model: config.ModelName, Client: client}), nil
}

// CreateChatModelChain 按有序配置装配一条 chat 弹性链（configs[0] 为主，其余为后备）。
// 弹性关闭或仅一个配置时退化为单目标行为。
func (f *modelFactory) CreateChatModelChain(configs []*domainllm.ModelConfig) (domainllm.LLMClient, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("empty model chain")
	}
	if !f.resilience.Enabled || len(configs) == 1 {
		return f.CreateChatModel(configs[0])
	}
	targets := make([]resilience.ChatTarget, 0, len(configs))
	for _, cfg := range configs {
		client, err := f.createRawChatModel(cfg)
		if err != nil {
			return nil, err
		}
		targets = append(targets, resilience.ChatTarget{Provider: cfg.Provider, Model: cfg.ModelName, Client: client})
	}
	return resilience.NewResilientClient(f.resilience, nil, targets...), nil
}

// CreateEmbeddingModel 创建嵌入向量模型（默认包装单目标弹性）。
func (f *modelFactory) CreateEmbeddingModel(config *domainllm.ModelConfig) (domainllm.EmbeddingRepository, error) {
	repo, err := newEmbeddingRepo(config)
	if err != nil {
		return nil, err
	}
	if !f.resilience.Enabled {
		return repo, nil
	}
	return resilience.NewResilientEmbedding(f.resilience, nil,
		resilience.EmbeddingTarget{Provider: config.Provider, Model: config.ModelName, Client: repo}), nil
}

// CreateRerankModel 创建重排模型（默认包装单目标弹性）。
func (f *modelFactory) CreateRerankModel(config *domainllm.ModelConfig) (domainllm.RerankRepository, error) {
	repo, err := newRerankRepo(config)
	if err != nil {
		return nil, err
	}
	if !f.resilience.Enabled {
		return repo, nil
	}
	return resilience.NewResilientRerank(f.resilience, nil,
		resilience.RerankTarget{Provider: config.Provider, Model: config.ModelName, Client: repo}), nil
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
