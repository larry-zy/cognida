package taskcontrol

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/stretchr/testify/require"
)

func underestimatedPlan() PlanSpec {
	p := testPlan()
	p.Budget.MaxAttempts, p.Budget.MaxCalls = 1, 1
	p.Budget.MaxCostMicros, p.Budget.TimeoutSeconds = 0, 1
	return p
}

func TestSupervisorBudgetCompilationPreservesNodesAndCeilings(t *testing.T) {
	p := underestimatedPlan()
	before, err := json.Marshal(p)
	require.NoError(t, err)
	compiled, adjustments, err := compileSupervisorBudget(p, testPlan().Budget)
	require.NoError(t, err)
	require.Equal(t, Budget{MaxNodes: 4, MaxConcurrency: 2, MaxAttempts: 3, MaxCalls: 5, MaxCostMicros: 30, TimeoutSeconds: 30}, compiled.Budget)
	require.Equal(t, []BudgetAdjustment{
		{Field: "max_attempts", Before: 1, After: 3, Reason: "node_retry_reservations"},
		{Field: "max_calls", Before: 1, After: 5, Reason: "node_retry_reservations"},
		{Field: "max_cost_micros", Before: 0, After: 30, Reason: "node_retry_reservations"},
		{Field: "timeout_seconds", Before: 1, After: 30, Reason: "node_retry_reservations"},
	}, adjustments)
	require.Equal(t, p.Nodes, compiled.Nodes)
	after, err := json.Marshal(p)
	require.NoError(t, err)
	require.Equal(t, before, after, "the planner's candidate must not be mutated")
	v, _ := testValidator(t)
	validation, err := v.Validate(context.Background(), testScope, compiled)
	require.NoError(t, err)
	require.Equal(t, compiled.Budget.MaxAttempts, validation.ReservedAttempts)
	require.Equal(t, compiled.Budget.MaxCalls, validation.ReservedCalls)
	require.Equal(t, compiled.Budget.MaxCostMicros, validation.ReservedCostMicros)
	require.Equal(t, compiled.Budget.TimeoutSeconds, validation.ReservedSeconds)
	// Exact reservations and overestimates are retained; zero cost is legitimate.
	for i := range p.Nodes {
		p.Nodes[i].MaxCostMicrosPerAttempt = 0
	}
	p.Budget = testPlan().Budget
	p.Budget.MaxCostMicros = 0
	compiled, adjustments, err = compileSupervisorBudget(p, testPlan().Budget)
	require.NoError(t, err)
	require.Equal(t, p, compiled)
	require.Empty(t, adjustments)
}

