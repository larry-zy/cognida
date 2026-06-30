// Package handler 提供租户管理的HTTP处理器
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	appAccount "link/internal/service/account"
)

// ========================================
// TenantHandler 租户处理器
// ========================================

// TenantHandler 租户处理器
type TenantHandler struct {
	accountService *appAccount.AccountService
}

// NewTenantHandler 创建租户处理器
func NewTenantHandler(accountService *appAccount.AccountService) *TenantHandler {
	return &TenantHandler{
		accountService: accountService,
	}
}

// CreateTenant 创建租户
func (h *TenantHandler) CreateTenant(c *gin.Context) {
	var req appAccount.CreateTenantRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.accountService.CreateTenant(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Created(c, result)
}

// ListTenants 列出租户
func (h *TenantHandler) ListTenants(c *gin.Context) {
	page, pageSize := GetPageParams(c)

	result, total, err := h.accountService.ListTenants(c.Request.Context(), page, pageSize)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	PageSuccessJSON(c, total, result, page, pageSize)
}

// GetTenant 获取租户详情
func (h *TenantHandler) GetTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的租户ID")
		return
	}

	result, err := h.accountService.GetTenantByID(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	OK(c, result)
}

// UpdateTenant 更新租户
func (h *TenantHandler) UpdateTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的租户ID")
		return
	}

	var req appAccount.UpdateTenantRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.accountService.UpdateTenant(c.Request.Context(), id, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// DeleteTenant 删除租户
func (h *TenantHandler) DeleteTenant(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的租户ID")
		return
	}

	if err := h.accountService.DeleteTenant(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "删除成功"})
}

// GetTenantStats 获取租户统计
func (h *TenantHandler) GetTenantStats(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "无效的租户ID")
		return
	}

	resp, err := h.accountService.GetStorageUsage(c.Request.Context(), id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}
