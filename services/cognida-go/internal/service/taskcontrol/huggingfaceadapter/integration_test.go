//go:build integration

package huggingfaceadapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	repository "cognida/internal/repository/mysql"
	"cognida/internal/service/collection/fetcher"
	hf "cognida/internal/service/huggingface"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type noQueue struct{}

func (noQueue) Enqueue(context.Context, job.Job) error                 { return d.ErrUnavailable }
func (noQueue) Claim(context.Context, time.Duration) (*job.Job, error) { return nil, d.ErrUnavailable }
func (noQueue) Ack(context.Context, job.Job) error                     { return d.ErrUnavailable }
func (noQueue) Retry(context.Context, job.Job, time.Duration) error    { return d.ErrUnavailable }
func (noQueue) PromoteDue(context.Context, int) (int, error)           { return 0, d.ErrUnavailable }
func (noQueue) Recover(context.Context) (int, error)                   { return 0, d.ErrUnavailable }

// The upstream is a real local HTTP fixture, not a public Hub acceptance. The
// production acquire/decoder/asset ledger and MySQL transactions are exercised.
type localHub struct {
	base   string
	client *http.Client
}

func (h localHub) FetchHuggingFace(ctx context.Context, endpoint string, _ fetcher.JSONCredential, limit int64) (*fetcher.Result, error) {
	path := strings.TrimPrefix(endpoint, "https://huggingface.co")
	req, e := http.NewRequestWithContext(ctx, "GET", h.base+path, nil)
	if e != nil {
		return nil, e
	}
	response, e := h.client.Do(req)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(raw)) > limit {
		return nil, d.ErrLimit
	}
	return &fetcher.Result{Body: raw, Header: response.Header}, nil
}

