//go:build integration

package huggingface_test

import (
	"bytes"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	a "cognida/internal/model/apisource"
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/collection/fetcher"
	"cognida/internal/service/datasource"
	hf "cognida/internal/service/huggingface"
	"cognida/internal/service/lifecycle"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type resolver struct{}

func (resolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

type noQueue struct{}

func (noQueue) Enqueue(context.Context, job.Job) error                 { return d.ErrUnavailable }
func (noQueue) Claim(context.Context, time.Duration) (*job.Job, error) { return nil, d.ErrUnavailable }
func (noQueue) Ack(context.Context, job.Job) error                     { return d.ErrUnavailable }
func (noQueue) Retry(context.Context, job.Job, time.Duration) error    { return d.ErrUnavailable }
func (noQueue) PromoteDue(context.Context, int) (int, error)           { return 0, d.ErrUnavailable }
func (noQueue) Recover(context.Context) (int, error)                   { return 0, d.ErrUnavailable }

func TestHuggingFaceMySQLHTTPImportIndependentAndFenced(t *testing.T) {
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	cfg, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 8123}
	t.Cleanup(func() {
		defer sql.Close()
		for _, table := range []string{"huggingface_imports", "lifecycle_assets", "api_source_credentials"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	var deny atomic.Bool
	authorize := func(context.Context, l.Principal, string) error {
		if deny.Load() {
			return l.ErrForbidden
		}
		return nil
	}
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	var service *hf.Service
	assets, e := lifecycle.NewService(repo.NewLifecycleRepository(db), store, noQueue{}, lifecycle.Config{Authorize: authorize, ValidateGovernedOrigin: func(ctx context.Context, who l.Principal, ref l.DataAssetRef) error {
		if strings.HasPrefix(ref.ProducerID, "huggingface-import:") {
			return service.ValidateGovernedOrigin(ctx, who, ref)
		}
		return nil
	}})
	require.NoError(t, e)
	credRepo := repo.NewAPISourceRepository(db)
	cipher, e := datasource.NewCipher("hf-review-fixture-encryption-only")
	require.NoError(t, e)
	encrypted, e := cipher.Encrypt("synthetic-private-hf-token")
	require.NoError(t, e)
	credential, e := credRepo.CreateCredential(context.Background(), p, a.Credential{ID: uuid.NewString(), Name: "HF fixture", Origin: "https://huggingface.co/", Kind: "bearer", Ciphertext: encrypted, Enabled: true})
	require.NoError(t, e)
	commit := strings.Repeat("a", 40)
	raw := []byte("{\"id\":\"00123\",\"text\":\"帮我打开音乐\",\"label\":\"gold_music\",\"amount\":9007199254740993.123456789012345678}\n{\"id\":\"00124\",\"text\":\"明天下雨吗\",\"label\":\"gold_weather\",\"amount\":2}\n")
	sum := sha256.Sum256(raw)
	var downloads atomic.Int64
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-private-hf-token", r.Header.Get("Authorization"))
		switch {
		case strings.Contains(r.URL.Path, "/revision/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "org/data", "sha": commit, "cardData": map[string]any{"license": "cc-by-4.0", "configs": []any{map[string]any{"config_name": "zh-CN", "data_files": []any{map[string]any{"split": "test", "path": "test.jsonl"}}}}}, "siblings": []any{map[string]any{"rfilename": "README.md"}, map[string]any{"rfilename": "test.jsonl", "size": len(raw), "lfs": map[string]any{"sha256": hex.EncodeToString(sum[:]), "size": len(raw)}}}})
		case strings.HasSuffix(r.URL.Path, "README.md"):
			_, _ = w.Write([]byte("Public static fixture card"))
		case strings.HasSuffix(r.URL.Path, "test.jsonl"):
			downloads.Add(1)
			_, _ = w.Write(raw)
		default:
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"id": "org/data", "sha": commit}})
		}
	}))
	defer hub.Close()
	target, e := url.Parse(hub.URL)
	require.NoError(t, e)
	fetch := fetcher.New(fetcher.Options{Resolver: resolver{}, Transport: transport(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		u := *r.URL
		u.Host = target.Host
		u.Scheme = target.Scheme
		copy.URL = &u
		return http.DefaultTransport.RoundTrip(copy)
	})})
	// Real Python decoder behind a short-lived HTTP fixture. No external Hub,
	// model, dataset script, persistent server or local file path input is used.
	pythonDir, e := filepath.Abs("../../../../cognida-python")
	require.NoError(t, e)
	require.FileExists(t, filepath.Join(pythonDir, ".venv/bin/python"))
	decoderHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-internal-token", r.Header.Get("Authorization"))
		command := exec.CommandContext(r.Context(), filepath.Join(pythonDir, ".venv/bin/python"), "-c", "import sys,json; from services.huggingface.decode import DecodeRequest,decode; print(json.dumps(decode(DecodeRequest.model_validate_json(sys.stdin.buffer.read())),ensure_ascii=False))")
		command.Dir = pythonDir
		command.Stdin = r.Body
		output, err := command.Output()
		if err != nil {
			w.WriteHeader(422)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(output)
	}))
	defer decoderHTTP.Close()
	decoder, e := hf.NewHTTPDecoder(decoderHTTP.URL, "synthetic-internal-token")
	require.NoError(t, e)
	repository := repo.NewHuggingFaceRepository(db)
	service, e = hf.New(repository, assets, hf.NewManagedCredentials(credRepo, cipher), fetch, decoder, authorize)
	require.NoError(t, e)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		user := p.UserID
		if c.GetHeader("Other-User") == "1" {
			user++
		}
		c.Set("tenant_id", p.TenantID)
		c.Set("user_id", user)
		c.Next()
	})
	handler.NewHuggingFaceHandler(service).Register(router.Group("/api/v1"))
	server := httptest.NewServer(router)
	defer server.Close()
	input := d.Request{RepoID: "org/data", Revision: commit, Config: "zh-CN", Split: "test", Files: []string{"test.jsonl"}, FieldMapping: d.FieldMapping{TextField: "text", IDField: "id"}, MaxRows: 1, MaxBytes: 4096, CredentialID: credential.ID}
	call := func(method, path string, body any, key string, other bool) (int, []byte) {
		var content io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			content = bytes.NewReader(raw)
		}
		request, err := http.NewRequest(method, server.URL+path, content)
		require.NoError(t, err)
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		if other {
			request.Header.Set("Other-User", "1")
		}
		response, err := server.Client().Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, raw
	}
	status, body := call("POST", "/api/v1/huggingface/preview", input, "", false)
	require.Equal(t, 200, status, string(body))
	require.NotContains(t, string(body), "gold_music")
	require.Contains(t, string(body), `"truncated":true`)
	var assetCount int64
	require.NoError(t, db.Table("lifecycle_assets").Where("tenant_id=?", p.TenantID).Count(&assetCount).Error)
	require.Zero(t, assetCount)
	before := downloads.Load()
	const concurrent = 5
	var wg sync.WaitGroup
	ids := make(chan string, concurrent)
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := call("POST", "/api/v1/huggingface/imports", input, "one-import", false)
			require.Equal(t, 200, status, string(body))
			var out struct {
				Data d.Import `json:"data"`
			}
			require.NoError(t, json.Unmarshal(body, &out))
			ids <- out.Data.ID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for got := range ids {
		if id == "" {
			id = got
		}
		require.Equal(t, id, got)
	}
	require.Equal(t, before+1, downloads.Load())
	status, body = call("GET", "/api/v1/huggingface/imports/"+id, nil, "", false)
	require.Equal(t, 200, status, string(body))
	var receipt struct {
		Data d.Import `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &receipt))
	require.Equal(t, "succeeded", receipt.Data.Status)
	require.Len(t, receipt.Data.OutputRefs, 1)
	require.Equal(t, int64(2), *receipt.Data.Manifest.TotalRows)
	_, payload, e := assets.Resolve(context.Background(), p.TenantID, p.UserID, receipt.Data.OutputRefs[0])
	require.NoError(t, e)
	require.NotContains(t, string(payload), "gold_music")
	require.Contains(t, string(payload), "00123")
	require.Contains(t, string(payload), "huggingface_manifest")
	_, _, e = assets.Resolve(context.Background(), p.TenantID, p.UserID+1, receipt.Data.OutputRefs[0])
	require.Error(t, e)
	status, _ = call("GET", "/api/v1/huggingface/imports/"+id, nil, "", true)
	require.Equal(t, 404, status)
	changed := input
	changed.MaxRows = 2
	status, _ = call("POST", "/api/v1/huggingface/imports", changed, "one-import", false)
	require.Equal(t, 409, status)
	numeric := input
	numeric.FieldMapping = d.FieldMapping{Columns: []d.ColumnMapping{{Source: "id", Target: "id"}, {Source: "amount", Target: "amount"}}}
	status, body = call("POST", "/api/v1/huggingface/imports", numeric, "numeric-import", false)
	require.Equal(t, 200, status, string(body))
	require.NoError(t, json.Unmarshal(body, &receipt))
	_, payload, e = assets.Resolve(context.Background(), p.TenantID, p.UserID, receipt.Data.OutputRefs[0])
	require.NoError(t, e)
	require.Contains(t, string(payload), "9007199254740993.123456789012345678")
	deny.Store(true)
	before = downloads.Load()
	status, _ = call("POST", "/api/v1/huggingface/imports", input, "one-import", false)
	require.Equal(t, 403, status)
	require.Equal(t, before, downloads.Load())
	_, _, e = assets.Resolve(context.Background(), p.TenantID, p.UserID, receipt.Data.OutputRefs[0])
	require.Error(t, e)
	deny.Store(false)
	require.NoError(t, credRepo.DisableCredential(context.Background(), p, credential.ID))
	status, _ = call("GET", "/api/v1/huggingface/imports/"+id, nil, "", false)
	require.Equal(t, 403, status)
	_, _, e = assets.Resolve(context.Background(), p.TenantID, p.UserID, receipt.Data.OutputRefs[0])
	require.Error(t, e)
	// A process lost after reservation is closed by DB time. Late completion
	// cannot turn an expired run into a readable imported asset.
	pending, created, e := repository.Reserve(context.Background(), p, strings.Repeat("b", 64), strings.Repeat("c", 64), input)
	require.NoError(t, e)
	require.True(t, created)
	require.NoError(t, db.Table("huggingface_imports").Where("id=?", pending.ID).Update("deadline_us", 1).Error)
	done, e := repository.Finish(context.Background(), p, pending.ID, receipt.Data.Manifest, receipt.Data.OutputRefs, "")
	require.NoError(t, e)
	require.Equal(t, "failed", done.Status)
	require.Empty(t, done.OutputRefs)
	require.Equal(t, "deadline_exceeded", done.ErrorCode)
}
