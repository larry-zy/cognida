//go:build integration
// +build integration

// Package mysql: 指标语义层（NL2Semantics）真实 DB 集成测试。
//
// 针对真实 MySQL 运行，受 `integration` 构建标签门控（见 CLAUDE.md 集成测试）。
// 通过 MYSQL_DSN 提供 DSN，例如：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/cognida?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestSemantic -v
//
// 覆盖：语义模型 bundle 落库→回读往返、经指标引擎端到端取数生成治理 SQL、
// 以及「口径一致性」——同一治理指标无论用规范名还是同义词表述，都稳定生成同一 SQL。
package mysql

import (
	"context"
	"strings"
	"testing"

	"gorm.io/gorm"

	"cognida/internal/model/semantic"
	"cognida/internal/service/agent/metricsql"
)

const itSemanticTenant = int64(990001)

func newSemanticIT(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	db := newIntegrationDB(t) // 复用同包 graph_store_integration_test 的 MYSQL_DSN 装配
	if err := db.AutoMigrate(
		&SemanticModelModel{}, &SemanticLogicalTableModel{}, &SemanticDimensionModel{},
		&SemanticMeasureModel{}, &SemanticMetricModel{}, &SemanticRelationModel{},
	); err != nil {
		t.Fatalf("automigrate semantic tables: %v", err)
	}
	modelID := "it_sales_model"
	clean := func() {
		db.Where("model_id = ?", modelID).Delete(&SemanticLogicalTableModel{})
		db.Where("model_id = ?", modelID).Delete(&SemanticDimensionModel{})
		db.Where("model_id = ?", modelID).Delete(&SemanticMeasureModel{})
		db.Where("model_id = ?", modelID).Delete(&SemanticMetricModel{})
		db.Where("model_id = ?", modelID).Delete(&SemanticRelationModel{})
		db.Where("id = ?", modelID).Delete(&SemanticModelModel{})
	}
	clean() // 清理上次残留
	return db, clean
}

// itSalesBundle 构造一个受治理的销售语义模型：度量 金额(sum)、指标 营收（口径固定为
// SUM(orders.amount)，同义词 revenue），维度 区域（同义词 大区）。
func itSalesBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{
			ID: "it_sales_model", TenantID: itSemanticTenant, Name: "it_sales",
			Version: 3, Status: semantic.ModelStatusActive,
		},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "it_lt_order", ModelID: "it_sales_model", Name: "orders", PhysicalTable: "t_order"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "it_d_region", ModelID: "it_sales_model", LogicalTableID: "it_lt_order", Name: "区域", Expr: "region", Synonyms: []string{"大区"}},
		},
		Measures: []*semantic.Measure{
			{ID: "it_ms_amt", ModelID: "it_sales_model", LogicalTableID: "it_lt_order", Name: "金额", Expr: "amount", Aggregation: semantic.AggSum},
		},
		Metrics: []*semantic.Metric{
			{ID: "it_mt_rev", ModelID: "it_sales_model", Name: "营收", Expr: "SUM(orders.amount)", Caliber: "已支付订单金额之和", Synonyms: []string{"revenue"}},
		},
	}
}

func TestSemanticRepository_RoundTripAndEndToEnd(t *testing.T) {
	db, clean := newSemanticIT(t)
	defer clean()
	repo := NewSemanticRepository(db)
	ctx := context.Background()

	if err := repo.UpsertBundle(ctx, itSalesBundle()); err != nil {
		t.Fatalf("UpsertBundle: %v", err)
	}

	// 按租户 + 名称回读生效模型，验证往返完整性。
	got, err := repo.GetActiveModel(ctx, itSemanticTenant, "it_sales")
	if err != nil {
		t.Fatalf("GetActiveModel: %v", err)
	}
	if got.Model.Version != 3 || got.Model.Status != semantic.ModelStatusActive {
		t.Fatalf("model head mismatch: %+v", got.Model)
	}
	if len(got.Metrics) != 1 || got.Metrics[0].Expr != "SUM(orders.amount)" || got.Metrics[0].Caliber != "已支付订单金额之和" {
		t.Fatalf("metric caliber not persisted: %+v", got.Metrics)
	}
	if len(got.Dimensions) != 1 || len(got.Dimensions[0].Synonyms) != 1 || got.Dimensions[0].Synonyms[0] != "大区" {
		t.Fatalf("dimension synonyms not persisted: %+v", got.Dimensions)
	}

	// 端到端：经指标引擎按落库模型生成治理 SQL。
	res, err := metricsql.Build(got, metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}})
	if err != nil {
		t.Fatalf("metricsql.Build: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("expected covered, uncovered=%v", res.Coverage.Uncovered)
	}
	for _, want := range []string{"SUM(orders.amount) AS `营收`", "orders.region AS `区域`", "FROM `t_order` orders", "GROUP BY orders.region"} {
		if !strings.Contains(res.SQL, want) {
			t.Errorf("SQL missing %q\n  got: %s", want, res.SQL)
		}
	}
}

func TestSemanticRepository_CaliberConsistency(t *testing.T) {
	db, clean := newSemanticIT(t)
	defer clean()
	repo := NewSemanticRepository(db)
	ctx := context.Background()
	if err := repo.UpsertBundle(ctx, itSalesBundle()); err != nil {
		t.Fatalf("UpsertBundle: %v", err)
	}
	bundle, err := repo.GetActiveModel(ctx, itSemanticTenant, "it_sales")
	if err != nil {
		t.Fatalf("GetActiveModel: %v", err)
	}

	// 口径一致性：同一治理指标/维度，无论用规范名（营收/区域）还是同义词（revenue/大区）
	// 表述，指标引擎都按既定口径生成「同一条」SQL——这是治理路径相对裸 NL2SQL 的核心保证：
	// 裸 NL2SQL 会因措辞不同而漂移聚合/口径，治理路径把口径钉死在语义模型里。
	canonical, err := metricsql.Build(bundle, metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}})
	if err != nil {
		t.Fatalf("build canonical: %v", err)
	}
	synonym, err := metricsql.Build(bundle, metricsql.Query{Metrics: []string{"revenue"}, Dimensions: []string{"大区"}})
	if err != nil {
		t.Fatalf("build synonym: %v", err)
	}
	if canonical.SQL != synonym.SQL {
		t.Fatalf("caliber drift: same metric via synonym produced different SQL\n  canonical: %s\n  synonym:   %s", canonical.SQL, synonym.SQL)
	}
	// 且钉死的是治理口径表达式，而非裸列猜测。
	if !strings.Contains(canonical.SQL, "SUM(orders.amount) AS `营收`") {
		t.Errorf("governed caliber expr not pinned: %s", canonical.SQL)
	}
}
