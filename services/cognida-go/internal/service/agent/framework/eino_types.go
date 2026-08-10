// 由 eino_agent.go 拆出——同包、行为等价（M1 god-file 拆分）。
package framework

import (
	"cognida/internal/model/memory"
	"context"
)

// ========================================
// ContextBuilder 接口
// ========================================

// ContextBuilder 定义 Agent 需要的上下文构建器接口
type ContextBuilder interface {
	Build(ctx context.Context, req *memory.BuildContextRequest) (*memory.BuildContext, error)
	BuildForCollaboration(ctx context.Context, req *memory.BuildContextRequest, mode string, contextLimit int) (*memory.BuildContext, error)
}

// Agent is the core interface for AI agents.
// An Agent can chat with users, stream responses, and use tools.
type Agent interface {
	// Chat processes a message and returns a complete response.
	// It includes tool calls in the response if any tools were invoked.
	Chat(ctx context.Context, message string) (*Response, error)

	// Stream processes a message and streams chunks of the response.
	// The channel closes when streaming is complete.
	Stream(ctx context.Context, message string) (<-chan *Chunk, error)

	// Name returns the agent's name.
	Name() string
}

// Response represents a complete agent response.
type Response struct {
	// Content is the text content of the response.
	Content string

	// ToolCalls contains information about tools called during processing.
	ToolCalls []*ToolCall

	// Metadata contains additional information about the response.
	Metadata map[string]interface{}
}

// ToolCall represents a single tool invocation.
type ToolCall struct {
	// Name is the name of the tool that was called.
	Name string

	// Input is the parameters passed to the tool.
	Input map[string]interface{}

	// Output is the result returned by the tool.
	Output string

	// Error contains any error that occurred during tool execution.
	Error error
}

// Chunk represents a streaming response chunk.
type Chunk struct {
	// Content is a partial piece of the response content.
	Content string

	// Done indicates whether this is the final chunk.
	Done bool

	// Metadata contains additional information about this chunk.
	Metadata map[string]interface{}
}

// ChunkEvent 表示流式响应的事件类型
type ChunkEvent string

const (
	// EventContent 内容块事件
	EventContent ChunkEvent = "content"
	// EventToolCall 工具调用事件
	EventToolCall ChunkEvent = "tool_call"
	// EventToolResult 工具执行结果事件
	EventToolResult ChunkEvent = "tool_result"
	// EventError 错误事件
	EventError ChunkEvent = "error"
	// EventEnd 结束事件
	EventEnd ChunkEvent = "end"
)

// ToolCallInStream 表示流式响应中的工具调用
type ToolCallInStream struct {
	ID     string                 // 工具调用 ID
	Name   string                 // 工具名称
	Input  map[string]interface{} // 工具输入参数
	Status string                 // 状态: "calling", "success", "error"
	Output string                 // 工具输出（执行后）
	Error  string                 // 错误信息（如果有）
}

// BeforeHook is a function that runs before the agent processes a message.
type BeforeHook func(ctx context.Context, message string) (context.Context, string, error)

// AfterHook is a function that runs after the agent generates a response.
type AfterHook func(ctx context.Context, resp *Response) error

// userQueryCtxKey 承载本轮用户原始问题（preProcess 入口写入）。
type userQueryCtxKey struct{}

// WithUserQuery 把本轮用户原始问题写入 ctx，供 Hook/Observer 按真实问题检索/聚类
// （而非误用系统提示词或已注入 playbook 的消息）。
func WithUserQuery(ctx context.Context, query string) context.Context {
	return context.WithValue(ctx, userQueryCtxKey{}, query)
}

// UserQueryFromContext 读取本轮用户原始问题；不存在时返回空串。
func UserQueryFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userQueryCtxKey{}).(string); ok {
		return v
	}
	return ""
}
