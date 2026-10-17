//go:build integration

package handler

import (
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/queryjob"
	mysqlrepo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent/resultstore"
	source "cognida/internal/service/datasource"
	jobs "cognida/internal/service/queryjob"
	"context"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Query execution uses the explicitly synthetic remote adapter below. The job
// receipt, source configuration and guarded result persistence use real MySQL
// and Redis, including the production exact-number decoding path.
func TestQueryJobMySQLRedisGuardedResultIdentity(t *testing.T) {
	f, jobsService, _, a, _ := queryFixture(t)
	ctx := context.Background()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD")})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Ping(ctx).Err())
	sourceRepo := mysqlrepo.NewDataSourceRepository(f.db)
	a.source.ID, a.source.Name = uuid.NewString(), "guarded-result-fixture"
	a.source.PasswordEncrypted = "synthetic-credential-binding"
	require.NoError(t, sourceRepo.Create(ctx, a.source))
	t.Cleanup(func() { require.NoError(t, sourceRepo.Delete(ctx, a.source.ID, f.p.TenantID)) })
	var err error
	a.source, err = sourceRepo.Get(ctx, a.source.ID, f.p.TenantID)
	require.NoError(t, err)
	in := queryInput()
	result, err := jobsService.Execute(ctx, f.p.TenantID, f.p.UserID, a.source.ID, ds.QueryRequest{SQL: in.SQL, MaxRows: in.MaxRows, MaxBytes: in.MaxBytes, Timeout: 30 * time.Second, MaxScanBytes: in.MaxScanBytes}, "guarded-result")
	require.NoError(t, err)
	require.Equal(t, ds.QueryConnectionVersion(a.source), result.SourceVersion)
	sources := source.NewService(sourceRepo, nil, nil, nil)
	store := sources.GuardResults(resultstore.NewRedisStore(client))
	owner := resultstore.OwnerKey(f.p.TenantID, uuid.NewString())
	id, err := store.Put(ctx, &resultstore.Result{Owner: owner, DatasourceID: a.source.ID, SourceVersion: result.SourceVersion, Columns: result.Columns, ColumnTypes: result.ColumnTypes, Rows: result.Rows}, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Del(ctx, "agent:resultstore:"+id).Err()) })
	exact := store.(interface {
		GetExact(context.Context, string, string) (*resultstore.Result, error)
	})
	for _, renamed := range []bool{false, true} {
		if renamed {
			a.source.Name, a.source.Description = "renamed-result", "display only"
			require.NoError(t, sourceRepo.Update(ctx, a.source))
		}
		_, err = store.Get(ctx, owner, id)
		require.NoError(t, err)
		value, err := exact.GetExact(ctx, owner, id)
		require.NoError(t, err)
		require.Equal(t, json.Number("9223372036854775807"), value.Rows[0]["large"])
		require.Equal(t, "9007199254740993.123456789012345678", value.Rows[0]["amount"])
	}
	_, err = exact.GetExact(ctx, resultstore.OwnerKey(f.p.TenantID+1, "other"), id)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized)
	a.source.PasswordEncrypted = "rotated-synthetic-credential"
	require.NoError(t, sourceRepo.Update(ctx, a.source))
	_, err = store.Get(ctx, owner, id)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized)
	_, err = exact.GetExact(ctx, owner, id)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized)
	require.Equal(t, int64(1), a.submits.Load())
}

