// Package agent provides Agent implementation using Eino framework.
//
// NOTE: For domain-level agent concepts, see link/internal/model/agent.
package framework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	obs "link/internal/infrastructure/observability"
	domainagent "link/internal/model/agent"
	"link/internal/model/memory"
)

// frameworkTracer 供工具级 span 使用。otel 全局 tracer 走延迟委派，包初始化时创建、
// SetTracerProvider 之后依旧生效；未注册真实 provider 时为 no-op，零开销。
var frameworkTracer = obs.NewTracer()

// ========================================
// MemoryService and ContextBuilder 接口
// ========================================

// MemoryService 定义 Agent 需要的记忆服务接口
// 这是一个简化接口，避免循环依赖
type MemoryService interface {
	SaveMessage(ctx context.Context, msg *memory.Message) error
	LoadHistoryWithLimit(ctx context.Context, sessionID string, limit int) ([]*memory.Message, error)
	UpdateSummary(ctx context.Context, sessionID string, summary string) error
	GetSummary(ctx context.Context, sessionID string) (string, error)
}

// ContextBuilder 定义 Agent 需要的上下文构建器接口
type ContextBuilder interface {
	Build(ctx context.Context, req *memory.BuildContextRequest) (*memory.BuildContext, error)
	BuildForCollaboration(ctx context.Context, req *memory.BuildContextRequest, mode string, contextLimit int) (*memory.BuildContext, error)
}

// Agent is the core interface for AI agents.
// An Agent can chat with users, stream responses, and use tools.
type Agent interface {
	// Chat processes a message and returns a complete response.
	// It includes tool calls in the response if any tools were invoked.
	Chat(ctx context.Context, message string) (*Response, error)

	// Stream processes a message and streams chunks of the response.
	// The channel closes when streaming is complete.
	Stream(ctx context.Context, message string) (<-chan *Chunk, error)

	// Name returns the agent's name.
	Name() string
}

// Response represents a complete agent response.
type Response struct {
	// Content is the text content of the response.
	Content string

	// ToolCalls contains information about tools called during processing.
	ToolCalls []*ToolCall

	// Metadata contains additional information about the response.
	Metadata map[string]interface{}
}

// ToolCall represents a single tool invocation.
type ToolCall struct {
	// Name is the name of the tool that was called.
	Name string

	// Input is the parameters passed to the tool.
	Input map[string]interface{}

	// Output is the result returned by the tool.
	Output string

	// Error contains any error that occurred during tool execution.
	Error error
}

// Chunk represents a streaming response chunk.
type Chunk struct {
	// Content is a partial piece of the response content.
	Content string

	// Done indicates whether this is the final chunk.
	Done bool

	// Metadata contains additional information about this chunk.
	Metadata map[string]interface{}
}

// ChunkEvent 表示流式响应的事件类型
type ChunkEvent string

const (
	// EventContent 内容块事件
	EventContent ChunkEvent = "content"
	// EventToolCall 工具调用事件
	EventToolCall ChunkEvent = "tool_call"
	// EventToolResult 工具执行结果事件
	EventToolResult ChunkEvent = "tool_result"
	// EventError 错误事件
	EventError ChunkEvent = "error"
	// EventEnd 结束事件
	EventEnd ChunkEvent = "end"
)

// ToolCallInStream 表示流式响应中的工具调用
type ToolCallInStream struct {
	ID     string                 // 工具调用 ID
	Name   string                 // 工具名称
	Input  map[string]interface{} // 工具输入参数
	Status string                 // 状态: "calling", "success", "error"
	Output string                 // 工具输出（执行后）
	Error  string                 // 错误信息（如果有）
}

// BeforeHook is a function that runs before the agent processes a message.
type BeforeHook func(ctx context.Context, message string) (context.Context, string, error)

// AfterHook is a function that runs after the agent generates a response.
type AfterHook func(ctx context.Context, resp *Response) error

// userQueryCtxKey 承载本轮用户原始问题（preProcess 入口写入）。
type userQueryCtxKey struct{}

// WithUserQuery 把本轮用户原始问题写入 ctx，供 Hook/Observer 按真实问题检索/聚类
// （而非误用系统提示词或已注入 playbook 的消息）。
func WithUserQuery(ctx context.Context, query string) context.Context {
	return context.WithValue(ctx, userQueryCtxKey{}, query)
}

// UserQueryFromContext 读取本轮用户原始问题；不存在时返回空串。
func UserQueryFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userQueryCtxKey{}).(string); ok {
		return v
	}
	return ""
}

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

	// Memory 和 Context Builder 支持
	memoryService  MemoryService  // 记忆服务接口
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

