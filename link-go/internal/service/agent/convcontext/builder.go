// Package convcontext 提供基于会话消息表（messages）的对话上下文构建器，
// 补齐 framework 记忆扩展点长期缺失的 ContextBuilder 具体实现，使 Agent 具备跨轮对话记忆。
//
// 设计要点：
//   - 单一数据源：直接读 persistence.go 与前端共用的 messages 表（conversation.MessageRepository），
//     不引入 memory 子系统的 conversation_messages 第二套存储，避免双写与 UI 展示分叉。
//   - 只读不写：消息落库仍由 handler 侧 persistence.go 承担（SaveUserMessage/SaveAssistantMessage），
//     本构建器仅按会话装配 [系统提示 + 历史 + 当前问题] 供 LLM 使用，不做任何写入。
//   - 能力/业务分离：窗口化、抽取式摘要、分层 token 预算治理等「上下文工程能力」全部下沉到
//     internal/service/agent/context 包（纯函数、可复用、自带默认计数器）；本构建器只做业务装配，
//     负责读消息表、把历史喂给能力层、并把结果映射回 memory.Message。
//
// 读/写路径边界（架构加固 Phase 6，见 agentstate 包门面）：本包是「跨轮记忆只读投影」
// 读路径；UI 会话写入（chat.session：service/chat + model/conversation 落 messages）是写路径。
// 二者以 messages 表为单一数据源、以本构建器的只读契约为分界——写路径落库、读路径回放，
// 读投影不得反向改写会话写入状态，防止写路径污染读投影。
package convcontext

import (
	"context"

	"link/internal/model/conversation"
	"link/internal/model/memory"
	ctxeng "link/internal/service/agent/context"
)

// defaultHistoryLimit 默认从消息表回放的最近历史条数（窗口/摘要在此基础上按 token 预算细化）。
// 放宽到 200：长对话下让 token 预算（约 64k）成为真正约束，而非被 20 条的条数硬顶提前截断。
const defaultHistoryLimit = 200

// ConversationContextBuilder 基于 messages 表的上下文构建器。
// 结构上实现 framework.ContextBuilder（Build + BuildForCollaboration）。
type ConversationContextBuilder struct {
	messageRepo  conversation.MessageRepository
	historyLimit int
	// counter 为 token 预算治理与窗口化用的计数器；默认能力层自带的近似计数器（不依赖基础设施层）。
	counter ctxeng.TokenCounter
	// summarizer 为超窗早期轮的摘要器；nil 时能力层退化为确定性抽取式摘要（逐轮截断）。
	// 业务侧可注入 LLM 语义摘要器（llmsummary），让开场装配与 ③ 折叠共用一套语义压缩。
	summarizer ctxeng.Summarizer
}

// NewConversationContextBuilder 创建上下文构建器。messageRepo 为 nil 时退化为仅回放当前问题。
func NewConversationContextBuilder(messageRepo conversation.MessageRepository) *ConversationContextBuilder {
	return &ConversationContextBuilder{
		messageRepo:  messageRepo,
		historyLimit: defaultHistoryLimit,
		counter:      ctxeng.ApproxTokenCounter{},
	}
}

// WithSummarizer 注入超窗早期轮的摘要器（能力层 ctxeng.Summarizer）。传 nil 保持默认抽取式行为。
// 用于让开场历史装配与运行时 ③ 折叠复用同一个 LLM 语义摘要器，语义口径一致。
func (b *ConversationContextBuilder) WithSummarizer(s ctxeng.Summarizer) *ConversationContextBuilder {
	b.summarizer = s
	return b
}

// WithCounter 注入 token 计数器（能力层 ctxeng.TokenCounter），用于开场装配的窗口化与预算治理。
// 不注入时保持默认的字符启发式 ApproxTokenCounter；注入真实 BPE 分词器（bpecount）可与运行时
// ③ 折叠共用同一计数口径，避免开场与运行时对同段历史的 token 估算不一致。传 nil 忽略。
func (b *ConversationContextBuilder) WithCounter(c ctxeng.TokenCounter) *ConversationContextBuilder {
	if c != nil {
		b.counter = c
	}
	return b
}

