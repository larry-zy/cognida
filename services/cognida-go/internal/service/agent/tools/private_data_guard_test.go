package tools

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	"cognida/internal/model/dataprofile"
	"cognida/internal/service/agent/resultstore"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPrivateMemorySQLReadAndWriteAttacks(t *testing.T) {
	reads := []string{
		"SELECT content FROM user_memories",
		"SELECT content FROM cognida.USER_MEMORIES",
		"SELECT content FROM `cognida`.`UsEr_MeMoRiEs`",
		"SELECT content FROM \"user_memories\"",
		"SELECT content FROM [user_memories]",
		"SELECT m.content FROM orders o, user_memories m",
		"SELECT o.id FROM orders o JOIN user_memories m ON o.id=m.user_id",
		"SELECT * FROM orders UNION SELECT * FROM user_memories",
		"WITH secret AS (SELECT * FROM user_memories) SELECT * FROM secret",
		"SELECT (SELECT content FROM user_memories LIMIT 1)",
		"SELECT * FROM/**/`user_memories`",
		"SELECT * FROM `cognida` /*hidden*/ . `user_memories`",
		"SELECT * FROM orders # comment\n UNION SELECT * FROM user_memories",
	}
	for _, query := range reads {
		t.Run(query, func(t *testing.T) {
			require.ErrorContains(t, validateSQL(query), "private system tables")
			require.ErrorContains(t, NewSQLValidator().Validate(query), "private system tables")
			// Reject before routing/SQL execution, including explicit and session-selected external connections.
			for _, datasource := range []string{"", "external-back-to-app"} {
				ctx := agentctx.WithDatasourceID(context.Background(), datasource)
				_, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: query, DatabaseID: datasource}, nil, nil, nil, nil, nil)
				require.ErrorContains(t, err, "private system tables")
			}
		})
	}
	writes := []string{
		"UPDATE user_memories SET content='changed' WHERE id='other-owner'",
		"DELETE FROM `cognida`.`USER_MEMORIES` WHERE id='other-owner'",
		"INSERT INTO user_memories (content) VALUES ('inferred')",
		"INSERT INTO agent_etl_leak SELECT * FROM user_memories",
		"UPDATE orders SET note=(SELECT content FROM user_memories LIMIT 1)",
	}
	for _, query := range writes {
		require.ErrorContains(t, validateDML(query), "private system tables")
		o, mock, audit, _, ctx := setupOperationTest(t)
		result, err := o.sqlMutate(ctx, &SQLMutateRequest{SQL: query, IdempotencyKey: "private-attempt"})
		require.NoError(t, err)
		require.Equal(t, operations.StatusRejected, result.Status)
		require.Equal(t, operations.StatusRejected, audit.lastAudit(t).Status)
		require.NoError(t, mock.ExpectationsWereMet()) // no mutation or transaction was issued
	}
	require.NoError(t, validateSQL("SELECT * FROM orders WHERE note='remember my preference'"))
	require.NoError(t, validateSQL("SELECT * FROM user_memories_archive"))
}

func TestPrivateMemorySchemaAndProfileCannotDiscloseData(t *testing.T) {
	for _, table := range []string{"user_memories", "USER_MEMORIES", "cognida.user_memories", "`cognida`.`user_memories`"} {
		_, err := FetchSchema(context.Background(), nil, nil, "external", table)
		require.ErrorContains(t, err, "private system tables")
		_, err = getSchema(context.Background(), &GetSchemaRequest{TableName: table, DatabaseID: "external"}, nil, nil, nil)
		require.ErrorContains(t, err, "private system tables")
		_, err = queryTableColumns(context.Background(), nil, table)
		require.ErrorContains(t, err, "private system tables")
		_, err = queryPrimaryKey(context.Background(), nil, table)
		require.ErrorContains(t, err, "private system tables")
		schema := TableSchema{TableName: table, Columns: []ColumnSchema{{Name: "content", Type: "text"}}}
		_, err = computeColumnProfiles(context.Background(), nil, 1, "external", "app", schema)
		require.ErrorContains(t, err, "private system tables")
		store := &fakeProfileStore{profiles: []*dataprofile.ColumnProfile{{ColumnName: "content", TopValues: []dataprofile.ValueFrequency{{Value: "private"}}}}}
		stale, ok := attachCachedFacts(context.Background(), store, nil, "", &schema)
		require.False(t, stale)
		require.False(t, ok)
		require.Nil(t, schema.Columns[0].Facts)
		require.Nil(t, columnFactsHint(context.Background(), store, nil, "", table))
		triggerBackgroundProfile(store, nil, nil, 1, "", "", schema) // denied before spawning work
	}
}

func TestPrivateMemoryOmittedFromSchemaDiscovery(t *testing.T) {
	db, mock := setupMockSchemaDB(t)
	raw, err := db.DB()
	require.NoError(t, err)
	defer raw.Close()
	target := &queryTarget{db: raw, dbName: "app"}
	expectLoadTableCards(mock, "app",
		sqlmock.NewRows([]string{"table_name", "table_comment"}).AddRow("USER_MEMORIES", "private").AddRow("orders", "orders"),
		sqlmock.NewRows([]string{"table_name", "column_name", "column_comment"}).AddRow("USER_MEMORIES", "content", "private").AddRow("orders", "id", "id"))
	cards, err := loadTableCards(context.Background(), target)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, "orders", cards[0].Name)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' ORDER BY table_name")).WithArgs("app").WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("user_memories").AddRow("orders"))
	names, err := listTableNames(context.Background(), target)
	require.NoError(t, err)
	require.Equal(t, []string{"orders"}, names)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrivateMemoryETLRejectedBeforeReplace(t *testing.T) {
	o, mock, _, _, ctx := setupOperationTest(t)
	_, err := o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_private", SQL: "SELECT * FROM user_memories", Replace: true})
	require.ErrorContains(t, err, "private system tables")
	err = validateSQL("SELECT * FROM `app`.`user_memories`")
	require.ErrorContains(t, err, "private system tables")
	require.NoError(t, mock.ExpectationsWereMet()) // no DROP or CREATE
}

func TestPrivateMemoryResultCannotExportOrMaterialize(t *testing.T) {
	o, mock, _, _, ctx := setupOperationTest(t)
	o.resultStore = resultstore.NewMemoryStore()
	id, err := o.resultStore.Put(ctx, &resultstore.Result{
		Owner: resultstore.OwnerKey(1, "sess-op"), SourceSQL: "SELECT content FROM `app`.`user_memories`",
		Columns: []string{"content"}, Rows: []map[string]interface{}{{"content": "private content"}},
	}, resultstore.DefaultTTL)
	require.NoError(t, err)
	_, err = o.dataExport(ctx, &DataExportRequest{ResultID: id, Format: "csv"})
	require.ErrorContains(t, err, "private system tables")
	_, err = o.loadETLResult(ctx, id)
	require.ErrorContains(t, err, "private system tables")
	_, err = o.etlRun(ctx, &ETLRunRequest{TargetTable: "agent_etl_private", ResultID: id, Replace: true})
	require.ErrorContains(t, err, "private system tables")
	files, err := os.ReadDir(o.cfg.ExportDir)
	require.NoError(t, err)
	require.Empty(t, files)
	require.NoError(t, mock.ExpectationsWereMet())
	// Identifier matching is exact; harmless similarly named business tables stay usable.
	require.NoError(t, validatePrivateDataAccess(strings.Repeat("x", 10)+"user_memories"))
}
