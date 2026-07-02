package genui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// 真实形态样本：sql_execute 的月度销售额行集。
const sampleSQL = `{
  "columns": ["month", "revenue"],
  "rows": [
    {"month": "2026-01", "revenue": 120},
    {"month": "2026-02", "revenue": 135},
    {"month": "2026-03", "revenue": 150},
    {"month": "2026-04", "revenue": 168}
  ],
  "count": 4,
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
	if dm := AssembleDataModel(`{"columns":["a"],"rows":[],"count":0}`, ""); dm != nil {
		t.Errorf("empty rows should yield nil datamodel, got %v", dm)
	}
	if dm := AssembleDataModel("", ""); dm != nil {
		t.Errorf("empty sql should yield nil datamodel")
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
		Question: "营收趋势", SQLOutput: sampleSQL, AnalysisOutput: sampleTrend,
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
	for name, s := range map[string]string{"sql": sampleSQL, "trend": sampleTrend} {
		var v interface{}
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Errorf("sample %s invalid JSON: %v", name, err)
		}
	}
}
