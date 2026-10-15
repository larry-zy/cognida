package taskcontrol

import (
	"context"
	"encoding/json"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

// Any accidental executor/repository call outside the draft API panics through
// the nil embedded interface, rather than falsely simulating job execution.
type draftRepository struct {
	d.Repository
	task             *d.Task
	creates, appends int
}

func (r *draftRepository) CreateTask(_ context.Context, p d.Scope, in d.CreateTaskInput) (*d.Task, error) {
	r.creates++
	return &d.Task{Scope: p, ID: "draft", Goal: in.Goal, InputRefs: in.InputRefs, Budget: in.Budget, Status: d.StatusDraft, Version: 1}, nil
}
func (r *draftRepository) GetTask(_ context.Context, p d.Scope, _ string) (*d.Task, error) {
	if r.task == nil || r.task.Scope != p {
		return nil, d.ErrNotFound
	}
	return r.task, nil
}
func (r *draftRepository) AppendPlan(_ context.Context, _ d.Scope, id string, _ int64, raw json.RawMessage) (*d.Plan, error) {
	r.appends++
	return &d.Plan{TaskID: id, Version: 1, Snapshot: raw}, nil
}
func draftFixture(t *testing.T) (*Service, *draftRepository) {
	t.Helper()
	v, _ := testValidator(t)
	refs, _ := json.Marshal([]l.RefID{testRef})
	budget, _ := json.Marshal(testPlan().Budget)
	repo := &draftRepository{task: &d.Task{Scope: testScope, ID: "draft", Goal: "clean and publish", InputRefs: refs, Budget: budget, Status: d.StatusDraft, Version: 1}}
	s, e := NewService(repo, v)
	require.NoError(t, e)
	return s, repo
}

func TestDraftChecksAuthorizationBeforePersistenceOnEveryReplay(t *testing.T) {
	s, repo := draftFixture(t)
	in := DraftInput{Goal: " clean ", InputRefs: []l.RefID{testRef}, Budget: testPlan().Budget}
	task, e := s.CreateDraft(context.Background(), testScope, in, "key", "rid")
	require.NoError(t, e)
	require.Equal(t, "clean", task.Goal)
	require.Equal(t, d.StatusDraft, task.Status)
	require.Equal(t, 1, repo.creates)
	s.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
	_, e = s.CreateDraft(context.Background(), testScope, in, "key", "rid")
	require.ErrorIs(t, e, d.ErrForbidden)
	require.Equal(t, 1, repo.creates)
}

func TestPlanCannotExpandOriginalTask(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Service, *draftRepository, *AppendPlanInput)
		want   error
	}{
		{"stale", func(_ *Service, _ *draftRepository, in *AppendPlanInput) { in.ExpectedVersion = 2 }, d.ErrConflict},
		{"cancelled", func(_ *Service, r *draftRepository, _ *AppendPlanInput) { r.task.Status = d.StatusCancelled }, d.ErrConflict},
		{"expanded budget", func(_ *Service, _ *draftRepository, in *AppendPlanInput) { in.Plan.Budget.MaxCalls++ }, d.ErrInvalid},
		{"other authorized input", func(s *Service, _ *draftRepository, in *AppendPlanInput) {
			ref := l.RefID{AssetID: "22222222-2222-4222-8222-222222222222", Version: 1}
			in.Plan.Nodes[0].Inputs[0].Asset = &ref
			s.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return l.RawAsset, nil }
		}, d.ErrInvalid},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, r := draftFixture(t)
			in := AppendPlanInput{ExpectedVersion: 1, Plan: testPlan()}
			tt.change(s, r, &in)
			_, e := s.AppendPlan(context.Background(), testScope, "draft", in)
			require.ErrorIs(t, e, tt.want)
			require.Zero(t, r.appends)
		})
	}
}

func TestSavedPlanRemainsReviewableAndDoesNotDispatch(t *testing.T) {
	s, r := draftFixture(t)
	result, e := s.AppendPlan(context.Background(), testScope, "draft", AppendPlanInput{ExpectedVersion: 1, Plan: testPlan()})
	require.NoError(t, e)
	require.Equal(t, 1, r.appends)
	require.False(t, result.Validation.ExecutionEnabled)
	require.Equal(t, d.StatusDraft, r.task.Status)
	var snapshot PlanSpec
	require.NoError(t, json.Unmarshal(result.Plan.Snapshot, &snapshot))
	require.Equal(t, 2, len(snapshot.Nodes))
	require.Equal(t, "lifecycle.publish", snapshot.Nodes[1].Capability)
	_, e = s.AppendPlan(context.Background(), d.Scope{TenantID: 7, UserID: 12}, "draft", AppendPlanInput{ExpectedVersion: 1, Plan: testPlan()})
	require.ErrorIs(t, e, d.ErrNotFound)
}
