// 由 eino_agent.go 拆出——同包、行为等价（M1 god-file 拆分）。
package framework

import (
	domainagent "cognida/internal/model/agent"
	"cognida/internal/model/memory"
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

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

	// 构建上下文请求（CurrentMessage 用处理后的消息，Builder 会将其追加到 prompt 末尾）。
	// 开场装配预算：默认 4000；启用运行时压缩时对齐折叠目标水位 compactTarget，
	// 使「开场装配」与「循环内折叠」用同一水位，长对话回放不再被 3k 死顶。
	maxTok := 4000
	reserveTok := 1000
	if a.compactTarget > 0 {
		maxTok = a.compactTarget
		reserveTok = 0
	}
	buildReq := &memory.BuildContextRequest{
		SessionID:      req.sessionID,
		CurrentMessage: req.message,
		Config: &memory.ContextBuilderConfig{
			SystemPrompt:  a.prompt,
			MaxTokens:     maxTok,
			ReserveTokens: reserveTok,
		},
	}

	var builtCtx *memory.BuildContext
	var err error
	if hasCollab {
		builtCtx, err = a.contextBuilder.BuildForCollaboration(ctx, buildReq, string(collabCtx.Mode), collabCtx.ContextLimit)
	} else {
		builtCtx, err = a.contextBuilder.Build(ctx, buildReq)
	}

	// 用户/助手消息的落库统一由 handler 层 AgentPersistenceService 负责
	// （SaveUserMessage/SaveAssistantMessage → messageRepo），此处只读装配历史，不再重复写。

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

// persistResult 是 MemoryStrategy 的写出口：更新本轮协作摘要（in-memory）。
// 仅记忆激活时生效（非记忆分支从不触碰记忆/摘要）。
// 助手响应的落库由 handler 层 AgentPersistenceService.SaveAssistantMessage 负责，此处不重复写。
func (a *agentImpl) persistResult(_ context.Context, rc *runContext, messages []*schema.Message, content string) {
	if !rc.memoryActive {
		return
	}

	// 更新协作摘要（in-memory：供本请求内其他协作参与者读取同一份 collabCtx）
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
