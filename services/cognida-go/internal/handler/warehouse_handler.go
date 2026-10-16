package handler

import (
	agentctx "cognida/internal/model/agent"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/warehouse"
	"cognida/internal/service/warehouse"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

type WarehouseHandler struct{ s *warehouse.Service }

func NewWarehouseHandler(s *warehouse.Service) *WarehouseHandler { return &WarehouseHandler{s} }
func (h *WarehouseHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/warehouse")
	g.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request = c.Request.WithContext(agentctx.WithRequestID(c.Request.Context(), lifecycleRequestID(c)))
		c.Next()
	})
	g.POST("/preview", h.preview)
	g.POST("/models", h.createModel)
	g.GET("/models", h.models)
	g.GET("/models/:id", h.model)
	g.POST("/models/:id/versions", h.createModel)
	g.GET("/models/:id/requirements", h.requirements)
	g.POST("/runs", h.createRun)
	g.GET("/runs", h.runs)
	g.GET("/runs/:id", h.run)
	g.GET("/runs/:id/events", h.events)
	g.POST("/runs/:id/:action", h.control)
}
func warehouseErr(c *gin.Context, e error) {
	code := 500
	switch {
	case errors.Is(e, d.ErrInvalid), errors.Is(e, l.ErrInvalid), errors.Is(e, g.ErrInvalid):
		code = 400
	case errors.Is(e, d.ErrConflict), errors.Is(e, l.ErrConflict):
		code = 409
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		code = 403
	case errors.Is(e, d.ErrNotFound), errors.Is(e, l.ErrNotFound):
		code = 404
	}
	message := "数仓操作失败"
	if code < 500 {
		message = e.Error()
	}
	c.JSON(code, gin.H{"code": code, "message": message})
}
func (h *WarehouseHandler) preview(c *gin.Context) {
	var in struct {
		Definition d.Definition `json:"definition"`
		Limit      int          `json:"limit"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Preview(c.Request.Context(), gp(c), in.Definition, in.Limit)
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
func (h *WarehouseHandler) createModel(c *gin.Context) {
	var in struct {
		Definition      d.Definition `json:"definition"`
		ExpectedVersion int          `json:"expected_version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.SaveModel(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, in.Definition, c.GetHeader("Idempotency-Key"))
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
func (h *WarehouseHandler) models(c *gin.Context) {
	v, next, e := h.s.ListModels(c.Request.Context(), gp(c), c.Query("cursor"))
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, gin.H{"items": v, "next_cursor": next})
}
func (h *WarehouseHandler) model(c *gin.Context) {
	version, _ := strconv.Atoi(c.Query("version"))
	v, e := h.s.GetModel(c.Request.Context(), gp(c), c.Param("id"), version)
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
func (h *WarehouseHandler) requirements(c *gin.Context) {
	version, _ := strconv.Atoi(c.Query("version"))
	v, e := h.s.Requirements(c.Request.Context(), gp(c), c.Param("id"), version)
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
func (h *WarehouseHandler) createRun(c *gin.Context) {
	var in d.RunRequest
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.CreateRun(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"), nil)
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
func (h *WarehouseHandler) runs(c *gin.Context) {
	v, next, e := h.s.ListRuns(c.Request.Context(), gp(c), c.Query("model_id"), c.Query("cursor"))
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, gin.H{"items": v, "next_cursor": next})
}
func (h *WarehouseHandler) run(c *gin.Context) {
	v, e := h.s.GetRun(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
func (h *WarehouseHandler) events(c *gin.Context) {
	v, e := h.s.Events(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, gin.H{"items": v})
}
func (h *WarehouseHandler) control(c *gin.Context) {
	var in struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	var v *d.Run
	var e error
	switch c.Param("action") {
	case "cancel":
		v, e = h.s.Cancel(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	case "approve":
		v, e = h.s.Approve(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	default:
		e = d.ErrInvalid
	}
	if e != nil {
		warehouseErr(c, e)
		return
	}
	OK(c, v)
}
