// Package tools 测试操作工具（sql_mutate / etl_run / data_export）。
package tools

import (
	"context"
	"encoding/csv"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/pendingaction"
	"cognida/internal/service/agent/resultstore"
)

// ========================================
// 测试脚手架
// ========================================

// fakeAuditRepo 进程内审计仓储：记录所有留痕，支持预置幂等命中。
type fakeAuditRepo struct {
	records []*operations.OperationAudit
	prior   *operations.OperationAudit
}

func (f *fakeAuditRepo) Record(ctx context.Context, audit *operations.OperationAudit) error {
	f.records = append(f.records, audit)
	return nil
}

func (f *fakeAuditRepo) FindByIdempotencyKey(ctx context.Context, tenantID int64, key string) (*operations.OperationAudit, error) {
	if f.prior != nil && f.prior.IdempotencyKey == key {
		return f.prior, nil
	}
	return nil, nil
}

// lastAudit 最近一条审计。
func (f *fakeAuditRepo) lastAudit(t *testing.T) *operations.OperationAudit {
	t.Helper()
	if len(f.records) == 0 {
		t.Fatalf("expected at least one audit record")
	}
	return f.records[len(f.records)-1]
}

// setupOperationTest 构造 sqlmock DB + 假审计 + 内存 pending store，装配成 *opTools 依赖载体。
func setupOperationTest(t *testing.T) (*opTools, sqlmock.Sqlmock, *fakeAuditRepo, *pendingaction.MemoryStore, context.Context) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}

	audit := &fakeAuditRepo{}
	pending := pendingaction.NewMemoryStore()

	o := &opTools{cfg: OperationConfig{
		DB:                 gormDB,
		Audit:              audit,
		Pending:            pending,
		RedlineTables:      []string{"users", "tenants"},
		DangerRowThreshold: 100,
		ExportDir:          t.TempDir(),
	}}

	ctx := agentctx.WithTenantID(context.Background(), 1)
	ctx = agentctx.WithSessionID(ctx, "sess-op")
	return o, mock, audit, pending, ctx
}

// ========================================
// 公共层校验
// ========================================

func TestValidateDML(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{"valid insert", "INSERT INTO orders (id) VALUES (1)", false},
		{"valid update", "UPDATE orders SET status = 'done' WHERE id = 1", false},
		{"valid delete", "DELETE FROM orders WHERE id = 1", false},
		{"trailing semicolon ok", "DELETE FROM orders WHERE id = 1;", false},
		{"reject select", "SELECT * FROM orders", true},
		{"reject create", "CREATE TABLE t (id INT)", true},
		{"reject drop", "DROP TABLE orders", true},
		{"reject alter", "ALTER TABLE orders ADD c INT", true},
		{"reject truncate", "TRUNCATE TABLE orders", true},
		{"reject embedded ddl", "UPDATE orders SET note = 1 WHERE id = 1 OR (SELECT 1 FROM x); DROP TABLE orders", true},
		{"reject comment", "UPDATE orders SET a = 1 -- hidden", true},
		{"reject multi statement", "UPDATE orders SET a = 1; UPDATE orders SET b = 2", true},
		{"reject empty", "  ", true},
		// 多表写防护：JOIN/USING/多表形式可携带第二写目标绕过红线校验
		{"reject update join", "UPDATE agent_etl_tmp JOIN users ON agent_etl_tmp.uid = users.id SET users.pwd = 'x'", true},
		{"reject update comma multi-table", "UPDATE agent_etl_tmp, users SET users.pwd = 'x'", true},
		{"reject delete multi-table", "DELETE users FROM users JOIN agent_etl_tmp ON users.id = agent_etl_tmp.uid", true},
		{"reject delete using", "DELETE FROM users USING users JOIN agent_etl_tmp ON 1=1", true},
		{"allow update qualified table", "UPDATE db1.orders SET a = 1 WHERE id = 1", false},
		{"allow delete with subquery", "DELETE FROM orders WHERE id IN (SELECT id FROM t1)", false},
		{"allow insert select join (join只读)", "INSERT INTO agent_etl_tmp SELECT u.id FROM users u JOIN orders o ON u.id = o.uid", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateDML(tt.sql); (err != nil) != tt.wantErr {
				t.Errorf("validateDML(%q) err=%v wantErr=%v", tt.sql, err, tt.wantErr)
			}
		})
	}
}

