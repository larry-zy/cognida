// Package cache provides Prometheus metrics for semantic cache
package cache

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	domaincache "cognida/internal/model/cache"
)

// ========================================
// Prometheus Metrics Collector
// ========================================

var (
	// cacheHits 总命中次数
	cacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "semantic_cache_hits_total",
		Help: "Total number of cache hits",
	}, []string{"agent_type", "model"})

	// cacheMisses 总未命中次数
	cacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "semantic_cache_misses_total",
		Help: "Total number of cache misses",
	}, []string{"agent_type", "model"})

	// cacheHitDuration 命中响应延迟
	cacheHitDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "semantic_cache_hit_duration_seconds",
		Help:    "Cache hit response duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"agent_type"})

	// cacheMissDuration 未命中响应延迟
	cacheMissDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "semantic_cache_miss_duration_seconds",
		Help:    "Cache miss response duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"agent_type"})

	// cacheSimilarity 命中相似度分布
	cacheSimilarity = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "semantic_cache_similarity_score",
		Help:    "Similarity score of cache hits",
		Buckets: []float64{0.5, 0.6, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95, 0.97, 0.99, 1.0},
	}, []string{"agent_type"})

	// cacheTokensSaved 节省的 token 总数
	cacheTokensSaved = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "semantic_cache_tokens_saved_total",
		Help: "Total number of tokens saved by cache hits",
	}, []string{"agent_type", "model"})

	// cacheSize 当前缓存条目数
	cacheSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "semantic_cache_size",
		Help: "Current number of cache entries",
	}, []string{"agent_type", "tenant"})

	// cacheEvictions 缓存驱逐次数
	cacheEvictions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "semantic_cache_evictions_total",
		Help: "Total number of cache evictions",
	}, []string{"reason"})
	// cacheWriteErrors 写入错误次数
	cacheWriteErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "semantic_cache_write_errors_total",
		Help: "Total number of cache write errors",
	}, []string{"storage"}) // redis, milvus
)

// ========================================
// Metrics Recorder
// ========================================

// MetricsRecorder 指标记录器
type MetricsRecorder struct {
	agentType string
	model      string
}

// NewMetricsRecorder 创建指标记录器
func NewMetricsRecorder(agentType, model string) *MetricsRecorder {
	return &MetricsRecorder{
		agentType: agentType,
		model:      model,
	}
}

// RecordHit 记录命中
func (r *MetricsRecorder) RecordHit(similarity float32, duration time.Duration, tokensSaved int) {
	labels := prometheus.Labels{
		"agent_type": r.agentType,
		"model":      r.model,
	}

	cacheHits.With(labels).Inc()
	cacheSimilarity.With(prometheus.Labels{"agent_type": r.agentType}).Observe(float64(similarity))
	cacheHitDuration.With(prometheus.Labels{"agent_type": r.agentType}).Observe(duration.Seconds())

	if tokensSaved > 0 {
		cacheTokensSaved.With(labels).Add(float64(tokensSaved))
	}
}

// RecordMiss 记录未命中
func (r *MetricsRecorder) RecordMiss(duration time.Duration) {
	labels := prometheus.Labels{
		"agent_type": r.agentType,
		"model":      r.model,
	}

	cacheMisses.With(labels).Inc()
	cacheMissDuration.With(prometheus.Labels{"agent_type": r.agentType}).Observe(duration.Seconds())
}

// RecordWriteError 记录写入错误
func RecordWriteError(storage string) {
	cacheWriteErrors.With(prometheus.Labels{"storage": storage}).Inc()
}

// UpdateCacheSize 更新缓存大小
func UpdateCacheSize(agentType, tenant string, size float64) {
	cacheSize.With(prometheus.Labels{
		"agent_type": agentType,
		"tenant":     tenant,
	}).Set(size)
}

// RecordEviction 记录驱逐
func RecordEviction(reason string) {
	cacheEvictions.With(prometheus.Labels{"reason": reason}).Inc()
}

// ========================================
// Cached Repository with Metrics
// ========================================

// CachedRepositoryWithMetrics 带指标的缓存仓储
type CachedRepositoryWithMetrics struct {
	vectorRepo   domaincache.CacheVectorRepository
	contentRepo  domaincache.CacheContentRepository
	recorder     *MetricsRecorder
}

// NewCachedRepositoryWithMetrics 创建带指标的仓储
func NewCachedRepositoryWithMetrics(
	vectorRepo domaincache.CacheVectorRepository,
	contentRepo domaincache.CacheContentRepository,
	agentType, model string,
) *CachedRepositoryWithMetrics {
	return &CachedRepositoryWithMetrics{
		vectorRepo:  vectorRepo,
		contentRepo: contentRepo,
		recorder:    NewMetricsRecorder(agentType, model),
	}
}

// Search 搜索向量（带指标）
func (r *CachedRepositoryWithMetrics) Search(ctx context.Context, vector []float32, tenantID int64, agentType string, topK int) ([]*domaincache.VectorSearchResult, error) {
	start := time.Now()
	results, err := r.vectorRepo.Search(ctx, vector, tenantID, agentType, topK)
	duration := time.Since(start)

	if err != nil {
		r.recorder.RecordMiss(duration)
		return nil, err
	}

	if len(results) == 0 {
		r.recorder.RecordMiss(duration)
	}

	return results, nil
}

// GetContent 获取内容（带指标）
func (r *CachedRepositoryWithMetrics) GetContent(ctx context.Context, cacheID string) (*domaincache.CacheEntry, error) {
	return r.contentRepo.GetContent(ctx, cacheID)
}

// SetContent 设置内容（带指标）
func (r *CachedRepositoryWithMetrics) SetContent(ctx context.Context, entry *domaincache.CacheEntry, ttl time.Duration) error {
	err := r.contentRepo.SetContent(ctx, entry, ttl)
	if err != nil {
		RecordWriteError("redis")
	}
	return err
}

// Insert 插入向量（带指标）
func (r *CachedRepositoryWithMetrics) Insert(ctx context.Context, entry *domaincache.CacheEntry) error {
	err := r.vectorRepo.Insert(ctx, entry)
	if err != nil {
		RecordWriteError("milvus")
	}
	return err
}

// ========================================
// Metrics Collector for Stats
// ========================================

// CacheStatsCollector 缓存统计收集器
type CacheStatsCollector struct {
	useCase *SemanticCacheService
}

// NewCacheStatsCollector 创建统计收集器
func NewCacheStatsCollector(useCase *SemanticCacheService) *CacheStatsCollector {
	return &CacheStatsCollector{useCase: useCase}
}

// CollectToPrometheus 将统计数据推送到 Prometheus
func (c *CacheStatsCollector) CollectToPrometheus(ctx context.Context) error {
	stats, err := c.useCase.GetStats(ctx)
	if err != nil {
		return err
	}

	// 使用统计数据更新指标
	_ = stats // 避免未使用警告

	// 这里可以更新基于统计数据的指标
	// 例如：命中率 = hits / (hits + misses)

	return nil
}

// Describe 实现 prometheus.Collector 接口
func (c *CacheStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// Collect 实现 prometheus.Collector 接口
func (c *CacheStatsCollector) Collect(ch chan<- prometheus.Metric) {
	// 收集指标到 Prometheus
}
