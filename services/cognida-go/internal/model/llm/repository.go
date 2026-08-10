// Package llm 提供 LLM 领域仓储接口定义
package llm

import (
	"context"
	"io"
)

// ========================================
// LLM 客户端接口
// ========================================

// LLMClient LLM 客户端接口（用于调用外部 LLM 服务）
// 注意：这是外部服务客户端，不是数据持久化的仓储
type LLMClient interface {
	// Chat 执行单次聊天请求
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ChatStream 执行流式聊天请求
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan *ChatChunk, error)

	// GetModelInfo 获取模型信息
	GetModelInfo(ctx context.Context) (*ModelInfo, error)

	// SupportsTools 检查是否支持工具调用
	SupportsTools() bool

	// SupportsStreaming 检查是否支持流式输出
	SupportsStreaming() bool
}

// ChatRepository 聊天仓储接口（用于数据持久化）
// @Deprecated: 使用 LLMClient 表示外部服务客户端，此接口保留用于持久化场景
type ChatRepository = LLMClient

// ========================================
// Embedding 仓储接口
// ========================================

// EmbeddingRepository 嵌入向量仓储接口
type EmbeddingRepository interface {
	// Embed 生成文本嵌入向量
	Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)

	// EmbedBatch 批量生成嵌入向量
	EmbedBatch(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)

	// GetDimension 获取向量维度
	GetDimension(ctx context.Context) (int, error)
}

// ========================================
// Rerank 仓储接口
// ========================================

// RerankRepository 重排仓储接口
type RerankRepository interface {
	// Rerank 重排文档列表
	Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error)

	// GetMaxDocs 获取支持的最大文档数
	GetMaxDocs() int
}

// ========================================
// Model 配置仓储接口
// ========================================

// ModelRepository 模型配置仓储接口
type ModelRepository interface {
	// Create 创建模型配置
	Create(ctx context.Context, config *ModelConfig) error

	// Update 更新模型配置
	Update(ctx context.Context, config *ModelConfig) error

	// Delete 删除模型配置
	Delete(ctx context.Context, id int64) error

	// GetByID 根据 ID 获取模型配置
	GetByID(ctx context.Context, id int64) (*ModelConfig, error)

	// GetByTenant 获取租户的所有模型配置
	GetByTenant(ctx context.Context, tenantID int64) ([]*ModelConfig, error)

	// GetByType 根据类型获取模型配置
	GetByType(ctx context.Context, tenantID int64, modelType ModelType) ([]*ModelConfig, error)

	// GetDefault 获取默认模型配置
	GetDefault(ctx context.Context, tenantID int64, modelType ModelType) (*ModelConfig, error)

	// List 列出模型配置
	List(ctx context.Context, tenantID int64, offset, limit int) ([]*ModelConfig, int64, error)

	// ListByCursor 游标（keyset）分页〔M5〕：按 created_at DESC, id DESC 稳定排序，
	// 以 cursor 为锚点取下一页；modelType 为空表示不过滤类型，enabled 为 nil 表示不过滤启用状态。
	// 返回本页配置与「下一页游标」；nextCursor 为空表示已到末页。不返回总数。
	ListByCursor(ctx context.Context, tenantID int64, modelType string, enabled *bool, cursor string, limit int) (configs []*ModelConfig, nextCursor string, err error)
}

// ========================================
// Model 工厂接口
// ========================================

// ModelFactory 模型工厂接口
type ModelFactory interface {
	// CreateChatModel 创建聊天模型客户端
	CreateChatModel(config *ModelConfig) (LLMClient, error)

	// CreateEmbeddingModel 创建嵌入向量模型
	CreateEmbeddingModel(config *ModelConfig) (EmbeddingRepository, error)

	// CreateRerankModel 创建重排模型
	CreateRerankModel(config *ModelConfig) (RerankRepository, error)

	// RegisterProvider 注册提供商
	RegisterProvider(provider Provider, creator ModelCreator)

	// GetRegisteredProviders 获取已注册的提供商
	GetRegisteredProviders() []Provider
}

// ModelCreator 模型创建器函数类型
type ModelCreator func(config *ModelConfig) (interface{}, error)

// ========================================
// 通用接口
// ========================================

// ModelInfo 模型信息
type ModelInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Provider    Provider `json:"provider"`
	Types       []ModelType `json:"types"`
	Features    []string `json:"features"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
}

// ModelRegistry 模型注册表接口
type ModelRegistry interface {
	// Register 注册模型
	Register(model *ModelInfo) error

	// Get 获取模型信息
	Get(name string) (*ModelInfo, error)

	// ListByType 按类型列出模型
	ListByType(modelType ModelType) ([]*ModelInfo, error)

	// ListAll 列出所有模型
	ListAll() ([]*ModelInfo, error)
}

// StreamingWriter 流式写入器接口
type StreamingWriter interface {
	// WriteChunk 写入分块
	WriteChunk(chunk *ChatChunk) error

	// Close 关闭写入器
	Close() error
}

// ========================================
// 配置加载器接口
// ========================================

// ConfigLoader 配置加载器接口
type ConfigLoader interface {
	// LoadFromDB 从数据库加载配置
	LoadFromDB(ctx context.Context, tenantID int64) error

	// LoadFromFile 从文件加载配置
	LoadFromFile(path string) error

	// LoadFromEnv 从环境变量加载配置
	LoadFromEnv() error

	// GetConfig 获取配置
	GetConfig(name string) (*ModelConfig, error)

	// GetAllConfigs 获取所有配置
	GetAllConfigs() []*ModelConfig
}

// ========================================
// 监控接口
// ========================================

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
	// RecordRequest 记录请求
	RecordRequest(modelName, requestType string)

	// RecordResponse 记录响应
	RecordResponse(modelName, requestType string, tokenCount int, duration int64)

	// RecordError 记录错误
	RecordError(modelName, requestType, errorType string)

	// GetMetrics 获取指标
	GetMetrics(modelName string) *ModelMetrics
}

// ModelMetrics 模型指标
type ModelMetrics struct {
	TotalRequests    int64   `json:"total_requests"`
	TotalErrors      int64   `json:"total_errors"`
	TotalTokens      int64   `json:"total_tokens"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	P95LatencyMs     float64 `json:"p95_latency_ms"`
	P99LatencyMs     float64 `json:"p99_latency_ms"`
}

// LLMLogger LLM 日志接口
type LLMLogger interface {
	LogRequest(ctx context.Context, req *ChatRequest)
	LogResponse(ctx context.Context, resp *ChatResponse, duration int64)
	LogError(ctx context.Context, req *ChatRequest, err error)
}

// LLMTracer LLM 追踪接口
type LLMTracer interface {
	// StartSpan 开始追踪
	StartSpan(ctx context.Context, name string) (context.Context, io.Closer)

	// AddEvent 添加事件
	AddEvent(ctx context.Context, name string, attrs map[string]interface{})
}
