//go:build integration

package datasource

import (
	m "cognida/internal/model/datasource"
	"cognida/internal/service/datasource/hivewire"
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestHivePreparedRecoveryPagesAndUnknownSubmissionCleanup(t *testing.T) {
	if os.Getenv("COGNIDA_TEST_HIVE") != "1" {
		t.Skip("explicit isolated Hive required")
	}
	cipher, e := NewCipher("isolated-hive-fixture-key")
	require.NoError(t, e)
	a := &hiveAsyncAdapter{sqlQueryAdapter: &sqlQueryAdapter{source: &m.DataSource{ID: "hive-async-test", TenantID: 7, Type: m.TypeHive, UpdatedAt: time.Now()}}, config: hivewire.Config{Host: "127.0.0.1", Port: 62308, Username: "hive", Database: "cognida_query_test", Auth: "NONE"}, cipher: cipher}
	req := m.QueryRequest{SQL: "SELECT code,amount,big,missing FROM samples ORDER BY code", MaxRows: 5, MaxBytes: 4096, Timeout: 30 * time.Second, OperationID: "hive-durable-one"}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	prepared, e := a.Prepare(ctx, req)
	require.NoError(t, e)
	require.NotContains(t, prepared, "session")
	defer a.AbandonPrepared(context.Background(), prepared)
	other := req
	other.SQL = "SELECT 1"
	_, e = a.SubmitPrepared(ctx, prepared, other)
	require.Error(t, e)
	id, e := a.SubmitPrepared(ctx, prepared, req)
	require.NoError(t, e)
	// A new adapter has no local session cache or cursor; all state is restored
	// from the encrypted handle and the server's actual retained operation.
	restored := *a
	require.Eventually(t, func() bool { state, e := restored.Status(ctx, id); require.NoError(t, e); return state == "succeeded" }, 20*time.Second, 100*time.Millisecond)
	first, next, e := restored.Page(ctx, id, "", 1)
	require.NoError(t, e)
	require.Len(t, first.Rows, 1)
	require.Equal(t, "00123", first.Rows[0]["code"])
	require.Equal(t, "9007199254740993.123456789", first.Rows[0]["amount"])
	require.Equal(t, "9223372036854775807", first.Rows[0]["big"])
	require.Nil(t, first.Rows[0]["missing"])
	require.Equal(t, "1", next)
	second, end, e := restored.Page(ctx, id, next, 1)
	require.NoError(t, e)
	require.Len(t, second.Rows, 1)
	require.NotEqual(t, first.Rows[0]["code"], second.Rows[0]["code"])
	require.Empty(t, end)
	again, _, e := restored.Page(ctx, id, "", 1)
	require.NoError(t, e)
	require.Equal(t, first.Rows, again.Rows, "page retry must not consume the next batch")
	foreign := restored
	source := *a.source
	source.TenantID = 8
	foreign.sqlQueryAdapter = &sqlQueryAdapter{source: &source}
	_, e = foreign.Status(ctx, id)
	require.Error(t, e)
	require.NoError(t, restored.Cancel(ctx, id))
	_, e = restored.Status(ctx, id)
	require.Error(t, e, "closed operation cannot be read")
	// Simulate losing the ExecuteStatement response. Only the pre-saved session
	// survives; its cleanup terminates the actual operation without replaying it.
	req.OperationID = "hive-durable-unknown"
	prepared2, e := a.Prepare(ctx, req)
	require.NoError(t, e)
	defer a.AbandonPrepared(context.Background(), prepared2)
	lost, e := a.SubmitPrepared(ctx, prepared2, req)
	require.NoError(t, e)
	require.NoError(t, restored.AbandonPrepared(ctx, prepared2))
	_, e = restored.Status(ctx, lost)
	require.Error(t, e)
	_, e = restored.SubmitPrepared(ctx, prepared2, req)
	require.Error(t, e, "abandoned session must not execute again")
}
