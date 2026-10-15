package mysql

import (
	"context"
	"testing"
	"time"

	d "cognida/internal/model/taskcontrol"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskRuntimeValidationBeforeDatabase(t *testing.T) {
	r := NewTaskControlRepository(nil)
	ctx := context.Background()
	scope := d.Scope{TenantID: 1, UserID: 2}
	id := uuid.NewString()
	_, err := r.BeginExecution(ctx, scope, id, 1, d.StartExecutionInput{})
	require.ErrorIs(t, err, d.ErrInvalid)
	_, err = r.ClaimTask(ctx, scope, id, "worker", time.Hour)
	require.ErrorIs(t, err, d.ErrInvalid)
	_, err = r.ClaimTask(ctx, scope, id, "worker 空格", time.Second)
	require.ErrorIs(t, err, d.ErrInvalid)
	_, err = r.RenewTaskLease(ctx, scope, id, d.ExecutionFence{}, 0)
	require.ErrorIs(t, err, d.ErrInvalid)
	_, err = r.ObserveChild(ctx, scope, id, 1, 1, d.ChildObservation{DomainStatus: "invented"})
	require.ErrorIs(t, err, d.ErrInvalid)
	_, err = r.FinalizeTask(ctx, scope, id, 1, d.TaskUpdate{Status: d.StatusReady})
	require.ErrorIs(t, err, d.ErrInvalid)
	_, err = r.FinalizeTask(ctx, d.Scope{}, id, 1, d.TaskUpdate{Status: d.StatusFailed})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = r.ListRunnable(ctx, d.ListFilter{Limit: 101})
	require.ErrorIs(t, err, d.ErrInvalid)
	require.True(t, tcValidTTL(100*time.Millisecond))
	require.False(t, tcValidTTL(99*time.Millisecond))
	require.False(t, tcValidTTL(6*time.Minute))
	for _, status := range []string{d.DomainSucceeded, d.DomainFailed, d.DomainTimedOut, d.DomainCancelled, d.DomainQuarantined} {
		require.True(t, tcDomainTerminal(status))
		require.True(t, tcDomainValid(status))
	}
	for _, status := range []string{d.DomainUnknown, d.DomainQueued, d.DomainRunning, d.DomainAwaitingApproval} {
		require.False(t, tcDomainTerminal(status))
		require.True(t, tcDomainValid(status))
	}
	require.False(t, tcDomainValid("complete"))
}
func TestRuntimeBudgetHardBounds(t *testing.T) {
	b := tcRuntimeBudget{MaxNodes: 32, MaxConcurrency: 8, MaxAttempts: 160, MaxCalls: 4096, MaxCostMicros: 1_000_000_000, TimeoutSeconds: 86400}
	require.True(t, b.valid())
	for _, mutate := range []func(*tcRuntimeBudget){func(b *tcRuntimeBudget) { b.MaxNodes = 33 }, func(b *tcRuntimeBudget) { b.MaxConcurrency = 9 }, func(b *tcRuntimeBudget) { b.MaxAttempts = 161 }, func(b *tcRuntimeBudget) { b.MaxCalls = 4097 }, func(b *tcRuntimeBudget) { b.MaxCostMicros++ }, func(b *tcRuntimeBudget) { b.TimeoutSeconds++ }} {
		copy := b
		mutate(&copy)
		require.False(t, copy.valid())
	}
	require.True(t, d.CanTransition(d.StatusCancelRequested, d.StatusTimedOut))
	lease := d.TaskLease{TaskID: "task", Owner: "worker", Epoch: 9}
	require.Equal(t, d.ExecutionFence{TaskID: "task", Owner: "worker", Epoch: 9}, lease.Fence())
}
