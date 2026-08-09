package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	agentctx "cognida/internal/model/agent"
	modeltools "cognida/internal/model/agent/tools"
	"cognida/internal/service/agent/resultstore"
)

// mockInvoker 实现 MCPInvoker，用于单测无需真实 MCP server。
type mockInvoker struct {
	gotSkill  string
	gotParams map[string]interface{}
	result    *modeltools.SkillInvokeResult
	err       error
}

func (m *mockInvoker) Invoke(ctx interface{}, skillName string, params map[string]interface{}) (*modeltools.SkillInvokeResult, error) {
	m.gotSkill = skillName
	m.gotParams = params
	return m.result, m.err
}

// newAnalysisTool 用显式注入的调用器与结果存储构造 data_analysis 工具（取代包级全局）。
func newAnalysisTool(t *testing.T, inv MCPInvoker, rs resultstore.Store) tool.InvokableTool {
	t.Helper()
	tl, err := NewDataAnalysisTool(inv, rs, nil)
	if err != nil {
		t.Fatalf("NewDataAnalysisTool: %v", err)
	}
	return tl
}

func TestDataAnalysisTool_Info(t *testing.T) {
	tool, err := NewDataAnalysisTool(&mockInvoker{}, nil, nil)
	if err != nil {
		t.Fatalf("NewDataAnalysisTool: %v", err)
	}
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "data_analysis" {
		t.Errorf("name = %q, want data_analysis", info.Name)
	}
}

func TestDataAnalysisTool_Success(t *testing.T) {
	inv := &mockInvoker{
		result: &modeltools.SkillInvokeResult{
			Success: true,
			Data:    map[string]interface{}{"row_count": 6},
		},
	}
	tool := newAnalysisTool(t, inv, nil)
	args := `{"analysis_type":"describe","data":{"columns":["a"],"rows":[[1],[2]]},"options":{"columns":["a"]}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	// describe → data_describe
	if inv.gotSkill != "data_describe" {
		t.Errorf("skill = %q, want data_describe", inv.gotSkill)
	}
	// options 应被展开进 params，data 透传
	if _, ok := inv.gotParams["data"]; !ok {
		t.Errorf("params missing data: %v", inv.gotParams)
	}
	if cols, ok := inv.gotParams["columns"]; !ok {
		t.Errorf("options not spread into params: %v", inv.gotParams)
	} else {
		_ = cols
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if parsed["success"] != true {
		t.Errorf("success = %v, want true", parsed["success"])
	}
	if parsed["analysis_type"] != "describe" {
		t.Errorf("analysis_type = %v", parsed["analysis_type"])
	}
}

func TestDataAnalysisTool_UnknownType(t *testing.T) {
	tool := newAnalysisTool(t, &mockInvoker{}, nil)
	args := `{"analysis_type":"bogus","data":{"columns":["a"],"rows":[[1]]}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun should not error on unknown type: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("unknown type should yield success=false, got %v", parsed["success"])
	}
}

