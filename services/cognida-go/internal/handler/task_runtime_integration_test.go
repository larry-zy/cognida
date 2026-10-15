//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	svc "cognida/internal/service/taskcontrol"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// This fixture injects domain faults while the HTTP handler, task controller,
// transactions, leases, outbox and reservations use real migrated MySQL.
// Real lifecycle/Python/Redis execution is covered separately by browser E2E.
type runtimeFaultAdapter struct {
	jobs                                        map[string]*svc.DomainObservation
	handles                                     map[string]*svc.ExecutionHandle
	commands                                    map[string]svc.ExecutionCommand
	output                                      l.RefID
	submits, cancels                            int
	loseResponse, absentResponse, invalidInputs bool
	onSubmit                                    func()
}

func newRuntimeFaultAdapter(ref l.RefID) *runtimeFaultAdapter {
	return &runtimeFaultAdapter{jobs: map[string]*svc.DomainObservation{}, handles: map[string]*svc.ExecutionHandle{}, commands: map[string]svc.ExecutionCommand{}, output: ref}
}
func (a *runtimeFaultAdapter) Support() svc.ExecutionSupport {
	return svc.ExecutionSupport{Profile: "test-controlled", BaseCalls: 1, CallsPerInput: 0, MaxTimeoutSeconds: 300}
}
func (a *runtimeFaultAdapter) Validate(context.Context, d.Scope, svc.Node) error { return nil }
func (a *runtimeFaultAdapter) ValidateInputs(context.Context, d.Scope, []l.RefID) error {
	if a.invalidInputs {
		return d.ErrForbidden
	}
	return nil
}
func (a *runtimeFaultAdapter) Submit(_ context.Context, c svc.ExecutionCommand) (*svc.ExecutionHandle, error) {
	a.submits++
	if a.onSubmit != nil {
		a.onSubmit()
	}
	if a.absentResponse {
		a.absentResponse = false
		return nil, errors.New("connection lost before acceptance")
	}
	if h := a.handles[c.OperationKey]; h != nil {
		return h, nil
	}
	h := &svc.ExecutionHandle{DomainID: uuid.NewString(), Receipt: json.RawMessage(`{"accepted":true}`)}
	a.handles[c.OperationKey] = h
	a.commands[c.OperationKey] = c
	a.jobs[c.OperationKey] = &svc.DomainObservation{Status: d.DomainRunning, ObservedAt: time.Now().UTC()}
	if a.loseResponse {
		a.loseResponse = false
		return nil, errors.New("connection lost after durable submit")
	}
	return h, nil
}
func (a *runtimeFaultAdapter) Lookup(_ context.Context, c svc.ExecutionCommand) (*svc.ExecutionHandle, error) {
	return a.handles[c.OperationKey], nil
}
func (a *runtimeFaultAdapter) CancelSubmission(_ context.Context, c svc.ExecutionCommand) (*svc.ExecutionHandle, error) {
	if a.handles[c.OperationKey] == nil {
		a.handles[c.OperationKey] = &svc.ExecutionHandle{DomainID: uuid.NewString(), Receipt: json.RawMessage(`{"cancelled":true}`)}
		a.commands[c.OperationKey] = c
		a.jobs[c.OperationKey] = &svc.DomainObservation{Status: d.DomainCancelled, ObservedAt: time.Now().UTC()}
	}
	return a.handles[c.OperationKey], nil
}
func (a *runtimeFaultAdapter) Inspect(_ context.Context, c svc.ExecutionCommand, _ svc.ExecutionHandle) (*svc.DomainObservation, error) {
	if a.invalidInputs {
		return nil, d.ErrForbidden
	}
	return a.jobs[c.OperationKey], nil
}
func (a *runtimeFaultAdapter) Cancel(_ context.Context, c svc.ExecutionCommand, _ svc.ExecutionHandle) (*svc.DomainObservation, error) {
	a.cancels++
	job := a.jobs[c.OperationKey]
	if job.Status != d.DomainSucceeded && job.Status != d.DomainFailed {
		job.Status = d.DomainCancelled
		job.ObservedAt = time.Now().UTC()
	}
	return job, nil
}
func (a *runtimeFaultAdapter) finish(status string) {
	for _, job := range a.jobs {
		job.Status = status
		job.ObservedAt = time.Now().UTC()
		if status == d.DomainSucceeded {
			job.OutputRefs = []l.RefID{a.output}
		}
	}
}
func runtimeHTTPFixture(t *testing.T) (*supervisorHTTPFixture, *svc.Runtime, *runtimeFaultAdapter) {
	f := newSupervisorHTTPFixture(t)
	f.kinds = map[l.RefID]l.Kind{f.extra: l.CleanedAsset}
	a := newRuntimeFaultAdapter(f.extra)
	r, err := svc.NewRuntime(f.service, f.repo, map[string]svc.ExecutionAdapter{"lifecycle.clean": a}, true)
	require.NoError(t, err)
	f.handler.SetRuntime(r)
	return f, r, a
}
func runtimeSaved(t *testing.T, f *supervisorHTTPFixture) d.Task {
	task := f.draft()
	supervisorDecode[svc.PlanResult](t, f.request(http.MethodPost, "/"+task.ID+"/plans", svc.AppendPlanInput{ExpectedVersion: task.Version, Plan: f.plan}, ""), 201)
	return supervisorDecode[d.Task](t, f.request(http.MethodGet, "/"+task.ID, nil, ""), 200)
}
func runtimeStart(t *testing.T, f *supervisorHTTPFixture, task d.Task) d.Task {
	return supervisorDecode[d.Task](t, f.request(http.MethodPost, "/"+task.ID+"/start", svc.StartInput{ExpectedVersion: task.Version, PlanVersion: task.CurrentPlanVersion}, ""), 200)
}
func runtimeStep(t *testing.T, f *supervisorHTTPFixture, r *svc.Runtime, id string) error {
	t.Helper()
	// Advance the poll schedule for only this fixture; lease expiry tests use real DB time elsewhere.
	require.NoError(t, f.db.Table("task_control_leases").Where("tenant_id=? AND user_id=? AND task_id=?", f.owner.TenantID, f.owner.UserID, id).Update("next_poll_us", 0).Error)
	return r.Reconcile(context.Background(), f.owner, id)
}
func runtimeTask(t *testing.T, f *supervisorHTTPFixture, id string) *d.Task {
	task, err := f.repo.GetTask(context.Background(), f.owner, id)
	require.NoError(t, err)
	return task
}
func TestTaskRuntimeHTTPStartReadOnlyAndTerminal(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	task := runtimeSaved(t, f)
	path := "/" + task.ID
	before := f.counts()
	ready := supervisorDecode[svc.ExecutionReadiness](t, f.request(http.MethodGet, path+"/execution-readiness", nil, ""), 200)
	require.True(t, ready.Allowed)
	require.Equal(t, before, f.counts())
	require.Zero(t, a.submits)
	require.Equal(t, 404, f.request(http.MethodPost, path+"/start", svc.StartInput{ExpectedVersion: task.Version, PlanVersion: 1}, "tenant").Code)
	require.Equal(t, 409, f.request(http.MethodPost, path+"/start", svc.StartInput{ExpectedVersion: task.Version - 1, PlanVersion: 1}, "").Code)
	started := runtimeStart(t, f, task)
	replay := runtimeStart(t, f, task)
	require.Equal(t, started.Version, replay.Version)
	require.Equal(t, started.DeadlineAt, replay.DeadlineAt)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Zero(t, a.submits, "enqueue is distinct from domain submit")
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 1, a.submits)
	summary := supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, path+"/supervisor/summary", nil, ""), 200)
	require.False(t, summary.Terminal)
	require.Equal(t, "running", summary.Nodes[0].Status)
	require.Equal(t, "invocation", summary.Nodes[0].DomainKind)
	require.Equal(t, 1, summary.Usage.ReservedAttempts)
	require.Equal(t, 1, summary.Usage.ReservedCalls)
	a.finish(d.DomainSucceeded)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, d.StatusCompleted, runtimeTask(t, f, task.ID).Status)
	summary = supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, path+"/supervisor/summary", nil, ""), 200)
	require.True(t, summary.Terminal)
	require.False(t, summary.CanCancel)
	require.Equal(t, []l.RefID{f.extra}, summary.ResultRefs)
	replay = runtimeStart(t, f, task)
	require.Equal(t, d.StatusCompleted, replay.Status)
	require.Equal(t, 1, a.submits)
}
func TestTaskRuntimeRecoversUnknownSubmitWithoutDuplicate(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	a.loseResponse = true
	require.Error(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, d.StatusReconcileRequired, runtimeTask(t, f, task.ID).Status)
	restarted, err := svc.NewRuntime(f.service, f.repo, map[string]svc.ExecutionAdapter{"lifecycle.clean": a}, false)
	require.NoError(t, err)
	require.NoError(t, runtimeStep(t, f, restarted, task.ID))
	require.Equal(t, 1, a.submits)
	require.Equal(t, d.StatusRunning, runtimeTask(t, f, task.ID).Status)
	a.finish(d.DomainSucceeded)
	require.NoError(t, runtimeStep(t, f, restarted, task.ID))
	require.Equal(t, d.StatusCompleted, runtimeTask(t, f, task.ID).Status)
}
func TestTaskRuntimeCancelUnknownAbsenceFencesLateSubmit(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	a.absentResponse = true
	require.Error(t, runtimeStep(t, f, r, task.ID))
	current := runtimeTask(t, f, task.ID)
	cancelled := supervisorDecode[d.Task](t, f.request(http.MethodPost, "/"+task.ID+"/cancel", svc.CancelInput{ExpectedVersion: current.Version}, ""), 200)
	require.Equal(t, d.StatusCancelRequested, cancelled.Status)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, d.StatusCancelled, runtimeTask(t, f, task.ID).Status)
	require.Len(t, a.commands, 1)
	for key, cmd := range a.commands {
		h, err := a.Submit(context.Background(), cmd)
		require.NoError(t, err)
		require.Equal(t, a.handles[key], h)
		require.Equal(t, d.DomainCancelled, a.jobs[key].Status)
	}
}
func TestTaskRuntimeCancelDuringSubmitAndRevokedSource(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel races submit", true: "source revoked after submit"}[revoke], func(t *testing.T) {
			f, r, a := runtimeHTTPFixture(t)
			task := runtimeStart(t, f, runtimeSaved(t, f))
			require.NoError(t, runtimeStep(t, f, r, task.ID))
			if !revoke {
				a.onSubmit = func() {
					current := runtimeTask(t, f, task.ID)
					_, err := r.Cancel(context.Background(), f.owner, task.ID, svc.CancelInput{ExpectedVersion: current.Version})
					require.NoError(t, err)
				}
			}
			require.NoError(t, runtimeStep(t, f, r, task.ID))
			if revoke {
				a.invalidInputs = true
				require.NoError(t, runtimeStep(t, f, r, task.ID))
				if !runtimeTask(t, f, task.ID).Status.Terminal() {
					require.NoError(t, runtimeStep(t, f, r, task.ID))
				}
			}
			require.True(t, runtimeTask(t, f, task.ID).Status.Terminal())
			require.Equal(t, 1, a.submits)
			require.Greater(t, a.cancels, 0)
		})
	}
}
func TestTaskRuntimeRetryBudgetAndTimeout(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	f.plan.Nodes[0].MaxAttempts = 2
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	a.finish(d.DomainFailed)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 2, a.submits)
	a.finish(d.DomainFailed)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, d.StatusFailed, runtimeTask(t, f, task.ID).Status)
	children, err := f.repo.ListChildren(context.Background(), f.owner, task.ID)
	require.NoError(t, err)
	require.Len(t, children, 2)
	t.Run("expired deadline never dispatches", func(t *testing.T) {
		f, r, a := runtimeHTTPFixture(t)
		task := runtimeStart(t, f, runtimeSaved(t, f))
		require.NoError(t, f.db.Table("task_control_tasks").Where("id=? AND tenant_id=?", task.ID, f.owner.TenantID).Update("deadline_at", time.Now().Add(-time.Minute)).Error)
		require.NoError(t, runtimeStep(t, f, r, task.ID))
		require.Zero(t, a.submits)
		require.Equal(t, d.StatusTimedOut, runtimeTask(t, f, task.ID).Status)
	})
}
func TestTaskRuntimeReadinessBlocksUnsupportedAndRevokedInputs(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	task := runtimeSaved(t, f)
	a.invalidInputs = true
	ready, err := r.Readiness(context.Background(), f.owner, task.ID)
	require.NoError(t, err)
	require.False(t, ready.Allowed)
	require.Equal(t, "source_not_supported", ready.Blockers[0].Code)
	require.Equal(t, 400, f.request(http.MethodPost, "/"+task.ID+"/start", svc.StartInput{ExpectedVersion: task.Version, PlanVersion: 1}, "").Code)
	require.Zero(t, a.submits)
	disabled, err := svc.NewRuntime(f.service, f.repo, map[string]svc.ExecutionAdapter{"lifecycle.clean": a}, false)
	require.NoError(t, err)
	ready, err = disabled.Readiness(context.Background(), f.owner, task.ID)
	require.NoError(t, err)
	require.False(t, ready.ExecutionEnabled)
	require.Equal(t, "runtime_disabled", ready.Blockers[0].Code)
}