func TestQueryJobMySQLHTTPDisplayEditPreservesCancellation(t *testing.T) {
	for _, reason := range []string{"cancel", "deadline"} {
		t.Run(reason, func(t *testing.T) {
			f, s, repo, a, _ := queryFixture(t)
			ctx := context.Background()
			sourceRepo := mysqlrepo.NewDataSourceRepository(f.db)
			a.source.ID = uuid.NewString()
			a.source.Host = "bigquery.googleapis.com"
			a.source.Port = 443
			a.source.DatabaseName = "fixture"
			a.source.Username = "fixture-ref"
			a.source.Extra = []byte(`{"project":"fixture","location":"US","credential_ref":"fixture-ref","max_scan_bytes":1000}`)
			require.NoError(t, sourceRepo.Create(ctx, a.source))
			t.Cleanup(func() { require.NoError(t, sourceRepo.Delete(ctx, a.source.ID, f.p.TenantID)) })
			var err error
			a.source, err = sourceRepo.Get(ctx, a.source.ID, f.p.TenantID)
			require.NoError(t, err)
			in := queryInput()
			in.DataSourceID = a.source.ID
			code, raw := f.request(t, "POST", "/query-jobs", "display-edit", in)
			require.Equal(t, 200, code)
			var job d.Job
			require.NoError(t, json.Unmarshal(raw, &job))
			require.NoError(t, s.Tick(ctx, f.p, job.ID))
			require.Equal(t, int64(1), a.submits.Load())
			// Actual SQL update advances updated_at without changing the connection.
			a.source.Name = "renamed"
			a.source.Description = "only metadata changed"
			require.NoError(t, sourceRepo.Update(ctx, a.source))
			a.source, err = sourceRepo.Get(ctx, a.source.ID, f.p.TenantID)
			require.NoError(t, err)
			code, _ = f.request(t, "GET", "/query-jobs/"+job.ID, "", nil)
			require.Equal(t, 200, code)
			if reason == "cancel" {
				code, _ = f.request(t, "POST", "/query-jobs/"+job.ID+"/cancel", "", map[string]any{})
				require.Equal(t, 200, code)
			} else {
				_, err = repo.Change(ctx, f.p, job.ID, 0, func(r *d.Record, now time.Time) error { r.Job.Deadline = now.Add(-time.Second); return nil })
				require.NoError(t, err)
			}
			restarted := *s
			require.NoError(t, restarted.Tick(ctx, f.p, job.ID))
			actual, err := repo.Get(ctx, f.p, job.ID)
			require.NoError(t, err)
			require.True(t, actual.Job.Terminal())
			require.Equal(t, int64(1), a.submits.Load())
			require.Equal(t, int64(1), a.cancels.Load())
			if reason == "cancel" {
				require.Equal(t, "cancelled", actual.Job.Status)
			} else {
				require.Equal(t, "timed_out", actual.Job.Status)
			}
		})
	}
}

// This adapter is an explicit deterministic remote protocol fixture. MySQL,
// HTTP, row locks and restart checkpoints are real; this is not live BigQuery.
type queryRemoteFixture struct {
	source                  *ds.DataSource
	submits, pages, cancels atomic.Int64
	lost                    atomic.Bool
	found                   atomic.Bool
}

