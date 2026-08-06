package genui

import (
	"fmt"
	"strconv"
	"strings"
)

// TemplateCompose 是 Level 1 确定性生成器：不依赖 LLM，依据 DataModel 的形态
// （有无指标 / 序列 / 行集）拼装一份稳定、经过校验的 UISpec。
//
// 它既是「封装组件」能力本身，也是 Level 2 LLM 生成失败时的兜底，保证功能始终可用。
func TemplateCompose(dm *DataModel, question string) *UISpec {
	spec := &UISpec{
		Surface:   "text2sql",
		Title:     titleFor(dm, question),
		Catalog:   Catalog,
		GenMode:   GenModeTemplate,
		DataModel: dm,
	}

	rootChildren := []string{}
	add := func(c Component) {
		spec.Components = append(spec.Components, c)
		rootChildren = append(rootChildren, c.ID)
	}

	// included 记录本次实际生成了哪些视图，供概览 Callout 自然语言描述。
	included := []string{}

	// 1. 指标卡区：每个 metric 一张卡，value 绑定到 /metrics/<key>。
	if len(dm.Metrics) > 0 {
		metricRowID := "metrics_row"
		cardIDs := []string{}
		i := 0
		for key := range dm.Metrics {
			cardID := fmt.Sprintf("metric_%d", i)
			i++
			spec.Components = append(spec.Components, Component{
				ID:   cardID,
				Type: CompMetricCard,
				Props: map[string]interface{}{
					"label": key,
					"value": binding("/metrics/" + jsonPtrEscape(key)),
				},
			})
			cardIDs = append(cardIDs, cardID)
		}
		spec.Components = append(spec.Components, Component{
			ID: metricRowID, Type: CompRow, Children: cardIDs,
		})
		rootChildren = append(rootChildren, metricRowID)
		included = append(included, "关键指标")
	}

	// 2. 图表：按数据形态择一渲染。
	//    - correlation 分析 + 二维点集 → 散点图；
	//    - Meta.chart_kind 显式指定 pie/funnel（opt-in）→ 饼图/漏斗图（复用 /series）；
	//    - 分类标签（非数值/非日期、类目不多）→ 柱状图；
	//    - 其余时序序列 → 折线图（含预测段）。
	//
	// chart_kind 是「opt-in」提示：AssembleDataModel 默认不写入该键，故默认路径的
	// 图表选型（散点/柱/线）与既有输出逐字节一致；仅当上游显式设置 Meta.chart_kind
	// 时才改走饼图/漏斗图分支——PieChart/Funnel 与 BarChart 一样复用 /series（labels+actual）。
	analysisType, _ := dm.Meta["analysis_type"].(string)
	chartKind, _ := dm.Meta["chart_kind"].(string)
	switch {
	case analysisType == "correlation" && dm.Scatter != nil:
		add(Component{
			ID:   "chart",
			Type: CompScatter,
			Props: map[string]interface{}{
				"title":  scatterTitle(dm.Scatter),
				"points": binding("/scatter"),
			},
		})
		included = append(included, "相关性散点图")
	case chartKind == "pie" && dm.Series != nil && len(dm.Series.Actual) >= 2:
		add(Component{
			ID:   "chart",
			Type: CompPieChart,
			Props: map[string]interface{}{
				"title":  seriesTitle(dm.Series),
				"series": binding("/series"),
			},
		})
		included = append(included, "占比饼图")
	case chartKind == "funnel" && dm.Series != nil && len(dm.Series.Actual) >= 2:
		add(Component{
			ID:   "chart",
			Type: CompFunnel,
			Props: map[string]interface{}{
				"title":  seriesTitle(dm.Series),
				"series": binding("/series"),
			},
		})
		included = append(included, "转化漏斗图")
	case dm.Series != nil && len(dm.Series.Actual) >= 2:
		chartType := CompLineChart
		chartLabel := "趋势折线图"
		if looksCategorical(dm.Series.Labels) {
			chartType = CompBarChart
			chartLabel = "分类柱状图"
		}
		add(Component{
			ID:   "chart",
			Type: chartType,
			Props: map[string]interface{}{
				"title":  seriesTitle(dm.Series),
				"series": binding("/series"),
			},
		})
		included = append(included, chartLabel)
	}

	// 3. 数据表：始终展示真实行集。
	if dm.Table != nil && len(dm.Table.Rows) > 0 {
		add(Component{
			ID:   "table",
			Type: CompTable,
			Props: map[string]interface{}{
				"title": "查询结果",
				"data":  binding("/table"),
			},
		})
		included = append(included, "明细数据表")
	}

	// 4. 结论条（Callout）：把结果概览做成自然语言提示条，置于顶部。
	//    文本由真实 row_count / analysis_type 确定性拼装，绝不臆造数字。
	if text, tone := summarize(dm, included); text != "" {
		spec.Components = append(spec.Components, Component{
			ID:   "summary",
			Type: CompCallout,
			Props: map[string]interface{}{
				"title": "结果概览",
				"text":  text,
				"tone":  tone,
			},
		})
		rootChildren = append([]string{"summary"}, rootChildren...)
	}

	// 5. 脚注（Text）：数据来源与按引用提示，置于底部。
	if caption := captionFor(dm); caption != "" {
		spec.Components = append(spec.Components, Component{
			ID:   "footnote",
			Type: CompText,
			Props: map[string]interface{}{
				"text":    caption,
				"variant": "caption",
			},
		})
		rootChildren = append(rootChildren, "footnote")
	}

	// 6. 根容器（纵向）。
	spec.Components = append(spec.Components, Component{
		ID: RootID, Type: CompColumn, Children: rootChildren,
	})

	return spec
}

