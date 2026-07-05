package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"link/internal/model/agent"
	agentreflection "link/internal/model/agent/reflection"
	"link/internal/service/agent/framework/hooks"
	reflecthooks "link/internal/service/agent/framework/reflection"
)

// Builder provides a fluent API for constructing Agent instances.
type Builder struct {
	model             model.BaseChatModel // 使用 BaseChatModel（ChatModel 已废弃）
	toolModel         model.ToolCallingChatModel // 用于工具调用
	name              string
	description       string
	prompt            string
	tools             []tool.BaseTool
	beforeHooks       []BeforeHook
	afterHooks        []AfterHook
	middleware        []Middleware
	registry          ToolRegistry
	ragService        RAGService
	memory            Memory
	sessionID         string
	autoSelect        bool
	maxIter           int // 最大迭代次数
	tokenBudget       int // token 预算（0 表示不限），与 maxIter 共同约束 ReAct 循环
	collabRegistry    *CollaborationRegistry
	collabConfig      *CollaborationConfig

	// Memory and Context Builder support (Phase 6)
	memoryService   MemoryService  // 记忆服务接口
	contextBuilder  ContextBuilder // 上下文构建器接口
	enableMemory    bool           // 是否启用记忆功能
}

// CollaborationConfig defines which collaboration tools are enabled.
type CollaborationConfig struct {
	EnableDelegate bool
	EnableAsk      bool
	EnableHandoff  bool
}

// CollaborationOption is a function that configures collaboration settings.
type CollaborationOption func(*CollaborationConfig)

// ToolRegistry defines the interface for accessing tool registry.
type ToolRegistry interface {
	GetTools() []tool.BaseTool
	GetToolsByNames(names []string) ([]tool.BaseTool, error)
	List() []string
	GetEinoTool(toolName string) (tool.BaseTool, bool)
	IsEnabled(toolName string) bool
}

// RAGService defines the interface for RAG capabilities.
type RAGService interface {
	// RAG retrieval methods will be defined by the specific implementation
}

// Memory defines the interface for session/memory management.
type Memory interface {
	LoadHistory(ctx context.Context, sessionID string) ([]*schema.Message, error)
	SaveMessage(ctx context.Context, sessionID string, message *schema.Message) error
}

// Name sets the agent's name.
func (b *Builder) Name(name string) *Builder {
	b.name = name
	return b
}

// Description sets the agent's description.
func (b *Builder) Description(desc string) *Builder {
	b.description = desc
	return b
}

// Prompt sets the system prompt for the agent.
func (b *Builder) Prompt(prompt string) *Builder {
	b.prompt = prompt
	return b
}

// Tools adds specific tools to the agent.
func (b *Builder) Tools(tools ...tool.BaseTool) *Builder {
	b.tools = append(b.tools, tools...)
	return b
}

// ToolsFromRegistry loads tools from the registry by name.
func (b *Builder) ToolsFromRegistry(names ...string) *Builder {
	if b.registry != nil {
		tools, err := b.registry.GetToolsByNames(names)
		if err == nil {
			b.tools = append(b.tools, tools...)
		}
	}
	return b
}

// ToolsAutoSelect enables automatic tool selection by the LLM.
// All tools from the registry will be made available to the LLM.
func (b *Builder) ToolsAutoSelect() *Builder {
	b.autoSelect = true
	return b
}

// Before adds a hook that runs before the agent processes a message.
func (b *Builder) Before(hook BeforeHook) *Builder {
	b.beforeHooks = append(b.beforeHooks, hook)
	return b
}

// After adds a hook that runs after the agent generates a response.
func (b *Builder) After(hook AfterHook) *Builder {
	b.afterHooks = append(b.afterHooks, hook)
	return b
}

// Middleware adds middleware to the agent.
func (b *Builder) Middleware(mw ...Middleware) *Builder {
	b.middleware = append(b.middleware, mw...)
	return b
}

// WithRegistry sets the tool registry for this agent.
func (b *Builder) WithRegistry(registry ToolRegistry) *Builder {
	b.registry = registry
	return b
}

// WithRAG enables RAG capabilities for this agent.
func (b *Builder) WithRAG(ragService RAGService) *Builder {
	b.ragService = ragService
	return b
}

// WithMemory sets the memory for this agent.
func (b *Builder) WithMemory(memory Memory) *Builder {
	b.memory = memory
	return b
}

