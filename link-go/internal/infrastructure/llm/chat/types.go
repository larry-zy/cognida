package chat

import (
	"encoding/json"

	"link/internal/model/common"
)

// ========================================
// 聊天消息类型
// ========================================

// Message 表示聊天消息
type Message struct {
	Role       string     `json:"role"`                   // 角色：system, user, assistant, tool
	Content    string     `json:"content"`                // 消息内容
	Name       string     `json:"name,omitempty"`         // Function/tool name (for tool role)
	ToolCallID string     `json:"tool_call_id,omitempty"` // Tool call ID (for tool role)
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // Tool calls (for assistant role)
}

// ToolCall represents a tool call in a message
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON
}

// ========================================
// 聊天选项
// ========================================

// ChatOptions 聊天选项
type ChatOptions struct {
	Temperature         float64         `json:"temperature"`           // 温度参数
	TopP                float64         `json:"top_p"`                 // Top P 参数
	Seed                int             `json:"seed"`                  // 随机种子
	MaxTokens           int             `json:"max_tokens"`            // 最大 token 数
	MaxCompletionTokens int             `json:"max_completion_tokens"` // 最大完成 token 数
	FrequencyPenalty    float64         `json:"frequency_penalty"`     // 频率惩罚
	PresencePenalty     float64         `json:"presence_penalty"`      // 存在惩罚
	Thinking            *bool           `json:"thinking"`              // 是否启用思考
	Tools               []Tool          `json:"tools,omitempty"`       // 可用工具列表
	ToolChoice          string          `json:"tool_choice,omitempty"` // "auto", "required", "none", or specific tool
	Format              json.RawMessage `json:"format,omitempty"`      // 响应格式定义
}

// Tool represents a function/tool definition
type Tool struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef represents a function definition
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ========================================
// 聊天配置
// ========================================

// ChatConfig 聊天配置
type ChatConfig struct {
	Source    common.ModelSource // 模型源：local/remote
	BaseURL   string            `json:"base_url"`   // API Base URL
	ModelName string            `json:"model_name"` // 模型名称
	APIKey    string            `json:"api_key"`    // API密钥
	ModelID   string            `json:"model_id"`   // 模型ID
	Provider  string            `json:"provider"`   // Provider: openai, aliwen, deepseek等
	Extra     map[string]any    `json:"extra"`      // 额外配置
}

// ========================================
// 响应类型
// ========================================

// ChatResponse 聊天响应
type ChatResponse struct {
	MessageID    string     `json:"message_id"`    // 消息ID
	Content      string     `json:"content"`       // 响应内容
	Role         string     `json:"role"`          // 角色
	TokenCount   int        `json:"token_count"`   // Token数量
	ToolCalls    []ToolCall `json:"tool_calls"`    // 工具调用
	FinishReason string     `json:"finish_reason"` // 结束原因
}

// StreamResponse 流式响应
type StreamResponse struct {
	Event      string     `json:"event"`       // 事件类型: start/content/end/error
	Content    string     `json:"content"`     // 内容片段
	MessageID  string     `json:"message_id"`  // 消息ID
	TokenCount int        `json:"token_count"` // 当前token计数
	ToolCalls  []ToolCall `json:"tool_calls"`  // 工具调用
	Error      string     `json:"error"`       // 错误信息
}

// ========================================
// SSE事件类型
// ========================================

// SSEEvent SSE事件
type SSEEvent struct {
	Event string
	Data  string
}

// StreamEvent 流式事件类型
const (
	EventStart   = "start"   // 开始
	EventContent = "content" // 内容块
	EventEnd     = "end"     // 结束
	EventError   = "error"   // 错误
)
