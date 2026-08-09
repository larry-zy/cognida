package metricsql

import (
	"fmt"
	"regexp"
	"strings"

	"cognida/internal/model/semantic"
)

// metricRefRe 匹配派生指标表达式中对其它指标/度量的引用：{指标名} / {度量名}。
var metricRefRe = regexp.MustCompile(`\{([^{}]+)\}`)

// index 是语义模型 bundle 的解析索引：把名称/同义词映射到构件，并缓存逻辑表别名。
type index struct {
	bundle  *semantic.ModelBundle
	dialect Dialect // 目标方言，决定标识符引用（`` vs ""）；空视作 MySQL

	dimByKey  map[string]*semantic.Dimension
	measByKey map[string]*semantic.Measure
	metByKey  map[string]*semantic.Metric

	tableByID    map[string]*semantic.LogicalTable
	aliasByID    map[string]string // 逻辑表 ID → SQL 别名
	measNameByID map[string][]*semantic.Measure
}

// newIndex 构建解析索引。名称与同义词统一小写去空白作为键。
func newIndex(b *semantic.ModelBundle) *index {
	idx := &index{
		bundle:       b,
		dimByKey:     map[string]*semantic.Dimension{},
		measByKey:    map[string]*semantic.Measure{},
		metByKey:     map[string]*semantic.Metric{},
		tableByID:    map[string]*semantic.LogicalTable{},
		aliasByID:    map[string]string{},
		measNameByID: map[string][]*semantic.Measure{},
	}
	for i, t := range b.LogicalTables {
		idx.tableByID[t.ID] = b.LogicalTables[i]
		idx.aliasByID[t.ID] = tableAlias(t, i)
	}
	for i, d := range b.Dimensions {
		idx.dimByKey[key(d.Name)] = b.Dimensions[i]
		for _, s := range d.Synonyms {
			idx.dimByKey[key(s)] = b.Dimensions[i]
		}
	}
	for i, m := range b.Measures {
		idx.measByKey[key(m.Name)] = b.Measures[i]
		idx.measNameByID[m.LogicalTableID] = append(idx.measNameByID[m.LogicalTableID], b.Measures[i])
	}
	for i, m := range b.Metrics {
		idx.metByKey[key(m.Name)] = b.Metrics[i]
		for _, s := range m.Synonyms {
			idx.metByKey[key(s)] = b.Metrics[i]
		}
	}
	return idx
}

func (idx *index) dimension(name string) (*semantic.Dimension, bool) {
	d, ok := idx.dimByKey[key(name)]
	return d, ok
}

func (idx *index) measure(name string) (*semantic.Measure, bool) {
	m, ok := idx.measByKey[key(name)]
	return m, ok
}

func (idx *index) metric(name string) (*semantic.Metric, bool) {
	m, ok := idx.metByKey[key(name)]
	return m, ok
}

// primaryTimeDimension 返回模型的主时间维度：按 bundle 顺序取首个时间类型维度，
// 供 time_grain 在未显式选择时间维度时自动注入。无则返回 nil。
func (idx *index) primaryTimeDimension() *semantic.Dimension {
	for i, d := range idx.bundle.Dimensions {
		if isTimeType(d.DataType) {
			return idx.bundle.Dimensions[i]
		}
	}
	return nil
}

// expandMetric 递归展开派生/复合指标 Expr 中的 {指标名}/{度量名} 引用，直至无引用，
// 得到最终可直接进 SELECT 的物理 SQL 片段。seen 记录当前展开路径上的指标（按归一化名）
// 用于环检测：A→B→A 立即报错，避免无限展开。
//
// 信任边界：{ref} 里的名称经语义模型解析后被丢弃，替换进来的是被引指标/度量本身受治理的
// Expr（同样是建模侧录入的可信片段），故展开结果仍全部来自治理表达式，不含用户输入。
func (idx *index) expandMetric(m *semantic.Metric) (string, error) {
	return idx.expandExpr(m.Name, m.Expr, map[string]bool{})
}

// expandExpr 展开单个表达式：把 {name} 替换为被引指标（递归展开、括号包裹）或度量
// （套默认聚合 + 限定表别名）的表达式。任一引用无法解析或成环即返回 error。
func (idx *index) expandExpr(selfName, expr string, seen map[string]bool) (string, error) {
	k := key(selfName)
	if seen[k] {
		return "", fmt.Errorf("派生指标存在循环引用: %s", selfName)
	}
	seen[k] = true
	defer delete(seen, k)

	var expandErr error
	out := metricRefRe.ReplaceAllStringFunc(expr, func(tok string) string {
		if expandErr != nil {
			return tok
		}
		name := strings.TrimSpace(tok[1 : len(tok)-1])
		if ref, ok := idx.metric(name); ok {
			sub, err := idx.expandExpr(ref.Name, ref.Expr, seen)
			if err != nil {
				expandErr = err
				return tok
			}
			return "(" + sub + ")"
		}
		if ms, ok := idx.measure(name); ok {
			return "(" + applyAgg(ms.Aggregation, qualify(idx, ms.LogicalTableID, ms.Expr)) + ")"
		}
		expandErr = fmt.Errorf("派生指标 %s 引用了未知指标/度量: %s", selfName, name)
		return tok
	})
	if expandErr != nil {
		return "", expandErr
	}
	return out, nil
}