func (a *queryRemoteFixture) AcquireQuery(context.Context, int64, string) (ds.QueryAdapter, *ds.DataSource, error) {
	return a, a.source, nil
}
func (a *queryRemoteFixture) Capabilities() ds.QueryCapabilities {
	return ds.QueryCapabilities{Engine: ds.TypeBigQuery, Async: true}
}
func (a *queryRemoteFixture) Query(context.Context, ds.QueryRequest) (*ds.QueryResult, error) {
	return nil, errors.New("unexpected synchronous query")
}
func (a *queryRemoteFixture) Submit(context.Context, ds.QueryRequest) (string, error) {
	a.submits.Add(1)
	a.found.Store(true)
	if a.lost.Swap(false) {
		return "", errors.New("lost response")
	}
	return "remote-private-handle", nil
}
func (a *queryRemoteFixture) Status(context.Context, string) (string, error) { return "succeeded", nil }
func (a *queryRemoteFixture) Cancel(context.Context, string) error           { a.cancels.Add(1); return nil }
func (a *queryRemoteFixture) LookupSubmission(context.Context, ds.QueryRequest) (string, string, bool, error) {
	return "remote-private-handle", "succeeded", a.found.Load(), nil
}
func (a *queryRemoteFixture) Page(_ context.Context, _, cursor string, _ int) (*ds.QueryResult, string, error) {
	a.pages.Add(1)
	next := ""
	if cursor == "" {
		next = "secret-page-token"
	}
	return &ds.QueryResult{SourceVersion: ds.QueryConnectionVersion(a.source), Columns: []string{"code", "amount", "large"}, ColumnTypes: map[string]string{"code": "STRING", "amount": "NUMERIC", "large": "INT64"}, Rows: []map[string]any{{"code": "00123", "amount": "9007199254740993.123456789012345678", "large": json.Number("9223372036854775807")}}}, next, nil
}
func queryFixture(t *testing.T) (*reviewedFixture, *jobs.Service, d.Repository, *queryRemoteFixture, *atomic.Bool) {
	f := newReviewedFixture(t)
	var version int
	require.NoError(t, f.db.Raw("SELECT version FROM schema_migrations").Scan(&version).Error)
	require.GreaterOrEqual(t, version, 32)
	t.Cleanup(func() {
		require.NoError(t, f.db.Table("query_jobs").Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
	})
	a := &queryRemoteFixture{source: &ds.DataSource{ID: "synthetic-source", TenantID: f.p.TenantID, Type: ds.TypeBigQuery, Status: "active", UpdatedAt: time.Now().UTC()}}
	deny := &atomic.Bool{}
	repo := mysqlrepo.NewQueryJobRepository(f.db)
	cipher, e := source.NewCipher("isolated-query-test-key")
	require.NoError(t, e)
	s, e := jobs.New(repo, a, func(_ context.Context, p l.Principal, _ string) (*ds.DataSource, error) {
		if deny.Load() || p != f.p {
			return nil, d.ErrForbidden
		}
		return a.source, nil
	}, cipher)
	require.NoError(t, e)
	NewQueryJobHandler(s).Register(f.router.Group("/api"))
	return f, s, repo, a, deny
}
func queryInput() d.Input {
	return d.Input{DataSourceID: "synthetic-source", SQL: "SELECT code, amount, large FROM samples", MaxRows: 10, MaxBytes: 4096, TimeoutSeconds: 30, MaxScanBytes: 1000}
}
func TestQueryJobMySQLHTTPCheckpointRestartScopeAndPrecision(t *testing.T) {
	f, s, repo, a, deny := queryFixture(t)
	ctx := context.Background()
	in := queryInput()
	code, raw := f.request(t, "POST", "/query-jobs", "same-key", in)
	require.Equal(t, 200, code)
	var job d.Job
	require.NoError(t, json.Unmarshal(raw, &job))
	code, replay := f.request(t, "POST", "/query-jobs", "same-key", in)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(raw), string(replay))
	changed := in
	changed.MaxRows--
	code, _ = f.request(t, "POST", "/query-jobs", "same-key", changed)
	require.Equal(t, 409, code)
	for _, header := range []string{"X-Other-User", "X-Other-Tenant"} {
		code, _ = f.request(t, "GET", "/query-jobs/"+job.ID, "", nil, header)
		require.Equal(t, 404, code)
	}
	require.NoError(t, s.Tick(ctx, f.p, job.ID))
	saved, e := repo.Get(ctx, f.p, job.ID)
	require.NoError(t, e)
	require.Equal(t, 1, saved.Job.RowCount)
	require.Equal(t, int64(1), a.submits.Load())
	require.NotContains(t, saved.Operation, "remote-private-handle")
	require.NotContains(t, saved.Cursor, "secret-page-token")
	// Reconstructing the service consumes the persisted second cursor, never SQL.
	restored := *s
	require.NoError(t, restored.Tick(ctx, f.p, job.ID))
	require.Equal(t, int64(1), a.submits.Load())
	require.Equal(t, int64(2), a.pages.Load())
	code, raw = f.request(t, "GET", "/query-jobs/"+job.ID+"/results?page_size=1", "", nil)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), "9007199254740993.123456789012345678")
	require.Contains(t, string(raw), "9223372036854775807")
	require.Contains(t, string(raw), "00123")
	var page jobs.Results
	require.NoError(t, json.Unmarshal(raw, &page))
	require.NotEmpty(t, page.NextCursor)
	code, raw = f.request(t, "GET", "/query-jobs/"+job.ID, "", nil)
	require.Equal(t, 200, code)
	require.NotContains(t, string(raw), "remote-private-handle")
	require.NotContains(t, string(raw), "secret-page-token")
	require.NotContains(t, string(raw), "prepared")
	deny.Store(true)
	code, _ = f.request(t, "GET", "/query-jobs/"+job.ID+"/results", "", nil)
	require.Equal(t, 403, code)
	code, _ = f.request(t, "POST", "/query-jobs/"+job.ID+"/cancel", "", map[string]any{})
	require.Equal(t, 200, code)
}
func TestQueryJobMySQLLeaseFenceDeadlineAndUnknownCancellation(t *testing.T) {
	f, s, repo, a, _ := queryFixture(t)
	ctx := context.Background()
	j, e := s.Create(ctx, f.p, queryInput(), "claim")
	require.NoError(t, e)
	var wg sync.WaitGroup
	wins := atomic.Int64{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := repo.Claim(ctx, f.p, j.ID); e == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), wins.Load())
	old, e := repo.Get(ctx, f.p, j.ID)
	require.NoError(t, e)
	require.NoError(t, f.db.Exec("UPDATE query_jobs SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=? AND tenant_id=?", j.ID, f.p.TenantID).Error)
	newLease, e := repo.Claim(ctx, f.p, j.ID)
	require.NoError(t, e)
	_, e = repo.Change(ctx, f.p, j.ID, old.Epoch, func(*d.Record, time.Time) error { return nil })
	require.ErrorIs(t, e, d.ErrConflict)
	require.NoError(t, repo.Release(ctx, f.p, j.ID, newLease.Epoch))
	a.lost.Store(true)
	require.NoError(t, s.Tick(ctx, f.p, j.ID))
	require.Equal(t, int64(1), a.submits.Load())
	_, e = s.Cancel(ctx, f.p, j.ID)
	require.NoError(t, e)
	require.NoError(t, s.Tick(ctx, f.p, j.ID))
	require.Equal(t, int64(1), a.submits.Load())
	done, e := repo.Get(ctx, f.p, j.ID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", done.Job.Status)
	require.Equal(t, "succeeded", done.Job.RemoteStatus)
	late, e := s.Create(ctx, f.p, queryInput(), "deadline")
	require.NoError(t, e)
	_, e = repo.Change(ctx, f.p, late.ID, 0, func(r *d.Record, now time.Time) error { r.Job.Deadline = now.Add(-time.Second); return nil })
	require.NoError(t, e)
	require.NoError(t, s.Tick(ctx, f.p, late.ID))
	require.Equal(t, int64(1), a.submits.Load())
	expired, e := repo.Get(ctx, f.p, late.ID)
	require.NoError(t, e)
	require.Equal(t, "timed_out", expired.Job.Status)
}

