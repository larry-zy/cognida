package genui

import (
	"encoding/json"
	"strconv"
)

// sqlExecuteOutput 对应 tools.SQLExecuteResult 的信封 JSON 形态（仅取需要的字段）。
//
// 数据按引用（data-by-reference）改造后，sql_execute 的 tool_output 不再回传全量行，
// 而是「结果信封」：samples 为有界样本（默认 ≤20 行），完整结果集存于 Result Store，
// 由 result_id 索引。genUI 以样本作为「有界数据快照」渲染；样本被截断时通过 Meta
// 暴露 result_id 与 truncated，供前端/后续 Phase 3 的按引用回放升级。
type sqlExecuteOutput struct {
	Columns     []string                 `json:"columns"`
	Samples     []map[string]interface{} `json:"samples"`
	RowCount    int                      `json:"row_count"`
	ResultID    string                   `json:"result_id"`
	ExecutedSQL string                   `json:"executed_sql"`
}

// analysisOutput 对应 data_analysis 工具的 JSON 形态：{analysis_type, success, data}。
type analysisOutput struct {
	AnalysisType string                 `json:"analysis_type"`
	Success      bool                   `json:"success"`
	Data         map[string]interface{} `json:"data"`
}

// AssembleDataModel 把真实的 sql_execute 信封与（可选的）data_analysis 输出解析、
// 融合成一份 DataModel。sqlJSON 为空或无样本时返回 nil（无可展示数据）。
//
// 关键约束：本函数是 dataModel 中数字的唯一来源，全部取自解析结果，绝不臆造。
// 渲染以信封的 samples（有界快照）为准；当 row_count 超过样本数（truncated）时，
// Meta 携带 result_id 供按引用回放完整结果集。
func AssembleDataModel(sqlJSON, analysisJSON string) *DataModel {
	var sqlOut sqlExecuteOutput
	if sqlJSON == "" || json.Unmarshal([]byte(sqlJSON), &sqlOut) != nil {
		return nil
	}
	if len(sqlOut.Samples) == 0 {
		return nil
	}

	dm := &DataModel{
		Table: &TableData{Columns: sqlOut.Columns, Rows: sqlOut.Samples},
		Meta:  map[string]interface{}{},
	}
	dm.Meta["row_count"] = sqlOut.RowCount
	if truncated := sqlOut.RowCount > len(sqlOut.Samples); truncated {
		dm.Meta["truncated"] = true
		if sqlOut.ResultID != "" {
			dm.Meta["result_id"] = sqlOut.ResultID
		}
	}
	if sqlOut.ExecutedSQL != "" {
		dm.Meta["executed_sql"] = sqlOut.ExecutedSQL
	}

	var an analysisOutput
	hasAnalysis := analysisJSON != "" && json.Unmarshal([]byte(analysisJSON), &an) == nil && an.Success
	if hasAnalysis {
		dm.Meta["analysis_type"] = an.AnalysisType
		dm.Metrics = flattenMetrics(an)
	}

	// 单行结果（如 KPI 汇总查询）：数值列派生为标量指标，供 MetricCard 绑定 /metrics/<列>。
	// 已有的分析指标优先，不被行派生覆盖。
	if len(sqlOut.Samples) == 1 {
		if dm.Metrics == nil {
			dm.Metrics = map[string]interface{}{}
		}
		for k, v := range deriveRowMetrics(sqlOut) {
			if _, exists := dm.Metrics[k]; !exists {
				dm.Metrics[k] = v
			}
		}
	}

	// 序列：优先用分析结果给出的 value_col / time_col 与 forecast，否则从行集启发式推断。
	dm.Series = buildSeries(sqlOut, an, hasAnalysis)
	// 散点：行集含 ≥2 个数值列时构造二维点集（供散点图/相关性可视化）。
	dm.Scatter = buildScatter(sqlOut)
	return dm
}

