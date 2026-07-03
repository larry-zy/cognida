// Package agent provides Agent implementation using Eino framework.
//
// NOTE: For domain-level agent concepts, see link/internal/model/agent.
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	domainagent "link/internal/model/agent"
	"link/internal/model/memory"
)

// ========================================
// MemoryService and ContextBuilder 接口
// ========================================

// MemoryService 定义 Agent 需要的记忆服务接口
// 这是一个简化接口，避免循环依赖
type MemoryService interface {
	SaveMessage(ctx context.Context, msg *memory.Message) error
	LoadHistoryWithLimit(ctx context.Context, sessionID string, limit int) ([]*memory.Message, error)
	UpdateSummary(ctx context.Context, sessionID string, summary string) error
	GetSummary(ctx context.Context, sessionID string) (string, error)
}

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

// agentImpl is the default implementation of Agent.
type agentImpl struct {
	name        string
	model       model.BaseChatModel // 使用 BaseChatModel（ChatModel 已废弃）
	toolModel   model.ToolCallingChatModel // 用于工具调用
	prompt      string
	tools       []tool.BaseTool
	beforeHooks []BeforeHook
	afterHooks  []AfterHook
	middleware  []Middleware
	maxIter     int // 最大迭代次数（用于工具调用循环）

	// Memory 和 Context Builder 支持
	memoryService   MemoryService // 记忆服务接口
	contextBuilder  ContextBuilder  // 上下文构建器接口
	enableMemory    bool            // 是否启用记忆功能
}

// New creates a new Builder for constructing an Agent.
// Note: Accepts BaseChatModel (ChatModel is deprecated)
func New(chatModel model.BaseChatModel) *Builder {
	return &Builder{
		model: chatModel,
	}
}

// Chat implements Agent.Chat with tool calling support.
func (a *agentImpl) Chat(ctx context.Context, message string) (*Response, error) {
	// 获取会话 ID
	sessionID, _ := domainagent.GetSessionID(ctx)

	// 如果启用了记忆且有上下文构建器，使用 ContextBuilder 构建上下文
	if a.enableMemory && a.contextBuilder != nil && sessionID != "" {
		return a.chatWithMemory(ctx, message, sessionID)
	}

	// Execute before hooks
	for _, hook := range a.beforeHooks {
		var err error
		ctx, message, err = hook(ctx, message)
		if err != nil {
			return nil, err
		}
	}

	// Execute middleware before
	for _, mw := range a.middleware {
		var err error
		ctx, message, err = mw.Before(ctx, message)
		if err != nil {
			return nil, err
		}
	}

	// 如果有工具且支持工具调用，使用工具调用模式
	if len(a.tools) > 0 && a.toolModel != nil {
		return a.chatWithTools(ctx, message)
	}

	// 普通聊天模式
	return a.chatWithoutTools(ctx, message)
}

