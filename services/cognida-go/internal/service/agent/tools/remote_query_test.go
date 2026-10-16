package tools

import (
	agent "cognida/internal/model/agent"
	ds "cognida/internal/model/datasource"
	"cognida/internal/model/semantic"
	"cognida/internal/service/agent/resultstore"
	src "cognida/internal/service/datasource"
	"context"
	"database/sql"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type remoteToolsFixture struct {
	legacy, direct, durable int
	keys                    []string
}

func (a *remoteToolsFixture) Acquire(context.Context, int64, string) (*sql.DB, *ds.DataSource, error) {
	a.legacy++
	return nil, nil, errors.New("SQL connection must not be used")
}
func (a *remoteToolsFixture) AcquireQuery(context.Context, int64, string) (ds.QueryAdapter, *ds.DataSource, error) {
	return a, &ds.DataSource{ID: "bq", Type: ds.TypeBigQuery}, nil
}
func (a *remoteToolsFixture) Capabilities() ds.QueryCapabilities {
	return ds.QueryCapabilities{Engine: ds.TypeBigQuery, Async: true}
}
func (a *remoteToolsFixture) Query(context.Context, ds.QueryRequest) (*ds.QueryResult, error) {
	a.direct++
	return nil, errors.New("must use durable query")
}
func (a *remoteToolsFixture) ExecuteDurableQuery(_ context.Context, tenant, user int64, id string, in ds.QueryRequest, key string) (*ds.QueryResult, error) {
	a.durable++
	a.keys = append(a.keys, key)
	return &ds.QueryResult{Columns: []string{"code"}, ColumnTypes: map[string]string{"code": "STRING"}, Rows: []map[string]any{{"code": "00123"}}, ExecutedSQL: in.SQL, SourceVersion: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
func (a *remoteToolsFixture) Get(context.Context, int64, string) (*src.Sanitized, error) {
	return &src.Sanitized{ID: "bq", Type: "bigquery", DatabaseName: "authorized_dataset", Status: "active"}, nil
}
func (a *remoteToolsFixture) ListTables(context.Context, int64, string) ([]src.TableInfo, error) {
	return []src.TableInfo{{Name: "samples"}}, nil
}
func (a *remoteToolsFixture) DescribeTable(context.Context, int64, string, string) (*src.TableSchema, error) {
	return &src.TableSchema{Name: "samples", Columns: []src.ColumnInfo{{Name: "code", Type: "STRING", Nullable: true}}}, nil
}
func TestRemoteAgentSQLDurableIdentityAndMetadata(t *testing.T) {
	p := &remoteToolsFixture{}
	ctx := agent.WithRequestID(agent.WithSessionID(agent.WithUserID(agent.WithTenantID(context.Background(), 7), 9), "session"), "request")
	in := &SQLExecuteRequest{SQL: "SELECT code FROM samples", DatabaseID: "bq", MaxRows: 10}
	for i := 0; i < 2; i++ {
		out, e := sqlExecute(ctx, in, nil, p, nil, nil, nil)
		require.NoError(t, e)
		require.Equal(t, 1, out.RowCount)
	}
	require.Equal(t, p.keys[0], p.keys[1])
	require.Equal(t, 2, p.durable)
	require.Zero(t, p.direct)
	require.Zero(t, p.legacy)
	schema, e := getSchema(ctx, &GetSchemaRequest{DatabaseID: "bq", TableName: "samples"}, nil, p, nil)
	require.NoError(t, e)
	require.Equal(t, "STRING", schema.Tables[0].Columns[0].Type)
	require.Zero(t, p.legacy)
	_, e = sqlExecute(agent.WithTenantID(context.Background(), 7), in, nil, p, nil, nil, nil)
	require.ErrorContains(t, e, "authenticated request")
	require.Equal(t, 2, p.durable)
}

func TestRemoteSemanticBigQueryUsesMetadataWithoutSQLConnection(t *testing.T) {
	p := &remoteToolsFixture{}
	b := salesBundle()
	b.LogicalTables[0].DatabaseID = "bq"
	b.Dimensions = append(b.Dimensions, &semantic.Dimension{ID: "day", LogicalTableID: "lt_order", Name: "日期", Expr: "orders.created_at", DataType: "timestamp"})
	ctx := agent.WithRequestID(agent.WithSessionID(agent.WithUserID(agent.WithTenantID(context.Background(), 7), 9), "session"), "semantic-request")
	compiled, err := semanticQuery(ctx, &SemanticQueryRequest{Model: "sales", Metrics: []string{"营收"}, TimeGrain: "week"}, &stubSemanticRepo{bundle: b}, nil, nil, p)
	require.NoError(t, err)
	require.True(t, compiled.Covered)
	require.Contains(t, compiled.SQL, "FORMAT_DATE('%G-W%V', CAST(orders.created_at AS DATE))")
	require.Equal(t, "bq", compiled.DatabaseID)
	store := resultstore.NewMemoryStore()
	executed, err := sqlExecute(ctx, &SQLExecuteRequest{SQL: compiled.SQL, DatabaseID: compiled.DatabaseID, MaxRows: 10}, nil, p, store, nil, nil)
	require.NoError(t, err)
	stored, err := store.Get(ctx, resultstore.OwnerKey(7, "session"), executed.ResultID)
	require.NoError(t, err)
	require.Equal(t, "00123", stored.Rows[0]["code"])
	require.Equal(t, "STRING", stored.ColumnTypes["code"])
	require.Equal(t, compiled.SQL, stored.SourceSQL)
	require.Zero(t, p.legacy)
	require.Zero(t, p.direct)
}
