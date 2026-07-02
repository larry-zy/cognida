// Package agent provides Agent domain-specific service interfaces
package agent

import "context"

// ========================================
// HookService Hook服务接口
// ========================================

// HookService defines the lifecycle hook interface for Agent execution.
// Hooks allow custom logic to be injected before and after agent processing.
type HookService interface {
	// Before executes before the agent processes a message.
	// Can modify the context and message, or return an error to halt execution.
	Before(ctx context.Context, message string) (context.Context, string, error)

	// After executes after the agent generates a response.
	// Can modify the response metadata or return an error (non-blocking by default).
	After(ctx context.Context, resp interface{}) error
}

// ========================================
// MemoryService Memory 服务接口
// ========================================

// MemoryService defines the interface for memory management in Agent domain.
// This allows agents to store and retrieve conversation context and long-term knowledge.
type MemoryService interface {
	// SaveMessage saves a message to memory (write-through: MySQL → Redis).
	SaveMessage(ctx context.Context, sessionID, message string, role string) error

	// LoadHistory loads conversation history for a session.
	// Returns messages in chronological order.
	LoadHistory(ctx context.Context, sessionID string, limit int) ([]string, error)

	// ClearSession clears all messages for a session.
	ClearSession(ctx context.Context, sessionID string) error

	// CompressContext compresses the conversation context when needed.
	CompressContext(ctx context.Context, sessionID string) error

	// BuildContext builds the complete context for LLM consumption.
	BuildContext(ctx context.Context, sessionID string, currentMessage string) (string, error)

	// StoreLongTermMemory stores important information as long-term memory.
	StoreLongTermMemory(ctx context.Context, tenantID int64, content, category string, importance float64) error

	// RecallLongTermMemory retrieves relevant long-term memories.
	RecallLongTermMemory(ctx context.Context, tenantID int64, query string, limit int) ([]string, error)
}

// ========================================
// ContextBuilderService 上下文构建服务接口
// ========================================

// ContextBuilderService defines the interface for building agent context.
// This integrates with MemoryService to construct the final context for LLM calls.
type ContextBuilderService interface {
	// Build builds the complete context for an agent execution.
	Build(ctx context.Context, req *BuildContextRequest) (*BuildContext, error)

	// Validate validates that the context is within token limits.
	Validate(ctx context.Context, context *BuildContext) error
}

// BuildContextRequest 上下文构建请求
type BuildContextRequest struct {
	SessionID      string
	TenantID       int64
	UserID         int64
	CurrentMessage string
	Config         *ContextConfig
}

// BuildContext 构建的上下文
type BuildContext struct {
	SystemPrompt string
	Messages     []string
	Metadata     *ContextMetadata
}

// ContextMetadata 上下文元数据
type ContextMetadata struct {
	SystemPromptTokens int
	HistoryTokens      int
	CurrentTokens      int
	TotalTokens        int
	MessageCount       int
}

// ========================================
// ToolRegistry 工具注册表接口
// ========================================

// ToolRegistry defines the interface for managing tools in the Agent domain.
// Tools are domain concepts that represent capabilities an agent can use.
type ToolRegistry interface {
	// Register registers a new tool.
	Register(tool Tool) error

	// Unregister removes a tool by name.
	Unregister(toolName string) error

	// Get retrieves a tool by name.
	Get(toolName string) (Tool, bool)

	// List returns all registered tools.
	List() []Tool

	// ListByType returns tools filtered by type.
	ListByType(toolType ToolType) []Tool

	// Enable marks a tool as enabled for use.
	Enable(toolName string) error

	// Disable marks a tool as disabled.
	Disable(toolName string) error

	// IsEnabled checks if a tool is enabled.
	IsEnabled(toolName string) bool
}

// ========================================
// ToolExecutor 工具执行器接口
// ========================================

// ToolExecutor defines the interface for executing tools.
type ToolExecutor interface {
	// Execute runs a tool synchronously and returns the result.
	Execute(ctx context.Context, toolName string, input string) (string, error)

	// ExecuteWithStream runs a tool with streaming output.
	ExecuteWithStream(ctx context.Context, toolName string, input string) (<-chan string, error)
}

// ========================================
// AgentExecutor Agent执行器接口
// ========================================

// AgentExecutor defines the interface for executing agent logic.
// This is a domain service interface that orchestrates agent execution.
//
// Note: 具体的请求/响应类型由 Application 层定义，
// 这里使用通用接口类型以保持 Domain 层的纯粹性。
type AgentExecutor interface {
	// Execute runs the agent with the given input.
	Execute(ctx context.Context, agentID string, input string) (string, error)

	// ExecuteStream runs the agent with streaming response.
	// 返回结构化事件流，保留工具调用轨迹与思考步骤，供上层可视化。
	ExecuteStream(ctx context.Context, agentID string, input string) (<-chan *StreamEvent, error)

	// GetStatus returns the current agent status.
	GetStatus(agentID string) (AgentStatus, error)
}

// ========================================
// StreamEvent Agent 流式执行事件（领域实体）
// ========================================

// StreamEventType 流式事件类型
type StreamEventType string

const (
	// StreamEventContent 内容增量（LLM 生成的文本 token）
	StreamEventContent StreamEventType = "content"
	// StreamEventToolCall 工具调用开始
	StreamEventToolCall StreamEventType = "tool_call"
	// StreamEventToolResult 工具执行结果
	StreamEventToolResult StreamEventType = "tool_result"
	// StreamEventError 执行错误
	StreamEventError StreamEventType = "error"
)

// StreamEvent 表示 Agent 执行过程中的一个结构化事件。
//
// 相比纯文本流，它保留了"第几步、调用了哪个工具、入参/出参、状态"等
// 结构化信息，使前端可以把 Agent 的思考过程渲染成可视化时间线。
type StreamEvent struct {
	Type      StreamEventType // 事件类型
	Content   string          // 文本增量（Type=content 时有效）
	Step      int             // 步骤序号（工具调用轮次，从 1 递增）
	ToolID    string          // 工具调用 ID
	ToolName  string          // 工具名称
	ToolInput string          // 工具入参（JSON 字符串）
	Output    string          // 工具输出（JSON 字符串，Type=tool_result 时有效）
	Status    string          // 工具状态: "calling" | "success" | "error"
	Error     string          // 错误信息
}

// ========================================
// ToolCall 工具调用记录（领域实体）
// ========================================

// ToolCall 工具调用记录（纯粹领域实体）
// 注意：这只是记录基本信息，详细记录由 Application 层 DTO 定义
type ToolCall struct {
	Name      string
	Input     string
	Output    string
	Error     string
	Duration  int64 // 毫秒
	CalledAt  int64 // Unix timestamp
}

// NewToolCall 创建新的工具调用记录
func NewToolCall(name, input string) *ToolCall {
	return &ToolCall{
		Name:     name,
		Input:    input,
		CalledAt: toUnixMillis(),
	}
}

// WithOutput 设置输出
func (tc *ToolCall) WithOutput(output string) *ToolCall {
	tc.Output = output
	return tc
}

// WithError 设置错误
func (tc *ToolCall) WithError(err error) *ToolCall {
	if err != nil {
		tc.Error = err.Error()
	}
	return tc
}

// WithDuration 设置耗时
func (tc *ToolCall) WithDuration(duration int64) *ToolCall {
	tc.Duration = duration
	return tc
}

// toUnixMillis 获取当前 Unix 毫秒时间戳
func toUnixMillis() int64 {
	return 0 // 具体实现由 Infrastructure 提供
}