// WithMemoryService sets the memory service for this agent (Phase 6 - 协作记忆支持).
// MemoryService 提供更强大的记忆管理，包括消息保存、历史加载、摘要更新等。
func (b *Builder) WithMemoryService(memoryService MemoryService) *Builder {
	b.memoryService = memoryService
	b.enableMemory = true
	return b
}

// WithContextBuilder sets the context builder for this agent (Phase 6 - 协作上下文构建).
// ContextBuilder 负责根据不同的协作模式构建 LLM 上下文。
// 设置 ContextBuilder 即启用记忆分支（enableMemory），使统一主干 run 的 buildInitialMessages
// 用装配后的多轮上下文替换默认的 [系统提示 + 当前问题]，从而具备跨轮对话记忆。
func (b *Builder) WithContextBuilder(contextBuilder ContextBuilder) *Builder {
	b.contextBuilder = contextBuilder
	b.enableMemory = true
	return b
}

// WithSession sets the session ID for this agent.
func (b *Builder) WithSession(sessionID string) *Builder {
	b.sessionID = sessionID
	return b
}

// WithMaxIterations sets the maximum number of iterations for tool calling.
func (b *Builder) WithMaxIterations(maxIter int) *Builder {
	b.maxIter = maxIter
	return b
}

// WithTokenBudget 设置 ReAct 循环的 token 预算（累计生成消耗达到该值即终止并收尾）。
// 传入 <=0 表示不限预算，仅受 maxIter 约束。
func (b *Builder) WithTokenBudget(tokenBudget int) *Builder {
	b.tokenBudget = tokenBudget
	return b
}

// WithToolModel sets the ToolCallingChatModel for tool calling support.
// 如果不设置，有工具时会尝试将 model 转换为 ToolCallingChatModel。
func (b *Builder) WithToolModel(toolModel model.ToolCallingChatModel) *Builder {
	b.toolModel = toolModel
	return b
}

// WithCollaboration enables LLM-callable collaboration tools for multi-agent systems.
// This allows the agent to delegate tasks, ask questions, or handoff control to other agents.
func (b *Builder) WithCollaboration(registry *CollaborationRegistry, opts ...CollaborationOption) *Builder {
	b.collabRegistry = registry
	b.collabConfig = &CollaborationConfig{
		EnableDelegate: false,
		EnableAsk:      false,
		EnableHandoff:  false,
	}
	for _, opt := range opts {
		opt(b.collabConfig)
	}
	return b
}

// ========================================
// Collaboration Option Functions
// ========================================

// EnableDelegate enables the delegate tool.
func EnableDelegate() CollaborationOption {
	return func(cfg *CollaborationConfig) {
		cfg.EnableDelegate = true
	}
}

// EnableAsk enables the ask tool.
func EnableAsk() CollaborationOption {
	return func(cfg *CollaborationConfig) {
		cfg.EnableAsk = true
	}
}

// EnableHandoff enables the handoff tool.
func EnableHandoff() CollaborationOption {
	return func(cfg *CollaborationConfig) {
		cfg.EnableHandoff = true
	}
}

// EnableAllCollaboration enables all collaboration tools.
func EnableAllCollaboration() CollaborationOption {
	return func(cfg *CollaborationConfig) {
		cfg.EnableDelegate = true
		cfg.EnableAsk = true
		cfg.EnableHandoff = true
	}
}

// WithConclusion 配置数据结论生成 Hook。
// 结论生成器会在 Agent 响应后检测数据工具调用，并使用 LLM 分析结果生成结构化结论。
func (b *Builder) WithConclusion(generator *hooks.ConclusionGenerator) *Builder {
	// 将 ConclusionGenerator 的 Hook 方法转换为 AfterHook
	afterHook := func(ctx context.Context, resp *Response) error {
		// 将 Response 转换为 map[string]interface{} 格式
		respMap := make(map[string]interface{})
		respMap["content"] = resp.Content
		if len(resp.ToolCalls) > 0 {
			toolCalls := make([]map[string]interface{}, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				toolCalls[i] = map[string]interface{}{
					"name":   tc.Name,
					"input":  tc.Input,
					"output": tc.Output,
				}
				if tc.Error != nil {
					toolCalls[i]["error"] = tc.Error.Error()
				}
			}
			respMap["tool_calls"] = toolCalls
		}
		if resp.Metadata != nil {
			respMap["metadata"] = resp.Metadata
		}

		// 调用 Hook
		hook := generator.Hook()
		if err := hook(ctx, respMap); err != nil {
			return err
		}

		// 将处理结果写回 Response
		if content, ok := respMap["content"].(string); ok {
			resp.Content = content
		}
		if metadata, ok := respMap["metadata"].(map[string]interface{}); ok {
			if resp.Metadata == nil {
				resp.Metadata = make(map[string]interface{})
			}
			for k, v := range metadata {
				resp.Metadata[k] = v
			}
		}

		return nil
	}
	b.afterHooks = append(b.afterHooks, afterHook)
	return b
}