func TestDataAnalysisTool_MissingAnalysisType(t *testing.T) {
	tool := newAnalysisTool(t, &mockInvoker{}, nil)
	// 缺参属调用方错误：非致命 failResult 让 LLM 自纠，而非终止 ReAct 循环
	out, err := tool.InvokableRun(context.Background(), `{"data":{"columns":["a"],"rows":[[1]]}}`)
	if err != nil {
		t.Fatalf("missing analysis_type should not be fatal: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("missing analysis_type should yield success=false, got %v", parsed["success"])
	}
}

func TestDataAnalysisTool_MissingData(t *testing.T) {
	tool := newAnalysisTool(t, &mockInvoker{}, nil)
	out, err := tool.InvokableRun(context.Background(), `{"analysis_type":"describe"}`)
	if err != nil {
		t.Fatalf("missing data should not be fatal: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("missing data should yield success=false, got %v", parsed["success"])
	}
}

func TestDataAnalysisTool_NoInvoker(t *testing.T) {
	tool := newAnalysisTool(t, nil, nil)
	args := `{"analysis_type":"describe","data":{"columns":["a"],"rows":[[1]]}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("no invoker should yield success=false, got %v", parsed["success"])
	}
}

func TestDataAnalysisTool_InvokeError(t *testing.T) {
	tool := newAnalysisTool(t, &mockInvoker{err: errors.New("connection refused")}, nil)
	args := `{"analysis_type":"trend","data":{"columns":["a"],"rows":[[1]]},"options":{"value_col":"a"}}`
	out, err := tool.InvokableRun(context.Background(), args)
	if err != nil {
		t.Fatalf("InvokableRun should swallow invoke error into result: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("invoke error should yield success=false, got %v", parsed["success"])
	}
	if parsed["error"] == nil {
		t.Error("expected error message in result")
	}
}

// ========================================
// 命名能力路由 + result_id 引用取数 + 归因信封（任务 4a.1 / 4a.3 / 4a.5）
// ========================================

// setupAnalysisStore 构造内存 Result Store，返回带租户/会话的 ctx 与该存储。
func setupAnalysisStore(t *testing.T) (context.Context, resultstore.Store) {
	t.Helper()
	rs := resultstore.NewMemoryStore()
	return agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 1), "sess-da"), rs
}

// putRows 向指定存储预置一份行集，返回 result_id。
func putRows(t *testing.T, ctx context.Context, rs resultstore.Store, owner string) string {
	t.Helper()
	id, err := rs.Put(ctx, &resultstore.Result{
		Owner:   owner,
		Columns: []string{"month", "region", "gmv"},
		Rows: []map[string]interface{}{
			{"month": "2026-05", "region": "华北", "gmv": 1000.0},
			{"month": "2026-06", "region": "华北", "gmv": 600.0},
		},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置行集失败: %v", err)
	}
	return id
}

// 命名能力应路由到对应的 Python MCP 工具（报告解读复用综合洞察引擎）。
func TestDataAnalysisTool_NamedCapabilityRouting(t *testing.T) {
	cases := []struct {
		analysisType string
		wantSkill    string
	}{
		{"trend", "data_trend"},
		{"comparison", "data_comparison"},
		{"attribution", "data_attribution"},
		{"report", "data_insight"},
	}
	for _, c := range cases {
		inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}
		tool := newAnalysisTool(t, inv, nil)
		args := `{"analysis_type":"` + c.analysisType + `","data":[{"a":1}]}`
		if _, err := tool.InvokableRun(context.Background(), args); err != nil {
			t.Fatalf("%s: %v", c.analysisType, err)
		}
		if inv.gotSkill != c.wantSkill {
			t.Errorf("%s → %q, want %q", c.analysisType, inv.gotSkill, c.wantSkill)
		}
	}
}

// result_id 引用取数：解析行集后以 records 数组传给 Python。
func TestDataAnalysisTool_ResultIDResolved(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	id := putRows(t, ctx, rs, resultstore.OwnerKey(1, "sess-da"))
	inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}

	tool := newAnalysisTool(t, inv, rs)
	args := `{"analysis_type":"trend","result_id":"` + id + `","options":{"value_col":"gmv"}}`
	if _, err := tool.InvokableRun(ctx, args); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	rows, ok := inv.gotParams["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("data should be resolved records array, got %T", inv.gotParams["data"])
	}
	if len(rows) != 2 || rows[0]["region"] != "华北" {
		t.Errorf("resolved rows mismatch: %v", rows)
	}
	if inv.gotParams["value_col"] != "gmv" {
		t.Errorf("options not spread: %v", inv.gotParams)
	}
}

// result_id 不存在/已过期 → 非致命错误，供 Agent 自纠。
func TestDataAnalysisTool_ResultIDNotFound(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	tool := newAnalysisTool(t, &mockInvoker{}, rs)
	out, err := tool.InvokableRun(ctx, `{"analysis_type":"trend","result_id":"res_missing"}`)
	if err != nil {
		t.Fatalf("should be non-fatal: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("success = %v, want false", parsed["success"])
	}
	if msg, _ := parsed["error"].(string); !strings.Contains(msg, "不存在或已过期") {
		t.Errorf("error = %q", msg)
	}
}

// 跨会话引用应被拒绝（越权防护）。
func TestDataAnalysisTool_ResultIDCrossSession(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	id := putRows(t, ctx, rs, resultstore.OwnerKey(2, "other-session"))
	tool := newAnalysisTool(t, &mockInvoker{}, rs)
	out, err := tool.InvokableRun(ctx, `{"analysis_type":"trend","result_id":"`+id+`"}`)
	if err != nil {
		t.Fatalf("should be non-fatal: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("success = %v, want false", parsed["success"])
	}
	if msg, _ := parsed["error"].(string); !strings.Contains(msg, "不属于当前会话") {
		t.Errorf("error = %q", msg)
	}
}

// Result Store 未启用时按 result_id 取数 → 非致命错误提示改传 data。
func TestDataAnalysisTool_ResultIDNoStore(t *testing.T) {
	tool := newAnalysisTool(t, &mockInvoker{}, nil)
	out, err := tool.InvokableRun(context.Background(), `{"analysis_type":"trend","result_id":"res_x"}`)
	if err != nil {
		t.Fatalf("should be non-fatal: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("success = %v, want false", parsed["success"])
	}
}

// attributionResultData 模拟 Python data_attribution 经 JSON 解码后的返回。
func attributionResultData() map[string]interface{} {
	return map[string]interface{}{
		"metric":          "gmv",
		"dimension":       "region",
		"baseline":        map[string]interface{}{"label": "2026-05", "total": 2300.0},
		"current":         map[string]interface{}{"label": "2026-06", "total": 1950.0},
		"total_delta":     -350.0,
		"total_delta_pct": -15.22,
		"drivers": []interface{}{
			map[string]interface{}{
				"segment": "华北", "baseline": 1000.0, "current": 600.0, "delta": -400.0,
				"contribution_pct": 114.29, "share_of_change": 0.89, "direction": "down",
			},
			map[string]interface{}{
				"segment": "华东", "baseline": 800.0, "current": 850.0, "delta": 50.0,
				"contribution_pct": -14.29, "share_of_change": 0.11, "direction": "up",
			},
		},
		"hidden_factor": map[string]interface{}{
			"suspected": true,
			"notes":     []interface{}{"Top3 切片存在新增/消失，疑似结构性变化"},
		},
		"confidence": "high",
		"caliber": map[string]interface{}{
			"metric_col": "gmv", "dim_col": "region", "agg": "sum",
			"method": "additive_variance_decomposition",
		},
	}
}

// 归因成功：drivers 落 Result Store 得新 result_id，信封含洞察/口径/置信/下钻（任务 4a.3）。
func TestDataAnalysisTool_AttributionEnvelope(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	srcID := putRows(t, ctx, rs, resultstore.OwnerKey(1, "sess-da"))
	tool := newAnalysisTool(t, &mockInvoker{result: &modeltools.SkillInvokeResult{
		Success: true,
		Data:    attributionResultData(),
	}}, rs)
	args := `{"analysis_type":"attribution","result_id":"` + srcID + `","options":{"value_col":"gmv","period_col":"month"}}`
	out, err := tool.InvokableRun(ctx, args)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}

	// drivers 表新 result_id：可从 Result Store 取回且行内容一致
	newID, _ := parsed["result_id"].(string)
	if newID == "" {
		t.Fatal("expected drivers result_id in envelope")
	}
	stored, err := rs.Get(ctx, resultstore.OwnerKey(1, "sess-da"), newID)
	if err != nil {
		t.Fatalf("drivers result not retrievable: %v", err)
	}
	if len(stored.Rows) != 2 || stored.Rows[0]["segment"] != "华北" {
		t.Errorf("drivers rows mismatch: %v", stored.Rows)
	}
	if len(stored.Columns) == 0 || stored.Columns[0] != "segment" {
		t.Errorf("drivers columns mismatch: %v", stored.Columns)
	}

	// 确定性文字洞察：含指标、头号驱动、隐藏因子与置信标注
	insight, _ := parsed["insight"].(string)
	for _, want := range []string{"gmv", "华北", "-400", "隐藏因子", "置信度：高"} {
		if !strings.Contains(insight, want) {
			t.Errorf("insight missing %q: %s", want, insight)
		}
	}

	// 口径/置信上提到顶层
	caliber, _ := parsed["caliber"].(map[string]interface{})
	if caliber["method"] != "additive_variance_decomposition" {
		t.Errorf("caliber not surfaced: %v", parsed["caliber"])
	}
	if parsed["confidence"] != "high" {
		t.Errorf("confidence = %v", parsed["confidence"])
	}

	// 下钻建议引用来源 result_id 与头号驱动切片
	dd, _ := parsed["drill_down"].(map[string]interface{})
	if dd["source_result_id"] != srcID {
		t.Errorf("drill_down.source_result_id = %v, want %s", dd["source_result_id"], srcID)
	}
	if sug, _ := dd["suggestion"].(string); !strings.Contains(sug, "region") || !strings.Contains(sug, "华北") {
		t.Errorf("drill_down.suggestion = %q", sug)
	}
}

// ========================================
// ① 会话最近取数回退（修复「结合特征分析…卡住」的 Analysis 空转，任务 19）
// ========================================

// newAnalysisToolWithReg 用显式注入的调用器 + 结果存储 + 会话取数注册表构造工具。
func newAnalysisToolWithReg(t *testing.T, inv MCPInvoker, rs resultstore.Store, reg *resultstore.SessionRegistry) tool.InvokableTool {
	t.Helper()
	tl, err := NewDataAnalysisTool(inv, rs, reg)
	if err != nil {
		t.Fatalf("NewDataAnalysisTool: %v", err)
	}
	return tl
}

// 既无内联 data 又无 result_id 时，回退到本会话最近一次取数结果（SessionRegistry.Latest）。
func TestDataAnalysisTool_SessionLatestFallback(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	owner := resultstore.OwnerKey(1, "sess-da")
	id := putRows(t, ctx, rs, owner)

	reg := resultstore.NewSessionRegistry(resultstore.DefaultTTL)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ..."), id)

	inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}
	tool := newAnalysisToolWithReg(t, inv, rs, reg)

	// 注意：既不带 data 也不带 result_id —— 正是 Analysis 子代理未显式串联 result_id 的委派场景。
	out, err := tool.InvokableRun(ctx, `{"analysis_type":"trend","options":{"value_col":"gmv"}}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	rows, ok := inv.gotParams["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("fallback should resolve latest rows into data, got %T", inv.gotParams["data"])
	}
	if len(rows) != 2 || rows[0]["region"] != "华北" {
		t.Errorf("fallback rows mismatch: %v", rows)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != true {
		t.Errorf("success = %v, want true", parsed["success"])
	}
}

// 无内联 data、无 result_id、且本会话尚无取数记录 → 非致命失败（供 Agent 自纠，不 dead-end）。
func TestDataAnalysisTool_SessionLatestFallbackEmpty(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	reg := resultstore.NewSessionRegistry(resultstore.DefaultTTL) // 未 RecordFetch 任何结果
	tool := newAnalysisToolWithReg(t, &mockInvoker{}, rs, reg)

	out, err := tool.InvokableRun(ctx, `{"analysis_type":"trend"}`)
	if err != nil {
		t.Fatalf("should be non-fatal: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("success = %v, want false", parsed["success"])
	}
	if msg, _ := parsed["error"].(string); !strings.Contains(msg, "尚无可复用") {
		t.Errorf("error = %q, want 提示本会话尚无可复用取数", msg)
	}
}

// 委派带外句柄：模型未在参数里给 result_id/data 时，采用委派信封钉进 childCtx 的确切句柄，
// 且该「指挥官选定」的句柄优先于「会话最近一次取数」的猜测（防多取数场景取错数据）。
func TestDataAnalysisTool_DelegatedResultIDPinnedOverLatest(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	owner := resultstore.OwnerKey(1, "sess-da")

	// pinned：指挥官为本次委派选定的结果（华南），经带外通道钉入 childCtx。
	pinnedID, err := rs.Put(ctx, &resultstore.Result{
		Owner:   owner,
		Columns: []string{"region", "gmv"},
		Rows:    []map[string]interface{}{{"region": "华南", "gmv": 888.0}},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置 pinned 行集失败: %v", err)
	}
	// latest：另有一次更晚的取数（华北）登记为会话最近——若错误地取 latest 就会命中它。
	latestID := putRows(t, ctx, rs, owner)
	reg := resultstore.NewSessionRegistry(resultstore.DefaultTTL)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ... latest"), latestID)

	inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}
	tool := newAnalysisToolWithReg(t, inv, rs, reg)

	// 把指挥官选定的句柄钉进 ctx（模拟 executeDelegation 对 childCtx 的处理）。
	ctx = agentctx.WithDelegatedResultID(ctx, pinnedID)

	// 既不带 data 也不带 result_id —— 正是 Analysis 子代理未显式串联 result_id 的委派场景。
	if _, err := tool.InvokableRun(ctx, `{"analysis_type":"trend","options":{"value_col":"gmv"}}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	rows, ok := inv.gotParams["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("应据带外句柄解析出行集，got %T", inv.gotParams["data"])
	}
	if len(rows) != 1 || rows[0]["region"] != "华南" {
		t.Errorf("应取 pinned（华南）而非 latest（华北），got %v", rows)
	}
}

// 嵌套委派边界：内层信封未带 result_id 时，须清除继承自外层的钉入句柄（写空串覆盖），
// 使深层 data_analysis 回退到「本会话最近取数」而非误取上层句柄。
func TestDataAnalysisTool_DelegatedResultIDClearedFallsBackToLatest(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	owner := resultstore.OwnerKey(1, "sess-da")

	// 外层曾钉入的句柄（华南）——内层应当看不到它。
	ancestorID, err := rs.Put(ctx, &resultstore.Result{
		Owner:   owner,
		Columns: []string{"region", "gmv"},
		Rows:    []map[string]interface{}{{"region": "华南", "gmv": 888.0}},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置 ancestor 行集失败: %v", err)
	}
	latestID := putRows(t, ctx, rs, owner) // 会话最近取数（华北）
	reg := resultstore.NewSessionRegistry(resultstore.DefaultTTL)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ... latest"), latestID)

	inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}
	tool := newAnalysisToolWithReg(t, inv, rs, reg)

	// 模拟委派链：外层钉入 ancestorID，内层信封无 result_id → 写空串清除继承。
	ctx = agentctx.WithDelegatedResultID(ctx, ancestorID)
	ctx = agentctx.WithDelegatedResultID(ctx, "")

	if _, err := tool.InvokableRun(ctx, `{"analysis_type":"trend","options":{"value_col":"gmv"}}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	rows, ok := inv.gotParams["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("应回退到最近取数并解析出行集，got %T", inv.gotParams["data"])
	}
	if rows[0]["region"] != "华北" {
		t.Errorf("清除继承句柄后应取 latest（华北）而非 ancestor（华南），got %v", rows)
	}
}

// 路由消歧：本会话有 ≥2 份 live 取数、且既无 pin 又无显式 result_id 时，宁拒不猜——
// 返回非致命消歧提示并列出候选 result_id（含「最近一次取数」标注），而非盲取 latest 静默分析错数据。
func TestDataAnalysisTool_MultipleFetchesDisambiguation(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	owner := resultstore.OwnerKey(1, "sess-da")

	// 第一份取数（华北，2 行）。
	firstID := putRows(t, ctx, rs, owner)
	// 第二份取数（华南，1 行）——不同 SQL、不同结果，构成真歧义。
	secondID, err := rs.Put(ctx, &resultstore.Result{
		Owner:   owner,
		Columns: []string{"region", "gmv"},
		Rows:    []map[string]interface{}{{"region": "华南", "gmv": 888.0}},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置第二份行集失败: %v", err)
	}
	reg := resultstore.NewSessionRegistry(resultstore.DefaultTTL)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ... north"), firstID)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ... south"), secondID) // 最近一次

	inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}
	tool := newAnalysisToolWithReg(t, inv, rs, reg)

	out, err := tool.InvokableRun(ctx, `{"analysis_type":"trend","options":{"value_col":"gmv"}}`)
	if err != nil {
		t.Fatalf("消歧应为非致命结果: %v", err)
	}
	// 不得盲取任一份去调用 Python。
	if inv.gotParams != nil {
		t.Fatalf("多份候选时不应静默取数调用分析，got params=%v", inv.gotParams)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("success = %v, want false（消歧提示）", parsed["success"])
	}
	msg, _ := parsed["error"].(string)
	if !strings.Contains(msg, "多份") || !strings.Contains(msg, "result_id") {
		t.Errorf("error = %q, want 提示存在多份候选且需显式指定 result_id", msg)
	}
	if !strings.Contains(msg, firstID) || !strings.Contains(msg, secondID) {
		t.Errorf("error = %q, want 列出两个候选 result_id %s / %s", msg, firstID, secondID)
	}
	if !strings.Contains(msg, "[最近一次取数]") {
		t.Errorf("error = %q, want 标注最近一次取数以便快速确认", msg)
	}
}

// 消歧守卫只在「回退」路径生效：即便本会话存在多份候选，显式 result_id 仍直接命中、不触发消歧。
func TestDataAnalysisTool_ExplicitResultIDOverridesAmbiguity(t *testing.T) {
	ctx, rs := setupAnalysisStore(t)
	owner := resultstore.OwnerKey(1, "sess-da")

	firstID := putRows(t, ctx, rs, owner) // 华北
	secondID, err := rs.Put(ctx, &resultstore.Result{
		Owner:   owner,
		Columns: []string{"region", "gmv"},
		Rows:    []map[string]interface{}{{"region": "华南", "gmv": 888.0}},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置第二份行集失败: %v", err)
	}
	reg := resultstore.NewSessionRegistry(resultstore.DefaultTTL)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ... north"), firstID)
	reg.RecordFetch(owner, resultstore.SQLSignature("db1", "SELECT ... south"), secondID)

	inv := &mockInvoker{result: &modeltools.SkillInvokeResult{Success: true}}
	tool := newAnalysisToolWithReg(t, inv, rs, reg)

	// 显式指定第一份，尽管存在多份候选也不应触发消歧。
	args := `{"analysis_type":"trend","result_id":"` + firstID + `","options":{"value_col":"gmv"}}`
	if _, err := tool.InvokableRun(ctx, args); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	rows, ok := inv.gotParams["data"].([]map[string]interface{})
	if !ok {
		t.Fatalf("显式 result_id 应直接解析行集，got %T", inv.gotParams["data"])
	}
	if len(rows) != 2 || rows[0]["region"] != "华北" {
		t.Errorf("应取显式指定的第一份（华北），got %v", rows)
	}
}

// 归因失败时不拼装信封（无 result_id / insight）。
func TestDataAnalysisTool_AttributionFailureNoEnvelope(t *testing.T) {
	_, rs := setupAnalysisStore(t)
	tool := newAnalysisTool(t, &mockInvoker{result: &modeltools.SkillInvokeResult{
		Success: false,
		Error:   "缺少必需参数: value_col",
	}}, rs)
	out, err := tool.InvokableRun(context.Background(), `{"analysis_type":"attribution","data":[{"a":1}]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var parsed map[string]interface{}
	_ = json.Unmarshal([]byte(out), &parsed)
	if parsed["success"] != false {
		t.Errorf("success = %v, want false", parsed["success"])
	}
	if _, has := parsed["result_id"]; has {
		t.Error("failed attribution should not produce result_id")
	}
	if _, has := parsed["insight"]; has {
		t.Error("failed attribution should not produce insight")
	}
}
