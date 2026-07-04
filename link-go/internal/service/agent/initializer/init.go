// Package agentinit 提供 Agent 初始化功能
// 在这里定义并注册所有 Agent
package agentinit

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	infraagent "link/internal/service/agent/framework"
	dataagent "link/internal/service/agent/presets/data_agent"
	"link/internal/service/agent/presets/text2sql"
	toolregistry "link/internal/service/agent/tools"
	"link/internal/model/agent"
)

// Initializer Agent 初始化器
type Initializer struct {
	registry agent.AgentRegistry
}

// NewInitializer 创建初始化器
func NewInitializer(registry agent.AgentRegistry) *Initializer {
	return &Initializer{
		registry: registry,
	}
}

// Initialize 初始化所有 Agent
// chatModel: LLM 模型，支持 ChatModel 或 ToolCallingChatModel
func (init *Initializer) Initialize(ctx context.Context, chatModel any) error {
	log.Println("[========== Agent 初始化开始 ============]")

	// 0. 注册默认 Agent（优先）
	if err := init.registerDefaultAgent(ctx, chatModel); err != nil {
		return fmt.Errorf("注册默认 Agent 失败: %w", err)
	}

	// 1. 注册 RAG Agent
	if err := init.registerRAGAgent(ctx, chatModel); err != nil {
		return fmt.Errorf("注册 RAG Agent 失败: %w", err)
	}

	// 2. 注册 Text2SQL Agent
	if err := init.registerText2SQLAgent(ctx, chatModel); err != nil {
		log.Printf("⚠️  注册 Text2SQL Agent 失败: %v", err)
	}

	// 3. 注册简单聊天 Agent
	if err := init.registerChatAgent(ctx, chatModel); err != nil {
		return fmt.Errorf("注册 Chat Agent 失败: %w", err)
	}

	// 4. 注册 Data Agent（单一 ReAct 内核）
	if err := init.registerDataAgent(ctx, chatModel); err != nil {
		log.Printf("⚠️  注册 Data Agent 失败: %v", err)
	}

	log.Println("[========== ✅ Agent 初始化完成 ============]")
	return nil
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

	// 从全局注册表获取工具
	var tools []tool.BaseTool

	// 添加 web_search 工具
	if t, ok := toolregistry.GetTool("web_search"); ok {
		tools = append(tools, t)
	}

	// 添加 sql_execute 工具（取代旧的 data_query）
	if t, ok := toolregistry.GetTool("sql_execute"); ok {
		tools = append(tools, t)
		log.Println("[Agent] ✓ sql_execute 工具已添加")
	}

	// 构建默认 Agent（带工具能力）
	builder := infraagent.New(nil).
		Name("默认助手").
		Prompt(`你是一个有帮助的 AI 助手。

你的任务：
- 回答用户的各种问题
- 提供帮助和建议
- 保持对话友好和连贯
- 可以使用工具来获取更准确的信息

可用工具：
- web_search: 网络搜索，获取最新信息
- sql_execute: 数据库查询，执行只读 SQL 查询`).
		WithToolModel(toolModel).
		WithMaxIterations(5) // 支持多轮工具调用

	if len(tools) > 0 {
		builder = builder.Tools(tools...)
	}

	defaultAgent, err := builder.Build(ctx)

	if err != nil {
		return fmt.Errorf("构建默认 Agent 失败: %w", err)
	}

	// 构建工具列表字符串
	toolsList := "web_search"
	if _, ok := toolregistry.GetTool("sql_execute"); ok {
		toolsList += ", sql_execute"
	}

	// 注册到注册中心
	def := &agent.AgentDefinition{
		ID:          "default",
		Name:        "默认助手",
		Description: "默认对话 Agent，支持工具调用",
		Type:        agent.AgentTypeNormal,
		Status:      agent.AgentStatusIdle,
		Metadata: map[string]string{
			"builtin": "true",
			"version": "1.0.0",
			"tools":   toolsList,
		},
	}

	if err := init.registry.Register(ctx, def); err != nil {
		return err
	}

	// 存储 Agent 实例
	SetDefaultAgent(defaultAgent)

	log.Println("[Agent] ✓ Default agent registered: id=default, type=normal")
	return nil
}