// WithClarification 配置意图澄清 Hook。
// 意图澄清器会在处理查询前分析清晰度，需要澄清时返回 ClarificationNeededError。
func (b *Builder) WithClarification(clarifier *hooks.IntentClarifier) *Builder {
	// 将 IntentClarifier 的 Hook 方法转换为 BeforeHook
	beforeHook := clarifier.Hook()
	b.beforeHooks = append(b.beforeHooks, beforeHook)
	return b
}

// WithReflection 配置反思 Hook。
// 反思器会在 Agent 响应后进行自我评估和改进，提升输出质量。
func (b *Builder) WithReflection(
	chatModel model.ChatModel,
	config *agentreflection.ReflectionConfig,
	agentID string,
) *Builder {
	// 创建 Reflection Hook
	hook, err := reflecthooks.NewReflectionHookFromConfig(chatModel, config, agentID)
	if err != nil || !hook.IsEnabled() {
		return b
	}

	// 将 ReflectionHook 的 Refine 方法转换为 AfterHook
	afterHook := func(ctx context.Context, resp *Response) error {
		// 跳过流式响应或空响应
		if resp == nil || resp.Content == "" {
			return nil
		}

		// 使用当前 prompt 作为任务描述
		task := b.prompt
		if task == "" {
			task = "agent_response"
		}

		// 执行反思改进
		result := hook.Refine(ctx, task, resp.Content)

		// 如果有改进结果，更新响应内容
		if result != nil && result.FinalContent != "" {
			resp.Content = result.FinalContent

			// 将反思元数据添加到响应
			if resp.Metadata == nil {
				resp.Metadata = make(map[string]interface{})
			}
			resp.Metadata["reflection"] = map[string]interface{}{
				"iterations":    result.Iterations,
				"initial_score": result.InitialScore,
				"final_score":   result.FinalScore,
				"used_memory":   result.UsedMemory,
				"success":       result.Success,
				"duration_ms":   result.Duration.Milliseconds(),
			}
		}

		return nil
	}
	b.afterHooks = append(b.afterHooks, afterHook)
	return b
}

// WithAutoCompress 配置自动压缩 Hook。
// 自动压缩器会在每次响应后检查会话 token 使用量，超过阈值时自动压缩历史消息。
func (b *Builder) WithAutoCompress(compressHook *hooks.AutoCompressHook) *Builder {
	// 将 AutoCompressHook 的 Hook 方法转换为 AfterHook
	afterHook := func(ctx context.Context, resp *Response) error {
		// 确保 sessionID 在 context 中
		if b.sessionID != "" {
			ctx = hooks.ContextWithSessionID(ctx, b.sessionID)
		}
		// 调用 Hook
		hook := compressHook.Hook()
		return hook(ctx, resp)
	}
	b.afterHooks = append(b.afterHooks, afterHook)
	return b
}

// Build constructs the Agent with the configured options.
func (b *Builder) Build(ctx context.Context) (Agent, error) {
	// Apply default prompt if none set
	if b.prompt == "" {
		b.prompt = "You are a helpful AI assistant."
	}

	// Inject collaboration tools if configured
	if b.collabRegistry != nil && b.collabConfig != nil {
		if b.collabConfig.EnableDelegate {
			// 委派能力成对启用：单次委派（依赖链串行）+ 并行 fan-out（独立子任务，
			// 受并发上限护栏）。
			b.tools = append(b.tools, NewDelegateTool(b.collabRegistry))
			b.tools = append(b.tools, NewParallelDelegateTool(b.collabRegistry, 0))
		}
		if b.collabConfig.EnableAsk {
			b.tools = append(b.tools, NewAskTool(b.collabRegistry))
		}
		if b.collabConfig.EnableHandoff {
			b.tools = append(b.tools, NewHandoffTool(b.collabRegistry))
		}
	}

	// Apply auto-select tools
	if b.autoSelect && b.registry != nil {
		allTools := b.registry.GetTools()
		b.tools = append(b.tools, allTools...)
	}

	// Set default max iterations
	if b.maxIter <= 0 {
		b.maxIter = 10 // 默认最多10轮工具调用
	}

	// 如果没有显式设置 toolModel，尝试从 model 转换
	if b.toolModel == nil && len(b.tools) > 0 {
		if tc, ok := b.model.(model.ToolCallingChatModel); ok {
			b.toolModel = tc
		}
	}

	// Create the agent implementation
	a := &agentImpl{
		name:           b.name,
		model:          b.model,
		toolModel:      b.toolModel,
		prompt:         b.prompt,
		tools:          b.tools,
		beforeHooks:    b.beforeHooks,
		afterHooks:     b.afterHooks,
		middleware:     b.middleware,
		maxIter:        b.maxIter,
		tokenBudget:    b.tokenBudget,
		memoryService:  b.memoryService,
		contextBuilder: b.contextBuilder,
		enableMemory:   b.enableMemory,
	}

	return a, nil
}

