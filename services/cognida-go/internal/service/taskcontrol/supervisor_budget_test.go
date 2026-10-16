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

func executionBudgetFixture() (PlanSpec, []Capability, Budget) {
	ceiling := Budget{MaxNodes: 4, MaxConcurrency: 2, MaxAttempts: 6, MaxCalls: 30, MaxCostMicros: 0, TimeoutSeconds: 60}
	plan := PlanSpec{SchemaVersion: 1, Budget: ceiling}
	plan.Budget.MaxAttempts, plan.Budget.MaxCalls, plan.Budget.TimeoutSeconds = 4, 16, 40
	ids := []string{"huggingface.import", "lifecycle.data-cleaning", "lifecycle.data-preparation", "governance.sft"}
	counts, base := []int{11, 2, 1, 2}, []int{11, 2, 1, 2}
	var capabilities []Capability
	for i, id := range ids {
		node := Node{ID: []string{"import", "clean", "prepare", "sft"}[i], Capability: id, CapabilityVersion: 1, MaxCallsPerAttempt: counts[i], MaxAttempts: 1, TimeoutSeconds: 10, Tools: []string{id}, Permissions: []string{"read"}, OutputKinds: []l.Kind{l.RawAsset}}
		support := &ExecutionSupport{BaseCalls: base[i], MaxTimeoutSeconds: 120}
		if i != 0 {
			node.Inputs = []InputBinding{{FromNode: plan.Nodes[i-1].ID, Kind: l.RawAsset}}
			node.DependsOn = []string{plan.Nodes[i-1].ID}
			support.CallsPerInput = 1
		}
		plan.Nodes = append(plan.Nodes, node)
		capabilities = append(capabilities, Capability{ID: id, Version: 1, Execution: support})
	}
	return plan, capabilities, ceiling
}

func TestSupervisorCompilesTrustedNodeCallMinimumBeforeAggregate(t *testing.T) {
	p, capabilities, ceiling := executionBudgetFixture()
	before, err := json.Marshal(p)
	require.NoError(t, err)
	compiled, adjustments, err := compileSupervisorBudget(p, ceiling, capabilities...)
	require.NoError(t, err)
	require.Equal(t, 19, compiled.Budget.MaxCalls)
	require.Equal(t, []BudgetAdjustment{
		{NodeID: "clean", Field: "max_calls_per_attempt", Before: 2, After: 3, Reason: "trusted_execution_minimum"},
		{NodeID: "prepare", Field: "max_calls_per_attempt", Before: 1, After: 2, Reason: "trusted_execution_minimum"},
		{NodeID: "sft", Field: "max_calls_per_attempt", Before: 2, After: 3, Reason: "trusted_execution_minimum"},
		{Field: "max_calls", Before: 16, After: 19, Reason: "node_retry_reservations"},
	}, adjustments)
	for i, node := range compiled.Nodes {
		if i == 0 {
			require.Equal(t, 11, node.MaxCallsPerAttempt, "zero-input HF does not reserve a nonexistent input")
		}
		node.MaxCallsPerAttempt = p.Nodes[i].MaxCallsPerAttempt
		require.Equal(t, p.Nodes[i], node, "only call reservations may change")
	}
	after, err := json.Marshal(p)
	require.NoError(t, err)
	require.Equal(t, before, after, "compilation cannot mutate the raw model proposal")
	again, adjustments, err := compileSupervisorBudget(compiled, ceiling, capabilities...)
	require.NoError(t, err)
	require.Equal(t, compiled, again)
	require.Empty(t, adjustments, "already sufficient reservations have no synthetic changes")
}

func TestSupervisorMinimumCallsCountsAllInputsAndRetries(t *testing.T) {
	p, capabilities, ceiling := executionBudgetFixture()
	p.Nodes[1].Inputs = append(p.Nodes[1].Inputs, InputBinding{Asset: &testRef, Kind: l.RawAsset})
	p.Nodes[1].MaxAttempts = 2
	compiled, _, err := compileSupervisorBudget(p, ceiling, capabilities...)
	require.NoError(t, err)
	require.Equal(t, 4, compiled.Nodes[1].MaxCallsPerAttempt)
	require.Equal(t, 5, compiled.Budget.MaxAttempts)
	require.Equal(t, 24, compiled.Budget.MaxCalls, "11 + 2*4 + 2 + 3")
	require.Equal(t, 50, compiled.Budget.TimeoutSeconds, "timeouts themselves remain unchanged")
	ceiling.MaxCalls = 23
	_, adjustments, err := compileSupervisorBudget(p, ceiling, capabilities...)
	require.ErrorIs(t, err, d.ErrInvalid)
	require.Empty(t, adjustments, "an ungranted increase is never exposed as a saveable candidate")
}

