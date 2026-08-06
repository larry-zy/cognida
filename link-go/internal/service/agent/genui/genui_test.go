package genui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// 真实形态样本：sql_execute 的月度销售额信封（samples 为有界样本，全量存 Result Store）。
const sampleSQL = `{
  "result_id": "rs_demo",
  "columns": ["month", "revenue"],
  "samples": [
    {"month": "2026-01", "revenue": 120},
    {"month": "2026-02", "revenue": 135},
    {"month": "2026-03", "revenue": 150},
    {"month": "2026-04", "revenue": 168}
  ],
  "row_count": 4,
  "executed_sql": "SELECT month, revenue FROM sales ORDER BY month LIMIT 100"
}`

// 真实形态样本：data_analysis trend 输出（对齐 trend_tool.py 的返回结构）。
const sampleTrend = `{
  "analysis_type": "trend",
  "success": true,
  "data": {
    "value_col": "revenue",
    "time_col": "month",
    "row_count": 4,
    "trend": {"direction": "up", "strength": "strong", "slope": 15.6, "r_squared": 0.997, "p_value": 0.001},
    "forecast": [{"value": 184.2}, {"value": 199.8}],
    "growth": {"period_over_period": 0.12, "cagr": 0.077}
  }
}`

func TestAssembleDataModel_FusesRealSQLAndAnalysis(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)
	if dm == nil {
		t.Fatal("expected non-nil datamodel")
	}
	// 表格来自真实行集
	if dm.Table == nil || len(dm.Table.Rows) != 4 {
		t.Fatalf("table rows = %v, want 4", dm.Table)
	}
	// 指标来自真实分析，数字未被篡改
	if got := dm.Metrics["斜率"]; got != 15.6 {
		t.Errorf("metrics 斜率 = %v, want 15.6", got)
	}
	if got := dm.Metrics["CAGR"]; got != 0.077 {
		t.Errorf("metrics CAGR = %v, want 0.077", got)
	}
	// 序列：actual 取自行集，forecast 取自分析
	if dm.Series == nil || len(dm.Series.Actual) != 4 {
		t.Fatalf("series actual = %v, want 4 points", dm.Series)
	}
	if dm.Series.Actual[0] != 120 || dm.Series.Actual[3] != 168 {
		t.Errorf("series actual = %v, want [120..168]", dm.Series.Actual)
	}
	if len(dm.Series.Forecast) != 2 || dm.Series.Forecast[0] != 184.2 {
		t.Errorf("series forecast = %v, want [184.2, 199.8]", dm.Series.Forecast)
	}
	if len(dm.Series.Labels) != 4 {
		t.Errorf("series labels = %v, want 4", dm.Series.Labels)
	}
}

func TestAssembleDataModel_NoRowsReturnsNil(t *testing.T) {
	if dm := AssembleDataModel(`{"columns":["a"],"samples":[],"row_count":0}`, ""); dm != nil {
		t.Errorf("empty samples should yield nil datamodel, got %v", dm)
	}
	if dm := AssembleDataModel("", ""); dm != nil {
		t.Errorf("empty sql should yield nil datamodel")
	}
}

// 全量结果超过样本数时，genUI 以有界样本渲染，并在 Meta 暴露 result_id + truncated，
// 作为前端/后续 Phase 3 按引用回放完整结果集的挂钩。
func TestAssembleDataModel_TruncatedSurfacesResultID(t *testing.T) {
	const bounded = `{
	  "result_id": "rs_big",
	  "columns": ["month", "revenue"],
	  "samples": [
	    {"month": "2026-01", "revenue": 120},
	    {"month": "2026-02", "revenue": 135}
	  ],
	  "row_count": 5000
	}`
	dm := AssembleDataModel(bounded, "")
	if dm == nil {
		t.Fatal("expected non-nil datamodel")
	}
	if dm.Meta["row_count"] != 5000 {
		t.Errorf("row_count = %v, want 5000 (真实全量)", dm.Meta["row_count"])
	}
	if dm.Meta["truncated"] != true {
		t.Errorf("truncated = %v, want true", dm.Meta["truncated"])
	}
	if dm.Meta["result_id"] != "rs_big" {
		t.Errorf("result_id = %v, want rs_big", dm.Meta["result_id"])
	}
	// 未截断时不应携带 result_id / truncated
	full := AssembleDataModel(sampleSQL, "")
	if _, ok := full.Meta["truncated"]; ok {
		t.Error("未截断结果不应标记 truncated")
	}
}

