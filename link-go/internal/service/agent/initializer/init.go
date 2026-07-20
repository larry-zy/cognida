// Package agentinit 提供 Agent 初始化功能
// 在这里定义并注册所有 Agent
package agentinit

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	"link/internal/model/agent"
	"link/internal/model/conversation"
	"link/internal/service/agent/convcontext"
	infraagent "link/internal/service/agent/framework"
	dataagent "link/internal/service/agent/presets/data_agent"
	"link/internal/service/agent/skills"
	toolregistry "link/internal/service/agent/tools"
)

// Initializer Agent 初始化器
type Initializer struct {
	// registry 是声明式 Agent 注册表：RegisterSpec 按 AgentSpec 构建实例并登记元信息，
	// 取代了此前各包的 Set*Agent 单例 + GetAgentByID switch。
	registry *infraagent.SpecRegistry
	// tools 是注入的工具注册表：各 Agent 构建时按名/分组解析工具，取代 tools 包级默认槽位。
	tools *toolregistry.ToolRegistry
	// messageRepo 供 Data Agent 启用跨轮对话记忆（读 messages 表回放历史）；nil 时记忆退化。
	messageRepo conversation.MessageRepository
	// guardrail 是组合根下发的护栏装配器：按会话/agent 级开关把护栏 Hook 装配进各 Agent 的
	// Builder。默认 nil（恒等装配）→ 所有 Agent 不获得任何护栏 Hook，构建行为逐字节不变（零回归）。
	guardrail infraagent.GuardrailDecorator
	// embedder 是组合根下发的向量化组件：供 Data Agent 反思记忆（自我进化）向量化任务/教训。
	// 默认 nil → data_agent 反思退化为「仅评估不沉淀」，不影响主流程（零回归）。
	embedder embedding.Embedder
}

// NewInitializer 创建初始化器。
// tools 为注入的工具注册表（组合根构造后传入），各 Agent 构建时经它解析工具。
// messageRepo 可选：传入后 Data Agent 具备跨轮对话记忆；不传则保持原有无记忆行为
// （变参形式避免破坏既有测试构造调用）。
func NewInitializer(registry *infraagent.SpecRegistry, tools *toolregistry.ToolRegistry, messageRepo ...conversation.MessageRepository) *Initializer {
	init := &Initializer{
		registry: registry,
		tools:    tools,
	}
	if len(messageRepo) > 0 {
		init.messageRepo = messageRepo[0]
	}
	return init
}

// WithGuardrail 挂接组合根护栏装配器（会话/agent 级开关，默认全关）。
// 传入 nil 或未调用 → 恒等装配，所有 Agent 不获得任何护栏 Hook（零回归）。
// 返回自身以支持链式调用。
func (init *Initializer) WithGuardrail(decorator infraagent.GuardrailDecorator) *Initializer {
	init.guardrail = decorator
	return init
}

// WithEmbedder 挂接组合根下发的向量化组件，供 Data Agent 反思记忆接线自我进化闭环。
// 传入 nil 或未调用 → data_agent 反思无记忆（仅评估、不检索/沉淀经验），零回归。
// 返回自身以支持链式调用。
func (init *Initializer) WithEmbedder(embedder embedding.Embedder) *Initializer {
	init.embedder = embedder
	return init
}

// Initialize 初始化所有 Agent
// chatModel: LLM 模型，支持 ChatModel 或 ToolCallingChatModel
func (init *Initializer) Initialize(ctx context.Context, chatModel any) error {
	log.Println("[========== Agent 初始化开始 ============]")

	// 先加载 Skill 定义到全局注册表，再注册各 Agent——保证所有 Agent 构建时
	// AutoInjectHook / 硬工具门都能匹配到已加载的 Skill。加载失败不阻断启动（降级为无 Skill）。
	if err := skills.InitializeFromEnv(); err != nil {
		log.Printf("⚠️  Skill 系统初始化失败（降级为无 Skill 运行）: %v", err)
	} else {
		log.Printf("[Agent] ✓ Skill 系统就绪，已加载 %d 个技能", len(skills.ListAllSkills()))
	}

	// 0. 注册默认 Agent（优先）
	if err := init.registerDefaultAgent(ctx, chatModel); err != nil {
		return fmt.Errorf("注册默认 Agent 失败: %w", err)
	}

	// 1. 注册 RAG Agent
	if err := init.registerRAGAgent(ctx, chatModel); err != nil {
		return fmt.Errorf("注册 RAG Agent 失败: %w", err)
	}

	// 2. 注册简单聊天 Agent
	if err := init.registerChatAgent(ctx, chatModel); err != nil {
		return fmt.Errorf("注册 Chat Agent 失败: %w", err)
	}

	// 3. 注册 Data Agent（单一 ReAct 内核）
	if err := init.registerDataAgent(ctx, chatModel); err != nil {
		log.Printf("⚠️  注册 Data Agent 失败: %v", err)
	}

	log.Println("[========== ✅ Agent 初始化完成 ============]")
	return nil
}

