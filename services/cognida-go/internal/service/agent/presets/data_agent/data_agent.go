package dataagent

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	domainagent "cognida/internal/model/agent"
	"cognida/internal/model/conversation"
	"cognida/internal/service/agent/context/bpecount"
	"cognida/internal/service/agent/context/llmsummary"
	"cognida/internal/service/agent/convcontext"
	experiencesvc "cognida/internal/service/agent/experience"
	infraagent "cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/skills"
	toolregistry "cognida/internal/service/agent/tools"
)

// DataAgentID 是 Data Agent 预设的注册 ID。
const DataAgentID = "agent-data-agent"

// DataAgentCacheKey 是 data agent 在语义缓存策略中的逻辑键。
// 注意：data agent 与 RAG 助手的领域 AgentType 同为 agentic_rag，故用此逻辑键
// （而非 AgentType）隔离命名空间与开关，避免二者软/硬缓存互相串味。
const DataAgentCacheKey = "data_agent"

const (
	// defaultMaxIter 单一 ReAct 循环默认最大步数。长对话/深下钻放宽到 24 步：
	// 上下文规模不再由「达标即终止」的 token 预算约束，改由运行时就地折叠（不停机）治理。
	defaultMaxIter = 24
	// defaultTokenBudget 默认 token 预算。设为 0（不限）：react 循环不再因累计计费 token 达标而被动终止，
	// 上下文规模改由三级压缩（就地折叠，见下）治理，避免长对话被过早掐断。
	defaultTokenBudget = 0

	// defaultWallClock 默认挂钟预算：一次运行累计耗时达到该值即终止并 wind-down，交付基于现有观察的
	// 部分结论。收敛「步数/计费 token 都没到上限、却因推理模型某几步极慢或工具阻塞（长 SQL/子代理委派）
	// 让用户干等数分钟」的「感知到的卡住」。取 3 分钟：足够覆盖正常的多步取数+分析链，又不至于让用户长等。
	defaultWallClock = 3 * time.Minute

	// defaultToolTimeout 单次工具调用的统一挂钟兜底：wallClock 只在工具执行「之后」检查，救不了自身
	// 无内部超时且永久阻塞的工具（如 get_schema/graph_query 未自带超时）——那种情况下循环会卡死在那一步、
	// wallClock 永远轮不到。取 90s：宽于各工具自带超时（sql/MCP 30s、委派 30/60s），只在「没自带超时」的
	// 工具真卡住时兜底触发，正常慢查询/委派不会误伤。可经 DATA_AGENT_TOOL_TIMEOUT_SECONDS 覆盖（0=关闭）。
	defaultToolTimeout = 90 * time.Second

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

// toolTimeoutFromEnv 读取 DATA_AGENT_TOOL_TIMEOUT_SECONDS 覆盖默认单次工具超时；未设/非法用默认，
// 显式设为 0（或负）则关闭统一超时兜底（回退到「仅各工具自带超时」的行为）。
func toolTimeoutFromEnv() time.Duration {
	v := strings.TrimSpace(os.Getenv("DATA_AGENT_TOOL_TIMEOUT_SECONDS"))
	if v == "" {
		return defaultToolTimeout
	}
	secs, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("[Agent] DATA_AGENT_TOOL_TIMEOUT_SECONDS=%q 解析失败，回退默认 %s", v, defaultToolTimeout)
		return defaultToolTimeout
	}
	if secs <= 0 {
		return 0 // 显式关闭统一兜底
	}
	return time.Duration(secs) * time.Second
}

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
// recaller 非 nil 时启用「历史经验召回」首答注入（自进化读侧）；nil 时恒等透传。
// softCache 非 nil 且启用时，data agent 应答经软写穿装饰器回写语义缓存，供后续会话 few-shot 召回
// （写侧；读侧由 fewShot 的 Before hook 承担）。nil 时恒等透传，零回归。
func Spec(
	toolModel model.ToolCallingChatModel,
	msgRepo conversation.MessageRepository,
	registry *toolregistry.ToolRegistry,
	recaller *experiencesvc.GraphRecaller,
	fewShot SemanticRecaller,
	softCache infraagent.ResponseCache,
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
			return buildDataAgent(ctx, toolModel, msgRepo, registry, recaller, fewShot, softCache, gd)
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
	recaller *experiencesvc.GraphRecaller,
	fewShot SemanticRecaller,
	softCache infraagent.ResponseCache,
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

	// 真实 BPE token 计数器（内嵌 DeepSeek 词表）：三级压缩的触发线（①逐条 / ③折叠 / ③.5 剥离）
	// 与开场装配的预算治理统一用它。字符启发式会对「中文夹数字/百分号」的分析型回答（Data Agent 主流量）
	// 低估约 30% → 折叠触发偏晚；真实分词消除该偏差。加载失败时自动降级为启发式，绝不阻断 Agent 启动。
	tokenCounter, exact := bpecount.NewOrApprox()
	if !exact {
		log.Printf("[Agent] DataAgent：内嵌 BPE 分词器加载失败，token 计数降级为字符启发式（折叠触发线可能偏晚）")
	}

	builder := infraagent.New(nil).
		Name("DataAgent").
		Prompt(skills.AugmentPromptWithCatalog(systemPrompt)). // 追加技能目录（Level 1 渐进式披露）
		WithToolModel(toolModel).
		Tools(tools...).
		Before(toolPolicyHook()).                                // 硬工具门：以原始消息匹配 skill，须先于 playbook 注入
		Before(intentRoutingHook(toolModel)).                    // LLM 意图分类（词法兜底），注入对应 playbook
		Before(skills.InjectFromContextHook()).                  // 把入口暂存的命中 skill 指导自动注入（自动注入）
		Before(experienceRecallHook(recaller)).                  // 图谱经验召回（结构化问题→解法）
		Before(semanticFewShotHook(fewShot, DataAgentCacheKey)). // 末位：软语义缓存 few-shot（相似历史问答向量近邻，与图谱召回互补）
		WithMaxIterations(defaultMaxIter).
		WithTokenBudget(defaultTokenBudget).
		WithWallClock(defaultWallClock).                                  // 挂钟护栏：单次运行超 3min 即收尾交付部分结论，兜住「感知到的卡住」
		WithToolTimeout(toolTimeoutFromEnv()).                            // 单次工具超时兜底：统一封顶工具级 hang，防其拖死循环使 wallClock 失效
		WithContextCompaction(compactTriggerTokens, compactTargetTokens). // ③整段对话：累计≥128k就地折叠回~64k
		WithMaxMessageTokens(maxMessageTokens).                           // ①单条消息：>32k先压该条（保 result_id）
		WithReasoningEviction(reasoningEvictTokens).                      // ③.5陈旧思考：旧轮reasoning累计≥16k整轮折走，只留本轮thinking
		WithTokenCounter(tokenCounter).                                   // 三级压缩统一用真实 BPE 计数（消除对分析型文本的低估）
		WithCollaboration(collabRegistry, infraagent.EnableDelegate())
	if msgRepo != nil {
		// 跨轮对话记忆：读 messages 表回放历史（与 UI 同源，只读不写），启用 framework 记忆分支。
		// 开场装配的超窗早期轮摘要注入 LLM 语义摘要器（llmsummary，自带抽取式降级），与运行时 ③
		// 折叠共用同一套语义压缩口径；toolModel 不可用时 llmsummary 内部退化为确定性抽取式。
		builder = builder.WithContextBuilder(
			convcontext.NewConversationContextBuilder(msgRepo).
				WithSummarizer(llmsummary.New(toolModel)).
				WithCounter(tokenCounter), // 与运行时 ③ 折叠共用同一 BPE 计数口径，开场/运行时估算一致
		)
	}
	// 组合根护栏装配（默认关闭 → 恒等，零回归）：输入把关 + 逐工具/最终输出脱敏，
	// 与硬工具门/写审批叠加，构成 Data Agent 的多层安全缝。
	builder = guardrail.Apply(builder)

	reactAgent, err := builder.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("构建 Data Agent 失败: %w", err)
	}
	// 软写穿装饰（写侧）：softCache 非 nil 且启用时，每次应答回写语义缓存供后续 few-shot 召回；
	// 从不短路（数据会变，读侧由 semanticFewShotHook 注入参考、LLM 结合实时数据重算）。
	// nil 或未启用时 NewWriteThroughAgent 恒等透传，零回归。
	return infraagent.NewWriteThroughAgent(reactAgent, softCache, DataAgentCacheKey), nil
}
