package tools

import (
	d "cognida/internal/model/governedops"
	"cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/pendingaction"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGovernedToolsPlanOnlyAndNoLegacyConfirmation(t *testing.T) {
	o, mock, _, _, ctx := setupOperationTest(t)
	calls := 0
	o.cfg.PrepareSQL = func(_ context.Context, sql, key string) (*d.Job, error) {
		calls++
		require.Equal(t, "UPDATE agent_etl_sales SET amount=3 WHERE id=1", sql)
		require.Equal(t, "fixed-key", key)
		return &d.Job{ID: "plan", Plan: d.Plan{Hash: "bound-hash"}}, nil
	}
	req := &SQLMutateRequest{SQL: "UPDATE agent_etl_sales SET amount=3 WHERE id=1", IdempotencyKey: "fixed-key"}
	for i := 0; i < 2; i++ {
		result, e := o.sqlMutate(ctx, req)
		require.NoError(t, e)
		require.Equal(t, "pending_confirm", result.Status)
		require.Equal(t, "plan", result.PlanID)
		require.Equal(t, "bound-hash", result.PlanHash)
		require.Equal(t, "/operations?job_id=plan", result.ConfirmURL)
		require.Zero(t, result.RowsAffected)
	}
	raw, e := o.handleWriteApproval(ctx, framework.ApprovalRequest{Tool: "sql_mutate", Arguments: `{"sql":"UPDATE agent_etl_sales SET amount=3 WHERE id=1","idempotency_key":"fixed-key"}`})
	require.NoError(t, e)
	require.Contains(t, raw, `"plan_id":"plan"`)
	require.NotContains(t, raw, "confirm_token")
	rejected, e := o.ExecuteConfirmedMutation(ctx, &pendingaction.PendingAction{SQL: req.SQL})
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.Status)
	require.Equal(t, 3, calls)
	require.NoError(t, mock.ExpectationsWereMet())
	o.cfg.PrepareSQL = nil
	_, e = o.sqlMutate(ctx, req)
	require.Error(t, e)
}
func TestGovernedETLUsesDurableCallbackWithoutDroppingPriorTable(t *testing.T) {
	o, mock, _, _, ctx := setupOperationTest(t)
	calls := 0
	o.cfg.PrepareETL = func(_ context.Context, target, result, sql, key string, replace bool) (*d.Job, error) {
		calls++
		require.Equal(t, "agent_etl_sales", target)
		require.Empty(t, result)
		require.Equal(t, "SELECT id FROM sales", sql)
		require.Equal(t, "etl-key", key)
		require.True(t, replace)
		return &d.Job{ID: "etl-plan", Plan: d.Plan{Hash: "etl-hash"}}, nil
	}
	req := &ETLRunRequest{TargetTable: "agent_etl_sales", SQL: "SELECT id FROM sales", Replace: true, IdempotencyKey: "etl-key"}
	result, e := o.etlRun(ctx, req)
	require.NoError(t, e)
	require.Equal(t, "pending_confirm", result.Status)
	require.Equal(t, "etl-plan", result.PlanID)
	require.Zero(t, result.RowCount)
	raw, e := o.handleWriteApproval(ctx, framework.ApprovalRequest{Tool: "etl_run", Arguments: `{"target_table":"agent_etl_sales","sql":"SELECT id FROM sales","replace":true,"idempotency_key":"etl-key"}`})
	require.NoError(t, e)
	require.Contains(t, raw, "etl-plan")
	rejected, e := o.ExecuteConfirmedETL(ctx, &pendingaction.PendingAction{})
	require.NoError(t, e)
	require.Equal(t, "rejected", rejected.Status)
	require.Equal(t, 2, calls)
	require.NoError(t, mock.ExpectationsWereMet())
	o.cfg.PrepareETL = func(context.Context, string, string, string, string, bool) (*d.Job, error) {
		return nil, errors.New("source unavailable")
	}
	_, e = o.etlRun(ctx, req)
	require.ErrorContains(t, e, "source unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
	req.IdempotencyKey = ""
	_, e = o.etlRun(ctx, req)
	require.Error(t, e)
}