// ========================================
// StreamSink：输出侧差异抽象
// ========================================

// execSink 抽象「一次执行主干」的输出侧差异：
// bufferedSink 累积为 *Response（Chat）；streamSink 即时下发 *Chunk（Stream）。
// 主干 run 只依赖本接口，不再为 chat/stream × memory × tools 维护多份变体。
type execSink interface {
	// start 发出起始信号。streaming 发 start 事件；buffered 空实现。
	// 返回 false 表示下游已断开（streaming），主干应静默终止。
	start(ctx context.Context) bool

	// generate 产出一轮完整的 assistant 消息。
	// streaming 实现边收边下发 content 增量，收满后 ConcatMessages 合并返回；
	// buffered 实现直接 Generate。aborted=true 表示下游断开（streaming）。
	generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, iteration int) (msg *schema.Message, aborted bool, err error)

	// onToolCall / onToolResult 下发工具事件。buffered 空实现返回 true。
	// 返回 false 表示下游断开。
	onToolCall(ctx context.Context, ev *ToolCallInStream) bool
	onToolResult(ctx context.Context, ev *ToolCallInStream, iteration int) bool

	// fail 终态错误。streaming 发 error 事件；buffered 空实现（错误经 run 返回值上抛）。
	fail(ctx context.Context, err error)

	// finish 终态成功。buffered 执行 middleware.After + afterHooks（对 *Response 收尾）；
	// streaming 发 end 事件。
	finish(ctx context.Context, response *Response) error
}

// bufferedSink 是 Chat 的输出侧：累积为完整 *Response，收尾时跑 middleware.After + afterHooks。
type bufferedSink struct{ a *agentImpl }

func (s *bufferedSink) start(context.Context) bool { return true }

func (s *bufferedSink) generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, _ int) (*schema.Message, bool, error) {
	msg, err := m.Generate(ctx, msgs)
	return msg, false, err
}

func (s *bufferedSink) onToolCall(context.Context, *ToolCallInStream) bool        { return true }
func (s *bufferedSink) onToolResult(context.Context, *ToolCallInStream, int) bool { return true }
func (s *bufferedSink) fail(context.Context, error)                               {}

// finish 在 buffered 路径执行 middleware.After（逆序）+ afterHooks。
// 注意：流式路径正文已逐块下发、无完整 *Response 可供事后变换，故 After/afterHooks
// 仅在此收敛（Phase 3 刻意保留的输出侧边界，不做愈合）。
func (s *bufferedSink) finish(ctx context.Context, response *Response) error {
	for i := len(s.a.middleware) - 1; i >= 0; i-- {
		if err := s.a.middleware[i].After(ctx, response); err != nil {
			return err
		}
	}
	for _, hook := range s.a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return err
		}
	}
	return nil
}

// streamSink 是 Stream 的输出侧：把每一步即时下发为 *Chunk 事件。
type streamSink struct {
	a  *agentImpl
	ch chan *Chunk
}

func (s *streamSink) start(ctx context.Context) bool {
	return sendChunk(ctx, s.ch, &Chunk{Metadata: map[string]interface{}{"event": "start"}})
}

// generate 流式生成：内容分块即时下发，收满整条流后 ConcatMessages 合并返回。
func (s *streamSink) generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, iteration int) (*schema.Message, bool, error) {
	reader, err := m.Stream(ctx, msgs)
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()

	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if recvErr != nil {
			// io.EOF 是正常结束；其余是真实错误（网络中断等），必须上报而非静默截断。
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return nil, false, recvErr
		}
		if chunk == nil {
			break
		}
		chunks = append(chunks, chunk)

		// 内容分块即时下发以保证流式 UX；reasoning/tool_call 分块留待整流合并。
		if chunk.Content != "" {
			if !sendChunk(ctx, s.ch, &Chunk{
				Content: chunk.Content,
				Metadata: map[string]interface{}{
					"event":     string(EventContent),
					"iteration": iteration,
				},
			}) {
				return nil, true, nil // 客户端已断开
			}
		}
	}

	if len(chunks) == 0 {
		return &schema.Message{Role: schema.Assistant}, false, nil
	}

	// 收满整条流后再合并，得到完整 content / reasoning_content / tool_calls。
	// eino 流式协议下 tool_calls 以 delta 分块到达（按 Index 合并、参数分片拼接），
	// 绝不能只取首个带 tool_calls 的分块——否则参数残缺、assistant.tool_calls 与随后
	// 追加的 tool 消息数目错位，触发 "insufficient tool messages following tool_calls" 400。
	merged, concatErr := schema.ConcatMessages(chunks)
	if concatErr != nil {
		return nil, false, fmt.Errorf("合并流式响应失败: %w", concatErr)
	}
	return merged, false, nil
}

