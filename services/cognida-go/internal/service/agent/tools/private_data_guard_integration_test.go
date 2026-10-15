//go:build integration

package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	memorymodel "cognida/internal/model/memory"
	repository "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	memorysvc "cognida/internal/service/memory"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exercise the actual database and the alternate connection route. The only
// private row and derived table created here have a random test owner/name.
func TestPrivateMemorySQLIntegration(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	config, err := mysqlDriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(config.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	raw, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	scope := memorymodel.Scope{TenantID: tenant, UserID: 51}
	repo := repository.NewLongTermMemoryRepository(db)
	service := memorysvc.NewService(repo, nil, "")
	secret := "private-isolation-" + uuid.NewString()
	memory, err := service.Create(context.Background(), scope, memorymodel.WriteInput{Category: "fact", Content: secret}, uuid.NewString(), "user")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Table("user_memories").Where("tenant_id = ?", tenant).Delete(map[string]any{}).Error)
	})
	ctx := agentctx.WithUserID(agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), tenant+1), "private-guard-test"), 52)
	dsp := &fakeConnProvider{id: "loopback-app", dbName: config.DBName, db: raw}
	query := fmt.Sprintf("SELECT content FROM `%s`.`USER_MEMORIES` WHERE id='%s'", config.DBName, memory.ID)
	for _, source := range []string{"", "loopback-app"} {
		_, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: query, DatabaseID: source}, db, dsp, nil, nil, nil)
		require.ErrorContains(t, err, "private system tables")
		_, err = FetchSchema(ctx, db, dsp, source, "user_memories")
		require.ErrorContains(t, err, "private system tables")
		_, err = getSchema(ctx, &GetSchemaRequest{DatabaseID: source, TableName: "user_memories"}, db, dsp, nil)
		require.ErrorContains(t, err, "private system tables")
		allowed, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: "SELECT 7 AS allowed", DatabaseID: source}, db, dsp, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 1, allowed.RowCount)
	}
	cards, err := loadTableCards(ctx, &queryTarget{db: raw, dbName: config.DBName})
	require.NoError(t, err)
	for _, card := range cards {
		require.NotEqual(t, "user_memories", strings.ToLower(card.Name))
	}
	_, err = computeColumnProfiles(ctx, raw, tenant+1, "", config.DBName,
		TableSchema{TableName: "user_memories", Columns: []ColumnSchema{{Name: "content", Type: "text"}}})
	require.ErrorContains(t, err, "private system tables")
	audit := &fakeAuditRepo{}
	o := &opTools{cfg: OperationConfig{DB: db, Audit: audit, ExportDir: t.TempDir()}, resultStore: resultstore.NewMemoryStore()}
	mutation, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: fmt.Sprintf("UPDATE user_memories SET content='overwritten' WHERE id='%s'", memory.ID), IdempotencyKey: uuid.NewString()})
	require.NoError(t, err)
	require.Equal(t, operations.StatusRejected, mutation.Status)
	unchanged, err := repo.Get(ctx, scope, memory.ID)
	require.NoError(t, err)
	require.Equal(t, secret, unchanged.Content)
	require.EqualValues(t, 1, unchanged.Version)

	target := "agent_etl_guard_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	require.NoError(t, db.Exec("CREATE TABLE `"+target+"` (id INT)").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE IF EXISTS `"+target+"`").Error) })
	require.NoError(t, db.Exec("INSERT INTO `"+target+"` VALUES (7)").Error)
	_, err = o.etlRun(ctx, &ETLRunRequest{TargetTable: target, SQL: query, Replace: true})
	require.ErrorContains(t, err, "private system tables")
	id, err := o.resultStore.Put(ctx, &resultstore.Result{Owner: resultstore.OwnerKey(tenant+1, "private-guard-test"), SourceSQL: query,
		Columns: []string{"content"}, Rows: []map[string]interface{}{{"content": secret}}}, resultstore.DefaultTTL)
	require.NoError(t, err)
	_, err = o.etlRun(ctx, &ETLRunRequest{TargetTable: target, ResultID: id, Replace: true})
	require.ErrorContains(t, err, "private system tables")
	_, err = o.dataExport(ctx, &DataExportRequest{ResultID: id, Format: "csv"})
	require.ErrorContains(t, err, "private system tables")
	var value int
	require.NoError(t, db.Raw("SELECT id FROM `"+target+"`").Scan(&value).Error)
	require.Equal(t, 7, value, "rejected private input must not DROP the previous derived table")
	files, err := os.ReadDir(o.cfg.ExportDir)
	require.NoError(t, err)
	require.Empty(t, files)
}