// AssembleReportDataModel 把一次回答里的「多个」sql_execute 信封与「多个」data_analysis
// 输出融合成一份 DataModel，用于报告/综合解读这类会跑多段查询的场景：
//   - 取样本最多的（多行优先）结果作为主表/序列/散点；
//   - 其余单行结果的数值列派生为标量指标（/metrics/<列>）——这是 KPI 汇总卡的数据来源；
//   - 叠加各 data_analysis 提炼的语义指标（同名以分析结果为准）。
//
// 与单结果的 AssembleDataModel 一致：所有数字均取自真实工具输出，绝不臆造；
// 无任何可展示数据时返回 nil。
//
// 说明：不跨结果集把某次 data_analysis 的 value_col/forecast 强行套到主表上
// （二者未必对应同一 result_id），主表的序列/散点仅按其自身行集形态推断，避免错配。
func AssembleReportDataModel(sqlJSONs, analysisJSONs []string) *DataModel {
	var results []sqlExecuteOutput
	for _, s := range sqlJSONs {
		var o sqlExecuteOutput
		if s == "" || json.Unmarshal([]byte(s), &o) != nil || len(o.Samples) == 0 {
			continue
		}
		results = append(results, o)
	}
	var analyses []analysisOutput
	for _, s := range analysisJSONs {
		var a analysisOutput
		if s == "" || json.Unmarshal([]byte(s), &a) != nil || !a.Success {
			continue
		}
		analyses = append(analyses, a)
	}
	if len(results) == 0 && len(analyses) == 0 {
		return nil
	}

	// 主结果 = 样本最多者（多行优先，作为主表/序列/散点）。
	primaryIdx := -1
	for i := range results {
		if primaryIdx == -1 || len(results[i].Samples) > len(results[primaryIdx].Samples) {
			primaryIdx = i
		}
	}

	dm := &DataModel{Meta: map[string]interface{}{}}
	metrics := map[string]interface{}{}

	if primaryIdx >= 0 {
		primary := results[primaryIdx]
		dm.Table = &TableData{Columns: primary.Columns, Rows: primary.Samples}
		dm.Meta["row_count"] = primary.RowCount
		if primary.RowCount > len(primary.Samples) {
			dm.Meta["truncated"] = true
			if primary.ResultID != "" {
				dm.Meta["result_id"] = primary.ResultID
			}
		}
		if primary.ExecutedSQL != "" {
			dm.Meta["executed_sql"] = primary.ExecutedSQL
		}
		dm.Series = buildSeries(primary, analysisOutput{}, false)
		dm.Scatter = buildScatter(primary)
	}

	// 非主结果里的单行结果 → 数值列派生标量指标。
	for i := range results {
		if i == primaryIdx {
			continue
		}
		for k, v := range deriveRowMetrics(results[i]) {
			metrics[k] = v
		}
	}
	// 主结果本身即单行（无多行结果时）：其数值列也派生为指标。
	if primaryIdx >= 0 && len(results[primaryIdx].Samples) == 1 {
		for k, v := range deriveRowMetrics(results[primaryIdx]) {
			if _, exists := metrics[k]; !exists {
				metrics[k] = v
			}
		}
	}

	// 分析提炼的语义指标叠加（同名以分析为准）。
	for _, a := range analyses {
		for k, v := range flattenMetrics(a) {
			metrics[k] = v
		}
	}
	// 仅当恰有一次分析时才标注 analysis_type（多次分析无法确定主图口径，交由数据形态决定）。
	if len(analyses) == 1 {
		dm.Meta["analysis_type"] = analyses[0].AnalysisType
	}
	if len(metrics) > 0 {
		dm.Metrics = metrics
	}

	if dm.Table == nil && len(dm.Metrics) == 0 && dm.Series == nil && dm.Scatter == nil {
		return nil
	}
	return dm
}

// deriveRowMetrics 从「恰好一行」的结果里把数值列扁平成标量指标（列名 → 数值）。
// 非单行结果、非数值列一律跳过——MetricCard 的 value 需要标量，行集/多行表格不适用。
func deriveRowMetrics(o sqlExecuteOutput) map[string]interface{} {
	m := map[string]interface{}{}
	if len(o.Samples) != 1 {
		return m
	}
	row := o.Samples[0]
	for _, col := range o.Columns {
		if f, ok := toFloat(row[col]); ok {
			m[col] = f
		}
	}
	return m
}

// buildScatter 从行集的前两个数值列构造散点图点集（X=首个数值列，Y=次个数值列）。
// 少于两个数值列，或有效点不足 3 个时返回 nil（不构造）。
func buildScatter(sqlOut sqlExecuteOutput) *ScatterData {
	xCol, yCol := "", ""
	for _, col := range sqlOut.Columns {
		if _, ok := toFloat(sqlOut.Samples[0][col]); !ok {
			continue
		}
		if xCol == "" {
			xCol = col
		} else {
			yCol = col
			break
		}
	}
	if xCol == "" || yCol == "" {
		return nil
	}
	s := &ScatterData{Name: yCol + " vs " + xCol, XLabel: xCol, YLabel: yCol}
	for _, row := range sqlOut.Samples {
		x, xok := toFloat(row[xCol])
		y, yok := toFloat(row[yCol])
		if xok && yok {
			s.X = append(s.X, x)
			s.Y = append(s.Y, y)
		}
	}
	if len(s.X) < 3 {
		return nil
	}
	return s
}

