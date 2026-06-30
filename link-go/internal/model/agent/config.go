// Package agent 提供代理(Agent)领域的配置定义
package agent

import (
	"fmt"

	"link/internal/model/agent/reflection"
)

// ========================================
// AgentConfig Agent配置
// ========================================

// AgentConfig Agent配置
type AgentConfig struct {
	// 基本配置
	MaxIterations        int
	EnableSmartRetrieval bool
	DefaultTopK          int

	// 查询配置
	EnableQueryRewrite bool
	EnableResultEval   bool
	MinConfidence      float64

	// 搜索配置
	SearchConfig *SearchConfig

	// Hook 配置
	HookConfig *HookConfig

	// Memory 配置
	MemoryConfig *MemoryConfig

	// Reflection 配置
	ReflectionConfig *reflection.ReflectionConfig

	// 扩展配置
	Extra map[string]interface{}
}

// DefaultAgentConfig 默认 Agent 配置
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		MaxIterations:        10,
		EnableSmartRetrieval: true,
		DefaultTopK:          5,
		EnableQueryRewrite:   true,
		EnableResultEval:     true,
		MinConfidence:        0.6,
		SearchConfig:         DefaultSearchConfig(),
		HookConfig:           DefaultHookConfig(),
		MemoryConfig:         DefaultMemoryConfig(),
	}
}

// Validate 验证配置
func (c *AgentConfig) Validate() error {
	if c.MaxIterations < 1 {
		return fmt.Errorf("max_iterations must be at least 1")
	}
	if c.DefaultTopK < 1 {
		return fmt.Errorf("default_top_k must be at least 1")
	}
	if c.MinConfidence < 0 || c.MinConfidence > 1 {
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}
	return nil
}

// ========================================
// HookConfig Hook 配置
// ========================================

// HookConfig Hook 配置
type HookConfig struct {
	// 结论生成配置
	EnableConclusion bool     `json:"enable_conclusion"`
	DataTools       []string `json:"data_tools"`       // 数据工具列表
	Timeout         int      `json:"timeout"`          // 超时时间（秒）

	// 意图澄清配置
	EnableClarification bool   `json:"enable_clarification"`
	BusinessContext     string `json:"business_context"` // 业务上下文
	MaxRounds           int    `json:"max_rounds"`       // 最大澄清轮次

	// 记忆和压缩配置
	EnableMemoryWrite   bool    `json:"enable_memory_write"`   // 启用记忆写入
	EnableAutoCompress   bool    `json:"enable_auto_compress"`   // 启用自动压缩
	EnableLongTermMemory bool    `json:"enable_long_term_memory"` // 启用长期记忆
	MemoryWriteTimeout  int     `json:"memory_write_timeout"`  // 记忆写入超时（秒）
	CompressThreshold   float64 `json:"compress_threshold"`    // 压缩阈值（默认 0.85）
}

// DefaultHookConfig 默认 Hook 配置
func DefaultHookConfig() *HookConfig {
	return &HookConfig{
		EnableConclusion:     false,
		EnableClarification:  false,
		EnableMemoryWrite:    true,
		EnableAutoCompress:   false,
		EnableLongTermMemory: false,
		MemoryWriteTimeout:   5,
		CompressThreshold:    0.85,
		MaxRounds:            3,
	}
}

// ========================================
// SearchConfig 搜索配置
// ========================================

// SearchConfig 搜索配置
type SearchConfig struct {
	Enabled     bool
	Engine      string
	APIKey      string
	MaxResults  int
	TimeRange   string
	SearchDepth int
}

// DefaultSearchConfig 默认搜索配置
func DefaultSearchConfig() *SearchConfig {
	return &SearchConfig{
		Enabled:    false,
		MaxResults: 10,
	}
}

// ========================================
// MemoryConfig Memory 配置
// ========================================

// MemoryConfig Memory 配置（Agent 级别）
type MemoryConfig struct {
	// 存储配置
	StorageBackend  string `json:"storage_backend"`  // "mysql", "redis", "hybrid"
	CacheIntermediate bool  `json:"cache_intermediate"` // 是否缓存中间过程消息
	CacheTTL        int    `json:"cache_ttl"`          // Redis 缓存 TTL（秒）

	// 长期记忆配置
	EnableLongTermMemory   bool   `json:"enable_long_term_memory"`
	LongTermMemoryCategory string `json:"long_term_memory_category"`

	// 上下文构建配置
	ContextConfig *ContextConfig `json:"context_config"`

	// 压缩配置
	CompressionConfig *CompressionConfig `json:"compression_config"`
}

// DefaultMemoryConfig 默认记忆配置
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		StorageBackend:      "mysql",
		CacheIntermediate:   false,
		CacheTTL:            3600,
		ContextConfig:       DefaultContextConfig(),
		CompressionConfig:   DefaultCompressionConfig(),
	}
}

// ========================================
// ContextConfig 上下文构建配置
// ========================================

