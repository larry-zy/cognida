package taskcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

type plannerFunc func(context.Context, PlanningInput) (PlannerDecision, error)

func (f plannerFunc) Propose(ctx context.Context, in PlanningInput) (PlannerDecision, error) {
	return f(ctx, in)
}

type supervisorRepository struct {
	*draftRepository
	children []d.Child
	plan     *d.Plan
}

func (r *supervisorRepository) GetTask(ctx context.Context, p d.Scope, id string) (*d.Task, error) {
	task, err := r.draftRepository.GetTask(ctx, p, id)
	if err != nil {
		return nil, err
	}
	copy := *task
	return &copy, nil
}
func (r *supervisorRepository) ListChildren(context.Context, d.Scope, string) ([]d.Child, error) {
	return r.children, nil
}
func (r *supervisorRepository) GetPlan(context.Context, d.Scope, string, int64) (*d.Plan, error) {
	return r.plan, nil
}

func supervisorFixture(t *testing.T) (*Supervisor, *supervisorRepository, *int) {
	t.Helper()
	tasks, repo := draftFixture(t)
	r := &supervisorRepository{draftRepository: repo}
	r.task.ResultRefs = json.RawMessage(`[]`)
	tasks.repo = r
	calls := 0
	s, err := NewSupervisor(tasks, plannerFunc(func(_ context.Context, in PlanningInput) (PlannerDecision, error) {
		calls++
		require.Equal(t, repo.task.Goal, in.Goal)
		require.Equal(t, []SupervisorSource{{Ref: testRef, Kind: l.RawAsset}}, in.Sources)
		plan := testPlan()
		return PlannerDecision{Kind: "proposal", Explanation: "清洗后请求发布审批。", Plan: &plan}, nil
	}))
	require.NoError(t, err)
	return s, r, &calls
}

func TestSupervisorClarifiesMissingInputsWithoutModelOrPersistence(t *testing.T) {
	s, r, calls := supervisorFixture(t)
	result, err := s.Assess(context.Background(), testScope, AssessmentInput{})
	require.NoError(t, err)
	require.False(t, result.Ready)
	require.False(t, result.ExecutionEnabled)
	require.Len(t, result.Questions, 3)
	require.Zero(t, *calls)
	require.Zero(t, r.creates)
	budget := testPlan().Budget
	result, err = s.Assess(context.Background(), testScope, AssessmentInput{Goal: "清洗数据", InputRefs: []l.RefID{testRef}, Budget: &budget})
	require.NoError(t, err)
	require.True(t, result.Ready)
	require.Empty(t, result.Questions)
	s.tasks.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
	_, err = s.Assess(context.Background(), testScope, AssessmentInput{InputRefs: []l.RefID{testRef}})
	require.ErrorIs(t, err, d.ErrForbidden)
}

func TestSupervisorProposalRequiresExplicitSave(t *testing.T) {
	s, r, calls := supervisorFixture(t)
	out, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.NoError(t, err)
	require.Equal(t, 1, *calls)
	require.Equal(t, 1, out.PlanningCalls)
	require.False(t, out.Saved)
	require.False(t, out.ExecutionEnabled)
	require.True(t, out.Validation.Valid)
	require.Equal(t, []string{"publish"}, out.Validation.ApprovalRequired)
	require.EqualValues(t, 1, r.task.Version)
	require.Equal(t, d.StatusDraft, r.task.Status)
	require.Zero(t, r.appends)
	require.Zero(t, r.creates)
	_, err = s.tasks.AppendPlan(context.Background(), testScope, "draft", AppendPlanInput{ExpectedVersion: out.TaskVersion, Plan: *out.Decision.Plan})
	require.NoError(t, err)
	require.Equal(t, 1, r.appends)
}

func TestSupervisorRejectsStaleOrDispatchedTasksBeforeModel(t *testing.T) {
	for _, change := range []func(*supervisorRepository){
		func(r *supervisorRepository) { r.task.Version++ },
		func(r *supervisorRepository) { r.task.Status = d.StatusCancelled },
		func(r *supervisorRepository) { r.children = []d.Child{{Status: d.ChildPending}} },
	} {
		s, r, calls := supervisorFixture(t)
		change(r)
		_, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
		require.ErrorIs(t, err, d.ErrConflict)
		require.Zero(t, *calls)
	}
	s, _, calls := supervisorFixture(t)
	_, err := s.Propose(context.Background(), d.Scope{TenantID: 8, UserID: 11}, "draft", ProposeInput{ExpectedVersion: 1})
	require.ErrorIs(t, err, d.ErrNotFound)
	require.Zero(t, *calls)
}