type gateAssets struct {
	*lifecycle.Service
	hold    atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (a *gateAssets) ImportDerived(ctx context.Context, p l.Principal, kind l.Kind, producer string, parents []l.RefID, payload json.RawMessage, policy l.PolicySnapshot) (l.DataAssetRef, error) {
	ref, e := a.Service.ImportDerived(ctx, p, kind, producer, parents, payload, policy)
	if e == nil && a.hold.CompareAndSwap(true, false) {
		close(a.entered)
		select {
		case <-a.release:
		case <-ctx.Done():
		}
	}
	return ref, e
}

type fixture struct {
	db        *gorm.DB
	p         l.Principal
	repo      *repository.HuggingFaceRepository
	service   *hf.Service
	adapter   *Adapter
	assets    *gateAssets
	denied    atomic.Bool
	downloads atomic.Int64
	block     atomic.Bool
	entered   chan struct{}
	released  chan struct{}
	server    *httptest.Server
	restart   func() *hf.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated migrated MySQL required")
	}
	cfg, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	f := &fixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 9123}, entered: make(chan struct{}), released: make(chan struct{})}
	t.Cleanup(func() {
		for _, table := range []string{"huggingface_imports", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		_ = sql.Close()
	})
	auth := func(ctx context.Context, p l.Principal, _ string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if f.denied.Load() {
			return l.ErrForbidden
		}
		return nil
	}
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	assets, e := lifecycle.NewService(repository.NewLifecycleRepository(db), store, noQueue{}, lifecycle.Config{Authorize: auth, ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		return f.service.ValidateGovernedOrigin(ctx, p, ref)
	}})
	require.NoError(t, e)
	f.assets = &gateAssets{Service: assets, entered: make(chan struct{}), release: make(chan struct{})}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/revision/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "org/data", "sha": strings.Repeat("a", 40), "cardData": map[string]any{"configs": []any{map[string]any{"config_name": "default", "data_files": []any{map[string]any{"split": "train", "path": "train.jsonl"}}}}}, "siblings": []any{map[string]any{"rfilename": "README.md"}, map[string]any{"rfilename": "train.jsonl"}}})
		case strings.HasSuffix(r.URL.Path, "README.md"):
			_, _ = w.Write([]byte("Untrusted fixture card"))
		default:
			f.downloads.Add(1)
			if f.block.CompareAndSwap(true, false) {
				close(f.entered)
				select {
				case <-f.released:
				case <-r.Context().Done():
					return
				}
			}
			_, _ = w.Write([]byte("{\"text\":\"Restart the service and verify the log\",\"label\":\"gold not imported\",\"amount\":9007199254740993.123456789012345678}\n"))
		}
	}))
	t.Cleanup(hub.Close)
	pythonDir, e := filepath.Abs("../../../../../cognida-python")
	require.NoError(t, e)
	require.FileExists(t, filepath.Join(pythonDir, ".venv/bin/python"))
	decoderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer synthetic-internal-hf", r.Header.Get("Authorization"))
		cmd := exec.CommandContext(r.Context(), filepath.Join(pythonDir, ".venv/bin/python"), "-c", "import sys,json; from services.huggingface.decode import DecodeRequest,decode; print(json.dumps(decode(DecodeRequest.model_validate_json(sys.stdin.buffer.read())),ensure_ascii=False))")
		cmd.Dir = pythonDir
		cmd.Stdin = r.Body
		raw, err := cmd.Output()
		if err != nil {
			w.WriteHeader(422)
			return
		}
		_, _ = w.Write(raw)
	}))
	t.Cleanup(decoderServer.Close)
	decoder, e := hf.NewHTTPDecoder(decoderServer.URL, "synthetic-internal-hf")
	require.NoError(t, e)
	f.repo = repository.NewHuggingFaceRepository(db)
	f.service, e = hf.New(f.repo, f.assets, nil, localHub{hub.URL, hub.Client()}, decoder, auth)
	require.NoError(t, e)
	f.restart = func() *hf.Service {
		service, err := hf.New(repository.NewHuggingFaceRepository(db), f.assets, nil, localHub{hub.URL, hub.Client()}, decoder, auth)
		require.NoError(t, err)
		return service
	}
	f.adapter, e = New(f.service)
	require.NoError(t, e)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", f.p.TenantID)
		user := f.p.UserID
		if c.GetHeader("Other-User") == "true" {
			user++
		}
		c.Set("user_id", user)
	})
	handler.NewHuggingFaceHandler(f.service).Register(router.Group("/api/v1"))
	f.server = httptest.NewServer(router)
	t.Cleanup(f.server.Close)
	return f
}
func (f *fixture) command() tc.ExecutionCommand {
	c := testCommand()
	c.Scope = m.Scope{TenantID: f.p.TenantID, UserID: f.p.UserID}
	c.TaskID = uuid.NewString()
	seal(&c)
	return c
}
func (f *fixture) readHTTP(t *testing.T, id string, other bool) int {
	t.Helper()
	req, e := http.NewRequest("GET", f.server.URL+"/api/v1/huggingface/imports/"+id, nil)
	require.NoError(t, e)
	if other {
		req.Header.Set("Other-User", "true")
	}
	response, e := f.server.Client().Do(req)
	require.NoError(t, e)
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func TestHFGovernedMySQLAdapterHTTP(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	t.Run("atomic submit exact receipt actual artifact and independent read", func(t *testing.T) {
		c := f.command()
		const n = 6
		handles := make(chan *tc.ExecutionHandle, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); h, e := f.adapter.Submit(ctx, c); require.NoError(t, e); handles <- h }()
		}
		wg.Wait()
		close(handles)
		var h *tc.ExecutionHandle
		for got := range handles {
			if h == nil {
				h = got
			}
			require.Equal(t, h.DomainID, got.DomainID)
		}
		require.Zero(t, f.downloads.Load())
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainQueued, obs.Status)
		bad := *h
		bad.DomainID = uuid.NewString()
		_, e = f.adapter.Cancel(ctx, c, bad)
		require.ErrorIs(t, e, m.ErrConflict)
		require.NoError(t, f.service.ProcessGoverned(ctx, f.p, h.DomainID))
		obs, e = f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainSucceeded, obs.Status)
		require.Len(t, obs.OutputRefs, 1)
		ref, payload, e := f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, obs.OutputRefs[0])
		require.NoError(t, e)
		require.Equal(t, l.RawAsset, ref.Kind)
		require.NotContains(t, string(payload), "gold not imported")
		require.NoError(t, f.adapter.ValidateOutputs(ctx, c.Scope, obs.OutputRefs))
		require.ErrorIs(t, f.adapter.ValidateInputs(ctx, c.Scope, obs.OutputRefs), m.ErrInvalid, "output authorization must not admit dispatch inputs")
		outputRefs := append([]l.RefID(nil), obs.OutputRefs...)
		for _, other := range []m.Scope{{TenantID: c.Scope.TenantID, UserID: c.Scope.UserID + 1}, {TenantID: c.Scope.TenantID + 1, UserID: c.Scope.UserID}} {
			require.Error(t, f.adapter.ValidateOutputs(ctx, other, outputRefs))
		}
		require.Equal(t, 200, f.readHTTP(t, h.DomainID, false))
		require.Equal(t, 404, f.readHTTP(t, h.DomainID, true))
		before := f.downloads.Load()
		require.NoError(t, f.restart().ProcessGoverned(ctx, f.p, h.DomainID))
		restored, e := New(f.restart())
		require.NoError(t, e)
		require.NoError(t, restored.ValidateOutputs(ctx, c.Scope, outputRefs))
		again, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		require.Equal(t, h.DomainID, again.DomainID)
		require.Equal(t, before, f.downloads.Load())
		changed := c
		changed.Node.Parameters = &tc.NodeParameters{HuggingFace: &tc.HFImportParameters{Request: c.Node.Parameters.HuggingFace.Request}}
		changed.Node.Parameters.HuggingFace.Request.MaxRows = 4
		seal(&changed)
		_, e = f.adapter.Submit(ctx, changed)
		require.ErrorIs(t, e, m.ErrConflict)
		f.denied.Store(true)
		require.ErrorIs(t, f.adapter.ValidateOutputs(ctx, c.Scope, outputRefs), m.ErrForbidden)
		_, e = f.adapter.Inspect(ctx, c, *h)
		require.ErrorIs(t, e, m.ErrForbidden)
		obs, e = f.adapter.Cancel(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainSucceeded, obs.Status)
		require.Empty(t, obs.OutputRefs)
		require.Equal(t, 403, f.readHTTP(t, h.DomainID, false))
		f.denied.Store(false)
		require.NoError(t, f.adapter.ValidateOutputs(ctx, c.Scope, outputRefs))
		require.Equal(t, before, f.downloads.Load(), "output reads, restart and authorization changes must not download again")
		obs, e = f.adapter.Cancel(ctx, c, *h)
		require.NoError(t, e)
		require.Len(t, obs.OutputRefs, 1)
	})
	t.Run("cancel first tombstone and racing late submit never downloads", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.CancelSubmission(ctx, c)
		require.NoError(t, e)
		late, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		require.Equal(t, h.DomainID, late.DomainID)
		before := f.downloads.Load()
		require.NoError(t, f.service.ProcessGoverned(ctx, f.p, h.DomainID))
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainCancelled, obs.Status)
		require.Equal(t, before, f.downloads.Load())
		for i := 0; i < 8; i++ {
			c := f.command()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); _, err := f.adapter.Submit(ctx, c); require.NoError(t, err) }()
			go func() { defer wg.Done(); _, err := f.adapter.CancelSubmission(ctx, c); require.NoError(t, err) }()
			wg.Wait()
			h, e := f.adapter.Lookup(ctx, c)
			require.NoError(t, e)
			require.NoError(t, f.service.ProcessGoverned(ctx, f.p, h.DomainID))
			obs, e := f.adapter.Inspect(ctx, c, *h)
			require.NoError(t, e)
			require.Equal(t, m.DomainCancelled, obs.Status)
		}
		require.Equal(t, before, f.downloads.Load())
	})
	t.Run("queued inspect revocation but cancellation still possible", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		f.denied.Store(true)
		_, e = f.adapter.Inspect(ctx, c, *h)
		require.ErrorIs(t, e, m.ErrForbidden)
		lookup, e := f.adapter.Lookup(ctx, c)
		require.NoError(t, e)
		require.Equal(t, h.DomainID, lookup.DomainID)
		obs, e := f.adapter.Cancel(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainCancelled, obs.Status)
		f.denied.Store(false)
	})
	t.Run("claim crash deadline and stale completion cannot publish", func(t *testing.T) {
		c := f.command()
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		receipt, claimed, e := f.repo.ClaimGoverned(ctx, f.p, h.DomainID)
		require.NoError(t, e)
		require.True(t, claimed)
		before := f.downloads.Load()
		require.NoError(t, f.restart().ProcessGoverned(ctx, f.p, h.DomainID))
		require.Equal(t, before, f.downloads.Load())
		require.NoError(t, f.db.Table("huggingface_imports").Where("id=?", h.DomainID).Update("deadline_us", time.Now().Add(-time.Second).UnixMicro()).Error)
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainTimedOut, obs.Status)
		done, e := f.repo.FinishGoverned(ctx, f.p, h.DomainID, receipt.ClaimToken, &d.Manifest{RequestHash: receipt.RequestHash}, []l.RefID{{AssetID: uuid.NewString(), Version: 1}}, "")
		require.NoError(t, e)
		require.Equal(t, "timed_out", done.Status)
		require.Empty(t, done.OutputRefs)
		_, e = f.repo.Finish(ctx, f.p, h.DomainID, nil, nil, "override")
		require.ErrorIs(t, e, d.ErrConflict)
	})
	t.Run("database node deadline bounds parent and expires queued work", func(t *testing.T) {
		c := f.command()
		c.Node.TimeoutSeconds = 1
		seal(&c)
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		receipt, e := f.repo.Get(ctx, f.p, h.DomainID)
		require.NoError(t, e)
		require.Equal(t, time.Second, receipt.Deadline.Sub(receipt.CreatedAt))
		require.True(t, receipt.Deadline.Before(c.Deadline))
		before := f.downloads.Load()
		require.Eventually(t, func() bool {
			observed, err := f.adapter.Inspect(ctx, c, *h)
			return err == nil && observed.Status == m.DomainTimedOut
		}, 3*time.Second, 50*time.Millisecond)
		require.NoError(t, f.restart().ProcessGoverned(ctx, f.p, h.DomainID))
		require.Equal(t, before, f.downloads.Load())
	})
}