func TestTaskRuntimeParallelDispatchWaitsForAllDependencies(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	f.budget = svc.Budget{MaxNodes: 3, MaxConcurrency: 2, MaxAttempts: 3, MaxCalls: 3, MaxCostMicros: 100, TimeoutSeconds: 60}
	first := f.plan.Nodes[0]
	first.ID = "first"
	second := first
	second.ID = "second"
	third := first
	third.ID = "third"
	third.DependsOn = []string{"first", "second"}
	f.plan = svc.PlanSpec{SchemaVersion: 1, Budget: f.budget, Nodes: []svc.Node{third, first, second}}
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 2, a.submits)
	for key, cmd := range a.commands {
		if cmd.Node.ID == "first" {
			a.jobs[key].Status = d.DomainSucceeded
			a.jobs[key].OutputRefs = []l.RefID{f.extra}
			a.jobs[key].ObservedAt = time.Now().UTC()
		}
	}
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 2, a.submits, "one completed prerequisite cannot dispatch third")
	a.finish(d.DomainSucceeded)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, 3, a.submits)
	a.finish(d.DomainSucceeded)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.Equal(t, d.StatusCompleted, runtimeTask(t, f, task.ID).Status)
}
func TestTaskRuntimeCancellationPreservesSuccessfulPartialFacts(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	f.budget.MaxConcurrency = 2
	f.plan.Budget = f.budget
	other := f.plan.Nodes[0]
	other.ID = "second"
	f.plan.Nodes = append(f.plan.Nodes, other)
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	for key, cmd := range a.commands {
		if cmd.Node.ID == "clean" {
			a.jobs[key].Status = d.DomainSucceeded
			a.jobs[key].OutputRefs = []l.RefID{f.extra}
			a.jobs[key].ObservedAt = time.Now().UTC()
		}
	}
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	current := runtimeTask(t, f, task.ID)
	_, err := r.Cancel(context.Background(), f.owner, task.ID, svc.CancelInput{ExpectedVersion: current.Version})
	require.NoError(t, err)
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	current = runtimeTask(t, f, task.ID)
	require.Equal(t, d.StatusPartiallyCompleted, current.Status)
	var refs []l.RefID
	require.NoError(t, json.Unmarshal(current.ResultRefs, &refs))
	require.Equal(t, []l.RefID{f.extra}, refs)
	summary := supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, "/"+task.ID+"/supervisor/summary", nil, ""), 200)
	require.Equal(t, []l.RefID{f.extra}, summary.ResultRefs)
	a.invalidInputs = true
	summary = supervisorDecode[svc.SupervisorSummary](t, f.request(http.MethodGet, "/"+task.ID+"/supervisor/summary", nil, ""), 200)
	require.Empty(t, summary.ResultRefs, "revoked source chain hides results while preserving historical completion facts")
	require.Equal(t, d.StatusPartiallyCompleted, summary.Status)
}