func TestSupervisorRevalidatesUntrustedPlanAndCurrentAuthority(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Supervisor, *supervisorRepository, *PlanSpec)
		want   error
	}{
		{"other root", func(_ *Supervisor, _ *supervisorRepository, p *PlanSpec) {
			ref := testRef
			ref.Version++
			p.Nodes[0].Inputs[0].Asset = &ref
		}, d.ErrInvalid},
		{"expanded budget", func(_ *Supervisor, _ *supervisorRepository, p *PlanSpec) { p.Budget.MaxCostMicros++ }, d.ErrInvalid},
		{"cycle", func(_ *Supervisor, _ *supervisorRepository, p *PlanSpec) { p.Nodes[0].DependsOn = []string{"publish"} }, d.ErrInvalid},
		{"tool escalation", func(_ *Supervisor, _ *supervisorRepository, p *PlanSpec) {
			p.Nodes[0].Tools = append(p.Nodes[0].Tools, "shell")
		}, d.ErrInvalid},
		{"unknown capability", func(_ *Supervisor, _ *supervisorRepository, p *PlanSpec) { p.Nodes[0].Capability = "unknown" }, d.ErrForbidden},
		{"task changed", func(_ *Supervisor, r *supervisorRepository, _ *PlanSpec) { r.task.Version++ }, d.ErrConflict},
		{"source revoked", func(s *Supervisor, _ *supervisorRepository, _ *PlanSpec) {
			s.tasks.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
		}, d.ErrForbidden},
		{"write revoked", func(s *Supervisor, _ *supervisorRepository, _ *PlanSpec) {
			s.tasks.validator.authorize = func(_ context.Context, _ d.Scope, scope string) error {
				if scope == "write" {
					return d.ErrForbidden
				}
				return nil
			}
		}, d.ErrForbidden},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s, r, _ := supervisorFixture(t)
			s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
				p := testPlan()
				tt.change(s, r, &p)
				return PlannerDecision{Kind: "proposal", Explanation: "候选", Plan: &p}, nil
			})
			_, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
			require.ErrorIs(t, err, tt.want)
			require.Zero(t, r.appends)
		})
	}
}

func TestSupervisorClarificationCarriesQuestionAndAnswerWithoutApproval(t *testing.T) {
	s, r, _ := supervisorFixture(t)
	s.tasks.validator.authorize = func(_ context.Context, _ d.Scope, scope string) error {
		if scope == "write" {
			return d.ErrForbidden
		}
		return nil
	}
	answer := SupervisorAnswer{QuestionID: "q1", Question: "需要什么产物？", Answer: "清洗结果即可，不发布"}
	s.planner = plannerFunc(func(_ context.Context, in PlanningInput) (PlannerDecision, error) {
		require.Equal(t, []SupervisorAnswer{answer}, in.Answers)
		require.Len(t, in.Capabilities, 1)
		return PlannerDecision{Kind: "clarify", Explanation: "需要确认清洗范围", Questions: []SupervisorQuestion{{ID: "q2", Question: "哪些列需要清洗？"}}}, nil
	})
	out, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1, Answers: []SupervisorAnswer{answer}})
	require.NoError(t, err)
	require.Equal(t, "clarify", out.Decision.Kind)
	require.Nil(t, out.Validation)
	require.Zero(t, r.appends)
	s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
		return PlannerDecision{Kind: "clarify", Explanation: "重复问题", Questions: []SupervisorQuestion{{ID: "q1", Question: "再次询问"}}}, nil
	})
	_, err = s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1, Answers: []SupervisorAnswer{answer}})
	require.ErrorIs(t, err, ErrPlannerOutput)
	answer.Question = ""
	_, err = s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1, Answers: []SupervisorAnswer{answer}})
	require.ErrorIs(t, err, d.ErrInvalid)
}

func TestSupervisorSummaryDoesNotInventDomainSuccessOrExposeReceipts(t *testing.T) {
	s, r, calls := supervisorFixture(t)
	r.children = []d.Child{{ID: "child", NodeID: "clean", Status: d.ChildBound, Domain: "lifecycle", DomainID: "real-run", Receipt: json.RawMessage(`{"private_token":"do-not-expose"}`)}}
	r.task.Status = d.StatusCancelRequested
	r.task.CurrentPlanVersion = 1
	raw, _ := json.Marshal(testPlan())
	r.plan = &d.Plan{Snapshot: raw, Version: 1}
	refs, _ := json.Marshal([]l.RefID{testRef})
	r.task.ResultRefs = refs
	s.tasks.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
	out, err := s.Summarize(context.Background(), testScope, "draft")
	require.NoError(t, err)
	require.Zero(t, *calls)
	require.Equal(t, d.StatusCancelRequested, out.Status)
	require.Equal(t, "unknown", out.Children[0].DomainStatus)
	require.True(t, out.Children[0].ReceiptPresent)
	require.Equal(t, 2, out.PlannedNodes)
	require.Empty(t, out.ResultRefs)
	require.Len(t, out.Warnings, 3)
	encoded, _ := json.Marshal(out)
	require.NotContains(t, string(encoded), "do-not-expose")
}

func TestSupervisorRejectsPlannerFailureAndMixedDecision(t *testing.T) {
	s, r, _ := supervisorFixture(t)
	s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
		return PlannerDecision{}, ErrPlannerUnavailable
	})
	_, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.ErrorIs(t, err, ErrPlannerUnavailable)
	s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
		p := testPlan()
		return PlannerDecision{Kind: "clarify", Explanation: "bad", Plan: &p}, nil
	})
	_, err = s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.True(t, errors.Is(err, ErrPlannerOutput))
	require.Zero(t, r.appends)
}
