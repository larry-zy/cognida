// Package evaluation 提供评测系统应用层实现
package evaluation

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	agentctx "link/internal/model/agent"
	model_datasource "link/internal/model/datasource"
	"link/internal/service/evaluation/executor"
)

// sqlRunnerMaxRows Text2SQL 评测执行准确率比对的取行上限。
// 金标准查询通常聚合/明细有限，1000 行足以判定结果集是否等价，同时限制内存与耗时。
const sqlRunnerMaxRows = 1000

// sqlRunnerQueryTimeout 单条只读查询超时（与 sql_execute 工具一致）。
const sqlRunnerQueryTimeout = 30 * time.Second

// sqlRunner 实现 executor.SQLRunner：按 ctx 的 datasource_id 路由到业务库或已注册外部数据源，
// 只读执行 SQL 并返回完整结果集。校验/强制 LIMIT/超时对两条路径同等生效，与被测 Agent 的
// sql_execute 工具走同一条数据链路，保证金标准与生成 SQL 在同一库上比对。
//
// 本文件的只读校验/LIMIT 补全逻辑与 agent/tools/sql_execute.go 同源但自持一份，避免评测应用层
// 反向依赖 agent 工具层（防止 import 环）。
type sqlRunner struct {
	businessDB *gorm.DB
	dsp        model_datasource.ConnectionProvider
}

// NewSQLRunner 创建 Text2SQL 评测用的只读 SQL 执行器。dsp 可为 nil（仅支持业务库路径）。
func NewSQLRunner(businessDB *gorm.DB, dsp model_datasource.ConnectionProvider) executor.SQLRunner {
	return &sqlRunner{businessDB: businessDB, dsp: dsp}
}

// RunReadOnly 只读执行单条查询并返回完整结果集。
func (r *sqlRunner) RunReadOnly(ctx context.Context, sqlStr string) (*executor.SQLResultSet, error) {
	if strings.TrimSpace(sqlStr) == "" {
		return nil, fmt.Errorf("sql 不能为空")
	}
	if err := validateReadOnlySQL(sqlStr); err != nil {
		return nil, fmt.Errorf("SQL 验证失败: %w", err)
	}

	db, err := r.resolveTarget(ctx)
	if err != nil {
		return nil, err
	}

	execSQL := ensureReadOnlyLimit(sqlStr, sqlRunnerMaxRows)

	queryCtx, cancel := context.WithTimeout(ctx, sqlRunnerQueryTimeout)
	defer cancel()

	rows, err := db.QueryContext(queryCtx, execSQL)
	if err != nil {
		return nil, fmt.Errorf("执行查询失败: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取列名失败: %w", err)
	}

	result := &executor.SQLResultSet{Columns: columns, Rows: make([][]interface{}, 0)}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("扫描行数据失败: %w", err)
		}
		row := make([]interface{}, len(columns))
		for i := range columns {
			if b, ok := values[i].([]byte); ok {
				row[i] = string(b) // []byte -> string，使 JSON 序列化与比对稳定
			} else {
				row[i] = values[i]
			}
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取结果集失败: %w", err)
	}
	return result, nil
}

// resolveTarget 解析只读查询目标：空 datasource_id → 业务库；非空 → 已注册外部数据源。
// 与 agent/tools 的 resolveQueryTarget 同语义，但只需底层 *sql.DB，不解析库名。
func (r *sqlRunner) resolveTarget(ctx context.Context) (*sql.DB, error) {
	datasourceID := agentctx.MustGetDatasourceID(ctx)
	if datasourceID == "" {
		if r.businessDB == nil {
			return nil, fmt.Errorf("数据库未初始化")
		}
		db, err := r.businessDB.DB()
		if err != nil {
			return nil, fmt.Errorf("获取数据库连接失败: %w", err)
		}
		return db, nil
	}
	if r.dsp == nil {
		return nil, fmt.Errorf("外部数据源功能未启用，无法使用 datasource_id=%q", datasourceID)
	}
	db, _, err := r.dsp.Acquire(ctx, agentctx.MustGetTenantID(ctx), datasourceID)
	if err != nil {
		return nil, fmt.Errorf("数据源 %q 不可用: %w", datasourceID, err)
	}
	return db, nil
}

// readOnlyBlacklist 写/DDL/权限类关键词黑名单——只匹配真正会改数据/结构/权限或执行存储过程的语句。
// SHOW/DESCRIBE/EXPLAIN 是只读的，已移入白名单首关键字（readOnlyLeadingKeywords），不再列入黑名单。
var readOnlyBlacklist = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER",
	"CREATE", "TRUNCATE", "GRANT", "REVOKE", "EXECUTE",
	"CALL",
	// SELECT ... INTO OUTFILE/DUMPFILE 会以 SELECT 开头骗过白名单，却把结果写到 DB 服务器
	// 磁盘（需 FILE 权限），是伪装成只读的写操作，必须拦截。
	"OUTFILE", "DUMPFILE",
}

// readOnlyLeadingKeywords 白名单首关键字：只读语句必须以其中之一开头。
// 相比纯黑名单，白名单能拦住黑名单未穷举的写法，同时放行合法的 SHOW/DESC/EXPLAIN。
var readOnlyLeadingKeywords = []string{"SELECT", "WITH", "SHOW", "DESCRIBE", "DESC", "EXPLAIN"}

