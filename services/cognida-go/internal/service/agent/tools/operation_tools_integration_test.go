//go:build integration

// 操作工具集成测试（真实 MySQL）：
// - sql_mutate 事务内 dry-run + 危险阈值暂停 + 幂等去重
// - etl_run 派生 agent_etl_* 新表且不动源表
// - 全链路审计留痕（agent_operation_audit）
//
// 通过 MYSQL_DSN 提供 DSN，例如：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/cognida?charset=utf8mb4&parseTime=True&loc=Local' \
//	go test -tags=integration ./internal/service/agent/tools/ -run TestOperation -v
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/pendingaction"
	mysqlrepo "cognida/internal/repository/mysql"
)

// setupOperationIntegration 连接真实 DB，准备源表并装配 *opTools 依赖载体。
func setupOperationIntegration(t *testing.T) (*opTools, *gorm.DB, *mysqlrepo.OperationAuditRepository, context.Context) {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN not set; skipping operation tools integration test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// 确保审计表存在（migrate-db 同步的表；这里兜底建表以便独立运行）
	if err := db.AutoMigrate(&mysqlrepo.OperationAuditModel{}); err != nil {
		t.Fatalf("automigrate audit table: %v", err)
	}

	// 源表：固定 3 行数据
	source := "it_op_source"
	db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", source))
	if err := db.Exec(fmt.Sprintf(
		"CREATE TABLE `%s` (id INT PRIMARY KEY, region VARCHAR(32), gmv DOUBLE)", source)).Error; err != nil {
		t.Fatalf("create source table: %v", err)
	}
	if err := db.Exec(fmt.Sprintf(
		"INSERT INTO `%s` (id, region, gmv) VALUES (1,'华北',100),(2,'华东',200),(3,'华南',300)", source)).Error; err != nil {
		t.Fatalf("seed source table: %v", err)
	}
	t.Cleanup(func() { db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", source)) })

	auditRepo := mysqlrepo.NewOperationAuditRepository(db)
	o := &opTools{cfg: OperationConfig{
		DB:                 db,
		Audit:              auditRepo,
		Pending:            pendingaction.NewMemoryStore(),
		RedlineTables:      []string{"users", "tenants"},
		DangerRowThreshold: 2, // 低阈值便于触发危险分级
		ExportDir:          t.TempDir(),
	}}

	ctx := agentctx.WithTenantID(context.Background(), 990)
	ctx = agentctx.WithSessionID(ctx, "sess-op-it")
	return o, db, auditRepo, ctx
}

// countSourceRows 源表当前行数。
func countSourceRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT COUNT(*) FROM it_op_source").Scan(&n).Error; err != nil {
		t.Fatalf("count source: %v", err)
	}
	return n
}

// lastAuditRow 该租户最近一条审计。
func lastAuditRow(t *testing.T, db *gorm.DB) *mysqlrepo.OperationAuditModel {
	t.Helper()
	var m mysqlrepo.OperationAuditModel
	if err := db.Where("tenant_id = ?", 990).Order("id DESC").First(&m).Error; err != nil {
		t.Fatalf("load last audit: %v", err)
	}
	return &m
}

func TestOperationIntegration_MutateDryRunSuspendsAndResumes(t *testing.T) {
	o, db, _, ctx := setupOperationIntegration(t)

	// 影响 3 行 ≥ 阈值 2 → 事务回滚 + 暂停；源表必须原样
	res, err := o.sqlMutate(ctx, &SQLMutateRequest{
		SQL:            "UPDATE it_op_source SET gmv = gmv * 2",
		IdempotencyKey: "it-danger-1",
	})
	if err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if res.Status != operations.StatusPendingConfirm || res.RowsAffected != 3 {
		t.Fatalf("expected pending_confirm affecting 3 rows, got %+v", res)
	}
	var gmv float64
	db.Raw("SELECT gmv FROM it_op_source WHERE id = 1").Scan(&gmv)
	if gmv != 100 {
		t.Fatalf("dry-run must rollback, gmv=%v", gmv)
	}
	if a := lastAuditRow(t, db); a.Status != operations.StatusPendingConfirm {
		t.Fatalf("audit should be pending_confirm, got %s", a.Status)
	}

	// 人工确认后恢复执行
	owner := operationOwner(ctx)
	action, err := o.cfg.Pending.Consume(ctx, owner, res.PendingActionID, res.ConfirmToken)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	confirmed, err := o.ExecuteConfirmedMutation(ctx, action)
	if err != nil {
		t.Fatalf("confirmed execute: %v", err)
	}
	if confirmed.Status != operations.StatusSuccess || confirmed.RowsAffected != 3 {
		t.Fatalf("expected confirmed success, got %+v", confirmed)
	}
	db.Raw("SELECT gmv FROM it_op_source WHERE id = 1").Scan(&gmv)
	if gmv != 200 {
		t.Fatalf("confirmed mutation should be committed, gmv=%v", gmv)
	}
	if a := lastAuditRow(t, db); a.Status != operations.StatusSuccess {
		t.Fatalf("audit should be success, got %s", a.Status)
	}

	// 幂等去重：同键再次提交不再写库
	dup, err := o.sqlMutate(ctx, &SQLMutateRequest{
		SQL:            "UPDATE it_op_source SET gmv = gmv * 2",
		IdempotencyKey: "it-danger-1",
	})
	if err != nil {
		t.Fatalf("duplicate mutate: %v", err)
	}
	if !dup.Duplicate {
		t.Fatalf("expected idempotent duplicate, got %+v", dup)
	}
	db.Raw("SELECT gmv FROM it_op_source WHERE id = 1").Scan(&gmv)
	if gmv != 200 {
		t.Fatalf("idempotent duplicate must not re-write, gmv=%v", gmv)
	}
}

