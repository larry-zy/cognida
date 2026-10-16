//go:build integration

package datasource

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	model "cognida/internal/model/datasource"
	"github.com/stretchr/testify/require"
)

func TestClickHouseRealAdapter(t *testing.T) {
	dsn := os.Getenv("COGNIDA_TEST_CLICKHOUSE_DSN")
	require.NotEmpty(t, dsn, "set isolated ClickHouse test DSN")
	db, err := sql.Open("clickhouse", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx))
	drv, _ := DriverFor(model.TypeClickHouse)
	tables, err := drv.ListTables(ctx, db, "cognida_query_test")
	require.NoError(t, err)
	require.NotEmpty(t, tables)
	schema, err := drv.DescribeTable(ctx, db, "cognida_query_test", "samples")
	require.NoError(t, err)
	require.Len(t, schema.Columns, 4)
	a := &sqlQueryAdapter{db: db, source: &model.DataSource{Type: model.TypeClickHouse, UpdatedAt: time.Now()}}
	result, err := a.Query(ctx, model.QueryRequest{SQL: "SELECT amount, code, big, missing FROM samples ORDER BY code", MaxRows: 1, MaxBytes: 4096, Timeout: time.Second})
	require.NoError(t, err)
	require.True(t, result.Truncated)
	require.Equal(t, "9007199254740993.123456789", result.Rows[0]["amount"])
	require.Equal(t, "00123", result.Rows[0]["code"])
	require.Equal(t, "18446744073709551615", result.Rows[0]["big"])
	require.Nil(t, result.Rows[0]["missing"])
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = a.Query(ctx, model.QueryRequest{SQL: "SELECT count() FROM numbers(1000000000)", MaxRows: 1, MaxBytes: 4096, Timeout: time.Second})
	require.Error(t, err)
	_, err = db.Exec("INSERT INTO samples VALUES (1,'bad',1,NULL)")
	require.Error(t, err, "server readonly session must reject writes")
}

func TestMySQLProtocolWarehouseRealAdapters(t *testing.T) {
	for _, engine := range []model.Type{model.TypeDoris, model.TypeStarRocks} {
		t.Run(string(engine), func(t *testing.T) {
			key := "COGNIDA_TEST_DORIS_DSN"
			if engine == model.TypeStarRocks {
				key = "COGNIDA_TEST_STARROCKS_DSN"
			}
			dsn := os.Getenv(key)
			if dsn == "" {
				t.Skip("explicit isolated engine DSN required")
			}
			db, e := sql.Open("mysql", dsn)
			require.NoError(t, e)
			defer db.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			require.NoError(t, db.PingContext(ctx))
			drv, e := DriverFor(engine)
			require.NoError(t, e)
			tables, e := drv.ListTables(ctx, db, "cognida_query_test")
			require.NoError(t, e)
			require.NotEmpty(t, tables)
			schema, e := drv.DescribeTable(ctx, db, "cognida_query_test", "samples")
			require.NoError(t, e)
			require.Len(t, schema.Columns, 4)
			a := &sqlQueryAdapter{db: db, source: &model.DataSource{Type: engine, UpdatedAt: time.Now()}}
			result, e := a.Query(ctx, model.QueryRequest{SQL: "SELECT code,amount,big,missing FROM samples ORDER BY code", MaxRows: 1, MaxBytes: 4096, Timeout: 5 * time.Second})
			require.NoError(t, e)
			require.True(t, result.Truncated)
			require.Equal(t, "00123", result.Rows[0]["code"])
			require.Equal(t, "9007199254740993.123456789", result.Rows[0]["amount"])
			require.Equal(t, "9223372036854775807", result.Rows[0]["big"])
			require.Nil(t, result.Rows[0]["missing"])
			_, e = db.ExecContext(ctx, "INSERT INTO samples VALUES ('bad',1,1,NULL)")
			require.Error(t, e, "external warehouse account must enforce SELECT-only rights")
			cancelled, stop := context.WithCancel(ctx)
			stop()
			_, e = a.Query(cancelled, model.QueryRequest{SQL: "SELECT * FROM samples", MaxRows: 1, MaxBytes: 4096, Timeout: time.Second})
			require.Error(t, e)
		})
	}
}

func TestHiveRealAdapter(t *testing.T) {
	if os.Getenv("COGNIDA_TEST_HIVE") != "1" {
		t.Skip("explicit isolated Hive fixture required")
	}
	drv, _ := DriverFor(model.TypeHive)
	dsn, e := drv.BuildDSN(&model.DataSource{Host: "127.0.0.1", Port: 62308, Username: "hive", DatabaseName: "cognida_query_test", Extra: []byte(`{"auth":"NONE","tls":false}`)}, "")
	require.NoError(t, e)
	db, e := sql.Open(drv.DriverName(), dsn)
	require.NoError(t, e)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	tables, e := drv.ListTables(ctx, db, "cognida_query_test")
	require.NoError(t, e)
	require.NotEmpty(t, tables)
	schema, e := drv.DescribeTable(ctx, db, "cognida_query_test", "samples")
	require.NoError(t, e)
	require.Len(t, schema.Columns, 4)
	a := &sqlQueryAdapter{db: db, source: &model.DataSource{Type: model.TypeHive, UpdatedAt: time.Now()}}
	result, e := a.Query(ctx, model.QueryRequest{SQL: "SELECT code,amount,big,missing FROM samples", MaxRows: 1, MaxBytes: 4096, Timeout: 15 * time.Second})
	require.NoError(t, e)
	require.True(t, result.Truncated)
	require.Equal(t, "00123", result.Rows[0]["code"])
	require.Equal(t, "9007199254740993.123456789", result.Rows[0]["amount"])
	require.Equal(t, "9223372036854775807", result.Rows[0]["big"])
	require.Nil(t, result.Rows[0]["missing"])
	_, e = db.QueryContext(ctx, "INSERT INTO samples VALUES ('bad',1,1,NULL)")
	require.Error(t, e, "Cognida Hive connection forbids writes")
	short, stop := context.WithTimeout(ctx, time.Millisecond)
	defer stop()
	_, e = a.Query(short, model.QueryRequest{SQL: "SELECT count(*) FROM samples a CROSS JOIN samples b CROSS JOIN samples c", MaxRows: 1, MaxBytes: 4096, Timeout: time.Second})
	require.Error(t, e)
	require.NoError(t, db.PingContext(ctx), "connection remains usable after a cancelled operation")
}