func (s *streamSink) onToolCall(ctx context.Context, ev *ToolCallInStream) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolCall),
			"tool_call": ev,
		},
	})
}

func (s *streamSink) onToolResult(ctx context.Context, ev *ToolCallInStream, iteration int) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolResult),
			"tool_call": ev,
			"iteration": iteration,
		},
	})
}

func (s *streamSink) fail(ctx context.Context, err error) {
	sendChunk(ctx, s.ch, &Chunk{
		Done: true,
		Metadata: map[string]interface{}{
			"event": string(EventError),
			"error": err.Error(),
		},
	})
}

// finish 发送 end 事件，并从 response.Metadata 透传 iterations/terminated_by/partial。
func (s *streamSink) finish(ctx context.Context, response *Response) error {
	meta := map[string]interface{}{"event": string(EventEnd)}
	if it, ok := response.Metadata["iterations"]; ok {
		meta["iterations"] = it
	}
	if tb, ok := response.Metadata["terminated_by"]; ok {
		meta["terminated_by"] = tb
		if tb == TerminatedByMaxIter {
			meta["max_reached"] = true
		}
	}
	if p, ok := response.Metadata["partial"]; ok {
		meta["partial"] = p
	}
	sendChunk(ctx, s.ch, &Chunk{Done: true, Metadata: meta})
	return nil
}

// bufferedStreamSink 是「启用输出护栏的流式会话」的输出侧：正文按缓冲方式产出（不逐块下发
// content），工具事件仍即时下发以保留进度 UX；finish 时先跑 middleware.After + afterHooks
// （含输出护栏脱敏/替换），再把最终安全内容作为单个 content 分块下发，最后发 end 事件。
// 这样兼顾「流式接口契约（仍是 chunk 流）」与「输出护栏必须作用于完整回答」两个约束。
type bufferedStreamSink struct {
	a  *agentImpl
	ch chan *Chunk
}

func (s *bufferedStreamSink) start(ctx context.Context) bool {
	return sendChunk(ctx, s.ch, &Chunk{Metadata: map[string]interface{}{"event": "start"}})
}

// generate 缓冲生成：直接 Generate 出完整消息，不逐块下发 content（护栏需作用于完整回答）。
func (s *bufferedStreamSink) generate(ctx context.Context, m model.BaseChatModel, msgs []*schema.Message, _ int) (*schema.Message, bool, error) {
	msg, err := m.Generate(ctx, msgs)
	return msg, false, err
}

// onToolCall / onToolResult 与 streamSink 一致：工具进度仍即时下发。
func (s *bufferedStreamSink) onToolCall(ctx context.Context, ev *ToolCallInStream) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolCall),
			"tool_call": ev,
		},
	})
}

func (s *bufferedStreamSink) onToolResult(ctx context.Context, ev *ToolCallInStream, iteration int) bool {
	return sendChunk(ctx, s.ch, &Chunk{
		Metadata: map[string]interface{}{
			"event":     string(EventToolResult),
			"tool_call": ev,
			"iteration": iteration,
		},
	})
}

func (s *bufferedStreamSink) fail(ctx context.Context, err error) {
	sendChunk(ctx, s.ch, &Chunk{
		Done: true,
		Metadata: map[string]interface{}{
			"event": string(EventError),
			"error": err.Error(),
		},
	})
}

// finish 先跑 middleware.After（逆序）+ afterHooks（含输出护栏），对完整 *Response 收尾脱敏，
// 再一次性下发最终安全内容与 end 事件。
func (s *bufferedStreamSink) finish(ctx context.Context, response *Response) error {
	for i := len(s.a.middleware) - 1; i >= 0; i-- {
		if err := s.a.middleware[i].After(ctx, response); err != nil {
			return err
		}
	}
	for _, hook := range s.a.afterHooks {
		if err := hook(ctx, response); err != nil {
			return err
		}
	}

	// 下发最终（已过护栏）内容为单个 content 分块。
	if response.Content != "" {
		if !sendChunk(ctx, s.ch, &Chunk{
			Content: response.Content,
			Metadata: map[string]interface{}{
				"event":    string(EventContent),
				"buffered": true,
			},
		}) {
			return nil // 客户端已断开
		}
	}

	// end 事件：透传与 streamSink.finish 一致的元数据。
	meta := map[string]interface{}{"event": string(EventEnd)}
	if it, ok := response.Metadata["iterations"]; ok {
		meta["iterations"] = it
	}
	if tb, ok := response.Metadata["terminated_by"]; ok {
		meta["terminated_by"] = tb
		if tb == TerminatedByMaxIter {
			meta["max_reached"] = true
		}
	}
	if p, ok := response.Metadata["partial"]; ok {
		meta["partial"] = p
	}
	sendChunk(ctx, s.ch, &Chunk{Done: true, Metadata: meta})
	return nil
}

