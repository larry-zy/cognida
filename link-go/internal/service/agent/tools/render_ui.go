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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool/utils"
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

	// Components 自定义组件布局（可选）。含且仅含一个 id=="root" 的节点；
	// 数据经 {"path": "/..."} RFC6901 Pointer 绑定引用，可用路径：/table /series /scatter /metrics/<名> /meta/<键>。
	// 留空则使用确定性模板布局（指标卡 + 折线图 + 数据表）。
	//
	// 反序列化容忍两种形态（见 UnmarshalJSON）：扁平邻接表（children 填子节点 id 字符串）
	// 或内联嵌套树（children 直接放子组件对象）；后者会被自动摊平成邻接表。
	Components []genui.Component `json:"components" jsonschema:"description=可选的自定义组件布局（含唯一root节点；children可填子节点id数组或直接内联子组件对象，均可）。组件类型仅限catalog白名单；数据用{\"path\":\"/table\"}等JSON Pointer绑定引用，禁止内联具体数字。留空用默认模板"`
}

// looseComponent 是 genui.Component 的宽松镜像：children 允许两种 JSON 形态——
// 字符串 id 数组（标准邻接表）或内联子组件对象数组（LLM 常产出的嵌套树）。
// 用 json.RawMessage 延迟到 flattenComponents 中按实际形态分派。
type looseComponent struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Props    map[string]interface{} `json:"props,omitempty"`
	Children json.RawMessage        `json:"children,omitempty"`
}

// UnmarshalJSON 容忍 LLM 产出的两种 components 形态，统一摊平为邻接表：
//  1. 标准扁平邻接表：[{id,type,props,children:[id...]}, ...]
//  2. 内联嵌套树：单个 root 对象（或对象数组），children 直接内联子组件对象。
//
// 内联形态会被摊平成邻接表（为缺失 id 的节点自动编号，避让既有 id），
// 从而对 LLM 的结构差异鲁棒——历史 bug：LLM 传嵌套对象导致
// "cannot unmarshal object into []genui.Component"，渲染整体失败。
func (r *RenderUIRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		ResultID   string          `json:"result_id"`
		Title      string          `json:"title"`
		Components json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.ResultID = raw.ResultID
	r.Title = raw.Title
	r.Components = nil

	trimmed := bytes.TrimSpace(raw.Components)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	comps, err := flattenComponents(trimmed)
	if err != nil {
		return err
	}
	r.Components = comps
	return nil
}

// flattenComponents 把 components 原始 JSON（扁平数组或内联嵌套树）统一摊平为邻接表。
func flattenComponents(raw []byte) ([]genui.Component, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}

	// 顶层允许「组件数组」或「单个 root 对象」。
	var roots []looseComponent
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &roots); err != nil {
			return nil, fmt.Errorf("components 数组解析失败: %w", err)
		}
	case '{':
		var single looseComponent
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, fmt.Errorf("components 对象解析失败: %w", err)
		}
		// 单个 root 对象若省略 id，补成固定根 id，避免因缺 root 被 Validate 拒绝。
		if single.ID == "" {
			single.ID = genui.RootID
		}
		roots = []looseComponent{single}
	default:
		return nil, fmt.Errorf("components 须为组件数组或单个 root 节点对象")
	}

	// 先收集所有显式 id，供自动编号避让，防止生成 id 与既有 id 冲突。
	// 逐元素下钻：只有内联对象子节点才递归，字符串 id 子节点是对既有节点的引用。
	used := map[string]bool{}
	var collect func(n looseComponent)
	collect = func(n looseComponent) {
		if n.ID != "" {
			used[n.ID] = true
		}
		for _, el := range childElems(n.Children) {
			if child, ok := elemAsComponent(el); ok {
				collect(child)
			}
		}
	}
	for _, n := range roots {
		collect(n)
	}

	counter := 0
	genID := func() string {
		for {
			counter++
			id := fmt.Sprintf("c%d", counter)
			if !used[id] {
				used[id] = true
				return id
			}
		}
	}

	var out []genui.Component
	var walk func(n looseComponent) string
	walk = func(n looseComponent) string {
		id := n.ID
		if id == "" {
			id = genID()
		}
		comp := genui.Component{ID: id, Type: n.Type, Props: n.Props}
		// children 逐元素处理，容忍「字符串 id」与「内联对象」混用：
		// 字符串 → 直接作为引用；对象 → 递归摊平并回填其（生成的）id。
		for _, el := range childElems(n.Children) {
			if ref, ok := elemAsID(el); ok {
				comp.Children = append(comp.Children, ref)
			} else if child, ok := elemAsComponent(el); ok {
				comp.Children = append(comp.Children, walk(child))
			}
		}
		out = append(out, comp)
		return id
	}
	for _, n := range roots {
		walk(n)
	}
	return out, nil
}

