// Package tools —— SQL 执行失败的「分级 + 可修复观察」支撑。
//
// 自我修复的第一层在工具内落地：把底层驱动错误分级为可判定的 error_kind，
// 并组装成一段紧凑 JSON「可修复观察」（含定向线索与脱敏摘要），经
// framework.RepairableToolError 原样回灌 LLM——引导定向改写而非盲目重试。
// error_kind 同时供框架层按 toolName+error_kind 计数、触发再规划/提前收尾。
package tools

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"link/internal/service/agent/framework"
)

// sqlErrorKind 是 SQL 失败的可判定分级。字符串取值即回灌观察里的 error_kind 字段，
// 与框架层失败签名计数（toolName:error_kind）严格一致，切勿随意改动取值。
type sqlErrorKind string

const (
	sqlErrSyntax        sqlErrorKind = "syntax"         // 语法错误（1064）
	sqlErrUnknownColumn sqlErrorKind = "unknown_column" // 列不存在（1054）
	sqlErrUnknownTable  sqlErrorKind = "unknown_table"  // 表不存在（1146）
	sqlErrTimeout       sqlErrorKind = "timeout"        // 查询超时（30s 截止）
	sqlErrPermission    sqlErrorKind = "permission"     // 权限不足（1044/1142）
	sqlErrTransient     sqlErrorKind = "transient"      // 瞬时故障（死锁/锁等待/连接抖动，可自动重试）
	sqlErrOther         sqlErrorKind = "other"          // 其余未归类
)

// 线索规模上限：只在失败路径检索，仍需截断防止把全库结构灌回 LLM。
const (
	maxHintColumns = 40 // 每表回传的列名上限
	maxHintTables  = 20 // unknown_table 回传的候选表名上限
	hintQueryTTL   = 5 * time.Second
)

// retriable 表示「定向改写后重试有望成功」——供 LLM 判断是否值得再试。
// permission/other 无从改写，标记 false；其余（改列名/改表名/修语法/缩范围/等瞬时恢复）为 true。
func (k sqlErrorKind) retriable() bool {
	switch k {
	case sqlErrUnknownColumn, sqlErrUnknownTable, sqlErrSyntax, sqlErrTimeout, sqlErrTransient:
		return true
	default:
		return false
	}
}

var reFirstQuoted = regexp.MustCompile(`'([^']*)'`)

// reTableRef 从 SQL 里粗取 FROM/JOIN 后的表名（含可选反引号），仅用于取列名线索，非严格解析。
var reTableRef = regexp.MustCompile("(?is)\\b(?:FROM|JOIN)\\s+`?([a-zA-Z_][\\w]*)`?")

// classifySQLError 将底层错误分级，并在可能时抽取「肇事标识符」（列名/表名）。
func classifySQLError(err error) (sqlErrorKind, string) {
	if err == nil {
		return sqlErrOther, ""
	}
	// 查询上下文 30s 截止 → 超时。
	if errors.Is(err, context.DeadlineExceeded) {
		return sqlErrTimeout, ""
	}
	// 驱动坏连接 → 瞬时。
	if errors.Is(err, driver.ErrBadConn) {
		return sqlErrTransient, ""
	}
	// MySQL 错误码优先（最可靠）。
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case 1054:
			return sqlErrUnknownColumn, extractQuoted(myErr.Message)
		case 1146:
			return sqlErrUnknownTable, extractTableName(myErr.Message)
		case 1064:
			return sqlErrSyntax, ""
		case 1044, 1142:
			return sqlErrPermission, ""
		case 1205, 1213: // 锁等待超时 / 死锁
			return sqlErrTransient, ""
		}
	}
	// 兜底文本匹配（非 MySQLError 的网络/驱动错误）。
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "bad connection"),
		strings.Contains(msg, "invalid connection"),
		strings.Contains(msg, "i/o timeout"):
		return sqlErrTransient, ""
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "timeout"):
		return sqlErrTimeout, ""
	}
	return sqlErrOther, ""
}

