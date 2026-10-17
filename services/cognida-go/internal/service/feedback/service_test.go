package feedback

import (
	a "cognida/internal/model/agent"
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/rag"
	"cognida/internal/service/evaluation"
	control "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type memory struct {
	d.Repository
	c              *d.Case
	v              *d.Publication
	saves, commits int
}

func clone[T any](v *T) *T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return &out
}
func (m *memory) GetCase(_ context.Context, p l.Principal, id string) (*d.Case, error) {
	if m.c == nil || m.c.ID != id || m.c.UserID != p.UserID || m.c.TenantID != p.TenantID {
		return nil, d.ErrNotFound
	}
	return clone(m.c), nil
}
func (m *memory) SaveCase(_ context.Context, c *d.Case, expected int, _ d.Event) error {
	if m.c.Version != expected {
		return d.ErrConflict
	}
	c.Version++
	m.c = clone(c)
	m.saves++
	return nil
}
func (m *memory) GetPublication(_ context.Context, p l.Principal, id string) (*d.Publication, error) {
	if m.v == nil || m.v.ID != id || m.v.UserID != p.UserID || m.v.TenantID != p.TenantID {
		return nil, d.ErrNotFound
	}
	return clone(m.v), nil
}
func (m *memory) SavePublication(_ context.Context, v *d.Publication, expected int, _ d.Event) error {
	if m.v.Version != expected {
		return d.ErrConflict
	}
	v.Version++
	m.v = clone(v)
	m.saves++
	return nil
}
func (m *memory) ScanCases(context.Context, string, int) ([]d.Case, string, error) {
	return []d.Case{*clone(m.c)}, "", nil
}
func (m *memory) ListPublications(context.Context, l.Principal, string) ([]d.Publication, error) {
	return []d.Publication{*clone(m.v)}, nil
}
func (m *memory) FindKnowledge(_ context.Context, tenant int64, id string) (*d.Case, *d.Publication, error) {
	if tenant != m.c.TenantID {
		return nil, nil, nil
	}
	if id == m.c.BaselineKnowledgeID {
		return clone(m.c), nil, nil
	}
	if id == m.v.KnowledgeID {
		return clone(m.c), clone(m.v), nil
	}
	return nil, nil, nil
}
func (m *memory) Commit(_ context.Context, p l.Principal, id, pub string, version, pv int, digest, action string) error {
	if m.c.UserID != p.UserID || m.c.TenantID != p.TenantID || m.c.ID != id {
		return d.ErrNotFound
	}
	if m.c.Version != version {
		return d.ErrConflict
	}
	m.commits++
	m.c.Version++
	if action == "withdraw" {
		m.c.Status = "withdrawn"
		m.c.ActiveVersionID = ""
	} else {
		if m.v.Status != "approved" || m.v.ValidationDigest != digest || m.v.Version != pv {
			return d.ErrGate
		}
		m.c.ActiveVersionID = pub
		m.c.Status = "active"
	}
	return nil
}

type inputs struct {
	err error
	raw json.RawMessage
}

func (i *inputs) Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	return l.DataAssetRef{}, i.raw, i.err
}

type projection struct {
	baselineID string
	Projection
	err               error
	receipt           json.RawMessage
	prepares, removes int
}

func (p *projection) CurrentLabels(context.Context, l.Principal, string) ([]string, error) {
	return []string{}, nil
}
func (p *projection) Validate(context.Context, l.Principal, string, string, string) error {
	return p.err
}
func (p *projection) Visible(context.Context, l.Principal, string, string, bool) (bool, error) {
	return p.err == nil, p.err
}
func (p *projection) Prepare(context.Context, l.Principal, *d.Case, *d.Publication, string) (json.RawMessage, error) {
	p.prepares++
	return p.receipt, p.err
}
func (p *projection) Lookup(context.Context, l.Principal, *d.Case, *d.Publication) (json.RawMessage, error) {
	return p.receipt, p.err
}
func (p *projection) Remove(context.Context, l.Principal, string, string) error {
	p.removes++
	return nil
}

