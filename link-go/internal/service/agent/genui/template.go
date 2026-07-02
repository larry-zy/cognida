package genui

import "fmt"

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
	}

	// 2. 折线图：有 actual 序列时渲染（含预测段）。
	if dm.Series != nil && len(dm.Series.Actual) >= 2 {
		add(Component{
			ID:   "chart",
			Type: CompLineChart,
			Props: map[string]interface{}{
				"title":  seriesTitle(dm.Series),
				"series": binding("/series"),
			},
		})
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
	}

	// 4. 根容器（纵向）。
	spec.Components = append(spec.Components, Component{
		ID: RootID, Type: CompColumn, Children: rootChildren,
	})

	return spec
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