// chatWithMemory 使用记忆功能的聊天，支持工具调用
func (a *agentImpl) chatWithMemory(ctx context.Context, message string, sessionID string) (*Response, error) {
	// 1. 获取协作上下文（如果有）
	collabCtx, hasCollabCtx := domainagent.GetCollaborationContext(ctx)

	// 2. 先同步保存用户消息到记忆，确保上下文能包含它
	if a.memoryService != nil {
		userMsg := &memory.Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Type:      memory.MessageTypeUser,
			Role:      "user",
			Content:   message,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		// 同步保存，确保后续 LoadHistory 能取到
		_ = a.memoryService.SaveMessage(ctx, userMsg)
	}

	// 3. 构建上下文请求
	buildReq := &memory.BuildContextRequest{
		SessionID:      sessionID,
		CurrentMessage: message,
		Config: &memory.ContextBuilderConfig{
			SystemPrompt:  a.prompt,
			MaxTokens:     4000,
			ReserveTokens: 1000,
		},
	}

	// 4. 根据协作上下文模式构建
	var builtCtx *memory.BuildContext
	var err error

	if hasCollabCtx {
		// 使用协作上下文模式
		builtCtx, err = a.contextBuilder.BuildForCollaboration(
			ctx,
			buildReq,
			string(collabCtx.Mode),
			collabCtx.ContextLimit,
		)
	} else {
		// 默认模式
		builtCtx, err = a.contextBuilder.Build(ctx, buildReq)
	}

	if err != nil {
		// 降级到简单模式
		if len(a.tools) > 0 && a.toolModel != nil {
			return a.chatWithTools(ctx, message)
		}
		return a.chatWithoutTools(ctx, message)
	}

	// 5. 构建 Eino 消息列表（包含历史对话）
	messages := make([]*schema.Message, 0)
	for _, msg := range builtCtx.Messages {
		// 转换角色类型
		var role schema.RoleType
		switch msg.Role {
		case "system":
			role = schema.System
		case "user":
			role = schema.User
		case "assistant":
			role = schema.Assistant
		default:
			role = schema.User
		}
		messages = append(messages, &schema.Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	// 6. 如果有工具且支持工具调用，使用工具调用模式
	if len(a.tools) > 0 && a.toolModel != nil {
		return a.chatWithMemoryAndTools(ctx, messages, sessionID, hasCollabCtx, collabCtx)
	}

	// 7. 普通聊天模式（无工具）
	return a.chatWithMemoryOnly(ctx, messages, sessionID, builtCtx, hasCollabCtx, collabCtx)
}

// chatWithMemoryAndTools 使用记忆功能 + 工具调用的聊天
func (a *agentImpl) chatWithMemoryAndTools(ctx context.Context, messages []*schema.Message, sessionID string, hasCollabCtx bool, collabCtx *domainagent.CollaborationContext) (*Response, error) {
	response := &Response{
		ToolCalls: make([]*ToolCall, 0),
		Metadata: map[string]interface{}{
			"with_memory": true,
			"with_tools":  true,
		},
	}

	// 提取所有工具的 ToolInfo
	toolInfos := make([]*schema.ToolInfo, 0, len(a.tools))
	for _, t := range a.tools {
		info, infoErr := t.Info(ctx)
		if infoErr != nil {
			continue // 跳过无法获取信息的工具
		}
		toolInfos = append(toolInfos, info)
	}

	// 将工具绑定到模型
	var boundModel model.ToolCallingChatModel
	if len(toolInfos) > 0 {
		var bindErr error
		boundModel, bindErr = a.toolModel.WithTools(toolInfos)
		if bindErr != nil {
			return nil, fmt.Errorf("bind tools to model failed: %w", bindErr)
		}
	} else {
		boundModel = a.toolModel
	}

	// 迭代处理（可能需要多轮工具调用）
	for i := 0; i < a.maxIter; i++ {
		// 生成响应（包含历史消息）
		resp, err := boundModel.Generate(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("generate with memory failed: %w", err)
		}

		// 检查是否有工具调用
		if len(resp.ToolCalls) == 0 {
			// 没有工具调用，返回最终回复
			response.Content = resp.Content
			response.Metadata["iterations"] = i + 1
			break
		}

		// 处理工具调用
		for _, tc := range resp.ToolCalls {
			toolCall := &ToolCall{
				Name: tc.Function.Name,
			}

			// 解析参数
			var args map[string]interface{}
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					toolCall.Error = fmt.Errorf("invalid arguments: %w", err)
					toolCall.Input = nil
				} else {
					toolCall.Input = args
				}
			}

			// 执行工具
			output, err := a.invokeTool(ctx, tc)
			if err != nil {
				toolCall.Error = err
				toolCall.Output = fmt.Sprintf("Error: %v", err)
			} else {
				toolCall.Output = output
			}

			response.ToolCalls = append(response.ToolCalls, toolCall)

			// 添加工具响应消息到历史
			messages = append(messages, &schema.Message{
				Role:      schema.Assistant,
				Content:   "",
				ToolCalls: []schema.ToolCall{tc},
			})

			messages = append(messages, schema.ToolMessage(compactObservation(toolCall.Output), tc.ID))
		}
	}

	// 保存助手响应到记忆
	if a.memoryService != nil && response.Content != "" {
		assistantMsg := &memory.Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Type:      memory.MessageTypeAssistant,
			Role:      "assistant",
			Content:   response.Content,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		go a.memoryService.SaveMessage(context.Background(), assistantMsg)
	}

	// 更新协作摘要
	if hasCollabCtx {
		newSummary := collabCtx.Summary
		if newSummary == "" {
			newSummary = fmt.Sprintf("User: ... (history)\nAssistant: %s", truncateString(response.Content, 200))
		} else {
			newSummary += fmt.Sprintf("\nAssistant: %s", truncateString(response.Content, 200))
		}
		collabCtx.UpdateSummary(newSummary)

		if a.memoryService != nil {
			go a.memoryService.UpdateSummary(context.Background(), sessionID, newSummary)
		}
	}

	// 执行中间件和钩子
	for i := len(a.middleware) - 1; i >= 0; i-- {
		if err := a.middleware[i].After(ctx, response); err != nil {
			return response, err
		}
	}

	for _, hook := range a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return response, err
		}
	}

	return response, nil
}