func TestTemplateCompose_ProducesValidSpec(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)
	spec := TemplateCompose(dm, "各月营收趋势如何？")
	if spec.GenMode != GenModeTemplate {
		t.Errorf("genMode = %s, want template", spec.GenMode)
	}
	if err := Validate(spec); err != nil {
		t.Fatalf("template spec failed validation: %v", err)
	}
	// 模板应包含图表与表格
	if !hasType(spec, CompLineChart) {
		t.Error("template spec missing LineChart")
	}
	if !hasType(spec, CompTable) {
		t.Error("template spec missing Table")
	}
	if !hasType(spec, CompMetricCard) {
		t.Error("template spec missing MetricCard")
	}
	// 富化：概览 Callout 与脚注 Text 应始终出现，且 Callout 置于根容器首位。
	if !hasType(spec, CompCallout) {
		t.Error("template spec missing Callout summary")
	}
	if !hasType(spec, CompText) {
		t.Error("template spec missing Text footnote")
	}
	if root := findNode(spec, RootID); root == nil {
		t.Fatal("template spec missing root node")
	} else if len(root.Children) == 0 || root.Children[0] != "summary" {
		t.Errorf("root children = %v, want summary first", root.Children)
	}
}

// findNode 按 id 取组件节点（测试辅助）。
func findNode(spec *UISpec, id string) *Component {
	for i := range spec.Components {
		if spec.Components[i].ID == id {
			return &spec.Components[i]
		}
	}
	return nil
}

// 分类样本：各地区销售额，标签为离散类目（非日期/非数值），应触发柱状图。
const sampleCategorical = `{
  "result_id": "rs_region",
  "columns": ["region", "revenue"],
  "samples": [
    {"region": "华东", "revenue": 320},
    {"region": "华南", "revenue": 280},
    {"region": "华北", "revenue": 210},
    {"region": "西南", "revenue": 150}
  ],
  "row_count": 4,
  "executed_sql": "SELECT region, revenue FROM sales GROUP BY region"
}`

// 双数值列样本：广告投放 vs 销售额，用于散点图/相关性。
const sampleTwoNumeric = `{
  "result_id": "rs_corr",
  "columns": ["ad_spend", "sales"],
  "samples": [
    {"ad_spend": 10, "sales": 105},
    {"ad_spend": 20, "sales": 190},
    {"ad_spend": 30, "sales": 305},
    {"ad_spend": 40, "sales": 402}
  ],
  "row_count": 4,
  "executed_sql": "SELECT ad_spend, sales FROM campaigns"
}`

// correlation 分析输出（对齐 correlation_tool 的返回结构）。
const sampleCorrelation = `{
  "analysis_type": "correlation",
  "success": true,
  "data": {
    "significant_pairs": [{"a": "ad_spend", "b": "sales", "r": 0.999}]
  }
}`

func TestTemplateCompose_CategoricalUsesBarChart(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	if dm == nil {
		t.Fatal("expected non-nil datamodel")
	}
	spec := TemplateCompose(dm, "各地区营收对比")
	if err := Validate(spec); err != nil {
		t.Fatalf("template spec failed validation: %v", err)
	}
	if !hasType(spec, CompBarChart) {
		t.Error("categorical labels should yield BarChart")
	}
	if hasType(spec, CompLineChart) {
		t.Error("categorical labels should not yield LineChart")
	}
}

func TestBuildScatter_TwoNumericColumns(t *testing.T) {
	dm := AssembleDataModel(sampleTwoNumeric, "")
	if dm == nil {
		t.Fatal("expected non-nil datamodel")
	}
	if dm.Scatter == nil {
		t.Fatal("two numeric columns should yield scatter data")
	}
	if dm.Scatter.XLabel != "ad_spend" || dm.Scatter.YLabel != "sales" {
		t.Errorf("scatter labels = %q/%q, want ad_spend/sales", dm.Scatter.XLabel, dm.Scatter.YLabel)
	}
	if len(dm.Scatter.X) != 4 || dm.Scatter.X[0] != 10 || dm.Scatter.Y[3] != 402 {
		t.Errorf("scatter points = %v / %v, want real values", dm.Scatter.X, dm.Scatter.Y)
	}
	// 单数值列不应构造散点
	if noScatter := AssembleDataModel(sampleSQL, ""); noScatter.Scatter != nil {
		t.Error("single numeric column should not yield scatter")
	}
}