// skillTools 返回注入注册表中 "skill" 分组的工具（skill_list/skill_invoke/skill_match），
// 供各 Agent 在渐进式披露下按需加载技能完整指导（Level 2）。分组为空时返回 nil（安全降级）。
func (init *Initializer) skillTools() []tool.BaseTool {
	return init.tools.GetByGroup("skill")
}

// registerDefaultAgent 注册默认 Agent（带工具支持）
func (init *Initializer) registerDefaultAgent(ctx context.Context, chatModel any) error {
	// 类型断言：获取 ToolCallingChatModel 以支持工具调用
	var toolModel model.ToolCallingChatModel
	if tm, ok := chatModel.(model.ToolCallingChatModel); ok {
		toolModel = tm
	} else {
		return fmt.Errorf("invalid model type: expected ToolCallingChatModel for default agent")
	}

	// 元信息中的工具清单（存在即列出）
	toolsList := "web_search"
	if _, ok := init.tools.Get("sql_execute"); ok {
		toolsList += ", sql_execute"
	}

	spec := infraagent.AgentSpec{
		ID:          "default",
		Name:        "默认助手",
		Description: "默认对话 Agent，支持工具调用",
		Type:        agent.AgentTypeNormal,
		Metadata: map[string]string{
			"builtin": "true",
			"version": "1.0.0",
			"tools":   toolsList,
		},
		Build: func(ctx context.Context) (infraagent.Agent, error) {
			// 从注入注册表获取工具
			var tools []tool.BaseTool
			if t, ok := init.tools.Get("web_search"); ok {
				tools = append(tools, t)
			}
			if t, ok := init.tools.Get("sql_execute"); ok {
				tools = append(tools, t)
			}
			// 挂载 skill 工具（skill_invoke 等），支持 LLM 按需加载技能完整指导（Level 2）
			tools = append(tools, init.skillTools()...)

			// 构建默认 Agent（带工具能力）。system prompt 追加技能目录（Level 1 渐进式披露）。
			builder := infraagent.New(nil).
				Name("默认助手").
				Prompt(skills.AugmentPromptWithCatalog(`你是一个有帮助的 AI 助手。

你的任务：
- 回答用户的各种问题
- 提供帮助和建议
- 保持对话友好和连贯
- 可以使用工具来获取更准确的信息

可用工具：
- web_search: 网络搜索，获取最新信息
- sql_execute: 数据库查询，执行只读 SQL 查询`)).
				WithToolModel(toolModel).
				Before(skills.AutoInjectHook(skills.FallbackInjectThreshold)). // 兜底：极高置信命中才词法注入，否则交由 LLM+skill_invoke
				WithMaxIterations(5)                                           // 支持多轮工具调用
			if len(tools) > 0 {
				builder = builder.Tools(tools...)
			}
			// 组合根护栏装配（默认关闭 → 恒等，零回归）。
			builder = init.guardrail.Apply(builder)
			return builder.Build(ctx)
		},
	}

	if err := init.registry.RegisterSpec(ctx, spec); err != nil {
		return err
	}

	log.Println("[Agent] ✓ Default agent registered: id=default, type=normal")
	return nil
}

