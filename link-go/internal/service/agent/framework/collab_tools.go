// Package agent provides LLM-callable collaboration tools for multi-agent systems.
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"link/internal/model/agent"
)

// ========================================
// Context Keys for Collaboration Tracking
// ========================================

type contextKey struct{}

var delegatePathKey = contextKey{}

// ========================================
// Delegate Tool - 委派任务给指定 Agent
// ========================================

// DelegateTool implements tool.InvokableTool for delegating tasks to other agents.
type DelegateTool struct {
	registry *CollaborationRegistry
	maxDepth int
	timeout  time.Duration
}

// NewDelegateTool creates a new delegate tool.
func NewDelegateTool(registry *CollaborationRegistry) *DelegateTool {
	return &DelegateTool{
		registry: registry,
		maxDepth:  5, // Prevent infinite loops
		timeout:  30 * time.Second,
	}
}

// Info returns the tool description for LLM.
func (t *DelegateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	agents := t.registry.ListWithDescriptions()
	agentList := t.formatAgentList(agents)

	return &schema.ToolInfo{
		Name: "delegate_to_agent",
		Desc: fmt.Sprintf("Delegate a task to another agent. Use this when you need a specialized agent to handle a specific sub-task.\n\nAvailable agents:\n%s", agentList),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"agent_name": {
				Type:     schema.String,
				Desc:     "The name of the agent to delegate the task to",
				Required: true,
			},
			"task": {
				Type:     schema.String,
				Desc:     "The specific task description to delegate",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun executes the delegation.
func (t *DelegateTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...any) (string, error) {
	// Parse arguments
	var args struct {
		AgentName string `json:"agent_name"`
		Task      string `json:"task"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate parameters
	if args.AgentName == "" {
		return "", fmt.Errorf("agent_name is required")
	}
	if args.Task == "" {
		return "", fmt.Errorf("task is required")
	}

	// 获取协作上下文
	collabCtx, hasCtx := agent.GetCollaborationContext(ctx)

	// 检查循环委派
	if hasCtx && collabCtx.IsCyclic(args.AgentName) {
		return "", fmt.Errorf("检测到循环委派: %s → %s",
			collabCtx.ChainDescription(), args.AgentName)
	}

	// 检查深度限制
	if hasCtx && collabCtx.IsDepthExceeded() {
		return "", fmt.Errorf("超过最大委派深度: %d", collabCtx.MaxDepth)
	}

	// Check if agent exists
	targetAgent, err := t.registry.GetByName(args.AgentName)
	if err != nil {
		agents := t.registry.ListWithDescriptions()
		suggestions := t.formatAgentList(agents)
		return "", fmt.Errorf("agent '%s' not found. Available agents:\n%s", args.AgentName, suggestions)
	}

	// 更新委派链路
	if hasCtx {
		collabCtx.AddDelegate(args.AgentName)
		ctx = agent.WithCollaborationContext(ctx, collabCtx)
	} else {
		// 如果没有协作上下文，使用旧的路径追踪方式
		if t.detectLoop(ctx, args.AgentName) {
			path := getDelegatePath(ctx)
			return "", NewCollabLoopError(path, args.AgentName)
		}
		ctx = withDelegatePath(ctx, args.AgentName)
	}

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// 构建消息
	message := t.buildMessage(ctx, collabCtx, hasCtx, args.Task)

	// Execute delegation
	startTime := time.Now()
	resp, err := targetAgent.Chat(ctx, message)
	duration := time.Since(startTime)

	if err != nil {
		// 存储错误结果
		if hasCtx {
			collabCtx.StoreResult(args.AgentName, &agent.TaskResult{
				AgentName: args.AgentName,
				Error:     err,
				Duration:  duration,
				StartTime: startTime,
				EndTime:   startTime.Add(duration),
			})
		}
		return "", fmt.Errorf("delegation to agent '%s' failed: %w", args.AgentName, err)
	}

	// 存储成功结果
	if hasCtx {
		collabCtx.StoreResult(args.AgentName, &agent.TaskResult{
			AgentName: args.AgentName,
			Content:   resp.Content,
			Duration:  duration,
			StartTime: startTime,
			EndTime:   startTime.Add(duration),
		})
	}

	return resp.Content, nil
}

// buildMessage 根据协作上下文模式构建消息
func (t *DelegateTool) buildMessage(ctx context.Context, collabCtx *agent.CollaborationContext, hasCtx bool, task string) string {
	if !hasCtx || collabCtx.Mode == agent.ContextModeNone || collabCtx.Mode == agent.ContextModeIsolated {
		// 无上下文模式，只传递任务
		return task
	}

	var builder strings.Builder

	switch collabCtx.Mode {
	case agent.ContextModeSummary:
		// 摘要模式：传递摘要、原始问题、委派链路
		builder.WriteString("## 协作上下文\n\n")
		if collabCtx.OriginalQuery != "" {
			builder.WriteString(fmt.Sprintf("**原始问题**: %s\n\n", collabCtx.OriginalQuery))
		}
		if collabCtx.Summary != "" {
			builder.WriteString(fmt.Sprintf("**对话摘要**: %s\n\n", collabCtx.Summary))
		}
		if len(collabCtx.DelegateChain) > 0 {
			builder.WriteString(fmt.Sprintf("**已执行流程**: %s\n\n", collabCtx.ChainDescription()))
		}
		builder.WriteString(fmt.Sprintf("**当前任务**: %s", task))

	case agent.ContextModeRecent:
		// 最近消息模式：传递最近 N 条消息
		builder.WriteString("## 最近对话\n\n")
		// 这里简化处理，实际应该从 MemoryService 加载
		if collabCtx.Summary != "" {
			builder.WriteString(fmt.Sprintf("[摘要] %s\n\n", collabCtx.Summary))
		}
		builder.WriteString(fmt.Sprintf("**当前任务**: %s", task))

	case agent.ContextModeFull:
		// 完整历史模式
		builder.WriteString("## 完整对话历史\n\n")
		if collabCtx.Summary != "" {
			builder.WriteString(fmt.Sprintf("[摘要] %s\n\n", collabCtx.Summary))
		}
		if len(collabCtx.DelegateChain) > 0 {
			builder.WriteString(fmt.Sprintf("[委派链路] %s\n\n", collabCtx.ChainDescription()))
		}
		builder.WriteString(fmt.Sprintf("**当前任务**: %s", task))

	default:
		// 默认只传递任务
		return task
	}

	return builder.String()
}

// formatAgentList formats agent list for tool description.
func (t *DelegateTool) formatAgentList(agents []AgentInfo) string {
	var sb strings.Builder
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", a.Name, a.Description))
	}
	return sb.String()
}

// detectLoop checks if the target agent is already in the delegation path.
func (t *DelegateTool) detectLoop(ctx context.Context, targetAgent string) bool {
	path := getDelegatePath(ctx)
	for _, agent := range path {
		if agent == targetAgent {
			return true
		}
	}
	return false
}

// getDelegatePath retrieves the delegation path from context.
func getDelegatePath(ctx context.Context) []string {
	if path, ok := ctx.Value(delegatePathKey).([]string); ok {
		return path
	}
	return nil
}

// withDelegatePath adds an agent to the delegation path.
func withDelegatePath(ctx context.Context, agentName string) context.Context {
	path := getDelegatePath(ctx)
	if path == nil {
		path = make([]string, 0)
	}
	path = append(path, agentName)
	return context.WithValue(ctx, delegatePathKey, path)
}

// ========================================
// Ask Tool - 向其他 Agent 咨询
// ========================================

// AskTool implements tool.InvokableTool for consulting other agents.
type AskTool struct {
	registry *CollaborationRegistry
	timeout  time.Duration
}

// NewAskTool creates a new ask tool.
func NewAskTool(registry *CollaborationRegistry) *AskTool {
	return &AskTool{
		registry: registry,
		timeout:  30 * time.Second,
	}
}

// Info returns the tool description for LLM.
func (t *AskTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	agents := t.registry.ListWithDescriptions()
	agentList := t.formatAgentList(agents)

	return &schema.ToolInfo{
		Name: "ask_agent",
		Desc: fmt.Sprintf("Ask another agent for information or advice. Use this when you need additional information or a second opinion from a specialized agent. The control remains with you.\n\nAvailable agents:\n%s", agentList),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"agent_name": {
				Type:     schema.String,
				Desc:     "The name of the agent to ask",
				Required: true,
			},
			"question": {
				Type:     schema.String,
				Desc:     "The question to ask",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun executes the ask operation.
func (t *AskTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...any) (string, error) {
	// Parse arguments
	var args struct {
		AgentName string `json:"agent_name"`
		Question  string `json:"question"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate parameters
	if args.AgentName == "" {
		return "", fmt.Errorf("agent_name is required")
	}
	if args.Question == "" {
		return "", fmt.Errorf("question is required")
	}

	// Check if agent exists
	targetAgent, err := t.registry.GetByName(args.AgentName)
	if err != nil {
		agents := t.registry.ListWithDescriptions()
		suggestions := t.formatAgentList(agents)
		return "", fmt.Errorf("agent '%s' not found. Available agents:\n%s", args.AgentName, suggestions)
	}

	// 获取协作上下文（Ask 不修改委派链路，但传递上下文）
	collabCtx, hasCtx := agent.GetCollaborationContext(ctx)

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// 构建消息（使用简化的上下文传递）
	message := args.Question
	if hasCtx && collabCtx.Mode != agent.ContextModeNone && collabCtx.Mode != agent.ContextModeIsolated {
		if collabCtx.Summary != "" {
			message = fmt.Sprintf("[上下文: %s]\n\n问题: %s", collabCtx.Summary, args.Question)
		}
	}

	// Execute ask
	startTime := time.Now()
	resp, err := targetAgent.Chat(ctx, message)
	duration := time.Since(startTime)

	// 存储结果到 SharedResults（Ask 也会记录结果）
	if hasCtx {
		collabCtx.StoreResult(args.AgentName, &agent.TaskResult{
			AgentName: args.AgentName,
			Content:   resp.Content,
			Duration:  duration,
			StartTime: startTime,
			EndTime:   startTime.Add(duration),
		})
	}

	if err != nil {
		return "", fmt.Errorf("asking agent '%s' failed: %w", args.AgentName, err)
	}

	return resp.Content, nil
}

// formatAgentList formats agent list for tool description.
func (t *AskTool) formatAgentList(agents []AgentInfo) string {
	var sb strings.Builder
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", a.Name, a.Description))
	}
	return sb.String()
}

// ========================================
// Handoff Tool - 转移对话控制权
// ========================================

// HandoffTool implements tool.InvokableTool for transferring conversation control.
type HandoffTool struct {
	registry *CollaborationRegistry
	timeout  time.Duration
}

// NewHandoffTool creates a new handoff tool.
func NewHandoffTool(registry *CollaborationRegistry) *HandoffTool {
	return &HandoffTool{
		registry: registry,
		timeout:  60 * time.Second,
	}
}

// Info returns the tool description for LLM.
func (t *HandoffTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	agents := t.registry.ListWithDescriptions()
	agentList := t.formatAgentList(agents)

	return &schema.ToolInfo{
		Name: "handoff_to",
		Desc: fmt.Sprintf("Transfer the conversation to another agent. Use this when the task is better handled by a different agent. The target agent will take over the conversation.\n\nAvailable agents:\n%s", agentList),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"agent_name": {
				Type:     schema.String,
				Desc:     "The name of the agent to transfer control to",
				Required: true,
			},
			"context": {
				Type:     schema.String,
				Desc:     "Optional context to pass to the target agent",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun executes the handoff.
func (t *HandoffTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...any) (string, error) {
	// Parse arguments
	var args struct {
		AgentName string `json:"agent_name"`
		Context   string `json:"context,omitempty"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate parameters
	if args.AgentName == "" {
		return "", fmt.Errorf("agent_name is required")
	}

	// Check if agent exists
	targetAgent, err := t.registry.GetByName(args.AgentName)
	if err != nil {
		agents := t.registry.ListWithDescriptions()
		suggestions := t.formatAgentList(agents)
		return "", fmt.Errorf("agent '%s' not found. Available agents:\n%s", args.AgentName, suggestions)
	}

	// 获取协作上下文
	collabCtx, hasCtx := agent.GetCollaborationContext(ctx)

	// Handoff 会转移控制权，更新委派链路
	if hasCtx {
		collabCtx.AddDelegate(args.AgentName + " [接管]")
		ctx = agent.WithCollaborationContext(ctx, collabCtx)
	}

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// Build context message
	message := args.Context
	if message == "" {
		message = "Taking over the conversation."
	}

	// 如果有协作上下文，添加完整上下文信息
	if hasCtx && collabCtx.Mode != agent.ContextModeNone && collabCtx.Mode != agent.ContextModeIsolated {
		var builder strings.Builder
		builder.WriteString("## 对话转移\n\n")
		if collabCtx.OriginalQuery != "" {
			builder.WriteString(fmt.Sprintf("**原始问题**: %s\n\n", collabCtx.OriginalQuery))
		}
		if collabCtx.Summary != "" {
			builder.WriteString(fmt.Sprintf("**对话摘要**: %s\n\n", collabCtx.Summary))
		}
		if len(collabCtx.DelegateChain) > 0 {
			builder.WriteString(fmt.Sprintf("**之前流程**: %s\n\n", collabCtx.ChainDescription()))
		}
		builder.WriteString(fmt.Sprintf("**转移原因**: %s", message))
		message = builder.String()
	}

	resp, err := targetAgent.Chat(ctx, message)
	if err != nil {
		return "", fmt.Errorf("handoff to agent '%s' failed: %w", args.AgentName, err)
	}

	return resp.Content, nil
}

// formatAgentList formats agent list for tool description.
func (t *HandoffTool) formatAgentList(agents []AgentInfo) string {
	var sb strings.Builder
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", a.Name, a.Description))
	}
	return sb.String()
}