// summarize 依据 DataModel 形态拼一句确定性的结果概览文本（数字全取自真实 tool_output），
// 供顶部 Callout 展示；tone 在检出异常时升级为 warning。
func summarize(dm *DataModel, included []string) (text, tone string) {
	tone = "info"
	parts := []string{}

	if rc, ok := toInt(dm.Meta["row_count"]); ok && rc > 0 {
		if trunc, _ := dm.Meta["truncated"].(bool); trunc && dm.Table != nil {
			parts = append(parts, fmt.Sprintf("查询共命中 %d 行，下方展示前 %d 行样本", rc, len(dm.Table.Rows)))
		} else {
			parts = append(parts, fmt.Sprintf("查询共命中 %d 行", rc))
		}
	}

	switch at, _ := dm.Meta["analysis_type"].(string); at {
	case "anomaly":
		if n, ok := toInt(dm.Metrics["异常点数"]); ok && n > 0 {
			tone = "warning"
			parts = append(parts, fmt.Sprintf("检出 %d 个异常点", n))
		}
	case "correlation":
		if n, ok := toInt(dm.Metrics["显著相关对"]); ok && n > 0 {
			parts = append(parts, fmt.Sprintf("发现 %d 组显著相关", n))
		}
	}

	if len(included) > 0 {
		parts = append(parts, "已自动生成"+strings.Join(included, "、"))
	}

	return strings.Join(parts, "；"), tone
}

// captionFor 生成底部脚注文本：始终声明数字取自真实结果；被截断时追加按引用提示。
func captionFor(dm *DataModel) string {
	base := "以上数字均取自真实查询结果，未经模型加工。"
	if _, ok := dm.Meta["result_id"]; ok {
		return "完整结果集已存储，可在后续追问中按引用调用。" + base
	}
	return base
}

// toInt 尽力把 JSON 数值（number 或数字字符串）转成 int。
func toInt(v interface{}) (int, bool) {
	if f, ok := toFloat(v); ok {
		return int(f), true
	}
	return 0, false
}

func titleFor(dm *DataModel, question string) string {
	if question != "" {
		return question
	}
	if at, ok := dm.Meta["analysis_type"].(string); ok && at != "" {
		return "分析结果：" + at
	}
	return "查询结果"
}

func seriesTitle(s *SeriesData) string {
	if s.Name != "" {
		return s.Name + " 趋势"
	}
	return "趋势"
}

func scatterTitle(s *ScatterData) string {
	if s.XLabel != "" && s.YLabel != "" {
		return s.YLabel + " 与 " + s.XLabel + " 相关性"
	}
	return "相关性"
}

// looksCategorical 判断一组标签是否更适合柱状图（离散分类）而非折线图（时序）：
// 标签均为字符串、非数值、非日期形态，且类目数量适中（2–12）。
// 数值/日期标签视为连续轴，交给折线图。
func looksCategorical(labels []interface{}) bool {
	if len(labels) < 2 || len(labels) > 12 {
		return false
	}
	for _, l := range labels {
		s, ok := l.(string)
		if !ok {
			return false // 数值等非字符串标签 → 连续轴
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return false // 纯数值字符串 → 连续轴
		}
		if looksDate(s) {
			return false // 日期形态 → 时序
		}
	}
	return true
}

// looksDate 粗略判断字符串是否为日期/时间形态（含日期分隔符、中文年月，或纯数字年月）。
func looksDate(s string) bool {
	if strings.ContainsAny(s, "-/年月季周日:") {
		return true
	}
	if len(s) >= 6 && isAllDigits(s) {
		return true // 202601 / 20260131 视作时间
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// binding 构造一个 {path} 数据绑定。
func binding(pointer string) map[string]interface{} {
	return map[string]interface{}{"path": pointer}
}

// jsonPtrEscape 按 RFC6901 转义指针片段中的 ~ 和 /。
func jsonPtrEscape(s string) string {
	out := ""
	for _, r := range s {
		switch r {
		case '~':
			out += "~0"
		case '/':
			out += "~1"
		default:
			out += string(r)
		}
	}
	return out
}
