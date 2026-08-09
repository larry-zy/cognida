package semantic

import "context"

// CoverageOutcome 语义查询的治理命中态：治理直出 / 受信缓存命中 / 回退词法。
// 覆盖率 = (covered + cache_hit) / total —— 衡量 semantic_query 走治理主路径而非回退的比例。
type CoverageOutcome string

const (
	// CoverageCovered 引擎按治理口径直出 SQL（治理命中）。
	CoverageCovered CoverageOutcome = "covered"
	// CoverageCacheHit 命中受信查询缓存（Verified/Golden，亦属治理命中）。
	CoverageCacheHit CoverageOutcome = "cache_hit"
	// CoverageFallback 未覆盖/无可用模型，回退词法 NL2SQL。
	CoverageFallback CoverageOutcome = "fallback"
)

// CoverageEvent 一次 semantic_query 的覆盖埋点（治理命中率的原子事件）。
//
// RequestID 透传全链路 request_id，与 audit_logs / Loki 采集的原始日志经同一 rid 关联，
// 使「某次会话为何回退」可在结构化审计与原始日志两侧交叉定位。
type CoverageEvent struct {
	TenantID  int64
	RequestID string
	Model     string // 命中/尝试的语义模型名；无可用模型时为空
	Outcome   CoverageOutcome
	Uncovered []string // Fallback 时未覆盖的语义名（可空），供定位建模缺口
}

// CoverageModelStat 单个语义模型的覆盖聚合。
type CoverageModelStat struct {
	Model    string  `json:"model"`
	Covered  int64   `json:"covered"`
	CacheHit int64   `json:"cache_hit"`
	Fallback int64   `json:"fallback"`
	Total    int64   `json:"total"`
	HitRatio float64 `json:"hit_ratio"` // (covered+cache_hit)/total，total=0 时为 0
}

// CoverageSink 覆盖埋点写入端口：best-effort。
// 实现须自洽吞错，调用方以「记录失败不影响查询主路径」为契约。
type CoverageSink interface {
	Record(ctx context.Context, ev CoverageEvent) error
}

// CoverageReporter 覆盖率读侧端口：按租户聚合每个语义模型的治理命中率。
type CoverageReporter interface {
	Stats(ctx context.Context, tenantID int64) ([]CoverageModelStat, error)
}

// CoverageRepository 覆盖埋点仓储（读写合一）。
type CoverageRepository interface {
	CoverageSink
	CoverageReporter
}