// chatWithMemoryOnly 仅使用记忆功能（无工具）的聊天
func (a *agentImpl) chatWithMemoryOnly(ctx context.Context, messages []*schema.Message, sessionID string, builtCtx *memory.BuildContext, hasCollabCtx bool, collabCtx *domainagent.CollaborationContext) (*Response, error) {
	// 确定使用的模型
	var chatModel interface {
		Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
	}

	if a.model != nil {
		chatModel = a.model
	} else if a.toolModel != nil {
		chatModel = a.toolModel
	} else {
		return nil, fmt.Errorf("agent: no chat model available")
	}

	resp, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}

	response := &Response{
		Content: resp.Content,
		Metadata: map[string]interface{}{
			"role":              string(resp.Role),
			"context_tokens":     builtCtx.Metadata.TotalTokens,
			"history_messages":   len(builtCtx.History),
			"with_memory":        true,
		},
	}

	// 保存助手响应到记忆
	if a.memoryService != nil {
		assistantMsg := &memory.Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			SessionID: sessionID,
			Type:      memory.MessageTypeAssistant,
			Role:      "assistant",
			Content:   resp.Content,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		go a.memoryService.SaveMessage(context.Background(), assistantMsg)
	}

	// 更新协作摘要
	if hasCollabCtx {
		// 获取最后一条用户消息
		lastUserMsg := ""
		if len(messages) > 0 {
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == schema.User {
					lastUserMsg = truncateString(messages[i].Content, 100)
					break
				}
			}
		}

		newSummary := collabCtx.Summary
		if newSummary == "" {
			newSummary = fmt.Sprintf("User: %s\nAssistant: %s", lastUserMsg, truncateString(resp.Content, 200))
		} else {
			newSummary += fmt.Sprintf("\nUser: %s\nAssistant: %s", lastUserMsg, truncateString(resp.Content, 200))
		}
		collabCtx.UpdateSummary(newSummary)

		if a.memoryService != nil {
			go a.memoryService.UpdateSummary(context.Background(), sessionID, newSummary)
		}
	}

	// 执行中间件和钩子
	for i := len(a.middleware) - 1; i >= 0; i-- {
		if err := a.middleware[i].After(ctx, response); err != nil {
			return response, err
		}
	}

	for _, hook := range a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return response, err
		}
	}

	return response, nil
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// chatWithTools 执行带工具调用的聊天
func (a *agentImpl) chatWithTools(ctx context.Context, message string) (*Response, error) {
	// 构建消息
	messages := []*schema.Message{
		schema.SystemMessage(a.prompt),
		schema.UserMessage(message),
	}

	response := &Response{
		ToolCalls: make([]*ToolCall, 0),
		Metadata:  make(map[string]interface{}),
	}

	// 提取所有工具的 ToolInfo
	toolInfos := make([]*schema.ToolInfo, 0, len(a.tools))
	for _, t := range a.tools {
		info, infoErr := t.Info(ctx)
		if infoErr != nil {
			continue // 跳过无法获取信息的工具
		}
		toolInfos = append(toolInfos, info)
	}

	// 将工具绑定到模型
	var boundModel model.ToolCallingChatModel
	if len(toolInfos) > 0 {
		var bindErr error
		boundModel, bindErr = a.toolModel.WithTools(toolInfos)
		if bindErr != nil {
			return nil, fmt.Errorf("bind tools to model failed: %w", bindErr)
		}
	} else {
		boundModel = a.toolModel
	}

	// 迭代处理（可能需要多轮工具调用）
	for i := 0; i < a.maxIter; i++ {
		// 生成响应
		resp, err := boundModel.Generate(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("generate failed: %w", err)
		}

		// 检查是否有工具调用
		if len(resp.ToolCalls) == 0 {
			// 没有工具调用，返回最终回复
			response.Content = resp.Content
			response.Metadata["iterations"] = i + 1
			break
		}

		// 处理工具调用
		for _, tc := range resp.ToolCalls {
			toolCall := &ToolCall{
				Name: tc.Function.Name,
			}

			// 解析参数
			var args map[string]interface{}
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					toolCall.Error = fmt.Errorf("invalid arguments: %w", err)
					toolCall.Input = nil
				} else {
					toolCall.Input = args
				}
			}

			// 执行工具
			output, err := a.invokeTool(ctx, tc)
			if err != nil {
				toolCall.Error = err
				toolCall.Output = fmt.Sprintf("Error: %v", err)
			} else {
				toolCall.Output = output
			}

			response.ToolCalls = append(response.ToolCalls, toolCall)

			// 添加工具响应消息
			messages = append(messages, &schema.Message{
				Role:      schema.Assistant,
				Content:   "",
				ToolCalls: []schema.ToolCall{tc},
			})

			messages = append(messages, schema.ToolMessage(compactObservation(toolCall.Output), tc.ID))
		}
	}

	// Execute middleware after
	for i := len(a.middleware) - 1; i >= 0; i-- {
		if err := a.middleware[i].After(ctx, response); err != nil {
			return response, err
		}
	}

	// Execute after hooks
	for _, hook := range a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return response, err
		}
	}

	return response, nil
}

