//go:build integration

package tools

import (
	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	mysqlrepo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/pendingaction"
	"context"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"testing"
	"time"
)

// Successful SQL/ETL commits, cancellation, rollback and HTTP confirmation are
// exercised by service/governedops/integration_test.go with the real service.
// This test verifies legacy tool/token routes cannot silently retain write power.
func TestOperationIntegrationLegacyToolsCannotMutateOrDropTables(t *testing.T) {
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL required")
	}
	cfg, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	require.NotContains(t, cfg.DBName, "e2e")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	name := "agent_etl_guard_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE TABLE `"+name+"` (value VARCHAR(40))").Error)
	require.NoError(t, db.Exec("INSERT INTO `"+name+"` VALUES (?)", "00123").Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+name+"`").Error)
		require.NoError(t, db.Table("agent_operation_audit").Where("tenant_id=?", tenant).Delete(map[string]any{}).Error)
		require.NoError(t, sql.Close())
	})
	ctx := agentctx.WithUserID(agentctx.WithTenantID(context.Background(), tenant), 619)
	o := &opTools{cfg: OperationConfig{DB: db, Audit: mysqlrepo.NewOperationAuditRepository(db)}}
	_, e = o.sqlMutate(ctx, &SQLMutateRequest{SQL: "UPDATE `" + name + "` SET value='changed' WHERE value='00123'", IdempotencyKey: "legacy"})
	require.Error(t, e)
	_, e = o.etlRun(ctx, &ETLRunRequest{TargetTable: name, SQL: "SELECT 1 AS value", Replace: true, IdempotencyKey: "legacy-etl"})
	require.Error(t, e)
	result, e := o.ExecuteConfirmedMutation(ctx, &pendingaction.PendingAction{SQL: "DELETE FROM `" + name + "`"})
	require.NoError(t, e)
	require.Equal(t, operations.StatusRejected, result.Status)
	etl, e := o.ExecuteConfirmedETL(ctx, &pendingaction.PendingAction{})
	require.NoError(t, e)
	require.Equal(t, operations.StatusRejected, etl.Status)
	var value string
	require.NoError(t, db.Raw("SELECT value FROM `"+name+"`").Scan(&value).Error)
	require.Equal(t, "00123", value)
}
