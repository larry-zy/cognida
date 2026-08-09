package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/service/agent/genui"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/agent/uibinding"
)

// setupRenderTest 构造内存 Result Store + 绑定存储，写入一份 n 行的结果集，
// 返回带租户/会话的 ctx、result_id 与注入用的 Result Store / 绑定存储。
// 绑定存储改为经参数注入 renderUI（去 uibinding 包级单例，架构加固 Phase 6）。
func setupRenderTest(t *testing.T, n int) (context.Context, string, resultstore.Store, uibinding.Store) {
	t.Helper()

	rs := resultstore.NewMemoryStore()
	ui := uibinding.NewMemoryStore()

	ctx := agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 1), "sess-render")

	rows := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, map[string]interface{}{
			"month": fmt.Sprintf("2026-%02d", i%12+1),
			"gmv":   float64(1000 + i),
		})
	}
	id, err := rs.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(1, "sess-render"),
		Columns: []string{"month", "gmv"},
		Rows:    rows,
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置结果集失败: %v", err)
	}
	return ctx, id, rs, ui
}

// TestRenderUI_MultipleSurfacesIndependent 多次渲染产出独立 surface（任务 4.10-1）。
func TestRenderUI_MultipleSurfacesIndependent(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 5)

	r1, err := renderUI(ctx, &RenderUIRequest{ResultID: id, Title: "面板一"}, rs, ui)
	if err != nil {
		t.Fatalf("第一次渲染失败: %v", err)
	}
	r2, err := renderUI(ctx, &RenderUIRequest{ResultID: id, Title: "面板二"}, rs, ui)
	if err != nil {
		t.Fatalf("第二次渲染失败: %v", err)
	}

	if r1.Surface == "" || r2.Surface == "" {
		t.Fatal("surface 不能为空")
	}
	if r1.Surface == r2.Surface {
		t.Errorf("两次渲染的 surface 应互相独立: %s == %s", r1.Surface, r2.Surface)
	}
	if r1.UISpec == nil || r2.UISpec == nil {
		t.Fatal("UISpec 不能为空")
	}
	if r1.UISpec.Title != "面板一" || r2.UISpec.Title != "面板二" {
		t.Errorf("标题未按调用区分: %q / %q", r1.UISpec.Title, r2.UISpec.Title)
	}
}

// TestRenderUI_LargeTableByReference 大表按引用：快照有界，不全量内联（任务 4.10-2 / 4.6）。
func TestRenderUI_LargeTableByReference(t *testing.T) {
	const total = 500
	ctx, id, rs, ui := setupRenderTest(t, total)

	r, err := renderUI(ctx, &RenderUIRequest{ResultID: id, Title: "大表"}, rs, ui)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	if r.RowCount != total {
		t.Errorf("RowCount 期望 %d, got %d", total, r.RowCount)
	}
	if r.SnapshotRows > resultstore.DefaultSampleRows {
		t.Errorf("快照行数超上限: %d > %d", r.SnapshotRows, resultstore.DefaultSampleRows)
	}
	if !r.Truncated {
		t.Error("大表应标记 Truncated")
	}
	if r.Note == "" {
		t.Error("截断时应给 LLM 提示 Note")
	}
	// 规格内实际快照的行也必须有界（持久化进消息记录的就是它）
	if dm := r.UISpec.DataModel; dm == nil || dm.Table == nil {
		t.Fatal("DataModel.Table 不能为空")
	} else if len(dm.Table.Rows) > resultstore.DefaultSampleRows {
		t.Errorf("规格内快照行数超上限: %d", len(dm.Table.Rows))
	}
	// result_id 必须随 Meta 暴露（交互回调与过期降级依据）
	if got := r.UISpec.DataModel.Meta["result_id"]; got != id {
		t.Errorf("Meta.result_id 期望 %q, got %v", id, got)
	}
}

// TestRenderUI_InvalidResultID 非法 result_id 拒绝渲染（任务 4.10-3 / 4.3）。
func TestRenderUI_InvalidResultID(t *testing.T) {
	ctx, _, rs, ui := setupRenderTest(t, 3)

	if _, err := renderUI(ctx, &RenderUIRequest{ResultID: "res_不存在"}, rs, ui); err == nil {
		t.Fatal("非法 result_id 应返回错误")
	} else if !strings.Contains(err.Error(), "不存在或已过期") {
		t.Errorf("错误信息应提示过期/不存在: %v", err)
	}

	if _, err := renderUI(ctx, &RenderUIRequest{}, rs, ui); err == nil {
		t.Fatal("空 result_id 应返回错误")
	}
}