// ========================================
// 快捷创建函数
// ========================================

// NewSimpleAgent 创建一个简单的 Agent（无工具）。
func NewSimpleAgent(chatModel model.ChatModel, name, prompt string) Agent {
	builder := New(chatModel).
		Name(name).
		Prompt(prompt)

	agent, _ := builder.Build(context.Background())
	return agent
}

// NewToolAgent 创建一个带工具的 Agent。
func NewToolAgent(toolModel model.ToolCallingChatModel, name, prompt string, toolsList ...tool.BaseTool) (Agent, error) {
	// 创建一个直接使用 toolModel 的 builder
	builder := &Builder{
		model:     nil,          // 不使用普通 ChatModel
		toolModel: toolModel,    // 直接使用 ToolCallingChatModel
		name:      name,
		prompt:    prompt,
		tools:     toolsList,
		maxIter:   10,
	}
	return builder.Build(context.Background())
}

// NewAgentFromRegistry 创建一个从注册中心加载工具的 Agent。
func NewAgentFromRegistry(toolModel model.ToolCallingChatModel, name, prompt string, registry ToolRegistry) (Agent, error) {
	// 创建一个直接使用 toolModel 的 builder
	builder := &Builder{
		model:     nil,
		toolModel: toolModel,
		name:      name,
		prompt:    prompt,
		registry:  registry,
		autoSelect: true,
		maxIter:   10,
	}
	return builder.Build(context.Background())
}

// ========================================
// 从配置创建 Agent
// ========================================

// llmAdapter 将 eino ChatModel 适配为 hooks.LLMClient
type llmAdapter struct {
	model model.ChatModel
}

func (a *llmAdapter) Chat(ctx context.Context, messages []hooks.Message) (string, error) {
	// 转换消息格式
	einoMessages := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			einoMessages[i] = schema.SystemMessage(msg.Content)
		case "assistant":
			einoMessages[i] = schema.AssistantMessage(msg.Content, nil)
		default: // user
			einoMessages[i] = schema.UserMessage(msg.Content)
		}
	}

	// 调用模型
	resp, err := a.model.Generate(ctx, einoMessages)
	if err != nil {
		return "", err
	}

	// 提取内容
	return resp.Content, nil
}

// toolModelToChatModelAdapter 将 ToolCallingChatModel 适配为 ChatModel
type toolModelToChatModelAdapter struct {
	model model.ToolCallingChatModel
}

func (a *toolModelToChatModelAdapter) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// ToolCallingChatModel 有 Generate 方法，直接调用
	return a.model.Generate(ctx, messages, opts...)
}

func (a *toolModelToChatModelAdapter) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// ToolCallingChatModel 有 Stream 方法，直接调用
	return a.model.Stream(ctx, messages, opts...)
}

func (a *toolModelToChatModelAdapter) BindTools(tools []*schema.ToolInfo) error {
	// ToolCallingChatModel 使用 WithTools 方法，返回新实例
	// 这里为了适配 ChatModel 接口，忽略返回值
	// 注意：这意味着工具绑定后需要使用返回的新实例
	_, err := a.model.WithTools(tools)
	return err
}

// toolModelAdapter 将 eino ToolCallingChatModel 适配为 hooks.LLMClient
// 用于需要工具调用能力的场景（如 Hook 中的 LLM 调用）
type toolModelAdapter struct {
	model model.ToolCallingChatModel
}