type evaluations struct {
	Evaluations
	comparison evaluation.Comparison
	err        error
	starts     int
}

func (e *evaluations) FeedbackValidate(context.Context, l.Principal, json.RawMessage) error {
	return e.err
}
func (e *evaluations) FeedbackCompare(context.Context, l.Principal, string, string, json.RawMessage, d.View, d.View, d.Gate) (*evaluation.Comparison, error) {
	return &e.comparison, e.err
}
func (e *evaluations) FeedbackStart(context.Context, l.Principal, string, json.RawMessage, d.View) (string, error) {
	e.starts++
	return "real-id-from-port", e.err
}
func fixture(t *testing.T) (*Service, *memory, *inputs, *projection, *evaluations, l.Principal) {
	t.Helper()
	p := l.Principal{TenantID: 1, UserID: 2}
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	m := &memory{c: &d.Case{ID: uuid.NewString(), TenantID: 1, UserID: 2, Version: 1, Status: "open", BaselineKnowledgeID: "base", BaselineLabels: []string{}, Create: d.Create{BaselineRef: ref, KnowledgeBaseID: "kb", Evidence: d.Evidence{OutputRef: ref, SourceRefs: []l.RefID{ref}}, Budget: d.Budget{MaxCalls: 100, MaxRounds: 2}, Gate: d.Gate{MinImprovement: .05}}, Deadline: time.Now().Add(time.Hour), EvaluationSnapshot: json.RawMessage(`{"qa_pairs":[{},{}]}`)}}
	m.v = &d.Publication{ID: uuid.NewString(), CaseID: m.c.ID, TenantID: 1, UserID: 2, Version: 1, Status: "candidate", CandidateInput: d.CandidateInput{SourceRef: ref, RoundID: "round"}, KnowledgeID: "candidate", ContentDigest: d.Digest("corrected"), Labels: []string{"new"}}
	i := &inputs{raw: json.RawMessage(`{"kind":"text","text":"corrected"}`)}
	pjt := &projection{receipt: json.RawMessage(`{"index":"milvus","state":"ready"}`)}
	ev := &evaluations{comparison: evaluation.Comparison{Gate: "eligible_for_review", Improvement: .2}}
	s, e := NewService(m, i, pjt, ev, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	require.NoError(t, e)
	return s, m, i, pjt, ev, p
}
func TestProjectionUnknownCallOnlyReconciles(t *testing.T) {
	s, m, _, pjt, _, p := fixture(t)
	pjt.receipt = nil
	pjt.err = nil
	v, e := s.Project(context.Background(), p, m.v.ID, 1)
	require.Error(t, e)
	require.Equal(t, "reconcile_required", v.Status)
	require.Equal(t, 1, pjt.prepares)
	require.Equal(t, 34, m.c.ReservedCalls)
	_, e = s.Project(context.Background(), p, m.v.ID, v.Version)
	require.NoError(t, e)
	require.Equal(t, 1, pjt.prepares)
	pjt.receipt = json.RawMessage(`{"index":"milvus"}`)
	v, e = s.Project(context.Background(), p, m.v.ID, v.Version)
	require.NoError(t, e)
	require.Equal(t, "ready", v.Status)
	require.Equal(t, 1, pjt.prepares)
	require.Equal(t, 34, m.c.ReservedCalls)
}
func TestFixedGateAndExplicitApprovalFenceActivation(t *testing.T) {
	s, m, _, _, ev, p := fixture(t)
	ctx := context.Background()
	m.v.Status = "validating"
	m.v.Projection = json.RawMessage(`{"index":"milvus","state":"ready"}`)
	ev.comparison.Gate = "regression"
	v, e := s.Compare(ctx, p, m.v.ID)
	require.NoError(t, e)
	require.Equal(t, "rejected", v.Status)
	_, e = s.Approve(ctx, p, v.ID, v.Version, "review")
	require.ErrorIs(t, e, d.ErrGate)
	ev.comparison.Gate = "eligible_for_review"
	v, e = s.Compare(ctx, p, v.ID)
	require.NoError(t, e)
	require.Equal(t, "validated", v.Status)
	_, e = s.Activate(ctx, p, v.ID, m.c.Version, v.Version)
	require.ErrorIs(t, e, d.ErrGate)
	v, e = s.Approve(ctx, p, v.ID, v.Version, "verified actual pairs")
	require.NoError(t, e)
	c, e := s.Activate(ctx, p, v.ID, m.c.Version, v.Version)
	require.NoError(t, e)
	require.Equal(t, v.ID, c.ActiveVersionID)
	ev.comparison.Improvement = .9
	_, e = s.Activate(ctx, p, v.ID, c.Version, v.Version)
	require.ErrorIs(t, e, d.ErrGate)
}
func TestCandidateNeverVisibleBeforeActivationAndViewScopeIsExact(t *testing.T) {
	s, m, _, _, _, p := fixture(t)
	ctx := a.WithUserID(context.Background(), p.UserID)
	docs := []*rag.Document{{KnowledgeID: "base", KnowledgeBaseID: "kb"}, {KnowledgeID: "candidate", KnowledgeBaseID: "kb"}}
	out, e := s.Filter(ctx, "1", "kb", docs)
	require.NoError(t, e)
	require.Len(t, out, 1)
	require.Equal(t, "base", out[0].KnowledgeID)
	m.v.Projection = json.RawMessage(`{"index":"milvus","state":"ready"}`)
	ctx = d.WithView(ctx, p, candidateView(m.c, m.v))
	out, e = s.Filter(ctx, "1", "", docs)
	require.NoError(t, e)
	require.Len(t, out, 1)
	require.Equal(t, "candidate", out[0].KnowledgeID)
	_, e = s.Filter(ctx, "2", "kb", docs)
	require.ErrorIs(t, e, d.ErrForbidden)
	ctx = d.WithLabels(ctx, []string{"new"})
	out, e = s.Filter(ctx, "1", "kb", docs)
	require.NoError(t, e)
	require.Len(t, out, 1)
	ctx = d.WithLabels(ctx, []string{"not-applied"})
	out, e = s.Filter(ctx, "1", "kb", docs)
	require.NoError(t, e)
	require.Empty(t, out)
}
func TestSourceOutageDoesNotDestructivelyInvalidateButRevocationDoes(t *testing.T) {
	s, m, in, pjt, _, p := fixture(t)
	in.err = errors.New("temporary database outage")
	_, e := s.ReconcileSources(context.Background(), "")
	require.Error(t, e)
	require.Equal(t, "open", m.c.Status)
	require.Zero(t, pjt.removes)
	_, e = s.Get(context.Background(), p, m.c.ID)
	require.Error(t, e)
	in.err = l.ErrForbidden
	_, e = s.ReconcileSources(context.Background(), "")
	require.NoError(t, e)
	require.Equal(t, "invalidated", m.c.Status)
	require.Equal(t, 1, pjt.removes)
}
func TestWithdrawRemainsAvailableAfterRevocationWithoutLeakingSources(t *testing.T) {
	s, m, in, _, _, p := fixture(t)
	in.err = l.ErrForbidden
	m.c.Corrections = []d.Correction{{Ref: m.c.BaselineRef}}
	out, e := s.Withdraw(context.Background(), p, m.c.ID, m.c.Version)
	require.NoError(t, e)
	require.Equal(t, "withdrawn", out.Status)
	require.Empty(t, out.Corrections)
	require.Empty(t, out.Evidence.Question)
	require.Empty(t, out.BaselineKnowledgeID)
	other := p
	other.UserID++
	_, e = s.Withdraw(context.Background(), other, m.c.ID, m.c.Version)
	require.Error(t, e)
}
func TestCandidateReplayKeepsOriginalIDAfterCaseVersionChanges(t *testing.T) {
	s, m, _, _, _, p := fixture(t)
	key := "key"
	m.v.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte("feedback-publication:"+m.c.ID+":"+key)).String()
	m.c.Version = 7
	out, e := s.CreateCandidate(context.Background(), p, m.c.ID, 1, m.v.CandidateInput, key)
	require.NoError(t, e)
	require.Equal(t, m.v.ID, out.ID)
	changed := m.v.CandidateInput
	changed.RoundID = "other"
	_, e = s.CreateCandidate(context.Background(), p, m.c.ID, 1, changed, key)
	require.ErrorIs(t, e, d.ErrConflict)
}
func TestReservationsAreBoundedAndIdempotent(t *testing.T) {
	s, m, _, _, _, p := fixture(t)
	c := clone(m.c)
	require.NoError(t, s.reserve(context.Background(), p, c, "step", 80))
	c = clone(m.c)
	require.NoError(t, s.reserve(context.Background(), p, c, "step", 80))
	require.Equal(t, 80, m.c.ReservedCalls)
	c = clone(m.c)
	require.ErrorIs(t, s.reserve(context.Background(), p, c, "next", 21), d.ErrBudget)
}