// childElems 把 children 拆成逐个原始 JSON 元素；非数组返回 nil。
func childElems(raw json.RawMessage) []json.RawMessage {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || t[0] != '[' {
		return nil
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(t, &elems); err != nil {
		return nil
	}
	return elems
}

// elemAsID 把一个 children 元素解析为非空字符串 id 引用；非字符串返回 false。
func elemAsID(el json.RawMessage) (string, bool) {
	t := bytes.TrimSpace(el)
	if len(t) == 0 || t[0] != '"' {
		return "", false
	}
	var id string
	if err := json.Unmarshal(t, &id); err != nil || id == "" {
		return "", false
	}
	return id, true
}

// elemAsComponent 把一个 children 元素解析为内联子组件对象；非对象返回 false。
func elemAsComponent(el json.RawMessage) (looseComponent, bool) {
	t := bytes.TrimSpace(el)
	if len(t) == 0 || t[0] != '{' {
		return looseComponent{}, false
	}
	var c looseComponent
	if err := json.Unmarshal(t, &c); err != nil {
		return looseComponent{}, false
	}
	return c, true
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
	t := NewTypedBaseTool("render_ui",
		fmt.Sprintf(`把查询/分析结果渲染成交互式 UI 面板，立即推送给用户（可多次调用，每次一个独立面板）。

用法：
- result_id：必填，来自 sql_execute 等工具返回的 result_id；所有数字都取自该结果集，你不需要也不允许内联具体数值
- title：面板标题（建议用用户问题或结论）
- components：可选自定义布局，必须含唯一 id=="root" 节点。两种写法皆可：扁平邻接表（每个节点的 children 填子节点 id 字符串数组）或内联嵌套树（children 直接放子组件对象，后端自动摊平）；留空用默认模板（指标卡+图表+数据表）

组件白名单（catalog）：%v
数据绑定：props 中用 {"path": "/table"} 形式的 RFC6901 JSON Pointer 引用真实数据，可用路径：
- /table            → {columns, rows}（有界快照）
- /series           → {labels, actual, forecast}（LineChart 时序 / BarChart 分类对比）
- /scatter          → {x, y, xLabel, yLabel}（ScatterChart 两数值列相关性，仅在有该路径时）
- /metrics/<名称>    → 单个指标值
- /meta/<键>        → row_count / truncated / result_id 等元信息

做「有解读」的富布局（别只堆指标卡+表格）：首个组件恒用 Callout 写一句关键结论
（props: title, text, tone=info|success|warning|error），必要时用 Text 补叙述/小标题（variant=title|subtitle|body|caption）。
主承载组件按场景选型：
- 时序趋势 → LineChart（series 绑定 /series，含 forecast）
- 分类对比 / 归因 drivers 排序 → BarChart
- 两数值列相关 → ScatterChart（绑定 /scatter）
- 单值/单指标 → MetricCard
- 明细行集 → Table；行多或可按维筛选时加 Filter、Pagination（action.name=filter/paginate，前端会回源续跑）
- 异常场景 → Callout 用 tone=warning 点明异常
数字一律用 {path} 绑定，禁止内联具体数值。

自定义布局示例（扁平邻接表，text 是解读、数字全走 {path} 绑定）：
[
  {"id":"root","type":"Column","children":["insight","kpis","trend","tbl"]},
  {"id":"insight","type":"Callout","props":{"title":"结论","text":"近 6 个月销售额稳步上升","tone":"success"}},
  {"id":"kpis","type":"Row","children":["m1"]},
  {"id":"m1","type":"MetricCard","props":{"label":"总销售额","value":{"path":"/metrics/总销售额"}}},
  {"id":"trend","type":"LineChart","props":{"title":"销售额趋势","series":{"path":"/series"}}},
  {"id":"tbl","type":"Table","props":{"title":"明细","data":{"path":"/table"}}}
]

校验失败（非法 result_id / 越界 Pointer / 目录外组件）会返回错误，请修正后重试。`, genui.Catalog),
		handler,
	)
	// 注入机器可读的参数 schema：由入参 struct + jsonschema tag 反射生成，
	// 下发给 LLM 的 function parameters 里 components 是「对象数组」而非自由文本，
	// 让 LLM 在格式层就遵循邻接表形状（修复其误当嵌套树内联 children 的偏差）。
	if p, err := utils.GoStruct2ParamsOneOf[RenderUIRequest](); err == nil {
		t.BaseTool.ParamsOneOf = p
	}
	return t
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
		return nil, fmt.Errorf("UI 规格校验失败: %v。组件类型仅限 %v，{path} 绑定须能在数据树上解析（/table /series /scatter /metrics/<名> /meta/<键>）", err, genui.Catalog)
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
