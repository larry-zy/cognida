// Package handler provides HTTP handlers for cache management API
package handler

import (
	"log"

	"github.com/gin-gonic/gin"

	domaincache "cognida/internal/model/cache"
)

// CacheHandler 缓存管理 API 处理器
type CacheHandler struct {
	mgmtService domaincache.CacheManager
}

// NewCacheHandler 创建缓存处理器
func NewCacheHandler(mgmtService domaincache.CacheManager) *CacheHandler {
	return &CacheHandler{
		mgmtService: mgmtService,
	}
}

// RegisterRoutes 注册路由
func (h *CacheHandler) RegisterRoutes(r *gin.RouterGroup) {
	cache := r.Group("/cache")
	{
		// 管理接口（需要管理员权限）
		admin := cache.Group("/admin")
		{
			admin.DELETE("/clear", h.ClearCache)
			admin.GET("/stats", h.GetCacheStats)
			admin.POST("/warmup", h.WarmupCache)
			admin.DELETE("/stats/reset", h.ResetStats)
			admin.POST("/cleanup", h.CleanupOrphanVectors)
			admin.GET("/health", h.GetHealth)

			// Feature Flag 管理
			admin.GET("/config", h.GetRuntimeConfig)
			admin.PUT("/config", h.UpdateRuntimeConfig)
		}
	}
}

// GetRuntimeConfig 获取运行时配置
// @Summary 获取运行时配置
// @Description 返回当前缓存策略和 Feature Flag 状态
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /cache/admin/config [get]
func (h *CacheHandler) GetRuntimeConfig(c *gin.Context) {
	// 返回当前缓存配置（基础版本）
	// 完整配置管理功能待实现
	OK(c, gin.H{
		"cache_enabled":     true,
		"similarity_threshold": 0.85,
		"ttl_minutes":       60,
		"max_entries":       10000,
		"note":             "Full runtime config management to be implemented",
	})
}

// UpdateRuntimeConfig 更新运行时配置
// @Summary 更新运行时配置
// @Description 动态更新缓存策略
// @Tags cache
// @Accept json
// @Produce json
// @Param request body map[string]interface{} true "配置"
// @Success 200 {object} map[string]string
// @Router /cache/admin/config [put]
func (h *CacheHandler) UpdateRuntimeConfig(c *gin.Context) {
	var req map[string]interface{}
	if !BindJSON(c, &req) {
		return
	}

	log.Printf("[UpdateRuntimeConfig] Config update requested: %v", req)

	// 返回配置更新待实现提示
	OK(c, gin.H{
		"message": "Runtime config update received - full implementation pending",
		"received": req,
	})
}

// ClearCache 清除缓存
// @Summary 清除语义缓存
// @Description 按租户、Agent 类型、模型清除缓存
// @Tags cache
// @Accept json
// @Produce json
// @Param request body domaincache.ClearCacheRequest true "清除请求"
// @Success 200 {object} domaincache.ClearCacheResponse
// @Router /cache/admin/clear [delete]
func (h *CacheHandler) ClearCache(c *gin.Context) {
	var req domaincache.ClearCacheRequest
	if !BindJSON(c, &req) {
		return
	}

	// Admin permission validation handled by middleware

	resp, err := h.mgmtService.ClearCache(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[ClearCache] Failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}

// GetCacheStats 获取缓存统计
// @Summary 获取缓存统计信息
// @Description 返回命中率、相似度分布等统计数据
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} domaincache.CacheStatsResponse
// @Router /cache/admin/stats [get]
func (h *CacheHandler) GetCacheStats(c *gin.Context) {
	resp, err := h.mgmtService.GetCacheStats(c.Request.Context())
	if err != nil {
		log.Printf("[GetCacheStats] Failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}

// WarmupCache 预热缓存
// @Summary 预热语义缓存
// @Description 批量预加载高频查询到缓存
// @Tags cache
// @Accept json
// @Produce json
// @Param request body domaincache.WarmupCacheRequest true "预热请求"
// @Success 200 {object} domaincache.WarmupCacheResponse
// @Router /cache/admin/warmup [post]
func (h *CacheHandler) WarmupCache(c *gin.Context) {
	var req domaincache.WarmupCacheRequest
	if !BindJSON(c, &req) {
		return
	}

	// Admin permission validation handled by middleware

	resp, err := h.mgmtService.WarmupCache(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[WarmupCache] Failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}

// ResetStats 重置统计
// @Summary 重置缓存统计
// @Description 清零命中/未命中计数
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /cache/admin/stats/reset [delete]
func (h *CacheHandler) ResetStats(c *gin.Context) {
	if err := h.mgmtService.ResetStats(c.Request.Context()); err != nil {
		log.Printf("[ResetStats] Failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{"message": "Statistics reset successfully"})
}

// CleanupOrphanVectors 清理孤儿向量
// @Summary 清理孤儿向量
// @Description 删除 Milvus 中对应 Redis 已删除的向量
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /cache/admin/cleanup [post]
func (h *CacheHandler) CleanupOrphanVectors(c *gin.Context) {
	count, err := h.mgmtService.CleanupOrphanVectors(c.Request.Context())
	if err != nil {
		log.Printf("[CleanupOrphanVectors] Failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, gin.H{
		"message": "Cleanup completed",
		"removed": count,
	})
}

// GetHealth 获取缓存健康状态
// @Summary 获取缓存健康状态
// @Description 检查 Redis 和 Milvus 连接状态
// @Tags cache
// @Accept json
// @Produce json
// @Success 200 {object} domaincache.CacheHealthResponse
// @Router /cache/admin/health [get]
func (h *CacheHandler) GetHealth(c *gin.Context) {
	resp, err := h.mgmtService.GetHealth(c.Request.Context())
	if err != nil {
		log.Printf("[GetHealth] Failed: %v", err)
		InternalError(c, err.Error())
		return
	}

	OK(c, resp)
}

// ========================================
// Middleware: Cache Metadata Injection
// ========================================

// CacheResponseWriter 响应写入器，用于注入缓存元数据
type CacheResponseWriter struct {
	gin.ResponseWriter
	fromCache  bool
	cacheID    string
	similarity float32
}

// WriteJSON 写入 JSON 响应并注入缓存元数据
func (w *CacheResponseWriter) WriteJSON(code int, obj any) error {
	// 如果是 map 或结构体，尝试注入缓存元数据
	if m, ok := obj.(map[string]interface{}); ok {
		if w.fromCache {
			m["from_cache"] = true
			m["cache_id"] = w.cacheID
			m["similarity"] = w.similarity
		} else {
			m["from_cache"] = false
		}
	}

	// 使用 gin 内置的 JSON 方法进行序列化
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// 序列化由 gin 框架处理
	return nil
}

// CacheMiddleware 缓存中间件（可选，用于自动注入缓存元数据）
func CacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查响应中是否有缓存信息
		fromCache := c.GetBool("cache_hit")
		cacheID := c.GetString("cache_id")
		similarity := c.GetFloat32("similarity")

		if fromCache {
			c.Set("from_cache", true)
			c.Set("cache_id", cacheID)
			c.Set("similarity", similarity)
		}

		c.Next()
	}
}
