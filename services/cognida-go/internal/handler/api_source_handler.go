package handler

import (
	d "cognida/internal/model/apisource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/apisource"
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

type APISourceHandler struct{ service *apisource.Service }

func NewAPISourceHandler(service *apisource.Service) *APISourceHandler {
	return &APISourceHandler{service}
}
func (h *APISourceHandler) Register(api *gin.RouterGroup) {
	scope := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if apiPrincipal(c).Validate() != nil {
			Unauthorized(c, "需要有效的用户和租户身份")
			c.Abort()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
	credentials := api.Group("/api-credentials", scope)
	credentials.GET("", h.ListCredentials)
	credentials.POST("", h.CreateCredential)
	credentials.POST("/:id/disable", h.DisableCredential)
	sources := api.Group("/api-sources", scope)
	sources.GET("/capabilities", func(c *gin.Context) { OK(c, apisource.Capabilities()) })
	sources.GET("", h.ListSources)
	sources.POST("", h.PutSource)
	sources.GET("/:id", h.GetSource)
	sources.PUT("/:id", h.PutSource)
	sources.POST("/:id/enabled", h.SetEnabled)
	sources.POST("/:id/runs", h.Start)
	runs := api.Group("/api-ingestion-runs", scope)
	runs.GET("/:id", h.GetRun)
	runs.POST("/:id/cancel", h.Cancel)
	runs.POST("/:id/resume", h.Resume)
}
func apiPrincipal(c *gin.Context) l.Principal {
	return l.Principal{TenantID: c.GetInt64("tenant_id"), UserID: c.GetInt64("user_id")}
}
func apiSourceError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, d.ErrInvalid):
		BadRequest(c, "API 来源参数或读取范围不合法")
	case errors.Is(e, d.ErrForbidden):
		Forbidden(c, "API 来源或凭据当前不可访问")
	case errors.Is(e, d.ErrNotFound):
		NotFound(c, "API 来源或采集任务不存在")
	case errors.Is(e, d.ErrConflict):
		c.JSON(409, gin.H{"error": "API 来源版本、幂等键或任务状态冲突"})
	case errors.Is(e, context.DeadlineExceeded):
		c.JSON(504, gin.H{"error": "读取超时，请刷新任务状态；未重新提交"})
	default:
		c.JSON(503, gin.H{"error": "API 来源服务暂不可用；提交结果请通过原幂等键核对"})
	}
}
func apiLimit(c *gin.Context) int {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		return 20
	}
	return limit
}
func (h *APISourceHandler) ListCredentials(c *gin.Context) {
	limit := apiLimit(c)
	rows, e := h.service.ListCredentials(c.Request.Context(), apiPrincipal(c), c.Query("cursor"), limit)
	if e != nil {
		apiSourceError(c, e)
		return
	}
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID
	}
	OK(c, gin.H{"items": rows, "next_cursor": next})
}
func (h *APISourceHandler) CreateCredential(c *gin.Context) {
	var in d.CredentialInput
	if !readTaskJSON(c, &in) {
		return
	}
	row, e := h.service.CreateCredential(c.Request.Context(), apiPrincipal(c), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		apiSourceError(c, e)
		return
	}
	Created(c, row)
}
func (h *APISourceHandler) DisableCredential(c *gin.Context) {
	if e := h.service.DisableCredential(c.Request.Context(), apiPrincipal(c), c.Param("id")); e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, gin.H{"disabled": true})
}
func (h *APISourceHandler) ListSources(c *gin.Context) {
	limit := apiLimit(c)
	rows, e := h.service.ListSources(c.Request.Context(), apiPrincipal(c), c.Query("cursor"), limit)
	if e != nil {
		apiSourceError(c, e)
		return
	}
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID
	}
	OK(c, gin.H{"items": rows, "next_cursor": next})
}
func (h *APISourceHandler) GetSource(c *gin.Context) {
	version, _ := strconv.Atoi(c.Query("version"))
	row, e := h.service.Source(c.Request.Context(), apiPrincipal(c), c.Param("id"), version)
	if e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, row)
}
func (h *APISourceHandler) PutSource(c *gin.Context) {
	var in d.SourceInput
	if !readTaskJSON(c, &in) {
		return
	}
	row, e := h.service.PutSource(c.Request.Context(), apiPrincipal(c), c.Param("id"), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, row)
}
func (h *APISourceHandler) SetEnabled(c *gin.Context) {
	var in struct {
		ExpectedVersion int  `json:"expected_version"`
		Enabled         bool `json:"enabled"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	row, e := h.service.SetEnabled(c.Request.Context(), apiPrincipal(c), c.Param("id"), in.ExpectedVersion, in.Enabled)
	if e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, row)
}
func (h *APISourceHandler) Start(c *gin.Context) {
	var in struct {
		Version int `json:"version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	row, e := h.service.Start(c.Request.Context(), apiPrincipal(c), c.Param("id"), in.Version, c.GetHeader("Idempotency-Key"))
	if e != nil {
		apiSourceError(c, e)
		return
	}
	Created(c, row)
}
func (h *APISourceHandler) GetRun(c *gin.Context) {
	row, e := h.service.Read(c.Request.Context(), apiPrincipal(c), c.Param("id"))
	if e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, row)
}
func (h *APISourceHandler) Cancel(c *gin.Context) {
	row, e := h.service.Cancel(c.Request.Context(), apiPrincipal(c), c.Param("id"))
	if e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, row)
}
func (h *APISourceHandler) Resume(c *gin.Context) {
	row, e := h.service.Resume(c.Request.Context(), apiPrincipal(c), c.Param("id"))
	if e != nil {
		apiSourceError(c, e)
		return
	}
	OK(c, row)
}
