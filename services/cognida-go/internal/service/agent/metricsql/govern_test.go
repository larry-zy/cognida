package metricsql

import (
	"strings"
	"testing"

	"cognida/internal/model/semantic"
)

// hasWarn 判断告警集合里是否含指定 Code。
func hasWarn(ws []Warning, code string) bool {
	for _, w := range ws {
		if w.Code == code {
			return true
		}
	}
	return false
}

// governedBundle 是「治理良好」的三表模型（对齐修复后的 productBundle 形状）：
// 事实表 order_items（有主键+度量），维表 products/categories 各带绑主键的身份维度 + name 展示维度，
// 全部 JOIN 命中被连表主键。ValidateBundle 应对其零告警。
func governedBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "mg", Name: "商品销售", Version: 1, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt_item", ModelID: "mg", Name: "order_items", PhysicalTable: "t_item", PrimaryKey: "id"},
			{ID: "lt_prod", ModelID: "mg", Name: "products", PhysicalTable: "t_prod", PrimaryKey: "id"},
			{ID: "lt_cat", ModelID: "mg", Name: "categories", PhysicalTable: "t_cat", PrimaryKey: "id"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d_pid", ModelID: "mg", LogicalTableID: "lt_prod", Name: "商品", Expr: "id", DataType: "int"},
			{ID: "d_pname", ModelID: "mg", LogicalTableID: "lt_prod", Name: "商品名称", Expr: "name", DataType: "string"},
			{ID: "d_cid", ModelID: "mg", LogicalTableID: "lt_cat", Name: "品类", Expr: "id", DataType: "int"},
			{ID: "d_cname", ModelID: "mg", LogicalTableID: "lt_cat", Name: "品类名称", Expr: "name", DataType: "string"},
		},
		Measures: []*semantic.Measure{
			{ID: "ms_sub", ModelID: "mg", LogicalTableID: "lt_item", Name: "小计", Expr: "subtotal", Aggregation: semantic.AggSum},
		},
		Relations: []*semantic.Relation{
			{ID: "r_ip", ModelID: "mg", LeftTableID: "lt_item", RightTableID: "lt_prod", JoinType: semantic.JoinLeft, JoinCondition: "order_items.product_id = products.id"},
			{ID: "r_pc", ModelID: "mg", LeftTableID: "lt_prod", RightTableID: "lt_cat", JoinType: semantic.JoinLeft, JoinCondition: "products.category_id = categories.id"},
		},
	}
}

func TestValidateBundle_GovernedModelIsClean(t *testing.T) {
	ws := ValidateBundle(governedBundle())
	if len(ws) != 0 {
		t.Fatalf("expected clean, got %d warnings: %v", len(ws), ws)
	}
}

// D1：维表按 name 展示列分组但缺绑主键的身份维度 → GRAIN_IDENTITY（原始 Top15 虚高 bug 形状）。
func TestValidateBundle_GrainIdentityFlaggedWhenNameOnly(t *testing.T) {
	b := governedBundle()
	// 移除品类身份维度（d_cid），只留「品类名称」(name) → categories 失去主键身份维度。
	kept := b.Dimensions[:0]
	for _, d := range b.Dimensions {
		if d.ID != "d_cid" {
			kept = append(kept, d)
		}
	}
	b.Dimensions = kept
	ws := ValidateBundle(b)
	if !hasWarn(ws, "GRAIN_IDENTITY") {
		t.Fatalf("expected GRAIN_IDENTITY, got: %v", ws)
	}
	for _, w := range ws {
		if w.Code == "GRAIN_IDENTITY" && !strings.Contains(w.Message, "categories") {
			t.Errorf("GRAIN_IDENTITY should name the table categories, got: %s", w.Message)
		}
	}
}

// D1 零误报：把 name 换成类别列（如 city），不应告警——按城市折叠维表是合法建模。
func TestValidateBundle_CategoricalDimNotFlagged(t *testing.T) {
	b := governedBundle()
	for _, d := range b.Dimensions {
		if d.ID == "d_cname" {
			d.Name, d.Expr = "地区", "region" // 非 name/title 的类别列
		}
	}
	// 同时移除品类身份维度，确认「无身份维度 + 类别列」也不误报。
	kept := b.Dimensions[:0]
	for _, d := range b.Dimensions {
		if d.ID != "d_cid" {
			kept = append(kept, d)
		}
	}
	b.Dimensions = kept
	if hasWarn(ValidateBundle(b), "GRAIN_IDENTITY") {
		t.Fatalf("categorical dim must not trigger GRAIN_IDENTITY: %v", ValidateBundle(b))
	}
}

// D2：JOIN 未命中任一端主键（两端主键均声明）→ FANOUT_RISK。
func TestValidateBundle_FanoutRiskOnNonKeyedJoin(t *testing.T) {
	b := governedBundle()
	// 把 明细→商品 改成按 region 连（都不是主键）。
	for _, r := range b.Relations {
		if r.ID == "r_ip" {
			r.JoinCondition = "order_items.region = products.region"
		}
	}
	if !hasWarn(ValidateBundle(b), "FANOUT_RISK") {
		t.Fatalf("expected FANOUT_RISK on non-keyed join, got: %v", ValidateBundle(b))
	}
}

// D2 零误报：星型 fact.fk = dim.pk（命中一端主键）不告警。
func TestValidateBundle_StarJoinNoFanoutWarn(t *testing.T) {
	if hasWarn(ValidateBundle(governedBundle()), "FANOUT_RISK") {
		t.Fatal("star join fact.fk=dim.pk must not trigger FANOUT_RISK")
	}
}

// D3：事实表（有度量）未声明主键 → FACT_NO_PK。
func TestValidateBundle_FactWithoutPKFlagged(t *testing.T) {
	b := governedBundle()
	for _, tb := range b.LogicalTables {
		if tb.ID == "lt_item" {
			tb.PrimaryKey = ""
		}
	}
	if !hasWarn(ValidateBundle(b), "FACT_NO_PK") {
		t.Fatalf("expected FACT_NO_PK, got: %v", ValidateBundle(b))
	}
}

// D3 基表选取：候选含维表（别名字典序更小）与事实表（有度量）时，preferFact 必须选事实表，
// 而非别名字典序偶然更小的维表——避免维表被误当基表。
func TestPreferFact_PicksFactOverLexicographicallySmallerDim(t *testing.T) {
	b := &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "mp", Name: "p", Version: 1, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			// 维表 aaa：别名 "aaa"（字典序更小），无度量。
			{ID: "lt_dim", ModelID: "mp", Name: "aaa", PhysicalTable: "t_aaa", PrimaryKey: "id"},
			// 事实表 zzz：别名 "zzz"（字典序更大），有度量。
			{ID: "lt_fact", ModelID: "mp", Name: "zzz", PhysicalTable: "t_zzz", PrimaryKey: "id"},
		},
		Measures: []*semantic.Measure{
			{ID: "ms", ModelID: "mp", LogicalTableID: "lt_fact", Name: "额", Expr: "amt", Aggregation: semantic.AggSum},
		},
	}
	idx := newIndex(b)
	got := idx.preferFact(map[string]struct{}{"lt_dim": {}, "lt_fact": {}})
	if got != "lt_fact" {
		t.Fatalf("preferFact should pick fact table lt_fact, got %q", got)
	}
}
