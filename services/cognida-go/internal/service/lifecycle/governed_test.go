package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type governedMemoryRepo struct {
	d.Repository
	mu, tx       sync.Mutex
	stages       map[string]d.StageRun
	assets       map[string]d.DataAssetRef
	beforeCreate func()
}

func cloneStage(st d.StageRun) *d.StageRun {
	raw, _ := json.Marshal(st)
	var out d.StageRun
	_ = json.Unmarshal(raw, &out)
	out.IdempotencyKey = st.IdempotencyKey
	return &out
}
func (r *governedMemoryRepo) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	r.tx.Lock()
	defer r.tx.Unlock()
	return fn(r)
}
func (r *governedMemoryRepo) FindStage(ctx context.Context, tenant int64, id string, _ bool) (*d.StageRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.stages[id]
	if !ok || st.TenantID != tenant {
		return nil, d.ErrNotFound
	}
	return cloneStage(st), nil
}
func (r *governedMemoryRepo) FindStageByKey(ctx context.Context, tenant int64, key string) (*d.StageRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, st := range r.stages {
		if st.TenantID == tenant && st.IdempotencyKey == key {
			return cloneStage(st), nil
		}
	}
	return nil, d.ErrNotFound
}
func (r *governedMemoryRepo) CreateStage(ctx context.Context, st *d.StageRun) error {
	if r.beforeCreate != nil {
		r.beforeCreate()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.stages {
		if old.TenantID == st.TenantID && old.IdempotencyKey == st.IdempotencyKey {
			return d.ErrConflict
		}
	}
	r.stages[st.ID] = *cloneStage(*st)
	return nil
}
func (r *governedMemoryRepo) UpdateStage(ctx context.Context, st *d.StageRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages[st.ID] = *cloneStage(*st)
	return nil
}
func (*governedMemoryRepo) AppendEvent(context.Context, *d.Event) error { return nil }
func (r *governedMemoryRepo) FindAsset(ctx context.Context, tenant int64, id d.RefID) (d.DataAssetRef, error) {
	if err := ctx.Err(); err != nil {
		return d.DataAssetRef{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.assets[id.String()]
	if !ok || v.TenantID != tenant {
		return v, d.ErrNotFound
	}
	v.Parents = append([]d.RefID(nil), v.Parents...)
	return v, nil
}
func (r *governedMemoryRepo) PutAsset(ctx context.Context, v d.DataAssetRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[v.RefID.String()] = v
	return nil
}

type governedQueue struct {
	job.Queue
	mu   sync.Mutex
	jobs []job.Job
	err  error
}

func (q *governedQueue) Enqueue(_ context.Context, j job.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, j)
	return nil
}

type governedCompute func(context.Context, string, d.Input, map[string]any, string) (ComputeResult, error)

func (f governedCompute) Call(c context.Context, o string, i d.Input, v map[string]any, r string) (ComputeResult, error) {
	return f(c, o, i, v, r)
}
func governedFixture(t *testing.T) (*Service, *governedMemoryRepo, *governedQueue, d.Principal, d.DataAssetRef) {
	t.Helper()
	r := &governedMemoryRepo{stages: map[string]d.StageRun{}, assets: map[string]d.DataAssetRef{}}
	q := &governedQueue{}
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	s, e := NewService(r, objects, q, Config{Authorize: func(_ context.Context, p d.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	p := d.Principal{TenantID: 11, UserID: 12}
	ref, e := s.ImportWithKey(context.Background(), p, d.Input{Kind: "text", Text: "  sample  sample "}, "upload", "test-upload")
	require.NoError(t, e)
	return s, r, q, p, ref
}
func governedRequest(ref d.RefID, agent string) GovernedInvocation {
	preset, _ := GovernedPresetFor(agent)
	return GovernedInvocation{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), AgentID: agent, Profile: preset.Profile, InputRefs: []d.RefID{ref}, TimeoutSeconds: 60, MaxCalls: 8, CallsReserved: 1 + preset.BaseCalls, Deadline: time.Now().Add(time.Minute)}
}

func TestGovernedSubmitIdentityReplayAndNoPublicOptions(t *testing.T) {
	s, r, q, p, ref := governedFixture(t)
	ctx := context.Background()
	in := governedRequest(ref.RefID, "data-quality")
	st, e := s.InvokeGoverned(ctx, p, in, "rid")
	require.NoError(t, e)
	require.Equal(t, 1, st.MaxAttempts)
	same, e := s.InvokeGoverned(ctx, p, in, "different-attempt-request")
	require.NoError(t, e)
	require.Equal(t, st.ID, same.ID)
	require.Len(t, q.jobs, 1)
	changed := in
	changed.PayloadHash = strings.Repeat("c", 64)
	_, e = s.InvokeGoverned(ctx, p, changed, "")
	require.ErrorIs(t, e, d.ErrConflict)
	_, e = s.InspectGoverned(ctx, p, in, uuid.NewString())
	require.ErrorIs(t, e, d.ErrNotFound)
	other := p
	other.UserID++
	v, e := s.LookupGoverned(ctx, other, in)
	require.NoError(t, e)
	require.Nil(t, v)
	_, e = s.Invoke(ctx, p, "data-quality", in.InputRefs, map[string]any{governedOption: in}, "forged", "")
	require.ErrorIs(t, e, d.ErrInvalid)
	require.Len(t, r.stages, 1)
	s.config.Authorize = nil
	_, e = s.LookupGoverned(ctx, p, in)
	require.ErrorIs(t, e, d.ErrForbidden)
}

func TestGovernedSourcesRejectExternalLegacyCyclesAndBounds(t *testing.T) {
	s, r, _, p, root := governedFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{root.RefID}))
	body := json.RawMessage(`{"evidence":true}`)
	derived, e := s.ImportDerived(ctx, p, d.QualityReport, "quality", []d.RefID{root.RefID}, body, root.Policy)
	require.NoError(t, e)
	require.NoError(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{derived.RefID}))
	legacy, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "legacy"}, "legacy")
	require.NoError(t, e)
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{legacy.RefID}), d.ErrForbidden)
	external := root
	external.Provenance.SourceType = "knowledge"
	r.assets[root.RefID.String()] = external
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{derived.RefID}), d.ErrForbidden)
	r.assets[root.RefID.String()] = root
	cycle := derived
	cycle.Parents = []d.RefID{derived.RefID}
	r.assets[derived.RefID.String()] = cycle
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{derived.RefID}), d.ErrInvalid)
	r.assets[derived.RefID.String()] = derived
	parent := root.RefID
	for i := 0; i < 17; i++ {
		next, e := s.ImportDerived(ctx, p, d.QualityReport, uuid.NewString(), []d.RefID{parent}, body, root.Policy)
		require.NoError(t, e)
		parent = next.RefID
	}
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{parent}), d.ErrInvalid)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, []d.RefID{root.RefID}), context.Canceled)
}