// TestRenderUI_CrossSessionRejected 跨会话读取拒绝（归属校验）。
func TestRenderUI_CrossSessionRejected(t *testing.T) {
	_, id, rs, ui := setupRenderTest(t, 3)

	otherCtx := agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 1), "sess-other")
	if _, err := renderUI(otherCtx, &RenderUIRequest{ResultID: id}, rs, ui); err == nil {
		t.Fatal("跨会话渲染应拒绝")
	} else if !strings.Contains(err.Error(), "不属于当前会话") {
		t.Errorf("错误信息应提示归属不符: %v", err)
	}
}

// TestRenderUI_NonCatalogComponentRejected 目录外组件拒绝（任务 4.10-3 / 4.3）。
func TestRenderUI_NonCatalogComponentRejected(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	_, err := renderUI(ctx, &RenderUIRequest{
		ResultID: id,
		Components: []genui.Component{
			{ID: "root", Type: "Column", Children: []string{"x"}},
			{ID: "x", Type: "Iframe", Props: map[string]interface{}{"src": "https://evil"}},
		},
	}, rs, ui)
	if err == nil {
		t.Fatal("目录外组件应拒绝")
	}
	if !strings.Contains(err.Error(), "校验失败") {
		t.Errorf("错误信息应说明校验失败: %v", err)
	}
}

// TestRenderUI_OutOfBoundsPointerRejected 越界 JSON Pointer 拒绝（任务 4.10-3 / 4.3）。
func TestRenderUI_OutOfBoundsPointerRejected(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	_, err := renderUI(ctx, &RenderUIRequest{
		ResultID: id,
		Components: []genui.Component{
			{ID: "root", Type: "Column", Children: []string{"m"}},
			{ID: "m", Type: "MetricCard", Props: map[string]interface{}{
				"label": "不存在的指标",
				"value": map[string]interface{}{"path": "/metrics/不存在"},
			}},
		},
	}, rs, ui)
	if err == nil {
		t.Fatal("越界 Pointer 应拒绝")
	}
}

// TestRenderUI_CustomLayoutValidated 合法自定义布局走 LLM 模式并通过校验。
func TestRenderUI_CustomLayoutValidated(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	r, err := renderUI(ctx, &RenderUIRequest{
		ResultID: id,
		Title:    "自定义",
		Components: []genui.Component{
			{ID: "root", Type: "Column", Children: []string{"t"}},
			{ID: "t", Type: "Table", Props: map[string]interface{}{
				"title": "明细",
				"data":  map[string]interface{}{"path": "/table"},
			}},
		},
	}, rs, ui)
	if err != nil {
		t.Fatalf("合法自定义布局不应拒绝: %v", err)
	}
	if r.GenMode != genui.GenModeLLM {
		t.Errorf("自定义布局应为 llm 模式, got %q", r.GenMode)
	}
}

// TestRenderUI_BindingTokenIssued 渲染成功后签发绑定 token 且绑定可路由（任务 4.8）。
func TestRenderUI_BindingTokenIssued(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	r, err := renderUI(ctx, &RenderUIRequest{ResultID: id}, rs, ui)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	token, _ := r.UISpec.DataModel.Meta["surface_token"].(string)
	if token == "" {
		t.Fatal("Meta.surface_token 未签发")
	}

	b, err := ui.Get(ctx, r.Surface)
	if err != nil {
		t.Fatalf("绑定应可按 surface 取回: %v", err)
	}
	if b.Token != token || b.ResultID != id || b.SessionID != "sess-render" {
		t.Errorf("绑定内容不一致: %+v", b)
	}
}

// TestRenderUI_NoBindingStoreDegrades 绑定存储未注入时渲染仍成功（降级为无交互）。
func TestRenderUI_NoBindingStoreDegrades(t *testing.T) {
	ctx, id, rs, _ := setupRenderTest(t, 3)

	// 绑定存储未注入（nil）：渲染应仍成功，只是降级为无交互回调。
	r, err := renderUI(ctx, &RenderUIRequest{ResultID: id}, rs, nil)
	if err != nil {
		t.Fatalf("绑定存储缺失不应阻断渲染: %v", err)
	}
	if _, ok := r.UISpec.DataModel.Meta["surface_token"]; ok {
		t.Error("无绑定存储时不应签发 token")
	}
}

