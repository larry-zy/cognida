// Package genui 在 Go 端把 Text2SQL 的真实取数结果（sql_execute）与量化分析结果
// （data_analysis）装配成一份「生成式 UI」契约（A2UI 子集），供前端渲染器直接渲染。
//
// 设计要点（见 memory: py-compute-go-backend）：
//   - Python 只负责计算；UI 契约的拼装、校验、catalog 白名单全部在 Go 端完成。
//   - 数据按引用（data-by-reference）：LLM 只产出「组件布局 + {path} 绑定」，
//     绝不产出具体数字；所有数值来自可信的 dataModel（由真实 tool_output 解析而来），
//     从根本上杜绝数字幻觉。
//   - 两级生成：
//       Level 1 template —— 确定性模板，无需 LLM，永远可用（封装组件）。
//       Level 2 llm      —— LLM 定制布局，经 catalog + 绑定校验；任何失败回退 Level 1。
package genui

// 组件类型白名单（catalog）。前端渲染器只认这些 type，未知 type 一律降级/丢弃。
// 每个 type 对应前端一个自研 Ui* 组件或图表：
//
//	Column/Row  → 布局容器（纵/横）
//	Text        → 标题/正文（props: text | {path}, variant）
//	MetricCard  → 指标卡（props: label, value{path}, unit, trend）
//	Callout     → 结论/提示条（props: title, text, tone）
//	Table        → 数据表（props: data{path} → {columns, rows}）—— 融合 sql_execute 行集
//	LineChart    → 折线图（props: title, series{path} → {labels, actual, forecast}）—— 时序趋势
//	BarChart     → 柱状图（props: title, series{path} → {labels, actual}）—— 分类对比（复用序列）
//	ScatterChart → 散点图（props: title, points{path} → {x, y, xLabel, yLabel}）—— 两数值列相关性
//
// 交互组件（回调驱动后续轮次，action 经 SSE 之外的 follow-up 通道回传）：
//
//	Button      → 动作按钮（props: label, action{name, params}, variant）
//	Confirm     → 危险操作确认（props: title, text, pending_action_id, confirmLabel, cancelLabel）
//	Form        → 参数表单（props: fields[{name,label,type,default}], submitAction{name}）
//	Filter      → 过滤器（props: field, options|{path}, action{name}）
//	Pagination  → 分页（props: page, pageSize, total|{path}, action{name}）
const (
	CompColumn     = "Column"
	CompRow        = "Row"
	CompText       = "Text"
	CompMetricCard = "MetricCard"
	CompCallout    = "Callout"
	CompTable      = "Table"
	CompLineChart  = "LineChart"
	CompBarChart   = "BarChart"
	CompScatter    = "ScatterChart"

	// 交互组件
	CompButton     = "Button"
	CompConfirm    = "Confirm"
	CompForm       = "Form"
	CompFilter     = "Filter"
	CompPagination = "Pagination"
)

// Catalog 是允许出现在规格中的组件类型全集。
var Catalog = []string{
	CompColumn, CompRow, CompText, CompMetricCard, CompCallout, CompTable,
	CompLineChart, CompBarChart, CompScatter,
	CompButton, CompConfirm, CompForm, CompFilter, CompPagination,
}

// InteractiveCatalog 是交互组件子集（产生回调、驱动后续轮次的组件）。
var InteractiveCatalog = []string{
	CompButton, CompConfirm, CompForm, CompFilter, CompPagination,
}

// RootID 是组件邻接表的固定根节点 id。
const RootID = "root"

// 生成模式标记，透传给前端便于调试与埋点。
const (
	GenModeTemplate = "template"
	GenModeLLM      = "llm"
)

// Component 是邻接表中的一个组件节点。
// Props 中的值可以是字面量，也可以是数据绑定 {"path": "/table"}（RFC6901 JSON Pointer）。
type Component struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Props    map[string]interface{} `json:"props,omitempty"`
	Children []string               `json:"children,omitempty"`
}

// UISpec 是下发给前端的完整 UI 契约信封。
type UISpec struct {
	Surface    string      `json:"surface"`           // surface 标识（一次回答一个）
	Title      string      `json:"title,omitempty"`   // 顶部标题
	Catalog    []string    `json:"catalog"`           // 组件白名单
	GenMode    string      `json:"genMode"`           // template | llm
	Components []Component `json:"components"`         // 邻接表，含且仅含一个 id=="root"
	DataModel  *DataModel  `json:"dataModel"`          // 真实数据树（数字唯一来源）
}

// DataModel 是被 {path} 绑定引用的真实数据树。所有字段都来自解析真实 tool_output，
// 不含任何 LLM 生成的数字。
type DataModel struct {
	Table   *TableData             `json:"table,omitempty"`   // 来自 sql_execute
	Metrics map[string]interface{} `json:"metrics,omitempty"` // 来自 data_analysis 的关键指标（扁平）
	Series  *SeriesData            `json:"series,omitempty"`  // 供折线图/柱状图使用的一维序列
	Scatter *ScatterData           `json:"scatter,omitempty"` // 供散点图使用的二维点集（两数值列）
	Meta    map[string]interface{} `json:"meta,omitempty"`    // analysis_type / row_count / executed_sql 等
}

// TableData 是 sql_execute 的行集（列名 + 记录数组）。
type TableData struct {
	Columns []string                 `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
}

// SeriesData 是一维序列：actual 为真实观测，forecast 为分析预测（可空）。
// 折线图按时序渲染；柱状图把 labels 当分类、actual 当柱高——两者共用本结构。
type SeriesData struct {
	Name     string        `json:"name,omitempty"`
	Labels   []interface{} `json:"labels,omitempty"`
	Actual   []float64     `json:"actual,omitempty"`
	Forecast []float64     `json:"forecast,omitempty"`
}

// ScatterData 是散点图的二维点集：X/Y 一一对应，取自行集中的前两个数值列，
// 用于展示两个数值变量的相关性（配合 correlation 分析）。
type ScatterData struct {
	Name   string    `json:"name,omitempty"`
	XLabel string    `json:"xLabel,omitempty"`
	YLabel string    `json:"yLabel,omitempty"`
	X      []float64 `json:"x,omitempty"`
	Y      []float64 `json:"y,omitempty"`
}
