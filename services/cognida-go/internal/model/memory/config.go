// Package memory 配置值对象
package memory

import "time"

// ========================================
// MemoryConfig 内存配置
// ========================================

// MemoryConfig 内存配置
type MemoryConfig struct {
	// 存储配置
	StorageBackend      string // "mysql", "redis", "hybrid"
	CacheIntermediate   bool   // 是否缓存中间过程消息
	CacheTTL            int    // Redis 缓存 TTL（秒）

	// 长期记忆配置
	EnableLongTermMemory bool
	LongTermMemoryCategory string

	// 默认值
}

// DefaultMemoryConfig 默认内存配置
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		StorageBackend:      "hybrid",
		CacheIntermediate:   false,
		CacheTTL:            86400, // 24小时
		EnableLongTermMemory: true,
	}
}

// ========================================
// CompressionConfig 压缩配置
// ========================================

// CompressionStrategy 压缩策略
type CompressionStrategy string

const (
	CompressionStrategyNone     CompressionStrategy = "none"
	CompressionStrategySliding  CompressionStrategy = "sliding"
	CompressionStrategySummary  CompressionStrategy = "summary"
	CompressionStrategyHybrid   CompressionStrategy = "hybrid"
)

// CompressionConfig 压缩配置
type CompressionConfig struct {
	// Token 限制
	MaxTokens          int
	SafetyMargin       int    // 安全边距（tokens）
	CompressThreshold  float64 // 触发压缩的阈值（0-1，如 0.8 表示 80%）

	// 策略配置
	Strategy           CompressionStrategy
	KeepSystemMessages bool
	KeepRecentN        int // 保留最近 N 条消息

	// 摘要配置
	SummaryModel       string // 用于生成摘要的模型
	SummaryMaxTokens   int

	// Offloading 配置
	OffloadThreshold   int    // 超过此 token 数自动卸载
	OffloadToStorage   bool

	// 默认值
}

// DefaultCompressionConfig 默认压缩配置
func DefaultCompressionConfig() *CompressionConfig {
	return &CompressionConfig{
		MaxTokens:          4000,
		SafetyMargin:       500,
		CompressThreshold:  0.85, // 85%
		Strategy:           CompressionStrategyHybrid,
		KeepSystemMessages: true,
		KeepRecentN:        5,
		SummaryModel:       "gpt-4o-mini",
		SummaryMaxTokens:   500,
		OffloadThreshold:   20000,
		OffloadToStorage:   true,
	}
}

// GetActualThreshold 获取实际触发压缩的 token 数量
func (c *CompressionConfig) GetActualThreshold() int {
	return int(float64(c.MaxTokens) * c.CompressThreshold)
}

// ShouldCompress 判断是否需要压缩
func (c *CompressionConfig) ShouldCompress(currentTokens int) bool {
	return currentTokens >= c.GetActualThreshold()
}

// ========================================
// ContextBuilderConfig 上下文构建配置
// ========================================

// ContextBuilderConfig 上下文构建配置
type ContextBuilderConfig struct {
	// 模板配置
	TemplateID         string
	SystemPrompt       string
	SystemPromptVersion string

	// Few-shot 配置
	FewShotCategory    string
	MaxFewShots        int

	// Token 预算
	MaxTokens          int
	ReserveTokens      int // 为用户响应保留的 tokens

	// 消息过滤
	IncludeIntermediate bool // 是否包含中间过程消息
	IncludeSummaries    bool // 是否包含摘要

	// 模板变量
	TemplateVariables  map[string]interface{}

	// 默认值
}

// DefaultContextBuilderConfig 默认上下文构建配置
func DefaultContextBuilderConfig() *ContextBuilderConfig {
	return &ContextBuilderConfig{
		SystemPrompt:       "你是一个有用的 AI 助手。",
		MaxFewShots:        3,
		MaxTokens:          4000,
		ReserveTokens:      1000,
		IncludeIntermediate: false,
		IncludeSummaries:   true,
		TemplateVariables:  make(map[string]interface{}),
	}
}

// GetAvailableTokens 获取可用的 token 数量
func (c *ContextBuilderConfig) GetAvailableTokens() int {
	return c.MaxTokens - c.ReserveTokens
}

// ========================================
// RecallOptions 检索选项
// ========================================

// RecallOptions 长期记忆检索选项
type RecallOptions struct {
	MaxResults   int
	MinImportance float64

	// 评分权重
	SimilarityWeight float64
	RecencyWeight    float64
	ImportanceWeight float64

	// 过滤
	Category    string
	UserID      *int64
	BeforeTime  *time.Time
	AfterTime   *time.Time

	// 默认值
}

// DefaultRecallOptions 默认检索选项
func DefaultRecallOptions() *RecallOptions {
	return &RecallOptions{
		MaxResults:        10,
		MinImportance:     0.3,
		SimilarityWeight:  0.5,
		RecencyWeight:     0.3,
		ImportanceWeight:  0.2,
	}
}

// CalculateScore 计算综合评分
func (o *RecallOptions) CalculateScore(similarity, recency, importance float64) float64 {
	return (similarity * o.SimilarityWeight) +
		(recency * o.RecencyWeight) +
		(importance * o.ImportanceWeight)
}

// ========================================
// CollaborationContextMode Agent 协作上下文模式
// ========================================

// CollaborationContextMode Agent 协作上下文模式
type CollaborationContextMode string

const (
	ContextModeNone     CollaborationContextMode = "none"      // 无上下文
	ContextModeSummary  CollaborationContextMode = "summary"   // 传递摘要
	ContextModeRecent   CollaborationContextMode = "recent"    // 最近 N 条
	ContextModeFull     CollaborationContextMode = "full"      // 完整历史
	ContextModeIsolated CollaborationContextMode = "isolated"  // 完全隔离（独立会话）
)

// CollaborationContextConfig Agent 协作上下文配置
type CollaborationContextConfig struct {
	Mode         CollaborationContextMode
	ContextLimit int       // recent 模式下传递的消息数量
	SessionID    string    // 关联的会话 ID

	// 输出控制
	OutputFormat string   // "summary", "full", "structured"
	MaxOutputLength int

	// 默认值
}

// DefaultCollaborationContextConfig 默认协作上下文配置
func DefaultCollaborationContextConfig() *CollaborationContextConfig {
	return &CollaborationContextConfig{
		Mode:          ContextModeNone,
		ContextLimit:  10,
		OutputFormat:  "summary",
		MaxOutputLength: 2000,
	}
}
