package dataagent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	domainagent "link/internal/model/agent"
	"link/internal/model/conversation"
	"link/internal/service/agent/convcontext"
	infraagent "link/internal/service/agent/framework"
	toolregistry "link/internal/service/agent/tools"
)

// DataAgentID 是 Data Agent 预设的注册 ID。
const DataAgentID = "agent-data-agent"

const (
	// defaultMaxIter 单一 ReAct 循环默认最大步数。
	defaultMaxIter = 12
	// defaultTokenBudget 默认 token 预算（与 maxIter 共同约束循环；模型未回传 usage 时自动退化为仅 maxIter 约束）。
	defaultTokenBudget = 120000
)

// capabilityGroups 是四类能力对应的工具分组（present-if-registered）：
// 查(sql/semantic/graph) / 析(analytics) / 渲(render) / 操(operation)，外加 skill 供 playbook 调用。
// render、operation 分组由后续 Phase 3/4 落地；此处按分组收集，工具就位后自动并入编排（通用/拓展）。
var capabilityGroups = []string{"sql", "semantic", "graph", "analytics", "render", "operation", "skill"}

// collectCapabilityTools 按能力分组收集全部已注册工具并按名去重。
func collectCapabilityTools(ctx context.Context) []tool.BaseTool {
	seen := make(map[string]struct{})
	var tools []tool.BaseTool
	for _, group := range capabilityGroups {
		for _, t := range toolregistry.GetToolsByGroup(group) {
			info, err := t.Info(ctx)
			if err != nil {
				continue
			}
			if _, dup := seen[info.Name]; dup {
				continue
			}
			seen[info.Name] = struct{}{}
			tools = append(tools, t)
		}
	}
	return tools
}

// RegisterDataAgentPreset 注册单一 ReAct 内核的 Data Agent 预设。
// 以一个 ReAct 循环 Agent 承载查/析/渲/操四类能力，入口挂载意图路由 BeforeHook，
// 循环受 maxIter + token 预算约束。collabRegistry 非 nil 时同时启用子代理委派
// （delegate_to_agent + delegate_parallel，Phase 7 orchestrator-worker）。
// msgRepo 非 nil 时启用跨轮对话记忆：从 messages 表回放会话历史，装配多轮上下文。
func RegisterDataAgentPreset(
	ctx context.Context,
	registry domainagent.AgentRegistry,
	toolModel model.ToolCallingChatModel,
	collabRegistry *infraagent.CollaborationRegistry,
	msgRepo conversation.MessageRepository,
) error {
	if toolModel == nil {
		return fmt.Errorf("data agent 预设需要 ToolCallingChatModel")
	}

	tools := collectCapabilityTools(ctx)
	if len(tools) == 0 {
		return fmt.Errorf("data agent 预设未收集到任何能力工具（分组均未注册）")
	}

	builder := infraagent.New(nil).
		Name("DataAgent").
		Prompt(systemPrompt).
		WithToolModel(toolModel).
		Tools(tools...).
		Before(toolPolicyHook()). // 硬工具门：以原始消息匹配 skill，须先于 playbook 注入
		Before(intentRoutingHook()).
		WithMaxIterations(defaultMaxIter).
		WithTokenBudget(defaultTokenBudget)
	if msgRepo != nil {
		// 跨轮对话记忆：读 messages 表回放历史（与 UI 同源，只读不写），启用 framework 记忆分支
		builder = builder.WithContextBuilder(convcontext.NewConversationContextBuilder(msgRepo))
	}
	if collabRegistry != nil {
		// 指挥官持委派能力：复杂任务可拆解给数据域子代理（每次委派授予最小 scope）
		builder = builder.WithCollaboration(collabRegistry, infraagent.EnableDelegate())
	}
	reactAgent, err := builder.Build(ctx)
	if err != nil {
		return fmt.Errorf("构建 Data Agent 失败: %w", err)
	}

	def := &domainagent.AgentDefinition{
		ID:          DataAgentID,
		Name:        "data_agent",
		Description: "Data Agent：单一 ReAct 内核，承载查/析/渲/操四类能力，入口意图路由 + maxIter/token 预算约束",
		Type:        domainagent.AgentTypeAgenticRAG,
		Status:      domainagent.AgentStatusIdle,
		Metadata: map[string]string{
			"version": "1.0.0",
			"pattern": "single-react",
			"kernel":  "reason-act-observe",
		},
	}
	if err := registry.Register(ctx, def); err != nil {
		return fmt.Errorf("注册 Data Agent 失败: %w", err)
	}

	return SetAgent(reactAgent)
}

// ========================================
// Agent 实例管理（单例）
// ========================================

var dataAgentInstance infraagent.Agent

// SetAgent 设置 Data Agent 实例。
func SetAgent(agent infraagent.Agent) error {
	dataAgentInstance = agent
	return nil
}

// GetAgent 获取 Data Agent 实例。
func GetAgent() infraagent.Agent {
	return dataAgentInstance
}
