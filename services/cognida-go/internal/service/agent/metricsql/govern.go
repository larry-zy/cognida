package metricsql

import (
	"fmt"
	"strings"

	"cognida/internal/model/semantic"
)

// Warning 是建模期语义治理校验（ValidateBundle）报出的一条风险项。非致命：仅提示建模者，
// 引擎运行时另有「未覆盖即回退」的兜底。Code 是稳定短码，便于 CI/脚本 grep 固定基线。
type Warning struct {
	Code    string // 稳定短码：FACT_NO_PK / FANOUT_RISK / GRAIN_IDENTITY
	Table   string // 相关逻辑表（或关系两端），供人工定位
	Message string // 人读说明 + 修复建议
}

// String 渲染为「[CODE] message」单行，便于日志/CI 输出。
func (w Warning) String() string {
	return fmt.Sprintf("[%s] %s", w.Code, w.Message)
}

// ValidateBundle 对语义模型 bundle 做「建模期」治理体检，返回零条或多条 Warning。
// 定位是建模侧 lint（在 seed/导入时跑），不改变运行时行为——引擎运行时仍以「任一名称/粒度
// 未覆盖即回退词法 NL2SQL」为准。刻意做成「零误报优先」：只报能从结构确证的风险，宁可漏报
// 也不对合法建模（如按城市/性别折叠维表）误报。当前覆盖三类：
//
//	D1 GRAIN_IDENTITY：维表挂了 name/title 展示裸列维度，却没有绑定主键的「身份维度」——
//	  按展示列分组会把同名不同主键的实体合并，令 Top 榜虚高（正是商品/品类重名塌陷的 bug 形状）。
//	  只针对「非事实表 + 已声明主键 + 命中 name/title 列」的窄场景，对城市/性别/状态等类别维零误报。
//	D2 FANOUT_RISK：关系两端主键均已声明，但 JOIN 等式未命中任一端主键——疑似多对多/非主键
//	  连接，从事实表出发聚合可能扇出虚增。星型 fact.fk=dim.pk（恰命中一端）不会触发。
//	D3 FACT_NO_PK：事实表（挂了度量）未声明主键——无法据主键校验关系基数/分组唯一性。
func ValidateBundle(b *semantic.ModelBundle) []Warning {
	if b == nil {
		return nil
	}
	idx := newIndex(b)
	var out []Warning

	// 按逻辑表归拢维度，供逐表体检。
	dimsByTable := map[string][]*semantic.Dimension{}
	for i, d := range b.Dimensions {
		dimsByTable[d.LogicalTableID] = append(dimsByTable[d.LogicalTableID], b.Dimensions[i])
	}

	for _, t := range b.LogicalTables {
		pk := strings.TrimSpace(t.PrimaryKey)
		isFact := idx.tableHasMeasure(t.ID)

		// D3：事实表必须声明主键，否则基数/唯一性无从校验。
		if isFact && pk == "" {
			out = append(out, Warning{Code: "FACT_NO_PK", Table: t.Name,
				Message: fmt.Sprintf("事实表 %q 未声明主键（PrimaryKey 空）：无法据主键校验关系基数与分组唯一性，建议补齐。", t.Name)})
		}

		// D1：非事实维表若有主键、挂了 name/title 展示裸列维度、却没有绑主键的身份维度，
		// 则按展示列分组会合并同名不同主键的实体。只对这个窄场景告警，避免误伤类别维。
		if !isFact && pk != "" {
			hasIdentity := false
			var displayNames []string
			for _, d := range dimsByTable[t.ID] {
				col := bareColumn(d.Expr)
				if col == "" {
					continue // 复杂表达式维度，非裸列，跳过
				}
				if strings.EqualFold(col, pk) {
					hasIdentity = true
				}
				if isDisplayLabelColumn(col) {
					displayNames = append(displayNames, fmt.Sprintf("%q", d.Name))
				}
			}
			if !hasIdentity && len(displayNames) > 0 {
				out = append(out, Warning{Code: "GRAIN_IDENTITY", Table: t.Name,
					Message: fmt.Sprintf("维表 %q 有主键 %q，但展示维度 %s 绑定非唯一名称列、且缺少绑定主键的身份维度：按其分组会合并同名不同主键的实体（Top 榜虚高）。建议增设 Expr=%q 的身份维度承接实体粒度。",
						t.Name, pk, strings.Join(displayNames, "、"), pk)})
			}
		}
	}

	// D2：逐关系判基数——两端主键均已知，且 JOIN 等式未命中任一端主键 → 无键连接/多对多，扇出风险。
	for _, rel := range b.Relations {
		lt := idx.tableByID[rel.LeftTableID]
		rt := idx.tableByID[rel.RightTableID]
		if lt == nil || rt == nil {
			continue
		}
		lCol, rCol, ok := joinKeyColumns(rel.JoinCondition, idx.aliasByID[rel.LeftTableID], idx.aliasByID[rel.RightTableID])
		if !ok {
			continue // 连接条件非简单 a.x=b.y，无法判定基数，不臆测（零误报优先）
		}
		lpk, rpk := strings.TrimSpace(lt.PrimaryKey), strings.TrimSpace(rt.PrimaryKey)
		if lpk == "" || rpk == "" {
			continue // 缺主键的基数问题已由 FACT_NO_PK / 建模自查覆盖，此处不重复告警
		}
		lKeyed := strings.EqualFold(lCol, lpk)
		rKeyed := strings.EqualFold(rCol, rpk)
		if !lKeyed && !rKeyed {
			out = append(out, Warning{Code: "FANOUT_RISK", Table: lt.Name + "⋈" + rt.Name,
				Message: fmt.Sprintf("关系 %q 的连接 %q 未命中任一端主键（%s.pk=%q / %s.pk=%q）：疑似多对多或非主键连接，从事实表出发聚合可能扇出虚增。请确认基数或改绑主键列。",
					rel.ID, rel.JoinCondition, lt.Name, lpk, rt.Name, rpk)})
		}
	}

	return out
}