func (p *projection) Baseline(context.Context, l.Principal, l.DataAssetRef, string) (string, string, error) {
	return p.baselineID, "digest", nil
}
func TestCandidateCannotBecomeAnotherBaselineLedger(t *testing.T) {
	s, m, _, pjt, _, p := fixture(t)
	pjt.baselineID = m.v.KnowledgeID
	input := d.Create{Title: "shadow", KnowledgeBaseID: "kb", BaselineRef: m.c.BaselineRef, Evidence: d.Evidence{OutputRef: m.c.BaselineRef, SourceRefs: []l.RefID{m.c.BaselineRef}, Question: "q", Problem: "p", Stage: "retrieval"}, DatasetVersionID: "fixed", EvaluationConfig: json.RawMessage(`{"type":"rag","kb_id":"kb"}`), Gate: d.Gate{Metric: "rouge_l", MinPairs: 2, MinImprovement: .1}, Budget: d.Budget{MaxRounds: 2, MaxCalls: 200, TimeoutSeconds: 3600}}
	_, e := s.Create(context.Background(), p, input, "shadow")
	require.ErrorIs(t, e, d.ErrInvalid)
}
func TestRepairCandidateRejectsReplannedCapabilityOrBudget(t *testing.T) {
	var fixed control.PlanSpec
	require.NoError(t, json.Unmarshal([]byte(`{"schema_version":1,"nodes":[{"id":"repair","capability":"lifecycle.data-cleaning","capability_version":1}],"budget":{"max_calls":2}}`), &fixed))
	raw, _ := json.Marshal(fixed)
	require.True(t, sameRepairPlan(raw, fixed))
	var changed control.PlanSpec
	require.NoError(t, json.Unmarshal(raw, &changed))
	changed.Nodes[0].Capability = "sandbox.execute"
	raw, _ = json.Marshal(changed)
	require.False(t, sameRepairPlan(raw, fixed))
	changed = fixed
	changed.Budget.MaxCalls++
	raw, _ = json.Marshal(changed)
	require.False(t, sameRepairPlan(raw, fixed))
}
func TestStoredProjectionDigestDetectsEditedChunkReceipt(t *testing.T) {
	s, m, _, pjt, _, p := fixture(t)
	m.v.Status = "approved"
	m.v.Projection = append([]byte{}, pjt.receipt...)
	m.c.ActiveVersionID = m.v.ID
	pjt.receipt = json.RawMessage(`{"index":"milvus","state":"ready","projection_digest":"edited-content"}`)
	managed, allowed, e := s.KnowledgeAccess(context.Background(), p, "kb", m.v.KnowledgeID)
	require.NoError(t, e)
	require.True(t, managed)
	require.False(t, allowed)
	require.ErrorIs(t, s.CheckView(context.Background(), p, candidateView(m.c, m.v)), d.ErrForbidden)
}

func (p *projection) CurrentChunk(context.Context, l.Principal, *rag.Document) (bool, error) {
	return p.err == nil, p.err
}