// chatWithoutTools 执行不带工具调用的聊天
func (a *agentImpl) chatWithoutTools(ctx context.Context, message string) (*Response, error) {
	// 确定使用的模型：优先使用 model，回退到 toolModel
	var chatModel interface {
		Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
	}

	if a.model != nil {
		chatModel = a.model
	} else if a.toolModel != nil {
		chatModel = a.toolModel
	} else {
		return nil, fmt.Errorf("agent: no chat model available")
	}

	// Build messages
	messages := []*schema.Message{
		schema.SystemMessage(a.prompt),
		schema.UserMessage(message),
	}

	// Generate response
	resp, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}

	response := &Response{
		Content: resp.Content,
		Metadata: map[string]interface{}{
			"role": string(resp.Role),
		},
	}

	// Execute middleware after
	for i := len(a.middleware) - 1; i >= 0; i-- {
		if err := a.middleware[i].After(ctx, response); err != nil {
			return response, err
		}
	}

	// Execute after hooks
	for _, hook := range a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return response, err
		}
	}

	return response, nil
}

// invokeTool 执行单个工具调用
func (a *agentImpl) invokeTool(ctx context.Context, toolCall schema.ToolCall) (string, error) {
	// 查找工具
	var selectedTool tool.BaseTool
	for _, t := range a.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == toolCall.Function.Name {
			selectedTool = t
			break
		}
	}

	if selectedTool == nil {
		return "", fmt.Errorf("tool not found: %s", toolCall.Function.Name)
	}

	// 执行工具
	switch t := selectedTool.(type) {
	case tool.InvokableTool:
		return t.InvokableRun(ctx, toolCall.Function.Arguments)
	case tool.StreamableTool:
		stream, err := t.StreamableRun(ctx, toolCall.Function.Arguments)
		if err != nil {
			return "", fmt.Errorf("streamable run failed: %w", err)
		}

		// 收集流式结果
		var result strings.Builder
		for {
			chunk, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				return "", fmt.Errorf("recv failed: %w", err)
			}
			result.WriteString(chunk)
		}
		stream.Close()
		return result.String(), nil
	default:
		return "", fmt.Errorf("unsupported tool type: %T", t)
	}
}

// Stream implements Agent.Stream with full tool calling support.
func (a *agentImpl) Stream(ctx context.Context, message string) (<-chan *Chunk, error) {
	// 提前检查模型是否可用
	if a.model == nil && a.toolModel == nil {
		return nil, fmt.Errorf("agent: no chat model available for streaming")
	}

	ch := make(chan *Chunk, 16)

	// 获取会话 ID
	sessionID, _ := domainagent.GetSessionID(ctx)

	// Execute before hooks
	for _, hook := range a.beforeHooks {
		var err error
		ctx, message, err = hook(ctx, message)
		if err != nil {
			close(ch)
			return nil, err
		}
	}

	// Execute middleware before
	for _, mw := range a.middleware {
		var err error
		ctx, message, err = mw.Before(ctx, message)
		if err != nil {
			close(ch)
			return nil, err
		}
	}

	// 启动流式处理 goroutine
	go a.streamInternal(ctx, message, sessionID, ch)

	return ch, nil
}