func TestGovernedCancelledTombstoneFencesLateSubmissionAndRevokedSource(t *testing.T) {
	s, r, q, p, ref := governedFixture(t)
	ctx := context.Background()
	in := governedRequest(ref.RefID, "data-quality")
	st, e := s.CancelGovernedSubmission(ctx, p, in, "cancel")
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, st.Status)
	late, e := s.InvokeGoverned(ctx, p, in, "late")
	require.NoError(t, e)
	require.Equal(t, st.ID, late.ID)
	require.Equal(t, d.StatusCancelled, late.Status)
	require.Empty(t, q.jobs)
	ref.Status = "disabled"
	r.assets[ref.RefID.String()] = ref
	found, e := s.LookupGoverned(ctx, p, in)
	require.NoError(t, e)
	require.Equal(t, st.ID, found.ID)
	_, e = s.InspectGoverned(ctx, p, in, st.ID)
	require.ErrorIs(t, e, d.ErrForbidden)
	cancelled, e := s.CancelGoverned(ctx, p, in, st.ID)
	require.NoError(t, e)
	require.Nil(t, cancelled.Output)
}

func TestGovernedConcurrentCancelAndSubmitHaveOneTerminalRecord(t *testing.T) {
	s, r, q, p, ref := governedFixture(t)
	in := governedRequest(ref.RefID, "data-quality")
	var arrived sync.WaitGroup
	arrived.Add(2)
	r.beforeCreate = func() { arrived.Done(); arrived.Wait() }
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() { defer wg.Done(); _, e := s.InvokeGoverned(context.Background(), p, in, "submit"); errs <- e }()
	go func() {
		defer wg.Done()
		_, e := s.CancelGovernedSubmission(context.Background(), p, in, "cancel")
		errs <- e
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	r.beforeCreate = nil
	st, e := s.LookupGoverned(context.Background(), p, in)
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, st.Status)
	require.Len(t, r.stages, 1)
	for _, j := range q.jobs {
		require.NoError(t, s.Handle(context.Background(), j))
	}
	latest, e := s.LookupGoverned(context.Background(), p, in)
	require.NoError(t, e)
	require.Equal(t, 0, latest.Attempt)
}