// ========================================
// MemoryStrategy：上下文构建（读）+ 落库/摘要（写）
// ========================================

// runContext 承载一次执行的记忆态，贯穿主干用于后续落库/摘要与元数据填充。
type runContext struct {
	sessionID    string
	hasCollab    bool
	collabCtx    *domainagent.CollaborationContext
	builtCtx     *memory.BuildContext // 记忆构建成功时非 nil；仅用于元数据
	memoryActive bool
}

// buildInitialMessages 是 MemoryStrategy 的读入口：构建初始消息列表并落库本轮用户消息。
// 记忆未激活 → [System(prompt), User(message)]；激活 → 经 ContextBuilder 构建（按协作模式，
// 愈合：无论 chat/stream 均走 BuildForCollaboration），失败降级到默认消息。
func (a *agentImpl) buildInitialMessages(ctx context.Context, req runRequest) ([]*schema.Message, *runContext) {
	rc := &runContext{
		sessionID:    req.sessionID,
		memoryActive: a.enableMemory && a.contextBuilder != nil && req.sessionID != "",
	}

	def := []*schema.Message{
		schema.SystemMessage(a.prompt),
		schema.UserMessage(req.message),
	}
	if !rc.memoryActive {
		return def, rc
	}

	// 获取协作上下文（如果有）
	collabCtx, hasCollab := domainagent.GetCollaborationContext(ctx)
	rc.hasCollab = hasCollab
	rc.collabCtx = collabCtx

	// 构建上下文请求（CurrentMessage 用处理后的消息，Builder 会将其追加到 prompt 末尾）
	buildReq := &memory.BuildContextRequest{
		SessionID:      req.sessionID,
		CurrentMessage: req.message,
		Config: &memory.ContextBuilderConfig{
			SystemPrompt:  a.prompt,
			MaxTokens:     4000,
			ReserveTokens: 1000,
		},
	}

	var builtCtx *memory.BuildContext
	var err error
	if hasCollab {
		builtCtx, err = a.contextBuilder.BuildForCollaboration(ctx, buildReq, string(collabCtx.Mode), collabCtx.ContextLimit)
	} else {
		builtCtx, err = a.contextBuilder.Build(ctx, buildReq)
	}

	// 同步持久化本轮用户消息：存 rawMessage（用户原话），不存 hook 注入后的版本。
	// 必须放在 Build 之后：Build 已把处理后的 CurrentMessage 追加到 prompt 末尾，
	// 若先落库，历史里的 raw 与末尾的 processed 内容不同、去重失效，用户问题会在 prompt 出现两遍。
	if a.memoryService != nil {
		userMsg := &memory.Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			SessionID: req.sessionID,
			Type:      memory.MessageTypeUser,
			Role:      "user",
			Content:   req.rawMessage,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		_ = a.memoryService.SaveMessage(ctx, userMsg)
	}

	if err != nil || builtCtx == nil {
		return def, rc // 降级到简单模式（builtCtx 保持 nil）
	}
	rc.builtCtx = builtCtx

	// 构建 Eino 消息列表（包含历史对话）
	messages := make([]*schema.Message, 0, len(builtCtx.Messages))
	for _, msg := range builtCtx.Messages {
		messages = append(messages, &schema.Message{
			Role:    roleOf(msg.Role),
			Content: msg.Content,
		})
	}
	return messages, rc
}

// roleOf 把记忆消息的字符串角色转换为 Eino RoleType（未知角色回退为 user）。
func roleOf(role string) schema.RoleType {
	switch role {
	case "system":
		return schema.System
	case "user":
		return schema.User
	case "assistant":
		return schema.Assistant
	default:
		return schema.User
	}
}

