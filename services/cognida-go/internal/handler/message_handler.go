// Package handler 提供消息管理的HTTP处理器
package handler

import (
	agentctx "cognida/internal/model/agent"
	"context"
	"errors"
	"github.com/gin-gonic/gin"

	app_chat "cognida/internal/service/chat"
)

// ========================================
// MessageHandler 消息处理器
// ========================================

// MessageHandler 消息处理器
type MessageHandler struct {
	messageService *app_chat.MessageService
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(messageService *app_chat.MessageService) *MessageHandler {
	return &MessageHandler{
		messageService: messageService,
	}
}

// CreateMessage 创建消息
func (h *MessageHandler) CreateMessage(c *gin.Context) {
	var req app_chat.CreateMessageRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.messageService.CreateMessage(messageIdentityContext(c), &req)
	if err != nil {
		messageError(c, err)
		return
	}

	Created(c, result)
}

// GetMessage 获取消息
func (h *MessageHandler) GetMessage(c *gin.Context) {
	id := c.Param("id")
	if !RequireParam(c, id, "消息ID") {
		return
	}

	result, err := h.messageService.GetMessageByID(messageIdentityContext(c), id)
	if err != nil {
		messageError(c, err)
		return
	}

	OK(c, result)
}

// ListMessages 列出消息
func (h *MessageHandler) ListMessages(c *gin.Context) {
	sessionID := c.Query("session_id")
	if !RequireParam(c, sessionID, "会话ID") {
		return
	}

	page, size := GetPageParams(c)
	role := c.Query("role")

	req := &app_chat.ListMessagesRequest{
		SessionID: sessionID,
		Page:      page,
		Size:      size,
		Role:      role,
	}

	result, err := h.messageService.ListMessages(messageIdentityContext(c), req)
	if err != nil {
		messageError(c, err)
		return
	}

	OK(c, result)
}

// UpdateMessage 更新消息
func (h *MessageHandler) UpdateMessage(c *gin.Context) {
	id := c.Param("id")
	if !RequireParam(c, id, "消息ID") {
		return
	}

	var req app_chat.UpdateMessageRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.messageService.UpdateMessage(messageIdentityContext(c), id, &req)
	if err != nil {
		messageError(c, err)
		return
	}

	OK(c, result)
}

// DeleteMessage 删除消息
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	id := c.Param("id")
	if !RequireParam(c, id, "消息ID") {
		return
	}

	if err := h.messageService.DeleteMessage(messageIdentityContext(c), id); err != nil {
		messageError(c, err)
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

func messageError(c *gin.Context, err error) {
	if errors.Is(err, app_chat.ErrMessageForbidden) {
		Forbidden(c, "无权访问该会话消息")
		return
	}
	InternalError(c, "消息操作失败")
}

// Message endpoints require authenticated scope, without development defaults.
func messageIdentityContext(c *gin.Context) context.Context {
	return agentctx.WithTenantID(agentctx.WithUserID(c.Request.Context(), c.GetInt64("user_id")), c.GetInt64("tenant_id"))
}
