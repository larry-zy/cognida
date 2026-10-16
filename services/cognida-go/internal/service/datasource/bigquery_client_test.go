package datasource

import (
	m "cognida/internal/model/datasource"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// This is a REST protocol fixture, not evidence of a live Google project.
func TestBigQueryRESTBudgetIdempotencyRecoveryAndExactPages(t *testing.T) {
	var mu sync.Mutex
	jobs := map[string]bqJob{}
	dry, paid, cancelled := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/jobs") {
			var job bqJob
			require.NoError(t, json.NewDecoder(r.Body).Decode(&job))
			if job.Configuration.DryRun {
				dry++
				job.Statistics.Query.TotalBytesProcessed = "10"
				job.Statistics.Query.StatementType = "SELECT"
				if strings.Contains(job.Configuration.Query.Query, "expensive") {
					job.Statistics.Query.TotalBytesProcessed = "100000"
				}
				_ = json.NewEncoder(w).Encode(job)
				return
			}
			paid++
			job.Status.State = "DONE"
			jobs[job.JobReference.JobID] = job
			_ = json.NewEncoder(w).Encode(job)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			cancelled++
			w.Write([]byte(`{}`))
			return
		}
		if strings.Contains(r.URL.Path, "/queries/") {
			require.Equal(t, "US", r.URL.Query().Get("location"))
			raw := `{"jobComplete":true,"schema":{"fields":[{"name":"code","type":"STRING"},{"name":"amount","type":"NUMERIC"},{"name":"big","type":"INTEGER"},{"name":"missing","type":"STRING"}]},"rows":[{"f":[{"v":"00123"},{"v":"9007199254740993.123456789"},{"v":"9223372036854775807"},{"v":null}]}],"pageToken":"second"}`
			if r.URL.Query().Get("pageToken") == "second" {
				raw = strings.ReplaceAll(strings.ReplaceAll(raw, `"pageToken":"second"`, `"pageToken":""`), `"00123"`, `"00456"`)
			}
			w.Write([]byte(raw))
			return
		}
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		job, ok := jobs[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(job)
	}))
	defer server.Close()
	a := &bigQueryAdapter{source: &m.DataSource{ID: "source", TenantID: 7, DatabaseName: "testdata", Type: m.TypeBigQuery, UpdatedAt: time.Now()}, options: BigQueryOptions{Project: "test-project", Location: "US", MaxScanBytes: 100}, client: server.Client(), endpoint: server.URL, prefix: "cognida_test_"}
	in := m.QueryRequest{SQL: "SELECT code, amount, big, missing FROM samples", MaxRows: 3, MaxBytes: 4096, Timeout: time.Second, OperationID: "durable-operation-one"}
	id, e := a.Submit(context.Background(), in)
	require.NoError(t, e)
	require.NotEmpty(t, id)
	recovered := *a
	again, e := recovered.Submit(context.Background(), in)
	require.NoError(t, e)
	require.Equal(t, id, again)
	result, e := recovered.Query(context.Background(), in)
	require.NoError(t, e)
	require.Len(t, result.Rows, 2)
	require.Equal(t, "00123", result.Rows[0]["code"])
	require.Equal(t, "00456", result.Rows[1]["code"])
	require.Equal(t, "9007199254740993.123456789", result.Rows[0]["amount"])
	require.Equal(t, "9223372036854775807", result.Rows[0]["big"])
	require.Nil(t, result.Rows[0]["missing"])
	changed := in
	changed.MaxBytes = 2048
	_, e = a.Submit(context.Background(), changed)
	require.ErrorContains(t, e, "conflict")
	expensive := in
	expensive.OperationID = "expensive-durable-op"
	expensive.SQL = "SELECT amount FROM expensive"
	_, e = a.Submit(context.Background(), expensive)
	require.ErrorContains(t, e, "scan budget")
	limited := in
	limited.OperationID = "truncated-durable-op"
	limited.MaxRows = 1
	result, e = a.Query(context.Background(), limited)
	require.NoError(t, e)
	require.Len(t, result.Rows, 1)
	require.True(t, result.Truncated)
	require.NoError(t, a.Cancel(context.Background(), id))
	_, e = a.Status(context.Background(), "other-tenant-job")
	require.ErrorContains(t, e, "scope")
	mu.Lock()
	require.Equal(t, 3, dry)
	require.Equal(t, 2, paid)
	require.Equal(t, 1, cancelled)
	mu.Unlock()
}
func TestBigQueryCredentialReferencesFailClosed(t *testing.T) {
	ds := &m.DataSource{ID: "test", TenantID: 7, Type: m.TypeBigQuery, Host: "bigquery.googleapis.com", Port: 443, Username: "private_ref", DatabaseName: "only_dataset", Extra: []byte(`{"credential_ref":"private_ref","project":"test-project","location":"US","max_scan_bytes":100}`)}
	t.Setenv("COGNIDA_BIGQUERY_CREDENTIALS", `{"private_ref":{"file":"/must-not-read","tenant_id":8,"project":"test-project","dataset":"only_dataset","location":"US","max_scan_bytes":100}}`)
	_, e := newBigQueryAdapter(context.Background(), ds)
	require.ErrorContains(t, e, "scope")
	ds.Host = "untrusted.example"
	_, e = parseBigQueryOptions(ds)
	require.Error(t, e)
	ds.Host = "bigquery.googleapis.com"
	ds.Extra = []byte(`{"credential_ref":"private_ref","project":"test-project","location":"US","max_scan_bytes":100,"token_uri":"https://untrusted.example"}`)
	_, e = parseBigQueryOptions(ds)
	require.Error(t, e)
}