// persistResult 是 MemoryStrategy 的写出口：落库助手响应并更新协作摘要。
// 仅记忆激活时生效（与旧记忆分支一致；非记忆分支从不触碰记忆/摘要）。
// 愈合：无论 chat/stream、有无工具，记忆激活即统一落库 + 更新摘要。
func (a *agentImpl) persistResult(ctx context.Context, rc *runContext, messages []*schema.Message, content string) {
	if !rc.memoryActive {
		return
	}

	// 保存助手响应到记忆
	if a.memoryService != nil && content != "" {
		assistantMsg := &memory.Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			SessionID: rc.sessionID,
			Type:      memory.MessageTypeAssistant,
			Role:      "assistant",
			Content:   content,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		go a.memoryService.SaveMessage(context.Background(), assistantMsg)
	}

	// 更新协作摘要
	if rc.hasCollab && rc.collabCtx != nil {
		// 获取最后一条用户消息
		lastUserMsg := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == schema.User {
				lastUserMsg = truncateString(messages[i].Content, 100)
				break
			}
		}

		newSummary := rc.collabCtx.Summary
		if newSummary == "" {
			newSummary = fmt.Sprintf("User: %s\nAssistant: %s", lastUserMsg, truncateString(content, 200))
		} else {
			newSummary += fmt.Sprintf("\nUser: %s\nAssistant: %s", lastUserMsg, truncateString(content, 200))
		}
		rc.collabCtx.UpdateSummary(newSummary)

		if a.memoryService != nil {
			// 分离 ctx（不随请求取消）但携带租户：首建摘要行需要 tenant_id，
			// 裸 context.Background() 会把 tenant_id=0 永久写入。
			tenantID, _ := domainagent.GetTenantID(ctx)
			bgCtx := domainagent.WithTenantID(context.Background(), tenantID)
			go a.memoryService.UpdateSummary(bgCtx, rc.sessionID, newSummary)
		}
	}
}

// fillMetadata 汇总响应元数据（愈合后各组合统一键位）。
func (a *agentImpl) fillMetadata(meta map[string]interface{}, rc *runContext, hasTools bool, res execResult) {
	if rc.memoryActive {
		meta["with_memory"] = true
	}
	if rc.builtCtx != nil && rc.builtCtx.Metadata != nil {
		meta["context_tokens"] = rc.builtCtx.Metadata.TotalTokens
		meta["history_messages"] = len(rc.builtCtx.History)
	}
	if hasTools {
		meta["with_tools"] = true
	}
	meta["iterations"] = res.iterations
	meta["tokens_used"] = res.tokensUsed
	if res.terminatedBy != "" {
		meta["terminated_by"] = res.terminatedBy
	}
	if res.partial {
		meta["partial"] = true
	}
	if res.role != "" {
		meta["role"] = res.role
	}
}

// ========================================
// ToolLoop：ReAct 执行循环（token 预算 + wind-down）
// ========================================

// ReAct 循环终止原因（写入 Response.Metadata["terminated_by"]）。
const (
	// TerminatedByMaxIter 表示达到最大迭代次数而终止。
	TerminatedByMaxIter = "max_iter"
	// TerminatedByTokenBudget 表示 token 预算耗尽而终止。
	TerminatedByTokenBudget = "token_budget"
)

// windDownPrompt 是达到 maxIter/预算上限时注入的收尾指令：要求模型不再调用工具，
// 直接基于已获得的观察结果交付一个自洽的最终答复，避免返回空内容或过程性话术。
//
// Bug B 修复：旧版只要求「部分结论」，模型在原地打转（如治理口径不匹配致 SUM=NULL）后
// 被动收尾时，常把「我发现了问题…我修正后重新执行」这类过程叙述当成最终答案交付——
// 用户拿到的是一句「我待会儿再算」而非任何数值或可用结论。此处显式禁止「将/会/稍后/
// 重新执行/继续尝试」等承诺性话术（本条回复之后没有新的执行轮次），并强制模型要么给出
// 已取到的具体数值/结果，要么直接说明卡点根因与基于现有信息的最佳可得结论。
const windDownPrompt = "已达到本轮的最大步数或 token 预算上限，请勿再调用任何工具。" +
	"注意：本条回复就是最终答复，之后不会再有新的执行轮次——" +
	"因此严禁使用「我将/我会/接下来/稍后/我修正后重新执行/继续尝试」之类的过程性或承诺性话术，" +
	"不要只描述你做了什么或打算怎么做。请直接用中文交付一个自洽的最终结论：" +
	"（1）优先给出你已经从观察结果中取到的具体数据/数值/结果，哪怕只是部分口径或中间结果；" +
	"（2）若因某个明确卡点（如口径/枚举值不匹配、工具报错）始终未取到目标结果，" +
	"请直接说明该卡点与你已定位到的根因或正确口径，并据此给出基于现有信息的最佳可得结论或估计；" +
	"（3）明确标注结果可能不完整及原因，必要时提示用户可如何继续（如按 result_id 取用、缩小范围或修正口径）。"