func TestSupervisorMinimumCallsNeverGuessesUnsupportedMetadata(t *testing.T) {
	for _, scenario := range []string{"not provided", "planning only", "other version", "unknown capability"} {
		t.Run(scenario, func(t *testing.T) {
			p, capabilities, ceiling := executionBudgetFixture()
			switch scenario {
			case "not provided":
				capabilities = nil
			case "planning only":
				for i := range capabilities {
					capabilities[i].Execution = nil
				}
			case "other version":
				for i := range capabilities {
					capabilities[i].Version++
				}
			case "unknown capability":
				for i := range capabilities {
					capabilities[i].ID = "another.capability"
				}
			}
			compiled, adjustments, err := compileSupervisorBudget(p, ceiling, capabilities...)
			require.NoError(t, err)
			require.Equal(t, p, compiled)
			require.Empty(t, adjustments)
		})
	}
	for _, mutate := range []func(*PlanSpec, []Capability, *Budget){
		func(p *PlanSpec, _ []Capability, b *Budget) { p.Budget.MaxCalls = b.MaxCalls + 1 },
		func(p *PlanSpec, _ []Capability, _ *Budget) { p.Nodes[1].MaxCallsPerAttempt = 0 },
		func(_ *PlanSpec, c []Capability, _ *Budget) { c[1].Execution.BaseCalls = 0 },
		func(_ *PlanSpec, c []Capability, _ *Budget) { c[1].Execution.CallsPerInput = math.MaxInt },
		func(_ *PlanSpec, c []Capability, _ *Budget) { c[1].Execution.BaseCalls = 128 },
	} {
		p, capabilities, ceiling := executionBudgetFixture()
		mutate(&p, capabilities, &ceiling)
		_, adjustments, err := compileSupervisorBudget(p, ceiling, capabilities...)
		require.Error(t, err)
		require.Empty(t, adjustments)
	}
}

func TestSupervisorTrustedCallsProposalIsUnsavedAndStrictlySaved(t *testing.T) {
	s, repo, _ := supervisorFixture(t)
	capability := s.tasks.validator.capabilities["lifecycle.clean"]
	capability.Execution = &ExecutionSupport{Profile: "fixture", BaseCalls: 2, CallsPerInput: 1, MaxTimeoutSeconds: 30}
	s.tasks.validator.capabilities[capability.ID] = capability
	p := testPlan()
	calls := 0
	s.planner = plannerFunc(func(context.Context, PlanningInput) (PlannerDecision, error) {
		calls++
		return PlannerDecision{Kind: "proposal", Explanation: "候选", Plan: &p}, nil
	})
	proposal, err := s.Propose(context.Background(), testScope, "draft", ProposeInput{ExpectedVersion: 1})
	require.NoError(t, err)
	require.Equal(t, 3, proposal.Decision.Plan.Nodes[0].MaxCallsPerAttempt)
	require.Equal(t, []BudgetAdjustment{{NodeID: "clean", Field: "max_calls_per_attempt", Before: 2, After: 3, Reason: "trusted_execution_minimum"}}, proposal.BudgetAdjustments)
	require.True(t, proposal.Validation.Valid)
	require.False(t, proposal.Saved)
	require.Equal(t, 1, calls)
	require.Zero(t, repo.appends)
	_, err = s.tasks.AppendPlan(context.Background(), testScope, "draft", AppendPlanInput{ExpectedVersion: 1, Plan: p})
	require.ErrorIs(t, err, d.ErrInvalid)
	require.Zero(t, repo.appends)
	saved, err := s.tasks.AppendPlan(context.Background(), testScope, "draft", AppendPlanInput{ExpectedVersion: 1, Plan: *proposal.Decision.Plan})
	require.NoError(t, err)
	var persisted PlanSpec
	require.NoError(t, json.Unmarshal(saved.Plan.Snapshot, &persisted))
	require.Equal(t, *proposal.Decision.Plan, persisted)
	require.Equal(t, 1, calls, "explicit save cannot retry the model")
}
