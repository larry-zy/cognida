// Package metricsql 是指标语义层的「指标引擎」：给定受治理的语义模型 bundle 与一份
// 结构化取数请求（指标 + 维度 + 过滤 + 排序），确定性地装配出治理口径下的 SQL。
//
// 它把「LLM 裸拼 SQL」升级为「按既定口径经引擎生成 SQL」：指标/维度/口径由建模侧
// 一处定义，引擎只做名称解析 + 装配，不做语义推断。请求中任一名称无法在语义模型中
// 解析（含同义词）即报告未覆盖，调用方据此回退词法 NL2SQL（get_schema + sql_execute）。
//
// 信任边界：Metric.Expr / Measure.Expr / Dimension.Expr / Relation.JoinCondition 均视为
// 建模侧（受治理、管理员权限）录入的可信 SQL 片段，直接进入 SELECT/ON 子句而不做二次
// 转义。用户可控输入只有名称（解析后丢弃）与过滤值（经 sqlLiteral 转义）+ 算子（白名单）。
// 若日后放开由低权限角色编辑指标/关系表达式，必须在写入侧（UpsertBundle）增加表达式校验。
package metricsql

import (
	"fmt"
	"sort"
	"strings"

	"link/internal/model/semantic"
)

// FilterOp 过滤算子白名单。
type FilterOp string

const (
	OpEq   FilterOp = "="
	OpNe   FilterOp = "!="
	OpGt   FilterOp = ">"
	OpGte  FilterOp = ">="
	OpLt   FilterOp = "<"
	OpLte  FilterOp = "<="
	OpLike FilterOp = "like"
	OpIn   FilterOp = "in"
)

var allowedOps = map[FilterOp]struct{}{
	OpEq: {}, OpNe: {}, OpGt: {}, OpGte: {}, OpLt: {}, OpLte: {}, OpLike: {}, OpIn: {},
}

// TimeGrain 时间智能的上卷粒度（日/周/月/季/年）。空值表示不做时间上卷。
type TimeGrain string

const (
	GrainDay     TimeGrain = "day"
	GrainWeek    TimeGrain = "week"
	GrainMonth   TimeGrain = "month"
	GrainQuarter TimeGrain = "quarter"
	GrainYear    TimeGrain = "year"
)

// Dialect SQL 方言，决定时间粒度等函数的生成形态。空值默认 MySQL。
type Dialect string

const (
	DialectMySQL    Dialect = "mysql"
	DialectPostgres Dialect = "postgres"
)

// Filter 是一条过滤条件，Field 为维度语义名（含同义词），Op 取算子白名单。
type Filter struct {
	Field  string   `json:"field"`
	Op     FilterOp `json:"op"`
	Values []string `json:"values"` // in 用多值，其余取第一个
}

// OrderKey 是一条排序键，Field 为已选的指标或维度语义名。
type OrderKey struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc"`
}

