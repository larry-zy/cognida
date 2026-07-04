// Package convcontext 提供基于会话消息表（messages）的对话上下文构建器，
// 补齐 framework 记忆扩展点长期缺失的 ContextBuilder 具体实现，使 Agent 具备跨轮对话记忆。
//
// 设计要点：
//   - 单一数据源：直接读 persistence.go 与前端共用的 messages 表（conversation.MessageRepository），
//     不引入 memory 子系统的 conversation_messages 第二套存储，避免双写与 UI 展示分叉。
//   - 只读不写：消息落库仍由 handler 侧 persistence.go 承担（SaveUserMessage/SaveAssistantMessage），
//     本构建器仅按会话装配 [系统提示 + 历史 + 当前问题] 供 LLM 使用，不做任何写入。
package convcontext

import (
	"context"

	"link/internal/model/conversation"
	"link/internal/model/memory"
)

// defaultHistoryLimit 默认回放的历史消息条数（约等于最近若干轮对话）。
const defaultHistoryLimit = 20

// ConversationContextBuilder 基于 messages 表的上下文构建器。
// 结构上实现 framework.ContextBuilder（Build + BuildForCollaboration）。
type ConversationContextBuilder struct {
	messageRepo  conversation.MessageRepository
	historyLimit int
}

// NewConversationContextBuilder 创建上下文构建器。messageRepo 为 nil 时退化为仅回放当前问题。
func NewConversationContextBuilder(messageRepo conversation.MessageRepository) *ConversationContextBuilder {
	return &ConversationContextBuilder{
		messageRepo:  messageRepo,
		historyLimit: defaultHistoryLimit,
	}
}

// Build 按会话装配 LLM 消息列表：系统提示 + 历史对话 + 当前问题。
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
	if req.Config != nil {
		systemPrompt = req.Config.SystemPrompt
	}

	messages := make([]*memory.Message, 0, limit+2)
	if systemPrompt != "" {
		messages = append(messages, &memory.Message{
			Type:    memory.MessageTypeSystem,
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// 加载历史对话（时间正序），仅保留 user/assistant 的非空内容，跳过系统/工具噪声
	history := make([]*memory.Message, 0, limit)
	if b.messageRepo != nil && req.SessionID != "" {
		rows, err := b.messageRepo.FindRecentBySessionID(ctx, req.SessionID, limit)
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
	}
	messages = append(messages, history...)

	// 确保当前问题在末尾（与历史末条去重）
	if req.CurrentMessage != "" {
		last := lastMessage(history)
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
		History:      history,
		Messages:     messages,
	}, nil
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
