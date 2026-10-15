package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	d "cognida/internal/model/memory"
	"github.com/gin-gonic/gin"
)

type MemoryHandler struct{ service d.LongTermMemoryService }

func NewMemoryHandler(s d.LongTermMemoryService) *MemoryHandler { return &MemoryHandler{service: s} }

// Register expects the existing authenticated, tenant-authorized API group.
// The extra identity check intentionally avoids the legacy helpers' default user/tenant.
func (h *MemoryHandler) Register(api *gin.RouterGroup) {
	g := api.Group("/memories")
	g.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if err := memoryScope(c).Validate(); err != nil {
			Unauthorized(c, "需要有效的用户和租户身份")
			c.Abort()
			return
		}
		c.Next()
	})
	g.GET("", h.List)
	g.POST("", h.Create)
	g.POST("/recall", h.Recall)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}
func memoryScope(c *gin.Context) d.Scope {
	return d.Scope{TenantID: c.GetInt64("tenant_id"), UserID: c.GetInt64("user_id")}
}
func memoryJSON(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		BadRequest(c, "记忆请求格式不正确或超过大小限制")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		BadRequest(c, "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}
func memoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, d.ErrUnauthorized):
		Unauthorized(c, "需要有效的用户和租户身份")
	case errors.Is(err, d.ErrInvalid):
		BadRequest(c, "请检查记忆内容、类别、版本和请求参数")
	case errors.Is(err, d.ErrNotFound):
		NotFound(c, "记忆不存在或已删除")
	case errors.Is(err, d.ErrConflict):
		Conflict(c, "记忆已被修改，或幂等键已用于其他请求；请刷新后重试", nil)
	default:
		InternalError(c, "记忆操作暂时失败，请重试")
	}
}
func (h *MemoryHandler) List(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil {
		memoryError(c, d.ErrInvalid)
		return
	}
	page, err := h.service.List(c.Request.Context(), memoryScope(c), d.ListFilter{Category: c.Query("category"), Query: c.Query("q"), Cursor: c.Query("cursor"), Limit: limit})
	if err != nil {
		memoryError(c, err)
		return
	}
	OK(c, page)
}
func (h *MemoryHandler) Create(c *gin.Context) {
	var input d.WriteInput
	if !memoryJSON(c, &input) {
		return
	}
	m, err := h.service.Create(c.Request.Context(), memoryScope(c), input, c.GetHeader("Idempotency-Key"), "user")
	if err != nil {
		memoryError(c, err)
		return
	}
	Created(c, m)
}
func (h *MemoryHandler) Get(c *gin.Context) {
	m, err := h.service.Get(c.Request.Context(), memoryScope(c), c.Param("id"))
	if err != nil {
		memoryError(c, err)
		return
	}
	OK(c, m)
}
func (h *MemoryHandler) Update(c *gin.Context) {
	var input d.UpdateInput
	if !memoryJSON(c, &input) {
		return
	}
	m, err := h.service.Update(c.Request.Context(), memoryScope(c), c.Param("id"), input)
	if err != nil {
		memoryError(c, err)
		return
	}
	OK(c, m)
}
func (h *MemoryHandler) Delete(c *gin.Context) {
	version, err := strconv.ParseInt(c.Query("expected_version"), 10, 64)
	if err != nil {
		memoryError(c, d.ErrInvalid)
		return
	}
	if err := h.service.Delete(c.Request.Context(), memoryScope(c), c.Param("id"), version); err != nil {
		memoryError(c, err)
		return
	}
	OK(c, gin.H{"deleted": true})
}
func (h *MemoryHandler) Recall(c *gin.Context) {
	var input d.RecallInput
	if !memoryJSON(c, &input) {
		return
	}
	result, err := h.service.Recall(c.Request.Context(), memoryScope(c), input)
	if err != nil {
		memoryError(c, err)
		return
	}
	OK(c, result)
}
