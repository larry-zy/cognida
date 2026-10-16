//go:build integration

package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	d "cognida/internal/model/taskcontrol"
	svc "cognida/internal/service/taskcontrol"
	"github.com/stretchr/testify/require"
)

func replanFixture(t *testing.T) (*supervisorHTTPFixture, *svc.Runtime, *runtimeFaultAdapter) {
	f, r, a := runtimeHTTPFixture(t)
	next := f.clonePlan().Nodes[0]
	next.ID = "future"
	next.DependsOn = []string{"clean"}
	f.plan.Nodes = append(f.plan.Nodes, next)
	t.Cleanup(func() {
		require.NoError(t, f.db.Table("task_control_replans").Where("tenant_id=? AND user_id=?", f.owner.TenantID, f.owner.UserID).Delete(map[string]any{}).Error)
	})
	return f, r, a
}
func TestTaskRuntimeReplanPreservesReservedChildrenAndBudget(t *testing.T) {
	f, r, a := replanFixture(t)
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	children, e := f.repo.ListChildren(context.Background(), f.owner, task.ID)
	require.NoError(t, e)
	require.Len(t, children, 1)
	old := children[0]
	require.NoError(t, f.db.Table("task_control_leases").Where("task_id=?", task.ID).Update("next_poll_us", 0).Error)
	lease, e := f.repo.ClaimTask(context.Background(), f.owner, task.ID, "replan-old-worker", time.Second*10)
	require.NoError(t, e)
	current := runtimeTask(t, f, task.ID)
	plan := f.clonePlan()
	plan.Nodes[1].ID = "replacement"
	in := svc.ReplanInput{ExpectedVersion: current.Version, Plan: plan, Reason: "Replace only the unreserved future step"}
	path := "/" + task.ID + "/replan"
	saved := supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, path, in, ""), 200)
	require.Equal(t, int64(2), saved.Plan.Version)
	require.Equal(t, task.DeadlineAt, runtimeTask(t, f, task.ID).DeadlineAt)
	_, e = f.repo.RenewTaskLease(context.Background(), f.owner, task.ID, lease.Fence(), time.Second*10)
	require.ErrorIs(t, e, d.ErrConflict)
	replay := supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, path, in, ""), 200)
	require.Equal(t, saved.Plan.Version, replay.Plan.Version)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 1, a.submits)
	children, e = f.repo.ListChildren(context.Background(), f.owner, task.ID)
	require.NoError(t, e)
	require.Equal(t, old.ID, children[0].ID)
	require.Equal(t, old.OperationKey, children[0].OperationKey)
	require.Equal(t, int64(1), children[0].PlanVersion)
	a.finish(d.DomainSucceeded)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 2, a.submits)
	a.finish(d.DomainSucceeded)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, d.StatusCompleted, runtimeTask(t, f, task.ID).Status)
	summary := supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, "/"+task.ID+"/supervisor/summary", nil, ""), 200)
	require.True(t, summary.Terminal)
	require.Equal(t, 2, summary.Usage.ReservedAttempts)
	require.Len(t, summary.Nodes, 2)
	require.Equal(t, "replacement", summary.Nodes[1].NodeID)
	require.Equal(t, 2, a.submits, "old committed child is never resubmitted")
	replay = supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, path, in, ""), 200)
	require.Equal(t, int64(2), replay.Plan.Version)
	f.revoked.Store(true)
	require.Equal(t, http.StatusForbidden, f.request(http.MethodPost, path, in, "").Code)
}
func TestTaskRuntimeReplanRejectsChangedReservedScopeBudgetAndDeadline(t *testing.T) {
	for _, which := range []string{"reserved", "budget", "new-source", "cancelled", "deadline", "stale"} {
		t.Run(which, func(t *testing.T) {
			f, r, _ := replanFixture(t)
			task := runtimeStart(t, f, runtimeSaved(t, f))
			require.NoError(t, runtimeStep(t, f, r, task.ID))
			current := runtimeTask(t, f, task.ID)
			plan := f.clonePlan()
			plan.Nodes[1].ID = "replacement"
			in := svc.ReplanInput{ExpectedVersion: current.Version, Plan: plan, Reason: "future revision"}
			switch which {
			case "reserved":
				in.Plan.Nodes[0].TimeoutSeconds++
			case "budget":
				in.Plan.Budget.MaxCalls++
			case "new-source":
				in.Plan.Nodes[1].Inputs[0].Asset = &f.extra
			case "cancelled":
				_, e := r.Cancel(context.Background(), f.owner, task.ID, svc.CancelInput{ExpectedVersion: current.Version})
				require.NoError(t, e)
			case "deadline":
				require.NoError(t, f.db.Table("task_control_tasks").Where("id=?", task.ID).Update("deadline_at", time.Now().Add(-time.Hour)).Error)
			case "stale":
				in.ExpectedVersion--
			}
			response := f.request(http.MethodPost, "/"+task.ID+"/replan", in, "")
			require.Contains(t, []int{400, 403, 409}, response.Code, response.Body.String())
			require.Equal(t, int64(1), runtimeTask(t, f, task.ID).ExecutionPlanVersion)
			require.Equal(t, http.StatusNotFound, f.request(http.MethodPost, "/"+task.ID+"/replan", in, "user").Code)
		})
	}
}
func TestTaskRuntimeReplanFencesInFlightSubmitButRecoversItsTrueHandle(t *testing.T) {
	f, r, a := replanFixture(t)
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	a.onSubmit = func() {
		a.onSubmit = nil
		current := runtimeTask(t, f, task.ID)
		plan := f.clonePlan()
		plan.Nodes[1].ID = "replacement"
		supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, "/"+task.ID+"/replan", svc.ReplanInput{ExpectedVersion: current.Version, Plan: plan, Reason: "in flight keeps original command"}, ""), 200)
	}
	require.Error(t, runtimeStep(t, f, r, task.ID), "old controller epoch cannot bind after replan")
	require.Equal(t, 1, a.submits)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 1, a.submits, "lookup recovers the accepted operation instead of another submit")
	children, e := f.repo.ListChildren(context.Background(), f.owner, task.ID)
	require.NoError(t, e)
	require.Len(t, children, 1)
	require.Equal(t, d.ChildBound, children[0].Status)
	require.Equal(t, int64(1), children[0].PlanVersion)
	require.NotEmpty(t, children[0].DomainID)
}
