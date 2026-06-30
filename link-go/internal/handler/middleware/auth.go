// Package middleware 提供HTTP中间件
package middleware

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	appAccount "link/internal/service/account"
	"link/internal/infrastructure/config"
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
			// 记录安全警告日志
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

		// 从 API Key 获取
		if authHeader == "" {
			apiKey := c.GetHeader("X-API-Key")
			if apiKey != "" {
				// API Key 验证需要查询 api_keys 表并检查有效期、权限等信息
				log.Printf("[Auth] API Key auth bypass in DEV_MODE")
			c.Set("user_id", config.GetUserIDWithDefault(0))
				c.Set("tenant_id", int64(1))
				c.Next()
				return
			}
		}

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

// NewCORSMiddleware 创建CORS中间件
func NewCORSMiddleware() *CORSMiddleware {
	return &CORSMiddleware{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Origin", "Content-Type", "Authorization", "X-API-Key", "X-Tenant-ID", "X-Request-ID"},
	}
}

// Apply 应用中间件
func (m *CORSMiddleware) Apply() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// 设置 CORS 头
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Methods", strings.Join(m.AllowedMethods, ","))
		c.Writer.Header().Set("Access-Control-Allow-Headers", strings.Join(m.AllowedHeaders, ","))

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

		c.Next()
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