// Datetime columns are SQL wall clocks used only inside SQL. JSON deadlines and
// public timestamps must remain actual instants regardless of driver loc.
func TestQueryJobMySQLNonUTCLocation(t *testing.T) {
	raw := os.Getenv("MYSQL_DSN")
	if raw == "" {
		t.Skip("MYSQL_DSN required")
	}
	cfg, err := driver.ParseDSN(raw)
	require.NoError(t, err)
	cfg.Loc, err = time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	t.Setenv("MYSQL_DSN", cfg.FormatDSN())
	f, s, repo, a, _ := queryFixture(t)
	ctx := context.Background()
	job, err := s.Create(ctx, f.p, queryInput(), "non-utc")
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), job.CreatedAt, 3*time.Second)
	require.WithinDuration(t, time.Now().Add(30*time.Second), job.Deadline, 3*time.Second)
	lease, err := repo.Claim(ctx, f.p, job.ID)
	require.NoError(t, err)
	_, err = repo.Claim(ctx, f.p, job.ID)
	require.ErrorIs(t, err, d.ErrConflict)
	runnable, err := repo.Runnable(ctx, 100)
	require.NoError(t, err)
	for _, item := range runnable {
		require.NotEqual(t, job.ID, item.ID, "unexpired lease cannot be polled")
	}
	_, err = repo.Change(ctx, f.p, job.ID, lease.Epoch, func(r *d.Record, now time.Time) error {
		require.WithinDuration(t, time.Now(), now, 3*time.Second)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, f.db.Exec("UPDATE query_jobs SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE id=? AND tenant_id=?", job.ID, f.p.TenantID).Error)
	_, err = repo.Change(ctx, f.p, job.ID, lease.Epoch, func(*d.Record, time.Time) error { return nil })
	require.ErrorIs(t, err, d.ErrConflict)
	next, err := repo.Claim(ctx, f.p, job.ID)
	require.NoError(t, err)
	require.Greater(t, next.Epoch, lease.Epoch)
	require.NoError(t, repo.Release(ctx, f.p, job.ID, next.Epoch))
	require.NoError(t, s.Tick(ctx, f.p, job.ID))
	require.NoError(t, s.Tick(ctx, f.p, job.ID))
	done, err := repo.Get(ctx, f.p, job.ID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", done.Job.Status)
	require.Equal(t, int64(1), a.submits.Load())
	require.WithinDuration(t, time.Now(), done.Job.UpdatedAt, 3*time.Second)
}
