// Package middleware 提供HTTP中间件
package middleware

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	agentctx "link/internal/model/agent"
	appAccount "link/internal/service/account"
)

// ========================================
// AuthMiddleware 认证中间件
// ========================================

// AuthMiddleware 认证中间件
type AuthMiddleware struct {
	accountService *appAccount.AccountService
}

// NewAuthMiddleware 创建认证中间件
func NewAuthMiddleware(accountService *appAccount.AccountService) *AuthMiddleware {
	return &AuthMiddleware{
		accountService: accountService,
	}
}

// Apply 应用中间件
func (m *AuthMiddleware) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 Authorization Header 获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 尝试从 Query 获取
			authHeader = c.Query("token")
		}

		// 开发模式绕过认证（仅用于开发环境）
		// 生产环境禁止使用 DEV_MODE 绕过认证
		if authHeader == "" && os.Getenv("DEV_MODE") == "true" {
			// 记录安全警告日志（生产环境出现该日志说明配置错误）
			log.Printf("[Auth][SECURITY] DEV_MODE 认证绕过生效: %s %s", c.Request.Method, c.Request.URL.Path)
			c.Set("auth_bypass", true)

			const devUserID = int64(1)
			// 尝试从数据库获取用户信息
			if usr, err := m.accountService.GetUserByID(c.Request.Context(), devUserID); err == nil && usr != nil {
				c.Set("user_id", devUserID)
				c.Set("username", usr.Username)
				c.Set("tenant_id", usr.TenantID)
			} else {
				// 如果查询失败，使用默认值
				c.Set("user_id", devUserID)
				c.Set("username", "dev_user")
				c.Set("tenant_id", int64(1))
			}
			c.Next()
			return
		}

		// 注意：不提供 X-API-Key 认证。在 api_keys 表校验（有效期/权限/租户归属）
		// 真正落地前，任何仅凭非空 API Key 放行的分支都等于无认证，已移除。

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
			c.Abort()
			return
		}

		// 支持 Bearer Token
		tokenString := authHeader
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// 验证 Token
		claims, err := m.accountService.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的令牌"})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("tenant_id", claims.TenantID)

		c.Next()
	}
}

// ========================================
// TenantMiddleware 租户中间件
// ========================================

// TenantMiddleware 租户中间件
type TenantMiddleware struct{}

// NewTenantMiddleware 创建租户中间件
func NewTenantMiddleware() *TenantMiddleware {
	return &TenantMiddleware{}
}

// Apply 应用中间件
func (m *TenantMiddleware) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查租户ID是否存在于上下文中
		tenantID, exists := c.Get("tenant_id")
		if !exists || tenantID == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少租户信息"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// ========================================
// CORSMiddleware CORS中间件
// ========================================

// CORSMiddleware CORS中间件
type CORSMiddleware struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
}

// NewCORSMiddleware 创建CORS中间件。
// 白名单从 CORS_ALLOWED_ORIGINS（逗号分隔）读取；未配置时白名单为空（fail-closed），
// 本地开发经 Vite proxy 走同源不受影响。禁止默认 "*"：反射任意 Origin 且带
// Allow-Credentials 等于允许任意站点携带凭证跨域。
func NewCORSMiddleware() *CORSMiddleware {
	var origins []string
	for _, o := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			origins = append(origins, o)
		}
	}
	return &CORSMiddleware{
		AllowedOrigins: origins,
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Origin", "Content-Type", "Authorization", "X-API-Key", "X-Tenant-ID", "X-Request-ID"},
	}
}

// originAllowed 判断请求 Origin 是否命中白名单
func (m *CORSMiddleware) originAllowed(origin string) bool {
	for _, allowed := range m.AllowedOrigins {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// Apply 应用中间件
func (m *CORSMiddleware) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 仅当 Origin 命中白名单时才回写该 Origin 与 Allow-Credentials；
		// 不命中则不回写 Allow-Origin（浏览器将拦截跨域响应）。
		if origin != "" && m.originAllowed(origin) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(m.AllowedMethods, ","))
			c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(m.AllowedHeaders, ","))
			c.Writer.Header().Add("Vary", "Origin")
		}

		// 处理 OPTIONS 请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// ========================================
// RecoveryMiddleware 恢复中间件
// ========================================

// RecoveryMiddleware 恢复中间件
type RecoveryMiddleware struct{}

// NewRecoveryMiddleware 创建恢复中间件
func NewRecoveryMiddleware() *RecoveryMiddleware {
	return &RecoveryMiddleware{}
}

// Apply 应用中间件
func (m *RecoveryMiddleware) Apply() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err interface{}) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "服务器内部错误",
		})
	})
}

// ========================================
// LoggerMiddleware 日志中间件
// ========================================

// LoggerMiddleware 日志中间件
type LoggerMiddleware struct{}

// NewLoggerMiddleware 创建日志中间件
func NewLoggerMiddleware() *LoggerMiddleware {
	return &LoggerMiddleware{}
}

// Apply 应用中间件
func (m *LoggerMiddleware) Apply() gin.HandlerFunc {
	return gin.Logger()
}

// ========================================
// TraceMiddleware 追踪中间件
// ========================================

// TraceMiddleware 追踪中间件
type TraceMiddleware struct{}

// NewTraceMiddleware 创建追踪中间件
func NewTraceMiddleware() *TraceMiddleware {
	return &TraceMiddleware{}
}

// Apply 应用中间件
func (m *TraceMiddleware) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 生成请求ID
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Set("request_id", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)

		// 注入 Go context，使下游 handler/service/gRPC 拦截器可全链路读取 request_id。
		// 仅 c.Set 只能在 gin.Context 内取用，无法随 context.Context 传递到 gRPC 出站与 agent 编排。
		ctx := agentctx.WithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		c.Next()

		// 打一行带 request_id 的访问日志到 stdout，供 Loki 按 rid 做全链路检索
		// （结构化审计另由 AuditMiddleware 落库 MySQL，二者共用同一 request_id）。
		if path := c.Request.URL.Path; path != "/health" && !strings.HasPrefix(path, "/assets") {
			log.Printf("[req] rid=%s %s %s %d %dms ip=%s",
				requestID, c.Request.Method, path, c.Writer.Status(),
				time.Since(start).Milliseconds(), c.ClientIP())
		}
	}
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	return uuid.New().String()
}

// ========================================
// 函数式中间件（用于 gin.Use()）
// ========================================

// CORS CORS中间件函数
func CORS() gin.HandlerFunc {
	return NewCORSMiddleware().Apply()
}

// Recovery 恢复中间件函数
func Recovery() gin.HandlerFunc {
	return NewRecoveryMiddleware().Apply()
}

// Logger 日志中间件函数
func Logger() gin.HandlerFunc {
	return NewLoggerMiddleware().Apply()
}

// Trace 追踪中间件函数
func Trace() gin.HandlerFunc {
	return NewTraceMiddleware().Apply()
}
