// Package agent provides LLM-callable collaboration tools for multi-agent systems.
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"cognida/internal/model/agent"
)

// 编译期断言：这些协作元工具必须实现 eino tool.InvokableTool，否则
// runSelectedTool 的 type switch 会落到 default 报「unsupported tool type」——
// 工具能被 Info 广播给 LLM、却在执行时被拒（历史上 opts ...any 误签名即如此潜伏）。
var (
	_ tool.InvokableTool = (*DelegateTool)(nil)
	_ tool.InvokableTool = (*AskTool)(nil)
	_ tool.InvokableTool = (*HandoffTool)(nil)
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
// Phase 7 起以结构化委派信封为参数契约（校验型：缺必填字段拒绝并回灌 LLM）。
type DelegateTool struct {
	registry *CollaborationRegistry
}

// NewDelegateTool creates a new delegate tool.
func NewDelegateTool(registry *CollaborationRegistry) *DelegateTool {
	return &DelegateTool{registry: registry}
}

// delegationEnvelopeParams 委派信封的工具参数 schema（单次与并行委派共用）。
func delegationEnvelopeParams() map[string]*schema.ParameterInfo {
	return map[string]*schema.ParameterInfo{
		"agent_name": {
			Type:     schema.String,
			Desc:     "The name of the agent to delegate the task to",
			Required: true,
		},
		"goal": {
			Type:     schema.String,
			Desc:     "该子任务要达成的目标（必填）",
			Required: true,
		},
		"inputs": {
			Type: schema.Object,
			Desc: "任务输入（按需）：result_id 传既有结果句柄，sql 传待执行/待分析语句，question 传自然语言问题",
			SubParams: map[string]*schema.ParameterInfo{
				"result_id": {Type: schema.String, Desc: "既有查询/分析结果的句柄"},
				"sql":       {Type: schema.String, Desc: "SQL 语句"},
				"question":  {Type: schema.String, Desc: "自然语言问题"},
			},
		},
		"constraints": {
			Type:     schema.Object,
			Desc:     "委派约束（必填）：scope 声明本次授予的权限级，max_rows 约束结果规模",
			Required: true,
			SubParams: map[string]*schema.ParameterInfo{
				"scope": {
					Type:     schema.String,
					Desc:     "本次委派授予的权限 scope（必填，不得超过会话自身 scope）",
					Enum:     []string{ScopeRead, ScopeWrite, ScopeETL},
					Required: true,
				},
				"max_rows": {Type: schema.Integer, Desc: "结果行数上限"},
			},
		},
		"return": {
			Type: schema.String,
			Desc: "期望回传形态（如 \"result_id + 结论摘要\"）",
		},
	}
}

// Info returns the tool description for LLM.
func (t *DelegateTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	agentList := formatAgentInfos(t.registry.ListWithDescriptions())

	return &schema.ToolInfo{
		Name:        "delegate_to_agent",
		Desc:        fmt.Sprintf("Delegate a task to a specialized agent with a structured envelope. 子代理只回传紧凑 handle/摘要（如 result_id + 结论）。缺 goal 或 constraints.scope 的委派会被拒绝。\n\nAvailable agents:\n%s", agentList),
		ParamsOneOf: schema.NewParamsOneOfByParams(delegationEnvelopeParams()),
	}, nil
}

// InvokableRun executes the delegation.
func (t *DelegateTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var env DelegationEnvelope
	if err := json.Unmarshal([]byte(argumentsInJSON), &env); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}
	// 契约校验、循环/深度/scope 越权护栏、每次委派授予与留痕均在执行内核
	return executeDelegation(ctx, t.registry, &env)
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
func (t *AskTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
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
func (t *HandoffTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
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