// inferTables 从（已展开的）指标表达式推断其涉及的全部逻辑表：优先按逻辑表别名的
// `alias.` 限定前缀扫描（覆盖跨表派生指标，如毛利同时引用 order_items 与 products），
// 其次按度量名子串，最后单表模型兜底。返回可能为空（由 chooseBase 兜底/回退）。
func (idx *index) inferTables(expr string) map[string]struct{} {
	lower := strings.ToLower(expr)
	out := map[string]struct{}{}
	for id, alias := range idx.aliasByID {
		if alias == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(alias)+".") {
			out[id] = struct{}{}
		}
	}
	if len(out) == 0 {
		for _, ms := range idx.bundle.Measures {
			if ms.LogicalTableID != "" && strings.Contains(lower, strings.ToLower(ms.Name)) {
				out[ms.LogicalTableID] = struct{}{}
			}
		}
	}
	if len(out) == 0 && len(idx.bundle.LogicalTables) == 1 {
		out[idx.bundle.LogicalTables[0].ID] = struct{}{}
	}
	return out
}

// tableHasMeasure 判定逻辑表是否为「事实表」：拥有至少一个度量即视作事实表（度量是
// 可聚合的原子列，只挂在事实表上）。用于 D3——基表/代表表选取以「声明的事实表」为准，
// 而非别名字典序的偶然结果。
func (idx *index) tableHasMeasure(id string) bool {
	return len(idx.measNameByID[id]) > 0
}

// preferFact 在候选表集合里挑一个确定性代表：先在「事实表（有度量）」中取别名字典序最小，
// 若无事实表再退回全体别名字典序最小。把 D3 的选取从「纯字典序」升级为「事实表优先、
// 字典序仅作同类内定序」，避免维表因别名恰好更小被误当基表。
func (idx *index) preferFact(cands map[string]struct{}) string {
	var factBest, anyBest string
	for id := range cands {
		if anyBest == "" || idx.aliasByID[id] < idx.aliasByID[anyBest] {
			anyBest = id
		}
		if idx.tableHasMeasure(id) {
			if factBest == "" || idx.aliasByID[id] < idx.aliasByID[factBest] {
				factBest = id
			}
		}
	}
	if factBest != "" {
		return factBest
	}
	return anyBest
}

// chooseBase 选基表：优先取某个指标/度量归属的表（事实表），否则取首个维度的表，
// 否则取模型唯一逻辑表。needed 为所有已解析字段涉及的表集合。
func (idx *index) chooseBase(metrics, dims []selectItem, needed map[string]struct{}) string {
	for _, m := range metrics {
		if m.tableID != "" {
			return m.tableID
		}
	}
	for _, d := range dims {
		if d.tableID != "" {
			return d.tableID
		}
	}
	if len(idx.bundle.LogicalTables) == 1 {
		return idx.bundle.LogicalTables[0].ID
	}
	// D3：needed 里取确定性代表——事实表优先，同类内按别名字典序。
	return idx.preferFact(needed)
}

// planFrom 从基表出发，用已定义的 Relation 连接所有 needed 表，生成 FROM+JOIN 片段。
// 连接不足以覆盖 needed 时返回错误（调用方据此回退）。
func (idx *index) planFrom(base string, needed map[string]struct{}) (string, error) {
	baseTable := idx.tableByID[base]
	if baseTable == nil {
		return "", fmt.Errorf("基表 %s 不存在于模型", base)
	}
	included := map[string]bool{base: true}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s", quoteIdent(idx.dialect, baseTable.PhysicalTable), idx.aliasByID[base]))

	// 反复扫描 relations，把与已含表相连的表接进来，直至无新增。
	for {
		added := false
		for _, rel := range idx.bundle.Relations {
			l, r := rel.LeftTableID, rel.RightTableID
			var newID, knownID string
			switch {
			case included[l] && !included[r]:
				knownID, newID = l, r
			case included[r] && !included[l]:
				knownID, newID = r, l
			default:
				continue
			}
			// 仅在新表确属所需，或作为连接桥梁时接入。
			nt := idx.tableByID[newID]
			if nt == nil {
				continue
			}
			joinKw := joinKeyword(rel.JoinType)
			sb.WriteString(fmt.Sprintf(" %s %s %s ON %s", joinKw, quoteIdent(idx.dialect, nt.PhysicalTable), idx.aliasByID[newID], rel.JoinCondition))
			included[newID] = true
			added = true
			_ = knownID
		}
		if !added {
			break
		}
	}

	// 校验所有 needed 表都已连接。
	for id := range needed {
		if !included[id] {
			t := idx.tableByID[id]
			nm := id
			if t != nil {
				nm = t.Name
			}
			return "", fmt.Errorf("逻辑表 %s 无可用关系连接到基表", nm)
		}
	}
	return sb.String(), nil
}

