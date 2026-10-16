package scenariotools

import (
	"context"
	"encoding/json"
	"testing"

	a "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
)

func TestToolBoundaryRequiresContextIdentityAndRejectsInjectedFields(t *testing.T) {
	calls := 0
	item := newOperation("fixture", "fixture", func(_ context.Context, p l.Principal, in IDInput) (any, error) {
		calls++
		require.EqualValues(t, 11, p.UserID)
		return in, nil
	}).(tool.InvokableTool)
	_, err := item.InvokableRun(context.Background(), `{"id":"x"}`)
	require.ErrorIs(t, err, l.ErrForbidden)
	ctx := a.WithUserID(a.WithTenantID(context.Background(), 10), 11)
	for _, raw := range []string{`{"id":"x","user_id":12}`, `{"id":"x"} {}`, `[]`} {
		_, err = item.InvokableRun(ctx, raw)
		require.ErrorIs(t, err, l.ErrInvalid)
	}
	require.Zero(t, calls)
	out, err := item.InvokableRun(ctx, `{"id":"x"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"x"}`, out)
	require.Equal(t, 1, calls)
}

func TestStandaloneToolsNeverExposeApprovalAndScopeIdempotency(t *testing.T) {
	names := map[string]bool{}
	for _, item := range New(&governance.Service{}, &lifecycle.Service{}, nil) {
		info, err := item.Info(context.Background())
		require.NoError(t, err)
		require.False(t, names[info.Name])
		names[info.Name] = true
		raw, err := json.Marshal(info)
		require.NoError(t, err)
		require.NotEmpty(t, raw)
	}
	for _, name := range []string{"data_stage_run", "data_prepare", "annotation_create", "annotation_preannotate", "sft_dataset_draft", "feature_job_create"} {
		require.True(t, names[name])
	}
	for _, name := range []string{"annotation_review", "sft_publish", "label_standard_publish"} {
		require.False(t, names[name])
	}
	first, err := operationKey(l.Principal{TenantID: 1, UserID: 1}, "same")
	require.NoError(t, err)
	retry, err := operationKey(l.Principal{TenantID: 1, UserID: 1}, "same")
	require.NoError(t, err)
	require.Equal(t, first, retry)
	other, err := operationKey(l.Principal{TenantID: 1, UserID: 2}, "same")
	require.NoError(t, err)
	require.NotEqual(t, first, other)
}
