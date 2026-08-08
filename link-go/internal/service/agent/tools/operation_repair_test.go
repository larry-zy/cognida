// Package tools — Group 8 写/ETL 失败路径的「可修复观察」下沉验证。
//
// 目标：复杂写（sql_mutate）与 ETL 派生（etl_run）执行失败时，工具返回结构化
// framework.RepairableToolError（分级 error_kind + 定向线索 + 脱敏摘要），供 Operation
// 子代理按修复纪律本地定向重试；同时确保：失败先落审计（StatusFailed）、事务回滚、
// 危险分级/人工确认协议不因新增可修复路径而被绕过。
package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"

	"link/internal/model/agent/operations"
	"link/internal/service/agent/framework"
)

// TestSQLMutate_WriteFailureRepairable 校验：直接写（DML）执行失败时返回可修复观察，
// error_kind 正确分级，肇事列名进入脱敏摘要，且失败已先落 StatusFailed 审计、事务已回滚。
func TestSQLMutate_WriteFailureRepairable(t *testing.T) {
	o, mock, audit, _, ctx := setupOperationTest(t)

	mysqlErr := &mysql.MySQLError{Number: 1054, Message: "Unknown column 'stauts' in 'field list'"}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE orders SET stauts = 'done' WHERE id = 5")).
		WillReturnError(mysqlErr)
	mock.ExpectRollback()
	// UPDATE 无 FROM，referencedTables 为空：即便库名解析失败也降级为 unknown_column 通用提示，
	// 肇事列名经脱敏摘要透传。这里不铺 schema 检索期望，验证降级路径依然产出可修复观察。

	_, err := o.sqlMutate(ctx, &SQLMutateRequest{
		SQL:            "UPDATE orders SET stauts = 'done' WHERE id = 5",
		IdempotencyKey: "k-repair",
	})
	if err == nil {
		t.Fatal("写执行失败应返回错误")
	}
	re, ok := framework.AsRepairable(err)
	if !ok {
		t.Fatalf("写失败应返回可修复观察, got %T: %v", err, err)
	}
	if re.ErrorKind != string(sqlErrUnknownColumn) {
		t.Fatalf("error_kind 应为 unknown_column, got %q", re.ErrorKind)
	}

	var obs repairObservation
	if jerr := json.Unmarshal([]byte(re.Observation), &obs); jerr != nil {
		t.Fatalf("观察应为合法 JSON: %v (%s)", jerr, re.Observation)
	}
	if !obs.Retriable {
		t.Fatal("unknown_column 应标记为可重试（定向改列名）")
	}
	if !strings.Contains(obs.Detail, "stauts") {
		t.Fatalf("脱敏摘要应保留肇事列名（内部库透传）, got %q", obs.Detail)
	}

	// 失败必须已落审计（StatusFailed），且事务回滚（危险确认协议不被绕过：exec-error 不产生 pending）。
	last := audit.lastAudit(t)
	if last.Status != operations.StatusFailed || last.OperationType != operations.OpMutate {
		t.Fatalf("写失败必须落 StatusFailed 审计, got %+v", last)
	}
	for _, r := range audit.records {
		if r.Status == operations.StatusPendingConfirm {
			t.Fatal("执行失败路径绝不应产生 pending_confirm（危险确认协议仅在 dry-run 成功且超阈时触发）")
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db 期望（须回滚）: %v", err)
	}
}

// TestETLRun_SourceFailureRepairableWithColumnHint 校验：ETL 派生的源 SELECT 引用了不存在的列时，
// CTAS 失败被包成可修复观察，并携带从源表检索到的 available_columns 定向线索（buildSchemaHint 全链路）。
func TestETLRun_SourceFailureRepairableWithColumnHint(t *testing.T) {
	o, mock, audit, _, ctx := setupOperationTest(t)

	mysqlErr := &mysql.MySQLError{Number: 1054, Message: "Unknown column 'gvm' in 'field list'"}
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `agent_etl_sales` AS (SELECT region, gvm FROM sales)")).
		WillReturnError(mysqlErr)
	// buildSchemaHint：先解析业务库名（gorm mysql Migrator 两跳：SELECT DATABASE() → SCHEMATA 规范化），
	// 再按源 SELECT 的 FROM 表（sales）取可用列清单。
	mock.ExpectQuery("SELECT DATABASE").
		WillReturnRows(sqlmock.NewRows([]string{"DATABASE()"}).AddRow("linkdb"))
	mock.ExpectQuery("Information_schema.SCHEMATA").
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).AddRow("linkdb"))
	mock.ExpectQuery("information_schema.columns").
		WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "is_nullable", "column_comment"}).
			AddRow("region", "varchar", "YES", "地区").
			AddRow("gmv", "decimal", "YES", "成交额"))

	_, err := o.etlRun(ctx, &ETLRunRequest{
		TargetTable: "agent_etl_sales",
		SQL:         "SELECT region, gvm FROM sales",
	})
	if err == nil {
		t.Fatal("ETL 派生失败应返回错误")
	}
	re, ok := framework.AsRepairable(err)
	if !ok {
		t.Fatalf("ETL 失败应返回可修复观察, got %T: %v", err, err)
	}
	if re.ErrorKind != string(sqlErrUnknownColumn) {
		t.Fatalf("error_kind 应为 unknown_column, got %q", re.ErrorKind)
	}
	// 观察里应含从源表检索到的真实列名清单，引导定向改写（gvm→gmv）。
	if !strings.Contains(re.Observation, "available_columns") ||
		!strings.Contains(re.Observation, "region") ||
		!strings.Contains(re.Observation, "gmv") {
		t.Fatalf("应携带 available_columns 定向线索, got %s", re.Observation)
	}

	// ETL 失败必须已落 StatusFailed 审计。
	last := audit.lastAudit(t)
	if last.Status != operations.StatusFailed || last.OperationType != operations.OpETL {
		t.Fatalf("ETL 失败必须落 StatusFailed 审计, got %+v", last)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db 期望: %v", err)
	}
}

// TestWriteQueryTarget_NilDBDegrades 校验：写库未初始化时 writeQueryTarget 返回 nil，
// newRepairableSQLError 据此降级为通用提示而非 panic（线索检索绝不阻断可修复观察）。
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