// TestRenderUIRequest_NestedTreeTolerated 复现历史 bug：LLM 把 components 传成
// 内联嵌套树（单个 root 对象，children 直接内联子组件），过去导致
// "cannot unmarshal object into []genui.Component"。现应被摊平并成功渲染。
func TestRenderUIRequest_NestedTreeTolerated(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 5)

	payload := fmt.Sprintf(`{
		"result_id": %q,
		"title": "🏆 工单看板",
		"components": {
			"id": "root",
			"type": "Column",
			"children": [
				{"type": "Row", "children": [
					{"type": "MetricCard", "props": {"label": "结果集", "value": {"path": "/meta/result_id"}}}
				]},
				{"type": "Table", "props": {"title": "明细", "data": {"path": "/table"}}},
				{"type": "Callout", "props": {"title": "结论", "text": "看板", "tone": "info"}}
			]
		}
	}`, id)

	var req RenderUIRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("嵌套树 components 应被容忍解析: %v", err)
	}
	// root(Column) + Row + MetricCard + Table + Callout = 5 个节点，摊平为邻接表。
	if len(req.Components) != 5 {
		t.Fatalf("摊平后组件数期望 5, got %d: %+v", len(req.Components), req.Components)
	}
	assertExactlyOneRoot(t, req.Components)
	assertChildrenResolve(t, req.Components)

	r, err := renderUI(ctx, &req, rs, ui)
	if err != nil {
		t.Fatalf("摊平后的自定义布局应通过校验并渲染: %v", err)
	}
	if r.GenMode != genui.GenModeLLM {
		t.Errorf("自定义布局应为 llm 模式, got %q", r.GenMode)
	}
	if r.UISpec.Title != "🏆 工单看板" {
		t.Errorf("标题未透传: %q", r.UISpec.Title)
	}
}

// TestRenderUIRequest_FlatArrayUnchanged 标准扁平邻接表（children 为 id 字符串数组）
// 经 UnmarshalJSON 后应原样保留、逐节点一一对应。
func TestRenderUIRequest_FlatArrayUnchanged(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	payload := fmt.Sprintf(`{
		"result_id": %q,
		"components": [
			{"id": "root", "type": "Column", "children": ["tbl", "cal"]},
			{"id": "tbl", "type": "Table", "props": {"title": "明细", "data": {"path": "/table"}}},
			{"id": "cal", "type": "Callout", "props": {"title": "结论", "text": "ok", "tone": "info"}}
		]
	}`, id)

	var req RenderUIRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("扁平邻接表解析失败: %v", err)
	}
	if len(req.Components) != 3 {
		t.Fatalf("组件数期望 3, got %d", len(req.Components))
	}
	if req.Components[0].ID != "root" || len(req.Components[0].Children) != 2 {
		t.Errorf("root 节点未原样保留: %+v", req.Components[0])
	}
	assertExactlyOneRoot(t, req.Components)
	assertChildrenResolve(t, req.Components)

	if _, err := renderUI(ctx, &req, rs, ui); err != nil {
		t.Fatalf("扁平邻接表应正常渲染: %v", err)
	}
}