func (a *toolModelAdapter) Chat(ctx context.Context, messages []hooks.Message) (string, error) {
	// 转换消息格式
	einoMessages := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			einoMessages[i] = schema.SystemMessage(msg.Content)
		case "assistant":
			einoMessages[i] = schema.AssistantMessage(msg.Content, nil)
		default: // user
			einoMessages[i] = schema.UserMessage(msg.Content)
		}
	}

	// 调用模型（不绑定工具，用于纯对话）
	resp, err := a.model.Generate(ctx, einoMessages)
	if err != nil {
		return "", err
	}

	// 提取内容
	return resp.Content, nil
}

// NewAgentFromConfig 根据配置创建 Agent
func NewAgentFromConfig(
	chatModel model.ChatModel,
	config *agent.AgentConfig,
) (Agent, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	builder := New(chatModel)

	// 基本配置
	if config.MaxIterations > 0 {
		builder.WithMaxIterations(config.MaxIterations)
	}

	// Hook 配置
	if config.HookConfig != nil {
		llmAdapter := &llmAdapter{model: chatModel}

		// 结论生成 Hook
		if config.HookConfig.EnableConclusion {
			gen := hooks.NewConclusionGenerator(llmAdapter)
			if len(config.HookConfig.DataTools) > 0 {
				gen.AddDataTools(config.HookConfig.DataTools...)
			}
			if config.HookConfig.Timeout > 0 {
				gen.WithTimeout(time.Duration(config.HookConfig.Timeout) * time.Second)
			}
			gen.Enable()
			builder.WithConclusion(gen)
		}

		// 意图澄清 Hook
		if config.HookConfig.EnableClarification {
			clarifier := hooks.NewIntentClarifier(llmAdapter)
			if config.HookConfig.BusinessContext != "" {
				clarifier.WithBusinessContext(config.HookConfig.BusinessContext)
			}
			if config.HookConfig.MaxRounds > 0 {
				clarifier.WithMaxRounds(config.HookConfig.MaxRounds)
			}
			clarifier.Enable()
			builder.WithClarification(clarifier)
		}
	}

	// Reflection 配置
	if config.ReflectionConfig != nil && config.ReflectionConfig.Enabled {
		// Reflection 需要一个 embedder，这里暂时使用 nil
		// 实际使用时需要从外部注入 embedder
		builder.WithReflection(chatModel, config.ReflectionConfig, "")
	}

	return builder.Build(context.Background())
}

// NewAgentFromConfigWithTools 根据配置创建带工具的 Agent
func NewAgentFromConfigWithTools(
	toolModel model.ToolCallingChatModel,
	tools []tool.BaseTool,
	config *agent.AgentConfig,
) (Agent, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	builder := &Builder{
		model:     nil,
		toolModel: toolModel,
		tools:     tools,
		maxIter:   10,
	}

	if config.MaxIterations > 0 {
		builder.maxIter = config.MaxIterations
	}

	// 创建 toolModelAdapter 供 hooks 使用
	var adapter *toolModelAdapter
	if config.HookConfig != nil {
		adapter = &toolModelAdapter{model: toolModel}
	}

	// 创建 ChatModel 适配器供 Reflection 使用
	var chatModelAdapter model.ChatModel
	if config.ReflectionConfig != nil && config.ReflectionConfig.Enabled {
		chatModelAdapter = &toolModelToChatModelAdapter{model: toolModel}
	}

	// Hook 配置
	if config.HookConfig != nil {
		// 结论生成 Hook
		if config.HookConfig.EnableConclusion {
			gen := hooks.NewConclusionGenerator(adapter)
			if len(config.HookConfig.DataTools) > 0 {
				gen.AddDataTools(config.HookConfig.DataTools...)
			}
			if config.HookConfig.Timeout > 0 {
				gen.WithTimeout(time.Duration(config.HookConfig.Timeout) * time.Second)
			}
			gen.Enable()
			builder.WithConclusion(gen)
		}

		// 意图澄清 Hook
		if config.HookConfig.EnableClarification {
			clarifier := hooks.NewIntentClarifier(adapter)
			if config.HookConfig.BusinessContext != "" {
				clarifier.WithBusinessContext(config.HookConfig.BusinessContext)
			}
			if config.HookConfig.MaxRounds > 0 {
				clarifier.WithMaxRounds(config.HookConfig.MaxRounds)
			}
			clarifier.Enable()
			builder.WithClarification(clarifier)
		}
	}

	// Reflection 配置
	if config.ReflectionConfig != nil && config.ReflectionConfig.Enabled {
		// Reflection 需要一个 embedder，这里暂时使用 nil
		// 实际使用时需要从外部注入 embedder
		builder.WithReflection(chatModelAdapter, config.ReflectionConfig, "")
	}

	return builder.Build(context.Background())
}
