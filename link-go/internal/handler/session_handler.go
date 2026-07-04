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

// identityContext 把认证身份（user_id/tenant_id）注入请求上下文，
// 供 service 层做会话归属校验（防越权 IDOR）。所有按 ID 操作单个会话的处理器统一使用。
func identityContext(c *gin.Context) context.Context {
	ctx := context.WithValue(c.Request.Context(), "user_id", GetUserID(c))
	return context.WithValue(ctx, "tenant_id", GetTenantID(c))
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

	result, err := h.sessionService.GetSessionByID(identityContext(c), id)
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

	result, err := h.sessionService.GetSessionDetail(identityContext(c), id)
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

	result, err := h.sessionService.ListSessions(identityContext(c), req)
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

	result, err := h.sessionService.UpdateSession(identityContext(c), id, &req)
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

	if err := h.sessionService.DeleteSession(identityContext(c), id); err != nil {
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

	if err := h.sessionService.ArchiveSession(identityContext(c), id); err != nil {
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

	if err := h.sessionService.ActivateSession(identityContext(c), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "激活成功"})
}
