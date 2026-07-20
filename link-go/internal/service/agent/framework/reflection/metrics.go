// Package reflection provides metrics for the Reflection Hook.
package reflection

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ========================================
// Reflection Metrics
// ========================================

var (
	// invocationTotal 反思调用总数
	invocationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_reflection_invocation_total",
		Help: "Total number of reflection invocations",
	}, []string{"agent_id", "critic_type"})

	// iterations 反思迭代次数
	iterations = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agent_reflection_iterations",
		Help:    "Number of iterations in reflection loop",
		Buckets: []float64{1, 2, 3, 5, 10},
	}, []string{"agent_id"})

	// durationSeconds 反思处理耗时
	durationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "agent_reflection_duration_seconds",
		Help:    "Reflection processing duration in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
	}, []string{"agent_id", "success"})

	// errorsTotal 反思错误总数
	errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_reflection_errors_total",
		Help: "Total number of reflection errors",
	}, []string{"agent_id", "error_type"})

	// memoryHitRate 记忆命中率
	memoryHits   atomic.Uint64
	memoryMisses atomic.Uint64

	// qualityScoresBefore 反思前质量分数
	qualityScoresBefore = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agent_reflection_quality_score_before",
		Help: "Quality score before reflection",
	}, []string{"agent_id", "dimension"})

	// qualityScoresAfter 反思后质量分数
	qualityScoresAfter = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "agent_reflection_quality_score_after",
		Help: "Quality score after reflection",
	}, []string{"agent_id", "dimension"})

	// enqueuedTotal 异步反思入队总数
	enqueuedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_reflection_enqueued_total",
		Help: "Total reflection jobs enqueued for async processing",
	}, []string{"agent_id"})

	// droppedTotal 因队列已满被丢弃的入队总数（非阻塞降级，宁可少学不拖慢对话）
	droppedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_reflection_dropped_total",
		Help: "Total reflection jobs dropped due to full queue",
	}, []string{"agent_id"})

	// processedTotal 后台处理的反思任务总数（按结果: stored/skipped/error）
	processedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "agent_reflection_processed_total",
		Help: "Total reflection jobs processed by outcome",
	}, []string{"agent_id", "outcome"})
)

// ========================================
// Metric Recording Functions
// ========================================

// RecordInvocation 记录反思调用
func RecordInvocation(agentID, criticType string) {
	invocationTotal.WithLabelValues(agentID, criticType).Inc()
}

// RecordIterations 记录迭代次数
func RecordIterations(agentID string, count float64) {
	iterations.WithLabelValues(agentID).Observe(count)
}

// RecordDuration 记录处理耗时
func RecordDuration(agentID string, success bool, duration float64) {
	successLabel := "false"
	if success {
		successLabel = "true"
	}
	durationSeconds.WithLabelValues(agentID, successLabel).Observe(duration)
}

// RecordError 记录错误
func RecordError(agentID, errorType string) {
	errorsTotal.WithLabelValues(agentID, errorType).Inc()
}

// RecordMemoryHit 记录记忆命中
func RecordMemoryHit() {
	memoryHits.Add(1)
}

// RecordMemoryMiss 记录记忆未命中
func RecordMemoryMiss() {
	memoryMisses.Add(1)
}

// GetMemoryHitRate 获取记忆命中率
func GetMemoryHitRate() float64 {
	hits := memoryHits.Load()
	misses := memoryMisses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// RecordQualityBefore 记录反思前质量分数
func RecordQualityBefore(agentID, dimension string, score float64) {
	qualityScoresBefore.WithLabelValues(agentID, dimension).Set(score)
}

// RecordQualityAfter 记录反思后质量分数
func RecordQualityAfter(agentID, dimension string, score float64) {
	qualityScoresAfter.WithLabelValues(agentID, dimension).Set(score)
}

// RecordEnqueued 记录一次异步反思入队
func RecordEnqueued(agentID string) {
	enqueuedTotal.WithLabelValues(agentID).Inc()
}

// RecordDropped 记录一次因队列已满的入队丢弃
func RecordDropped(agentID string) {
	droppedTotal.WithLabelValues(agentID).Inc()
}

// RecordProcessed 记录一次后台反思任务处理结果（outcome: stored/skipped/error）
func RecordProcessed(agentID, outcome string) {
	processedTotal.WithLabelValues(agentID, outcome).Inc()
}