func TestExtractMutateTarget(t *testing.T) {
	tests := []struct {
		sql  string
		want string
	}{
		{"INSERT INTO orders (id) VALUES (1)", "orders"},
		// 反引号限定名必须完整提取（提取半截会让红线校验错判库名为表名）
		{"insert ignore into `db1`.`x` values (1)", "db1.x"},
		{"INSERT INTO `db1`.`users` VALUES (1)", "db1.users"},
		{"UPDATE agent_etl_tmp SET a = 1", "agent_etl_tmp"},
		{"UPDATE IGNORE agent_etl_tmp SET a = 1", "agent_etl_tmp"},
		{"DELETE FROM sales.orders WHERE 1", "sales.orders"},
		{"SELECT 1", ""},
	}
	for _, tt := range tests {
		if got := extractMutateTarget(tt.sql); got != tt.want {
			t.Errorf("extractMutateTarget(%q) = %q, want %q", tt.sql, got, tt.want)
		}
	}
}

func TestValidateETLTarget(t *testing.T) {
	if err := validateETLTarget("agent_etl_sales_2026"); err != nil {
		t.Errorf("valid target rejected: %v", err)
	}
	for _, bad := range []string{"orders", "etl_orders", "agent_etl_", "agent_etl_a;drop", "agent_etl_a b"} {
		if err := validateETLTarget(bad); err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
	}
}

// ========================================
// sql_mutate
// ========================================

func TestSQLMutate_RejectDDL(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "DROP TABLE orders", IdempotencyKey: "k-ddl"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected {
		t.Fatalf("expected rejected, got %s", res.Status)
	}
	// 被拒也必须留痕
	last := audit.lastAudit(t)
	if last.Status != operations.StatusRejected || last.OperationType != operations.OpMutate {
		t.Fatalf("audit mismatch: %+v", last)
	}
}

func TestSQLMutate_RejectRedlineTable(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "DELETE FROM users WHERE id = 1", IdempotencyKey: "k-red"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected || !strings.Contains(res.Message, "红线") {
		t.Fatalf("expected redline rejection, got %+v", res)
	}
	if audit.lastAudit(t).Status != operations.StatusRejected {
		t.Fatalf("redline rejection must be audited")
	}
}

func TestSQLMutate_IdempotentDuplicate(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)
	audit.prior = &operations.OperationAudit{
		IdempotencyKey: "k-dup",
		Target:         "orders",
		Status:         operations.StatusSuccess,
		Result:         "影响 3 行",
	}

	// 幂等命中：不应有任何 DB 交互（sqlmock 未设置期望，若交互会报错）
	res, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "UPDATE orders SET a = 1", IdempotencyKey: "k-dup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Duplicate || res.Status != operations.StatusSuccess {
		t.Fatalf("expected duplicate success, got %+v", res)
	}
}

