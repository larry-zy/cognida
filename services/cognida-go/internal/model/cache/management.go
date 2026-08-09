// Package cache 定义缓存领域的实体、DTO 与端口接口。
package cache

import (
	"context"
	"time"
)

// ========================================
// 缓存管理 DTO（领域层）
// ========================================

// ClearCacheRequest 清除缓存请求
type ClearCacheRequest struct {
	TenantID  int64  `json:"tenant_id"`
	AgentType string `json:"agent_type"`
	Model     string `json:"model"`
}

// ClearCacheResponse 清除缓存响应
type ClearCacheResponse struct {
	DeletedCount int64  `json:"deleted_count"`
	Message      string `json:"message"`
}

// CacheStatsResponse 缓存统计响应
type CacheStatsResponse struct {
	Hits           int64            `json:"hits"`
	Misses         int64            `json:"misses"`
	HitRate        float64          `json:"hit_rate"`
	SimilarityDist map[string]int64 `json:"similarity_dist"`
}

// WarmupCacheQuery 预热缓存查询项
type WarmupCacheQuery struct {
	Query          string            `json:"query"`
	PromptTemplate string            `json:"prompt_template"`
	AgentType      string            `json:"agent_type"`
	Model          string            `json:"model"`
	Metadata       map[string]string `json:"metadata"`
}

// WarmupCacheRequest 预热缓存请求
type WarmupCacheRequest struct {
	TenantID int64              `json:"tenant_id"`
	Queries  []WarmupCacheQuery `json:"queries"`
}

// WarmupCacheResponse 预热缓存响应
type WarmupCacheResponse struct {
	Total    int      `json:"total"`
	Success  int      `json:"success"`
	Failed   int      `json:"failed"`
	Progress float64  `json:"progress"`
	Message  string   `json:"message"`
	CacheIDs []string `json:"cache_ids"`
}

// CacheHealthResponse 缓存健康状态响应
type CacheHealthResponse struct {
	Healthy      bool   `json:"healthy"`
	RedisStatus  string `json:"redis_status"`
	MilvusStatus string `json:"milvus_status"`
	VectorCount  int64  `json:"vector_count"`
	CacheCount   int64  `json:"cache_count"`
	Message      string `json:"message"`
}

// CacheConfigMetrics 缓存配置指标
type CacheConfigMetrics struct {
	GlobalEnabled  bool      `json:"global_enabled"`
	AgentCount     int       `json:"agent_count"`
	EnabledAgents  []string  `json:"enabled_agents"`
	DisabledAgents []string  `json:"disabled_agents"`
	LastUpdateTime time.Time `json:"last_update_time"`
}

// ========================================
// 缓存管理端口（消费端接口）
// ========================================

// CacheManager 缓存管理端口。Handler 经此接口依赖缓存管理能力，
// 避免直接耦合 infrastructure 实现。
type CacheManager interface {
	ClearCache(ctx context.Context, req *ClearCacheRequest) (*ClearCacheResponse, error)
	GetCacheStats(ctx context.Context) (*CacheStatsResponse, error)
	WarmupCache(ctx context.Context, req *WarmupCacheRequest) (*WarmupCacheResponse, error)
	ResetStats(ctx context.Context) error
	CleanupOrphanVectors(ctx context.Context) (int, error)
	GetHealth(ctx context.Context) (*CacheHealthResponse, error)
}

// FeatureFlagView 缓存特性开关只读视图端口。
type FeatureFlagView interface {
	GetAll() map[string]interface{}
	GetMetrics() *CacheConfigMetrics
}

// FeatureFlagReloader 缓存特性开关热加载端口。
type FeatureFlagReloader interface {
	GetFeatureFlag() FeatureFlagView
	ToggleGlobal(enabled bool) error
	ToggleAgent(agentType string, enabled bool) error
}