func TestGovernedWorkerUsesOnlyPresetAndSingleAttempt(t *testing.T) {
	for _, agent := range []string{"data-quality", "data-cleaning", "data-enrichment", "data-evaluation"} {
		t.Run(agent, func(t *testing.T) {
			s, _, q, p, ref := governedFixture(t)
			var operations []string
			s.config.Compute = governedCompute(func(_ context.Context, op string, input d.Input, opts map[string]any, _ string) (ComputeResult, error) {
				operations = append(operations, op)
				out := ComputeResult{SchemaVersion: "1", Method: "fixture-rules", Result: map[string]any{}}
				switch op {
				case "profile":
					out.Result["overall_score"] = 95
				case "clean":
					require.Equal(t, []any{"trim", "dedup"}, opts["operations"])
					require.Equal(t, "first", opts["keep"])
					out.Result["data"] = input
				case "enrich":
					require.Equal(t, "rules", opts["method"])
					out.Result["keywords"] = []string{"sample"}
				default:
					t.Fatalf("unexpected compute %s", op)
				}
				return out, nil
			})
			s.adapters[agent] = adapterFunc(func(context.Context, d.StageRequest) (d.Output, error) {
				t.Fatal("custom adapter must not run")
				return d.Output{}, nil
			})
			if agent == "data-evaluation" {
				var e error
				ref, e = s.ImportDerived(context.Background(), p, d.QualityReport, "prior-quality", []d.RefID{ref.RefID}, json.RawMessage(`{"quality":95}`), ref.Policy)
				require.NoError(t, e)
			}
			in := governedRequest(ref.RefID, agent)
			st, e := s.InvokeGoverned(context.Background(), p, in, "")
			require.NoError(t, e)
			require.NoError(t, s.Handle(context.Background(), q.jobs[0]))
			got, e := s.InspectGoverned(context.Background(), p, in, st.ID)
			require.NoError(t, e)
			require.Equal(t, d.StatusSucceeded, got.Status)
			require.NotEmpty(t, got.Output.Refs)
			require.Equal(t, 1, got.Attempt)
			require.NoError(t, s.Handle(context.Background(), q.jobs[0]))
			want := map[string][]string{"data-quality": {"profile"}, "data-cleaning": {"clean", "profile"}, "data-enrichment": {"enrich"}, "data-evaluation": nil}
			require.Equal(t, want[agent], operations)
		})
	}
}

func TestGovernedWorkerTimeoutAndFailureNeverRetry(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "compute_failure", true: "deadline"}[timeout], func(t *testing.T) {
			s, _, q, p, ref := governedFixture(t)
			var calls atomic.Int32
			s.config.StageTimeout = 25 * time.Millisecond
			s.config.Compute = governedCompute(func(ctx context.Context, _ string, _ d.Input, _ map[string]any, _ string) (ComputeResult, error) {
				calls.Add(1)
				if timeout {
					<-ctx.Done()
					return ComputeResult{}, ctx.Err()
				}
				return ComputeResult{}, errors.New("failed")
			})
			in := governedRequest(ref.RefID, "data-quality")
			st, e := s.InvokeGoverned(context.Background(), p, in, "")
			require.NoError(t, e)
			require.NoError(t, s.Handle(context.Background(), q.jobs[0]))
			got, e := s.LookupGoverned(context.Background(), p, in)
			require.NoError(t, e)
			require.Equal(t, st.ID, got.ID)
			require.Equal(t, d.StatusFailed, got.Status)
			require.Equal(t, timeout, GovernedTimedOut(got))
			require.NoError(t, s.Handle(context.Background(), q.jobs[0]))
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, 1, got.MaxAttempts)
		})
	}
}