// usageTotalTokens 从一次生成响应中安全提取本轮消耗的 total tokens（缺失则 0）。
func usageTotalTokens(msg *schema.Message) int {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return 0
	}
	return msg.ResponseMeta.Usage.TotalTokens
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// execResult 汇总一次执行循环的产物，供 run 落库与填充元数据。
type execResult struct {
	content      string
	iterations   int
	tokensUsed   int
	terminatedBy string
	partial      bool
	role         string
	aborted      bool // 流式下游断开
}

// execLoop 是 ToolLoop 的核心：无工具时单轮生成；有工具时跑 ReAct 循环。
// 愈合：ReAct 循环的 token 预算前/后置检查与 wind-down 收尾对所有组合（含 memory+tools、streaming）统一生效。
func (a *agentImpl) execLoop(ctx context.Context, messages []*schema.Message, hasTools bool, sink execSink, response *Response) (execResult, error) {
	if !hasTools {
		// 确定使用的模型：优先 model，回退 toolModel。
		var chatModel model.BaseChatModel
		if a.model != nil {
			chatModel = a.model
		} else {
			chatModel = a.toolModel
		}

		msg, aborted, err := sink.generate(ctx, chatModel, messages, 1)
		if err != nil {
			return execResult{}, err
		}
		if aborted {
			return execResult{aborted: true}, nil
		}
		res := execResult{iterations: 1, tokensUsed: usageTotalTokens(msg)}
		if msg != nil {
			res.content = msg.Content
			res.role = string(msg.Role)
		}
		return res, nil
	}

	// 提取所有工具的 ToolInfo 并绑定到模型。
	// 按工具名去重：LLM 供应商（OpenAI/DeepSeek 兼容接口）硬性要求 tools 数组内工具名唯一，
	// 上游任一装配路径若混入同名工具都会导致 400 "Tool names must be unique"。此处收口去重
	// （保留首个、丢弃后续同名），并对被丢弃者打 WARN 便于回溯真正的重复来源。
	toolInfos := make([]*schema.ToolInfo, 0, len(a.tools))
	seenTools := make(map[string]struct{}, len(a.tools))
	for _, t := range a.tools {
		info, infoErr := t.Info(ctx)
		if infoErr != nil {
			continue // 跳过无法获取信息的工具
		}
		if _, dup := seenTools[info.Name]; dup {
			log.Printf("[agent:%s] 丢弃重名工具 %q（tools 数组要求唯一），请检查上游工具装配", a.name, info.Name)
			continue
		}
		seenTools[info.Name] = struct{}{}
		toolInfos = append(toolInfos, info)
	}
	var boundModel model.ToolCallingChatModel = a.toolModel
	if len(toolInfos) > 0 {
		bound, bindErr := a.toolModel.WithTools(toolInfos)
		if bindErr != nil {
			return execResult{}, fmt.Errorf("bind tools to model failed: %w", bindErr)
		}
		boundModel = bound
	}

	// 迭代处理（可能需要多轮工具调用）。
	// ReAct 循环受 maxIter 与 token 预算共同约束：任一到达上限即终止并收尾。
	var res execResult
	naturalFinish := false // 模型主动收尾（返回无工具调用的回复）；区别于达上限被动终止
	// 自我修复护栏：每次运行私有，按失败签名计数触发再规划/提前收尾（并发安全）。
	guard := newFailureGuard()
	for i := 0; i < a.maxIter; i++ {
		// token 预算前置检查：预算已耗尽则不再发起新一轮生成。
		if a.tokenBudget > 0 && res.tokensUsed >= a.tokenBudget {
			res.terminatedBy = TerminatedByTokenBudget
			res.iterations = i
			break
		}

		msg, aborted, err := sink.generate(ctx, boundModel, messages, i+1)
		if err != nil {
			return execResult{}, err
		}
		if aborted {
			return execResult{aborted: true}, nil
		}
		res.tokensUsed += usageTotalTokens(msg)

		// 检查是否有工具调用
		if len(msg.ToolCalls) == 0 {
			// 没有工具调用，模型主动收尾（Content 可能为空，但仍属自然结束，不应误判为达上限）
			res.content = msg.Content
			res.role = string(msg.Role)
			res.iterations = i + 1
			naturalFinish = true
			break
		}

		// assistant 轮整体只入队一次：保留 msg 自带的 reasoning_content 与全部 tool_calls，
		// 满足 DeepSeek V4 thinking "工具调用轮必须原样回传 reasoning_content" 的约束，
		// 同时避免把并行工具调用拆成多条畸形 assistant 消息。
		messages = append(messages, msg)

		// 处理工具调用
		for _, tc := range msg.ToolCalls {
			obs, ok := a.handleToolCall(ctx, tc, i+1, sink, response, guard)
			if !ok {
				return execResult{aborted: true}, nil
			}
			messages = append(messages, obs)
		}

		// 自我修复护栏：同一失败签名达阈值 → 注入一次再规划提示（引导换路径，非盲目重试）。
		if note := guard.replanNote(); note != "" {
			messages = append(messages, schema.SystemMessage(note))
		}
		// 累计失败过多（原地打转）→ 提前收尾，交由下方 wind-down 给出诚实的部分结论。
		if guard.shouldWindDown() {
			res.terminatedBy = TerminatedByRepairExhausted
			res.iterations = i + 1
			break
		}

		// 本轮工具执行后再判预算：耗尽则标记终止，交由下方 wind-down 收尾。
		if a.tokenBudget > 0 && res.tokensUsed >= a.tokenBudget {
			res.terminatedBy = TerminatedByTokenBudget
			res.iterations = i + 1
			break
		}
	}

	// 循环耗尽 maxIter 却未自然收尾（最后一步仍是工具调用）→ 标记 max_iter。
	// 以 naturalFinish 判定而非 Content==""：模型主动收尾但返回空内容时仍属自然结束。
	if !naturalFinish && res.terminatedBy == "" {
		res.terminatedBy = TerminatedByMaxIter
		res.iterations = a.maxIter
	}

	// wind-down：因达到 maxIter 或预算耗尽而被动终止（非自然收尾）时，做一次「无工具」收尾生成，
	// 让模型基于已观察到的结果（Scratchpad/result_id 信封）给出部分结论，而非返回空串。
	if !naturalFinish && res.terminatedBy != "" {
		windMsgs := append(messages, schema.SystemMessage(windDownPrompt))
		final, aborted, ferr := sink.generate(ctx, a.toolModel, windMsgs, res.iterations+1)
		if aborted {
			return execResult{aborted: true}, nil
		}
		if ferr == nil && final != nil {
			res.content = final.Content
			res.role = string(final.Role)
			res.tokensUsed += usageTotalTokens(final)
		}
		res.partial = true
	}

	return res, nil
}

