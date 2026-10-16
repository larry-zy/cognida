package handler

import (
	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/governedops"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/governedops"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

type GovernedOperationHandler struct{ s *governedops.Service }

func NewGovernedOperationHandler(s *governedops.Service) *GovernedOperationHandler {
	return &GovernedOperationHandler{s}
}
func (h *GovernedOperationHandler) Register(api *gin.RouterGroup) {
	group := api.Group("/operations")
	group.Use(func(c *gin.Context) {
		if c.GetInt64("tenant_id") <= 0 || c.GetInt64("user_id") <= 0 {
			Unauthorized(c, "authenticated identity required")
			c.Abort()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		ctx = agentctx.WithUserID(agentctx.WithTenantID(ctx, c.GetInt64("tenant_id")), c.GetInt64("user_id"))
		c.Request = c.Request.WithContext(ctx)
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	group.GET("/targets", h.targets)
	group.GET("/targets/:name/versions/:version", h.version)
	group.POST("/plans", h.prepare)
	group.GET("/jobs", h.list)
	group.GET("/jobs/:id", h.get)
	group.POST("/jobs/:id/confirm", h.confirm)
	group.POST("/jobs/:id/cancel", h.cancel)
}
func operationPrincipal(c *gin.Context) l.Principal {
	return l.Principal{TenantID: c.GetInt64("tenant_id"), UserID: c.GetInt64("user_id")}
}
func operationReply(c *gin.Context, value any, e error) {
	if e == nil {
		OK(c, value)
		return
	}
	code := 500
	message := "受控操作失败，请核对持久状态。"
	switch {
	case errors.Is(e, d.ErrInvalid), errors.Is(e, l.ErrInvalid):
		code = 400
		message = "参数或表结构不受支持；只允许有界固定输入与字面值操作。"
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		code = 403
		message = "操作或来源当前不可访问。"
	case errors.Is(e, d.ErrNotFound), errors.Is(e, l.ErrNotFound):
		code = 404
		message = "受管目标或操作不存在。"
	case errors.Is(e, d.ErrConflict), errors.Is(e, l.ErrConflict):
		code = 409
		message = "计划内容、确认或目标版本已变化，请重新核对。"
	case errors.Is(e, d.ErrUncertain):
		code = 503
		message = "操作已保存，结果待核对。请读取原任务，不要更换幂等键重做。"
	case errors.Is(e, context.DeadlineExceeded):
		code = 504
		message = "读取超时，请核对原操作状态。"
	}
	c.JSON(code, gin.H{"code": code, "message": message})
}
func (h *GovernedOperationHandler) prepare(c *gin.Context) {
	var in d.Request
	if !readTaskJSON(c, &in) {
		return
	}
	if in.SourceQueryHash != "" {
		operationReply(c, nil, d.ErrInvalid)
		return
	}
	v, e := h.s.Prepare(c.Request.Context(), operationPrincipal(c), in, c.GetHeader("Idempotency-Key"))
	operationReply(c, v, e)
}
func (h *GovernedOperationHandler) get(c *gin.Context) {
	v, e := h.s.Read(c.Request.Context(), operationPrincipal(c), c.Param("id"))
	operationReply(c, v, e)
}
func (h *GovernedOperationHandler) list(c *gin.Context) {
	v, next, e := h.s.List(c.Request.Context(), operationPrincipal(c), c.Query("cursor"))
	operationReply(c, gin.H{"items": v, "next_cursor": next}, e)
}
func (h *GovernedOperationHandler) targets(c *gin.Context) {
	v, e := h.s.Targets(c.Request.Context(), operationPrincipal(c))
	operationReply(c, gin.H{"items": v}, e)
}
func (h *GovernedOperationHandler) version(c *gin.Context) {
	version, e := strconv.ParseInt(c.Param("version"), 10, 64)
	if e != nil || version < 0 {
		operationReply(c, nil, d.ErrInvalid)
		return
	}
	v, e := h.s.Version(c.Request.Context(), operationPrincipal(c), c.Param("name"), version)
	operationReply(c, v, e)
}
func (h *GovernedOperationHandler) confirm(c *gin.Context) {
	var in d.Confirmation
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Confirm(c.Request.Context(), operationPrincipal(c), c.Param("id"), in)
	operationReply(c, v, e)
}
func (h *GovernedOperationHandler) cancel(c *gin.Context) {
	var in struct{}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Cancel(c.Request.Context(), operationPrincipal(c), c.Param("id"))
	operationReply(c, v, e)
}