// TestRenderUIRequest_EmptyComponentsFallsBackToTemplate components 省略/为 null
// 时走确定性模板路径。
func TestRenderUIRequest_EmptyComponentsFallsBackToTemplate(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"缺省", `{"result_id": %q}`},
		{"null", `{"result_id": %q, "components": null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, id, rs, ui := setupRenderTest(t, 3)
			var req RenderUIRequest
			if err := json.Unmarshal([]byte(fmt.Sprintf(tc.body, id)), &req); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if req.Components != nil {
				t.Errorf("空 components 应为 nil, got %+v", req.Components)
			}
			r, err := renderUI(ctx, &req, rs, ui)
			if err != nil {
				t.Fatalf("渲染失败: %v", err)
			}
			if r.GenMode != genui.GenModeTemplate {
				t.Errorf("空布局应走模板模式, got %q", r.GenMode)
			}
		})
	}
}

// TestRenderUIRequest_MixedChildrenPreserved children 混用「字符串 id 引用」与
// 「内联子对象」时，两类都不能丢——逐元素处理而非整段丢弃。
func TestRenderUIRequest_MixedChildrenPreserved(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	// root.children 混用：字符串引用 "tbl" + 一个内联 Callout 对象。
	payload := fmt.Sprintf(`{
		"result_id": %q,
		"components": [
			{"id": "root", "type": "Column", "children": [
				"tbl",
				{"type": "Callout", "props": {"title": "结论", "text": "ok", "tone": "info"}}
			]},
			{"id": "tbl", "type": "Table", "props": {"title": "明细", "data": {"path": "/table"}}}
		]
	}`, id)

	var req RenderUIRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("混用 children 解析失败: %v", err)
	}
	// root + tbl + 摊平出的 Callout = 3 个节点。
	if len(req.Components) != 3 {
		t.Fatalf("混用 children 摊平后组件数期望 3, got %d: %+v", len(req.Components), req.Components)
	}
	// root 的两个子引用都应保留（"tbl" + 生成 id 的 Callout）。
	var root *genui.Component
	for i := range req.Components {
		if req.Components[i].ID == genui.RootID {
			root = &req.Components[i]
		}
	}
	if root == nil || len(root.Children) != 2 {
		t.Fatalf("root 应保留 2 个子引用（字符串+内联），got %+v", root)
	}
	assertExactlyOneRoot(t, req.Components)
	assertChildrenResolve(t, req.Components)

	if _, err := renderUI(ctx, &req, rs, ui); err != nil {
		t.Fatalf("混用 children 应正常渲染: %v", err)
	}
}

// TestRenderUIRequest_NestedRootWithoutID 单个 root 对象省略 id 时补成固定根 id，
// 不因缺 root 被校验拒绝。
func TestRenderUIRequest_NestedRootWithoutID(t *testing.T) {
	ctx, id, rs, ui := setupRenderTest(t, 3)

	payload := fmt.Sprintf(`{
		"result_id": %q,
		"components": {
			"type": "Column",
			"children": [
				{"type": "Table", "props": {"title": "明细", "data": {"path": "/table"}}}
			]
		}
	}`, id)

	var req RenderUIRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	assertExactlyOneRoot(t, req.Components)
	assertChildrenResolve(t, req.Components)

	if _, err := renderUI(ctx, &req, rs, ui); err != nil {
		t.Fatalf("补 root id 后应正常渲染: %v", err)
	}
}

// TestRenderUITool_ExposesParamsSchema 断言 render_ui 工具的 Info() 带上了机器可读的
// 参数 schema（由 RenderUIRequest 反射生成），且 components 是「对象数组」而非自由文本——
// 这是让 LLM 在格式层遵循邻接表形状的根本手段（历史 bug：LLM 误传嵌套树）。
func TestRenderUITool_ExposesParamsSchema(t *testing.T) {
	tool := NewRenderUITool(nil, nil)
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info 失败: %v", err)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("render_ui 应下发机器可读的 ParamsOneOf，got nil")
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("导出 JSON schema 失败: %v", err)
	}
	raw, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("序列化 schema 失败: %v", err)
	}
	s := string(raw)
	for _, want := range []string{"result_id", "components"} {
		if !strings.Contains(s, want) {
			t.Errorf("schema 应含字段 %q, schema=%s", want, s)
		}
	}
	// components 必须被反射为数组类型（array），而非退化成 string/自由文本。
	if !strings.Contains(s, `"array"`) {
		t.Errorf("components 应为对象数组(array)类型, schema=%s", s)
	}
}

// assertExactlyOneRoot 断言邻接表含且仅含一个 id=="root" 的节点。
func assertExactlyOneRoot(t *testing.T, comps []genui.Component) {
	t.Helper()
	n := 0
	for _, c := range comps {
		if c.ID == genui.RootID {
			n++
		}
	}
	if n != 1 {
		t.Errorf("应含且仅含一个 root 节点, got %d", n)
	}
}

// assertChildrenResolve 断言所有 children 引用的 id 都能在邻接表内找到（无悬挂引用）。
func assertChildrenResolve(t *testing.T, comps []genui.Component) {
	t.Helper()
	ids := make(map[string]bool, len(comps))
	for _, c := range comps {
		if c.ID == "" {
			t.Error("摊平后仍存在空 id 节点")
		}
		if ids[c.ID] {
			t.Errorf("摊平后出现重复 id: %s", c.ID)
		}
		ids[c.ID] = true
	}
	for _, c := range comps {
		for _, child := range c.Children {
			if !ids[child] {
				t.Errorf("节点 %q 引用了不存在的子节点 %q", c.ID, child)
			}
		}
	}
}