func TestOperationIntegration_MutateSafeCommit(t *testing.T) {
	o, db, _, ctx := setupOperationIntegration(t)

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{
		SQL:            "UPDATE it_op_source SET region = '华中' WHERE id = 1",
		IdempotencyKey: "it-safe-1",
	})
	if err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowsAffected != 1 {
		t.Fatalf("expected success 1 row, got %+v", res)
	}
	var region string
	db.Raw("SELECT region FROM it_op_source WHERE id = 1").Scan(&region)
	if region != "华中" {
		t.Fatalf("safe mutation should commit, region=%s", region)
	}
	if a := lastAuditRow(t, db); a.Status != operations.StatusSuccess || a.IdempotencyKey != "it-safe-1" {
		t.Fatalf("audit mismatch: %+v", a)
	}
}

func TestOperationIntegration_ETLDerivesWithoutTouchingSource(t *testing.T) {
	o, db, _, ctx := setupOperationIntegration(t)

	target := "agent_etl_it_regions"
	db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", target))
	t.Cleanup(func() { db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", target)) })

	before := countSourceRows(t, db)
	res, err := o.etlRun(ctx, &ETLRunRequest{
		TargetTable: target,
		SQL:         "SELECT region, SUM(gmv) AS total FROM it_op_source GROUP BY region",
	})
	if err != nil {
		t.Fatalf("etl: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowCount != 3 {
		t.Fatalf("expected 3 derived rows, got %+v", res)
	}

	// 派生表存在且行数正确
	var derived int64
	db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", target)).Scan(&derived)
	if derived != 3 {
		t.Fatalf("derived table should have 3 rows, got %d", derived)
	}
	// 源表原样
	if after := countSourceRows(t, db); after != before {
		t.Fatalf("source table must be untouched: before=%d after=%d", before, after)
	}
	if a := lastAuditRow(t, db); a.OperationType != operations.OpETL || a.Status != operations.StatusSuccess {
		t.Fatalf("etl audit mismatch: %+v", a)
	}

	// 前缀违规被拒且留痕
	rej, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "it_op_source_copy", SQL: "SELECT 1"})
	if err != nil {
		t.Fatalf("etl reject: %v", err)
	}
	if rej.Status != operations.StatusRejected {
		t.Fatalf("expected prefix rejection, got %+v", rej)
	}
	if a := lastAuditRow(t, db); a.Status != operations.StatusRejected {
		t.Fatalf("prefix violation must be audited, got %s", a.Status)
	}
}

func TestOperationIntegration_RedlineRejectedAndAudited(t *testing.T) {
	o, db, _, ctx := setupOperationIntegration(t)

	res, err := o.sqlMutate(ctx, &SQLMutateRequest{
		SQL:            "DELETE FROM users WHERE id = 999999",
		IdempotencyKey: "it-redline-1",
	})
	if err != nil {
		t.Fatalf("mutate: %v", err)
	}
	if res.Status != operations.StatusRejected || !strings.Contains(res.Message, "红线") {
		t.Fatalf("expected redline rejection, got %+v", res)
	}
	if a := lastAuditRow(t, db); a.Status != operations.StatusRejected {
		t.Fatalf("redline rejection must be audited, got %s", a.Status)
	}
}