func TestGovernedStageLimitsAndRevocationAtWorker(t *testing.T) {
	s, r, q, p, ref := governedFixture(t)
	ctx := context.Background()
	in := governedRequest(ref.RefID, "data-quality")
	in.MaxCalls = 1
	_, e := s.InvokeGoverned(ctx, p, in, "")
	require.ErrorIs(t, e, d.ErrInvalid)
	in.MaxCalls = 2
	in.TimeoutSeconds = 1
	in.Deadline = time.Now().Add(40 * time.Millisecond)
	st, e := s.InvokeGoverned(ctx, p, in, "")
	require.NoError(t, e)
	work, cancel, gov, e := s.governedStageContext(ctx, st)
	require.NoError(t, e)
	defer cancel()
	require.True(t, gov)
	limit, _ := work.Deadline()
	require.Equal(t, in.Deadline.UnixNano(), limit.UnixNano())
	ref.Status = "disabled"
	r.assets[ref.RefID.String()] = ref
	s.config.Compute = governedCompute(func(context.Context, string, d.Input, map[string]any, string) (ComputeResult, error) {
		t.Fatal("revoked input must not reach compute")
		return ComputeResult{}, nil
	})
	require.NoError(t, s.Handle(ctx, q.jobs[0]))
	got, e := s.LookupGoverned(ctx, p, in)
	require.NoError(t, e)
	require.Equal(t, d.StatusFailed, got.Status)
}

func TestGovernedRunningCancellationCannotCommitSuccess(t *testing.T) {
	s, _, q, p, ref := governedFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	s.config.Compute = governedCompute(func(ctx context.Context, _ string, _ d.Input, _ map[string]any, _ string) (ComputeResult, error) {
		close(entered)
		select {
		case <-release:
			return ComputeResult{SchemaVersion: "1", Method: "fixture", Result: map[string]any{"overall_score": 95}}, nil
		case <-ctx.Done():
			return ComputeResult{}, ctx.Err()
		}
	})
	in := governedRequest(ref.RefID, "data-quality")
	st, e := s.InvokeGoverned(context.Background(), p, in, "")
	require.NoError(t, e)
	done := make(chan error, 1)
	go func() { done <- s.Handle(context.Background(), q.jobs[0]) }()
	<-entered
	stopped, e := s.CancelGoverned(context.Background(), p, in, st.ID)
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, stopped.Status)
	close(release)
	require.NoError(t, <-done)
	final, e := s.InspectGoverned(context.Background(), p, in, st.ID)
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, final.Status)
	require.Nil(t, final.Output)
}

func TestGovernedLineageNodeLimitAndRevokedAncestor(t *testing.T) {
	s, r, _, p, root := governedFixture(t)
	ctx := context.Background()
	body := json.RawMessage(`{"quality":95}`)
	refs := []d.RefID{}
	for branch := 0; branch < 9; branch++ {
		parent := root.RefID
		for depth := 0; depth < 15; depth++ {
			next, e := s.ImportDerived(ctx, p, d.QualityReport, uuid.NewString(), []d.RefID{parent}, body, root.Policy)
			require.NoError(t, e)
			parent = next.RefID
		}
		refs = append(refs, parent)
	}
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, refs), d.ErrInvalid)
	require.NoError(t, s.ValidateGovernedInputs(ctx, p, refs[:1]))
	root.Status = "disabled"
	r.assets[root.RefID.String()] = root
	require.ErrorIs(t, s.ValidateGovernedInputs(ctx, p, refs[:1]), d.ErrForbidden)
}
