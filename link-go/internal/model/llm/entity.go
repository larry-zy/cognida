// Package llm 提供 LLM 领域实体定义
package llm

// "encoding/json"

// ========================================
// LLM 核心实体
// ========================================

// Provider LLM 提供商类型
type Provider string

const (
	ProviderOpenAI   Provider = "openai"
	ProviderOllama   Provider = "ollama"
	ProviderAliyun   Provider = "aliyun"
	ProviderDeepSeek Provider = "deepseek"
	ProviderLKEAP    Provider = "lkeap"
	ProviderQwen     Provider = "qwen"
	ProviderGeneric  Provider = "generic"
)

// ModelType 模型类型
type ModelType string

const (
	ModelTypeChat       ModelType = "chat"
	ModelTypeEmbedding  ModelType = "embedding"
	ModelTypeRerank     ModelType = "rerank"
	ModelTypeImage      ModelType = "image"
)

// Message 消息实体
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	// 可选字段
	Name      string                 `json:"name,omitempty"`
	ToolCalls []*ToolCall            `json:"tool_calls,omitempty"`
	ToolID    string                 `json:"tool_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Role 常量
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall 工具调用
type ToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function *FunctionCall     `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool 工具定义
type Tool struct {
	Type     string           `json:"type"`
	Function *FunctionSpec    `json:"function"`
}

// FunctionSpec 函数规格
type FunctionSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatOptions 聊天选项
type ChatOptions struct {
	Temperature     float64               `json:"temperature,omitempty"`
	MaxTokens       int                   `json:"max_tokens,omitempty"`
	TopP            float64               `json:"top_p,omitempty"`
	TopK            int                   `json:"top_k,omitempty"`
	FrequencyPenalty float64              `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64              `json:"presence_penalty,omitempty"`
	Stop            []string              `json:"stop,omitempty"`
	Stream          bool                  `json:"stream,omitempty"`
	Tools           []*Tool               `json:"tools,omitempty"`
	ToolChoice      interface{}           `json:"tool_choice,omitempty"`
	Extra           map[string]interface{} `json:"extra,omitempty"`
}

// ChatChunk 流式响应分块
type ChatChunk struct {
	Content    string                 `json:"content"`
	Done       bool                   `json:"done"`
	Role       string                 `json:"role,omitempty"`
	ToolCalls  []*ToolCall            `json:"tool_calls,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Message       *Message              `json:"message"`
	Content       string                `json:"content"`
	ToolCalls     []*ToolCall           `json:"tool_calls,omitempty"`
	Usage         *Usage                `json:"usage,omitempty"`
	Thinking      string                `json:"thinking,omitempty"`
	FinishReason  string                `json:"finish_reason,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// Usage 使用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Messages    []*Message     `json:"messages"`
	Options     *ChatOptions   `json:"options,omitempty"`
	Model       string         `json:"model,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
}

// ========================================
// Embedding 相关实体
// ========================================

// EmbeddingRequest 嵌入向量请求
type EmbeddingRequest struct {
	Texts   []string `json:"texts"`
	Model   string   `json:"model,omitempty"`
	Options *EmbeddingOptions `json:"options,omitempty"`
}

// EmbeddingOptions 嵌入向量选项
type EmbeddingOptions struct {
	Dimensions int      `json:"dimensions,omitempty"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

// EmbeddingResponse 嵌入向量响应
type EmbeddingResponse struct {
	Embeddings [][]float32            `json:"embeddings"`
	Usage      *Usage                 `json:"usage,omitempty"`
	Model      string                 `json:"model,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ========================================
// Rerank 相关实体
// ========================================

// RerankRequest 重排请求
type RerankRequest struct {
	Query   string   `json:"query"`
	Docs    []string `json:"docs"`
	Model   string   `json:"model,omitempty"`
	Options *RerankOptions `json:"options,omitempty"`
}

// RerankOptions 重排选项
type RerankOptions struct {
	TopK    int                    `json:"top_k,omitempty"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

// RerankResponse 重排响应
type RerankResponse struct {
	Results  []*RerankResult        `json:"results"`
	Model    string                 `json:"model,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// RerankResult 重排结果
type RerankResult struct {
	Index     int     `json:"index"`
	Score     float64 `json:"score"`
	Document  string  `json:"document,omitempty"`
}

// ========================================
// Model 配置实体
// ========================================

// ModelConfig 模型配置
type ModelConfig struct {
	ID          int64    `json:"id"`
	TenantID    int64    `json:"tenant_id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Provider    Provider `json:"provider"`
	ModelType   ModelType `json:"model_type"`
	APIKey      string   `json:"api_key,omitempty"`
	BaseURL     string   `json:"base_url,omitempty"`
	ModelName   string   `json:"model_name"`
	Enabled     bool     `json:"enabled"`
	IsDefault   bool     `json:"is_default"`
	Config      string   `json:"config,omitempty"` // JSON 配置
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// GetExtraConfig 获取额外配置
func (m *ModelConfig) GetExtraConfig() map[string]interface{} {
	// 简化实现，实际需要解析 JSON
	return make(map[string]interface{})
}
