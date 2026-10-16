package datasource

import (
	"context"
	"net/url"
	"testing"
	"time"

	model "cognida/internal/model/datasource"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestReadQueryRejectsWriteAndExternalFunctionPaths(t *testing.T) {
	for _, query := range []string{"DELETE FROM orders", "SELECT 1; SELECT 2", "SELECT * INTO OUTFILE '/tmp/x' FROM t", "WITH removed AS (DELETE FROM t RETURNING *) SELECT * FROM removed", "SELECT * FROM url('https://evil')", "SELECT 1 SETTINGS readonly=0", "SELECT pg_read_file('/etc/passwd')", "SELECT sleep(10)", "SELECT 1 /* injected */"} {
		require.Error(t, ValidateReadQuery(query), query)
	}
	for _, query := range []string{"SELECT * FROM orders WHERE id = ?", "WITH a AS (SELECT 1 AS n) SELECT * FROM a", "SELECT SUM(price) AS total FROM orders GROUP BY customer"} {
		require.NoError(t, ValidateReadQuery(query), query)
	}
}
func TestWarehouseDriversDistinctContracts(t *testing.T) {
	for _, kind := range []model.Type{model.TypeClickHouse, model.TypeDoris, model.TypeStarRocks} {
		drv, err := DriverFor(kind)
		require.NoError(t, err)
		require.Equal(t, kind, drv.Type())
	}
	drv, _ := DriverFor(model.TypeClickHouse)
	dsn, err := drv.BuildDSN(&model.DataSource{Host: "::1", Port: 9000, Username: "a", DatabaseName: "warehouse", Extra: []byte(`{"tls":true}`)}, "p@:/?#")
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	p, _ := u.User.Password()
	require.Equal(t, "p@:/?#", p)
	require.Equal(t, "1", u.Query().Get("readonly"))
	require.Equal(t, "true", u.Query().Get("secure"))
	require.Equal(t, "30", u.Query().Get("max_execution_time"))
}
func TestSQLAdapterExactTypesBoundsAndIterationFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	a := &sqlQueryAdapter{db: db, source: &model.DataSource{Type: model.TypeMySQL, UpdatedAt: time.Now()}}
	query := "SELECT amount, code, big, missing FROM items"
	mock.ExpectBegin()
	rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("amount").OfType("DECIMAL", ""), sqlmock.NewColumn("code").OfType("VARCHAR", ""), sqlmock.NewColumn("big").OfType("BIGINT", int64(0)), sqlmock.NewColumn("missing").OfType("VARCHAR", "")).AddRow("9007199254740993.123456789", "00123", int64(9007199254740993), nil).AddRow("2", "00124", int64(2), nil)
	mock.ExpectQuery("SELECT \\* FROM").WillReturnRows(rows)
	mock.ExpectRollback()
	result, err := a.Query(context.Background(), model.QueryRequest{SQL: query, MaxRows: 1, MaxBytes: 4096, Timeout: time.Second})
	require.NoError(t, err)
	require.True(t, result.Truncated)
	require.Equal(t, "9007199254740993.123456789", result.Rows[0]["amount"])
	require.Equal(t, "00123", result.Rows[0]["code"])
	require.Equal(t, "9007199254740993", result.Rows[0]["big"])
	require.Nil(t, result.Rows[0]["missing"])
	require.NoError(t, mock.ExpectationsWereMet())
	_, err = a.Query(context.Background(), model.QueryRequest{SQL: query, MaxRows: 1, MaxBytes: 4096, Timeout: time.Second, MaxScanBytes: 100})
	require.ErrorContains(t, err, "budget")
}
