// Package cache provides semantic cache domain entities and interfaces
package cache

import (
	"context"
	"time"
)

// ========================================
// Semantic Cache Entities
// ========================================

// CacheRequest 缓存请求
type CacheRequest struct {
	Query          string            // 用户查询
	PromptTemplate string            // Prompt 模板
	TenantID       int64             // 租户 ID
	AgentType      string            // Agent 类型
	Model          string            // 模型名称
	Metadata       map[string]string // 附加元数据
}

// CacheResponse 缓存响应
type CacheResponse struct {
	Response    string    // LLM 响应内容
	CacheID     string    // 缓存 ID
	Similarity  float32   // 相似度分数
	FromCache   bool      // 是否来自缓存
	TokensSaved int       // 节省的 token 数量
	CreatedAt   time.Time // 缓存创建时间
}

// CacheEntry 缓存条目
type CacheEntry struct {
	CacheID       string            // 缓存 ID
	Query         string            // 查询文本
	Response      string            // 响应内容
	Model         string            // 模型名称
	AgentType     string            // Agent 类型
	TenantID      int64             // 租户 ID
	PromptHash    string            // Prompt 模板 hash
	Vector        []float32         // 查询向量
	CreatedAt     int64             // 创建时间（Unix 时间戳）
	UpdatedAt     int64             // 更新时间
	HitCount      int64             // 命中计数
	TTL           time.Duration     // 过期时间
	Metadata      map[string]string // 元数据
}

// CacheStats 缓存统计
type CacheStats struct {
	Hits          int64             // 命中次数
	Misses        int64             // 未命中次数
	HitRate       float64           // 命中率
	TotalSaved    int64             // 总共节省的 token 数
	SimilarityDist map[string]int64 // 相似度分布
}

// SearchResult 向量搜索结果
type SearchResult struct {
	CacheID    string  // 缓存 ID
	Similarity float32 // 相似度分数
}

// SemanticCacheConfig 语义缓存全局配置
type SemanticCacheConfig struct {
	Enabled  bool          // 是否启用
	Threshold float32       // 相似度阈值
	TTL      time.Duration // 缓存过期时间
	TopK     int           // 检索数量
}

// AgentCacheConfig Agent 级别缓存配置
type AgentCacheConfig struct {
	Enabled  bool          // 是否启用
	Threshold float32       // 相似度阈值
	TTL      time.Duration // 缓存过期时间
	TopK     int           // 检索数量
}

// AgentCacheStrategy Agent 缓存策略集合
type AgentCacheStrategy struct {
	Global   SemanticCacheConfig            // 全局配置
	Defaults AgentCacheConfig               // 默认配置
	Agents   map[string]AgentCacheConfig    // 各 Agent 配置
}

// GetAgentConfig 获取指定 Agent 的缓存配置
func (s *AgentCacheStrategy) GetAgentConfig(agentType string) *AgentCacheConfig {
	if !s.Global.Enabled {
		return &AgentCacheConfig{Enabled: false}
	}

	if config, ok := s.Agents[agentType]; ok {
		return &config
	}

	return &AgentCacheConfig{
		Enabled:  false, // 未配置的 Agent 默认关闭
		Threshold: s.Defaults.Threshold,
		TTL:      s.Defaults.TTL,
		TopK:     s.Defaults.TopK,
	}
}

// ========================================
// Vector Repository Interface
// ========================================

// CacheVectorRepository 缓存向量仓储接口
type CacheVectorRepository interface {
	// Search 搜索相似向量
	Search(ctx context.Context, vector []float32, tenantID int64, agentType string, topK int) ([]*VectorSearchResult, error)

	// Insert 插入向量
	Insert(ctx context.Context, entry *CacheEntry) error

	// Delete 删除向量
	Delete(ctx context.Context, cacheID string) error

	// DeleteByTenant 删除租户所有向量
	DeleteByTenant(ctx context.Context, tenantID int64) error

	// DeleteByAgent 删除指定 Agent 类型的所有向量
	DeleteByAgent(ctx context.Context, tenantID int64, agentType string) error
}

// VectorSearchResult 向量搜索结果
type VectorSearchResult struct {
	CacheID    string  // 缓存 ID
	Similarity float32 // 相似度分数
}

// ========================================
// Content Repository Interface
// ========================================

// CacheContentRepository 缓存内容仓储接口
type CacheContentRepository interface {
	// GetContent 获取缓存内容
	GetContent(ctx context.Context, cacheID string) (*CacheEntry, error)

	// SetContent 设置缓存内容
	SetContent(ctx context.Context, entry *CacheEntry, ttl time.Duration) error

	// MGetContents 批量获取缓存内容
	MGetContents(ctx context.Context, cacheIDs []string) (map[string]*CacheEntry, error)

	// Delete 删除缓存内容
	Delete(ctx context.Context, cacheID string) error

	// UpdateHitCount 更新命中计数
	UpdateHitCount(ctx context.Context, cacheID string) error

	// RecordHit 记录命中
	RecordHit(ctx context.Context) error

	// RecordMiss 记录未命中
	RecordMiss(ctx context.Context) error

	// GetStats 获取统计信息
	GetStats(ctx context.Context) (*CacheStats, error)

	// IncrementSimilarityBucket 增加相似度区间计数
	IncrementSimilarityBucket(ctx context.Context, bucket string) error
}

// ========================================
// Cache Use Case Interface
// ========================================

// SemanticCache 语义缓存用例接口
type SemanticCache interface {
	// IsEnabled 检查缓存是否启用
	IsEnabled(agentType string) bool

	// Get 获取缓存
	Get(ctx context.Context, req *CacheRequest) (*CacheResponse, error)

	// Set 设置缓存
	Set(ctx context.Context, req *CacheRequest, resp string, tokensUsed int) error

	// Clear 清除缓存
	Clear(ctx context.Context, tenantID int64, agentType, model string) error

	// GetStats 获取统计信息
	GetStats(ctx context.Context) (*CacheStats, error)
}