// registerRAGAgent 注册 RAG Agent
func (init *Initializer) registerRAGAgent(ctx context.Context, chatModel any) error {
	// rag_query 为核心工具，注册前先校验其存在（必需依赖 fail-fast）。
	if ragTool, ok := init.tools.Get("rag_query"); !ok || ragTool == nil {
		return fmt.Errorf("获取 RAG 工具失败: rag_query 未注册")
	}

	// 尝试将 chatModel 转换为 ToolCallingChatModel
	var toolModel model.ToolCallingChatModel
	if tc, ok := chatModel.(model.ToolCallingChatModel); ok {
		toolModel = tc
	} else {
		return fmt.Errorf("chatModel 不支持工具调用，RAG Agent 需要 ToolCallingChatModel")
	}

	// 元信息中的工具清单：核心 rag_query + 已注册的增强工具（存在即列出）。
	boundNames := []string{"rag_query"}
	for _, name := range []string{"kb_list", "kb_route", "graph_query"} {
		if t, ok := init.tools.Get(name); ok && t != nil {
			boundNames = append(boundNames, name)
		}
	}

	spec := infraagent.AgentSpec{
		ID:          "agent-rag-001",
		Name:        "rag_assistant",
		Description: "RAG 检索助手：在用户选定的知识库范围内检索文档，可选开启图谱增强进行关系检索",
		Type:        agent.AgentTypeAgenticRAG,
		Metadata: map[string]string{
			"version": "1.0.0",
			"tools":   strings.Join(boundNames, ","),
		},
		Build: func(ctx context.Context) (infraagent.Agent, error) {
			return init.buildRAGAgent(ctx, toolModel)
		},
	}

	if err := init.registry.RegisterSpec(ctx, spec); err != nil {
		return err
	}

	log.Println("[Agent] ✓ RAG agent registered: id=agent-rag-001, type=agentic_rag")
	return nil
}

// buildRAGAgent 装配 RAG Agent 实例：按名取工具（核心 rag_query + 增强 kb_*/graph_query）→ 建 Agent。
func (init *Initializer) buildRAGAgent(ctx context.Context, toolModel model.ToolCallingChatModel) (infraagent.Agent, error) {
	// rag_query（文档检索）为核心；kb_list（查看可用知识库）与 graph_query（关系检索，受图谱开关门控）为增强。
	ragTool, ok := init.tools.Get("rag_query")
	if !ok || ragTool == nil {
		return nil, fmt.Errorf("获取 RAG 工具失败: rag_query 未注册")
	}

	agentTools := []tool.BaseTool{ragTool}
	if kbListTool, ok := init.tools.Get("kb_list"); ok && kbListTool != nil {
		agentTools = append(agentTools, kbListTool)
	}
	if kbRouteTool, ok := init.tools.Get("kb_route"); ok && kbRouteTool != nil {
		agentTools = append(agentTools, kbRouteTool)
	}
	if graphTool, ok := init.tools.Get("graph_query"); ok && graphTool != nil {
		agentTools = append(agentTools, graphTool)
	}
	// 挂载 skill 工具，支持按需加载技能完整指导（Level 2）
	agentTools = append(agentTools, init.skillTools()...)

	// 构建 RAG Agent。system prompt 追加技能目录（Level 1 渐进式披露）。
	ragBuilder := infraagent.New(nil).
		Name("RAG助手").
		Prompt(skills.AugmentPromptWithCatalog(`你是一个严谨的知识库问答助手。回答问题必须基于知识库检索到的内容，不得编造。

【检索范围与选择模式】
- 检索范围受「知识库选择模式」约束，共三种：手动(manual) / 结合(hybrid) / 智能(auto)。
- 你无法、也无需在 rag_query / graph_query 参数中指定 kb_id；系统始终在允许范围内强制检索。
- 手动模式：范围由用户锁定，直接检索即可，无需选库。
- 结合 / 智能模式：可由你自主聚焦到最相关的知识库——先用 kb_list 查看返回中的 mode 与各库 selectable，
  再从 selectable=true 的库里挑出 1~N 个最相关的，调用 kb_route 声明；随后 rag_query / graph_query 会在该聚焦范围内检索。
  越权/超范围的库会被系统忽略；不确定选哪些时可以不 kb_route，系统会按允许范围全量检索。

【工具与使用时机】
1. rag_query —— 主力工具。用户询问文档内容、概念、操作指南、专业知识时使用。
   支持向量/关键词/混合检索及 HyDE、查询重写/扩展、多跳等优化，复杂问题可开启对应优化。
2. graph_query —— 关系检索工具，仅在用户开启「图谱增强」时可用。
   适合"谁负责什么""A 和 B 有何关联""下游依赖有哪些"等关系/关联/溯源类问题。
   若图谱未开启，该工具会返回提示——此时改用 rag_query，不要反复调用。
3. kb_list —— 查看当前可用知识库及选择模式(mode)、各库是否可聚焦(selectable)。
   当用户问"有哪些知识库"、检索为空需说明范围、或在结合/智能模式下准备 kb_route 选库前使用。
4. kb_route —— 结合/智能模式下声明你要聚焦的知识库（kb_ids 取自 kb_list 中 selectable=true 的库）。
   手动模式下该工具会被忽略，不必调用。

【回答要求】
1. 先检索、后作答；优先 rag_query，关系类问题在图谱开启时用 graph_query。
2. 基于检索结果作答，标注信息来源；检索不到相关信息时如实告知，不要编造。
3. 答案简洁准确，必要时分点说明。`)).
		WithToolModel(toolModel).
		Tools(agentTools...).
		Before(skills.AutoInjectHook(skills.FallbackInjectThreshold)). // 兜底：极高置信命中才词法注入，否则交由 LLM+skill_invoke
		WithMaxIterations(5)
	// 跨轮对话记忆：读 messages 表回放会话历史（与 UI 同源、只读不写），启用 framework 记忆分支，
	// 使知识库助手支持"继续""那第二点呢"等依赖上文的追问。与 Data Agent 同一套基建。
	if init.messageRepo != nil {
		ragBuilder = ragBuilder.WithContextBuilder(convcontext.NewConversationContextBuilder(init.messageRepo))
	}
	// 组合根护栏装配（默认关闭 → 恒等，零回归）。
	ragBuilder = init.guardrail.Apply(ragBuilder)
	ragAgent, err := ragBuilder.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("构建 RAG Agent 失败: %w", err)
	}
	return ragAgent, nil
}

