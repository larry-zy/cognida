package huggingface

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

type governedRepoStub struct {
	d.Repository
	value                          *d.Import
	claim                          bool
	reservations, claims, finishes int
	finishCode                     string
}

func (r *governedRepoStub) ReserveGoverned(_ context.Context, _ l.Principal, _ d.GovernedInvocation, _ bool) (*d.Import, error) {
	r.reservations++
	return r.value, nil
}
func (r *governedRepoStub) LookupGoverned(context.Context, l.Principal, string, string) (*d.Import, error) {
	return r.value, nil
}
func (r *governedRepoStub) ClaimGoverned(context.Context, l.Principal, string) (*d.Import, bool, error) {
	r.claims++
	return r.value, r.claim, nil
}
func (r *governedRepoStub) FinishGoverned(_ context.Context, _ l.Principal, _, _ string, _ *d.Manifest, _ []l.RefID, code string) (*d.Import, error) {
	r.finishes++
	r.finishCode = code
	return r.value, nil
}
func (r *governedRepoStub) PendingGoverned(context.Context, int) ([]d.Import, error) {
	return []d.Import{*r.value}, nil
}

type decoderStub struct{}

func (decoderStub) Decode(context.Context, []byte, string, string, int, d.FieldMapping) (d.DecodeResponse, error) {
	panic("must not decode")
}
func invocation() d.GovernedInvocation {
	return d.GovernedInvocation{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), TaskID: "task", PlanVersion: 1, NodeID: "hf", Request: d.Request{RepoID: "org/data", Revision: strings.Repeat("c", 40), Config: "default", Split: "train", Files: []string{"file.jsonl"}, FieldMapping: d.FieldMapping{TextField: "text"}, MaxRows: 1, MaxBytes: 1024}, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60}
}
func TestGovernedAuthorizationAndRecoveryNeverIssueDownload(t *testing.T) {
	in := invocation()
	p := l.Principal{TenantID: 1, UserID: 2}
	raw, _ := json.Marshal(in.Request)
	repo := &governedRepoStub{value: &d.Import{ID: "receipt", Request: in.Request, RequestHash: digest(raw), Governed: &in, Deadline: in.Deadline, ClaimToken: "claim", Status: "running"}}
	s := &Service{repo: repo, decoder: decoderStub{}, authorize: func(context.Context, l.Principal, string) error { return l.ErrForbidden }}
	_, e := s.SubmitGoverned(context.Background(), p, in)
	require.ErrorIs(t, e, d.ErrForbidden)
	require.Zero(t, repo.reservations)
	_, e = s.LookupGoverned(context.Background(), p, in)
	require.NoError(t, e)
	_, e = s.CancelGoverned(context.Background(), p, in)
	require.NoError(t, e)
	require.Equal(t, 1, repo.reservations)
	changed := in
	changed.Request.MaxRows++
	_, e = s.LookupGoverned(context.Background(), p, changed)
	require.ErrorIs(t, e, d.ErrConflict)
	require.NoError(t, s.ProcessGoverned(context.Background(), p, "receipt"))
	require.Zero(t, repo.finishes, "already claimed work must not repeat")
	repo.claim = true
	require.NoError(t, s.ProcessGoverned(context.Background(), p, "receipt"))
	require.Equal(t, 1, repo.finishes)
	require.Equal(t, "forbidden", repo.finishCode)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := repo.claims
	require.ErrorIs(t, s.Tick(ctx), context.Canceled)
	require.Equal(t, before, repo.claims)
}
func TestGovernedReadAndCancelDoNotDependOnDecoderAvailability(t *testing.T) {
	in := invocation()
	p := l.Principal{TenantID: 1, UserID: 2}
	repo := &governedRepoStub{value: &d.Import{ID: "receipt", Governed: &in, Status: "cancelled"}}
	s := &Service{repo: repo, authorize: func(context.Context, l.Principal, string) error { return nil }}
	require.ErrorIs(t, s.ValidateGovernedRequest(context.Background(), p, in.Request), d.ErrUnavailable)
	_, e := s.CancelGoverned(context.Background(), p, in)
	require.NoError(t, e)
	refs, e := s.GovernedOutputRefs(context.Background(), p, in, repo.value)
	require.NoError(t, e)
	require.Empty(t, refs)
	require.ErrorIs(t, publicError(l.ErrExpired), d.ErrForbidden)
	require.ErrorIs(t, publicError(l.ErrNotFound), d.ErrNotFound)
}