func TestSQLMutate_SafeCommit(t *testing.T) {
	o, mock, audit, _, ctx := setupOperationTest(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status = 'done' WHERE id = 5")).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "UPDATE orders SET status = 'done' WHERE id = 5", IdempotencyKey: "k-ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowsAffected != 3 {
		t.Fatalf("expected success 3 rows, got %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db expectations: %v", err)
	}
	last := audit.lastAudit(t)
	if last.Status != operations.StatusSuccess || last.IdempotencyKey != "k-ok" {
		t.Fatalf("audit mismatch: %+v", last)
	}
}

func TestSQLMutate_DangerThresholdSuspends(t *testing.T) {
	o, mock, audit, pending, ctx := setupOperationTest(t)

	// 影响 250 行 ≥ 阈值 100 → 回滚 + 暂停
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM orders WHERE created_at < '2020-01-01'")).
		WillReturnResult(sqlmock.NewResult(0, 250))
	mock.ExpectRollback()

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "DELETE FROM orders WHERE created_at < '2020-01-01'", IdempotencyKey: "k-danger"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusPendingConfirm {
		t.Fatalf("expected pending_confirm, got %+v", res)
	}
	if res.PendingActionID == "" || res.ConfirmToken == "" {
		t.Fatalf("expected pending_action_id + token, got %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db expectations (must rollback): %v", err)
	}
	if audit.lastAudit(t).Status != operations.StatusPendingConfirm {
		t.Fatalf("suspension must be audited as pending_confirm")
	}

	// 暂存的 action 可被正确的 owner+token 消费
	owner := resultstore.OwnerKey(1, "sess-op")
	action, err := pending.Consume(ctx, owner, res.PendingActionID, res.ConfirmToken)
	if err != nil {
		t.Fatalf("consume pending action: %v", err)
	}
	if action.SQL != "DELETE FROM orders WHERE created_at < '2020-01-01'" {
		t.Fatalf("pending action SQL mismatch: %q", action.SQL)
	}
	if key, _ := action.Params["idempotency_key"].(string); key != "k-danger" {
		t.Fatalf("pending action should carry idempotency_key, got %v", action.Params)
	}
}

func TestSQLMutate_DangerWithoutPendingStoreRejects(t *testing.T) {
	o, mock, audit, _, ctx := setupOperationTest(t)
	o.cfg.Pending = nil // 宁拒不闯

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM orders")).
		WillReturnResult(sqlmock.NewResult(0, 500))
	mock.ExpectRollback()

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "DELETE FROM orders", IdempotencyKey: "k-nopending"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected {
		t.Fatalf("expected rejected without pending store, got %+v", res)
	}
	if audit.lastAudit(t).Status != operations.StatusRejected {
		t.Fatalf("rejection must be audited")
	}
}

func TestSQLMutate_MissingIdempotencyKey(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)
	if _, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: "UPDATE orders SET a = 1"}); err == nil {
		t.Fatalf("expected error without idempotency_key")
	}
}

// ========================================
// etl_run
// ========================================

func TestETLRun_RejectBadPrefix(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)

	res, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "orders_backup", SQL: "SELECT * FROM orders"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected || !strings.Contains(res.Message, ETLTablePrefix) {
		t.Fatalf("expected prefix rejection, got %+v", res)
	}
	if audit.lastAudit(t).Status != operations.StatusRejected {
		t.Fatalf("prefix violation must be audited")
	}
}

func TestETLRun_SourceExclusive(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)

	if _, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_x"}); err == nil {
		t.Fatalf("expected error when no source given")
	}
	if _, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_x", SQL: "SELECT 1", ResultID: "rs_1"}); err == nil {
		t.Fatalf("expected error when both sources given")
	}
}

func TestETLRun_RejectNonSelectSource(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)

	_, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_x", SQL: "DELETE FROM orders"})
	if err == nil || !strings.Contains(err.Error(), "SELECT") {
		t.Fatalf("expected SELECT-only source error, got %v", err)
	}
	if audit.lastAudit(t).Status != operations.StatusFailed {
		t.Fatalf("source validation failure must be audited")
	}
}

func TestETLRun_FromSQL(t *testing.T) {
	o, mock, audit, _, ctx := setupOperationTest(t)

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `agent_etl_sales` AS (SELECT region, gmv FROM sales)")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM `agent_etl_sales`")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	res, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_sales", SQL: "SELECT region, gmv FROM sales"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowCount != 42 {
		t.Fatalf("expected success 42 rows, got %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db expectations: %v", err)
	}
	if audit.lastAudit(t).Status != operations.StatusSuccess {
		t.Fatalf("etl success must be audited")
	}
}

