// Package tools 提供渲染工具 render_ui：把 Result Store 中的结果集（按 result_id 引用）
// 装配成 A2UI 规格（UISpec），供 handler 即时下发 `ui` SSE 事件。
//
// 设计要点（openspec: data-agent-evolution Phase 3，design.md D3「渲染即工具」）：
//   - 数据按引用：LLM 只传 result_id + 组件布局（{path} RFC6901 绑定），绝不内联数字；
//     所有数值来自 Result Store 取回的真实行集（有界样本快照，≤ DefaultSampleRows 行）。
//   - 校验守门（复用 genui.Validate）：非法 result_id / 越界 Pointer / 目录外组件一律
//     返回 error —— eino 循环会把错误作为 ToolMessage 回灌 LLM 自纠，且工具事件
//     status=error，handler 据此不推 ui 事件。
//   - 每次调用产出独立 surface（唯一 ID），一次回答可渲染多个 UI 面板。
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	agentctx "link/internal/model/agent"
	"link/internal/service/agent/genui"
	"link/internal/service/agent/resultstore"
	"link/internal/service/agent/uibinding"
)

// RenderUIRequest render_ui 工具入参。
type RenderUIRequest struct {
	// ResultID 数据来源：sql_execute / data_analysis 回传的 result_id（必填）
	ResultID string `json:"result_id" jsonschema:"required,description=数据来源的result_id（来自sql_execute等工具的返回），渲染的所有数字都取自该结果集"`

	// Title UI 面板标题（可选，建议用用户问题或结论）
	Title string `json:"title" jsonschema:"description=UI面板标题，建议用用户问题或分析结论"`

	// Components 自定义组件布局（可选）。邻接表，必须含且仅含一个 id=="root" 的节点；
	// 数据经 {"path": "/..."} RFC6901 Pointer 绑定引用，可用路径：/table /series /metrics/<名> /meta/<键>。
	// 留空则使用确定性模板布局（指标卡 + 折线图 + 数据表）。
	Components []genui.Component `json:"components" jsonschema:"description=可选的自定义组件布局（邻接表，须含唯一root节点）。组件类型仅限catalog白名单；数据用{\"path\":\"/table\"}等JSON Pointer绑定引用，禁止内联具体数字。留空用默认模板"`
}

// RenderUIResult render_ui 工具出参。
// 注意字段顺序：UISpec 放最后，LLM 侧观察被 compactObservation 截断时优先保住摘要字段；
// handler 侧经 tool_output metadata 拿到的是完整 JSON，从 ui_spec 提取规格即时下发。
type RenderUIResult struct {
	// Surface 本次渲染的 UI 面板标识（每次调用独立）
	Surface string `json:"surface"`

	// GenMode template | llm（是否使用了自定义布局）
	GenMode string `json:"gen_mode"`

	// ComponentCount 组件数
	ComponentCount int `json:"component_count"`

	// RowCount 结果集总行数
	RowCount int `json:"row_count"`

	// SnapshotRows 实际快照进 UI 的行数（有界，≤ 信封样本上限）
	SnapshotRows int `json:"snapshot_rows"`

	// Truncated 快照是否少于总行数（大结果按引用，不全量内联）
	Truncated bool `json:"truncated"`

	// Note 给 LLM 的提示（如快照截断说明）
	Note string `json:"note,omitempty"`

	// UISpec 完整 A2UI 规格（handler 提取后作为 ui 事件下发前端）
	UISpec *genui.UISpec `json:"ui_spec"`
}

// NewRenderUITool 创建 render_ui 工具；rs 结果存储、ui 绑定存储（均可为 nil）经参数注入。
// ui 由组合根经 ToolDeps.UIBinding 显式传入，取代原 uibinding 包级单例读取。
func NewRenderUITool(rs resultstore.Store, ui uibinding.Store) *TypedBaseTool[RenderUIRequest, RenderUIResult] {
	handler := func(ctx context.Context, req *RenderUIRequest) (*RenderUIResult, error) {
		return renderUI(ctx, req, rs, ui)
	}
	return NewTypedBaseTool("render_ui",
		fmt.Sprintf(`把查询/分析结果渲染成交互式 UI 面板，立即推送给用户（可多次调用，每次一个独立面板）。

用法：
- result_id：必填，来自 sql_execute 等工具返回的 result_id；所有数字都取自该结果集，你不需要也不允许内联具体数值
- title：面板标题（建议用用户问题或结论）
- components：可选自定义布局（邻接表，必须含唯一 id=="root" 节点）；留空用默认模板（指标卡+图表+数据表）

组件白名单（catalog）：%v
数据绑定：props 中用 {"path": "/table"} 形式的 RFC6901 JSON Pointer 引用真实数据，可用路径：
- /table            → {columns, rows}（有界快照）
- /series           → {labels, actual, forecast}（折线图序列）
- /metrics/<名称>    → 单个指标值
- /meta/<键>        → row_count / truncated / result_id 等元信息

校验失败（非法 result_id / 越界 Pointer / 目录外组件）会返回错误，请修正后重试。`, genui.Catalog),
		handler,
	)
}

