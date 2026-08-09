// Package rag 提供 HyDE (Hypothetical Document Embeddings) 检索优化
package rag

import (
	"context"
)

// ========================================
// HyDE Generator 接口
// ========================================

// HyDEGenerator 假设文档生成器接口
// HyDE 通过 LLM 生成假设性答案，然后用该答案的向量进行检索，提高检索质量
type HyDEGenerator interface {
	// GenerateHypotheticalDoc 生成假设文档
	GenerateHypotheticalDoc(ctx context.Context, query string, opts *HyDEOptions) (string, error)

	// GenerateMultiple 生成多个假设文档（用于多样化检索）
	GenerateMultiple(ctx context.Context, query string, count int, opts *HyDEOptions) ([]string, error)
}

// HyDEOptions HyDE 配置选项
type HyDEOptions struct {
	// Temperature LLM 温度参数，控制生成多样性
	Temperature float64

	// MaxTokens 最大生成 token 数
	MaxTokens int

	// TaskType 任务类型，影响 prompt 生成
	TaskType string

	// Domain 知识域，用于生成更精准的假设文档
	Domain string

	// UseConversationHistory 是否使用对话历史
	UseConversationHistory bool

	// ConversationHistory 对话历史
	ConversationHistory []string
}

// DefaultHyDEOptions 默认 HyDE 配置
func DefaultHyDEOptions() *HyDEOptions {
	return &HyDEOptions{
		Temperature:            0.7,
		MaxTokens:              512,
		TaskType:               "qa",
		UseConversationHistory: false,
	}
}

// ========================================
// Query Rewriter 接口
// ========================================

// QueryRewriter 查询重写器接口
type QueryRewriter interface {
	// RewriteQuery 重写查询，提高检索质量
	RewriteQuery(ctx context.Context, query string, opts *RewriteOptions) (*RewrittenQuery, error)

	// ExpandQuery 扩展查询，生成多个变体
	ExpandQuery(ctx context.Context, query string, count int, opts *RewriteOptions) ([]string, error)

	// DecomposeQuery 分解复杂查询为多个子查询
	DecomposeQuery(ctx context.Context, query string, opts *RewriteOptions) ([]*SubQuery, error)
}

// RewriteOptions 重写配置选项
type RewriteOptions struct {
	// PreserveIntent 是否保留原始意图
	PreserveIntent bool

	// AddDomainKeywords 是否添加领域关键词
	AddDomainKeywords bool

	// Domain 知识域
	Domain string

	// ConversationHistory 对话历史（用于上下文重写）
	ConversationHistory []string

	// LastRetrieval 上次检索结果（用于查询扩展）
	LastRetrieval []string
}

// DefaultRewriteOptions 默认重写配置
func DefaultRewriteOptions() *RewriteOptions {
	return &RewriteOptions{
		PreserveIntent:      true,
		AddDomainKeywords:   false,
		ConversationHistory: nil,
	}
}

// RewrittenQuery 重写后的查询
type RewrittenQuery struct {
	// Original 原始查询
	Original string

	// Rewritten 重写后的查询
	Rewritten string

	// ExpansionQueries 扩展查询列表
	ExpansionQueries []string

	// Keywords 提取的关键词
	Keywords []string

	// Intent 查询意图
	Intent string

	// Metadata 元数据
	Metadata map[string]interface{}
}

// SubQuery 子查询（用于查询分解）
type SubQuery struct {
	// Query 子查询内容
	Query string

	// Priority 优先级
	Priority int

	// Dependencies 依赖的其他子查询索引
	Dependencies []int

	// Context 上下文信息
	Context string
}

// ========================================
// Multi-Hop Retriever 接口
// ========================================

// MultiHopRetriever 多跳检索器接口
// 用于需要多次推理才能回答的复杂查询
type MultiHopRetriever interface {
	// RetrieveMultiHop 执行多跳检索
	RetrieveMultiHop(ctx context.Context, query string, opts *MultiHopOptions) (*MultiHopResponse, error)
}

// MultiHopOptions 多跳检索配置
type MultiHopOptions struct {
	// MaxHops 最大跳数
	MaxHops int

	// HopStrategy 跳跃策略：sequential/parallel/tree
	HopStrategy string

	// ReasoningModel 推理模型（用于生成中间查询）
	ReasoningModel LLMChat

	// BaseRetriever 基础检索器
	BaseRetriever Retriever

	// EnableLog 是否启用推理日志
	EnableLog bool
}

// DefaultMultiHopOptions 默认多跳配置
func DefaultMultiHopOptions() *MultiHopOptions {
	return &MultiHopOptions{
		MaxHops:      3,
		HopStrategy:  "sequential",
		EnableLog:    true,
	}
}

// MultiHopResponse 多跳检索响应
type MultiHopResponse struct {
	// FinalAnswer 最终答案
	FinalAnswer string

	// Hops 每一跳的结果
	Hops []*HopResult

	// ReasoningChain 推理链
	ReasoningChain string

	// AllDocuments 所有检索到的文档
	AllDocuments []*Document

	// Metadata 元数据
	Metadata map[string]interface{}
}

// HopResult 单跳结果
type HopResult struct {
	// HopIndex 跳数索引（从 1 开始）
	HopIndex int

	// Query 该跳的查询
	Query string

	// IntermediateResult 中间结果
	IntermediateResult string

	// RetrievedDocs 该跳检索到的文档
	RetrievedDocs []*Document

	// NextQueryDecision 下一步查询决策
	NextQueryDecision string

	// Latency 延迟（毫秒）
	Latency int64
}

// ========================================
// 检索优化配置
// ========================================

// RetrievalOptimizationConfig 检索优化配置
type RetrievalOptimizationConfig struct {
	// HyDE HyDE 配置
	EnableHyDE   bool
	HyDEOptions  *HyDEOptions

	// QueryRewrite 查询重写配置
	EnableQueryRewrite bool
	RewriteOptions     *RewriteOptions

	// MultiHop 多跳检索配置
	EnableMultiHop bool
	MultiHopOptions *MultiHopOptions

	// FallbackStrategy 降级策略
	FallbackStrategy string
}

// DefaultRetrievalOptimizationConfig 默认检索优化配置
func DefaultRetrievalOptimizationConfig() *RetrievalOptimizationConfig {
	return &RetrievalOptimizationConfig{
		EnableHyDE:         false,
		HyDEOptions:        DefaultHyDEOptions(),
		EnableQueryRewrite: false,
		RewriteOptions:     DefaultRewriteOptions(),
		EnableMultiHop:     false,
		MultiHopOptions:    DefaultMultiHopOptions(),
		FallbackStrategy:   "vector",
	}
}
