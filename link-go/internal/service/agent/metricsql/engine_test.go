package metricsql

import (
	"strings"
	"testing"

	"link/internal/model/semantic"
)

// singleTableBundle 构造一个单逻辑表（订单）的语义模型：度量 金额(sum)、指标 营收，
// 维度 区域（含同义词 大区）。
func singleTableBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "m1", Name: "sales", Version: 3, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt_order", ModelID: "m1", Name: "orders", PhysicalTable: "t_order"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d_region", ModelID: "m1", LogicalTableID: "lt_order", Name: "区域", Expr: "region", Synonyms: []string{"大区"}},
		},
		Measures: []*semantic.Measure{
			{ID: "ms_amt", ModelID: "m1", LogicalTableID: "lt_order", Name: "金额", Expr: "amount", Aggregation: semantic.AggSum},
		},
		Metrics: []*semantic.Metric{
			{ID: "mt_rev", ModelID: "m1", Name: "营收", Expr: "SUM(orders.amount)", Caliber: "已支付订单金额之和", Synonyms: []string{"revenue"}},
		},
	}
}

func TestBuild_SingleTableMetricAndDimension(t *testing.T) {
	b := singleTableBundle()
	res, err := Build(b, Query{Metrics: []string{"金额"}, Dimensions: []string{"区域"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("expected covered, got uncovered=%v", res.Coverage.Uncovered)
	}
	sql := res.SQL
	for _, want := range []string{"SELECT", "SUM(orders.amount)", "AS `金额`", "orders.region AS `区域`", "FROM `t_order` orders", "GROUP BY orders.region"} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q\n  got: %s", want, sql)
		}
	}
}

func TestBuild_SynonymResolution(t *testing.T) {
	b := singleTableBundle()
	// 用同义词「大区」「revenue」
	res, err := Build(b, Query{Metrics: []string{"revenue"}, Dimensions: []string{"大区"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("synonyms should resolve, uncovered=%v", res.Coverage.Uncovered)
	}
	if !strings.Contains(res.SQL, "SUM(orders.amount) AS `营收`") {
		t.Errorf("metric synonym not resolved to governed expr: %s", res.SQL)
	}
}

func TestBuild_FilterAndOrderAndLimit(t *testing.T) {
	b := singleTableBundle()
	res, err := Build(b, Query{
		Metrics:    []string{"金额"},
		Dimensions: []string{"区域"},
		Filters:    []Filter{{Field: "区域", Op: OpEq, Values: []string{"华东"}}},
		OrderBy:    []OrderKey{{Field: "金额", Desc: true}},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	sql := res.SQL
	for _, want := range []string{"WHERE orders.region = '华东'", "ORDER BY SUM(orders.amount) DESC", "LIMIT 10"} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q\n  got: %s", want, sql)
		}
	}
}

func TestBuild_UncoveredTriggersFallback(t *testing.T) {
	b := singleTableBundle()
	res, err := Build(b, Query{Metrics: []string{"利润"}, Dimensions: []string{"区域"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("expected not covered")
	}
	if res.SQL != "" {
		t.Errorf("uncovered must not emit SQL, got: %s", res.SQL)
	}
	if len(res.Coverage.Uncovered) == 0 || res.Coverage.Uncovered[0] != "利润" {
		t.Errorf("expected 利润 in uncovered, got %v", res.Coverage.Uncovered)
	}
}

func TestBuild_FilterInjectionEscaped(t *testing.T) {
	b := singleTableBundle()
	res, err := Build(b, Query{
		Metrics:    []string{"金额"},
		Dimensions: []string{"区域"},
		Filters:    []Filter{{Field: "区域", Op: OpEq, Values: []string{"x' OR '1'='1"}}},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(res.SQL, "'x'' OR ''1''=''1'") {
		t.Errorf("single quotes not escaped: %s", res.SQL)
	}
}

// twoTableBundle 构造 订单←→用户 两表并定义关系，验证 JOIN 规划。
func twoTableBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "m2", Name: "sales2", Version: 1, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt_order", ModelID: "m2", Name: "orders", PhysicalTable: "t_order"},
			{ID: "lt_user", ModelID: "m2", Name: "users", PhysicalTable: "t_user"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d_city", ModelID: "m2", LogicalTableID: "lt_user", Name: "城市", Expr: "city"},
		},
		Measures: []*semantic.Measure{
			{ID: "ms_amt", ModelID: "m2", LogicalTableID: "lt_order", Name: "金额", Expr: "amount", Aggregation: semantic.AggSum},
		},
		Relations: []*semantic.Relation{
			{ID: "r1", ModelID: "m2", LeftTableID: "lt_order", RightTableID: "lt_user", JoinType: semantic.JoinLeft, JoinCondition: "orders.user_id = users.id"},
		},
	}
}

func TestBuild_JoinAcrossTables(t *testing.T) {
	b := twoTableBundle()
	res, err := Build(b, Query{Metrics: []string{"金额"}, Dimensions: []string{"城市"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("expected covered, uncovered=%v notes=%v", res.Coverage.Uncovered, res.Notes)
	}
	sql := res.SQL
	for _, want := range []string{"FROM `t_order` orders", "LEFT JOIN `t_user` users ON orders.user_id = users.id", "users.city AS `城市`", "SUM(orders.amount)"} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q\n  got: %s", want, sql)
		}
	}
}
