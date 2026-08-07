package dataagent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	domainagent "link/internal/model/agent"
	"link/internal/model/conversation"
	"link/internal/service/agent/context/llmsummary"
	"link/internal/service/agent/convcontext"
	infraagent "link/internal/service/agent/framework"
	"link/internal/service/agent/skills"
	toolregistry "link/internal/service/agent/tools"
)

// DataAgentID 是 Data Agent 预设的注册 ID。
const DataAgentID = "agent-data-agent"

const (
	// defaultMaxIter 单一 ReAct 循环默认最大步数。长对话/深下钻放宽到 24 步：
	// 上下文规模不再由「达标即终止」的 token 预算约束，改由运行时就地折叠（不停机）治理。
	defaultMaxIter = 24
	// defaultTokenBudget 默认 token 预算。设为 0（不限）：react 循环不再因累计计费 token 达标而被动终止，
	// 上下文规模改由三级压缩（就地折叠，见下）治理，避免长对话被过早掐断。
	defaultTokenBudget = 0

	// —— 三级上下文压缩阈值（业务策略层设定，能力层只提供机制）——
	// ① 单条消息：单条 token 超过该值 → 先压该条（保 result_id）。
	maxMessageTokens = 32000
	// ③ 整段对话：运行累计上下文 token ≥ 该值 → 就地折叠（不停机）。
	compactTriggerTokens = 128000
	// ③ 折叠目标水位：折叠后压回该 token 附近；也用作开场装配预算。
	compactTargetTokens = 64000
	// ③.5 陈旧思考剥离：DeepSeek thinking 默认开，中间每步的 reasoning 被契约强制回传后续所有轮，
	// 会在单次 ReAct 运行内堆积。「最近一轮之前」累计 reasoning ≥ 该值时把旧轮整轮折走（思考随之离场），
	// 只让最近一轮带 thinking——远早于 128k 触发线，避免陈旧思考在上下文里长期占位。
	reasoningEvictTokens = 16000
)

// capabilityGroups 是四类能力对应的工具分组（present-if-registered）：
// 查(sql/semantic/graph) / 析(analytics) / 渲(render) / 操(operation)，外加 skill 供 playbook 调用。
// render、operation 分组由后续 Phase 3/4 落地；此处按分组收集，工具就位后自动并入编排（通用/拓展）。
var capabilityGroups = []string{"sql", "semantic", "graph", "analytics", "render", "operation", "skill"}

