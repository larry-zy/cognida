// Package handler 提供 HTTP 处理器
package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cognida/internal/model/audit"
)

// AuditHandler 请求审计查询处理器。
//
// 仅提供只读查询/统计接口——写入由 AuditMiddleware + 异步 Writer 负责，
// 不经此 handler，保证写路径与查询路径解耦。
type AuditHandler struct {
	repo audit.Repository
}

// NewAuditHandler 创建审计查询处理器
func NewAuditHandler(repo audit.Repository) *AuditHandler {
	return &AuditHandler{repo: repo}
}

// ListAuditLogs 分页查询审计日志。
// 支持按 request_id / module / action / status / user_id / 时间范围 / 关键词过滤。
func (h *AuditHandler) ListAuditLogs(c *gin.Context) {
	page, pageSize := GetPageParams(c)

	q := &audit.Query{
		Module:       c.Query("module"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		Status:       c.Query("status"),
		Page:         page,
		PageSize:     pageSize,
	}

	// 租户隔离：非空时按登录租户过滤（dev 模式默认租户 1）。
	if tid := GetTenantID(c); tid > 0 {
		q.TenantID = &tid
	}

	if v := c.Query("request_id"); v != "" {
		q.RequestID = &v
	}
	if v := c.Query("keywords"); v != "" {
		q.Keywords = &v
	}
	if v := c.Query("user_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			q.UserID = &id
		}
	}
	if t, ok := parseTimeQuery(c.Query("start_time")); ok {
		q.StartTime = &t
	}
	if t, ok := parseTimeQuery(c.Query("end_time")); ok {
		q.EndTime = &t
	}

	_ = q.Validate()

	// 游标（keyset）分页〔M5〕：携带 cursor 参数即走 keyset，返回 next_cursor（空=末页）；
	// 未携带则维持既有 offset 分页（返回 total/page），保持前端契约向后兼容。
	if _, hasCursor := c.GetQuery("cursor"); hasCursor {
		q.Cursor = c.Query("cursor")
		logs, nextCursor, err := h.repo.QueryByCursor(c.Request.Context(), q)
		if err != nil {
			RespondError(c, err)
			return
		}
		OK(c, gin.H{
			"list":        logs,
			"next_cursor": nextCursor,
			"has_more":    nextCursor != "",
			"page_size":   q.PageSize,
		})
		return
	}

	logs, total, err := h.repo.Query(c.Request.Context(), q)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{
		"list":      logs,
		"total":     total,
		"page":      q.Page,
		"page_size": q.PageSize,
	})
}

// GetAuditStats 获取审计统计（总量、按模块、按日）。
func (h *AuditHandler) GetAuditStats(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			days = d
		}
	}

	var tenantID *int64
	if tid := GetTenantID(c); tid > 0 {
		tenantID = &tid
	}

	stats, err := h.repo.GetStats(c.Request.Context(), tenantID, days)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	OK(c, stats)
}

// GetAuditLog 按 ID 查询单条审计详情。
func (h *AuditHandler) GetAuditLog(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的日志 ID")
		return
	}
	entry, err := h.repo.FindByID(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
		return
	}
	OK(c, entry)
}

// parseTimeQuery 解析时间查询参数，支持 RFC3339 与常见日期时间格式。
func parseTimeQuery(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
