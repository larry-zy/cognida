package handler

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	"cognida/internal/service/sandbox"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

type SandboxHandler struct {
	runner  *sandbox.DockerRunner
	profile *d.Profile
}

func NewSandboxHandler(r *sandbox.DockerRunner, p *d.Profile) *SandboxHandler {
	return &SandboxHandler{r, p}
}
func (h *SandboxHandler) Register(api *gin.RouterGroup) {
	api.GET("/sandbox/profiles", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if h.profile == nil {
			OK(c, gin.H{"enabled": false, "reason": "受控开发执行环境尚未配置并验收"})
			return
		}
		OK(c, gin.H{"enabled": true, "profile": h.profile})
	})
	group := api.Group("/sandbox")
	group.Use(func(c *gin.Context) {
		if h.runner == nil {
			c.AbortWithStatusJSON(503, gin.H{"code": 503, "message": "Sandbox runner 未启用"})
			return
		}
		c.Next()
	})
	group.GET("/inputs/:id", func(c *gin.Context) {
		version, e := strconv.Atoi(c.Query("version"))
		if e != nil {
			BadRequest(c, "invalid version")
			return
		}
		v, e := h.runner.Metadata(c.Request.Context(), gp(c), l.RefID{AssetID: c.Param("id"), Version: version})
		if e != nil {
			sandboxErr(c, e)
			return
		}
		OK(c, v)
	})
	group.POST("/runs", func(c *gin.Context) {
		var in d.SubmitRequest
		if !readTaskJSON(c, &in) {
			return
		}
		in.IdempotencyKey = c.GetHeader("Idempotency-Key")
		v, e := h.runner.Submit(c.Request.Context(), gp(c), in)
		if e != nil {
			sandboxErr(c, e)
			return
		}
		OK(c, v)
	})
	group.GET("/runs", func(c *gin.Context) {
		rows, next, e := h.runner.List(c.Request.Context(), gp(c), c.Query("cursor"))
		if e != nil {
			sandboxErr(c, e)
			return
		}
		OK(c, gin.H{"items": rows, "next_cursor": next})
	})
	group.GET("/runs/:id", func(c *gin.Context) {
		v, e := h.runner.Get(c.Request.Context(), gp(c), c.Param("id"))
		if e != nil {
			sandboxErr(c, e)
			return
		}
		c.Header("Cache-Control", "no-store")
		OK(c, v)
	})
	group.POST("/runs/:id/cancel", func(c *gin.Context) {
		var in d.ExecutionRef
		if !readTaskJSON(c, &in) {
			return
		}
		if in.ID != c.Param("id") {
			BadRequest(c, "execution ID mismatch")
			return
		}
		v, e := h.runner.Cancel(c.Request.Context(), gp(c), in)
		if e != nil {
			sandboxErr(c, e)
			return
		}
		OK(c, v)
	})
}
func sandboxErr(c *gin.Context, e error) {
	switch {
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrNotFound):
		Forbidden(c, "无权访问Sandbox任务或来源")
	case errors.Is(e, d.ErrInvalid), errors.Is(e, d.ErrUnsupported):
		BadRequest(c, e.Error())
	case errors.Is(e, d.ErrConflict):
		c.JSON(409, gin.H{"code": 409, "message": "任务版本或产物内容已变化"})
	default:
		c.JSON(503, gin.H{"code": 503, "message": "Sandbox运行环境不可用"})
	}
}
