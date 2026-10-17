// Package handler 提供外部数据源管理的 HTTP 处理器。
// 安全约定：任何响应不回显密码（明文或密文）；测试连接/查询错误已在 service 层脱敏。
package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	domain_datasource "cognida/internal/model/datasource"
	app_datasource "cognida/internal/service/datasource"
)

// DataSourceHandler 数据源处理器
type DataSourceHandler struct {
	service    *app_datasource.Service
	management DataSourceManagementAuthorizer
}

// DataSourceManagementAuthorizer checks current tenant owner/admin membership.
// Read-only consumers do not receive permission to change or reuse credentials.
type DataSourceManagementAuthorizer interface {
	AuthorizeTenantManagement(context.Context, int64, int64) error
}

// NewDataSourceHandler 创建数据源处理器
func NewDataSourceHandler(service *app_datasource.Service, management ...DataSourceManagementAuthorizer) *DataSourceHandler {
	h := &DataSourceHandler{service: service}
	if len(management) == 1 {
		h.management = management[0]
	}
	return h
}

func (h *DataSourceHandler) requireManagement(c *gin.Context) bool {
	user, userOK := c.Get("user_id")
	tenant, tenantOK := c.Get("tenant_id")
	uid, validUser := user.(int64)
	tid, validTenant := tenant.(int64)
	if !userOK || !tenantOK || !validUser || !validTenant || uid <= 0 || tid <= 0 || h.management == nil || h.management.AuthorizeTenantManagement(c.Request.Context(), uid, tid) != nil {
		c.AbortWithStatusJSON(403, gin.H{"error": "无权管理该租户的数据源"})
		return false
	}
	return true
}

// List 数据源列表
// @Router /api/v1/datasources [get]
func (h *DataSourceHandler) List(c *gin.Context) {
	page, pageSize := GetPageParams(c)
	typ := strings.TrimSpace(c.Query("type"))
	if typ != "" {
		if _, err := app_datasource.DriverFor(domain_datasource.Type(typ)); err != nil {
			BadRequest(c, err.Error())
			return
		}
	}
	items, total, err := h.service.List(c.Request.Context(), domain_datasource.ListFilter{
		TenantID:     GetTenantID(c),
		Name:         strings.TrimSpace(c.Query("name")),
		Type:         typ,
		DatabaseName: strings.TrimSpace(c.Query("database")),
		Limit:        pageSize,
		Offset:       (page - 1) * pageSize,
	})
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	PageSuccessJSON(c, total, items, page, pageSize)
}

// Get 数据源详情
// @Router /api/v1/datasources/:id [get]
func (h *DataSourceHandler) Get(c *gin.Context) {
	item, err := h.service.Get(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if err != nil {
		if errors.Is(err, app_datasource.ErrNotFound) {
			NotFound(c, "数据源不存在")
			return
		}
		InternalError(c, err.Error())
		return
	}
	OK(c, item)
}

// Create 注册数据源
// @Router /api/v1/datasources [post]
func (h *DataSourceHandler) Create(c *gin.Context) {
	if !h.requireManagement(c) {
		return
	}
	var req app_datasource.SaveInput
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), GetTenantID(c), GetUserID(c), &req)
	if err != nil {
		if errors.Is(err, app_datasource.ErrNameConflict) {
			BadRequest(c, "数据源名称已存在")
			return
		}
		BadRequest(c, err.Error())
		return
	}
	OK(c, item)
}

// Update 更新数据源（密码留空表示不修改）
// @Router /api/v1/datasources/:id [put]
func (h *DataSourceHandler) Update(c *gin.Context) {
	if !h.requireManagement(c) {
		return
	}
	var req app_datasource.SaveInput
	if !BindJSON(c, &req) {
		return
	}
	item, err := h.service.Update(c.Request.Context(), GetTenantID(c), c.Param("id"), &req)
	if err != nil {
		switch {
		case errors.Is(err, app_datasource.ErrNotFound):
			NotFound(c, "数据源不存在")
		case errors.Is(err, app_datasource.ErrNameConflict):
			BadRequest(c, "数据源名称已存在")
		default:
			BadRequest(c, err.Error())
		}
		return
	}
	OK(c, item)
}

// Delete 删除数据源（联动关闭连接池）
// @Router /api/v1/datasources/:id [delete]
func (h *DataSourceHandler) Delete(c *gin.Context) {
	if !h.requireManagement(c) {
		return
	}
	if err := h.service.Delete(c.Request.Context(), GetTenantID(c), c.Param("id")); err != nil {
		if errors.Is(err, app_datasource.ErrNotFound) {
			NotFound(c, "数据源不存在")
			return
		}
		InternalError(c, err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}

// TestConnection 测试连接（表单配置或已存 id；临时连接不进缓存）
// @Router /api/v1/datasources/test [post]
func (h *DataSourceHandler) TestConnection(c *gin.Context) {
	if !h.requireManagement(c) {
		return
	}
	var req app_datasource.TestInput
	if !BindJSON(c, &req) {
		return
	}
	if err := h.service.TestConnection(c.Request.Context(), GetTenantID(c), &req); err != nil {
		if errors.Is(err, app_datasource.ErrCredentialTargetChanged) {
			BadRequest(c, err.Error())
			return
		}
		// 测试失败是预期业务结果而非服务器错误：200 + ok=false 便于前端展示原因
		OK(c, gin.H{"ok": false, "message": err.Error()})
		return
	}
	OK(c, gin.H{"ok": true})
}

// ListTables 外部数据源表清单
// @Router /api/v1/datasources/:id/tables [get]
func (h *DataSourceHandler) ListTables(c *gin.Context) {
	tables, err := h.service.ListTables(c.Request.Context(), GetTenantID(c), c.Param("id"))
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	OK(c, gin.H{"tables": tables})
}

// DescribeTable 外部数据源单表结构
// @Router /api/v1/datasources/:id/tables/:table [get]
func (h *DataSourceHandler) DescribeTable(c *gin.Context) {
	schema, err := h.service.DescribeTable(
		c.Request.Context(), GetTenantID(c), c.Param("id"), c.Param("table"))
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	OK(c, schema)
}