// Build 按会话装配 LLM 消息列表：系统提示 + 历史对话 + 当前问题。
//
// 上下文工程（能力层驱动）：
//   - 窗口化按 token 预算而非条数：从最新往回累计，超预算的更早轮次不再逐字回灌；
//   - 摘要：超窗早期轮被抽取式摘要压成一条紧凑记忆（保留 result_id 与结论），而非静默丢弃；
//   - 预算治理：系统提示为 Pinned 层永不裁剪，摘要作为最低优先的记忆层，预算紧张时先被截断/丢弃。
//
// 未提供 token 预算（Config 缺失或可用预算<=0）时退化为「最近 N 条逐字回放」的原始行为，零回归。
//
// 幂等去重：handler 在开流前已把当前用户消息落库，历史加载会带出它；
// 若历史末条恰为当前问题则不再重复追加，否则（例如落库失败）补上，
// 保证当前问题一定作为最后一条 user 消息存在。
func (b *ConversationContextBuilder) Build(ctx context.Context, req *memory.BuildContextRequest) (*memory.BuildContext, error) {
	limit := b.historyLimit
	if req.HistoryLimit > 0 {
		limit = req.HistoryLimit
	}

	var systemPrompt string
	available := 0
	if req.Config != nil {
		systemPrompt = req.Config.SystemPrompt
		available = req.Config.GetAvailableTokens()
	}

	// 加载并清洗历史（时间正序，仅 user/assistant 非空内容，跳过系统/工具噪声）。
	history, err := b.loadHistory(ctx, req.SessionID, limit)
	if err != nil {
		return nil, err
	}

	// 分层组装：纯系统提示 + 独立的早期轮摘要（可选）+ 结构化最近历史。
	systemContent, summaryContent, recent := b.assemble(ctx, systemPrompt, req.CurrentMessage, history, available)

	messages := make([]*memory.Message, 0, len(recent)+3)
	if systemContent != "" {
		messages = append(messages, &memory.Message{
			Type:    memory.MessageTypeSystem,
			Role:    "system",
			Content: systemContent,
		})
	}
	// 缓存前缀稳定性（③）：早期轮摘要作为紧邻系统提示的独立记忆消息注入，绝不并入 message[0]。
	// 系统提示逐字稳定 → 服务端前缀/KV 缓存可覆盖「系统提示 + 工具契约」这段最大的静态前缀；
	// 布局与运行时 ③ 折叠一致（[纯系统][摘要][最近轮][当前问题]），开场装配与折叠口径统一。
	if summaryContent != "" {
		messages = append(messages, &memory.Message{
			Type:    memory.MessageTypeSystem,
			Role:    "system",
			Content: summaryContent,
		})
	}
	messages = append(messages, recent...)

	// 确保当前问题在末尾（与最近历史末条去重）。
	if req.CurrentMessage != "" {
		last := lastMessage(recent)
		if last == nil || last.Role != conversation.RoleUser || last.Content != req.CurrentMessage {
			messages = append(messages, &memory.Message{
				Type:    memory.MessageTypeUser,
				Role:    "user",
				Content: req.CurrentMessage,
			})
		}
	}

	return &memory.BuildContext{
		SystemPrompt: systemPrompt,
		History:      recent,
		Messages:     messages,
	}, nil
}

