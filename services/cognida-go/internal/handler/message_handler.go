// Package handler 提供消息管理的HTTP处理器
package handler

import (
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

	result, err := h.messageService.CreateMessage(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
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

	result, err := h.messageService.GetMessageByID(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
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

	result, err := h.messageService.ListMessages(c.Request.Context(), req)
	if err != nil {
		InternalError(c, err.Error())
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

	result, err := h.messageService.UpdateMessage(c.Request.Context(), id, &req)
	if err != nil {
		InternalError(c, err.Error())
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

	if err := h.messageService.DeleteMessage(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}