// extractQuoted 取消息中首个单引号包裹的标识符（如 Unknown column 'foo' → foo）。
func extractQuoted(msg string) string {
	if m := reFirstQuoted.FindStringSubmatch(msg); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractTableName 取 Table 'db.orders' 中的表名部分（去掉库前缀）。
func extractTableName(msg string) string {
	q := extractQuoted(msg)
	if i := strings.LastIndex(q, "."); i >= 0 {
		return q[i+1:]
	}
	return q
}

// repairObservation 是回灌 LLM 的可修复观察 JSON。成功路径信封不受影响，仅失败路径产出此结构。
type repairObservation struct {
	ErrorKind string      `json:"error_kind"`
	Retriable bool        `json:"retriable"`
	Hint      interface{} `json:"hint,omitempty"`
	Detail    string      `json:"detail,omitempty"`
}

// newRepairableSQLError 是 SQL 工具失败路径的统一入口：分级 → 取线索 → 组装可修复观察 →
// 包成 framework.RepairableToolError（其 Error() 即观察 JSON，由框架原样回灌）。
// target 为 nil 时降级为通用提示（不检索 schema）。
func newRepairableSQLError(ctx context.Context, target *queryTarget, sql string, err error) *framework.RepairableToolError {
	kind, identifier := classifySQLError(err)

	var hint interface{}
	if target != nil {
		hint = buildSchemaHint(ctx, target, kind, identifier, sql)
	} else {
		hint = genericTip(kind)
	}

	external := target != nil && target.external
	obs := repairObservation{
		ErrorKind: string(kind),
		Retriable: kind.retriable(),
		Hint:      hint,
		Detail:    repairDetail(kind, external, err),
	}
	payload, mErr := json.Marshal(obs)
	if mErr != nil {
		// 退化：至少保证 error_kind/retriable 可被框架解析。
		payload = []byte(fmt.Sprintf(`{"error_kind":%q,"retriable":%t}`, kind, kind.retriable()))
	}
	return &framework.RepairableToolError{Observation: string(payload), ErrorKind: string(kind)}
}

// buildSchemaHint 在失败路径检索定向线索：列不存在→列清单、表不存在→候选表名，其余→通用提示。
// 全程 recover 兜底——线索检索失败绝不 panic、绝不阻断修复观察产出（降级为无 hint 或通用提示）。
func buildSchemaHint(ctx context.Context, target *queryTarget, kind sqlErrorKind, identifier, sql string) (hint interface{}) {
	defer func() {
		if r := recover(); r != nil {
			hint = genericTip(kind)
		}
	}()

	// 仅列/表不存在两类需要检索 information_schema 定向线索；其余分级（语法/超时/权限/瞬时/其它）
	// 无从据 schema 改写，直接给通用提示，绝不为其发起任何 DB 往返（含 dbName 解析）——
	// 尤其 timeout 时库已受压，不应再加一次 SELECT DATABASE()。
	if kind != sqlErrUnknownColumn && kind != sqlErrUnknownTable {
		return genericTip(kind)
	}

	// 用一段脱离父取消的短超时上下文：父 ctx 可能因查询超时已被取消，schema 检索需独立预算。
	// dbName 惰性解析与后续列/表检索同处该预算下，避免 DB 挂起时库名解析无界阻塞。
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), hintQueryTTL)
	defer cancel()

	// 业务库 dbName 惰性解析：columnHint/tableHint 查 information_schema 需库名过滤，
	// 未解析则 available_columns/available_tables 恒空。外部源注册时已知 dbName，早返回不发查询。
	// 解析失败仅降级为通用提示，不阻断可修复观察。
	if _, err := target.databaseNameContext(hctx); err != nil {
		return genericTip(kind)
	}

	switch kind {
	case sqlErrUnknownColumn:
		return columnHint(hctx, target, identifier, sql)
	default: // sqlErrUnknownTable
		return tableHint(hctx, target, identifier)
	}
}

// columnHint 取 SQL 引用表的可用列清单，引导 LLM 只用真实列名改写。
func columnHint(ctx context.Context, target *queryTarget, unknownColumn, sql string) interface{} {
	h := map[string]interface{}{
		"unknown_column": unknownColumn,
		"tip":            "列名不存在，请仅使用 available_columns 中列出的真实列名重写查询",
	}
	available := map[string][]string{}
	for _, tbl := range referencedTables(sql) {
		cols, err := queryTableColumns(ctx, target, tbl)
		if err != nil || len(cols) == 0 {
			continue
		}
		names := make([]string, 0, len(cols))
		for _, c := range cols {
			names = append(names, c.Name)
		}
		if len(names) > maxHintColumns {
			names = names[:maxHintColumns]
		}
		available[tbl] = names
	}
	if len(available) > 0 {
		h["available_columns"] = available
	}
	return h
}