// stripSQLStringLiterals 剥离 SQL 中的字符串字面量（'...' 与 "..." 内容，含 '' / "" 与反斜杠转义），
// 只保留字面量外的结构。避免把字面量里的关键字/分号/注释（如 WHERE status='delete'、
// remark='a;b'、note='x--y'）误判为写操作/多语句/注释注入而误杀合法金标准 SQL。
func stripSQLStringLiterals(sqlStr string) string {
	var b strings.Builder
	runes := []rune(sqlStr)
	n := len(runes)
	for i := 0; i < n; {
		c := runes[i]
		if c == '\'' || c == '"' {
			quote := c
			i++ // 跳过起始引号
			for i < n {
				ch := runes[i]
				if ch == '\\' && i+1 < n {
					i += 2 // 跳过反斜杠转义字符
					continue
				}
				if ch == quote {
					if i+1 < n && runes[i+1] == quote {
						i += 2 // 重复引号转义（'' / ""）
						continue
					}
					i++ // 结束引号
					break
				}
				i++
			}
			continue // 字面量内容整体丢弃
		}
		b.WriteRune(c)
		i++
	}
	return b.String()
}

// validateReadOnlySQL 只读安全校验：先剥离字符串字面量，再做「白名单首关键字 + 写/DDL 黑名单 +
// 注释注入 + 多语句」四道检查。剥离字面量使字面量里的关键字/分号/注释不再误伤合法金标准 SQL。
func validateReadOnlySQL(sqlStr string) error {
	sqlStr = strings.TrimSpace(sqlStr)
	if sqlStr == "" {
		return fmt.Errorf("sql 不能为空")
	}

	// 剥离字面量后的文本，用于所有基于关键字/分隔符的检查。
	stripped := strings.TrimSpace(stripSQLStringLiterals(sqlStr))
	upperStripped := strings.ToUpper(stripped)

	// 白名单首关键字：剥离前导空白/左括号后必须以只读语句开头。
	// 去掉前导 '(' 以放行括号包裹的子查询/并集，如 (SELECT ...) UNION (SELECT ...)。
	head := strings.TrimLeft(upperStripped, "( \t\n\r")
	allowed := false
	for _, kw := range readOnlyLeadingKeywords {
		if head == kw ||
			strings.HasPrefix(head, kw+" ") ||
			strings.HasPrefix(head, kw+"\t") ||
			strings.HasPrefix(head, kw+"\n") ||
			strings.HasPrefix(head, kw+"\r") ||
			strings.HasPrefix(head, kw+"(") {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("必须以只读语句开头（SELECT/WITH/SHOW/DESC/DESCRIBE/EXPLAIN）")
	}

	// 写/DDL/权限黑名单（词边界）——在剥离字面量后的文本上匹配，拦住形如 EXPLAIN DELETE ... 的写操作。
	for _, keyword := range readOnlyBlacklist {
		pattern := fmt.Sprintf(`\b%s\b`, keyword)
		if matched, _ := regexp.MatchString(pattern, upperStripped); matched {
			return fmt.Errorf("包含禁止的关键词: %s", keyword)
		}
	}

	// 注释注入（剥离字面量后再查，避免字面量内的 -- / /* 误杀）。
	if strings.Contains(stripped, "--") {
		return fmt.Errorf("不能包含 -- 注释")
	}
	if strings.Contains(stripped, "/*") {
		return fmt.Errorf("不能包含 /* */ 注释")
	}

	// 多语句：去掉结尾分号后中间仍含分号才算多语句（尾分号合法）。
	if strings.Contains(strings.TrimRight(stripped, "; \t\n\r"), ";") {
		return fmt.Errorf("不能包含多语句")
	}
	return nil
}

var readOnlyLimitRe = regexp.MustCompile(`\bLIMIT\s+(\d+)`)

// limitableLeadingRe 判断语句是否为可安全追加 LIMIT 的行式查询（SELECT/WITH）。
// SHOW/DESCRIBE/DESC/EXPLAIN 同为只读但语法上不接 LIMIT，追加会产出非法 SQL，故不补。
var limitableLeadingRe = regexp.MustCompile(`(?i)^[(\s]*(SELECT|WITH)\b`)

// ensureReadOnlyLimit 确保行式查询带 LIMIT，且不超过上限（超上限或解析失败一律压到 maxRows）。
// 仅对 SELECT/WITH 生效：SHOW/DESCRIBE/EXPLAIN 不接 LIMIT，原样返回避免拼出非法语句。
func ensureReadOnlyLimit(sqlStr string, maxRows int) string {
	if !limitableLeadingRe.MatchString(sqlStr) {
		return sqlStr
	}
	if !regexp.MustCompile(`\bLIMIT\s+\d+`).MatchString(sqlStr) {
		sqlStr = strings.TrimRight(sqlStr, "; ")
		return fmt.Sprintf("%s LIMIT %d", sqlStr, maxRows)
	}
	matches := readOnlyLimitRe.FindStringSubmatch(sqlStr)
	if len(matches) > 1 {
		limitVal, err := strconv.Atoi(matches[1])
		if err != nil || limitVal > maxRows {
			sqlStr = readOnlyLimitRe.ReplaceAllString(sqlStr, fmt.Sprintf("LIMIT %d", maxRows))
		}
	}
	return sqlStr
}
