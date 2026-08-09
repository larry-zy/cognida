// Package mysql 指标语义层覆盖埋点持久化（agent_semantic_coverage_logs 表）。
package mysql

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"cognida/internal/model/semantic"
)

// SemanticCoverageLogModel 语义查询覆盖埋点 GORM 模型。
//
// 每条 = 一次 semantic_query 的治理命中态（covered/cache_hit/fallback）。
// request_id 关联全链路审计与 Loki 原始日志；tenant_id+model 支撑按模型的命中率聚合。
type SemanticCoverageLogModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TenantID  int64     `gorm:"column:tenant_id;not null;index:idx_sem_cov_tenant_model,priority:1" json:"tenant_id"`
	Model     string    `gorm:"column:model;type:varchar(128);index:idx_sem_cov_tenant_model,priority:2" json:"model"`
	RequestID string    `gorm:"column:request_id;type:varchar(64);index:idx_sem_cov_rid" json:"request_id"`
	Outcome   string    `gorm:"column:outcome;type:varchar(16);not null" json:"outcome"`
	Uncovered string    `gorm:"column:uncovered;type:varchar(512)" json:"uncovered,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;not null;index:idx_sem_cov_created" json:"created_at"`
}

// TableName 指定表名。
func (SemanticCoverageLogModel) TableName() string { return "agent_semantic_coverage_logs" }

// SemanticCoverageRepository 覆盖埋点 MySQL 实现（读写合一，满足 semantic.CoverageRepository）。
type SemanticCoverageRepository struct {
	db *gorm.DB
}

// NewSemanticCoverageRepository 构造覆盖埋点仓储。
func NewSemanticCoverageRepository(db *gorm.DB) *SemanticCoverageRepository {
	return &SemanticCoverageRepository{db: db}
}

// uncoveredColMax 与 SemanticCoverageLogModel.Uncovered 的 varchar(512) 对齐；
// 埋点是诊断用途，超长时截断而非让 Create 在严格模式下报错（best-effort 不该噪声化）。
const uncoveredColMax = 512

// Record 落一条覆盖埋点。best-effort：调用方吞错，埋点失败不影响查询主路径。
func (r *SemanticCoverageRepository) Record(ctx context.Context, ev semantic.CoverageEvent) error {
	uncovered := strings.Join(ev.Uncovered, ",")
	if len(uncovered) > uncoveredColMax {
		// 按字节截断到列宽，保留可读前缀（诊断只需知道「有哪些」，非穷举全量）。
		uncovered = uncovered[:uncoveredColMax]
	}
	row := &SemanticCoverageLogModel{
		TenantID:  ev.TenantID,
		Model:     ev.Model,
		RequestID: ev.RequestID,
		Outcome:   string(ev.Outcome),
		Uncovered: uncovered,
		CreatedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Create(row).Error
}

// Stats 按语义模型聚合租户的治理命中率（DB 端 GROUP BY，应用端归并到 per-model）。
func (r *SemanticCoverageRepository) Stats(ctx context.Context, tenantID int64) ([]semantic.CoverageModelStat, error) {
	type aggRow struct {
		Model   string
		Outcome string
		Cnt     int64
	}
	var rows []aggRow
	if err := r.db.WithContext(ctx).
		Model(&SemanticCoverageLogModel{}).
		Select("model, outcome, COUNT(*) AS cnt").
		Where("tenant_id = ?", tenantID).
		Group("model, outcome").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	byModel := make(map[string]*semantic.CoverageModelStat)
	order := make([]string, 0)
	for _, x := range rows {
		s, ok := byModel[x.Model]
		if !ok {
			s = &semantic.CoverageModelStat{Model: x.Model}
			byModel[x.Model] = s
			order = append(order, x.Model)
		}
		switch semantic.CoverageOutcome(x.Outcome) {
		case semantic.CoverageCovered:
			s.Covered += x.Cnt
		case semantic.CoverageCacheHit:
			s.CacheHit += x.Cnt
		case semantic.CoverageFallback:
			s.Fallback += x.Cnt
			// 未知 outcome（旧数据/脏写）不计入任一桶，也不计入分母——
			// 保持 Total == 三桶之和 的不变式，HitRatio 才有确定语义。
		}
	}

	out := make([]semantic.CoverageModelStat, 0, len(order))
	for _, m := range order {
		s := byModel[m]
		s.Total = s.Covered + s.CacheHit + s.Fallback
		if s.Total > 0 {
			s.HitRatio = float64(s.Covered+s.CacheHit) / float64(s.Total)
		}
		out = append(out, *s)
	}
	return out, nil
}
