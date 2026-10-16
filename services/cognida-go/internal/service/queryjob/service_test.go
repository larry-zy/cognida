package queryjob

import (
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/queryjob"
	source "cognida/internal/service/datasource"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type memRepo struct {
	mu     sync.Mutex
	r      *d.Record
	hash   string
	leased bool
}

func clone(r *d.Record) *d.Record {
	out := *r
	raw, _ := json.Marshal(r)
	_ = json.Unmarshal(raw, &out)
	out.Job.TenantID = r.Job.TenantID
	out.Job.UserID = r.Job.UserID
	out.Epoch = r.Epoch
	return &out
}
func (m *memRepo) Create(_ context.Context, p l.Principal, in d.Input, engine ds.Type, version, key string) (*d.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := d.Digest([]any{p, in, engine, version, key})
	if m.r != nil {
		if hash != m.hash {
			return nil, d.ErrConflict
		}
		return clone(m.r), nil
	}
	m.hash = hash
	m.r = &d.Record{Job: d.Job{ID: "synthetic-operation", TenantID: p.TenantID, UserID: p.UserID, Input: in, Engine: engine, SourceVersion: version, Status: "queued", Deadline: time.Now().Add(30 * time.Second)}, Phase: "queued"}
	return clone(m.r), nil
}
func (m *memRepo) Get(_ context.Context, p l.Principal, id string) (*d.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r == nil || m.r.Job.TenantID != p.TenantID || m.r.Job.UserID != p.UserID || m.r.Job.ID != id {
		return nil, d.ErrNotFound
	}
	return clone(m.r), nil
}
func (m *memRepo) List(context.Context, l.Principal, string, int) ([]d.Job, error) { return nil, nil }
func (m *memRepo) Runnable(context.Context, int) ([]d.Item, error)                 { return nil, nil }
func (m *memRepo) Claim(ctx context.Context, p l.Principal, id string) (*d.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.leased || m.r.Job.Terminal() {
		return nil, d.ErrConflict
	}
	m.leased = true
	m.r.Epoch++
	return clone(m.r), nil
}
func (m *memRepo) Change(_ context.Context, p l.Principal, id string, epoch int64, fn func(*d.Record, time.Time) error) (*d.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if epoch != 0 && (!m.leased || epoch != m.r.Epoch) {
		return nil, d.ErrConflict
	}
	v := clone(m.r)
	if e := fn(v, time.Now()); e != nil {
		return nil, e
	}
	m.r = v
	return clone(v), nil
}
func (m *memRepo) Release(context.Context, l.Principal, string, int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leased = false
	return nil
}

type remote struct {
	source                        *ds.DataSource
	submit, lookup, cancel, pages int
	lost                          bool
	found                         bool
	state                         string
	afterPage                     func()
	row                           string
}

func (a *remote) AcquireQuery(context.Context, int64, string) (ds.QueryAdapter, *ds.DataSource, error) {
	return a, a.source, nil
}
func (a *remote) Capabilities() ds.QueryCapabilities {
	return ds.QueryCapabilities{Engine: a.source.Type, Async: true}
}
func (a *remote) Query(context.Context, ds.QueryRequest) (*ds.QueryResult, error) {
	return nil, errors.New("not used")
}
func (a *remote) Submit(context.Context, ds.QueryRequest) (string, error) {
	a.submit++
	a.found = true
	if a.lost {
		a.lost = false
		return "", errors.New("response lost")
	}
	return "private-remote-handle", nil
}
func (a *remote) LookupSubmission(context.Context, ds.QueryRequest) (string, string, bool, error) {
	a.lookup++
	return "private-remote-handle", a.state, a.found, nil
}
func (a *remote) Status(context.Context, string) (string, error) { return a.state, nil }
func (a *remote) Cancel(context.Context, string) error           { a.cancel++; return nil }
func (a *remote) Page(_ context.Context, _, cursor string, _ int) (*ds.QueryResult, string, error) {
	a.pages++
	next := ""
	if cursor == "" {
		next = "opaque-next"
	}
	if a.afterPage != nil {
		a.afterPage()
	}
	return &ds.QueryResult{Columns: []string{"amount"}, ColumnTypes: map[string]string{"amount": "NUMERIC"}, Rows: []map[string]any{{"amount": a.row}}}, next, nil
}

type preparedRemote struct {
	*remote
	prepared, executed, abandoned int
}

func (a *preparedRemote) AcquireQuery(context.Context, int64, string) (ds.QueryAdapter, *ds.DataSource, error) {
	return a, a.source, nil
}
func (a *preparedRemote) Prepare(context.Context, ds.QueryRequest) (string, error) {
	a.prepared++
	return "private-prepared", nil
}
func (a *preparedRemote) SubmitPrepared(context.Context, string, ds.QueryRequest) (string, error) {
	a.executed++
	return "", errors.New("lost execute response")
}
func (a *preparedRemote) AbandonPrepared(context.Context, string) error { a.abandoned++; return nil }
func setup(t *testing.T) (*Service, *memRepo, *remote, l.Principal) {
	t.Helper()
	p := l.Principal{TenantID: 1, UserID: 2}
	r := &memRepo{}
	a := &remote{source: &ds.DataSource{ID: "source", TenantID: 1, Type: ds.TypeBigQuery, Status: "active", UpdatedAt: time.Now()}, state: "succeeded", row: "9007199254740993.123456789012345678"}
	cipher, e := source.NewCipher("synthetic-unit-key")
	require.NoError(t, e)
	s, e := New(r, a, func(context.Context, l.Principal, string) (*ds.DataSource, error) { return a.source, nil }, cipher)
	require.NoError(t, e)
	return s, r, a, p
}
func input() d.Input {
	return d.Input{DataSourceID: "source", SQL: "SELECT amount FROM samples", MaxRows: 4, MaxBytes: 4096, TimeoutSeconds: 30, MaxScanBytes: 1000}
}
func TestResultCheckpointExactnessAndRestart(t *testing.T) {
	s, r, a, p := setup(t)
	j, e := s.Create(context.Background(), p, input(), "key")
	require.NoError(t, e)
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.Equal(t, 1, a.submit)
	require.Equal(t, 1, r.r.Job.RowCount)
	require.NotContains(t, r.r.Operation, "private-remote")
	copy := *s
	require.NoError(t, copy.Tick(context.Background(), p, j.ID))
	require.Equal(t, 1, a.submit)
	result, e := copy.Results(context.Background(), p, j.ID, "", 1)
	require.NoError(t, e)
	require.Equal(t, a.row, result.Rows[0]["amount"])
	require.NotEmpty(t, result.NextCursor)
	second, e := copy.Results(context.Background(), p, j.ID, result.NextCursor, 1)
	require.NoError(t, e)
	require.Len(t, second.Rows, 1)
	require.Empty(t, second.NextCursor)
	require.Equal(t, "succeeded", r.r.Job.Status)
	require.Equal(t, 1, a.cancel)
}
func TestUnknownCancellationDoesReadLookupNeverResubmit(t *testing.T) {
	s, r, a, p := setup(t)
	a.lost = true
	j, e := s.Create(context.Background(), p, input(), "key")
	require.NoError(t, e)
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.Equal(t, "submitting", r.r.Phase)
	_, e = s.Cancel(context.Background(), p, j.ID)
	require.NoError(t, e)
	a.found = false
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.Equal(t, "reconcile_required", r.r.Job.Status)
	require.Equal(t, 1, a.submit)
	a.found = true
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.Equal(t, 1, a.submit)
	require.Equal(t, 2, a.lookup)
	require.Equal(t, "cancelled", r.r.Job.Status)
	require.Equal(t, "succeeded", r.r.Job.RemoteStatus)
	require.Nil(t, r.r.Result)
}
func TestPreparedUnknownClosesSessionNeverReplaysStatement(t *testing.T) {
	s, r, a, p := setup(t)
	a.source.Type = ds.TypeHive
	h := &preparedRemote{remote: a}
	s.provider = h
	in := input()
	in.MaxScanBytes = 0
	j, e := s.Create(context.Background(), p, in, "key")
	require.NoError(t, e)
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.Equal(t, 1, h.prepared)
	require.Equal(t, 1, h.executed)
	require.Equal(t, 1, h.abandoned)
	require.Equal(t, "aborted_unknown", r.r.Job.Status)
	require.ErrorIs(t, s.Tick(context.Background(), p, j.ID), d.ErrConflict)
	require.Equal(t, 1, h.executed)
}
func TestCancellationDuringPageCannotPublishPartialSnapshot(t *testing.T) {
	s, r, a, p := setup(t)
	j, e := s.Create(context.Background(), p, input(), "key")
	require.NoError(t, e)
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	a.afterPage = func() { _, e := s.Cancel(context.Background(), p, j.ID); require.NoError(t, e) }
	require.ErrorIs(t, s.Tick(context.Background(), p, j.ID), d.ErrConflict)
	a.afterPage = nil
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.Equal(t, "cancelled", r.r.Job.Status)
	_, e = s.Results(context.Background(), p, j.ID, "", 100)
	require.ErrorIs(t, e, d.ErrConflict)
}
func TestBoundsAuthorizationAndExactPayloadBinding(t *testing.T) {
	s, r, a, p := setup(t)
	in := input()
	in.MaxRows = 1
	j, e := s.Create(context.Background(), p, in, "key")
	require.NoError(t, e)
	replay, e := s.Create(context.Background(), p, in, "key")
	require.NoError(t, e)
	require.Equal(t, j.ID, replay.ID)
	changed := in
	changed.MaxBytes++
	_, e = s.Create(context.Background(), p, changed, "key")
	require.ErrorIs(t, e, d.ErrConflict)
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.NoError(t, s.Tick(context.Background(), p, j.ID))
	require.True(t, r.r.Job.Truncated)
	require.Equal(t, 1, r.r.Job.RowCount)
	a.source.Status = "disabled"
	_, e = s.Get(context.Background(), p, j.ID)
	require.ErrorIs(t, e, d.ErrForbidden)
	_, e = s.Results(context.Background(), p, j.ID, "", 1)
	require.ErrorIs(t, e, d.ErrForbidden)
	_, e = s.Cancel(context.Background(), p, j.ID)
	require.NoError(t, e)
}

func TestBigQueryCancellationAcknowledgementKeepsReconciling(t *testing.T) {
	s, repo, remote, p := setup(t)
	remote.state = "running"
	ctx := context.Background()
	job, err := s.Create(ctx, p, input(), "cancel-ack")
	require.NoError(t, err)
	require.NoError(t, s.Tick(ctx, p, job.ID))
	_, err = s.Cancel(ctx, p, job.ID)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		require.NoError(t, s.Tick(ctx, p, job.ID))
		require.Equal(t, "reconcile_required", repo.r.Job.Status)
		require.Equal(t, "running", repo.r.Job.RemoteStatus)
		require.True(t, repo.r.Job.CleanupPending)
		require.False(t, repo.r.Job.Terminal())
	}
	remote.state = "cancelled"
	require.NoError(t, s.Tick(ctx, p, job.ID))
	require.Equal(t, "cancelled", repo.r.Job.Status)
	require.Equal(t, "cancelled", repo.r.Job.RemoteStatus)
	require.True(t, repo.r.Job.Terminal())
	require.Equal(t, 1, remote.submit)
}

func TestExecuteRejectsUnpersistedParametersAndCallerRemoteIdentity(t *testing.T) {
	s := &Service{}
	for _, in := range []ds.QueryRequest{{SQL: "SELECT ?", Parameters: []any{42}}, {SQL: "SELECT 1", OperationID: "caller-remote-id"}} {
		_, err := s.Execute(context.Background(), 1, 2, "source", in, "key")
		require.ErrorIs(t, err, d.ErrInvalid)
	}
}
