//go:build integration

package mysql

import (
	"cognida/internal/model/semantic"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestManagedSemanticListScopesStatesAndCursor(t *testing.T) {
	db := newIntegrationDB(t)
	r := NewSemanticRepository(db)
	ctx := context.Background()
	prefix := "list_" + uuid.NewString()[:8]
	ids := []string{prefix + "1", prefix + "2", prefix + "3", prefix + "4"}
	t.Cleanup(func() { db.Where("id IN ?", ids).Delete(&SemanticModelModel{}) })
	for i, status := range []semantic.ModelStatus{semantic.ModelStatusDraft, semantic.ModelStatusActive, semantic.ModelStatusDeprecated, semantic.ModelStatusDraft} {
		tenant := int64(990019)
		if i == 3 {
			tenant++
		}
		require.NoError(t, r.UpsertBundle(ctx, &semantic.ModelBundle{Model: &semantic.SemanticModel{ID: ids[i], TenantID: tenant, Name: ids[i], Status: status, Version: 1}}))
	}
	items, next, err := r.ListModels(ctx, 990019, "all", "", 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.NotEmpty(t, next)
	tail, next, err := r.ListModels(ctx, 990019, "all", next, 2)
	require.NoError(t, err)
	require.Len(t, tail, 1)
	require.Empty(t, next)
	require.Equal(t, ids[2], tail[0].ID)
	drafts, _, err := r.ListModels(ctx, 990019, "draft", "", 20)
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	require.Equal(t, ids[0], drafts[0].ID)
	_, _, err = r.ListModels(ctx, 990019, "all", "bad-cursor", 20)
	require.Error(t, err)
}
