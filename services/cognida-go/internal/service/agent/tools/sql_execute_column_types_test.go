package tools

import (
	"context"
	"regexp"
	"testing"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/service/agent/resultstore"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSQLExecute_PersistsDriverTypesAndExactValues(t *testing.T) {
	db, mock, _ := setupMockDB(t)
	raw, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = raw.Close() })
	rows := sqlmock.NewRowsWithColumnDefinition(
		sqlmock.NewColumn("id").OfType("BIGINT", int64(0)),
		sqlmock.NewColumn("code").OfType("VARCHAR", ""),
		sqlmock.NewColumn("amount").OfType("DECIMAL", ""),
		sqlmock.NewColumn("empty").OfType("DECIMAL", ""),
	).AddRow(int64(9007199254740993), []byte("00123"), []byte("123.456789012345678901"), nil)
	query := "SELECT id, code, amount, empty FROM typed_records"
	mock.ExpectQuery(regexp.QuoteMeta(query + " LIMIT 100")).WillReturnRows(rows)
	ctx := agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 42), "typed-results")
	store := resultstore.NewMemoryStore()
	registry := resultstore.NewSessionRegistry(resultstore.DefaultTTL)
	result, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: query}, db, nil, store, nil, registry)
	require.NoError(t, err)
	stored, err := store.Get(ctx, resultstore.OwnerKey(42, "typed-results"), result.ResultID)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"id": "BIGINT", "code": "VARCHAR", "amount": "DECIMAL", "empty": "DECIMAL"}, stored.ColumnTypes)
	require.Equal(t, int64(9007199254740993), stored.Rows[0]["id"])
	require.Equal(t, "00123", stored.Rows[0]["code"])
	require.Equal(t, "123.456789012345678901", stored.Rows[0]["amount"])
	require.Nil(t, stored.Rows[0]["empty"])
	require.Equal(t, "number", result.Dtypes["amount"])
	require.Equal(t, "string", result.Dtypes["code"])
	// A reused query must use persisted driver types, not parse its VARCHAR sample.
	cached, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: query}, db, nil, store, nil, registry)
	require.NoError(t, err)
	require.Equal(t, result.ResultID, cached.ResultID)
	require.Equal(t, result.Dtypes, cached.Dtypes)
	require.NoError(t, mock.ExpectationsWereMet())
}