// handleToolCall 执行单个工具调用：解析参数 → 下发工具事件（streaming）→ 执行 → 记录 ToolCall
// → 返回追加到历史的观察消息。ok=false 表示下游断开（streaming）。
// 愈合：参数不可解析时合成错误观察、不执行工具（原 buffered 变体会带残缺参数执行）；
// 无论解析成败都追加一条 tool 消息，保证 assistant.tool_calls 与 tool 消息严格 1:1。
func (a *agentImpl) handleToolCall(ctx context.Context, tc schema.ToolCall, iteration int, sink execSink, response *Response, guard *failureGuard) (*schema.Message, bool) {
	toolCall := &ToolCall{Name: tc.Function.Name}

	// 解析参数
	var args map[string]interface{}
	parseErr := false
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			toolCall.Error = fmt.Errorf("invalid arguments: %w", err)
			toolCall.Input = nil
			parseErr = true
		} else {
			toolCall.Input = args
		}
	}

	// 参数不可解析：合成错误观察，不执行工具。仍须为该 tool_call 追加一条 tool 消息，
	// 保证 assistant.tool_calls 与 tool 消息严格 1:1，杜绝下一轮生成因悬空
	// tool_call 触发 "insufficient tool messages following tool_calls" 400。
	if parseErr {
		if !sink.onToolCall(ctx, &ToolCallInStream{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Status: "error",
			Error:  fmt.Sprintf("参数解析失败: %v", toolCall.Error),
		}) {
			return nil, false
		}
		toolCall.Output = fmt.Sprintf("Error: 参数解析失败: %v", toolCall.Error)
		response.ToolCalls = append(response.ToolCalls, toolCall)
		return schema.ToolMessage(compactObservation(toolCall.Output), tc.ID), true
	}

	// 发送工具调用事件（buffered 空实现）
	if !sink.onToolCall(ctx, &ToolCallInStream{
		ID:     tc.ID,
		Name:   tc.Function.Name,
		Input:  args,
		Status: "calling",
	}) {
		return nil, false
	}

	// 执行工具
	output, synthetic, execErr := a.invokeTool(ctx, tc)

	// 逐工具输出护栏（post-invoke，任务 5）：成功观察在回灌 LLM 与下发事件前脱敏；
	// 未装配时零开销、观察逐字节不变；错误观察不脱敏（保留原始报错供自我修正）。
	// synthetic（工具门合成的 tool_blocked / pending_confirm 控制面载荷）不脱敏——
	// 其非真实工具观察，且携 confirm_token 等控制字段，脱敏会破坏确认续跑（不可误伤）。
	if execErr == nil && !synthetic && a.toolOutputHook != nil {
		output = a.toolOutputHook(ctx, tc.Function.Name, output)
	}

	// 组装工具结果事件
	resultEvent := &ToolCallInStream{
		ID:     tc.ID,
		Name:   tc.Function.Name,
		Input:  args,
		Status: "success",
		Output: output,
	}
	// 可修复观察（RepairableToolError）不是裸报错：其 Observation 为结构化 JSON（error_kind/
	// retriable/hint/detail），应原样回灌 LLM 引导定向修正，而非渲染成 "Error: %v"。
	// 同时把 error_kind 计入失败签名（下方 recordToolFailure），触发再规划/提前收尾。
	obs := output
	if execErr != nil {
		toolCall.Error = execErr
		resultEvent.Status = "error"
		resultEvent.Error = execErr.Error()
		if repair, ok := AsRepairable(execErr); ok {
			obs = repair.Observation
			toolCall.Output = repair.Observation
			guard.recordFailure(tc.Function.Name, repair.ErrorKind)
		} else {
			obs = fmt.Sprintf("Error: %v", execErr)
			toolCall.Output = obs
			guard.recordFailure(tc.Function.Name, "")
		}
	} else {
		toolCall.Output = output
		guard.recordSuccess(tc.Function.Name)
	}

	if !sink.onToolResult(ctx, resultEvent, iteration) {
		return nil, false
	}
	response.ToolCalls = append(response.ToolCalls, toolCall)

	// 追加该 tool_call 对应的结果消息
	return schema.ToolMessage(compactObservation(obs), tc.ID), true
}

