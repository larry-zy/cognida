// Package agent provides Agent implementation using Eino framework.
//
// NOTE: For domain-level agent concepts, see link/internal/model/agent.
package framework

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	obs "cognida/internal/infrastructure/observability"
	domainagent "cognida/internal/model/agent"
	ctxeng "cognida/internal/service/agent/context"
)

// frameworkTracer 供工具级 span 使用。otel 全局 tracer 走延迟委派，包初始化时创建、
// SetTracerProvider 之后依旧生效；未注册真实 provider 时为 no-op，零开销。
var frameworkTracer = obs.NewTracer()

// agentImpl is the default implementation of Agent.
type agentImpl struct {
	name        string
	model       model.BaseChatModel        // 使用 BaseChatModel（ChatModel 已废弃）
	toolModel   model.ToolCallingChatModel // 用于工具调用
	prompt      string
	tools       []tool.BaseTool
	beforeHooks []BeforeHook
	afterHooks  []AfterHook
	// observerHooks 是回答生成完成后触发的只读观察者：与 afterHooks 不同，它不得改写 resp
	// （流式路径正文可能已逐块下发无法回收），因此对所有 sink（含纯流式）都会触发，best-effort
	// （返回 error 仅记录不阻断）。用于异步自我进化等旁路消费最终回答。
	observerHooks []AfterHook
	middleware    []Middleware
	maxIter       int // 最大迭代次数（用于工具调用循环）
	tokenBudget   int // token 预算（0 表示不限），与 maxIter 共同约束 ReAct 循环

	// wallClock ReAct 循环的挂钟预算（0 表示不限）：一次运行累计耗时达到该值即终止并 wind-down。
	// 与 maxIter/tokenBudget 互补——推理模型某几步可能极慢（长思考/工具阻塞），步数/计费 token 都未到
	// 上限却已让用户干等数分钟；挂钟护栏为「感知到的卡住」兜底，到点即交付基于现有观察的部分结论。
	wallClock time.Duration

	// toolTimeout 单次工具调用的挂钟上限（0 表示不限）：wallClock 的两处检查都在工具执行「之后」，
	// 若某工具自身无内部超时且永久阻塞，循环会卡死在那一步、wallClock 永远轮不到——本字段为所有
	// 工具统一兜底。到点即以超时观察回灌 LLM（非致命，供其换路径），把工具级 hang 收敛为一次可恢复的失败。
	// 各工具自带的更短超时（sql 30s、MCP 30s、委派 30/60s）会先于本兜底触发，本兜底只兜「没自带超时」的缺口。
	toolTimeout time.Duration

	// 运行时上下文压缩（三级压缩之①③，见 normalizeContext）。都为 0 时不启用，零回归。
	compactTrigger   int // ③整段对话：累计上下文 token ≥ 该值 → 就地折叠
	compactTarget    int // ③折叠目标水位
	maxMessageTokens int // ①单条消息：单条 token 超过该值 → 先压该条（保 result_id）

	// ③.5 陈旧思考剥离：最近一轮之前累计的 reasoning token ≥ 该值 → 把旧轮整轮折走（思考随之离场）。
	// 0 时不启用。只让最近一轮带 thinking，更早的思考不在上下文里累积（见 normalizeContext）。
	reasoningEvictTokens int

	// summarizer ③ 折叠兜底（Level B）用的历史摘要器；nil 时用确定性抽取式摘要（ExtractiveSummarizer）。
	// 注入 LLM 语义摘要器（llmsummary）可显著提升折叠保真度，且其自带抽取式降级，绝不破坏主循环。
	summarizer ctxeng.Summarizer

	// counter 三级压缩（①逐条 / ③折叠触发线 / ③.5 剥离判定）统一使用的 token 计数器。
	// nil 时默认字符启发式 ApproxTokenCounter；业务侧可经 WithTokenCounter 注入真实 BPE 分词器
	// （bpecount），消除对「中文夹数字/百分号」分析型文本 ~30% 的低估、让折叠触发线不再偏晚。
	counter ctxeng.TokenCounter

	// Context Builder 支持（记忆读入口）
	contextBuilder ContextBuilder // 上下文构建器接口
	enableMemory   bool           // 是否启用记忆功能

	// outputGuardrailActive 标记启用了输出护栏：流式会话需强制走缓冲交付，
	// 让 AfterHook（含输出护栏）在最终内容下发前完成脱敏/替换。
	outputGuardrailActive bool

	// toolOutputHook 逐工具 post-invoke 输出护栏缝（任务 5）：工具返回后、回灌前脱敏；
	// nil 时零开销、观察逐字节不变。
	toolOutputHook ToolOutputHook

	// collabRegistry 子代理协作注册表（可为 nil）：非空时于 run 入口只读注入执行 ctx，
	// 使工具/skill handler 能经 InlineDelegate inline 编排子代理（design D7/D8）。
	collabRegistry *CollaborationRegistry
}