// registerRAGAgent 注册 RAG Agent
func (init *Initializer) registerRAGAgent(ctx context.Context, chatModel any) error {
	// 从全局注册表获取工具：
	// rag_query（文档检索）为核心；kb_list（查看可用知识库）与 graph_query（关系检索，受图谱开关门控）为增强。
	ragTool, ok := toolregistry.GetTool("rag_query")
	if !ok || ragTool == nil {
		return fmt.Errorf("获取 RAG 工具失败: rag_query 未注册")
	}

	agentTools := []tool.BaseTool{ragTool}
	boundNames := []string{"rag_query"}
	if kbListTool, ok := toolregistry.GetTool("kb_list"); ok && kbListTool != nil {
		agentTools = append(agentTools, kbListTool)
		boundNames = append(boundNames, "kb_list")
	}
	if kbRouteTool, ok := toolregistry.GetTool("kb_route"); ok && kbRouteTool != nil {
		agentTools = append(agentTools, kbRouteTool)
		boundNames = append(boundNames, "kb_route")
	}
	if graphTool, ok := toolregistry.GetTool("graph_query"); ok && graphTool != nil {
		agentTools = append(agentTools, graphTool)
		boundNames = append(boundNames, "graph_query")
	}

	// 尝试将 chatModel 转换为 ToolCallingChatModel
	var toolModel model.ToolCallingChatModel
	if tc, ok := chatModel.(model.ToolCallingChatModel); ok {
		toolModel = tc
	} else {
		return fmt.Errorf("chatModel 不支持工具调用，RAG Agent 需要 ToolCallingChatModel")
	}

	// 构建 RAG Agent
	ragAgent, err := infraagent.New(nil).
		Name("RAG助手").
		Prompt(`你是一个严谨的知识库问答助手。回答问题必须基于知识库检索到的内容，不得编造。

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
3. 答案简洁准确，必要时分点说明。`).
		WithToolModel(toolModel).
		Tools(agentTools...).
		WithMaxIterations(5).
		Build(ctx)

	if err != nil {
		return fmt.Errorf("构建 RAG Agent 失败: %w", err)
	}

	// 注册到注册中心
	def := &agent.AgentDefinition{
		ID:          "agent-rag-001",
		Name:        "rag_assistant",
		Description: "RAG 检索助手：在用户选定的知识库范围内检索文档，可选开启图谱增强进行关系检索",
		Type:        agent.AgentTypeAgenticRAG,
		Status:      agent.AgentStatusIdle,
		Metadata: map[string]string{
			"version": "1.0.0",
			"tools":   strings.Join(boundNames, ","),
		},
	}

	if err := init.registry.Register(ctx, def); err != nil {
		return err
	}

	// 存储 Agent 实例
	SetRAGAgent(ragAgent)

	log.Println("[Agent] ✓ RAG agent registered: id=agent-rag-001, type=agentic_rag")
	return nil
}

// registerText2SQLAgent 注册 Text2SQL Agent (Plan-Execute-Reflect 模式)
func (init *Initializer) registerText2SQLAgent(ctx context.Context, chatModel any) error {
	// 类型断言：获取 ToolCallingChatModel
	var toolModel model.ToolCallingChatModel
	if tm, ok := chatModel.(model.ToolCallingChatModel); ok {
		toolModel = tm
	} else {
		return fmt.Errorf("invalid model type: expected ToolCallingChatModel")
	}

	// 使用新的 PER 模式注册
	if err := text2sql.RegisterText2SQLAgent(ctx, init.registry, toolModel); err != nil {
		return err
	}

	log.Println("[Agent] ✓ Text2SQL PER agent registered: id=agent-text2sql-per, pattern=sequential+retry")
	return nil
}

