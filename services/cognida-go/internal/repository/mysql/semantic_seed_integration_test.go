//go:build integration
// +build integration

// Package mysql verifies the production seed definitions against a migrated MySQL
// database. Every test inserts namespaced fixtures and cleans them up; manual
// seeding and access to an ecommerce database are not prerequisites.
package mysql

import (
	semanticseed "cognida/internal/seed/semantic"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"

	"cognida/internal/model/semantic"
	"cognida/internal/service/agent/metricsql"
)

// getSeeded persists the same bundles used by cmd/seed/semantic in an isolated namespace.
func getSeeded(t *testing.T, name string) *semantic.ModelBundle {
	t.Helper()
	db := newIntegrationDB(t)
	repo := NewSemanticRepository(db)
	prefix := "it_" + uuid.NewString()[:8] + "_"
	const tenantID = int64(990017)
	b := semanticseed.SalesBundle(tenantID, "fixture-source", prefix)
	if name == "商品销售" {
		b = semanticseed.ProductBundle(tenantID, "fixture-source", prefix)
	}
	// Scope the lookup name too, so concurrently running tests cannot select each other's bundle.
	b.Model.Name = prefix + name
	t.Cleanup(func() {
		for _, table := range []string{"agent_semantic_dimensions", "agent_semantic_measures", "agent_semantic_metrics", "agent_semantic_relations", "agent_semantic_logical_tables"} {
			require.NoError(t, db.Table(table).Where("model_id = ?", b.Model.ID).Delete(map[string]interface{}{}).Error)
		}
		require.NoError(t, db.Where("id = ?", b.Model.ID).Delete(&SemanticModelModel{}).Error)
	})
	require.NoError(t, repo.UpsertBundle(context.Background(), b))
	loaded, err := repo.GetActiveModel(context.Background(), tenantID, b.Model.Name)
	require.NoError(t, err)
	return loaded
}

func mustBuild(t *testing.T, b *semantic.ModelBundle, q metricsql.Query) *metricsql.Result {
	t.Helper()
	res, err := metricsql.Build(b, q)
	if err != nil {
		t.Fatalf("metricsql.Build: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("期望 covered=true，实际未覆盖：uncovered=%v\n  query=%+v", res.Coverage.Uncovered, q)
	}
	return res
}

func assertContains(t *testing.T, sql string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(sql, w) {
			t.Errorf("SQL 缺少 %q\n  got: %s", w, sql)
		}
	}
}

// TestSeededSalesModel_Coverage 电商销售模型：营收/客单价按客户维度分组，
// 事实表 orders 经 to-one 关系 LEFT JOIN customers，扇出安全、口径钉死。
func TestSeededSalesModel_Coverage(t *testing.T) {
	b := getSeeded(t, "电商销售")

	// 营收 by 城市：跨表（orders 事实 + customers 维），治理口径 SUM(orders.pay_amount)。
	res := mustBuild(t, b, metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"城市"}})
	assertContains(t, res.SQL,
		"SUM(orders.pay_amount) AS `营收`",
		"customers.city AS `城市`",
		"FROM `orders` orders",
		"`customers` customers ON orders.customer_id = customers.id",
		"GROUP BY customers.city",
	)

	// 客单价 by 支付方式：复合表达式口径固定，单表 orders。
	res2 := mustBuild(t, b, metricsql.Query{Metrics: []string{"客单价"}, Dimensions: []string{"支付方式"}})
	assertContains(t, res2.SQL,
		"SUM(orders.pay_amount)/NULLIF(COUNT(orders.id),0) AS `客单价`",
		"orders.payment_method AS `支付方式`",
	)

	// 口径一致性：同义词 gmv 与规范名 营收 生成「同一条」SQL（治理路径核心保证）。
	viaSynonym := mustBuild(t, b, metricsql.Query{Metrics: []string{"gmv"}, Dimensions: []string{"城市"}})
	if viaSynonym.SQL != res.SQL {
		t.Fatalf("口径漂移：同义词 gmv 与 营收 生成不同 SQL\n  canonical: %s\n  synonym:   %s", res.SQL, viaSynonym.SQL)
	}
}

// TestSeededProductModel_Coverage 商品销售模型（明细粒度）：销量/销售额按品类分组，
// order_items 经 to-one 链 LEFT JOIN products、products→categories，扇出安全。
func TestSeededProductModel_Coverage(t *testing.T) {
	b := getSeeded(t, "商品销售")

	// 销量 + 商品销售额 by 品类：跨三表（明细→商品→品类）。
	res := mustBuild(t, b, metricsql.Query{
		Metrics:    []string{"销量", "商品销售额"},
		Dimensions: []string{"品类"},
	})
	assertContains(t, res.SQL,
		"SUM(order_items.quantity) AS `销量`",
		"SUM(order_items.subtotal) AS `商品销售额`",
		"categories.id AS `品类`",
		"FROM `order_items` order_items",
		"`products` products ON order_items.product_id = products.id",
		"`categories` categories ON products.category_id = categories.id",
		"GROUP BY categories.id",
	)

	// 均价 by 品牌：复合口径，商品维分组。
	res2 := mustBuild(t, b, metricsql.Query{Metrics: []string{"均价"}, Dimensions: []string{"品牌"}})
	assertContains(t, res2.SQL,
		"SUM(order_items.subtotal)/NULLIF(SUM(order_items.quantity),0) AS `均价`",
		"products.brand AS `品牌`",
	)
}

// TestSeededModel_UncoveredFallback 未建模的名称应 covered=false，触发词法回退。
func TestSeededModel_UncoveredFallback(t *testing.T) {
	b := getSeeded(t, "电商销售")
	res, err := metricsql.Build(b, metricsql.Query{Metrics: []string{"未定义业务指标"}, Dimensions: []string{"城市"}})
	if err != nil {
		t.Fatalf("metricsql.Build: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("期望未覆盖（未定义业务指标未建模），实际 covered=true：%s", res.SQL)
	}
}