// New creates a new Builder for constructing an Agent.
// Note: Accepts BaseChatModel (ChatModel is deprecated)
func New(chatModel model.BaseChatModel) *Builder {
	return &Builder{
		model: chatModel,
	}
}

// preProcess 统一前置流程：beforeHooks + middleware.Before。
// 记忆分支与非记忆分支都必须经过（旧实现记忆分支提前 return，绕过全部 hooks/middleware）。
func (a *agentImpl) preProcess(ctx context.Context, message string) (context.Context, string, error) {
	// 入口即固化用户原始问题：此刻 message 尚未被任何 hook（playbook/skill 注入等）改写，
	// 后续 Hook/Observer 可经 UserQueryFromContext 取到干净的真实问题。
	ctx = WithUserQuery(ctx, message)
	for _, hook := range a.beforeHooks {
		var err error
		ctx, message, err = hook(ctx, message)
		if err != nil {
			return ctx, message, err
		}
	}
	for _, mw := range a.middleware {
		var err error
		ctx, message, err = mw.Before(ctx, message)
		if err != nil {
			return ctx, message, err
		}
	}
	return ctx, message, nil
}

// ========================================
// 统一执行主干（Phase 3）
//
// Chat 与 Stream 共用唯一执行主干 run(ctx, req, sink)，由三组可插拔组件组合而成：
//   - MemoryStrategy：buildInitialMessages（读入口，含协作构建）+ persistResult（写出口，落库/摘要）
//   - ToolLoop：execLoop（ReAct 循环，token 预算 + wind-down）+ handleToolCall（单次工具调用）
//   - StreamSink：execSink 抽象输出侧差异——bufferedSink 累积为 *Response；streamSink 即时下发 *Chunk
//
// 由此收敛旧的 6 个 chatWithMemoryAndTools/chatWithMemoryOnly/chatWithTools/chatWithoutTools/
// streamWithTools/streamWithoutTools 变体为单一路径；memory×tool×stream 各组合行为一致（愈合）。
// ========================================

// runRequest 是一次执行的输入：message 为 hook 处理后的消息（仅进本轮 prompt），
// rawMessage 为用户原始输入（持久化到跨轮记忆），sessionID 为会话标识。
type runRequest struct {
	message    string
	rawMessage string
	sessionID  string
}

// Chat implements Agent.Chat with tool calling support.
func (a *agentImpl) Chat(ctx context.Context, message string) (*Response, error) {
	sessionID, _ := domainagent.GetSessionID(ctx)

	// 统一前置流程：先于分支判断执行，记忆分支不再绕过。
	// rawMessage 保留 hook 处理前的原始输入：hook 注入的 playbook 等只进本轮 prompt，
	// 跨轮记忆持久化的是用户原话。
	rawMessage := message
	ctx, message, err := a.preProcess(ctx, message)
	if err != nil {
		return nil, err
	}

	return a.run(ctx, runRequest{message: message, rawMessage: rawMessage, sessionID: sessionID}, &bufferedSink{a: a})
}

// Stream implements Agent.Stream with full tool calling support.
func (a *agentImpl) Stream(ctx context.Context, message string) (<-chan *Chunk, error) {
	// 提前检查模型是否可用
	if a.model == nil && a.toolModel == nil {
		return nil, fmt.Errorf("agent: no chat model available for streaming")
	}

	ch := make(chan *Chunk, 16)

	sessionID, _ := domainagent.GetSessionID(ctx)

	// 统一前置流程（与 Chat 共用）：beforeHooks + middleware.Before。
	// rawMessage 保留 hook 处理前的原始输入，跨轮记忆持久化用户原话。
	rawMessage := message
	ctx, message, err := a.preProcess(ctx, message)
	if err != nil {
		close(ch)
		return nil, err
	}

	go a.runStream(ctx, runRequest{message: message, rawMessage: rawMessage, sessionID: sessionID}, ch)

	return ch, nil
}

// runStream 在独立 goroutine 中驱动流式主干，负责 close(ch)。
// 启用输出护栏时切到 bufferedStreamSink：正文先缓冲跑完 + 过 AfterHook（含输出护栏脱敏/替换），
// 再把最终安全内容一次性下发——已逐块流出的 token 无法事后回收，故必须强制缓冲交付。
func (a *agentImpl) runStream(ctx context.Context, req runRequest, ch chan *Chunk) {
	defer close(ch)
	if a.outputGuardrailActive {
		_, _ = a.run(ctx, req, &bufferedStreamSink{a: a, ch: ch})
		return
	}
	_, _ = a.run(ctx, req, &streamSink{a: a, ch: ch})
}