// qualify 若 expr 是裸列名（无空格/括号/点号）则用逻辑表别名限定，否则原样返回
// （建模侧自定义表达式视为已限定）。
func qualify(idx *index, tableID, expr string) string {
	e := strings.TrimSpace(expr)
	if tableID == "" {
		return e
	}
	alias := idx.aliasByID[tableID]
	if alias == "" {
		return e
	}
	if strings.ContainsAny(e, " (),.`") {
		return e
	}
	return alias + "." + e
}

// applyAgg 对度量表达式套默认聚合；AggNone 表示表达式本身即最终形态。
func applyAgg(agg semantic.Aggregation, expr string) string {
	switch agg {
	case semantic.AggSum:
		return "SUM(" + expr + ")"
	case semantic.AggCount:
		return "COUNT(" + expr + ")"
	case semantic.AggAvg:
		return "AVG(" + expr + ")"
	case semantic.AggMax:
		return "MAX(" + expr + ")"
	case semantic.AggMin:
		return "MIN(" + expr + ")"
	default: // AggNone / 未知
		return expr
	}
}

// joinKeyword 把 JoinType 映射为 SQL 关键字，缺省 LEFT JOIN。
func joinKeyword(jt semantic.JoinType) string {
	switch jt {
	case semantic.JoinInner:
		return "INNER JOIN"
	case semantic.JoinRight:
		return "RIGHT JOIN"
	default:
		return "LEFT JOIN"
	}
}

// tableAlias 生成逻辑表 SQL 别名：优先用逻辑名的安全化，否则 t<index>。
func tableAlias(t *semantic.LogicalTable, i int) string {
	safe := sanitizeIdent(t.Name)
	if safe == "" {
		return fmt.Sprintf("t%d", i)
	}
	return safe
}

// sanitizeIdent 仅保留字母/数字/下划线，且不以数字开头。
func sanitizeIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if b.Len() == 0 {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// quoteIdent 按方言包裹标识符（表名/列别名）：PostgreSQL 用双引号，其余（MySQL/空）
// 用反引号；同引号字符按各自规则内部转义（双写）。
func quoteIdent(dialect Dialect, s string) string {
	if dialect == DialectPostgres {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// sqlLiteral 把过滤值转成安全的 SQL 字面量：转义单引号与反斜杠并单引号包裹。
func sqlLiteral(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "'", "''")
	return "'" + v + "'"
}

// key 归一化名称/同义词为查找键。
func key(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// isTimeType 判断维度数据类型是否为时间类（可套时间粒度上卷）。
func isTimeType(dataType string) bool {
	switch strings.ToLower(strings.TrimSpace(dataType)) {
	case "date", "datetime", "timestamp", "time":
		return true
	default:
		return false
	}
}

// validGrain 校验时间粒度取值。
func validGrain(g TimeGrain) bool {
	switch g {
	case GrainDay, GrainWeek, GrainMonth, GrainQuarter, GrainYear:
		return true
	default:
		return false
	}
}

// grainExpr 把时间维度表达式按粒度 + 方言包裹为「时间桶」表达式（同时用于 SELECT 与
// GROUP BY，保证分组键与展示列一致）。方言差异：MySQL 用 DATE_FORMAT/QUARTER，
// PostgreSQL 用 TO_CHAR。expr 为已限定表别名的时间列表达式。
func grainExpr(dialect Dialect, expr string, g TimeGrain) string {
	if dialect == DialectPostgres {
		switch g {
		case GrainDay:
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM-DD')", expr)
		case GrainWeek:
			return fmt.Sprintf("TO_CHAR(%s, 'IYYY-\"W\"IW')", expr)
		case GrainMonth:
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM')", expr)
		case GrainQuarter:
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY-\"Q\"Q')", expr)
		case GrainYear:
			return fmt.Sprintf("TO_CHAR(%s, 'YYYY')", expr)
		}
		return expr
	}
	// 默认 MySQL。日粒度也用 DATE_FORMAT 输出字符串，与周/月/季/年保持同为
	// VARCHAR 的列类型（避免跨粒度切换时桶列类型在 DATE 与字符串间摇摆）。
	switch g {
	case GrainDay:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m-%%d')", expr)
	case GrainWeek:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%x-W%%v')", expr)
	case GrainMonth:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y-%%m')", expr)
	case GrainQuarter:
		return fmt.Sprintf("CONCAT(YEAR(%s), '-Q', QUARTER(%s))", expr, expr)
	case GrainYear:
		return fmt.Sprintf("DATE_FORMAT(%s, '%%Y')", expr)
	}
	return expr
}
