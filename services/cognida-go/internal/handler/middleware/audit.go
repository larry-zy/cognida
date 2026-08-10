// Package middleware 提供HTTP中间件
package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	auditmodel "cognida/internal/model/audit"
	"cognida/internal/pkg/httputil"
	auditsvc "cognida/internal/service/audit"
)

// ========================================
// AuditMiddleware 请求审计中间件
// ========================================

// AuditMiddleware 请求审计中间件。
//
// 在 Trace 之后注册：复用 TraceMiddleware 注入的 request_id，为每个业务请求
// 构建一条审计记录并异步入队落库（非阻塞）。健康检查、静态资源、CORS 预检不审计。
type AuditMiddleware struct {
	writer *auditsvc.Writer
}

// NewAuditMiddleware 创建请求审计中间件
func NewAuditMiddleware(writer *auditsvc.Writer) *AuditMiddleware {
	return &AuditMiddleware{writer: writer}
}

// Apply 应用中间件
func (m *AuditMiddleware) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// 跳过非业务流量：CORS 预检、健康检查、前端静态资源。
		if c.Request.Method == http.MethodOptions ||
			path == "/health" ||
			strings.HasPrefix(path, "/assets") {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		durationMs := int(time.Since(start).Milliseconds())

		if m.writer != nil {
			m.writer.Enqueue(buildAuditLog(c, path, durationMs))
		}
	}
}

// buildAuditLog 从请求上下文构建审计实体。
func buildAuditLog(c *gin.Context, path string, durationMs int) *auditmodel.AuditLog {
	// 用 gin 匹配到的路由模板（含 :id 占位）作为 action，避免 ID 造成的高基数。
	route := c.FullPath()
	if route == "" {
		route = path
	}
	module := moduleFromRoute(route)
	action := c.Request.Method + " " + route

	// 用与查询侧一致的归一化租户/用户（GetTenantID/GetUserID，dev 缺省为 1）。
	// 否则写入原始 tenant_id=0、查询按默认 1 过滤，会导致查不到自己刚写入的审计。
	tid := httputil.GetTenantIDWithDefault(c.GetInt64("tenant_id"))
	uid := httputil.GetUserIDWithDefault(c.GetInt64("user_id"))

	entry := auditmodel.NewAuditLog(&tid, &uid, module, action)

	var ridPtr, ipPtr *string
	if rid := c.GetString("request_id"); rid != "" {
		ridPtr = &rid
	}
	if ip := c.ClientIP(); ip != "" {
		ipPtr = &ip
	}
	entry.WithRequest(ridPtr, ipPtr).WithDuration(durationMs)

	status := c.Writer.Status()
	if status >= http.StatusBadRequest {
		entry.Status = "error"
		msg := c.Errors.String()
		if msg == "" {
			msg = http.StatusText(status)
		}
		entry.ErrorMessage = &msg
	}

	// details 轻量记录 HTTP 元信息（JSON）。不落请求体，避免体积与敏感数据风险。
	details := map[string]interface{}{
		"http_status": status,
		"method":      c.Request.Method,
		"path":        path,
	}
	if raw := c.Request.URL.RawQuery; raw != "" {
		details["query"] = raw
	}
	if b, err := json.Marshal(details); err == nil {
		entry.Details = string(b)
	}

	return entry
}

// moduleFromRoute 从路由模板提取业务模块名，如 /api/v1/sessions/:id → "sessions"。
func moduleFromRoute(route string) string {
	trimmed := strings.TrimPrefix(route, "/api/v1/")
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return "root"
	}
	if idx := strings.IndexByte(trimmed, '/'); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}