// loadHistory 读消息表并清洗为时间正序的 user/assistant 非空历史。
func (b *ConversationContextBuilder) loadHistory(ctx context.Context, sessionID string, limit int) ([]*memory.Message, error) {
	history := make([]*memory.Message, 0, limit)
	if b.messageRepo == nil || sessionID == "" {
		return history, nil
	}
	rows, err := b.messageRepo.FindRecentBySessionID(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		if m.Content == "" {
			continue
		}
		if m.Role != conversation.RoleUser && m.Role != conversation.RoleAssistant {
			continue
		}
		history = append(history, &memory.Message{
			Type:    roleToType(m.Role),
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return history, nil
}

// assemble 施加窗口化 + 摘要 + 预算治理，返回三段：
//   - systemContent：始终为纯系统提示（Pinned，逐字稳定，永不并入摘要——服务端前缀缓存的地基）。
//   - summaryContent：超窗早期轮的紧凑记忆摘要（保 result_id/结论），作为紧邻系统提示的独立消息；
//     预算充足或摘要失败时为 ""（不注入）。
//   - recent：保留原文的最近历史。
//
// 边界：available<=0 或无计数器或历史为空 → 退化为原始行为（系统提示原样 + 全量历史逐字），零回归。
func (b *ConversationContextBuilder) assemble(
	ctx context.Context,
	systemPrompt, currentMessage string,
	history []*memory.Message,
	available int,
) (systemContent, summaryContent string, recent []*memory.Message) {
	if available <= 0 || b.counter == nil || len(history) == 0 {
		return systemPrompt, "", history
	}

	turns := historyToTurns(history)
	sysTokens := b.counter.Count(systemPrompt)
	curTokens := b.counter.Count(currentMessage)

	// 历史预算 = 可用预算 - 系统层(Pinned) - 当前问题；据此从最新往回定保留轮数。
	historyBudget := available - sysTokens - curTokens
	keep := ctxeng.KeepRecentByTokens(turns, historyBudget, b.counter)
	if keep >= len(history) {
		// 全部历史都在预算内：逐字保留，无需摘要。
		return systemPrompt, "", history
	}

	// 超窗：最近 keep 条逐字保留，更早轮次交给能力层摘要（保 result_id/结论）。
	recent = history[len(history)-keep:]
	windower := ctxeng.NewWindower(ctxeng.WindowConfig{KeepRecentTurns: keep}, b.summarizer)
	res, err := windower.Apply(ctx, turns)
	if err != nil || res.Summary == "" {
		// 摘要失败则安全降级：不注入摘要，仅保留最近窗口（早期轮丢弃，绝不因摘要错误阻断对话）。
		return systemPrompt, "", recent
	}

	// 预算治理：系统提示 Pinned 独占其 token 且永不裁剪（作为独立 message[0] 发出，不参与拼接）；
	// 早期摘要作为最低优先记忆层，仅在「可用预算 - 系统层 - 当前问题 - 最近轮」的剩余里按需截断/丢弃。
	recentTokens := 0
	for _, m := range recent {
		recentTokens += b.counter.Count(m.Content)
	}
	memBudget := available - sysTokens - curTokens - recentTokens
	asm := ctxeng.Assemble(
		[]ctxeng.Layer{ctxeng.MemoryLayer(res.Summary)},
		ctxeng.BudgetConfig{MaxTokens: memBudget},
		b.counter,
	)
	// 硬不变量兜底：摘要（记忆层）可能被预算截断/丢弃，但被引用的 result_id 不能因此丢失，
	// 否则后续轮次无法按引用取回完整结果集——预算截断之后再兜一次。
	summaryContent = ctxeng.EnsureResultIDsPresent(asm.Prompt, res.PreservedResultIDs)
	return systemPrompt, summaryContent, recent
}

// historyToTurns 把历史消息映射为能力层的紧凑轮次视图（result_id 由能力层从内容正则抽取）。
func historyToTurns(history []*memory.Message) []ctxeng.Turn {
	turns := make([]ctxeng.Turn, 0, len(history))
	for _, m := range history {
		turns = append(turns, ctxeng.Turn{Role: m.Role, Content: m.Content})
	}
	return turns
}

// BuildForCollaboration 协作模式下的上下文构建。Data Agent 主入口的历史回放语义与普通对话一致，
// 因此直接复用 Build；contextLimit 作为历史条数上限透传。保留签名以满足 framework.ContextBuilder 接口。
func (b *ConversationContextBuilder) BuildForCollaboration(ctx context.Context, req *memory.BuildContextRequest, mode string, contextLimit int) (*memory.BuildContext, error) {
	if contextLimit > 0 {
		req.HistoryLimit = contextLimit
	}
	return b.Build(ctx, req)
}

// roleToType 将对话角色映射为 memory 消息类型。
func roleToType(role string) memory.MessageType {
	switch role {
	case conversation.RoleAssistant:
		return memory.MessageTypeAssistant
	case conversation.RoleSystem:
		return memory.MessageTypeSystem
	default:
		return memory.MessageTypeUser
	}
}

// lastMessage 返回切片末条，空切片返回 nil。
func lastMessage(msgs []*memory.Message) *memory.Message {
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