func TestETLRun_FromResultID_CrossSessionRejected(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)
	o.resultStore = resultstore.NewMemoryStore()

	// 结果集属于其他会话
	id, err := o.resultStore.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(2, "other"),
		Columns: []string{"a"},
		Rows:    []map[string]interface{}{{"a": 1}},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	_, err = o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_x", ResultID: id})
	if err == nil || !strings.Contains(err.Error(), "不属于当前会话") {
		t.Fatalf("expected cross-session rejection, got %v", err)
	}
}

// ========================================
// data_export
// ========================================

// setupExportStore 为 o 注入内存 Result Store 并放入一份结果集。
func setupExportStore(t *testing.T, o *opTools, ctx context.Context) string {
	t.Helper()
	o.resultStore = resultstore.NewMemoryStore()

	id, err := o.resultStore.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(1, "sess-op"),
		Columns: []string{"month", "gmv"},
		Rows: []map[string]interface{}{
			{"month": "2026-01", "gmv": 1200.5},
			{"month": "2026-02", "gmv": 900.0},
		},
	}, resultstore.DefaultTTL)
	if err != nil {
		t.Fatalf("put result: %v", err)
	}
	return id
}

func TestDataExport_CSV(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)
	id := setupExportStore(t, o, ctx)

	res, err := o.dataExport(ctx, &DataExportRequest{ResultID: id, Format: "csv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowCount != 2 {
		t.Fatalf("expected success 2 rows, got %+v", res)
	}

	// 校验文件内容：BOM + 表头 + 2 行；返回的是引用而非内容
	raw, err := os.ReadFile(res.FilePath)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	content := strings.TrimPrefix(string(raw), "\xEF\xBB\xBF")
	records, err := csv.NewReader(strings.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 3 || records[0][0] != "month" || records[1][0] != "2026-01" {
		t.Fatalf("csv content mismatch: %v", records)
	}
	if audit.lastAudit(t).Status != operations.StatusSuccess || audit.lastAudit(t).OperationType != operations.OpExport {
		t.Fatalf("export must be audited")
	}
}

func TestDataExport_Excel(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)
	id := setupExportStore(t, o, ctx)

	res, err := o.dataExport(ctx, &DataExportRequest{ResultID: id, Format: "excel", Filename: "报表 2026"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusSuccess || !strings.HasSuffix(res.FilePath, ".xlsx") {
		t.Fatalf("expected xlsx export, got %+v", res)
	}
	if fi, err := os.Stat(res.FilePath); err != nil || fi.Size() == 0 {
		t.Fatalf("xlsx file missing or empty: %v", err)
	}
}

func TestDataExport_CrossSessionRejected(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)
	o.resultStore = resultstore.NewMemoryStore()

	id, _ := o.resultStore.Put(ctx, &resultstore.Result{
		Owner:   resultstore.OwnerKey(9, "someone-else"),
		Columns: []string{"a"},
		Rows:    []map[string]interface{}{{"a": 1}},
	}, resultstore.DefaultTTL)

	res, err := o.dataExport(ctx, &DataExportRequest{ResultID: id})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected || !strings.Contains(res.Message, "不属于当前会话") {
		t.Fatalf("expected cross-session rejection, got %+v", res)
	}
	if audit.lastAudit(t).Status != operations.StatusRejected {
		t.Fatalf("cross-session export attempt must be audited")
	}
}

func TestDataExport_NotFound(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)
	o.resultStore = resultstore.NewMemoryStore()

	res, err := o.dataExport(ctx, &DataExportRequest{ResultID: "rs_missing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected || !strings.Contains(res.Message, "不存在或已过期") {
		t.Fatalf("expected not-found rejection, got %+v", res)
	}
}