// collectCapabilityTools 按能力分组收集全部已注册工具并按名去重。
func collectCapabilityTools(ctx context.Context, registry *toolregistry.ToolRegistry) []tool.BaseTool {
	seen := make(map[string]struct{})
	var tools []tool.BaseTool
	for _, group := range capabilityGroups {
		for _, t := range registry.GetByGroup(group) {
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

// Spec 返回单一 ReAct 内核 Data Agent 预设的声明式描述。
//
// Build 闭包在注册时装配：建子代理协作注册表 → 注册数据域子代理 → 建承载查/析/渲/操
// 四类能力的 ReAct 循环 Agent（入口意图路由 BeforeHook，maxIter + token 预算约束，
// 委派能力，可选跨轮对话记忆）。
// msgRepo 非 nil 时启用跨轮对话记忆：从 messages 表回放会话历史，装配多轮上下文。
func Spec(
	toolModel model.ToolCallingChatModel,
	msgRepo conversation.MessageRepository,
	registry *toolregistry.ToolRegistry,
	guardrail ...infraagent.GuardrailDecorator,
) infraagent.AgentSpec {
	// 组合根护栏装配器（可选变参，避免破坏既有 Spec 调用）；未传 → nil 恒等（零回归）。
	var gd infraagent.GuardrailDecorator
	if len(guardrail) > 0 {
		gd = guardrail[0]
	}
	return infraagent.AgentSpec{
		ID:          DataAgentID,
		Name:        "data_agent",
		Description: "Data Agent：单一 ReAct 内核，承载查/析/渲/操四类能力，入口意图路由 + maxIter/token 预算约束",
		Type:        domainagent.AgentTypeAgenticRAG,
		ToolNames:   capabilityGroups,
		Metadata: map[string]string{
			"version": "1.0.0",
			"pattern": "single-react",
			"kernel":  "reason-act-observe",
		},
		Build: func(ctx context.Context) (infraagent.Agent, error) {
			return buildDataAgent(ctx, toolModel, msgRepo, registry, gd)
		},
	}
}

// buildDataAgent 装配 Data Agent 实例：子代理协作注册表 + ReAct 内核。
// registry 为注入的工具注册表（组合根经 Spec 下发）；nil 时 fail-fast，不读任何包级默认槽位。
func buildDataAgent(
	ctx context.Context,
	toolModel model.ToolCallingChatModel,
	msgRepo conversation.MessageRepository,
	registry *toolregistry.ToolRegistry,
	guardrail infraagent.GuardrailDecorator,
) (infraagent.Agent, error) {
	if toolModel == nil {
		return nil, fmt.Errorf("data agent 预设需要 ToolCallingChatModel")
	}
	if registry == nil {
		return nil, fmt.Errorf("data agent 预设需要工具注册表")
	}

	// 子代理协作注册表：注册数据域子代理，供指挥官委派（每次委派授予最小 scope）。
	collabRegistry := infraagent.NewCollaborationRegistry()
	if err := RegisterDataSubAgents(ctx, collabRegistry, toolModel, registry); err != nil {
		return nil, fmt.Errorf("注册 Data 子代理失败: %w", err)
	}

	// 复杂任务下沉 skill（多维归因/经营报告）：幂等注册可执行 skill，handler 内经注入 ctx 的
	// collabRegistry inline 编排上述子代理群（复用委派内核与护栏），主循环只见一次 skill 调用。
	RegisterDataComplexSkills()

	tools := collectCapabilityTools(ctx, registry)
	if len(tools) == 0 {
		return nil, fmt.Errorf("data agent 预设未收集到任何能力工具（分组均未注册）")
	}

	builder := infraagent.New(nil).
		Name("DataAgent").
		Prompt(skills.AugmentPromptWithCatalog(systemPrompt)). // 追加技能目录（Level 1 渐进式披露）
		WithToolModel(toolModel).
		Tools(tools...).
		Before(toolPolicyHook()).               // 硬工具门：以原始消息匹配 skill，须先于 playbook 注入
		Before(intentRoutingHook(toolModel)).   // LLM 意图分类（词法兜底），注入对应 playbook
		Before(skills.InjectFromContextHook()). // 末位：把入口暂存的命中 skill 指导自动注入（自动注入）
		WithMaxIterations(defaultMaxIter).
		WithTokenBudget(defaultTokenBudget).
		WithContextCompaction(compactTriggerTokens, compactTargetTokens). // ③整段对话：累计≥128k就地折叠回~64k
		WithMaxMessageTokens(maxMessageTokens).                           // ①单条消息：>32k先压该条（保 result_id）
		WithReasoningEviction(reasoningEvictTokens).                      // ③.5陈旧思考：旧轮reasoning累计≥16k整轮折走，只留本轮thinking
		WithCollaboration(collabRegistry, infraagent.EnableDelegate())
	if msgRepo != nil {
		// 跨轮对话记忆：读 messages 表回放历史（与 UI 同源，只读不写），启用 framework 记忆分支。
		// 开场装配的超窗早期轮摘要注入 LLM 语义摘要器（llmsummary，自带抽取式降级），与运行时 ③
		// 折叠共用同一套语义压缩口径；toolModel 不可用时 llmsummary 内部退化为确定性抽取式。
		builder = builder.WithContextBuilder(
			convcontext.NewConversationContextBuilder(msgRepo).
				WithSummarizer(llmsummary.New(toolModel)),
		)
	}
	// 组合根护栏装配（默认关闭 → 恒等，零回归）：输入把关 + 逐工具/最终输出脱敏，
	// 与硬工具门/写审批叠加，构成 Data Agent 的多层安全缝。
	builder = guardrail.Apply(builder)

	reactAgent, err := builder.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("构建 Data Agent 失败: %w", err)
	}
	return reactAgent, nil
}