// registerDataAgent 注册 Data Agent（单一 ReAct 内核，查/析/渲/操四类能力）
func (init *Initializer) registerDataAgent(ctx context.Context, chatModel any) error {
	var toolModel model.ToolCallingChatModel
	if tm, ok := chatModel.(model.ToolCallingChatModel); ok {
		toolModel = tm
	} else {
		return fmt.Errorf("invalid model type: expected ToolCallingChatModel")
	}

	// 声明式注册：预设 Build 工厂内部装配子代理协作注册表 + ReAct 内核 + 委派能力，
	// msgRepo 非 nil 时启用跨轮对话记忆。
	if err := init.registry.RegisterSpec(ctx, dataagent.Spec(toolModel, init.messageRepo, init.tools, init.guardrail)); err != nil {
		return err
	}

	log.Printf("[Agent] ✓ Data agent registered: id=%s, pattern=single-react+delegate", dataagent.DataAgentID)
	return nil
}

// registerChatAgent 注册简单聊天 Agent
func (init *Initializer) registerChatAgent(ctx context.Context, chatModel any) error {
	// 类型断言：使用 BaseChatModel（ChatModel 已废弃）
	var baseModel model.BaseChatModel
	if bm, ok := chatModel.(model.BaseChatModel); ok {
		baseModel = bm
	} else {
		return fmt.Errorf("invalid model type: expected BaseChatModel")
	}

	spec := infraagent.AgentSpec{
		ID:          "agent-chat-001",
		Name:        "chat_assistant",
		Description: "友好聊天助手，可以回答各类问题",
		Type:        agent.AgentTypeNormal,
		Metadata: map[string]string{
			"version": "1.0.0",
			"tools":   "",
		},
		Build: func(ctx context.Context) (infraagent.Agent, error) {
			// 构建简单聊天 Agent
			chatBuilder := infraagent.New(baseModel).
				Name("聊天助手").
				Prompt(`你是一个友好的 AI 助手。

你的任务：
- 回答用户的各种问题
- 提供帮助和建议
- 保持对话友好和连贯`).
				Before(skills.AutoInjectHook(0)) // 命中 Skill 时自动注入其指导内容
			// 组合根护栏装配（默认关闭 → 恒等，零回归）。
			chatBuilder = init.guardrail.Apply(chatBuilder)
			return chatBuilder.Build(ctx)
		},
	}

	if err := init.registry.RegisterSpec(ctx, spec); err != nil {
		return err
	}

	log.Println("[Agent] ✓ Chat agent registered: id=agent-chat-001, type=normal")
	return nil
}
