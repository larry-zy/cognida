//go:build integration

package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	objectstore "cognida/internal/infrastructure/objectstore"
	agent "cognida/internal/model/agent"
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/semantic"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	tools "cognida/internal/service/agent/tools"
	source "cognida/internal/service/datasource"
	life "cognida/internal/service/lifecycle"
	queryjob "cognida/internal/service/queryjob"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type isolatedBQTransport struct {
	destination *url.URL
	transport   http.RoundTripper
}

func (t isolatedBQTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "oauth2.googleapis.com" && r.URL.Host != "bigquery.googleapis.com" {
		return nil, fmt.Errorf("fixture forbids external host")
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.destination.Scheme
	u.Host = t.destination.Host
	copy.URL = &u
	copy.Host = t.destination.Host
	return t.transport.RoundTrip(copy)
}

// OAuth and BigQuery are a local REST protocol fixture, never a live Google
// project. The production adapter, durable MySQL jobs, semantic SQL tool,
// ResultStore, object assets and analysis invocation all execute normally.
func TestBigQuerySemanticDurableMySQLAssetReportFixture(t *testing.T) {
	f := newReviewedFixture(t)
	var mu sync.Mutex
	remoteJobs := map[string]map[string]any{}
	paid := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"access_token":"synthetic-fixture-only","token_type":"Bearer","expires_in":3600}`))
			return
		}
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/jobs") {
			var v map[string]any
			if json.NewDecoder(r.Body).Decode(&v) != nil {
				w.WriteHeader(400)
				return
			}
			c := v["configuration"].(map[string]any)
			q := c["query"].(map[string]any)
			statement := q["query"].(string)
			require.Contains(t, statement, "FORMAT_DATE('%Y-%m', CAST(sales.created_at AS DATE))")
			require.Equal(t, "1000", q["maximumBytesBilled"])
			if c["dryRun"] == true {
				v["statistics"] = map[string]any{"query": map[string]any{"totalBytesProcessed": "100", "statementType": "SELECT"}}
				json.NewEncoder(w).Encode(v)
				return
			}
			paid++
			v["status"] = map[string]any{"state": "DONE"}
			remoteJobs[v["jobReference"].(map[string]any)["jobId"].(string)] = v
			json.NewEncoder(w).Encode(v)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			w.Write([]byte(`{}`))
			return
		}
		if strings.Contains(r.URL.Path, "/queries/") {
			w.Write([]byte(`{"jobComplete":true,"schema":{"fields":[{"name":"month","type":"STRING"},{"name":"code","type":"STRING"},{"name":"revenue","type":"BIGNUMERIC"}]},"rows":[{"f":[{"v":"2026-10"},{"v":"00123"},{"v":"9007199254740993.123456789012345678"}]}]}`))
			return
		}
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if v, ok := remoteJobs[id]; ok {
			json.NewEncoder(w).Encode(v)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	destination, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := &http.Client{Transport: isolatedBQTransport{destination, server.Client().Transport}, Timeout: 5 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	ctx = agent.WithRequestID(agent.WithSessionID(agent.WithUserID(agent.WithTenantID(ctx, f.p.TenantID), f.p.UserID), "bq-"+uuid.NewString()), "bq-query-"+uuid.NewString())
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	credential := map[string]any{"type": "service_account", "project_id": "fixture-project", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token"}
	raw, err := json.Marshal(credential)
	require.NoError(t, err)
	credentialPath := filepath.Join(t.TempDir(), "synthetic-service-account.json")
	require.NoError(t, os.WriteFile(credentialPath, raw, 0600))
	refs, _ := json.Marshal(map[string]source.BigQueryCredential{"fixture": {File: credentialPath, TenantID: f.p.TenantID, Project: "fixture-project", Location: "US", Dataset: "dataset", MaxScanBytes: 1000}})
	t.Setenv("COGNIDA_BIGQUERY_CREDENTIALS", string(refs))
	cipher, err := source.NewCipher("fixture-cipher-key")
	require.NoError(t, err)
	sourceRepo := repo.NewDataSourceRepository(f.db)
	sourceID := uuid.NewString()
	record := &ds.DataSource{ID: sourceID, TenantID: f.p.TenantID, Name: sourceID, Type: ds.TypeBigQuery, Host: "bigquery.googleapis.com", Port: 443, DatabaseName: "dataset", Username: "fixture", Extra: []byte(`{"credential_ref":"fixture","project":"fixture-project","location":"US","max_scan_bytes":1000}`), Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	require.NoError(t, sourceRepo.Create(ctx, record))
	modelID := uuid.NewString()
	t.Cleanup(func() {
		for _, table := range []string{"query_jobs", "lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets", "data_sources"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		for _, table := range []string{"agent_semantic_relations", "agent_semantic_metrics", "agent_semantic_measures", "agent_semantic_dimensions", "agent_semantic_logical_tables"} {
			require.NoError(t, f.db.Table(table).Where("model_id=?", modelID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, f.db.Table("agent_semantic_models").Where("id=?", modelID).Delete(map[string]any{}).Error)
	})
	sources := source.NewService(sourceRepo, cipher, nil, nil)
	jobs, err := queryjob.New(repo.NewQueryJobRepository(f.db), sources, func(ctx context.Context, p l.Principal, id string) (*ds.DataSource, error) {
		if p != f.p {
			return nil, l.ErrForbidden
		}
		return sourceRepo.Get(ctx, id, p.TenantID)
	}, cipher)
	require.NoError(t, err)
	sources.WithDurableQueries(jobs.Execute)
	sem := repo.NewSemanticRepository(f.db)
	tableID := uuid.NewString()
	bundle := &semantic.ModelBundle{Model: &semantic.SemanticModel{ID: modelID, TenantID: f.p.TenantID, Name: "bq_fixture", Version: 1, Status: semantic.ModelStatusActive}, LogicalTables: []*semantic.LogicalTable{{ID: tableID, ModelID: modelID, Name: "sales", PhysicalTable: "samples", DatabaseID: sourceID}}, Dimensions: []*semantic.Dimension{{ID: uuid.NewString(), ModelID: modelID, LogicalTableID: tableID, Name: "month", Expr: "sales.created_at", DataType: "timestamp"}, {ID: uuid.NewString(), ModelID: modelID, LogicalTableID: tableID, Name: "code", Expr: "sales.code", DataType: "string"}}, Measures: []*semantic.Measure{{ID: uuid.NewString(), ModelID: modelID, LogicalTableID: tableID, Name: "revenue", Expr: "amount", Aggregation: semantic.AggSum}}}
	require.NoError(t, sem.UpsertBundle(ctx, bundle))
	text, err := tools.NewSemanticQueryTool(sem, nil, nil, nil, sources).InvokableRun(ctx, `{"model":"bq_fixture","metrics":["revenue"],"dimensions":["code"],"time_grain":"month"}`)
	require.NoError(t, err)
	var compiled tools.SemanticQueryResult
	require.NoError(t, json.Unmarshal([]byte(text), &compiled))
	require.True(t, compiled.Covered)
	store := resultstore.NewMemoryStore()
	sqltool := tools.NewSQLExecuteTool(nil, sources, store, nil, nil)
	args, _ := json.Marshal(tools.SQLExecuteRequest{SQL: compiled.SQL, DatabaseID: compiled.DatabaseID, MaxRows: 10})
	text, err = sqltool.InvokableRun(ctx, string(args))
	require.NoError(t, err)
	var executed tools.SQLExecuteResult
	require.NoError(t, json.Unmarshal([]byte(text), &executed))
	require.NotEmpty(t, executed.ResultID)
	original, err := store.Get(ctx, resultstore.OwnerKey(f.p.TenantID, agent.MustGetSessionID(ctx)), executed.ResultID)
	require.NoError(t, err)
	require.Equal(t, "9007199254740993.123456789012345678", original.Rows[0]["revenue"])
	// Identical parent operation reuses its paid remote job, while ResultStore may
	// create another local reference to the same immutable query result.
	_, err = sqltool.InvokableRun(ctx, string(args))
	require.NoError(t, err)
	mu.Lock()
	require.Equal(t, 1, paid)
	mu.Unlock()
	objects, err := objectstore.NewLocal(t.TempDir())
	require.NoError(t, err)
	q := &cleaningQueue{}
	access := func(ctx context.Context, p l.Principal, r life.SourceRequest) (l.Input, l.Provenance, l.PolicySnapshot, error) {
		if p != f.p || r.ResultID != executed.ResultID || r.SessionID != agent.MustGetSessionID(ctx) {
			return l.Input{}, l.Provenance{}, l.PolicySnapshot{}, l.ErrForbidden
		}
		data, err := store.Get(ctx, resultstore.OwnerKey(p.TenantID, r.SessionID), r.ResultID)
		if err != nil {
			return l.Input{}, l.Provenance{}, l.PolicySnapshot{}, err
		}
		in := l.Input{Kind: "table", Columns: data.Columns, ColumnTypes: data.ColumnTypes, Rows: [][]any{}}
		for _, row := range data.Rows {
			values := []any{}
			for _, name := range data.Columns {
				values = append(values, row[name])
			}
			in.Rows = append(in.Rows, values)
		}
		return in, l.Provenance{SourceType: "result", SourceID: r.ResultID}, l.PolicySnapshot{Scopes: []string{"read"}, Sensitivity: "unknown"}, nil
	}
	lifecycleRepo := repo.NewLifecycleRepository(f.db)
	lifecycle, err := life.NewService(lifecycleRepo, objects, q, life.Config{Access: access, Authorize: func(_ context.Context, p l.Principal, _ string) error {
		if p != f.p {
			return l.ErrForbidden
		}
		return nil
	}, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		var req life.SourceRequest
		if json.Unmarshal(ref.Provenance.AccessRequest, &req) != nil {
			return l.ErrForbidden
		}
		_, _, _, err := access(ctx, p, req)
		return err
	}})
	require.NoError(t, err)
	asset, err := lifecycle.Acquire(ctx, f.p, life.SourceRequest{Kind: "result", ResultID: executed.ResultID, SessionID: agent.MustGetSessionID(ctx)}, "BQ exact result", "bq-result")
	require.NoError(t, err)
	_, body, err := lifecycle.Resolve(ctx, f.p.TenantID, f.p.UserID, asset.RefID)
	require.NoError(t, err)
	require.Contains(t, string(body), "9007199254740993.123456789012345678")
	// Explicit trusted approval fixture; no claim that this bypasses production's
	// human publication workflow. The analysis stage itself is the real handler.
	published, err := lifecycle.ImportDerived(ctx, f.p, l.PublishedAsset, "fixture-approved", []l.RefID{asset.RefID}, body, l.PolicySnapshot{Approved: true, Scopes: []string{"read"}, Sensitivity: "unknown"})
	require.NoError(t, err)
	invocation, err := lifecycle.Invoke(ctx, f.p, "data-analysis", []l.RefID{published.RefID}, map[string]any{}, "bq-analysis", "bq-report")
	require.NoError(t, err)
	require.Len(t, q.items, 1)
	require.NoError(t, lifecycle.Handle(ctx, q.items[0]))
	done, err := lifecycleRepo.FindStage(ctx, f.p.TenantID, invocation.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, done.Status)
	require.Len(t, done.Output.Refs, 1)
	_, report, err := lifecycle.Resolve(ctx, f.p.TenantID, f.p.UserID, done.Output.Refs[0].RefID)
	require.NoError(t, err)
	require.Contains(t, string(report), `"numeric_count":1`)
	require.Contains(t, string(report), published.AssetRefID)
}
