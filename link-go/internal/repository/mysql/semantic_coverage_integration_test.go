//go:build integration
// +build integration

// Package mysql: 语义查询覆盖率埋点仓储真实 DB 集成测试。
//
// 受 `integration` 构建标签门控。示例：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestSemanticCoverage -v
//
// 覆盖：Record 写入 → Stats 聚合往返，验证 covered/cache_hit/fallback 按模型分组计数
// 与命中率（(covered+cache_hit)/total）计算，且严格按 tenant_id 隔离。
package mysql

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"link/internal/model/semantic"
)

const itCovTenant = int64(990002)

func newCoverageIT(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&SemanticCoverageLogModel{}); err != nil {
		t.Fatalf("automigrate coverage log: %v", err)
	}
	clean := func() {
		db.Where("tenant_id IN ?", []int64{itCovTenant, itCovTenant + 1}).Delete(&SemanticCoverageLogModel{})
	}
	clean()
	return db, clean
}

func TestSemanticCoverage_RecordAndStatsRoundTrip(t *testing.T) {
	db, clean := newCoverageIT(t)
	defer clean()
	repo := NewSemanticCoverageRepository(db)
	ctx := context.Background()

	// sales: 3 covered + 1 cache_hit + 1 fallback = total 5, hit_ratio 0.8
	events := []semantic.CoverageEvent{
		{TenantID: itCovTenant, RequestID: "r1", Model: "sales", Outcome: semantic.CoverageCovered},
		{TenantID: itCovTenant, RequestID: "r2", Model: "sales", Outcome: semantic.CoverageCovered},
		{TenantID: itCovTenant, RequestID: "r3", Model: "sales", Outcome: semantic.CoverageCovered},
		{TenantID: itCovTenant, RequestID: "r4", Model: "sales", Outcome: semantic.CoverageCacheHit},
		{TenantID: itCovTenant, RequestID: "r5", Model: "sales", Outcome: semantic.CoverageFallback, Uncovered: []string{"利润"}},
		// finance: 1 fallback = total 1, hit_ratio 0
		{TenantID: itCovTenant, RequestID: "r6", Model: "finance", Outcome: semantic.CoverageFallback, Uncovered: []string{"净利率"}},
		// 另一租户同名模型：不应污染本租户统计。
		{TenantID: itCovTenant + 1, RequestID: "x1", Model: "sales", Outcome: semantic.CoverageCovered},
	}
	for _, ev := range events {
		if err := repo.Record(ctx, ev); err != nil {
			t.Fatalf("Record %s/%s: %v", ev.Model, ev.Outcome, err)
		}
	}

	stats, err := repo.Stats(ctx, itCovTenant)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	byModel := map[string]semantic.CoverageModelStat{}
	for _, s := range stats {
		byModel[s.Model] = s
	}
	if len(byModel) != 2 {
		t.Fatalf("应仅统计本租户 2 个模型, got %d: %+v", len(byModel), stats)
	}

	sales := byModel["sales"]
	if sales.Covered != 3 || sales.CacheHit != 1 || sales.Fallback != 1 || sales.Total != 5 {
		t.Errorf("sales 计数错误: %+v", sales)
	}
	if sales.HitRatio < 0.79 || sales.HitRatio > 0.81 {
		t.Errorf("sales 命中率应约 0.8, got %v", sales.HitRatio)
	}

	finance := byModel["finance"]
	if finance.Fallback != 1 || finance.Total != 1 || finance.HitRatio != 0 {
		t.Errorf("finance 计数/命中率错误: %+v", finance)
	}
}

func TestSemanticCoverage_StatsEmptyForUnknownTenant(t *testing.T) {
	db, clean := newCoverageIT(t)
	defer clean()
	repo := NewSemanticCoverageRepository(db)

	stats, err := repo.Stats(context.Background(), int64(9999999))
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("无埋点租户应返回空统计, got %+v", stats)
	}
}
