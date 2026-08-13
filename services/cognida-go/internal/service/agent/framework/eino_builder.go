package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"cognida/internal/model/agent"
	agentreflection "cognida/internal/model/agent/reflection"
	domainguardrail "cognida/internal/model/guardrail"
	ctxeng "cognida/internal/service/agent/context"
	"cognida/internal/service/agent/context/llmsummary"
	"cognida/internal/service/agent/framework/hooks"
	reflecthooks "cognida/internal/service/agent/framework/reflection"
)

// Builder provides a fluent API for constructing Agent instances.
type Builder struct {
	model         model.BaseChatModel        // 使用 BaseChatModel（ChatModel 已废弃）
	toolModel     model.ToolCallingChatModel // 用于工具调用
	name          string
	description   string
	prompt        string
	tools         []tool.BaseTool
	beforeHooks   []BeforeHook
	afterHooks    []AfterHook
	observerHooks []AfterHook // 只读响应观察者（对所有 sink 生效，含纯流式），供异步旁路消费最终回答
	middleware    []Middleware
	registry      ToolRegistry
	ragService    RAGService
	sessionID     string
	autoSelect    bool
	maxIter       int           // 最大迭代次数
	tokenBudget   int           // token 预算（0 表示不限），与 maxIter 共同约束 ReAct 循环
	wallClock     time.Duration // 挂钟预算（0 表示不限），与 maxIter/tokenBudget 共同约束 ReAct 循环
	toolTimeout   time.Duration // 单次工具调用挂钟上限（0 表示不限），统一兜底工具级 hang

	// 运行时上下文压缩（三级压缩之②③）。都为 0 时不启用，逐字节零回归。
	compactTrigger   int // ③整段对话：运行累计上下文 token ≥ 该值 → 就地折叠（不停机）
	compactTarget    int // ③折叠目标水位：折叠后压回该 token 附近
	maxMessageTokens int // ①单条消息：单条 token 超过该值 → 先压该条（保 result_id）

	reasoningEvictTokens int // ③.5 陈旧思考剥离：最近一轮之前累计 reasoning token ≥ 该值 → 旧轮整轮折走

	summarizer ctxeng.Summarizer // ③ 折叠兜底摘要器；nil 且启用压缩时自动用 LLM 语义摘要（llmsummary），否则抽取式

	counter ctxeng.TokenCounter // 三级压缩统一 token 计数器；nil 时默认字符启发式，可注入真实 BPE（bpecount）

	collabRegistry *CollaborationRegistry
	collabConfig   *CollaborationConfig

	// Context Builder support (Phase 6)：记忆读入口，写出口在 handler 层 AgentPersistenceService
	contextBuilder ContextBuilder // 上下文构建器接口
	enableMemory   bool           // 是否启用记忆功能

	// 安全护栏支持（agent-guardrail-runtime）。
	// 输入护栏 Hook 独立存放，Build() 时置于 beforeHooks 最前（最先把关）；
	// 输出护栏 Hook 独立存放，Build() 时置于 afterHooks 最后（最后把关）。
	inputGuardrailHook      BeforeHook
	outputGuardrailHook     AfterHook
	toolOutputGuardrailHook ToolOutputHook // 逐工具 post-invoke 脱敏缝（任务 5）
	outputGuardrailActive   bool           // 启用输出护栏 → 流式会话强制走缓冲交付
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

// Observe adds a read-only response observer that fires for every sink (including
// pure streaming). Observers must not mutate the response (streamed content can't be
// recalled); they run best-effort. Used by async self-evolution to capture final answers.
func (b *Builder) Observe(hook AfterHook) *Builder {
	b.observerHooks = append(b.observerHooks, hook)
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

// WithWallClock 设置 ReAct 循环的挂钟预算（一次运行累计耗时达到该值即终止并 wind-down 收尾）。
// 与 maxIter/tokenBudget 互补：为「步数/计费 token 都没到上限、却因某几步极慢而让用户长时间干等」
// 的卡顿兜底，到点即交付基于现有观察的部分结论。传入 <=0 表示不限挂钟。
func (b *Builder) WithWallClock(d time.Duration) *Builder {
	b.wallClock = d
	return b
}

// WithToolTimeout 设置单次工具调用的挂钟上限：wallClock 的检查都在工具执行「之后」，若某工具自身
// 无内部超时且永久阻塞，循环会卡死在那一步、wallClock 永远轮不到——本兜底为所有工具统一封顶，到点即
// 以可恢复的超时观察回灌 LLM（换路径或收尾），把工具级 hang 收敛掉。各工具自带的更短超时会先触发；
// 本兜底只兜「没自带超时」的缺口（如 get_schema/graph_query）。传入 <=0 表示不套统一超时。
func (b *Builder) WithToolTimeout(d time.Duration) *Builder {
	b.toolTimeout = d
	return b
}

// WithContextCompaction 配置「整段对话」的运行时压缩（三级压缩之③）：
// ReAct 循环内累计上下文 token ≥ trigger 时就地折叠（不停机），把最早的整轮对话
// 抽取式摘要折进一条系统消息，逐字保留最近若干整轮，压回 target 附近。
// 按「整轮」折叠以保住 assistant.tool_calls ↔ tool 消息的 1:1 配对不变量。
// trigger<=0 或 target<=0 时不启用（零回归）。
func (b *Builder) WithContextCompaction(trigger, target int) *Builder {
	b.compactTrigger = trigger
	b.compactTarget = target
	return b
}

// WithContextSummarizer 注入 ③ 折叠兜底（Level B）用的历史摘要器（能力层 ctxeng.Summarizer）。
// 不注入时：启用上下文压缩且存在生成模型 → Build 自动装配 LLM 语义摘要器（llmsummary，自带抽取式降级）；
// 否则用确定性抽取式摘要。显式注入优先于自动装配（主要供测试或自定义摘要策略）。
func (b *Builder) WithContextSummarizer(s ctxeng.Summarizer) *Builder {
	b.summarizer = s
	return b
}

// WithReasoningEviction 配置「陈旧思考剥离」（三级压缩之③.5）：DeepSeek V4 契约要求带 tool_calls 的
// assistant 轮把 reasoning_content 原样回传后续所有轮（否则 400），思考会在单次 ReAct 运行内堆积。
// 当「最近一轮之前」累计的 reasoning token ≥ n 时，把这些旧轮整轮折走（思考随整轮离场），只让最近一轮
// 带 thinking。不能单删旧轮的 reasoning 字段（会破坏契约 → 400），故靠整轮折叠实现。n<=0 时不启用。
func (b *Builder) WithReasoningEviction(n int) *Builder {
	b.reasoningEvictTokens = n
	return b
}

// WithMaxMessageTokens 配置「单条消息」上限（三级压缩之①）：任一条消息 token 超过 n
// 时先就地压缩该条（复用 ctxeng.CapContentByTokens，保住其中的 result_id）。n<=0 时不启用。
func (b *Builder) WithMaxMessageTokens(n int) *Builder {
	b.maxMessageTokens = n
	return b
}

// WithTokenCounter 注入三级压缩统一使用的 token 计数器（能力层 ctxeng.TokenCounter）。
// 不注入时默认字符启发式 ApproxTokenCounter；注入真实 BPE 分词器（bpecount）可消除对
// 「中文夹数字/百分号」分析型文本约 30% 的低估，使 ①逐条压缩 / ③折叠 / ③.5 剥离的触发线更准。
// nil 时忽略（保持默认启发式）。
func (b *Builder) WithTokenCounter(c ctxeng.TokenCounter) *Builder {
	if c != nil {
		b.counter = c
	}
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

// WithReflectionFromToolModel 用 Builder 已持有的 ToolCallingChatModel 作为反思的 Actor/Critic 模型，
// 配置反思 Hook。供只持有 toolModel、拿不到 model.BaseChatModel 的调用方（如 data_agent 预设）使用：
// toolModel（ToolCallingChatModel）天然满足 BaseChatModel，直接传入，无需适配器。
// memory 可为 nil（仅评估不沉淀经验）；非 nil 时反思会检索/存储历史教训（自我进化闭环）。
func (b *Builder) WithReflectionFromToolModel(
	config *agentreflection.ReflectionConfig,
	agentID string,
	memory agentreflection.ReflectionMemory,
) *Builder {
	if b.toolModel == nil {
		return b
	}
	return b.WithReflection(b.toolModel, config, agentID, memory)
}

// WithReflection 配置反思 Hook。
// 反思器会在 Agent 响应后进行自我评估和改进，提升输出质量。
// memory 可为 nil（仅评估、不检索/沉淀历史经验）；非 nil 时启用自我进化闭环：
// 第一次迭代检索历史教训注入 refine prompt，评估结束异步存储成功/失败经验。
func (b *Builder) WithReflection(
	chatModel model.BaseChatModel,
	config *agentreflection.ReflectionConfig,
	agentID string,
	memory agentreflection.ReflectionMemory,
) *Builder {
	// 创建 Reflection Hook
	hook, err := reflecthooks.NewReflectionHookFromConfig(chatModel, config, agentID)
	if err != nil || !hook.IsEnabled() {
		return b
	}

	// 注入记忆（可选）：NewReflectionHookFromConfig 内部不建 memory（依赖外部 embedder/Milvus），
	// 由调用方在拿到 embedder 的组合层构造后经此注入。
	if memory != nil {
		hook.SetMemory(memory)
	}

	// 将 ReflectionHook 的 Refine 方法转换为 AfterHook
	afterHook := func(ctx context.Context, resp *Response) error {
		// 跳过流式响应或空响应
		if resp == nil || resp.Content == "" {
			return nil
		}

		// 任务描述取本轮「用户原始问题」（preProcess 入口经 WithUserQuery 写入 ctx）：
		// critic 据此评估回答是否切题、memory 据此按真实意图检索历史教训。
		// 以往误用 b.prompt（系统提示词/agent 指令），会让评估与检索脱靶——见 WithUserQuery 注释。
		// 兜底：ctx 无用户问题时退回系统提示词，再退回占位串。
		task := UserQueryFromContext(ctx)
		if task == "" {
			task = b.prompt
		}
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

// WithInputGuardrail 装配输入护栏。生成的 BeforeHook 在 Build() 时置于 before 链最前，
// 保证「最先把关」：越狱检测（不可脱敏，命中即中止）→ 通用输入检查（不安全时按 cfg.Sanitize
// 中止或脱敏放行）。gs 为 nil 或所有开关皆关时为空操作（不改变既有行为）。
func (b *Builder) WithInputGuardrail(gs domainguardrail.GuardrailService, cfg InputGuardrailConfig) *Builder {
	if gs == nil || (!cfg.EnableJailbreak && !cfg.EnableInputCheck) {
		return b
	}
	b.inputGuardrailHook = newInputGuardrailHook(gs, cfg)
	return b
}

// WithOutputGuardrail 装配输出护栏。生成的 AfterHook 在 Build() 时置于 after 链最后，
// 保证「最后把关」：对最终回答做 CheckOutput（含幻觉/PII），命中脱敏或替换安全兜底。
// 启用输出护栏会置位 outputGuardrailActive，使流式会话强制走缓冲交付（见 eino_agent.go）。
// gs 为 nil 时为空操作。
func (b *Builder) WithOutputGuardrail(gs domainguardrail.GuardrailService, cfg OutputGuardrailConfig) *Builder {
	if gs == nil {
		return b
	}
	b.outputGuardrailHook = newOutputGuardrailHook(gs, cfg)
	b.outputGuardrailActive = true
	return b
}

// WithToolOutputGuardrail 装配逐工具输出护栏（post-invoke 第二阶段，任务 5）：工具返回后、
// 回灌 LLM 前对纯文本观察脱敏；result_id 引用信封原样放行以保结构。与最终输出护栏各自独立
// 开关（任务 7.1），可单独启用。gs 为 nil 时为空操作（零回归）。
func (b *Builder) WithToolOutputGuardrail(gs domainguardrail.GuardrailService, cfg OutputGuardrailConfig) *Builder {
	if gs == nil {
		return b
	}
	b.toolOutputGuardrailHook = newToolOutputGuardrailHook(gs, cfg)
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

	// ③ 折叠兜底摘要器自动装配：未显式注入且启用了上下文压缩时，用生成模型（优先 model，回退 toolModel）
	// 装配 LLM 语义摘要器；无模型可用则留 nil（summarizeOlder 回退确定性抽取式摘要）。llmsummary 自带
	// 抽取式降级，故任何 LLM 慢/失败都不破坏折叠与主循环。
	if b.summarizer == nil && b.compactTrigger > 0 {
		if gen := firstGenModel(b.model, b.toolModel); gen != nil {
			b.summarizer = llmsummary.New(gen)
		}
	}

	// 护栏 Hook 排序：输入护栏最先把关（before 链最前），输出护栏最后把关（after 链最后）。
	// 未装配护栏时两条链保持原样，行为逐字节不变（零回归）。
	beforeHooks := b.beforeHooks
	if b.inputGuardrailHook != nil {
		beforeHooks = append([]BeforeHook{b.inputGuardrailHook}, beforeHooks...)
	}
	afterHooks := b.afterHooks
	if b.outputGuardrailHook != nil {
		afterHooks = append(append([]AfterHook{}, afterHooks...), b.outputGuardrailHook)
	}

	// Create the agent implementation
	a := &agentImpl{
		name:                  b.name,
		model:                 b.model,
		toolModel:             b.toolModel,
		prompt:                b.prompt,
		tools:                 b.tools,
		beforeHooks:           beforeHooks,
		afterHooks:            afterHooks,
		observerHooks:         b.observerHooks,
		middleware:            b.middleware,
		maxIter:               b.maxIter,
		tokenBudget:           b.tokenBudget,
		wallClock:             b.wallClock,
		toolTimeout:           b.toolTimeout,
		compactTrigger:        b.compactTrigger,
		compactTarget:         b.compactTarget,
		maxMessageTokens:      b.maxMessageTokens,
		reasoningEvictTokens:  b.reasoningEvictTokens,
		summarizer:            b.summarizer,
		counter:               b.counter,
		contextBuilder:        b.contextBuilder,
		enableMemory:          b.enableMemory,
		outputGuardrailActive: b.outputGuardrailActive,
		toolOutputHook:        b.toolOutputGuardrailHook,
		collabRegistry:        b.collabRegistry,
	}

	return a, nil
}

// firstGenModel 返回首个非 nil 生成模型作为 llmsummary.LLM（BaseChatModel/ToolCallingChatModel
// 均实现 Generate，天然满足 LLM 接口）。优先 model、回退 toolModel；都为 nil 时返回 nil。
// 逐一判空后再返回，避免把「持有 nil 具体值的非 nil 接口」误当可用模型（typed-nil 陷阱）。
func firstGenModel(m model.BaseChatModel, tm model.ToolCallingChatModel) llmsummary.LLM {
	if m != nil {
		return m
	}
	if tm != nil {
		return tm
	}
	return nil
}

// ========================================
// 快捷创建函数
// ========================================

// NewSimpleAgent 创建一个简单的 Agent（无工具）。
func NewSimpleAgent(chatModel model.BaseChatModel, name, prompt string) Agent {
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
		model:     nil,       // 不使用普通 ChatModel
		toolModel: toolModel, // 直接使用 ToolCallingChatModel
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
		model:      nil,
		toolModel:  toolModel,
		name:       name,
		prompt:     prompt,
		registry:   registry,
		autoSelect: true,
		maxIter:    10,
	}
	return builder.Build(context.Background())
}

// ========================================
// 从配置创建 Agent
// ========================================

// llmAdapter 将 eino ChatModel 适配为 hooks.LLMClient
type llmAdapter struct {
	model model.BaseChatModel
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
	chatModel model.BaseChatModel,
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
		builder.WithReflection(chatModel, config.ReflectionConfig, "", nil)
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

	// toolModel（ToolCallingChatModel）天然满足 BaseChatModel，直接供 Reflection 使用
	var reflectionModel model.BaseChatModel
	if config.ReflectionConfig != nil && config.ReflectionConfig.Enabled {
		reflectionModel = toolModel
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
		builder.WithReflection(reflectionModel, config.ReflectionConfig, "", nil)
	}

	return builder.Build(context.Background())
}
