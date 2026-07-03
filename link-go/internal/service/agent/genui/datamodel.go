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

	// 序列：优先用分析结果给出的 value_col / time_col 与 forecast，否则从行集启发式推断。
	dm.Series = buildSeries(sqlOut, an, hasAnalysis)
	return dm
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