func TestTemplateCompose_CorrelationUsesScatter(t *testing.T) {
	dm := AssembleDataModel(sampleTwoNumeric, sampleCorrelation)
	if dm == nil {
		t.Fatal("expected non-nil datamodel")
	}
	spec := TemplateCompose(dm, "广告投放与销售额相关性")
	if err := Validate(spec); err != nil {
		t.Fatalf("template spec failed validation: %v", err)
	}
	if !hasType(spec, CompScatter) {
		t.Error("correlation analysis with scatter data should yield ScatterChart")
	}
	if hasType(spec, CompLineChart) || hasType(spec, CompBarChart) {
		t.Error("correlation should prefer ScatterChart over line/bar")
	}
}

func TestValidate_RejectsNonCatalogAndBadBinding(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)

	// 非白名单类型
	bad := &UISpec{
		DataModel:  dm,
		Components: []Component{{ID: RootID, Type: "Iframe"}},
	}
	if err := Validate(bad); err == nil {
		t.Error("expected error for non-catalog type")
	}

	// 绑定到不存在的路径
	badBind := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompColumn, Children: []string{"t"}},
			{ID: "t", Type: CompTable, Props: map[string]interface{}{"data": binding("/nope")}},
		},
	}
	if err := Validate(badBind); err == nil {
		t.Error("expected error for unresolved binding path")
	}

	// 多个 root
	multiRoot := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompColumn},
			{ID: RootID, Type: CompColumn},
		},
	}
	if err := Validate(multiRoot); err == nil {
		t.Error("expected error for duplicate root id")
	}
}

func TestResolvePointer(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)
	root, _ := toGeneric(dm)

	if v, ok := ResolvePointer(root, "/series/actual/0"); !ok || v.(float64) != 120 {
		t.Errorf("resolve /series/actual/0 = %v, %v", v, ok)
	}
	if _, ok := ResolvePointer(root, "/table"); !ok {
		t.Error("resolve /table should succeed")
	}
	if _, ok := ResolvePointer(root, "/missing"); ok {
		t.Error("resolve /missing should fail")
	}
}

// --- Level 2 LLM 路径 ---

// fakeLLM 返回预置文本，实现最小 LLM 接口。
type fakeLLM struct{ reply string }

func (f *fakeLLM) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(f.reply, nil), nil
}

func TestLLMCompose_ValidLayoutPasses(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)
	// LLM 只给布局 + {path}，不含任何数字，还故意用围栏包裹。
	reply := "```json\n" + `{"components":[
      {"id":"root","type":"Column","children":["chart","tbl"]},
      {"id":"chart","type":"LineChart","props":{"title":"营收","series":{"path":"/series"}}},
      {"id":"tbl","type":"Table","props":{"data":{"path":"/table"}}}
    ]}` + "\n```"
	spec, err := LLMCompose(context.Background(), &fakeLLM{reply: reply}, dm, "营收趋势")
	if err != nil {
		t.Fatalf("valid LLM layout should pass: %v", err)
	}
	if spec.GenMode != GenModeLLM {
		t.Errorf("genMode = %s, want llm", spec.GenMode)
	}
	// 数据仍来自真实 dataModel
	if spec.DataModel.Series.Actual[3] != 168 {
		t.Error("datamodel numbers must be preserved from real output")
	}
}

func TestLLMCompose_HallucinatedPathRejected(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)
	reply := `{"components":[
      {"id":"root","type":"Column","children":["x"]},
      {"id":"x","type":"MetricCard","props":{"label":"编造","value":{"path":"/metrics/不存在"}}}
    ]}`
	if _, err := LLMCompose(context.Background(), &fakeLLM{reply: reply}, dm, "q"); err == nil {
		t.Error("expected rejection of hallucinated binding path")
	}
}

func TestCompose_FallsBackToTemplateOnBadLLM(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)
	_ = dm
	SetModel(&fakeLLM{reply: "这不是 JSON"})
	defer SetModel(nil)

	spec := Compose(context.Background(), ComposeInput{
		Question: "营收趋势", SQLOutputs: []string{sampleSQL}, AnalysisOutputs: []string{sampleTrend},
	})
	if spec == nil {
		t.Fatal("compose should fall back, not return nil")
	}
	if spec.GenMode != GenModeTemplate {
		t.Errorf("bad LLM should fall back to template, got %s", spec.GenMode)
	}
	if err := Validate(spec); err != nil {
		t.Errorf("fallback spec must be valid: %v", err)
	}
}

