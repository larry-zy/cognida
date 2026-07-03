package metricsql

import (
	"fmt"
	"strings"

	"link/internal/model/semantic"
)

// index 是语义模型 bundle 的解析索引：把名称/同义词映射到构件，并缓存逻辑表别名。
type index struct {
	bundle *semantic.ModelBundle

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

// metricTableID 推断指标的归属逻辑表：扫描 Expr 是否引用某度量名，取该度量的表；
// 否则若模型只有一张逻辑表则绑定该表；再否则返回空（由 chooseBase 兜底/回退）。
func (idx *index) metricTableID(m *semantic.Metric) string {
	lowerExpr := strings.ToLower(m.Expr)
	for _, ms := range idx.bundle.Measures {
		if strings.Contains(lowerExpr, strings.ToLower(ms.Name)) {
			return ms.LogicalTableID
		}
	}
	if len(idx.bundle.LogicalTables) == 1 {
		return idx.bundle.LogicalTables[0].ID
	}
	return ""
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
	// needed 里任取其一（确定性：按别名字典序）。
	var best string
	for id := range needed {
		if best == "" || idx.aliasByID[id] < idx.aliasByID[best] {
			best = id
		}
	}
	return best
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
	sb.WriteString(fmt.Sprintf("%s %s", quoteIdent(baseTable.PhysicalTable), idx.aliasByID[base]))

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
			sb.WriteString(fmt.Sprintf(" %s %s %s ON %s", joinKw, quoteIdent(nt.PhysicalTable), idx.aliasByID[newID], rel.JoinCondition))
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

// quoteIdent 用反引号包裹标识符（表名/列别名），内部反引号转义。
func quoteIdent(s string) string {
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