// ContextConfig 上下文构建配置
type ContextConfig struct {
	MaxTokens           int                    `json:"max_tokens"`
	ReserveTokens       int                    `json:"reserve_tokens"` // 为用户响应保留的 tokens
	SystemPrompt        string                 `json:"system_prompt"`
	SystemPromptVersion string                `json:"system_prompt_version"`
	FewShotCategory     string                `json:"few_shot_category"`
	MaxFewShots         int                    `json:"max_few_shots"`
	IncludeIntermediate bool                   `json:"include_intermediate"`
	IncludeSummaries    bool                   `json:"include_summaries"`
	TemplateVariables   map[string]interface{} `json:"template_variables"`
}

// DefaultContextConfig 默认上下文配置
func DefaultContextConfig() *ContextConfig {
	return &ContextConfig{
		MaxTokens:       4000,
		ReserveTokens:   1000,
		MaxFewShots:     3,
	}
}

// ========================================
// CompressionConfig 压缩配置
// ========================================

// CompressionConfig 压缩配置
type CompressionConfig struct {
	MaxTokens          int     `json:"max_tokens"`
	SafetyMargin       int     `json:"safety_margin"`
	CompressThreshold  float64 `json:"compress_threshold"`
	Strategy           string  `json:"strategy"` // "sliding", "summary", "hybrid"
	KeepSystemMessages bool    `json:"keep_system_messages"`
	KeepRecentN        int     `json:"keep_recent_n"`
	SummaryModel       string  `json:"summary_model"`
	SummaryMaxTokens   int     `json:"summary_max_tokens"`
	OffloadThreshold   int     `json:"offload_threshold"`
	OffloadToStorage   bool    `json:"offload_to_storage"`
}

// DefaultCompressionConfig 默认压缩配置
func DefaultCompressionConfig() *CompressionConfig {
	return &CompressionConfig{
		MaxTokens:         2000,
		SafetyMargin:      500,
		CompressThreshold: 0.85,
		Strategy:          "sliding",
		KeepSystemMessages: true,
		KeepRecentN:       10,
	}
}

// ========================================
// AgenticRAGConfig AgenticRAG 配置
// ========================================

// AgenticRAGConfig AgenticRAG配置
type AgenticRAGConfig struct {
	Name                 string
	Description          string
	MaxIterations        int
	SearchConfig         *SearchConfig
	EnableSmartRetrieval bool
	DefaultTopK          int
	EnableQueryRewrite   bool
	EnableResultEval     bool
	MinConfidence        float64
}

// DefaultAgenticRAGConfig 默认配置
func DefaultAgenticRAGConfig() *AgenticRAGConfig {
	return &AgenticRAGConfig{
		Name:                 "AgenticRAG",
		Description:          "智能检索助手",
		MaxIterations:        8,
		EnableSmartRetrieval: true,
		DefaultTopK:          10,
		EnableQueryRewrite:   true,
		EnableResultEval:     true,
		MinConfidence:        0.6,
	}
}

// Validate 验证配置
func (c *AgenticRAGConfig) Validate() error {
	if c.MaxIterations < 1 {
		return fmt.Errorf("max_iterations must be at least 1")
	}
	if c.MinConfidence < 0 || c.MinConfidence > 1 {
		return fmt.Errorf("min_confidence must be between 0 and 1")
	}
	return nil
}

// ========================================
// MultiAgentConfig 多代理配置
// ========================================

// MultiAgentConfig 多代理配置
type MultiAgentConfig struct {
	CoordinatorModel     interface{}
	SearchConfig         *SearchConfig
	EnableSmartRetrieval bool
	MaxIterations        int
}

// ========================================
// DeepResearchConfig Deep Research 配置
// ========================================

// DeepResearchConfig Deep Research Agent配置
type DeepResearchConfig struct {
	Name                  string
	Description           string
	MaxSubQuestions       int
	SimpleQueryLimit      int
	MaxConcurrency        int
	MaxToolRounds         int
	EnableFactCheck       bool
	MaxFactsToValidate    int
	MinFactConfidence     float64
	EnableCrossValidation bool
	MinValidationSources  int
	ConsensusThreshold    float64
}

// DefaultDeepResearchConfig 默认配置
func DefaultDeepResearchConfig() *DeepResearchConfig {
	return &DeepResearchConfig{
		Name:                  "DeepResearch",
		Description:           "深度研究助手 - Plan → Execute → Synthesize",
		MaxSubQuestions:       5,
		SimpleQueryLimit:      2,
		MaxConcurrency:        3,
		MaxToolRounds:         5,
		EnableFactCheck:       true,
		MaxFactsToValidate:    5,
		MinFactConfidence:     0.7,
		EnableCrossValidation: true,
		MinValidationSources:  3,
		ConsensusThreshold:    0.6,
	}
}

// Validate 验证配置
func (c *DeepResearchConfig) Validate() error {
	if c.MaxSubQuestions < 1 {
		return fmt.Errorf("max_sub_questions must be at least 1")
	}
	if c.MinFactConfidence < 0 || c.MinFactConfidence > 1 {
		return fmt.Errorf("min_fact_confidence must be between 0 and 1")
	}
	return nil
}

