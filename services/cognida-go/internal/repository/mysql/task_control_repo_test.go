package mysql

import (
	"context"
	"encoding/json"
	"testing"

	d "cognida/internal/model/taskcontrol"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskControlRejectsMissingScopeBeforeDatabaseAccess(t *testing.T) {
	repo := NewTaskControlRepository(nil)
	ctx := context.Background()
	id := uuid.NewString()
	scope := d.Scope{}
	_, err := repo.CreateTask(ctx, scope, d.CreateTaskInput{})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.GetTask(ctx, scope, id)
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.ListTasks(ctx, scope, d.ListFilter{})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.AppendPlan(ctx, scope, id, 1, json.RawMessage(`{}`))
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.GetPlan(ctx, scope, id, 1)
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.EnqueueChild(ctx, scope, id, 1, d.DispatchInput{})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.GetChild(ctx, scope, id)
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.ListChildren(ctx, scope, id)
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.BindChild(ctx, scope, id, 1, 1, d.Binding{})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.RequestCancel(ctx, scope, id, 1)
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.UpdateTaskStatus(ctx, scope, id, 1, d.TaskUpdate{})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.ListOutbox(ctx, scope, d.OutboxFilter{})
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.ClaimOutbox(ctx, scope, id, 1)
	require.ErrorIs(t, err, d.ErrForbidden)
	_, err = repo.MarkDispatchUnknown(ctx, scope, id, 1, "unknown")
	require.ErrorIs(t, err, d.ErrForbidden)
}
func TestTaskControlPaginationAndDeterministicOperationKey(t *testing.T) {
	_, _, err := tcPage(d.ListFilter{Cursor: "bad"})
	require.ErrorIs(t, err, d.ErrInvalid)
	_, _, err = tcPage(d.ListFilter{Limit: 101})
	require.ErrorIs(t, err, d.ErrInvalid)
	id := uuid.NewString()
	after, limit, err := tcPage(d.ListFilter{Cursor: tcCursor(id)})
	require.NoError(t, err)
	require.Equal(t, id, after)
	require.Equal(t, 20, limit)
	scope := d.Scope{TenantID: 1, UserID: 2}
	in := d.DispatchInput{PlanVersion: 1, NodeID: "clean", Attempt: 1, Domain: "lifecycle", Payload: json.RawMessage(`{"amount":"9007199254740993.1"}`)}
	_, key, hash, err := tcDispatch(scope, id, in)
	require.NoError(t, err)
	_, key2, hash2, err := tcDispatch(scope, id, in)
	require.NoError(t, err)
	require.Equal(t, key, key2)
	require.Equal(t, hash, hash2)
	in.Payload = json.RawMessage(`{"amount":"9007199254740993.2"}`)
	_, key2, hash2, err = tcDispatch(scope, id, in)
	require.NoError(t, err)
	require.Equal(t, key, key2)
	require.NotEqual(t, hash, hash2)
	in.Attempt = 2
	_, key2, _, err = tcDispatch(scope, id, in)
	require.NoError(t, err)
	require.NotEqual(t, key, key2)
	scope.UserID++
	_, key3, _, err := tcDispatch(scope, id, in)
	require.NoError(t, err)
	require.NotEqual(t, key2, key3)
	in.NodeID = "越界"
	_, _, _, err = tcDispatch(scope, id, in)
	require.ErrorIs(t, err, d.ErrInvalid)
}
