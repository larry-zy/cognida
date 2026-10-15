package lifecycleadapter

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Storage doubles are intentionally limited to adapter contract tests; the
// integration suite exercises the same path against MySQL and Redis.
type adapterRepo struct {
	l.Repository
	mu, tx sync.Mutex
	stages map[string]l.StageRun
	assets map[string]l.DataAssetRef
}

func clone(st l.StageRun) *l.StageRun {
	b, _ := json.Marshal(st)
	var v l.StageRun
	_ = json.Unmarshal(b, &v)
	v.IdempotencyKey = st.IdempotencyKey
	return &v
}
func (r *adapterRepo) Transaction(ctx context.Context, f func(l.Repository) error) error {
	r.tx.Lock()
	defer r.tx.Unlock()
	return f(r)
}
func (r *adapterRepo) FindStage(_ context.Context, t int64, id string, _ bool) (*l.StageRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.stages[id]
	if !ok || v.TenantID != t {
		return nil, l.ErrNotFound
	}
	return clone(v), nil
}
func (r *adapterRepo) FindStageByKey(_ context.Context, t int64, k string) (*l.StageRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.stages {
		if v.TenantID == t && v.IdempotencyKey == k {
			return clone(v), nil
		}
	}
	return nil, l.ErrNotFound
}
func (r *adapterRepo) CreateStage(_ context.Context, v *l.StageRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.stages {
		if old.TenantID == v.TenantID && old.IdempotencyKey == v.IdempotencyKey {
			return l.ErrConflict
		}
	}
	r.stages[v.ID] = *clone(*v)
	return nil
}
func (r *adapterRepo) UpdateStage(_ context.Context, v *l.StageRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages[v.ID] = *clone(*v)
	return nil
}
func (*adapterRepo) AppendEvent(context.Context, *l.Event) error { return nil }
func (r *adapterRepo) FindAsset(_ context.Context, t int64, id l.RefID) (l.DataAssetRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.assets[id.String()]
	if !ok || v.TenantID != t {
		return v, l.ErrNotFound
	}
	return v, nil
}
func (r *adapterRepo) PutAsset(_ context.Context, v l.DataAssetRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[v.RefID.String()] = v
	return nil
}

type adapterQueue struct {
	job.Queue
	jobs []job.Job
	err  error
}

func (q *adapterQueue) Enqueue(_ context.Context, j job.Job) error {
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, j)
	return nil
}

type adapterCompute struct{}

