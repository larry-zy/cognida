// Package tools 提供 SQL 执行工具
package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ========================================
// SQL Execute Tool
// ========================================

// SQLExecuteRequest SQL 执行请求
type SQLExecuteRequest struct {
	// SQL 要执行的 SQL 语句
	SQL string `json:"sql" jsonschema:"required,description=要执行的SELECT查询语句"`

	// DatabaseID 数据库ID（可选，默认使用租户默认库）
	DatabaseID string `json:"database_id" jsonschema:"description=数据库ID，可选"`

	// MaxRows 最大返回行数（默认100，最大1000）
	MaxRows int `json:"max_rows" jsonschema:"description=最大返回行数，默认100，最大1000"`
}

// SQLExecuteResult SQL 执行结果
type SQLExecuteResult struct {
	// Columns 列名
	Columns []string `json:"columns"`

	// Rows 数据行
	Rows []map[string]interface{} `json:"rows"`

	// Count 返回行数
	Count int `json:"count"`

	// LatencyMs 查询耗时（毫秒）
	LatencyMs int64 `json:"latency_ms"`

	// Warning 警告信息
	Warning string `json:"warning,omitempty"`

	// SQL 实际执行的SQL（添加LIMIT后）
	ExecutedSQL string `json:"executed_sql,omitempty"`
}

// 全局 DB（通过 init 设置）
var sqlDB *gorm.DB

// InitSQLExecuteTool 初始化 SQL 执行工具
func InitSQLExecuteTool(db *gorm.DB) {
	sqlDB = db
}

// NewSQLExecuteTool 创建 SQL 执行工具
func NewSQLExecuteTool() *TypedBaseTool[SQLExecuteRequest, SQLExecuteResult] {
	return NewTypedBaseTool("sql_execute",
		`执行只读 SQL 查询。

安全限制：
- 仅支持 SELECT 查询
- 自动添加 LIMIT 子句
- 检测并阻止 SQL 注入
- 超时时间 30 秒

参数：
- sql: SQL 语句（必需）
- database_id: 数据库ID（可选）
- max_rows: 最大行数（可选，默认100）`,
		sqlExecute,
	)
}

// sqlExecute 执行 SQL 查询
func sqlExecute(ctx context.Context, req *SQLExecuteRequest) (*SQLExecuteResult, error) {
	startTime := time.Now()

	// 参数验证
	if req.SQL == "" {
		return nil, fmt.Errorf("sql 不能为空")
	}

	if sqlDB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	// 设置默认值
	if req.MaxRows <= 0 {
		req.MaxRows = 100
	}
	if req.MaxRows > 1000 {
		req.MaxRows = 1000
	}

	// 安全验证
	if err := validateSQL(req.SQL); err != nil {
		return nil, fmt.Errorf("SQL 验证失败: %w", err)
	}

	// 添加 LIMIT
	execSQL := ensureLimit(req.SQL, req.MaxRows)

	// 设置超时
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 执行查询
	rows, err := sqlDB.WithContext(queryCtx).Raw(execSQL).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询执行失败: %w", err)
	}
	defer rows.Close()

	// 获取列名
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("获取列名失败: %w", err)
	}

	// 读取数据
	var rowData []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("扫描行数据失败: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		rowData = append(rowData, row)
	}

	// 检查结果数量
	warning := ""
	if len(rowData) >= req.MaxRows {
		warning = fmt.Sprintf("结果已限制在 %d 行", req.MaxRows)
	}

	return &SQLExecuteResult{
		Columns:    columns,
		Rows:       rowData,
		Count:      len(rowData),
		LatencyMs:  time.Since(startTime).Milliseconds(),
		Warning:    warning,
		ExecutedSQL: execSQL,
	}, nil
}

// ========================================
// 安全验证
// ========================================

// validateSQL 验证 SQL 安全性
func validateSQL(sqlStr string) error {
	sqlStr = strings.TrimSpace(sqlStr)
	upperSQL := strings.ToUpper(sqlStr)

	// 检查黑名单关键词
	blacklist := []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "ALTER",
		"CREATE", "TRUNCATE", "GRANT", "REVOKE", "EXECUTE",
		"CALL", "EXPLAIN", "SHOW", "DESCRIBE",
	}

	for _, keyword := range blacklist {
		pattern := fmt.Sprintf(`\b%s\b`, keyword)
		if matched, _ := regexp.MatchString(pattern, upperSQL); matched {
			return fmt.Errorf("包含禁止的关键词: %s", keyword)
		}
	}

	// 检查是否以 SELECT 开始
	if !strings.HasPrefix(upperSQL, "SELECT") && !strings.HasPrefix(upperSQL, "WITH") {
		return fmt.Errorf("必须以 SELECT 或 WITH 开头")
	}

	// 检查注释注入
	if strings.Contains(upperSQL, "--") {
		return fmt.Errorf("不能包含 -- 注释")
	}
	if strings.Contains(upperSQL, "/*") {
		return fmt.Errorf("不能包含 /* */ 注释")
	}

	// 检查多语句
	if strings.Contains(sqlStr, ";") {
		// 允许末尾分号
		trimmed := strings.TrimRight(sqlStr, "; ")
		if trimmed != sqlStr {
			return fmt.Errorf("不能包含多语句")
		}
	}

	return nil
}

// ensureLimit 确保 SQL 有 LIMIT 子句
func ensureLimit(sqlStr string, maxRows int) string {
	hasLimit := regexp.MustCompile(`\bLIMIT\s+\d+`).MatchString(sqlStr)

	if !hasLimit {
		sqlStr = strings.TrimRight(sqlStr, "; ")
		return fmt.Sprintf("%s LIMIT %d", sqlStr, maxRows)
	}

	// 检查 LIMIT 是否超过最大值
	re := regexp.MustCompile(`\bLIMIT\s+(\d+)`)
	matches := re.FindStringSubmatch(sqlStr)
	if len(matches) > 1 {
		limitStr := matches[1]
		if len(limitStr) > 4 { // 超过 9999
			sqlStr = re.ReplaceAllString(sqlStr, fmt.Sprintf("LIMIT %d", maxRows))
		}
	}

	return sqlStr
}