func TestTaskRuntimeRechecksWholeSourceChainAfterSuccessfulObservation(t *testing.T) {
	f, r, a := runtimeHTTPFixture(t)
	f.budget.MaxConcurrency = 2
	f.plan.Budget = f.budget
	other := f.plan.Nodes[0]
	other.ID = "second"
	f.plan.Nodes = append(f.plan.Nodes, other)
	task := runtimeStart(t, f, runtimeSaved(t, f))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	for key, cmd := range a.commands {
		if cmd.Node.ID == "clean" {
			a.jobs[key].Status = d.DomainSucceeded
			a.jobs[key].OutputRefs = []l.RefID{f.extra}
			a.jobs[key].ObservedAt = time.Now().UTC()
		}
	}
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	// The direct resolver still permits this derived input. Only the adapter's
	// complete provenance check now fails (e.g. an unavailable manual ancestor).
	a.invalidInputs = true
	require.NoError(t, runtimeStep(t, f, r, task.ID))
	current := runtimeTask(t, f, task.ID)
	require.Equal(t, d.StatusPartiallyCompleted, current.Status)
	require.Equal(t, "source_authorization_revoked", current.Reason)
	require.JSONEq(t, `[]`, string(current.ResultRefs))
	require.Equal(t, 2, a.submits)
}