func (adapterCompute) Call(_ context.Context, _ string, _ l.Input, _ map[string]any, _ string) (lifecycle.ComputeResult, error) {
	return lifecycle.ComputeResult{SchemaVersion: "1", Method: "unit-fixture", Result: map[string]any{"overall_score": 95}}, nil
}
func adapterFixture(t *testing.T) (*Adapter, *adapterRepo, *adapterQueue, tc.ExecutionCommand) {
	t.Helper()
	r := &adapterRepo{stages: map[string]l.StageRun{}, assets: map[string]l.DataAssetRef{}}
	q := &adapterQueue{}
	store, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	s, e := lifecycle.NewService(r, store, q, lifecycle.Config{Compute: adapterCompute{}, Authorize: func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	scope := d.Scope{TenantID: 11, UserID: 12}
	ref, e := s.ImportWithKey(context.Background(), principal(scope), l.Input{Kind: "text", Text: "scope test"}, "fixture", "upload-"+uuid.NewString())
	require.NoError(t, e)
	a, e := New(s, "lifecycle.data-quality")
	require.NoError(t, e)
	cmd := tc.ExecutionCommand{Scope: scope, TaskID: uuid.NewString(), PlanVersion: 1, Attempt: 1, InputRefs: []l.RefID{ref.RefID}, Deadline: time.Now().Add(time.Minute), RequestID: "request"}
	cmd.Node = tc.Node{ID: "quality", Capability: a.capability.ID, CapabilityVersion: a.capability.Version, Tools: append([]string(nil), a.capability.Tools...), Permissions: append([]string(nil), a.capability.Permissions...), Inputs: []tc.InputBinding{{Asset: &ref.RefID, Kind: l.RawAsset}}, OutputKinds: []l.Kind{l.QualityReport}, MaxAttempts: 2, MaxCallsPerAttempt: 2, TimeoutSeconds: 30}
	bindCommand(&cmd)
	return a, r, q, cmd
}
func bindCommand(cmd *tc.ExecutionCommand) {
	b, _ := json.Marshal([]any{cmd.Scope.TenantID, cmd.Scope.UserID, cmd.TaskID, cmd.PlanVersion, cmd.Node.ID, cmd.Attempt})
	cmd.OperationKey = d.Digest(b)
	b, _ = json.Marshal(cmd)
	b, _ = d.CanonicalJSON(b, '{', d.MaxSnapshotBytes)
	cmd.PayloadHash = d.Digest(b)
}

func TestAdapterSupportsOnlyFixedPresetsAndValidatesBounds(t *testing.T) {
	a, _, _, cmd := adapterFixture(t)
	ctx := context.Background()
	for _, capability := range []string{"lifecycle.data-quality", "lifecycle.data-cleaning", "lifecycle.data-enrichment", "lifecycle.data-evaluation"} {
		v, e := New(a.service, capability)
		require.NoError(t, e)
		require.NotEmpty(t, v.Support().Profile)
		require.Equal(t, 1, v.Support().CallsPerInput)
	}
	for _, capability := range []string{"lifecycle.data-publication", "lifecycle.data-analysis", "lifecycle.data-access", "research", ""} {
		_, e := New(a.service, capability)
		require.ErrorIs(t, e, d.ErrInvalid)
	}
	_, e := New(nil, cmd.Node.Capability)
	require.ErrorIs(t, e, d.ErrInvalid)
	require.NoError(t, a.Validate(ctx, cmd.Scope, cmd.Node))
	require.NoError(t, a.ValidateInputs(ctx, cmd.Scope, cmd.InputRefs))
	mutations := map[string]func(*tc.Node){"calls": func(n *tc.Node) { n.MaxCallsPerAttempt = 1 }, "timeout": func(n *tc.Node) { n.TimeoutSeconds = 301 }, "tools": func(n *tc.Node) { n.Tools = []string{"shell"} }, "permissions": func(n *tc.Node) { n.Permissions = []string{"write"} }, "version": func(n *tc.Node) { n.CapabilityVersion++ }, "capability": func(n *tc.Node) { n.Capability = "lifecycle.data-publication" }, "ambiguous input": func(n *tc.Node) {
		n.Inputs = []tc.InputBinding{{Asset: cmd.Node.Inputs[0].Asset, FromNode: "other", Kind: l.RawAsset}}
	}, "invalid output": func(n *tc.Node) { n.OutputKinds = []l.Kind{l.PublishedAsset} }, "duplicate output": func(n *tc.Node) { n.OutputKinds = []l.Kind{l.QualityReport, l.QualityReport} }, "attempts": func(n *tc.Node) { n.MaxAttempts = 0 }}
	for name, change := range mutations {
		t.Run(name, func(t *testing.T) {
			n := cmd.Node
			change(&n)
			require.ErrorIs(t, a.Validate(ctx, cmd.Scope, n), d.ErrInvalid)
		})
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, a.Validate(cancelled, cmd.Scope, cmd.Node), context.Canceled)
	require.Error(t, a.ValidateInputs(ctx, d.Scope{}, cmd.InputRefs))
}

func TestAdapterCommandBindingReceiptAndTruthfulObservation(t *testing.T) {
	a, r, q, cmd := adapterFixture(t)
	ctx := context.Background()
	missing, e := a.Lookup(ctx, cmd)
	require.NoError(t, e)
	require.Nil(t, missing)
	h, e := a.Submit(ctx, cmd)
	require.NoError(t, e)
	require.NotNil(t, h)
	same, e := a.Submit(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, h, same)
	require.Len(t, q.jobs, 1)
	observed, e := a.Inspect(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainQueued, observed.Status)
	require.Empty(t, observed.OutputRefs)
	require.Equal(t, 2, observed.CallsReserved)
	require.NoError(t, a.service.Handle(ctx, q.jobs[0]))
	observed, e = a.Inspect(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainSucceeded, observed.Status)
	require.NotEmpty(t, observed.OutputRefs)
	changed := cmd
	changed.RequestID = "changed"
	_, e = a.Lookup(ctx, changed)
	require.ErrorIs(t, e, d.ErrConflict)
	changed = cmd
	changed.Scope.UserID++
	_, e = a.Inspect(ctx, changed, *h)
	require.ErrorIs(t, e, d.ErrConflict)
	changed = cmd
	changed.Node.MaxCallsPerAttempt++
	bindCommand(&changed)
	_, e = a.Submit(ctx, changed)
	require.ErrorIs(t, e, d.ErrConflict)
	forged := *h
	forged.DomainID = uuid.NewString()
	_, e = a.Cancel(ctx, cmd, forged)
	require.ErrorIs(t, e, d.ErrConflict)
	forged.Receipt = handle(cmd, &l.StageRun{ID: forged.DomainID}).Receipt
	_, e = a.Cancel(ctx, cmd, forged)
	require.ErrorIs(t, e, d.ErrNotFound)
	visible, e := a.Cancel(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainSucceeded, visible.Status)
	require.Equal(t, observed.OutputRefs, visible.OutputRefs)
	// A receipt cannot grant access to disabled sources. Cancellation still
	// preserves already-committed success while withholding output references.
	ref := r.assets[cmd.InputRefs[0].String()]
	ref.Status = "disabled"
	r.assets[ref.RefID.String()] = ref
	_, e = a.Inspect(ctx, cmd, *h)
	require.ErrorIs(t, e, d.ErrForbidden)
	found, e := a.Lookup(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, h, found)
	observed, e = a.Cancel(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainSucceeded, observed.Status)
	require.Empty(t, observed.OutputRefs)
}

func TestAdapterUnknownCancellationExpiredDeadlineAndQueueReceipt(t *testing.T) {
	a, r, q, cmd := adapterFixture(t)
	ctx := context.Background()
	cmd.Deadline = time.Now().Add(-time.Second)
	bindCommand(&cmd)
	_, e := a.Submit(ctx, cmd)
	require.ErrorIs(t, e, context.DeadlineExceeded)
	ref := r.assets[cmd.InputRefs[0].String()]
	ref.Status = "disabled"
	r.assets[ref.RefID.String()] = ref
	h, e := a.CancelSubmission(ctx, cmd)
	require.NoError(t, e)
	out, e := a.Cancel(ctx, cmd, *h)
	require.NoError(t, e)
	require.Equal(t, d.DomainCancelled, out.Status)
	require.Empty(t, q.jobs)
	ref.Status = "available"
	r.assets[ref.RefID.String()] = ref
	late, e := a.Submit(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, h, late)
	require.Empty(t, q.jobs)
	a, _, q, cmd = adapterFixture(t)
	q.err = errors.New("redis password=do-not-leak")
	h, e = a.Submit(ctx, cmd)
	require.Error(t, e)
	require.NotContains(t, e.Error(), "do-not-leak")
	require.NotNil(t, h)
	found, e := a.Lookup(ctx, cmd)
	require.NoError(t, e)
	require.Equal(t, h, found)
}