// renderUI 执行渲染：取数 → 装配 DataModel → 组装/校验 UISpec。
func renderUI(ctx context.Context, req *RenderUIRequest, rs resultstore.Store, ui uibinding.Store) (*RenderUIResult, error) {
	if req.ResultID == "" {
		return nil, fmt.Errorf("result_id 不能为空：请先用 sql_execute 等工具取数，再用其返回的 result_id 渲染")
	}
	if rs == nil {
		return nil, fmt.Errorf("结果存储未启用，无法按 result_id 渲染")
	}

	// 1. 按引用取数（含归属校验：跨会话读取拒绝）
	owner := resultstore.OwnerKey(agentctx.MustGetTenantID(ctx), agentctx.MustGetSessionID(ctx))
	result, err := rs.Get(ctx, owner, req.ResultID)
	if errors.Is(err, resultstore.ErrNotFound) {
		return nil, fmt.Errorf("result_id %q 不存在或已过期，请重新执行查询后再渲染", req.ResultID)
	}
	if errors.Is(err, resultstore.ErrUnauthorized) {
		return nil, fmt.Errorf("result_id %q 不属于当前会话，拒绝渲染", req.ResultID)
	}
	if err != nil {
		return nil, fmt.Errorf("读取结果集失败: %w", err)
	}

	// 2. 装配 DataModel：以信封的有界样本作为数据快照（大结果不全量内联），
	//    复用 AssembleDataModel 的序列/元信息推导逻辑。
	env := resultstore.BuildEnvelope(result, resultstore.DefaultSampleRows)
	dm := assembleDataModelFromEnvelope(env)
	if dm == nil {
		return nil, fmt.Errorf("result_id %q 结果集为空，无可渲染数据", req.ResultID)
	}

	// 3. 组装 UISpec：有自定义布局走校验型 LLM 路径，否则确定性模板。
	var spec *genui.UISpec
	if len(req.Components) > 0 {
		spec = &genui.UISpec{
			Title:      req.Title,
			Catalog:    genui.Catalog,
			GenMode:    genui.GenModeLLM,
			Components: req.Components,
			DataModel:  dm,
		}
	} else {
		spec = genui.TemplateCompose(dm, req.Title)
	}
	spec.Surface = newSurfaceID()
	if req.Title != "" {
		spec.Title = req.Title
	}

	// 4. 校验守门：目录外组件 / 越界 Pointer / 结构缺陷一律拒绝（回灌 LLM 自纠）。
	if err := genui.Validate(spec); err != nil {
		return nil, fmt.Errorf("UI 规格校验失败: %v。组件类型仅限 %v，{path} 绑定须能在数据树上解析（/table /series /metrics/<名> /meta/<键>）", err, genui.Catalog)
	}

	// 5. 交互绑定状态：surface ↔ result_id + token 落 Redis（会话 TTL），
	//    支撑 Filter/Pagination 等组件回调路由；token 经 Meta 随规格下发前端。
	//    绑定存储未注入/写失败不阻断渲染，仅降级为无交互回调。
	if ui != nil {
		binding := &uibinding.Binding{
			Surface:   spec.Surface,
			TenantID:  agentctx.MustGetTenantID(ctx),
			SessionID: agentctx.MustGetSessionID(ctx),
			ResultID:  req.ResultID,
			Token:     uibinding.NewToken(),
		}
		if err := ui.Put(ctx, binding, uibinding.DefaultTTL); err == nil {
			dm.Meta["surface_token"] = binding.Token
		}
	}

	note := ""
	if env.Truncated {
		note = fmt.Sprintf("数据表仅快照前 %d 行（共 %d 行），完整结果按 result_id 引用", len(env.Samples), env.RowCount)
	}

	return &RenderUIResult{
		Surface:        spec.Surface,
		GenMode:        spec.GenMode,
		ComponentCount: len(spec.Components),
		RowCount:       env.RowCount,
		SnapshotRows:   len(env.Samples),
		Truncated:      env.Truncated,
		Note:           note,
		UISpec:         spec,
	}, nil
}

// assembleDataModelFromEnvelope 把结果信封转成 genui 期望的 sql_execute JSON 形态后
// 复用 AssembleDataModel（沿用其序列推导 / 元信息约定，保证与既有渲染路径一致）。
func assembleDataModelFromEnvelope(env *resultstore.Envelope) *genui.DataModel {
	payload := map[string]interface{}{
		"columns":   env.Columns,
		"samples":   env.Samples,
		"row_count": env.RowCount,
		"result_id": env.ResultID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	dm := genui.AssembleDataModel(string(b), "")
	if dm == nil {
		return nil
	}
	// 无论是否截断都在 Meta 暴露 result_id：交互回调（过滤/分页）与过期降级都要用它。
	dm.Meta["result_id"] = env.ResultID
	if env.Truncated {
		dm.Meta["truncated"] = true
	}
	return dm
}

// newSurfaceID 生成本次渲染的独立 surface 标识。
func newSurfaceID() string {
	return "sfc_" + uuid.NewString()[:8]
}
