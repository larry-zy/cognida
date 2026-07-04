package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	agentctx "link/internal/model/agent"
	"link/internal/service/agent/genui"
	"link/internal/service/agent/resultstore"
	"link/internal/service/agent/uibinding"
)

// setupRenderTest 注入内存 Result Store + 绑定存储，写入一份 n 行的结果集，
// 返回带租户/会话的 ctx 与 result_id。测试结束自动还原单例。
func setupRenderTest(t *testing.T, n int) (context.Context, string) {
	t.Helper()

	oldRS := resultStore
	oldBS := uibinding.GetStore()
	t.Cleanup(func() {
		resultStore = oldRS
		uibinding.SetStore(oldBS)
	})

	InitResultStore(resultstore.NewMemoryStore())
	uibinding.SetStore(uibinding.NewMemoryStore())

	ctx := agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 1), "sess-render")

	rows := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, map[string]interface{}{
			"month": fmt.Sprintf("2026-%02d", i%12+1),
			"gmv":   float64(1000 + i),
		})
	}
	id, err := resultStore.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(1, "sess-render"),
		Columns: []string{"month", "gmv"},
		Rows:    rows,
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("预置结果集失败: %v", err)
	}
	return ctx, id
}

// TestRenderUI_MultipleSurfacesIndependent 多次渲染产出独立 surface（任务 4.10-1）。
func TestRenderUI_MultipleSurfacesIndependent(t *testing.T) {
	ctx, id := setupRenderTest(t, 5)

	r1, err := renderUI(ctx, &RenderUIRequest{ResultID: id, Title: "面板一"})
	if err != nil {
		t.Fatalf("第一次渲染失败: %v", err)
	}
	r2, err := renderUI(ctx, &RenderUIRequest{ResultID: id, Title: "面板二"})
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
	ctx, id := setupRenderTest(t, total)

	r, err := renderUI(ctx, &RenderUIRequest{ResultID: id, Title: "大表"})
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
	ctx, _ := setupRenderTest(t, 3)

	if _, err := renderUI(ctx, &RenderUIRequest{ResultID: "res_不存在"}); err == nil {
		t.Fatal("非法 result_id 应返回错误")
	} else if !strings.Contains(err.Error(), "不存在或已过期") {
		t.Errorf("错误信息应提示过期/不存在: %v", err)
	}

	if _, err := renderUI(ctx, &RenderUIRequest{}); err == nil {
		t.Fatal("空 result_id 应返回错误")
	}
}

// TestRenderUI_CrossSessionRejected 跨会话读取拒绝（归属校验）。
func TestRenderUI_CrossSessionRejected(t *testing.T) {
	_, id := setupRenderTest(t, 3)

	otherCtx := agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 1), "sess-other")
	if _, err := renderUI(otherCtx, &RenderUIRequest{ResultID: id}); err == nil {
		t.Fatal("跨会话渲染应拒绝")
	} else if !strings.Contains(err.Error(), "不属于当前会话") {
		t.Errorf("错误信息应提示归属不符: %v", err)
	}
}

// TestRenderUI_NonCatalogComponentRejected 目录外组件拒绝（任务 4.10-3 / 4.3）。
func TestRenderUI_NonCatalogComponentRejected(t *testing.T) {
	ctx, id := setupRenderTest(t, 3)

	_, err := renderUI(ctx, &RenderUIRequest{
		ResultID: id,
		Components: []genui.Component{
			{ID: "root", Type: "Column", Children: []string{"x"}},
			{ID: "x", Type: "Iframe", Props: map[string]interface{}{"src": "https://evil"}},
		},
	})
	if err == nil {
		t.Fatal("目录外组件应拒绝")
	}
	if !strings.Contains(err.Error(), "校验失败") {
		t.Errorf("错误信息应说明校验失败: %v", err)
	}
}

// TestRenderUI_OutOfBoundsPointerRejected 越界 JSON Pointer 拒绝（任务 4.10-3 / 4.3）。
func TestRenderUI_OutOfBoundsPointerRejected(t *testing.T) {
	ctx, id := setupRenderTest(t, 3)

	_, err := renderUI(ctx, &RenderUIRequest{
		ResultID: id,
		Components: []genui.Component{
			{ID: "root", Type: "Column", Children: []string{"m"}},
			{ID: "m", Type: "MetricCard", Props: map[string]interface{}{
				"label": "不存在的指标",
				"value": map[string]interface{}{"path": "/metrics/不存在"},
			}},
		},
	})
	if err == nil {
		t.Fatal("越界 Pointer 应拒绝")
	}
}

// TestRenderUI_CustomLayoutValidated 合法自定义布局走 LLM 模式并通过校验。
func TestRenderUI_CustomLayoutValidated(t *testing.T) {
	ctx, id := setupRenderTest(t, 3)

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
	})
	if err != nil {
		t.Fatalf("合法自定义布局不应拒绝: %v", err)
	}
	if r.GenMode != genui.GenModeLLM {
		t.Errorf("自定义布局应为 llm 模式, got %q", r.GenMode)
	}
}

// TestRenderUI_BindingTokenIssued 渲染成功后签发绑定 token 且绑定可路由（任务 4.8）。
func TestRenderUI_BindingTokenIssued(t *testing.T) {
	ctx, id := setupRenderTest(t, 3)

	r, err := renderUI(ctx, &RenderUIRequest{ResultID: id})
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	token, _ := r.UISpec.DataModel.Meta["surface_token"].(string)
	if token == "" {
		t.Fatal("Meta.surface_token 未签发")
	}

	b, err := uibinding.GetStore().Get(ctx, r.Surface)
	if err != nil {
		t.Fatalf("绑定应可按 surface 取回: %v", err)
	}
	if b.Token != token || b.ResultID != id || b.SessionID != "sess-render" {
		t.Errorf("绑定内容不一致: %+v", b)
	}
}

// TestRenderUI_NoBindingStoreDegrades 绑定存储未注入时渲染仍成功（降级为无交互）。
func TestRenderUI_NoBindingStoreDegrades(t *testing.T) {
	ctx, id := setupRenderTest(t, 3)
	uibinding.SetStore(nil)

	r, err := renderUI(ctx, &RenderUIRequest{ResultID: id})
	if err != nil {
		t.Fatalf("绑定存储缺失不应阻断渲染: %v", err)
	}
	if _, ok := r.UISpec.DataModel.Meta["surface_token"]; ok {
		t.Error("无绑定存储时不应签发 token")
	}
}