func TestHFGovernedCancellationDuringRealHTTPAndCommitFence(t *testing.T) {
	t.Run("running download cancelled without output", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		c := f.command()
		f.block.Store(true)
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		finished := make(chan error, 1)
		go func() { finished <- f.service.ProcessGoverned(ctx, f.p, h.DomainID) }()
		select {
		case <-f.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("download did not begin")
		}
		obs, e := f.adapter.Cancel(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainRunning, obs.Status)
		select {
		case e = <-finished:
			require.NoError(t, e)
		case <-time.After(5 * time.Second):
			t.Fatal("HTTP context not cancelled")
		}
		obs, e = f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainCancelled, obs.Status)
		require.Empty(t, obs.OutputRefs)
		require.EqualValues(t, 1, f.downloads.Load())
		require.NoError(t, f.service.ProcessGoverned(ctx, f.p, h.DomainID))
		require.EqualValues(t, 1, f.downloads.Load())
	})
	t.Run("cancel after object registration fences receipt and origin", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()
		c := f.command()
		f.assets.hold.Store(true)
		h, e := f.adapter.Submit(ctx, c)
		require.NoError(t, e)
		finished := make(chan error, 1)
		go func() { finished <- f.service.ProcessGoverned(ctx, f.p, h.DomainID) }()
		select {
		case <-f.assets.entered:
		case <-time.After(10 * time.Second):
			t.Fatal("asset registration did not begin")
		}
		_, e = f.adapter.Cancel(ctx, c, *h)
		require.NoError(t, e)
		close(f.assets.release)
		select {
		case e = <-finished:
			require.NoError(t, e)
		case <-time.After(5 * time.Second):
			t.Fatal("finish blocked")
		}
		obs, e := f.adapter.Inspect(ctx, c, *h)
		require.NoError(t, e)
		require.Equal(t, m.DomainCancelled, obs.Status)
		require.Empty(t, obs.OutputRefs)
		var refs []struct {
			AssetID string
			Version int
		}
		require.NoError(t, f.db.Table("lifecycle_assets").Select("asset_id,version").Where("tenant_id=?", f.p.TenantID).Find(&refs).Error)
		require.Len(t, refs, 1)
		_, _, e = f.assets.Resolve(ctx, f.p.TenantID, f.p.UserID, l.RefID{AssetID: refs[0].AssetID, Version: refs[0].Version})
		require.Error(t, e)
	})
}
