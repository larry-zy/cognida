// Package handler 提供指标语义层「建模」的 HTTP 处理器。
//
// 建模写入口：让受治理的语义模型可经 REST 增删改与发布，补齐此前缺失的建模进料线
// （UpsertBundle 曾仅测试可达），使 semantic_query 走治理主路径而非恒回退词法 NL2SQL。
package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	domain_semantic "link/internal/model/semantic"
	app_semantic "link/internal/service/semantic"
)

// SemanticHandler 语义模型建模处理器。
type SemanticHandler struct {
	service *app_semantic.Service
}

// NewSemanticHandler 创建语义模型处理器。
func NewSemanticHandler(service *app_semantic.Service) *SemanticHandler {
	return &SemanticHandler{service: service}
}

// List 生效语义模型列表（仅模型头）。
// @Router /api/v1/semantic-models [get]
func (h *SemanticHandler) List(c *gin.Context) {
	items, err := h.service.List(c.Request.Context(), GetTenantID(c))
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	OK(c, gin.H{"items": items, "total": len(items)})
}

// Get 语义模型详情（完整 bundle，任意状态）。
// @Router /api/v1/semantic-models/:id [get]
func (h *SemanticHandler) Get(c *gin.Context) {
	item, err := h.service.Get(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if err != nil {
		if errors.Is(err, domain_semantic.ErrModelNotFound) {
			NotFound(c, "语义模型不存在")
			return
		}
		InternalError(c, err.Error())
		return
	}
	OK(c, item)
}

// Create 新建语义模型。
// @Router /api/v1/semantic-models [post]
func (h *SemanticHandler) Create(c *gin.Context) {
	var req app_semantic.SaveInput
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), GetTenantID(c), &req)
	if err != nil {
		h.writeErr(c, err)
		return
	}
	OK(c, item)
}

// Update 全量更新语义模型（bump 版本，失效受信查询缓存）。
// @Router /api/v1/semantic-models/:id [put]
func (h *SemanticHandler) Update(c *gin.Context) {
	var req app_semantic.SaveInput
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.service.Update(c.Request.Context(), GetTenantID(c), c.Param("id"), &req)
	if err != nil {
		h.writeErr(c, err)
		return
	}
	OK(c, item)
}

// Publish 发布模型（置为生效，进入查询主路径）。
// @Router /api/v1/semantic-models/:id/publish [post]
func (h *SemanticHandler) Publish(c *gin.Context) {
	item, err := h.service.Publish(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	OK(c, item)
}

// Deprecate 弃用模型（退出查询主路径）。
// @Router /api/v1/semantic-models/:id/deprecate [post]
func (h *SemanticHandler) Deprecate(c *gin.Context) {
	item, err := h.service.Deprecate(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if err != nil {
		h.writeErr(c, err)
		return
	}
	OK(c, item)
}

// Coverage 语义查询治理覆盖率（covered/cache_hit/fallback 按模型聚合）。
// @Router /api/v1/semantic-coverage [get]
func (h *SemanticHandler) Coverage(c *gin.Context) {
	stats, err := h.service.Coverage(c.Request.Context(), GetTenantID(c))
	if err != nil {
		if errors.Is(err, app_semantic.ErrCoverageDisabled) {
			OK(c, gin.H{"items": []any{}, "enabled": false})
			return
		}
		InternalError(c, err.Error())
		return
	}
	OK(c, gin.H{"items": stats, "total": len(stats), "enabled": true})
}

// writeErr 统一错误映射：not found→404，校验/命名冲突→400，其余→500。
func (h *SemanticHandler) writeErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain_semantic.ErrModelNotFound):
		NotFound(c, "语义模型不存在")
	case errors.Is(err, app_semantic.ErrValidation):
		BadRequest(c, err.Error())
	case errors.Is(err, app_semantic.ErrNameConflict):
		BadRequest(c, err.Error())
	default:
		InternalError(c, err.Error())
	}
}