// invokeTool 执行单个工具调用。synthetic 为 true 时 output 为工具门合成的控制面载荷
// （tool_blocked / pending_confirm，未触达底层工具），调用方据此跳过逐工具输出护栏，
// 避免脱敏破坏 confirm_token 等控制字段。
func (a *agentImpl) invokeTool(ctx context.Context, toolCall schema.ToolCall) (output string, synthetic bool, err error) {
	// 工具级 span：挂在外层 agent.chat 之下，还原调用链瀑布（含被门拒/待确认的合成结果，
	// 便于排查为何某工具未真正执行）。otel 未注册真实 provider 时为 no-op、零开销。
	ctx, span := frameworkTracer.StartToolSpan(ctx, a.name, toolCall.Function.Name, toolCall.Function.Arguments)
	defer func() {
		span.SetAttribute("tool.output_length", len(output))
		span.SetAttribute("tool.synthetic", synthetic)
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()

	// 执行前硬工具门（Phase 6 + 护栏三值门）：skill 策略 + 会话 scope 同为必要条件，
	// 其上叠加写/导出审批第三态。被拒/需审批调用不触达底层执行，以合成
	// tool_blocked / 待人工确认 ToolMessage 回灌 LLM。
	if blocked, ok := gateToolCall(ctx, toolCall.Function.Name, toolCall.Function.Arguments); !ok {
		return blocked, true, nil
	}

	// 查找工具
	var selectedTool tool.BaseTool
	for _, t := range a.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == toolCall.Function.Name {
			selectedTool = t
			break
		}
	}

	if selectedTool == nil {
		return "", false, fmt.Errorf("tool not found: %s", toolCall.Function.Name)
	}

	// 执行工具
	switch t := selectedTool.(type) {
	case tool.InvokableTool:
		out, runErr := t.InvokableRun(ctx, toolCall.Function.Arguments)
		return out, false, runErr
	case tool.StreamableTool:
		stream, err := t.StreamableRun(ctx, toolCall.Function.Arguments)
		if err != nil {
			return "", false, fmt.Errorf("streamable run failed: %w", err)
		}

		// 收集流式结果
		var result strings.Builder
		for {
			chunk, err := stream.Recv()
			if err != nil {
				// errors.Is 而非字符串比较：包装过的 io.EOF 也能识别为正常结束
				if errors.Is(err, io.EOF) {
					break
				}
				stream.Close()
				return "", false, fmt.Errorf("recv failed: %w", err)
			}
			result.WriteString(chunk)
		}
		stream.Close()
		return result.String(), false, nil
	default:
		return "", false, fmt.Errorf("unsupported tool type: %T", t)
	}
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
func compactObservation(output string) string {
	runes := []rune(output)
	if len(runes) <= maxObservationChars {
		return output
	}
	return string(runes[:maxObservationChars]) +
		fmt.Sprintf("\n...[观察已截断，省略 %d 字符；如需完整数据请按 result_id 取用]", len(runes)-maxObservationChars)
}
