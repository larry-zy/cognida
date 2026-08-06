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

// readOnlyBlacklist 只读校验黑名单关键词（与 sql_execute 工具一致）。
var readOnlyBlacklist = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "ALTER",
	"CREATE", "TRUNCATE", "GRANT", "REVOKE", "EXECUTE",
	"CALL", "EXPLAIN", "SHOW", "DESCRIBE",
}

// validateReadOnlySQL 只读安全校验：拒绝写/DDL/多语句/注释注入，要求以 SELECT/WITH 开头。
func validateReadOnlySQL(sqlStr string) error {
	sqlStr = strings.TrimSpace(sqlStr)
	upperSQL := strings.ToUpper(sqlStr)

	for _, keyword := range readOnlyBlacklist {
		pattern := fmt.Sprintf(`\b%s\b`, keyword)
		if matched, _ := regexp.MatchString(pattern, upperSQL); matched {
			return fmt.Errorf("包含禁止的关键词: %s", keyword)
		}
	}
	if !strings.HasPrefix(upperSQL, "SELECT") && !strings.HasPrefix(upperSQL, "WITH") {
		return fmt.Errorf("必须以 SELECT 或 WITH 开头")
	}
	if strings.Contains(upperSQL, "--") {
		return fmt.Errorf("不能包含 -- 注释")
	}
	if strings.Contains(upperSQL, "/*") {
		return fmt.Errorf("不能包含 /* */ 注释")
	}
	if strings.Contains(strings.TrimRight(sqlStr, "; \t\n"), ";") {
		return fmt.Errorf("不能包含多语句")
	}
	return nil
}

var readOnlyLimitRe = regexp.MustCompile(`\bLIMIT\s+(\d+)`)

// ensureReadOnlyLimit 确保查询带 LIMIT，且不超过上限（超上限或解析失败一律压到 maxRows）。
func ensureReadOnlyLimit(sqlStr string, maxRows int) string {
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