// flattenMetrics 依 analysis_type 从 data_analysis 结果中挑出关键指标，扁平成
// {name: value}，供 MetricCard 通过 /metrics/<name> 绑定。
func flattenMetrics(an analysisOutput) map[string]interface{} {
	m := map[string]interface{}{}
	d := an.Data
	if d == nil {
		return m
	}
	switch an.AnalysisType {
	case "trend":
		if trend, ok := d["trend"].(map[string]interface{}); ok {
			putNum(m, "方向", trend["direction"])
			putNum(m, "斜率", trend["slope"])
			putNum(m, "拟合优度", trend["r_squared"])
			putNum(m, "显著性p", trend["p_value"])
			if s, ok := trend["strength"]; ok {
				m["趋势强度"] = s
			}
		}
		if growth, ok := d["growth"].(map[string]interface{}); ok {
			putNum(m, "CAGR", growth["cagr"])
			putNum(m, "环比", growth["period_over_period"])
		}
	case "anomaly":
		putNum(m, "异常点数", d["anomaly_count"])
		putNum(m, "样本数", d["row_count"])
		if method, ok := d["method"]; ok {
			m["方法"] = method
		}
	case "describe":
		// describe 通常返回 per-column 统计，取首列的关键量作为概览。
		if stats, ok := firstColumnStats(d); ok {
			putNum(m, "均值", stats["mean"])
			putNum(m, "中位数", stats["median"])
			putNum(m, "标准差", stats["std"])
			putNum(m, "最小值", stats["min"])
			putNum(m, "最大值", stats["max"])
		}
	case "correlation":
		if pairs, ok := d["significant_pairs"].([]interface{}); ok {
			m["显著相关对"] = len(pairs)
		}
	case "insight":
		putNum(m, "指标数", d["metric_count"])
		if recs, ok := d["recommendations"].([]interface{}); ok {
			m["建议条数"] = len(recs)
		}
	default:
		// 未知类型：把顶层的数值字段扁平进来，尽量不丢信息。
		for k, v := range d {
			if isScalar(v) {
				m[k] = v
			}
		}
	}
	return m
}

// buildSeries 构造折线图序列：
//   - value/time 列：分析结果指定优先，否则从行集自动识别（首个数值列 / 首个时间或非数值列）。
//   - actual 取自真实行集；forecast 取自分析结果（若有）。
func buildSeries(sqlOut sqlExecuteOutput, an analysisOutput, hasAnalysis bool) *SeriesData {
	valueCol, timeCol := "", ""
	if hasAnalysis && an.Data != nil {
		valueCol, _ = an.Data["value_col"].(string)
		timeCol, _ = an.Data["time_col"].(string)
	}
	if valueCol == "" {
		valueCol = firstNumericColumn(sqlOut)
	}
	if valueCol == "" {
		return nil // 无数值列，不构造序列
	}
	if timeCol == "" {
		timeCol = firstNonNumericColumn(sqlOut, valueCol)
	}

	s := &SeriesData{Name: valueCol}
	for _, row := range sqlOut.Samples {
		if f, ok := toFloat(row[valueCol]); ok {
			s.Actual = append(s.Actual, f)
			if timeCol != "" {
				s.Labels = append(s.Labels, row[timeCol])
			}
		}
	}
	if len(s.Actual) < 2 {
		return nil
	}
	// 预测序列（来自 data_analysis.trend.forecast，元素可能是 number 或 {value:...}）。
	if hasAnalysis && an.Data != nil {
		if fc, ok := an.Data["forecast"].([]interface{}); ok {
			for _, item := range fc {
				if f, ok := toFloat(item); ok {
					s.Forecast = append(s.Forecast, f)
					continue
				}
				if obj, ok := item.(map[string]interface{}); ok {
					if f, ok := toFloat(obj["value"]); ok {
						s.Forecast = append(s.Forecast, f)
					}
				}
			}
		}
	}
	return s
}

// ---- 列类型启发式 ----

func firstNumericColumn(sqlOut sqlExecuteOutput) string {
	for _, col := range sqlOut.Columns {
		if _, ok := toFloat(sqlOut.Samples[0][col]); ok {
			return col
		}
	}
	return ""
}

func firstNonNumericColumn(sqlOut sqlExecuteOutput, exclude string) string {
	for _, col := range sqlOut.Columns {
		if col == exclude {
			continue
		}
		if _, ok := toFloat(sqlOut.Samples[0][col]); !ok {
			return col
		}
	}
	return ""
}

// firstColumnStats 从 describe 结果里取出第一列的统计字典。
func firstColumnStats(d map[string]interface{}) (map[string]interface{}, bool) {
	if cols, ok := d["columns"].(map[string]interface{}); ok {
		for _, v := range cols {
			if stats, ok := v.(map[string]interface{}); ok {
				return stats, true
			}
		}
	}
	// 兼容顶层直接是统计量的情况
	if _, ok := d["mean"]; ok {
		return d, true
	}
	return nil, false
}

// ---- 小工具 ----

func putNum(m map[string]interface{}, key string, v interface{}) {
	if v == nil {
		return
	}
	m[key] = v
}

func isScalar(v interface{}) bool {
	switch v.(type) {
	case float64, int, int64, string, bool:
		return true
	}
	return false
}

// toFloat 尽力把 JSON 反序列化后的值转成 float64（number 或数字字符串）。
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}