// bareColumn 若 expr 是「裸列名」（可带 alias. 限定前缀）返回其列名部分，否则返回 ""
// （含空格/括号/运算符等的复杂表达式不视作裸列）。用于判定维度是否直接绑定物理列。
func bareColumn(expr string) string {
	e := strings.TrimSpace(expr)
	if e == "" || strings.ContainsAny(e, " (),`*+-/") {
		return ""
	}
	if i := strings.LastIndex(e, "."); i >= 0 {
		e = e[i+1:]
	}
	if e == "" || strings.Contains(e, ".") {
		return ""
	}
	return e
}

// isDisplayLabelColumn 判断列名是否是典型的「实体展示名称」列（非唯一、用于人读展示，
// 拿来当分组键会合并同名实体）。刻意只收最常见的名称列，避免把 city/status 等类别列误判。
func isDisplayLabelColumn(col string) bool {
	switch strings.ToLower(col) {
	case "name", "title", "label", "display_name", "full_name", "fullname":
		return true
	}
	return false
}

// joinKeyColumns 解析简单等值连接条件 "leftAlias.x = rightAlias.y"，返回归属左表的列与
// 归属右表的列（不区分书写顺序）。非「单个 a.col = b.col」形式返回 ok=false（不臆测基数）。
func joinKeyColumns(cond, leftAlias, rightAlias string) (lCol, rCol string, ok bool) {
	parts := strings.Split(cond, "=")
	if len(parts) != 2 {
		return "", "", false
	}
	aAlias, aCol := splitQualified(parts[0])
	bAlias, bCol := splitQualified(parts[1])
	if aAlias == "" || bAlias == "" {
		return "", "", false
	}
	switch {
	case strings.EqualFold(aAlias, leftAlias) && strings.EqualFold(bAlias, rightAlias):
		return aCol, bCol, true
	case strings.EqualFold(aAlias, rightAlias) && strings.EqualFold(bAlias, leftAlias):
		return bCol, aCol, true
	}
	return "", "", false
}

// splitQualified 把 "alias.col" 拆成别名与列名；无点号则别名为空、整体作列名。
func splitQualified(s string) (alias, col string) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "."); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return "", s
}
