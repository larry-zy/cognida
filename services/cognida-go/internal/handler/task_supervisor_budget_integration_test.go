//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	d "cognida/internal/model/taskcontrol"
	svc "cognida/internal/service/taskcontrol"
	"github.com/stretchr/testify/require"
)

// Real MySQL and production HTTP/service/validator, with an explicitly injected
// deterministic planner and source authorizer. No live model or domain work.
func TestTaskSupervisorMySQLHTTPBudgetCompilation(t *testing.T) {
	underestimate := func(p *svc.PlanSpec) {
		p.Nodes[0].MaxAttempts = 2
		p.Nodes[0].MaxCallsPerAttempt = 2
		p.Nodes[0].MaxCostMicrosPerAttempt = 11
		p.Nodes[0].TimeoutSeconds = 15
		p.Budget.MaxAttempts, p.Budget.MaxCalls = 1, 1
		p.Budget.MaxCostMicros, p.Budget.TimeoutSeconds = 0, 1
	}
	t.Run("preview and exact explicit CAS save", func(t *testing.T) {
		f := newSupervisorHTTPFixture(t)
		task := f.draft()
		path := "/" + task.ID
		original := f.clonePlan()
		underestimate(&original)
		f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
			return svc.PlannerDecision{Kind: "proposal", Explanation: "合成节点候选。", Plan: &original}, nil
		}
		before := f.counts()
		proposal := supervisorDecode[svc.SupervisorProposal](t, f.request(http.MethodPost, path+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, ""), http.StatusOK)
		require.Equal(t, int64(1), f.calls.Load())
		require.Equal(t, 1, proposal.PlanningCalls)
		require.False(t, proposal.Saved)
		require.False(t, proposal.ExecutionEnabled)
		require.True(t, proposal.Validation.Valid)
		require.Equal(t, before, f.counts(), "preview cannot persist a plan or schedule work")
		require.Equal(t, svc.Budget{MaxNodes: 2, MaxConcurrency: 1, MaxAttempts: 2, MaxCalls: 4, MaxCostMicros: 22, TimeoutSeconds: 30}, proposal.Decision.Plan.Budget)
		require.Equal(t, original.Nodes, proposal.Decision.Plan.Nodes)
		require.Equal(t, []svc.BudgetAdjustment{
			{Field: "max_attempts", Before: 1, After: 2, Reason: "node_retry_reservations"},
			{Field: "max_calls", Before: 1, After: 4, Reason: "node_retry_reservations"},
			{Field: "max_cost_micros", Before: 0, After: 22, Reason: "node_retry_reservations"},
			{Field: "timeout_seconds", Before: 1, After: 30, Reason: "node_retry_reservations"},
		}, proposal.BudgetAdjustments)
		// The public save endpoint still refuses the uncompiled model arithmetic.
		w := f.request(http.MethodPost, path+"/plans", svc.AppendPlanInput{ExpectedVersion: task.Version, Plan: original}, "")
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "budget_exceeded")
		require.Equal(t, before, f.counts())
		input := svc.AppendPlanInput{ExpectedVersion: task.Version, Plan: *proposal.Decision.Plan}
		saved := supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, path+"/plans", input, ""), http.StatusCreated)
		var persisted svc.PlanSpec
		require.NoError(t, json.Unmarshal(saved.Plan.Snapshot, &persisted))
		require.Equal(t, *proposal.Decision.Plan, persisted)
		stored, err := f.repo.GetPlan(context.Background(), f.owner, task.ID, saved.Plan.Version)
		require.NoError(t, err)
		require.JSONEq(t, string(saved.Plan.Snapshot), string(stored.Snapshot))
		require.Equal(t, http.StatusConflict, f.request(http.MethodPost, path+"/plans", input, "").Code)
		require.Equal(t, int64(1), f.calls.Load())
		f.revoked.Store(true)
		input.ExpectedVersion++
		require.Equal(t, http.StatusForbidden, f.request(http.MethodPost, path+"/plans", input, "").Code)
		counts := f.counts()
		require.Equal(t, int64(1), counts["task_control_plans"])
		for _, table := range []string{"task_control_children", "task_control_outbox", "lifecycle_runs"} {
			require.Zero(t, counts[table], table)
		}
	})
	for _, tc := range []struct {
		name   string
		mutate func(*supervisorHTTPFixture, *svc.PlanSpec)
		status int
		issue  string
	}{
		{"retry reservations exceed task", func(_ *supervisorHTTPFixture, p *svc.PlanSpec) { p.Nodes[0].MaxAttempts = 3 }, 400, "task_budget_exceeded"},
		{"original aggregate exceeds task", func(f *supervisorHTTPFixture, p *svc.PlanSpec) { p.Budget.MaxCalls = f.budget.MaxCalls + 1 }, 400, "task_budget_exceeded"},
		{"unknown capability", func(_ *supervisorHTTPFixture, p *svc.PlanSpec) { p.Nodes[0].Capability = "host.shell" }, 403, ""},
		{"extra permission", func(_ *supervisorHTTPFixture, p *svc.PlanSpec) { p.Nodes[0].Permissions = []string{"read", "write"} }, 400, "scope_mismatch"},
		{"source outside task", func(f *supervisorHTTPFixture, p *svc.PlanSpec) { p.Nodes[0].Inputs[0].Asset = &f.extra }, 400, "task_input_scope"},
		{"revoked during model", func(f *supervisorHTTPFixture, _ *svc.PlanSpec) { f.revoked.Store(true) }, 403, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSupervisorHTTPFixture(t)
			task := f.draft()
			before := f.counts()
			f.planner = func(context.Context, svc.PlanningInput) (svc.PlannerDecision, error) {
				p := f.clonePlan()
				underestimate(&p)
				tc.mutate(f, &p)
				return svc.PlannerDecision{Kind: "proposal", Explanation: "越界或撤权的合成候选。", Plan: &p}, nil
			}
			w := f.request(http.MethodPost, "/"+task.ID+"/supervisor/propose", svc.ProposeInput{ExpectedVersion: task.Version}, "")
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.issue != "" {
				require.Contains(t, w.Body.String(), tc.issue)
			}
			require.Equal(t, int64(1), f.calls.Load())
			require.Equal(t, before, f.counts())
			stored, err := f.repo.GetTask(context.Background(), f.owner, task.ID)
			require.NoError(t, err)
			require.Equal(t, d.StatusDraft, stored.Status)
			require.Equal(t, task.Version, stored.Version)
		})
	}
}