// run 是唯一执行主干：pre-processing 已在入口完成。
// MemoryStrategy(buildInitialMessages/persistResult) + ToolLoop(execLoop) + StreamSink(sink) 三组件组合。
func (a *agentImpl) run(ctx context.Context, req runRequest, sink execSink) (*Response, error) {
	if !sink.start(ctx) {
		return nil, nil // 下游已断开（streaming）
	}

	if a.model == nil && a.toolModel == nil {
		err := fmt.Errorf("agent: no chat model available")
		sink.fail(ctx, err)
		return nil, err
	}

	// 委派痕迹侧信道：安装一个空累积器，委派边界（executeDelegation）在子代理返回后
	// 把子代理内部真实工具轨迹与用量追加进来（否则被上下文防火墙吞掉）。子委派的
	// 子代理 run 会各自安装自己的痕迹，逐层向上冒泡（executeDelegation 读取子代理已并入的
	// Response 再追加到本层痕迹），使 data_agent 等指挥官型 agent 的顶层 Response 反映
	// 全链路工具/用量，供 Agent 评测消费。
	trace := &delegationTrace{}
	ctx = withDelegationTrace(ctx, trace)

	// 协作注册表只读注入：使本 run 内的工具/skill handler 能经 InlineDelegate inline
	// 编排子代理（design D7/D8）。nil 时 WithCollaborationRegistry 原样返回，取用方走降级。
	ctx = WithCollaborationRegistry(ctx, a.collabRegistry)

	// MemoryStrategy：构建初始消息 + 落库本轮用户消息。
	messages, rc := a.buildInitialMessages(ctx, req)

	response := &Response{
		ToolCalls: make([]*ToolCall, 0),
		Metadata:  make(map[string]interface{}),
	}
	hasTools := len(a.tools) > 0 && a.toolModel != nil

	// ToolLoop：统一 ReAct（token 预算 + wind-down）/ 单轮生成。
	res, err := a.execLoop(ctx, messages, hasTools, sink, response)
	if err != nil {
		sink.fail(ctx, err)
		return nil, err
	}
	if res.aborted {
		return nil, nil // 流式下游断开，静默终止
	}

	response.Content = res.content

	// 并入委派痕迹：把子代理内部工具轨迹接到顶层轨迹尾部（顺序为 delegate_to_agent 后紧跟
	// 其真实子工具），运行时用量累加进本轮，随后由 fillMetadata 统一写入 Metadata。
	if tcs, tok, iters := trace.drain(); len(tcs) > 0 || tok > 0 || iters > 0 {
		response.ToolCalls = append(response.ToolCalls, tcs...)
		res.tokensUsed += tok
		res.iterations += iters
	}

	// MemoryStrategy：落库助手响应 + 更新协作摘要（仅记忆激活时）。
	a.persistResult(ctx, rc, messages, res.content)

	a.fillMetadata(response.Metadata, rc, hasTools, res)

	// 只读响应观察者：最终内容此刻已组装完毕（response.Content 对所有 sink 均已就绪），
	// 在下发/收尾前触发。与 afterHooks 不同，observer 对纯流式 sink 同样生效——这是
	// 异步自我进化能捕获流式回答的关键（afterHooks 在纯流式路径被刻意跳过）。best-effort。
	if len(a.observerHooks) > 0 {
		for _, hook := range a.observerHooks {
			if err := hook(ctx, response); err != nil {
				log.Printf("[agent] observer hook 出错（忽略）: %v", err)
			}
		}
	}

	if err := sink.finish(ctx, response); err != nil {
		return response, err
	}
	return response, nil
}

// sendChunk 遵守 ctx 取消的下游发送：客户端断开（ctx.Done）后不再发送，
// 返回 false 通知调用方终止上游生成，避免 goroutine 阻塞在 ch<- 上泄漏。
func sendChunk(ctx context.Context, ch chan<- *Chunk, c *Chunk) bool {
	select {
	case ch <- c:
		return true
	case <-ctx.Done():
		return false
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
//
// 硬不变量：盲切 runes[:上限] 可能把正文尾部的 result_id 一起切掉，而下游「按引用取回完整结果集」
// 依赖它——故先从完整输出收集 result_id，截断后再经 EnsureResultIDsPresent 兜回缺失的引用，
// 与 ① 路径 CapContentByTokens、③ 折叠 MaskObservation 的保真口径保持一致。
func compactObservation(output string) string {
	runes := []rune(output)
	if len(runes) <= maxObservationChars {
		return output
	}
	truncated := string(runes[:maxObservationChars]) +
		fmt.Sprintf("\n...[观察已截断，省略 %d 字符；如需完整数据请按 result_id 取用]", len(runes)-maxObservationChars)
	return ctxeng.EnsureResultIDsPresent(truncated, ctxeng.CollectResultIDs([]ctxeng.Turn{{Content: output}}))
}
