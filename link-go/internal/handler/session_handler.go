// Package handler 提供会话管理的HTTP处理器
package handler

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	app_chat "link/internal/service/chat"
)

// ========================================
// SessionHandler 会话处理器
// ========================================

// SessionHandler 会话处理器
type SessionHandler struct {
	sessionService *app_chat.SessionService
	chatService    *app_chat.ChatService
}

// NewSessionHandler 创建会话处理器
func NewSessionHandler(
	sessionService *app_chat.SessionService,
	chatService *app_chat.ChatService,
) *SessionHandler {
	return &SessionHandler{
		sessionService: sessionService,
		chatService:    chatService,
	}
}

// CreateSession 创建会话
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req app_chat.CreateSessionRequest
	if !BindJSON(c, &req) {
		return
	}

	userID := GetUserID(c)

	result, err := h.sessionService.CreateSession(c.Request.Context(), userID, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Created(c, result)
}

// GetSession 获取会话详情
func (h *SessionHandler) GetSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	result, err := h.sessionService.GetSessionByID(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, result)
}

// GetSessionDetail 获取会话详情（包含消息）
func (h *SessionHandler) GetSessionDetail(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	result, err := h.sessionService.GetSessionDetail(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, result)
}

// ListSessions 列出会话
func (h *SessionHandler) ListSessions(c *gin.Context) {
	page, size := GetPageParams(c)

	var status *int8
	if s := c.Query("status"); s != "" {
		if val, err := strconv.ParseInt(s, 10, 8); err == nil {
			statusVal := int8(val)
			status = &statusVal
		}
	}

	req := &app_chat.ListSessionsRequest{
		Page:   page,
		Size:   size,
		Status: status,
	}

	// 将 user_id 添加到 context（与 SessionService 使用的 key 一致）
	userID := GetUserID(c)
	ctx := context.WithValue(c.Request.Context(), "user_id", userID)

	result, err := h.sessionService.ListSessions(ctx, req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// UpdateSession 更新会话
func (h *SessionHandler) UpdateSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	var req app_chat.UpdateSessionRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.sessionService.UpdateSession(c.Request.Context(), id, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// DeleteSession 删除会话
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	if err := h.sessionService.DeleteSession(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

// ArchiveSession 归档会话
func (h *SessionHandler) ArchiveSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	if err := h.sessionService.ArchiveSession(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "归档成功"})
}

// ActivateSession 激活会话
func (h *SessionHandler) ActivateSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		BadRequest(c, "会话ID不能为空")
		return
	}

	if err := h.sessionService.ActivateSession(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "激活成功"})
}
