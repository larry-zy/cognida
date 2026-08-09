// Package executor 提供评测执行器实现
package executor

import "context"

// SQLResultSet 只读 SQL 查询返回的完整结果集（列名 + 行数据）。
// 供 Text2SQL 评测的执行准确率（execution accuracy）比对——Python 侧做无序集合比较。
type SQLResultSet struct {
	// Columns 列名（按查询列顺序）
	Columns []string `json:"columns"`
	// Rows 行数据（每行按列顺序的取值，[]byte 已转字符串）
	Rows [][]interface{} `json:"rows"`
}

// SQLRunner 只读执行 SQL 并返回完整结果集。
// 实现（应用层 sqlRunner）自持业务库 + 外部数据源提供者，按 ctx 的 datasource_id 路由，
// 与被测 Agent 的 sql_execute 走同一条数据链路，保证金标准与生成 SQL 在同一库上比对。
type SQLRunner interface {
	// RunReadOnly 只读执行单条 SELECT/WITH 查询，返回完整结果集。
	// 非只读语句、多语句、注释注入一律拒绝；查询自动补 LIMIT 且有超时。
	RunReadOnly(ctx context.Context, sql string) (*SQLResultSet, error)
}
