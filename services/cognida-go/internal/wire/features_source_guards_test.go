package wire

import (
	"context"
	"testing"

	agent "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestEvaluationSourceAuthorizationFailsClosedAndBridgesIdentity(t *testing.T) {
	f := &FeatureServices{}
	p := l.Principal{TenantID: 4, UserID: 7}
	require.ErrorIs(t, f.authorizeEvaluationSource(context.Background(), p, "eval"), l.ErrForbidden)
	calls := 0
	f.evaluationSourceRead = func(ctx context.Context, got l.Principal, id string) error {
		calls++
		require.Equal(t, p, got)
		require.Equal(t, p.TenantID, agent.MustGetTenantID(ctx))
		require.Equal(t, p.UserID, agent.MustGetUserID(ctx))
		require.Equal(t, "eval", id)
		return l.ErrForbidden
	}
	require.ErrorIs(t, f.authorizeEvaluationSource(context.Background(), l.Principal{}, "eval"), l.ErrForbidden)
	require.Equal(t, 0, calls)
	// Untrusted/stale typed context cannot override the authenticated principal.
	ctx := agent.WithUserID(agent.WithTenantID(context.Background(), 99), 98)
	require.ErrorIs(t, f.authorizeEvaluationSource(ctx, p, "eval"), l.ErrForbidden)
	require.Equal(t, 1, calls)
}

func TestFeatureDatasourceAvailability(t *testing.T) {
	for _, status := range []string{"active", "error"} {
		require.True(t, availableFeatureDatasource(status))
	}
	for _, status := range []string{"", "inactive", "disabled", "need_credentials", "unknown"} {
		require.False(t, availableFeatureDatasource(status))
	}
}