// streamInternal 内部流式处理逻辑
func (a *agentImpl) streamInternal(ctx context.Context, message string, sessionID string, ch chan *Chunk) {
	defer close(ch)

	// 发送开始事件
	ch <- &Chunk{
		Content: "",
		Done:    false,
		Metadata: map[string]interface{}{
			"event": "start",
		},
	}

	// 构建初始消息
	messages := []*schema.Message{
		schema.SystemMessage(a.prompt),
		schema.UserMessage(message),
	}

	// 如果启用记忆且有上下文构建器，使用记忆模式
	if a.enableMemory && a.contextBuilder != nil && sessionID != "" {
		// 先保存用户消息
		if a.memoryService != nil {
			userMsg := &memory.Message{
				ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				SessionID: sessionID,
				Type:      memory.MessageTypeUser,
				Role:      "user",
				Content:   message,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			_ = a.memoryService.SaveMessage(ctx, userMsg)
		}

		// 构建上下文
		buildReq := &memory.BuildContextRequest{
			SessionID:      sessionID,
			CurrentMessage: message,
			Config: &memory.ContextBuilderConfig{
				SystemPrompt:  a.prompt,
				MaxTokens:     4000,
				ReserveTokens: 1000,
			},
		}

		builtCtx, err := a.contextBuilder.Build(ctx, buildReq)
		if err == nil {
			// 重建消息列表
			messages = make([]*schema.Message, 0)
			for _, msg := range builtCtx.Messages {
				var role schema.RoleType
				switch msg.Role {
				case "system":
					role = schema.System
				case "user":
					role = schema.User
				case "assistant":
					role = schema.Assistant
				default:
					role = schema.User
				}
				messages = append(messages, &schema.Message{
					Role:    role,
					Content: msg.Content,
				})
			}
		}
	}

	// 如果没有工具或不支持工具调用，使用简单流式模式
	if len(a.tools) == 0 || a.toolModel == nil {
		a.streamWithoutTools(ctx, messages, ch)
		return
	}

	// 完整的流式 + 工具调用模式
	a.streamWithTools(ctx, messages, sessionID, ch)
}

// streamWithTools 流式处理带工具调用的响应
func (a *agentImpl) streamWithTools(ctx context.Context, messages []*schema.Message, sessionID string, ch chan *Chunk) {
	// 提取所有工具的 ToolInfo
	toolInfos := make([]*schema.ToolInfo, 0, len(a.tools))
	for _, t := range a.tools {
		info, infoErr := t.Info(ctx)
		if infoErr != nil {
			continue
		}
		toolInfos = append(toolInfos, info)
	}

	// 将工具绑定到模型
	var boundModel model.ToolCallingChatModel
	if len(toolInfos) > 0 {
		var bindErr error
		boundModel, bindErr = a.toolModel.WithTools(toolInfos)
		if bindErr != nil {
			ch <- &Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"event": string(EventError),
					"error": fmt.Sprintf("绑定工具失败: %v", bindErr),
				},
			}
			return
		}
	} else {
		boundModel = a.toolModel
	}

	// 迭代处理多轮工具调用
	for iteration := 0; iteration < a.maxIter; iteration++ {
		// 流式生成响应
		streamReader, err := boundModel.Stream(ctx, messages)
		if err != nil {
			ch <- &Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"event": string(EventError),
					"error": fmt.Sprintf("生成失败: %v", err),
				},
			}
			return
		}

		// 使用 defer 确保 streamReader 被关闭（只关闭一次）