// KPI 单行汇总结果（一行多数值列）：报告类查询的关键指标来源。
const sampleKPI = `{
  "result_id": "rs_kpi",
  "columns": ["GMV", "订单数", "客单价"],
  "samples": [{"GMV": 1250000, "订单数": 8600, "客单价": 145.3}],
  "row_count": 1,
  "executed_sql": "SELECT SUM(amount) GMV, COUNT(*) 订单数, AVG(amount) 客单价 FROM orders"
}`

// 报告场景：多段查询（单行 KPI 汇总 + 多行月度趋势）应融合成一份 DataModel——
// 多行结果作主表/序列，单行 KPI 的数值列派生为可绑定的标量指标。
func TestAssembleReportDataModel_MergesKPIScalarsAndTrendTable(t *testing.T) {
	dm := AssembleReportDataModel([]string{sampleKPI, sampleSQL}, []string{sampleTrend})
	if dm == nil {
		t.Fatal("expected non-nil report datamodel")
	}
	// 主表来自多行趋势结果（4 行），而非最后传入的顺序。
	if dm.Table == nil || len(dm.Table.Rows) != 4 {
		t.Fatalf("table rows = %v, want 4 (多行趋势结果)", dm.Table)
	}
	// KPI 单行结果的数值列派生为标量指标。
	if got := dm.Metrics["GMV"]; got != float64(1250000) {
		t.Errorf("metrics GMV = %v, want 1250000", got)
	}
	if got := dm.Metrics["客单价"]; got != 145.3 {
		t.Errorf("metrics 客单价 = %v, want 145.3", got)
	}
	// 分析语义指标同样并入。
	if got := dm.Metrics["斜率"]; got != 15.6 {
		t.Errorf("metrics 斜率 = %v, want 15.6 (分析指标应并入)", got)
	}
	// 序列取自主表行集。
	if dm.Series == nil || len(dm.Series.Actual) != 4 {
		t.Fatalf("series actual = %v, want 4", dm.Series)
	}
	// KPI 标量指标可被 MetricCard 绑定（标量校验通过）。
	spec := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompColumn, Children: []string{"gmv"}},
			{ID: "gmv", Type: CompMetricCard, Props: map[string]interface{}{"label": "GMV", "value": binding("/metrics/GMV")}},
		},
	}
	if err := Validate(spec); err != nil {
		t.Errorf("MetricCard 绑定 /metrics/GMV 应通过: %v", err)
	}
}

// 全空输入应返回 nil（无可展示数据）。
func TestAssembleReportDataModel_EmptyReturnsNil(t *testing.T) {
	if dm := AssembleReportDataModel(nil, nil); dm != nil {
		t.Errorf("empty inputs should yield nil, got %v", dm)
	}
	if dm := AssembleReportDataModel([]string{"", `{"samples":[]}`}, []string{`{"success":false}`}); dm != nil {
		t.Errorf("no usable results should yield nil, got %v", dm)
	}
}

// Layer A：MetricCard.value 绑到容器路径（/table）应被校验拒绝，绑到标量指标应通过。
func TestValidate_MetricCardValueMustBeScalar(t *testing.T) {
	dm := AssembleDataModel(sampleSQL, sampleTrend)

	bound := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompColumn, Children: []string{"m"}},
			{ID: "m", Type: CompMetricCard, Props: map[string]interface{}{"label": "x", "value": binding("/table")}},
		},
	}
	if err := Validate(bound); err == nil {
		t.Error("MetricCard.value 绑定到 /table（容器对象）应被拒绝")
	}

	ok := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompColumn, Children: []string{"m"}},
			{ID: "m", Type: CompMetricCard, Props: map[string]interface{}{"label": "斜率", "value": binding("/metrics/斜率")}},
		},
	}
	if err := Validate(ok); err != nil {
		t.Errorf("MetricCard 绑定标量指标 /metrics/斜率 应通过: %v", err)
	}
}

// 默认路径（无 chart_kind 提示）绝不产出饼图/漏斗图——保证既有模板输出零回归。
func TestTemplateCompose_DefaultDoesNotEmitPieOrFunnel(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	spec := TemplateCompose(dm, "各地区营收对比")
	if hasType(spec, CompPieChart) {
		t.Error("默认路径不应产出 pie_chart（需 Meta.chart_kind 显式 opt-in）")
	}
	if hasType(spec, CompFunnel) {
		t.Error("默认路径不应产出 funnel（需 Meta.chart_kind 显式 opt-in）")
	}
	// 默认仍走既有柱状图分支。
	if !hasType(spec, CompBarChart) {
		t.Error("默认分类数据仍应产出 BarChart")
	}
}

