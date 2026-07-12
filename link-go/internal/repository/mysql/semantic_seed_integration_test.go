//go:build integration
// +build integration

// Package mysql: cmd/seed-semantic 冷启动种子的端到端冒烟测试。
//
// 验证「跑过 seed-semantic 后」，tenant=1 的两套语义模型（电商销售 / 商品销售）
// 经指标引擎能稳定生成「治理口径 SQL」且 covered=true —— 即 semantic_query 走主路径
// 而非回退词法 NL2SQL。这是 P0「让 semantic_query 不再总是回退」的直接回归防线。
//
// 前置：先跑 seed（连的是 link 库）：
//
//	cd link-go && set -a && source .env && set +a && go run ./cmd/seed-semantic
//
// 再针对同一 link 库运行：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestSeeded -v
//
// 未 seed（找不到模型）则 Skip，不误报失败。
package mysql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"link/internal/model/semantic"
	"link/internal/service/agent/metricsql"
)

// seedTenantID 与 cmd/seed-semantic 的 seedTenant 一致（dev 用户）。
const seedTenantID = int64(1)

// getSeeded 读取已 seed 的生效模型；未 seed 则跳过测试。
func getSeeded(t *testing.T, name string) *semantic.ModelBundle {
	t.Helper()
	repo := NewSemanticRepository(newIntegrationDB(t))
	b, err := repo.GetActiveModel(context.Background(), seedTenantID, name)
	if err != nil {
		if errors.Is(err, semantic.ErrModelNotFound) {
			t.Skipf("语义模型 %q 未 seed，先跑 `go run ./cmd/seed-semantic`", name)
		}
		t.Fatalf("GetActiveModel(%q): %v", name, err)
	}
	return b
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
		"categories.name AS `品类`",
		"FROM `order_items` order_items",
		"`products` products ON order_items.product_id = products.id",
		"`categories` categories ON products.category_id = categories.id",
		"GROUP BY categories.name",
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
	res, err := metricsql.Build(b, metricsql.Query{Metrics: []string{"退货率"}, Dimensions: []string{"城市"}})
	if err != nil {
		t.Fatalf("metricsql.Build: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("期望未覆盖（退货率未建模），实际 covered=true：%s", res.SQL)
	}
}