func TestSupervisorBudgetCompilationRejectsUnauthorizedOrInvalidArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*PlanSpec, *Budget)
		want   error
	}{
		{"attempt ceiling", func(_ *PlanSpec, b *Budget) { b.MaxAttempts = 2 }, d.ErrInvalid},
		{"call ceiling", func(_ *PlanSpec, b *Budget) { b.MaxCalls = 4 }, d.ErrInvalid},
		{"cost ceiling", func(_ *PlanSpec, b *Budget) { b.MaxCostMicros = 29 }, d.ErrInvalid},
		{"time ceiling", func(_ *PlanSpec, b *Budget) { b.TimeoutSeconds = 29 }, d.ErrInvalid},
		{"already expanded model budget", func(p *PlanSpec, b *Budget) { p.Budget.MaxCalls = b.MaxCalls + 1 }, d.ErrInvalid},
		{"node count unchanged", func(p *PlanSpec, _ *Budget) { p.Budget.MaxNodes = 1; p.Budget.MaxConcurrency = 1 }, ErrPlannerOutput},
		{"concurrency not clamped", func(p *PlanSpec, _ *Budget) { p.Budget.MaxConcurrency = 3 }, d.ErrInvalid},
		{"attempt overflow", func(p *PlanSpec, _ *Budget) { p.Nodes[0].MaxAttempts = math.MaxInt }, ErrPlannerOutput},
		{"call overflow", func(p *PlanSpec, _ *Budget) { p.Nodes[0].MaxCallsPerAttempt = math.MaxInt }, ErrPlannerOutput},
		{"cost overflow", func(p *PlanSpec, _ *Budget) { p.Nodes[0].MaxCostMicrosPerAttempt = math.MaxInt64 }, ErrPlannerOutput},
		{"time overflow", func(p *PlanSpec, _ *Budget) { p.Nodes[0].TimeoutSeconds = math.MaxInt }, ErrPlannerOutput},
		{"negative cost", func(p *PlanSpec, _ *Budget) { p.Nodes[0].MaxCostMicrosPerAttempt = -1 }, ErrPlannerOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, ceiling := underestimatedPlan(), testPlan().Budget
			tc.change(&p, &ceiling)
			_, adjustments, err := compileSupervisorBudget(p, ceiling)
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, adjustments)
			if tc.want == d.ErrInvalid {
				var validation *ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, "task_budget_exceeded", validation.Issues[0].Code)
			}
		})
	}
	var total nodeReservations
	maxNode := Node{MaxAttempts: 5, MaxCallsPerAttempt: 128, MaxCostMicrosPerAttempt: 1_000_000_000, TimeoutSeconds: 3600}
	for i := 0; i < 32; i++ {
		require.True(t, total.add(maxNode))
	}
	require.Equal(t, 160, total.attempts)
	require.Equal(t, 20480, total.calls)
	require.EqualValues(t, 160_000_000_000, total.costMicros)
	require.Equal(t, 576000, total.seconds)
	before := total
	require.False(t, total.add(maxNode))
	require.Equal(t, before, total)
}

func TestSupervisorCompiledPlanStillRequiresAuthorityAndExplicitSave(t *testing.T) {
	s, r, _ := supervisorFixture(t)
	p := underestimatedPlan()
	calls := 0
	s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
		calls++
		return PlannerDecision{Kind: "proposal", Explanation: "候选清洗和审批节点。", Plan: &p}, nil
	})
	out, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, out.BudgetAdjustments, 4)
	require.False(t, out.Saved)
	require.False(t, out.ExecutionEnabled)
	require.Zero(t, r.appends)
	_, err = s.tasks.AppendPlan(context.Background(), testScope, "draft", AppendPlanInput{ExpectedVersion: 1, Plan: p})
	require.ErrorIs(t, err, d.ErrInvalid, "direct plan saving must retain strict budget validation")
	require.Zero(t, r.appends)
	saved, err := s.tasks.AppendPlan(context.Background(), testScope, "draft", AppendPlanInput{ExpectedVersion: 1, Plan: *out.Decision.Plan})
	require.NoError(t, err)
	var persisted PlanSpec
	require.NoError(t, json.Unmarshal(saved.Plan.Snapshot, &persisted))
	require.Equal(t, *out.Decision.Plan, persisted)
	require.Equal(t, p.Nodes, persisted.Nodes)
	for _, tc := range []struct {
		name   string
		change func(*Supervisor, *PlanSpec)
	}{
		{"unknown capability", func(_ *Supervisor, p *PlanSpec) { p.Nodes[0].Capability = "host.shell" }},
		{"permission revoked after model", func(s *Supervisor, _ *PlanSpec) {
			s.tasks.validator.authorize = func(_ context.Context, _ d.Scope, permission string) error {
				if permission == "write" {
					return d.ErrForbidden
				}
				return nil
			}
		}},
		{"source revoked after model", func(s *Supervisor, _ *PlanSpec) {
			s.tasks.validator.resolve = func(context.Context, d.Scope, l.RefID) (l.Kind, error) { return "", d.ErrForbidden }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, _ := supervisorFixture(t)
			s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
				p := underestimatedPlan()
				tc.change(s, &p)
				return PlannerDecision{Kind: "proposal", Explanation: "候选", Plan: &p}, nil
			})
			out, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
			require.ErrorIs(t, err, d.ErrForbidden)
			require.Nil(t, out)
			require.Zero(t, r.appends)
		})
	}
}