// registerDataAgent 注册 Data Agent（单一 ReAct 内核，查/析/渲/操四类能力）
func (init *Initializer) registerDataAgent(ctx context.Context, chatModel any) error {
	var toolModel model.ToolCallingChatModel
	if tm, ok := chatModel.(model.ToolCallingChatModel); ok {
		toolModel = tm
	} else {
		return fmt.Errorf("invalid model type: expected ToolCallingChatModel")
	}

	// Phase 7：先注册数据域子代理（orchestrator-worker 协作注册表 + 治理目录），
	// 再把注册表交给指挥官启用委派能力。
	collabRegistry := infraagent.NewCollaborationRegistry()
	if err := dataagent.RegisterDataSubAgents(ctx, collabRegistry, toolModel); err != nil {
		return fmt.Errorf("注册数据域子代理失败: %w", err)
	}
	log.Printf("[Agent] ✓ Data sub-agents registered: %v", collabRegistry.List())

	if err := dataagent.RegisterDataAgentPreset(ctx, init.registry, toolModel, collabRegistry); err != nil {
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

	// 构建简单聊天 Agent
	chatAgent, err := infraagent.New(baseModel).
		Name("聊天助手").
		Prompt(`你是一个友好的 AI 助手。

你的任务：
- 回答用户的各种问题
- 提供帮助和建议
- 保持对话友好和连贯`).
		Build(ctx)

	if err != nil {
		return fmt.Errorf("构建 Chat Agent 失败: %w", err)
	}

	// 注册到注册中心
	def := &agent.AgentDefinition{
		ID:          "agent-chat-001",
		Name:        "chat_assistant",
		Description: "友好聊天助手，可以回答各类问题",
		Type:        agent.AgentTypeNormal,
		Status:      agent.AgentStatusIdle,
		Metadata: map[string]string{
			"version": "1.0.0",
			"tools":   "",
		},
	}

	if err := init.registry.Register(ctx, def); err != nil {
		return err
	}

	// 存储 Agent 实例
	SetChatAgent(chatAgent)

	log.Println("[Agent] ✓ Chat agent registered: id=agent-chat-001, type=normal")
	return nil
}

// ========================================
// Agent 实例存储（单例模式）
// ========================================

var (
	defaultAgentInstance    infraagent.Agent
	ragAgentInstance        infraagent.Agent
	text2sqlAgentInstance   infraagent.Agent
	chatAgentInstance       infraagent.Agent
)

// SetDefaultAgent 设置默认 Agent 实例
func SetDefaultAgent(agent infraagent.Agent) {
	defaultAgentInstance = agent
}

// GetDefaultAgent 获取默认 Agent 实例
func GetDefaultAgent() infraagent.Agent {
	return defaultAgentInstance
}

// SetRAGAgent 设置 RAG Agent 实例
func SetRAGAgent(agent infraagent.Agent) {
	ragAgentInstance = agent
}

// GetRAGAgent 获取 RAG Agent 实例
func GetRAGAgent() infraagent.Agent {
	return ragAgentInstance
}

// SetChatAgent 设置聊天 Agent 实例
func SetChatAgent(agent infraagent.Agent) {
	chatAgentInstance = agent
}

// GetChatAgent 获取聊天 Agent 实例
func GetChatAgent() infraagent.Agent {
	return chatAgentInstance
}

// GetText2SQLAgent 获取 Text2SQL Agent 实例
// 优先返回新的 PER 模式 Agent
func GetText2SQLAgent() infraagent.Agent {
	// 优先返回新版本的 PER Agent
	if agent := text2sql.GetAgent(); agent != nil {
		return agent
	}
	// 降级到旧版本
	return text2sqlAgentInstance
}

// SetText2SQLAgent 设置 Text2SQL Agent 实例
func SetText2SQLAgent(agent infraagent.Agent) {
	text2sqlAgentInstance = agent
}

// ========================================
// Agent 获取器（用于 Orchestrator）
// ========================================

// GetAgentByID 根据 agentID 获取对应的 Agent 实例
// 用于 Orchestrator 动态获取 Agent
func GetAgentByID(agentID string) (infraagent.Agent, bool) {
	switch agentID {
	case "default":
		if defaultAgentInstance != nil {
			return defaultAgentInstance, true
		}
	case "agent-rag-001":
		if ragAgentInstance != nil {
			return ragAgentInstance, true
		}
	case "agent-text2sql-per", "agent-text2sql-001":
		// 优先返回 PER 模式 Agent
		if agent := text2sql.GetAgent(); agent != nil {
			return agent, true
		}
		if text2sqlAgentInstance != nil {
			return text2sqlAgentInstance, true
		}
	case "agent-chat-001":
		if chatAgentInstance != nil {
			return chatAgentInstance, true
		}
	case dataagent.DataAgentID:
		if agent := dataagent.GetAgent(); agent != nil {
			return agent, true
		}
	}
	return nil, false
}