// opt-in：Meta.chart_kind=="pie" 时以饼图替代默认柱/线，绑定 /series 且校验通过。
func TestTemplateCompose_PieChartOptIn(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	dm.Meta["chart_kind"] = "pie"
	spec := TemplateCompose(dm, "各地区营收占比")
	if err := Validate(spec); err != nil {
		t.Fatalf("pie opt-in spec failed validation: %v", err)
	}
	if !hasType(spec, CompPieChart) {
		t.Error("chart_kind=pie 应产出 pie_chart")
	}
	if hasType(spec, CompBarChart) || hasType(spec, CompLineChart) {
		t.Error("pie opt-in 应替代默认柱/线图")
	}
	// 饼图复用 /series 绑定（labels+actual）。
	chart := findNode(spec, "chart")
	if chart == nil {
		t.Fatal("missing chart node")
	}
	if p, _ := bindingPath(chart.Props["series"]); p != "/series" {
		t.Errorf("pie_chart series 应绑定 /series, got %q", p)
	}
}

// opt-in：Meta.chart_kind=="funnel" 时以漏斗图替代默认柱/线，绑定 /series 且校验通过。
func TestTemplateCompose_FunnelOptIn(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	dm.Meta["chart_kind"] = "funnel"
	spec := TemplateCompose(dm, "转化漏斗")
	if err := Validate(spec); err != nil {
		t.Fatalf("funnel opt-in spec failed validation: %v", err)
	}
	if !hasType(spec, CompFunnel) {
		t.Error("chart_kind=funnel 应产出 funnel")
	}
	if hasType(spec, CompBarChart) || hasType(spec, CompLineChart) {
		t.Error("funnel opt-in 应替代默认柱/线图")
	}
}

// 新组件通过 catalog 校验：Grid 容器（含子节点）、pie_chart/funnel 绑定 /series、date_picker 交互组件。
func TestValidate_AcceptsNewComponents(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	spec := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompGrid, Props: map[string]interface{}{"columns": 2}, Children: []string{"pie", "fun", "dp"}},
			{ID: "pie", Type: CompPieChart, Props: map[string]interface{}{"title": "占比", "series": binding("/series")}},
			{ID: "fun", Type: CompFunnel, Props: map[string]interface{}{"title": "漏斗", "series": binding("/series")}},
			{ID: "dp", Type: CompDatePicker, Props: map[string]interface{}{"field": "下单日期", "action": "filter_date"}},
		},
	}
	if err := Validate(spec); err != nil {
		t.Errorf("新组件规格应通过校验: %v", err)
	}
}

// Grid 子引用缺失应被拒绝（容器子节点走通用完整性校验）。
func TestValidate_RejectsGridMissingChild(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	spec := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompGrid, Children: []string{"ghost"}},
		},
	}
	if err := Validate(spec); err == nil {
		t.Error("Grid 引用不存在的子节点应被拒绝")
	}
}

// pie_chart 绑定不可解析路径应被拒绝。
func TestValidate_RejectsPieChartBadBinding(t *testing.T) {
	dm := AssembleDataModel(sampleCategorical, "")
	spec := &UISpec{
		DataModel: dm,
		Components: []Component{
			{ID: RootID, Type: CompColumn, Children: []string{"pie"}},
			{ID: "pie", Type: CompPieChart, Props: map[string]interface{}{"series": binding("/nope")}},
		},
	}
	if err := Validate(spec); err == nil {
		t.Error("pie_chart 绑定不可解析路径应被拒绝")
	}
}

func hasType(spec *UISpec, typ string) bool {
	for _, c := range spec.Components {
		if c.Type == typ {
			return true
		}
	}
	return false
}

// 确保样本 JSON 合法（防止手写样本走样）。
func TestSamplesAreValidJSON(t *testing.T) {
	for name, s := range map[string]string{
		"sql": sampleSQL, "trend": sampleTrend, "kpi": sampleKPI,
		"categorical": sampleCategorical, "twoNumeric": sampleTwoNumeric, "correlation": sampleCorrelation,
	} {
		var v interface{}
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Errorf("sample %s invalid JSON: %v", name, err)
		}
	}
}
