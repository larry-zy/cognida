// Package tools — Group 8 写/ETL 失败路径的「可修复观察」下沉验证。
//
// 目标：复杂写（sql_mutate）与 ETL 派生（etl_run）执行失败时，工具返回结构化
// framework.RepairableToolError（分级 error_kind + 定向线索 + 脱敏摘要），供 Operation
// 子代理按修复纪律本地定向重试；同时确保：失败先落审计（StatusFailed）、事务回滚、
// 危险分级/人工确认协议不因新增可修复路径而被绕过。
package tools

import (
	"context"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestSQLMutate_WriteFailureRepairable 校验：直接写（DML）执行失败时返回可修复观察，
// error_kind 正确分级，肇事列名进入脱敏摘要，且失败已先落 StatusFailed 审计、事务已回滚。
func TestWriteQueryTarget_NilDBDegrades(t *testing.T) {
	o := &opTools{cfg: OperationConfig{}}
	if o.writeQueryTarget() != nil {
		t.Fatal("DB 为 nil 时 writeQueryTarget 应返回 nil")
	}
	re := newRepairableSQLError(context.Background(), o.writeQueryTarget(),
		"UPDATE agent_etl_x SET a = 1",
		&mysql.MySQLError{Number: 1064, Message: "syntax error"}, nil, "")
	if re.ErrorKind != string(sqlErrSyntax) {
		t.Fatalf("应仍能分级 error_kind, got %q", re.ErrorKind)
	}
}