// tableHint 取库内候选表名（按与肇事表名的近似度排序），引导 LLM 选对表。
func tableHint(ctx context.Context, target *queryTarget, unknownTable string) interface{} {
	h := map[string]interface{}{
		"unknown_table": unknownTable,
		"tip":           "表不存在，请从 available_tables 中选择正确表名，或先用 get_schema 确认",
	}
	names, err := listTableNames(ctx, target)
	if err != nil || len(names) == 0 {
		return h
	}
	h["available_tables"] = rankNamesByProximity(names, unknownTable, maxHintTables)
	return h
}

// genericTip 为无结构线索的分级给出一句定向提示。
func genericTip(kind sqlErrorKind) interface{} {
	tips := map[sqlErrorKind]string{
		sqlErrSyntax:        "SQL 语法有误，请检查关键词/括号/引号配对后重写",
		sqlErrUnknownColumn: "列名不存在，请用 get_schema 确认列名后重写查询",
		sqlErrUnknownTable:  "表不存在，请用 get_schema 确认表名后重写查询",
		sqlErrTimeout:       "查询超时，请缩小时间范围、增加过滤条件或减少 JOIN 后重试",
		sqlErrPermission:    "当前数据源无此操作权限，请改用只读查询或联系管理员",
		sqlErrTransient:     "瞬时故障（死锁/连接抖动），自动重试已耗尽，可稍后再试",
		sqlErrOther:         "查询执行失败，请核对 SQL 与数据源后再试",
	}
	if t, ok := tips[kind]; ok {
		return map[string]interface{}{"tip": t}
	}
	return nil
}

// referencedTables 从 SQL 粗取 FROM/JOIN 表名（去重、保序），仅用于取列名线索。
func referencedTables(sql string) []string {
	matches := reTableRef.FindAllStringSubmatch(sql, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		key := strings.ToLower(m[1])
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, m[1])
	}
	return out
}

// rankNamesByProximity 把候选表名按与目标的近似度排序（相等>包含>共享前缀>其它），截断 Top-K。
func rankNamesByProximity(names []string, target string, topK int) []string {
	if strings.TrimSpace(target) == "" {
		if len(names) > topK {
			return names[:topK]
		}
		return names
	}
	t := strings.ToLower(target)
	type scored struct {
		name  string
		score int
	}
	ranked := make([]scored, 0, len(names))
	for _, n := range names {
		ln := strings.ToLower(n)
		score := 0
		switch {
		case ln == t:
			score = 3
		case strings.Contains(ln, t) || strings.Contains(t, ln):
			score = 2
		case sharePrefix(ln, t, 3):
			score = 1
		}
		ranked = append(ranked, scored{name: n, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].name < ranked[j].name
	})
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	out := make([]string, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.name)
	}
	return out
}

// sharePrefix 判断两串是否共享至少 n 个字符的前缀。
func sharePrefix(a, b string, n int) bool {
	limit := n
	if len(a) < limit || len(b) < limit {
		return false
	}
	return a[:limit] == b[:limit]
}

// repairDetail 生成脱敏后的错误摘要：外部数据源仅给通用语（不泄底层主机/账号），内部库截断透传。
func repairDetail(kind sqlErrorKind, external bool, err error) string {
	if external {
		return externalGenericDetail(kind)
	}
	if err == nil {
		return ""
	}
	return sanitizeDetail(err.Error())
}

// externalGenericDetail 外部数据源错误的通用摘要——绝不透传底层报文（可能含主机/账号）。
func externalGenericDetail(kind sqlErrorKind) string {
	switch kind {
	case sqlErrPermission:
		return "外部数据源拒绝该操作（权限不足）"
	case sqlErrTimeout:
		return "外部数据源查询超时"
	case sqlErrTransient:
		return "外部数据源瞬时故障"
	case sqlErrUnknownColumn:
		return "外部数据源列名不存在"
	case sqlErrUnknownTable:
		return "外部数据源表不存在"
	case sqlErrSyntax:
		return "外部数据源 SQL 语法有误"
	default:
		return "外部数据源查询执行失败"
	}
}

// sanitizeDetail 折叠空白并截断，避免把冗长驱动报文整体灌回。
func sanitizeDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 300
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
