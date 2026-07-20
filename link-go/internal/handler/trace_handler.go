package handler

import (
	"github.com/gin-gonic/gin"

	"link/internal/model/trace"
)

// TraceHandler 调用链追踪查询处理器。
//
// 只读：span 的写入由 OTel SpanExporter 异步落库，不经此 handler。
// 列表按 trace 聚合（一行一整棵调用链），详情把某条 trace 的 span 还原成树。
type TraceHandler struct {
	repo trace.Repository
}

// NewTraceHandler 创建追踪查询处理器。
func NewTraceHandler(repo trace.Repository) *TraceHandler {
	return &TraceHandler{repo: repo}
}

// ListTraces 分页查询 trace 摘要。
// 支持按 request_id / session_id / agent_name / trace_id / only_error / 时间范围过滤。
func (h *TraceHandler) ListTraces(c *gin.Context) {
	page, pageSize := GetPageParams(c)

	q := &trace.Query{
		RequestID: c.Query("request_id"),
		SessionID: c.Query("session_id"),
		AgentName: c.Query("agent_name"),
		TraceID:   c.Query("trace_id"),
		OnlyError: c.Query("only_error") == "true",
		Page:      page,
		PageSize:  pageSize,
	}

	// 租户隔离：非空时按登录租户过滤（dev 模式默认租户 1）。
	if tid := GetTenantID(c); tid > 0 {
		q.TenantID = &tid
	}
	if t, ok := parseTimeQuery(c.Query("start_time")); ok {
		q.StartTime = &t
	}
	if t, ok := parseTimeQuery(c.Query("end_time")); ok {
		q.EndTime = &t
	}

	_ = q.Validate()

	list, total, err := h.repo.ListTraces(c.Request.Context(), q)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      q.Page,
		"page_size": q.PageSize,
	})
}

// GetTrace 取某条 trace 的全部 span，并还原成 span 树返回。
func (h *TraceHandler) GetTrace(c *gin.Context) {
	traceID := c.Param("traceID")
	if traceID == "" {
		BadRequest(c, "缺少 trace_id")
		return
	}

	spans, err := h.repo.GetSpansByTraceID(c.Request.Context(), traceID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if len(spans) == 0 {
		NotFound(c, "trace 不存在或已过期")
		return
	}

	// 租户隔离：与 ListTraces 对齐——非空租户下，只允许读本租户的 trace（span 的
	// tenant_id 由 exporter 从 ctx 盖上）。跨租户按「不存在」处理，不泄漏其存在性。
	if tid := GetTenantID(c); tid > 0 {
		for _, s := range spans {
			if s.TenantID != nil && *s.TenantID != tid {
				NotFound(c, "trace 不存在或已过期")
				return
			}
		}
	}

	OK(c, gin.H{
		"trace_id": traceID,
		"spans":    spans,
		"tree":     trace.BuildTree(spans),
	})
}
