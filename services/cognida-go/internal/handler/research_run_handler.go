package handler

import (
	r "cognida/internal/model/researchrun"
	"cognida/internal/service/researchrun"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"time"
)

type ResearchRunHandler struct{ service *researchrun.Service }

func NewResearchRunHandler(service *researchrun.Service) *ResearchRunHandler {
	return &ResearchRunHandler{service}
}
func (h *ResearchRunHandler) Register(api *gin.RouterGroup) {
	api.POST("/research/runs", h.Create)
	api.GET("/research/runs/:id", h.Get)
	api.POST("/research/runs/:id/cancel", h.Cancel)
	api.GET("/research/runs/:id/events", h.Events)
}
func (h *ResearchRunHandler) Create(c *gin.Context) {
	p := taskControlScope(c)
	if p.Validate() != nil {
		Unauthorized(c, "需要有效身份")
		return
	}
	var req r.Request
	if !BindJSON(c, &req) {
		return
	}
	row, e := h.service.Create(c.Request.Context(), p, req, c.GetHeader("Idempotency-Key"))
	if e != nil {
		taskControlError(c, e)
		return
	}
	Created(c, row)
}
func (h *ResearchRunHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	p := taskControlScope(c)
	if p.Validate() != nil {
		Unauthorized(c, "需要有效身份")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	row, e := h.service.Read(ctx, p, c.Param("id"))
	if e != nil {
		taskControlError(c, e)
		return
	}
	OK(c, row)
}
func (h *ResearchRunHandler) Cancel(c *gin.Context) {
	p := taskControlScope(c)
	if p.Validate() != nil {
		Unauthorized(c, "需要有效身份")
		return
	}
	row, e := h.service.Cancel(c.Request.Context(), p, c.Param("id"))
	if e != nil {
		taskControlError(c, e)
		return
	}
	row.Report = nil
	row.OutputRefs = json.RawMessage(`[]`)
	OK(c, row)
}

// Events only reads a durable run. Reconnection cannot start paid model work.
func (h *ResearchRunHandler) Events(c *gin.Context) {
	p := taskControlScope(c)
	if p.Validate() != nil {
		Unauthorized(c, "需要有效身份")
		return
	}
	row, e := h.service.Read(c.Request.Context(), p, c.Param("id"))
	if e != nil {
		taskControlError(c, e)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var version int64
	for {
		if row.Version != version {
			version = row.Version
			c.SSEvent("state", row)
			c.Writer.Flush()
		}
		if row.Terminal() {
			c.SSEvent("terminal", row)
			c.Writer.Flush()
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
		row, e = h.service.Read(c.Request.Context(), p, c.Param("id"))
		if e != nil {
			c.SSEvent("error", gin.H{"error": "研究来源权限不可用，请重新读取；未重启研究"})
			c.Writer.Flush()
			return
		}
	}
}
