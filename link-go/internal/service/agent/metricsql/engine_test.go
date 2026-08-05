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

// valueMapBundle 单表订单，订单状态维度带 ValueMap（业务标签→物理枚举）。
func valueMapBundle() *semantic.ModelBundle {
	b := singleTableBundle()
	b.Dimensions = append(b.Dimensions, &semantic.Dimension{
		ID: "d_status", ModelID: "m1", LogicalTableID: "lt_order", Name: "订单状态", Expr: "status",
		Synonyms: []string{"状态"},
		ValueMap: map[string]string{"已完成": "completed", "已取消": "cancelled"},
	})
	return b
}

// TestBuild_FilterValueMapped 命中 ValueMap 的业务标签值被翻译为物理枚举（Bug C 修复）：
// status='已完成' 不再原样进 SQL（匹配 0 行致 NULL），而是翻译成 'completed'。
func TestBuild_FilterValueMapped(t *testing.T) {
	b := valueMapBundle()
	res, err := Build(b, Query{
		Metrics: []string{"金额"},
		Filters: []Filter{{Field: "订单状态", Op: OpEq, Values: []string{"已完成"}}},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("expected covered, uncovered=%v", res.Coverage.Uncovered)
	}
	if !strings.Contains(res.SQL, "orders.status = 'completed'") {
		t.Errorf("业务值未翻译为物理枚举: %s", res.SQL)
	}
	if strings.Contains(res.SQL, "已完成") {
		t.Errorf("业务标签值仍原样进入 SQL: %s", res.SQL)
	}
}

// TestBuild_FilterValueUnmappedPassthrough 未命中 ValueMap 的值原样透传
// （值本就是物理枚举 'cancelled'，或该维度无需映射），大小写/空白不敏感命中。
func TestBuild_FilterValueUnmappedPassthrough(t *testing.T) {
	b := valueMapBundle()
	res, err := Build(b, Query{
		Metrics: []string{"金额"},
		// 'shipped' 未在 ValueMap → 原样透传；' 已取消 ' 带空白仍命中 → cancelled。
		Filters: []Filter{{Field: "订单状态", Op: OpIn, Values: []string{"shipped", " 已取消 "}}},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(res.SQL, "'shipped'") {
		t.Errorf("未映射的物理值应原样透传: %s", res.SQL)
	}
	if !strings.Contains(res.SQL, "'cancelled'") {
		t.Errorf("带空白的业务值应归一化后命中映射: %s", res.SQL)
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

// ========================================
// ② 派生/复合指标：指标引用指标，递归展开为治理口径 SQL
// ========================================

// derivedBundle：order_items（明细，含数量/小计）↔ products（含成本），并定义派生指标链：
//
//	商品销售额 = SUM(order_items.subtotal)
//	商品成本   = SUM(order_items.quantity * products.cost)   ← 跨表
//	毛利       = {商品销售额} - {商品成本}
//	毛利率     = {毛利} / NULLIF({商品销售额}, 0)
func derivedBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "md", Name: "商品销售", Version: 1, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt_item", ModelID: "md", Name: "order_items", PhysicalTable: "t_order_item"},
			{ID: "lt_prod", ModelID: "md", Name: "products", PhysicalTable: "t_product"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d_brand", ModelID: "md", LogicalTableID: "lt_prod", Name: "品牌", Expr: "brand", DataType: "string"},
		},
		Measures: []*semantic.Measure{
			{ID: "ms_sub", ModelID: "md", LogicalTableID: "lt_item", Name: "小计", Expr: "subtotal", Aggregation: semantic.AggSum},
			{ID: "ms_qty", ModelID: "md", LogicalTableID: "lt_item", Name: "数量", Expr: "quantity", Aggregation: semantic.AggSum},
		},
		Metrics: []*semantic.Metric{
			{ID: "mt_sales", ModelID: "md", Name: "商品销售额", Expr: "SUM(order_items.subtotal)"},
			{ID: "mt_cogs", ModelID: "md", Name: "商品成本", Expr: "SUM(order_items.quantity * products.cost)"},
			{ID: "mt_gp", ModelID: "md", Name: "毛利", Expr: "{商品销售额} - {商品成本}"},
			{ID: "mt_gpm", ModelID: "md", Name: "毛利率", Expr: "{毛利} / NULLIF({商品销售额}, 0)", Format: "percent"},
		},
		Relations: []*semantic.Relation{
			{ID: "r_ip", ModelID: "md", LeftTableID: "lt_item", RightTableID: "lt_prod", JoinType: semantic.JoinLeft, JoinCondition: "order_items.product_id = products.id"},
		},
	}
}

func TestBuild_DerivedMetricRecursiveExpansion(t *testing.T) {
	b := derivedBundle()
	res, err := Build(b, Query{Metrics: []string{"毛利率"}, Dimensions: []string{"品牌"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("expected covered, uncovered=%v notes=%v", res.Coverage.Uncovered, res.Notes)
	}
	sql := res.SQL
	// 派生指标被完全展开成底层度量表达式（无残留 {ref}），跨表 JOIN 被规划。
	for _, want := range []string{
		"SUM(order_items.subtotal)",
		"SUM(order_items.quantity * products.cost)",
		"NULLIF",
		"AS `毛利率`",
		"FROM `t_order_item` order_items",
		"LEFT JOIN `t_product` products ON order_items.product_id = products.id",
		"products.brand AS `品牌`",
		"GROUP BY products.brand",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q\n  got: %s", want, sql)
		}
	}
	if strings.Contains(sql, "{") || strings.Contains(sql, "}") {
		t.Errorf("expanded SQL must not contain unresolved {ref}: %s", sql)
	}
}

func TestBuild_DerivedMetricCycleDetected(t *testing.T) {
	b := derivedBundle()
	// 注入一对相互引用的指标构成环：A→B→A。
	b.Metrics = append(b.Metrics,
		&semantic.Metric{ID: "mt_a", ModelID: "md", Name: "指标A", Expr: "{指标B} + 1"},
		&semantic.Metric{ID: "mt_b", ModelID: "md", Name: "指标B", Expr: "{指标A} + 2"},
	)
	res, err := Build(b, Query{Metrics: []string{"指标A"}, Dimensions: []string{"品牌"}})
	if err != nil {
		t.Fatalf("cycle should degrade to uncovered, not hard error: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("cyclic metric must not be covered")
	}
	if res.SQL != "" {
		t.Errorf("uncovered must not emit SQL, got: %s", res.SQL)
	}
	joined := strings.Join(res.Notes, "；")
	if !strings.Contains(joined, "循环引用") {
		t.Errorf("expected cycle note, got notes=%v", res.Notes)
	}
}

func TestBuild_DerivedMetricUnknownRef(t *testing.T) {
	b := derivedBundle()
	b.Metrics = append(b.Metrics, &semantic.Metric{ID: "mt_x", ModelID: "md", Name: "坏指标", Expr: "{不存在的指标} * 2"})
	res, err := Build(b, Query{Metrics: []string{"坏指标"}, Dimensions: []string{"品牌"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("metric with unknown ref must not be covered")
	}
	if len(res.Coverage.Uncovered) == 0 || res.Coverage.Uncovered[0] != "坏指标" {
		t.Errorf("expected 坏指标 uncovered, got %v", res.Coverage.Uncovered)
	}
}

// ========================================
// ③ 时间智能：TimeGrain + 方言感知的时间分桶
// ========================================

// timeBundle：单表订单，含时间维度（下单日期，datetime 原始列）与营收指标。
func timeBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "mt", Name: "sales", Version: 1, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt_order", ModelID: "mt", Name: "orders", PhysicalTable: "t_order"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d_status", ModelID: "mt", LogicalTableID: "lt_order", Name: "状态", Expr: "status", DataType: "string"},
			{ID: "d_date", ModelID: "mt", LogicalTableID: "lt_order", Name: "下单日期", Expr: "orders.created_at", DataType: "datetime"},
		},
		Measures: []*semantic.Measure{
			{ID: "ms_amt", ModelID: "mt", LogicalTableID: "lt_order", Name: "金额", Expr: "amount", Aggregation: semantic.AggSum},
		},
		Metrics: []*semantic.Metric{
			{ID: "mt_rev", ModelID: "mt", Name: "营收", Expr: "SUM(orders.amount)"},
		},
	}
}

func TestBuild_TimeGrainAutoInjectsPrimaryTimeDim(t *testing.T) {
	b := timeBundle()
	// 只给指标 + 粒度，不显式选时间维度 → 自动注入主时间维度并按月分桶。
	res, err := Build(b, Query{Metrics: []string{"营收"}, TimeGrain: GrainMonth})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Coverage.Covered {
		t.Fatalf("expected covered, uncovered=%v notes=%v", res.Coverage.Uncovered, res.Notes)
	}
	bucket := "DATE_FORMAT(orders.created_at, '%Y-%m')"
	if !strings.Contains(res.SQL, bucket+" AS `下单日期`") {
		t.Errorf("SQL missing month bucket select: %s", res.SQL)
	}
	if !strings.Contains(res.SQL, "GROUP BY "+bucket) {
		t.Errorf("SQL missing month bucket group by: %s", res.SQL)
	}
}

func TestBuild_TimeGrainSelectedDimQuarterMySQL(t *testing.T) {
	b := timeBundle()
	res, err := Build(b, Query{Metrics: []string{"营收"}, Dimensions: []string{"下单日期"}, TimeGrain: GrainQuarter})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "CONCAT(YEAR(orders.created_at), '-Q', QUARTER(orders.created_at))"
	if !strings.Contains(res.SQL, want) {
		t.Errorf("SQL missing quarter bucket %q\n  got: %s", want, res.SQL)
	}
}

func TestBuild_TimeGrainPostgresDialect(t *testing.T) {
	b := timeBundle()
	res, err := Build(b, Query{Metrics: []string{"营收"}, Dimensions: []string{"下单日期"}, TimeGrain: GrainMonth, Dialect: DialectPostgres})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(res.SQL, "TO_CHAR(orders.created_at, 'YYYY-MM')") {
		t.Errorf("SQL missing postgres month bucket: %s", res.SQL)
	}
	if strings.Contains(res.SQL, "DATE_FORMAT") {
		t.Errorf("postgres dialect must not emit MySQL DATE_FORMAT: %s", res.SQL)
	}
}

// PostgreSQL 方言下标识符须用双引号（“ 是 PG 语法错误），且时间分桶用 TO_CHAR。
func TestBuild_PostgresQuotesIdentifiersWithDoubleQuote(t *testing.T) {
	b := timeBundle()
	res, err := Build(b, Query{Metrics: []string{"营收"}, Dimensions: []string{"下单日期"}, TimeGrain: GrainMonth, Dialect: DialectPostgres})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(res.SQL, "`") {
		t.Errorf("postgres dialect must not emit backtick identifiers: %s", res.SQL)
	}
	if !strings.Contains(res.SQL, `"t_order" orders`) {
		t.Errorf("postgres FROM must double-quote table name: %s", res.SQL)
	}
	if !strings.Contains(res.SQL, `AS "下单日期"`) {
		t.Errorf("postgres column alias must be double-quoted: %s", res.SQL)
	}
}

// 日粒度也输出字符串桶（DATE_FORMAT），与其它粒度列类型一致，不再用 DATE()。
func TestBuild_TimeGrainDayEmitsStringBucketMySQL(t *testing.T) {
	b := timeBundle()
	res, err := Build(b, Query{Metrics: []string{"营收"}, Dimensions: []string{"下单日期"}, TimeGrain: GrainDay})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(res.SQL, "DATE_FORMAT(orders.created_at, '%Y-%m-%d')") {
		t.Errorf("day grain must emit string bucket: %s", res.SQL)
	}
	if strings.Contains(res.SQL, "DATE(orders.created_at)") {
		t.Errorf("day grain must not emit bare DATE(): %s", res.SQL)
	}
}

// 请求了时间上卷但模型无任何时间维度 → 标记未覆盖触发回退，而非静默返回全时段聚合。
func TestBuild_TimeGrainWithoutTimeDimFallsBack(t *testing.T) {
	b := &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "mn", Name: "notime", Version: 1, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt", ModelID: "mn", Name: "orders", PhysicalTable: "t_order"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d", ModelID: "mn", LogicalTableID: "lt", Name: "状态", Expr: "status", DataType: "string"},
		},
		Metrics: []*semantic.Metric{
			{ID: "m", ModelID: "mn", Name: "营收", Expr: "SUM(orders.amount)"},
		},
	}
	res, err := Build(b, Query{Metrics: []string{"营收"}, TimeGrain: GrainMonth})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("grain without any time dimension must fall back, got covered SQL: %s", res.SQL)
	}
	found := false
	for _, u := range res.Coverage.Uncovered {
		if strings.Contains(u, "time_grain(month)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected time_grain(month) uncovered marker, got %v", res.Coverage.Uncovered)
	}
}

func TestBuild_InvalidTimeGrainFallsBack(t *testing.T) {
	b := timeBundle()
	res, err := Build(b, Query{Metrics: []string{"营收"}, TimeGrain: TimeGrain("hour")})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Coverage.Covered {
		t.Fatalf("invalid grain must trigger fallback")
	}
	found := false
	for _, u := range res.Coverage.Uncovered {
		if strings.Contains(u, "time_grain(hour)") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected time_grain(hour) uncovered, got %v", res.Coverage.Uncovered)
	}
}
