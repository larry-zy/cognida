//go:build integration

package apisource_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/apisource"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/apisource"
	"cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	"cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type queue struct {
	job.Queue
	mu   sync.Mutex
	jobs map[string]job.Job
}

func (q *queue) Enqueue(_ context.Context, j job.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs[j.ID] = j
	return nil
}
func (q *queue) get(id string) job.Job { q.mu.Lock(); defer q.mu.Unlock(); return q.jobs[id] }

type resolver struct{ fallback bool }

func (r resolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	ips := []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}
	if r.fallback {
		ips = append(ips, net.IPAddr{IP: net.ParseIP("93.184.216.35")})
	}
	return ips, nil
}

type fixture struct {
	db               *gorm.DB
	p                l.Principal
	repo             d.Repository
	s                *svc.Service
	assets           *lifecycle.Service
	q                *queue
	router           *gin.Engine
	credential       *d.Credential
	calls            atomic.Int32
	dialCalls        atomic.Int32
	identityMismatch atomic.Bool
	stalledClosed    chan struct{}
	secondFail       atomic.Bool
	revoke           atomic.Bool
	target           string
}

// A real TLS server checks synthetic authentication, pagination and snapshot
// parameters. Only the numeric-address dial seam points at the test server;
// production DNS validation, TLS and HTTP transport run normally. This is not
// public-network evidence. MySQL, object store and HTTP are real.
func setup(t *testing.T) *fixture { return setupTLS(t, false) }
func setupTLS(t *testing.T, fallback bool) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated MySQL required")
	}
	config, e := sqlmysql.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(config.DBName, "cognida_review_"))
	require.NotContains(t, config.DBName, "e2e")
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 617}, q: &queue{jobs: map[string]job.Job{}}}
	t.Cleanup(func() {
		for _, table := range []string{"api_ingestion_runs", "api_source_versions", "api_sources", "api_source_credentials", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	secret := "synthetic-only-" + uuid.NewString()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Host != "example.com" || r.TLS.ServerName != "example.com" {
			f.identityMismatch.Store(true)
		}
		if r.Header.Get("Authorization") != "Bearer "+secret {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			io.WriteString(w, `{"items":[{"id":"00123","value":9007199254740993.123456789012345678,"updated":"2026-10-16T00:00:00Z","deleted":false}],"next":"opaque-page-two","snapshot":"fixed-snapshot"}`)
			return
		}
		if r.URL.Query().Get("cursor") != "opaque-page-two" || r.URL.Query().Get("snapshot") != "fixed-snapshot" {
			w.WriteHeader(409)
			return
		}
		if f.secondFail.Load() {
			w.WriteHeader(503)
			return
		}
		io.WriteString(w, `{"items":[{"id":"00123","value":9007199254740993.123456789012345678,"updated":"2026-10-16T00:00:00Z","deleted":false},{"id":"deleted-record","value":0,"updated":"2026-10-16T00:01:00Z","deleted":true}],"next":null,"snapshot":"fixed-snapshot"}`)
	}))
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	f.stalledClosed = make(chan struct{})
	fetch := fetcher.New(fetcher.Options{Resolver: resolver{fallback: fallback}, TLSConfig: &tls.Config{RootCAs: roots}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		f.dialCalls.Add(1)
		if fallback && address == "93.184.216.34:443" {
			client, peer := net.Pipe()
			go func() { defer close(f.stalledClosed); defer peer.Close(); _, _ = io.Copy(io.Discard, peer) }()
			return client, nil
		}
		if address != "93.184.216.34:443" && address != "93.184.216.35:443" {
			return nil, d.ErrForbidden
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}})
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	auth := func(_ context.Context, p l.Principal, _ string) error {
		if p != f.p || f.revoke.Load() {
			return l.ErrForbidden
		}
		return nil
	}
	f.assets, e = lifecycle.NewService(repo.NewLifecycleRepository(db), store, f.q, lifecycle.Config{Authorize: auth, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		return f.s.ValidateGovernedOrigin(ctx, p, ref)
	}})
	require.NoError(t, e)
	cipher, e := datasource.NewCipher("synthetic-encryption-key")
	require.NoError(t, e)
	f.repo = repo.NewAPISourceRepository(db)
	f.s, e = svc.New(f.repo, store, f.assets, f.q, cipher, fetch, auth)
	require.NoError(t, e)
	f.credential, e = f.s.CreateCredential(context.Background(), f.p, d.CredentialInput{Name: "synthetic review", Origin: "https://example.com/", Kind: "bearer", Secret: secret}, uuid.NewString())
	require.NoError(t, e)
	f.router = gin.New()
	f.router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Anonymous") != "1" {
			c.Set("tenant_id", f.p.TenantID)
			user := f.p.UserID
			if c.GetHeader("X-Test-Other") == "1" {
				user++
			}
			c.Set("user_id", user)
		}
		c.Next()
	})
	handler.NewAPISourceHandler(f.s).Register(f.router.Group("/api/v1"))
	return f
}
func (f *fixture) source(t *testing.T, snapshot bool) *d.Source {
	t.Helper()
	config := d.Config{URL: "https://example.com/records", CredentialID: f.credential.ID, ItemsPointer: "/items", RecordKey: "id", WatermarkField: "updated", WatermarkParameter: "since", DeletedField: "deleted", Pagination: d.Pagination{Mode: "cursor", Parameter: "cursor", NextPointer: "/next", SnapshotPointer: "/snapshot", SnapshotParameter: "snapshot"}, Limits: d.Limits{MaxPages: 10, MaxRecords: 100, MaxBytes: 8192, MaxPageBytes: 4096, MaxRequests: 15, TimeoutSeconds: 120}}
	if !snapshot {
		config.Pagination.SnapshotPointer = ""
		config.Pagination.SnapshotParameter = ""
	}
	source, e := f.s.PutSource(context.Background(), f.p, "", d.SourceInput{Name: "test JSON records", Config: config}, uuid.NewString())
	require.NoError(t, e)
	return source
}
func (f *fixture) http(method, path string, body any, other bool) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "synthetic-http-key")
	if other {
		req.Header.Set("X-Test-Other", "1")
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}
func TestAPIIngestionTLSMySQLHTTPResumeExactAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, true)
	f.secondFail.Store(true)
	response := f.http("POST", "/api-sources/"+source.ID+"/runs", map[string]int{"version": source.Version}, false)
	require.Equal(t, 201, response.Code, response.Body.String())
	var envelope struct {
		Data d.Run `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	run := envelope.Data
	require.NoError(t, f.s.Handle(ctx, f.q.get(run.ID)))
	partial, e := f.s.Read(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "partial", partial.Status)
	require.True(t, partial.Retryable)
	require.Len(t, partial.Pages, 1)
	require.Empty(t, partial.OutputRefs)
	head, e := f.s.Source(ctx, f.p, source.ID, 0)
	require.NoError(t, e)
	require.Empty(t, head.Watermark)
	f.secondFail.Store(false)
	resumed, e := f.s.Resume(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "queued", resumed.Status)
	require.NoError(t, f.s.Handle(ctx, f.q.get(run.ID)))
	complete, e := f.s.Read(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", complete.Status)
	require.Equal(t, 2, complete.RecordCount)
	require.Equal(t, 1, complete.DuplicateCount)
	require.Equal(t, 1, complete.DeletedCount)
	require.Len(t, complete.OutputRefs, 2)
	head, e = f.s.Source(ctx, f.p, source.ID, 0)
	require.NoError(t, e)
	require.Equal(t, "2026-10-16T00:01:00Z", head.Watermark)
	_, raw, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, complete.OutputRefs[0])
	require.NoError(t, e)
	require.Contains(t, string(raw), `"9007199254740993.123456789012345678"`)
	require.Contains(t, string(raw), `"00123"`)
	require.Contains(t, string(raw), `"value":"NUMERIC"`)
	require.NotContains(t, string(raw), "synthetic-only")
	require.NotContains(t, string(raw), "opaque-page-two")
	response = f.http("GET", "/api-ingestion-runs/"+run.ID, nil, false)
	require.Equal(t, 200, response.Code)
	require.NotContains(t, response.Body.String(), "Ciphertext")
	require.NotContains(t, response.Body.String(), "opaque-page-two")
	require.NotContains(t, response.Body.String(), "fixed-snapshot")
	response = f.http("GET", "/api-ingestion-runs/"+run.ID, nil, true)
	require.True(t, response.Code == 403 || response.Code == 404)
	calls := f.calls.Load()
	response = f.http("POST", "/api-sources/"+source.ID+"/runs", map[string]int{"version": source.Version}, false)
	require.Equal(t, 201, response.Code)
	require.NoError(t, f.s.Handle(ctx, f.q.get(run.ID)))
	require.Equal(t, calls, f.calls.Load())
	require.NoError(t, f.s.DisableCredential(ctx, f.p, f.credential.ID))
	response = f.http("GET", "/api-ingestion-runs/"+run.ID, nil, false)
	require.Equal(t, 403, response.Code)
	_, _, e = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, complete.OutputRefs[0])
	require.Error(t, e)
	cancelled, e := f.s.Cancel(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", cancelled.Status)
	require.Empty(t, cancelled.OutputRefs)
}
func TestAPIIngestionCancelFenceAndUnsafeRecovery(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	source := f.source(t, false)
	run, e := f.s.Start(ctx, f.p, source.ID, source.Version, "unsafe-recovery")
	require.NoError(t, e)
	claimed, e := f.repo.Claim(ctx, f.p, run.ID, time.Second)
	require.NoError(t, e)
	page := d.Checkpoint{ExpectedIndex: 0, Page: d.PageReceipt{Number: 1, Digest: strings.Repeat("a", 64), Bytes: 20, Records: 1, ObjectKey: "fixture/page.json", AcquiredAt: time.Now()}, Records: []json.RawMessage{[]byte(`{"id":"00123"}`)}, Keys: map[string]string{"fixture": "digest"}}
	_, e = f.repo.Checkpoint(ctx, f.p, run.ID, claimed.Epoch, page)
	require.NoError(t, e)
	// Simulate a lost worker lease in this isolated fixture; do not mutate business status.
	require.NoError(t, f.db.Exec("UPDATE api_ingestion_runs SET lease_until_us=0, body=JSON_SET(body,'$.LeaseUntil','2000-01-01T00:00:00Z') WHERE tenant_id=? AND id=?", f.p.TenantID, run.ID).Error)
	require.NoError(t, f.s.Handle(ctx, f.q.get(run.ID)))
	stopped, e := f.s.Read(ctx, f.p, run.ID)
	require.NoError(t, e)
	require.Equal(t, "partial", stopped.Status)
	require.False(t, stopped.Retryable)
	require.Zero(t, f.calls.Load())
	_, e = f.s.Resume(ctx, f.p, run.ID)
	require.ErrorIs(t, e, d.ErrConflict)
	source = f.source(t, true)
	cancelRun, e := f.s.Start(ctx, f.p, source.ID, source.Version, "cancel-fence")
	require.NoError(t, e)
	owned, e := f.repo.Claim(ctx, f.p, cancelRun.ID, time.Second*30)
	require.NoError(t, e)
	_, e = f.s.Cancel(ctx, f.p, cancelRun.ID)
	require.NoError(t, e)
	_, e = f.repo.Checkpoint(ctx, f.p, cancelRun.ID, owned.Epoch, page)
	require.ErrorIs(t, e, d.ErrConflict)
	require.NoError(t, f.s.Handle(ctx, f.q.get(cancelRun.ID)))
	require.Zero(t, f.calls.Load())
	response := f.http("POST", "/api-credentials", map[string]any{"name": "x", "origin": "https://example.com/", "kind": "bearer", "secret": "synthetic-secret-only", "extra": true}, false)
	require.Equal(t, 400, response.Code)
	req := httptest.NewRequest("GET", "/api/v1/api-sources", nil)
	req.Header.Set("X-Test-Anonymous", "1")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
}

func TestAPIIngestionCreationIdempotencyAndCurrentAuthorization(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in := d.CredentialInput{Name: "idempotent review", Origin: "https://example.com/", Kind: "bearer", Secret: "synthetic-idempotency-only"}
	a, e := f.s.CreateCredential(ctx, f.p, in, "credential-retry")
	require.NoError(t, e)
	b, e := f.s.CreateCredential(ctx, f.p, in, "credential-retry")
	require.NoError(t, e)
	require.Equal(t, a.ID, b.ID)
	require.NotContains(t, a.Ciphertext, in.Secret)
	in.Secret = "synthetic-changed-credential"
	_, e = f.s.CreateCredential(ctx, f.p, in, "credential-retry")
	require.ErrorIs(t, e, d.ErrConflict)
	source := f.source(t, true)
	definition := d.SourceInput{Name: "idempotent source", Config: source.Config}
	x, e := f.s.PutSource(ctx, f.p, "", definition, "source-retry")
	require.NoError(t, e)
	y, e := f.s.PutSource(ctx, f.p, "", definition, "source-retry")
	require.NoError(t, e)
	require.Equal(t, x.ID, y.ID)
	require.Equal(t, 1, y.Version)
	definition.Name = "changed"
	_, e = f.s.PutSource(ctx, f.p, "", definition, "source-retry")
	require.ErrorIs(t, e, d.ErrConflict)
	f.revoke.Store(true)
	_, e = f.s.CreateCredential(ctx, f.p, in, "credential-retry")
	require.ErrorIs(t, e, d.ErrForbidden)
	_, e = f.s.PutSource(ctx, f.p, "", definition, "source-retry")
	require.ErrorIs(t, e, d.ErrForbidden)
	require.Zero(t, f.calls.Load())
}

// The first approved IP accepts TCP but never completes TLS. The second
// performs a real certificate-verified TLS handshake. The two pages must each
// be sent only once and persisted with the normal request budget and receipts.
func TestAPIIngestionTLSAddressFallbackMySQLHTTP(t *testing.T) {
	f := setupTLS(t, true)
	ctx := context.Background()
	source := f.source(t, true)
	response := f.http("POST", "/api-sources/"+source.ID+"/runs", map[string]int{"version": source.Version}, false)
	require.Equal(t, 201, response.Code, response.Body.String())
	var envelope struct {
		Data d.Run `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.NoError(t, f.s.Handle(ctx, f.q.get(envelope.Data.ID)))
	response = f.http("GET", "/api-ingestion-runs/"+envelope.Data.ID, nil, false)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, "completed", envelope.Data.Status)
	require.Equal(t, 2, envelope.Data.Requests)
	require.Len(t, envelope.Data.Pages, 2)
	require.Len(t, envelope.Data.OutputRefs, 2)
	require.EqualValues(t, 2, f.calls.Load(), "no HTTP page replay during address fallback")
	require.EqualValues(t, 2, f.dialCalls.Load())
	require.False(t, f.identityMismatch.Load())
	select {
	case <-f.stalledClosed:
	case <-time.After(time.Second):
		t.Fatal("failed TLS connection not closed")
	}
	_, raw, err := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, envelope.Data.OutputRefs[0])
	require.NoError(t, err)
	require.Contains(t, string(raw), "9007199254740993.123456789012345678")
	require.NoError(t, f.s.Handle(ctx, f.q.get(envelope.Data.ID)))
	require.EqualValues(t, 2, f.calls.Load(), "completed receipt must not replay")
}
