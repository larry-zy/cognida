package datasource

import (
	m "cognida/internal/model/datasource"
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBigQuerySamplingUsesRESTMetadataAndDurableOwnedQuery(t *testing.T) {
	s, repo := newTestService(t)
	source := &m.DataSource{ID: "bq", TenantID: 7, Type: m.TypeBigQuery, Status: "active", DatabaseName: "dataset", Host: "bigquery.googleapis.com", Port: 443, Username: "managed", Extra: []byte(`{"credential_ref":"managed","project":"project","location":"US","max_scan_bytes":100}`), UpdatedAt: time.Now()}
	require.NoError(t, repo.Create(context.Background(), source))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/datasets/dataset/tables")
		w.Write([]byte(`{"tables":[{"tableReference":{"tableId":"samples"},"type":"TABLE"}]}`))
	}))
	defer server.Close()
	s.remoteFactory = func(_ context.Context, src *m.DataSource) (*bigQueryAdapter, error) {
		return &bigQueryAdapter{source: src, options: BigQueryOptions{Project: "project"}, client: server.Client(), endpoint: server.URL}, nil
	}
	calls := 0
	keys := []string{}
	s.WithDurableQueries(func(_ context.Context, tenant, user int64, id string, in m.QueryRequest, key string) (*m.QueryResult, error) {
		calls++
		require.Equal(t, int64(7), tenant)
		require.Equal(t, int64(9), user)
		require.Equal(t, "bq", id)
		require.Equal(t, "SELECT * FROM `samples`", in.SQL)
		require.Equal(t, int64(100), in.MaxScanBytes)
		keys = append(keys, key)
		return &m.QueryResult{Columns: []string{"amount", "code"}, ColumnTypes: map[string]string{"amount": "NUMERIC", "code": "STRING"}, Rows: []map[string]any{{"amount": "9007199254740993.123456789", "code": "00123"}}}, nil
	})
	ctx := m.WithQueryIdentity(context.Background(), 9, "one-parent-request")
	columns, rows, types, e := s.SampleRecordsWithTypes(ctx, 7, "bq", "samples", 10)
	require.NoError(t, e)
	require.Equal(t, []string{"amount", "code"}, columns)
	require.Equal(t, "9007199254740993.123456789", rows[0][0])
	require.Equal(t, "NUMERIC", types["amount"])
	csv, count, e := s.SampleCSV(ctx, 7, "bq", "samples", 10)
	require.NoError(t, e)
	require.Equal(t, 1, count)
	require.True(t, strings.Contains(string(csv), "9007199254740993.123456789,00123"))
	require.Equal(t, keys[0], keys[1])
	_, _, _, e = s.SampleRecordsWithTypes(context.Background(), 7, "bq", "samples", 10)
	require.Error(t, e)
	_, _, _, e = s.SampleRecordsWithTypes(ctx, 7, "bq", "missing", 10)
	require.Error(t, e)
	require.Equal(t, 2, calls)
}
