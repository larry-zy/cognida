//go:build integration

package wire

import (
	"bytes"
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

	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/datasource"
	"cognida/internal/service/queryjob"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type sourceBQTransport struct {
	destination *url.URL
	transport   http.RoundTripper
}

func (t sourceBQTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "oauth2.googleapis.com" && r.URL.Host != "bigquery.googleapis.com" {
		return nil, fmt.Errorf("fixture forbids external network")
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.destination.Scheme
	u.Host = t.destination.Host
	copy.URL = &u
	copy.Host = t.destination.Host
	return t.transport.RoundTrip(copy)
}

// Actual production lifecycle HTTP/access + datasource + durable MySQL job.
// Only Google OAuth/REST is local synthetic protocol data, not a paid cloud test.
func TestBigQueryDatasourceSourceHTTPUsesDurableREST(t *testing.T) {
	f := newSourceHTTPFixture(t)
	var mu sync.Mutex
	remote := map[string]map[string]any{}
	submits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"access_token":"synthetic-only","token_type":"Bearer","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, "/datasets/dataset/tables"):
			_, _ = w.Write([]byte(`{"tables":[{"tableReference":{"tableId":"samples"},"type":"TABLE"}]}`))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/jobs"):
			var v map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&v))
			config := v["configuration"].(map[string]any)
			query := config["query"].(map[string]any)
			require.Equal(t, "SELECT * FROM (SELECT * FROM `samples`) AS cognida_bounded LIMIT 1001", query["query"])
			if config["dryRun"] == true {
				v["statistics"] = map[string]any{"query": map[string]any{"totalBytesProcessed": "100", "statementType": "SELECT"}}
			} else {
				submits++
				v["status"] = map[string]any{"state": "DONE"}
				remote[v["jobReference"].(map[string]any)["jobId"].(string)] = v
			}
			_ = json.NewEncoder(w).Encode(v)
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "/queries/"):
			_, _ = w.Write([]byte(`{"jobComplete":true,"schema":{"fields":[{"name":"code","type":"STRING"},{"name":"amount","type":"BIGNUMERIC"}]},"rows":[{"f":[{"v":"00123"},{"v":"9007199254740993.123456789012345678"}]}]}`))
		default:
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if value, ok := remote[id]; ok {
				_ = json.NewEncoder(w).Encode(value)
			} else {
				w.WriteHeader(404)
			}
		}
	}))
	defer server.Close()
	destination, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := &http.Client{Transport: sourceBQTransport{destination, server.Client().Transport}, Timeout: 5 * time.Second}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	credential, _ := json.Marshal(map[string]any{"type": "service_account", "project_id": "fixture-project", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "client_email": "fixture@fixture-project.iam.gserviceaccount.com", "token_uri": "https://oauth2.googleapis.com/token"})
	path := filepath.Join(t.TempDir(), "synthetic-account.json")
	require.NoError(t, os.WriteFile(path, credential, 0600))
	refs, _ := json.Marshal(map[string]datasource.BigQueryCredential{"fixture": {File: path, TenantID: f.owner.TenantID, Project: "fixture-project", Dataset: "dataset", Location: "US", MaxScanBytes: 1000}})
	t.Setenv("COGNIDA_BIGQUERY_CREDENTIALS", string(refs))
	sources := mysql.NewDataSourceRepository(f.db)
	id := uuid.NewString()
	require.NoError(t, sources.Create(ctx, &ds.DataSource{ID: id, TenantID: f.owner.TenantID, Name: id, Type: ds.TypeBigQuery, DatabaseName: "dataset", Host: "bigquery.googleapis.com", Port: 443, Username: "fixture", Extra: []byte(`{"credential_ref":"fixture","project":"fixture-project","location":"US","max_scan_bytes":1000}`), Status: "active", CreatedBy: f.owner.UserID, CreatedAt: time.Now(), UpdatedAt: time.Now()}))
	cipher, err := datasource.NewCipher("source-fixture")
	require.NoError(t, err)
	jobs, err := queryjob.New(mysql.NewQueryJobRepository(f.db), f.sources, func(ctx context.Context, p l.Principal, id string) (*ds.DataSource, error) {
		if p != f.owner {
			return nil, l.ErrForbidden
		}
		return sources.Get(ctx, id, p.TenantID)
	}, cipher)
	require.NoError(t, err)
	f.sources.WithDurableQueries(jobs.Execute)
	t.Cleanup(func() {
		require.NoError(t, f.db.Table("query_jobs").Where("tenant_id=?", f.owner.TenantID).Delete(map[string]any{}).Error)
	})
	body, _ := json.Marshal(map[string]any{"name": "BQ source", "source": map[string]any{"kind": "datasource", "datasource_id": id, "table": "samples"}})
	key := uuid.NewString()
	var asset l.DataAssetRef
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/lifecycle/assets", bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var out struct {
			Data l.DataAssetRef `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		if i == 1 {
			require.Equal(t, asset.AssetRefID, out.Data.AssetRefID)
		}
		asset = out.Data
	}
	mu.Lock()
	require.Equal(t, 1, submits)
	mu.Unlock()
	w := f.request(t, http.MethodGet, "/lifecycle/assets/"+asset.AssetID, nil, "", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `9007199254740993.123456789012345678`)
	require.Contains(t, w.Body.String(), `00123`)
	require.Contains(t, w.Body.String(), `BIGNUMERIC`)
}
