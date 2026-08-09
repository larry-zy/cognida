// Package handler 提供聊天功能的HTTP处理器
package handler

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	domainllm "cognida/internal/model/llm"
	"cognida/internal/handler/sse"
)

// ========================================
// ChatService 接口（用于依赖注入和测试）
// ========================================

// ChatService 定义聊天服务接口
type ChatService interface {
	Chat(ctx context.Context, req *domainllm.ChatRequest) (*domainllm.ChatResponse, error)
	ChatStream(ctx context.Context, req *domainllm.ChatRequest) (<-chan *domainllm.ChatChunk, error)
}

// ========================================
// ChatHandler 聊天处理器
// ========================================

// ChatHandler 聊天处理器
type ChatHandler struct {
	chatService ChatService
}

// NewChatHandler 创建聊天处理器
func NewChatHandler(chatService ChatService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
	}
}

// Chat 聊天入口
// POST /api/v1/chat
func (h *ChatHandler) Chat(c *gin.Context) {
	var req ChatRequest
	if !BindJSON(c, &req) {
		return
	}

	// 从上下文获取租户和用户ID
	tenantID := GetTenantID(c)
	userID := GetUserID(c)

	// 构建上下文
	ctx := c.Request.Context()
	ctx = ContextWithTenantID(ctx, tenantID)
	ctx = ContextWithUserID(ctx, userID)

	// 构建应用层聊天请求
	chatReq, err := h.buildChatRequest(ctx, req)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	// 验证请求
	if len(chatReq.Messages) == 0 {
		BadRequest(c, "messages is required")
		return
	}

	// 判断是否流式响应
	if req.Stream {
		h.chatStream(c, ctx, chatReq)
		return
	}

	// 非流式响应
	resp, err := h.chatService.Chat(ctx, chatReq)
	if err != nil {
		log.Printf("[Chat] 聊天失败: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, toChatResponse(resp))
}

// chatStream 处理流式聊天请求
func (h *ChatHandler) chatStream(c *gin.Context, ctx context.Context, req *domainllm.ChatRequest) {
	// 设置 SSE 响应头
	sse.SetSSEHeaders(c.Writer)

	// 启动心跳机制
	stopHeartbeat := sse.StartHeartbeat(c.Request.Context(), c.Writer, nil)
	defer stopHeartbeat()

	// 执行流式聊天
	streamChan, err := h.chatService.ChatStream(ctx, req)
	if err != nil {
		log.Printf("[ChatStream] 流式聊天失败: %v", err)
		sse.SendError(c.Writer, err)
		return
	}

	// 发送流式数据
	for chunk := range streamChan {
		eventType := "content"
		if chunk.Done {
			eventType = "end"
		}
		sse.SendSSE(c.Writer, eventType, map[string]interface{}{
			"content": chunk.Content,
			"done":    chunk.Done,
			"role":    chunk.Role,
		})
	}
}

// ========================================
// 请求/响应 DTO
// ========================================

// ChatRequest 聊天请求
type ChatRequest struct {
	Content   string             `json:"content,omitempty"`
	History   []*ChatMessage     `json:"history,omitempty"`
	Options   *ChatOptions       `json:"options,omitempty"`
	SessionID string             `json:"session_id,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
	Messages  []*Message         `json:"messages,omitempty"` // 新格式
	AgentID   string             `json:"agent_id,omitempty"` // Agent ID
}

// ChatMessage 聊天消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Message 消息（新格式，兼容前端）
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatOptions 聊天选项
type ChatOptions struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Content    string                 `json:"content"`
	RequestID  string                 `json:"request_id"`
	SessionID  string                 `json:"session_id,omitempty"`
	AgentID    string                 `json:"agent_id,omitempty"`
	TokenCount int                    `json:"token_count,omitempty"`
	ToolCalls  []ToolCallResponse     `json:"tool_calls,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ToolCallResponse 工具调用响应
type ToolCallResponse struct {
	ID   string              `json:"id"`
	Type string              `json:"type"`
	Function FunctionResponse `json:"function"`
}

// FunctionResponse 函数响应
type FunctionResponse struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ========================================
// Helper Functions
// ========================================

// buildChatRequest 构建 Application 层聊天请求
func (h *ChatHandler) buildChatRequest(ctx context.Context, req ChatRequest) (*domainllm.ChatRequest, error) {
	// 转换消息格式
	messages := h.convertMessages(req)

	// 构建 ChatRequest
	chatReq := &domainllm.ChatRequest{
		Messages:  messages,
		SessionID: req.SessionID,
	}

	// 转换选项
	if req.Options != nil {
		chatReq.Options = &domainllm.ChatOptions{}
		// Temperature/TopP 直接透传指针：nil=未设置，显式值（含 0=确定性采样）如实下传，
		// 不再在此 deref 后被下游 `> 0` 守卫丢失。
		chatReq.Options.Temperature = req.Options.Temperature
		chatReq.Options.TopP = req.Options.TopP
		if req.Options.MaxTokens != nil {
			chatReq.Options.MaxTokens = *req.Options.MaxTokens
		}
		if req.Options.FrequencyPenalty != nil {
			chatReq.Options.FrequencyPenalty = *req.Options.FrequencyPenalty
		}
		if req.Options.PresencePenalty != nil {
			chatReq.Options.PresencePenalty = *req.Options.PresencePenalty
		}
	}

	return chatReq, nil
}

// convertMessages 转换消息格式
func (h *ChatHandler) convertMessages(req ChatRequest) []*domainllm.Message {
	// 优先使用 Messages 格式
	if len(req.Messages) > 0 {
		messages := make([]*domainllm.Message, len(req.Messages))
		for i, msg := range req.Messages {
			messages[i] = &domainllm.Message{
				Role:    msg.Role,
				Content: msg.Content,
			}
		}
		return messages
	}

	// 使用 Content + History 格式
	messages := make([]*domainllm.Message, 0, len(req.History)+1)

	// 添加历史消息
	for _, msg := range req.History {
		messages = append(messages, &domainllm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// 添加当前用户消息
	if req.Content != "" {
		messages = append(messages, &domainllm.Message{
			Role:    domainllm.RoleUser,
			Content: req.Content,
		})
	}

	return messages
}

// toChatResponse 转换为 HTTP 响应
func toChatResponse(resp *domainllm.ChatResponse) ChatResponse {
	result := ChatResponse{
		Content:  resp.Content,
		Metadata: resp.Metadata,
	}

	if resp.Usage != nil {
		result.TokenCount = resp.Usage.TotalTokens
	}

	if len(resp.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCallResponse, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			result.ToolCalls[i] = ToolCallResponse{
				ID:   tc.ID,
				Type: tc.Type,
				Function: FunctionResponse{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return result
}