// 收集完整响应和工具调用
			defer streamReader.Close()
		var contentBuilder strings.Builder
		var toolCallsInChunk []schema.ToolCall
		var hasToolCalls bool

		for {
			chunk, err := streamReader.Recv()
			if err != nil {
				// 流结束（包括 EOF）
				break
			}

			if chunk == nil {
				break
			}

			// 检查是否有工具调用
			if len(chunk.ToolCalls) > 0 {
				hasToolCalls = true
				toolCallsInChunk = chunk.ToolCalls
				break
			}

			// 累积内容
			if chunk.Content != "" {
				contentBuilder.WriteString(chunk.Content)
				// 发送内容块
				ch <- &Chunk{
					Content: chunk.Content,
					Done:    false,
					Metadata: map[string]interface{}{
						"event":    string(EventContent),
						"iteration": iteration + 1,
					},
				}
			}
		}

		// 如果没有工具调用，说明是最终响应
		if !hasToolCalls {
			// 保存助手响应到记忆
			if sessionID != "" && a.memoryService != nil && contentBuilder.Len() > 0 {
				assistantMsg := &memory.Message{
					ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
					SessionID: sessionID,
					Type:      memory.MessageTypeAssistant,
					Role:      "assistant",
					Content:   contentBuilder.String(),
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				go a.memoryService.SaveMessage(context.Background(), assistantMsg)
			}

			// 发送结束事件
			ch <- &Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"event":      string(EventEnd),
					"iterations": iteration + 1,
				},
			}
			return
		}

		// 处理工具调用
		for _, tc := range toolCallsInChunk {
			// 解析参数
			var args map[string]interface{}
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					ch <- &Chunk{
						Content: "",
						Done:    false,
						Metadata: map[string]interface{}{
							"event": string(EventToolCall),
							"tool_call": &ToolCallInStream{
								ID:     tc.ID,
								Name:   tc.Function.Name,
								Status: "error",
								Error:  fmt.Sprintf("参数解析失败: %v", err),
							},
						},
					}
					continue
				}
			}

			// 发送工具调用事件
			ch <- &Chunk{
				Content: "",
				Done:    false,
				Metadata: map[string]interface{}{
					"event": string(EventToolCall),
					"tool_call": &ToolCallInStream{
						ID:     tc.ID,
						Name:   tc.Function.Name,
						Input:  args,
						Status: "calling",
					},
				},
			}

			// 执行工具
			output, execErr := a.invokeTool(ctx, tc)

			// 发送工具结果事件
			resultEvent := &ToolCallInStream{
				ID:     tc.ID,
				Name:   tc.Function.Name,
				Input:  args,
				Status: "success",
				Output: output,
			}

			if execErr != nil {
				resultEvent.Status = "error"
				resultEvent.Error = execErr.Error()
			}

			ch <- &Chunk{
				Content: "",
				Done:    false,
				Metadata: map[string]interface{}{
					"event":      string(EventToolResult),
					"tool_call":  resultEvent,
					"iteration":  iteration + 1,
				},
			}

			// 添加工具调用和结果到消息历史
			toolOutput := output
			if execErr != nil {
				toolOutput = fmt.Sprintf("Error: %v", execErr)
			}

			messages = append(messages, &schema.Message{
				Role:      schema.Assistant,
				Content:   "",
				ToolCalls: []schema.ToolCall{tc},
			})

			messages = append(messages, schema.ToolMessage(compactObservation(toolOutput), tc.ID))
		}
	}

	// 达到最大迭代次数
	ch <- &Chunk{
		Content: "",
		Done:    true,
		Metadata: map[string]interface{}{
			"event":       string(EventEnd),
			"max_reached": true,
			"iterations":  a.maxIter,
		},
	}
}

// streamWithoutTools 流式处理不带工具调用的响应
func (a *agentImpl) streamWithoutTools(ctx context.Context, messages []*schema.Message, ch chan *Chunk) {
	// 确定使用的模型
	var streamModel interface {
		Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
	}

	if a.model != nil {
		streamModel = a.model
	} else if a.toolModel != nil {
		streamModel = a.toolModel
	} else {
		ch <- &Chunk{
			Content: "",
			Done:    true,
			Metadata: map[string]interface{}{
				"event": "error",
				"error": "no chat model available",
			},
		}
		return
	}

	streamReader, err := streamModel.Stream(ctx, messages)
	if err != nil {
		ch <- &Chunk{
			Content: "",
			Done:    true,
			Metadata: map[string]interface{}{
				"event": "error",
				"error": err.Error(),
			},
		}
		return
	}

	defer streamReader.Close()

	for {
		chunk, err := streamReader.Recv()
		if err != nil {
			ch <- &Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"event": "error",
					"error": err.Error(),
				},
			}
			return
		}

		if chunk == nil {
			ch <- &Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"event": "end",
				},
			}
			return
		}

		ch <- &Chunk{
			Content: chunk.Content,
			Done:    false,
			Metadata: map[string]interface{}{
				"event": "content",
				"role":  string(chunk.Role),
			},
		}
	}
}

// Name implements Agent.Name.
func (a *agentImpl) Name() string {
	return a.name
}

// maxObservationChars 单条工具观察进入历史前的字符上限。数据类工具（sql_execute 等）
// 已经回传紧凑信封，此处是对其余工具超长输出的通用安全网，防止长循环爆窗。
const maxObservationChars = 8000

// compactObservation 压缩工具观察：超过上限则截断并标注，避免原始大结果逐字灌入上下文。
// data-by-reference 的正道是工具回传 result_id 信封；本函数只兜底非信封化的超长输出。
func compactObservation(output string) string {
	runes := []rune(output)
	if len(runes) <= maxObservationChars {
		return output
	}
	return string(runes[:maxObservationChars]) +
		fmt.Sprintf("\n...[观察已截断，省略 %d 字符；如需完整数据请按 result_id 取用]", len(runes)-maxObservationChars)
}
