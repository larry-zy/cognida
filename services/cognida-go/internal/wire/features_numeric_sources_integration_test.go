//go:build integration

package wire

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	"cognida/internal/service/lifecycle"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
)

func TestFeatureSourcesHTTPDeclaredSQLNumericPrecision(t *testing.T) {
	f := newSourceHTTPFixture(t)
	ctx := context.Background()
	// Only this fixture's randomly named external-source table is changed.
	require.NoError(t, f.db.Exec("ALTER TABLE `"+f.table+"` MODIFY amount DECIMAL(38,18) NOT NULL, ADD code VARCHAR(32), ADD big_id BIGINT").Error)
	require.NoError(t, f.db.Exec("UPDATE `"+f.table+"` SET amount=?,code=?,big_id=?", "100.123456789012345678", "00123", "9007199254740993").Error)
	require.NoError(t, f.db.Exec("INSERT INTO `"+f.table+"` (region,amount,code,big_id) VALUES (?,?,?,?)", "south", "200.123456789012345678", "00456", "9007199254740995").Error)
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	require.NoError(t, err)
	input := f.acquire(t, lifecycle.SourceRequest{Kind: "datasource", DatasourceID: f.dsID, Table: f.table}, f.dsID+"/"+cfg.DBName+"/"+f.table)
	require.Equal(t, []string{"region", "amount", "code", "big_id"}, input.Columns)
	require.Equal(t, "DECIMAL", input.ColumnTypes["amount"])
	require.Equal(t, "VARCHAR", input.ColumnTypes["code"])
	require.Equal(t, "BIGINT", input.ColumnTypes["big_id"])
	require.Equal(t, []any{"north", "100.123456789012345678", "00123", "9007199254740993"}, input.Rows[0])
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	stats, err := lifecycle.AnalyzeInput(input)
	require.NoError(t, err)
	columns := stats["columns"].([]map[string]any)
	require.Equal(t, 2, columns[1]["numeric_count"])
	require.InDelta(t, 150.123456789012345678, columns[1]["mean"], 1e-10)
	require.Equal(t, 0, columns[2]["numeric_count"], "numeric-looking VARCHAR must stay textual")
	again, err := json.Marshal(input)
	require.NoError(t, err)
	require.Equal(t, string(raw), string(again), "analysis cannot round the immutable source payload")

	// Exercise actual MySQL expression metadata, Redis exact read, and HTTP
	// source acquisition together. SUM(DECIMAL) is text at the driver boundary.
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	rows, err := sqlDB.QueryContext(ctx, "SELECT SUM(amount) AS revenue, COUNT(*) AS record_count, MIN(code) AS code FROM `"+f.table+"`")
	require.NoError(t, err)
	defer rows.Close()
	names, err := rows.Columns()
	require.NoError(t, err)
	types, err := rows.ColumnTypes()
	require.NoError(t, err)
	columnTypes := map[string]string{}
	for i, column := range types {
		columnTypes[names[i]] = column.DatabaseTypeName()
	}
	values := make([]any, len(names))
	dest := make([]any, len(names))
	for i := range values {
		dest[i] = &values[i]
	}
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(dest...))
	row := map[string]any{}
	for i, value := range values {
		if b, ok := value.([]byte); ok {
			value = string(b)
		}
		row[names[i]] = value
	}
	require.NoError(t, rows.Close())
	require.Equal(t, "300.246913578024691356", row["revenue"])
	sessionID := uuid.NewString()
	require.NoError(t, f.db.Create(&mysql.SessionModel{ID: sessionID, TenantID: f.owner.TenantID, UserID: f.owner.UserID, Title: "numeric precision source", Status: 1}).Error)
	id, err := f.results.Put(ctx, &resultstore.Result{Owner: resultstore.OwnerKey(f.owner.TenantID, sessionID), Columns: names, ColumnTypes: columnTypes, Rows: []map[string]any{row}}, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.redis.Del(context.Background(), "agent:resultstore:"+id).Err()) })
	result := f.acquire(t, lifecycle.SourceRequest{Kind: "result", SessionID: sessionID, ResultID: id}, id)
	require.Equal(t, columnTypes, result.ColumnTypes)
	require.Equal(t, "300.246913578024691356", result.Rows[0][0])
	require.Equal(t, json.Number("2"), result.Rows[0][1])
	require.Equal(t, "00123", result.Rows[0][2])
	stats, err = lifecycle.AnalyzeInput(result)
	require.NoError(t, err)
	columns = stats["columns"].([]map[string]any)
	require.Equal(t, 1, columns[0]["numeric_count"])
	require.InDelta(t, 300.246913578024691356, columns[0]["mean"], 1e-10)
	require.Equal(t, 1, columns[1]["numeric_count"])
	require.Equal(t, 0, columns[2]["numeric_count"])
}