// Query 是一份结构化取数请求（由上层意图识别拆解得到）。
type Query struct {
	Metrics    []string   `json:"metrics"`
	Dimensions []string   `json:"dimensions"`
	Filters    []Filter   `json:"filters,omitempty"`
	OrderBy    []OrderKey `json:"order_by,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	// TimeGrain 时间智能上卷粒度（day/week/month/quarter/year）。非空时，引擎把选中的
	// 时间维度（或模型主时间维度）按该粒度分桶；未选任何时间维度则自动注入主时间维度。
	TimeGrain TimeGrain `json:"time_grain,omitempty"`
	// Dialect 目标数据源 SQL 方言（mysql/postgres），影响时间粒度函数生成。空则 MySQL。
	Dialect Dialect `json:"dialect,omitempty"`
}

// Coverage 报告本次请求被语义模型覆盖的情况。
type Coverage struct {
	Covered            bool     `json:"covered"`
	ResolvedMetrics    []string `json:"resolved_metrics,omitempty"`
	ResolvedDimensions []string `json:"resolved_dimensions,omitempty"`
	// Uncovered 无法解析的名称（指标/维度/过滤字段），非空即触发回退。
	Uncovered []string `json:"uncovered,omitempty"`
}

// Result 是引擎产物：治理口径下的 SQL + 覆盖报告 + 说明。
type Result struct {
	SQL      string   `json:"sql"`
	Coverage Coverage `json:"coverage"`
	Notes    []string `json:"notes,omitempty"`
}

// selectItem 是一个待装配的 SELECT 项（维度或指标解析后的统一形态）。
type selectItem struct {
	alias   string // 输出列名（语义名）
	expr    string // 物理 SQL 表达式（已按需限定表别名）
	tableID string // 归属逻辑表 ID（指标可能为空）
	isDim   bool   // 维度进 GROUP BY，指标不进
	isTime  bool   // 时间维度（可套时间粒度上卷）
}

// Build 依据语义模型 bundle 装配 SQL。任一名称未覆盖时返回 Covered=false（SQL 为空），
// 调用方据此回退词法 NL2SQL。
func Build(bundle *semantic.ModelBundle, q Query) (*Result, error) {
	if bundle == nil || bundle.Model == nil {
		return nil, fmt.Errorf("metricsql: 语义模型为空")
	}
	if len(q.Metrics) == 0 && len(q.Dimensions) == 0 {
		return nil, fmt.Errorf("metricsql: 请求需至少包含一个指标或维度")
	}

	idx := newIndex(bundle)
	idx.dialect = q.Dialect // 标识符引用按方言（空=MySQL）；与 grainExpr 的方言分桶一致
	res := &Result{}
	var uncovered []string
	// needed 累积所有已解析字段涉及的逻辑表（含跨表派生指标推断出的多张表），供 JOIN 规划。
	needed := map[string]struct{}{}

	// 1) 解析维度。
	dimItems := make([]selectItem, 0, len(q.Dimensions))
	resolvedDims := make([]string, 0, len(q.Dimensions))
	dimByAlias := map[string]selectItem{}
	selectedTime := false // 是否已选中至少一个时间维度
	for _, name := range q.Dimensions {
		d, ok := idx.dimension(name)
		if !ok {
			uncovered = append(uncovered, name)
			continue
		}
		item := selectItem{alias: d.Name, expr: qualify(idx, d.LogicalTableID, d.Expr), tableID: d.LogicalTableID, isDim: true, isTime: isTimeType(d.DataType)}
		if item.isTime {
			selectedTime = true
		}
		if item.tableID != "" {
			needed[item.tableID] = struct{}{}
		}
		dimItems = append(dimItems, item)
		dimByAlias[strings.ToLower(d.Name)] = item
		resolvedDims = append(resolvedDims, d.Name)
	}

	// 2) 解析指标（先按指标名/同义词——含递归展开的派生指标，再退化为度量名）。
	metricItems := make([]selectItem, 0, len(q.Metrics))
	resolvedMetrics := make([]string, 0, len(q.Metrics))
	metricByAlias := map[string]selectItem{}
	for _, name := range q.Metrics {
		if m, ok := idx.metric(name); ok {
			expr, err := idx.expandMetric(m)
			if err != nil {
				// 展开失败（未知引用/循环引用）视为未覆盖，回退词法 NL2SQL，并记录原因。
				uncovered = append(uncovered, name)
				res.Notes = append(res.Notes, err.Error())
				continue
			}
			tset := idx.inferTables(expr)
			item := selectItem{alias: m.Name, expr: expr, tableID: representativeTable(idx, tset)}
			for tid := range tset {
				needed[tid] = struct{}{}
			}
			metricItems = append(metricItems, item)
			metricByAlias[strings.ToLower(m.Name)] = item
			resolvedMetrics = append(resolvedMetrics, m.Name)
			continue
		}
		if ms, ok := idx.measure(name); ok {
			item := selectItem{alias: ms.Name, expr: applyAgg(ms.Aggregation, qualify(idx, ms.LogicalTableID, ms.Expr)), tableID: ms.LogicalTableID}
			if item.tableID != "" {
				needed[item.tableID] = struct{}{}
			}
			metricItems = append(metricItems, item)
			metricByAlias[strings.ToLower(ms.Name)] = item
			resolvedMetrics = append(resolvedMetrics, ms.Name)
			continue
		}
		uncovered = append(uncovered, name)
	}

	// 2.5) 时间智能：按 TimeGrain 把时间维度分桶（方言感知）。未选任何时间维度时
	//      自动注入模型主时间维度，实现「单指标按粒度自动上卷」。
	if q.TimeGrain != "" {
		if !validGrain(q.TimeGrain) {
			uncovered = append(uncovered, fmt.Sprintf("time_grain(%s)", q.TimeGrain))
		} else {
			dial := q.Dialect
			if !selectedTime {
				if d := idx.primaryTimeDimension(); d != nil {
					item := selectItem{alias: d.Name, expr: qualify(idx, d.LogicalTableID, d.Expr), tableID: d.LogicalTableID, isDim: true, isTime: true}
					if item.tableID != "" {
						needed[item.tableID] = struct{}{}
					}
					dimItems = append(dimItems, item)
					dimByAlias[strings.ToLower(d.Name)] = item
					resolvedDims = append(resolvedDims, d.Name)
				} else {
					// 请求了时间上卷但模型无任何时间维度：不能满足口径。标记未覆盖触发回退，
					// 而非静默返回一条不分桶的全时段聚合（会被误判 Covered 并写入受信缓存）。
					uncovered = append(uncovered, fmt.Sprintf("time_grain(%s)：模型无时间维度", q.TimeGrain))
				}
			}
			// 对全部时间维度套粒度桶（就地改写 expr，SELECT 与 GROUP BY 同步）。
			for i := range dimItems {
				if dimItems[i].isTime {
					dimItems[i].expr = grainExpr(dial, dimItems[i].expr, q.TimeGrain)
					dimByAlias[strings.ToLower(dimItems[i].alias)] = dimItems[i]
				}
			}
		}
	}

	// 3) 解析过滤字段（必须是维度）。
	filters := make([]genericFilter, 0, len(q.Filters))
	for _, f := range q.Filters {
		if _, ok := allowedOps[f.Op]; !ok {
			uncovered = append(uncovered, fmt.Sprintf("%s(算子%s)", f.Field, f.Op))
			continue
		}
		d, ok := idx.dimension(f.Field)
		if !ok {
			uncovered = append(uncovered, f.Field)
			continue
		}
		filters = append(filters, genericFilter{expr: qualify(idx, d.LogicalTableID, d.Expr), op: f.Op, vals: mapDimensionValues(d, f.Values)})
	}

	// 覆盖判定：任一未解析即回退，不生成部分 SQL（治理优先）。
	if len(uncovered) > 0 {
		res.Coverage = Coverage{Covered: false, ResolvedMetrics: resolvedMetrics, ResolvedDimensions: resolvedDims, Uncovered: dedup(uncovered)}
		res.Notes = append(res.Notes, "存在未被语义模型覆盖的名称，回退词法 NL2SQL")
		return res, nil
	}

	// 4) 确定基表并规划 JOIN（needed 已在解析各字段时累积，含跨表派生指标的多张表）。
	// 无法确定任何物理表（如仅有无表绑定的指标且模型多表）→ 回退。
	base := idx.chooseBase(metricItems, dimItems, needed)
	if base == "" {
		res.Coverage = Coverage{Covered: false, ResolvedMetrics: resolvedMetrics, ResolvedDimensions: resolvedDims}
		res.Notes = append(res.Notes, "无法确定基表，回退词法 NL2SQL")
		return res, nil
	}

	fromSQL, joinErr := idx.planFrom(base, needed)
	if joinErr != nil {
		res.Coverage = Coverage{Covered: false, ResolvedMetrics: resolvedMetrics, ResolvedDimensions: resolvedDims}
		res.Notes = append(res.Notes, "跨表关系不足以连接所需逻辑表，回退词法 NL2SQL："+joinErr.Error())
		return res, nil
	}

	// 5) 装配 SQL。
	sql, err := assemble(idx.dialect, dimItems, metricItems, fromSQL, filters, q.OrderBy, q.Limit, dimByAlias, metricByAlias)
	if err != nil {
		return nil, err
	}
	res.SQL = sql
	res.Coverage = Coverage{Covered: true, ResolvedMetrics: resolvedMetrics, ResolvedDimensions: resolvedDims}
	return res, nil
}

// representativeTable 从推断出的表集合里挑一个确定性代表（按别名字典序最小），
// 供 chooseBase 选基表用；完整表集合已并入 needed，JOIN 规划不依赖此代表。
func representativeTable(idx *index, tset map[string]struct{}) string {
	best := ""
	for id := range tset {
		if best == "" || idx.aliasByID[id] < idx.aliasByID[best] {
			best = id
		}
	}
	return best
}

// mapDimensionValues 把过滤值经维度 ValueMap 从业务标签翻译为物理列值（Bug C 修复）：
// 命中映射的值替换为物理枚举（如 已完成→completed），未命中的原样透传（值本就是物理值、
// 或该维度无需映射时不受影响）。归一化键做小写去空白，容忍 LLM 传入的大小写/首尾空白差异
// （中文无大小写，不受影响）。无映射或空值时零成本返回原切片。
func mapDimensionValues(d *semantic.Dimension, vals []string) []string {
	if len(d.ValueMap) == 0 || len(vals) == 0 {
		return vals
	}
	norm := make(map[string]string, len(d.ValueMap))
	for k, v := range d.ValueMap {
		norm[strings.ToLower(strings.TrimSpace(k))] = v
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		if phys, ok := norm[strings.ToLower(strings.TrimSpace(v))]; ok {
			out[i] = phys
		} else {
			out[i] = v
		}
	}
	return out
}

// genericFilter 是解析后的过滤条件（维度已限定为物理表达式）。
type genericFilter struct {
	expr string
	op   FilterOp
	vals []string
}

// assemble 拼装最终 SELECT。
func assemble(dialect Dialect, dims, metrics []selectItem, fromSQL string, filters []genericFilter, orderBy []OrderKey, limit int, dimByAlias, metricByAlias map[string]selectItem) (string, error) {
	var sb strings.Builder
	sb.WriteString("SELECT ")

	cols := make([]string, 0, len(dims)+len(metrics))
	for _, d := range dims {
		cols = append(cols, fmt.Sprintf("%s AS %s", d.expr, quoteIdent(dialect, d.alias)))
	}
	for _, m := range metrics {
		cols = append(cols, fmt.Sprintf("%s AS %s", m.expr, quoteIdent(dialect, m.alias)))
	}
	sb.WriteString(strings.Join(cols, ", "))
	sb.WriteString(" FROM ")
	sb.WriteString(fromSQL)

	if where := buildWhere(filters); where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(where)
	}

	if len(dims) > 0 {
		groupCols := make([]string, 0, len(dims))
		for _, d := range dims {
			groupCols = append(groupCols, d.expr)
		}
		sb.WriteString(" GROUP BY ")
		sb.WriteString(strings.Join(groupCols, ", "))
	}

	if ob := buildOrderBy(orderBy, dimByAlias, metricByAlias); ob != "" {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(ob)
	}

	if limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	}
	return sb.String(), nil
}

// buildWhere 拼装 WHERE 子句（值经转义，算子受白名单约束）。
func buildWhere(filters []genericFilter) string {
	if len(filters) == 0 {
		return ""
	}
	parts := make([]string, 0, len(filters))
	for _, f := range filters {
		switch f.op {
		case OpIn:
			if len(f.vals) == 0 {
				continue
			}
			quoted := make([]string, 0, len(f.vals))
			for _, v := range f.vals {
				quoted = append(quoted, sqlLiteral(v))
			}
			parts = append(parts, fmt.Sprintf("%s IN (%s)", f.expr, strings.Join(quoted, ", ")))
		case OpLike:
			if len(f.vals) == 0 {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s LIKE %s", f.expr, sqlLiteral(f.vals[0])))
		default:
			if len(f.vals) == 0 {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s %s %s", f.expr, string(f.op), sqlLiteral(f.vals[0])))
		}
	}
	return strings.Join(parts, " AND ")
}

// buildOrderBy 仅允许对已选的指标/维度排序，防止引入未治理表达式。
func buildOrderBy(keys []OrderKey, dimByAlias, metricByAlias map[string]selectItem) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		key := strings.ToLower(strings.TrimSpace(k.Field))
		var expr string
		if it, ok := dimByAlias[key]; ok {
			expr = it.expr
		} else if it, ok := metricByAlias[key]; ok {
			expr = it.expr
		} else {
			continue // 未在 SELECT 中的字段忽略，避免注入
		}
		dir := "ASC"
		if k.Desc {
			dir = "DESC"
		}
		parts = append(parts, expr+" "+dir)
	}
	return strings.Join(parts, ", ")
}

// dedup 去重且保序。
func dedup(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
