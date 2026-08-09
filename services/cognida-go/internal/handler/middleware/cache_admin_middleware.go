// Package middleware provides admin middleware for cache management API
package middleware

// 注意（未接线）：本文件的缓存管理/Feature-Flag admin 路由（RegisterRoutes）
// 目前**未在任何 router 中注册**，配套的 infrastructure/cache 热更新子系统也未装配。
// 保留为实验性/待接线代码；接线或确认废弃前，勿据此判断其为“在用”。

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	domaincache "cognida/internal/model/cache"
)

// ========================================
// Admin Authorization Middleware
// ========================================

// AdminAuthConfig 管理员认证配置
type AdminAuthConfig struct {
	AdminRole       string   // 管理员角色
	AdminUsers      []string // 管理员用户列表
	BearerToken     string   // Bearer Token（简单认证）
	EnableRoleCheck bool     // 是否启用角色检查
}

// DefaultAdminAuthConfig 默认管理员认证配置
var DefaultAdminAuthConfig = &AdminAuthConfig{
	AdminRole:       "admin",
	AdminUsers:      []string{"admin"},
	BearerToken:     "",
	EnableRoleCheck: true,
}

// AdminAuth 管理员认证中间件
func AdminAuth(cfg *AdminAuthConfig) gin.HandlerFunc {
	if cfg == nil {
		cfg = DefaultAdminAuthConfig
	}

	return func(c *gin.Context) {
		// 1. 检查 Bearer Token（最简单的方式）
		if cfg.BearerToken != "" {
			auth := c.GetHeader("Authorization")
			if auth == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
				c.Abort()
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			if token != cfg.BearerToken {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				c.Abort()
				return
			}

			c.Next()
			return
		}

		// 2. 检查用户角色
		if cfg.EnableRoleCheck {
			role := c.GetString("user_role")
			if role == "" {
				role = c.GetHeader("X-User-Role")
			}

			if role != cfg.AdminRole {
				c.JSON(http.StatusForbidden, gin.H{
					"error":   "Admin access required",
					"current": role,
				})
				c.Abort()
				return
			}
		}

		// 3. 检查用户白名单
		if len(cfg.AdminUsers) > 0 {
			userID := c.GetString("user_id")
			if userID == "" {
				userID = c.GetHeader("X-User-ID")
			}

			isAdmin := false
			for _, admin := range cfg.AdminUsers {
				if userID == admin {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				c.JSON(http.StatusForbidden, gin.H{
					"error":    "User not in admin list",
					"user_id":  userID,
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// ========================================
// Feature Flag Management Handler
// ========================================

// FeatureFlagHandler Feature Flag 管理处理器
type FeatureFlagHandler struct {
	reloader domaincache.FeatureFlagReloader
}

// NewFeatureFlagHandler 创建 Feature Flag 处理器
func NewFeatureFlagHandler(reloader domaincache.FeatureFlagReloader) *FeatureFlagHandler {
	return &FeatureFlagHandler{
		reloader: reloader,
	}
}

// GetFeatureFlag 获取 Feature Flag 状态
// @Summary 获取缓存开关状态
// @Description 返回全局和各 Agent 类型的缓存开关状态
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /cache/admin/feature-flag [get]
func (h *FeatureFlagHandler) GetFeatureFlag(c *gin.Context) {
	ff := h.reloader.GetFeatureFlag()
	c.JSON(http.StatusOK, ff.GetAll())
}

// ToggleGlobal 切换全局开关
// @Summary 切换全局缓存开关
// @Description 启用或禁用全局语义缓存
// @Tags cache
// @Accept json
// @Produce json
// @Param request body map[string]bool true "enabled: true/false"
// @Success 200 {object} map[string]string
// @Router /cache/admin/feature-flag/global [put]
func (h *FeatureFlagHandler) ToggleGlobal(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.reloader.ToggleGlobal(req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := "disabled"
	if req.Enabled {
		status = "enabled"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("Global cache %s", status),
		"global_enabled": req.Enabled,
	})
}

// ToggleAgent 切换 Agent 级别开关
// @Summary 切换 Agent 缓存开关
// @Description 启用或禁用指定 Agent 类型的缓存
// @Tags cache
// @Accept json
// @Produce json
// @Param agent_type path string true "Agent 类型"
// @Param request body map[string]bool true "enabled: true/false"
// @Success 200 {object} map[string]string
// @Router /cache/admin/feature-flag/agent/{agent_type} [put]
func (h *FeatureFlagHandler) ToggleAgent(c *gin.Context) {
	agentType := c.Param("agent_type")
	if agentType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_type is required"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.reloader.ToggleAgent(agentType, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := "disabled"
	if req.Enabled {
		status = "enabled"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    fmt.Sprintf("Agent %s cache %s", agentType, status),
		"agent_type": agentType,
		"enabled":    req.Enabled,
	})
}

// GetMetrics 获取配置指标
// @Summary 获取缓存配置指标
// @Description 返回当前缓存配置的统计信息
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} domaincache.CacheConfigMetrics
// @Router /cache/admin/feature-flag/metrics [get]
func (h *FeatureFlagHandler) GetMetrics(c *gin.Context) {
	ff := h.reloader.GetFeatureFlag()
	metrics := ff.GetMetrics()
	c.JSON(http.StatusOK, metrics)
}

// RegisterRoutes 注册 Feature Flag 路由
func (h *FeatureFlagHandler) RegisterRoutes(r *gin.RouterGroup, authCfg *AdminAuthConfig) {
	featureFlag := r.Group("/admin/feature-flag")
	{
		featureFlag.GET("", AdminAuth(authCfg), h.GetFeatureFlag)
		featureFlag.PUT("/global", AdminAuth(authCfg), h.ToggleGlobal)
		featureFlag.PUT("/agent/:agent_type", AdminAuth(authCfg), h.ToggleAgent)
		featureFlag.GET("/metrics", AdminAuth(authCfg), h.GetMetrics)
	}
}
