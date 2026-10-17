package datasource

import (
	"context"
	"testing"
	"time"

	m "cognida/internal/model/datasource"
	"cognida/internal/service/agent/resultstore"
	"github.com/stretchr/testify/require"
)

func TestResultGuardConnectionIdentityAndLegacyReceipts(t *testing.T) {
	s, repo := newTestService(t)
	ctx := context.Background()
	created, err := s.Create(ctx, 1, 10, validInput("results"))
	require.NoError(t, err)
	source, err := repo.Get(ctx, created.ID, 1)
	require.NoError(t, err)
	guard := s.GuardResults(resultstore.NewMemoryStore())
	owner := resultstore.OwnerKey(1, "synthetic-session")
	put := func(version string) string {
		id, e := guard.Put(ctx, &resultstore.Result{Owner: owner, DatasourceID: source.ID, SourceVersion: version}, time.Minute)
		require.NoError(t, e)
		return id
	}
	current := put(m.QueryConnectionVersion(source))
	legacy := put(source.UpdatedAt.UTC().Format(time.RFC3339Nano))
	exact := guard.(interface {
		GetExact(context.Context, string, string) (*resultstore.Result, error)
	})
	_, err = exact.GetExact(ctx, owner, legacy)
	require.NoError(t, err)
	source.Name, source.Description = "renamed", "display only"
	source.UpdatedAt = source.UpdatedAt.Add(time.Second)
	require.NoError(t, repo.Update(ctx, source))
	_, err = guard.Get(ctx, owner, current)
	require.NoError(t, err)
	_, err = exact.GetExact(ctx, owner, current)
	require.NoError(t, err)
	_, err = guard.Get(ctx, owner, legacy)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized, "old timestamp receipts retain their strict binding")
	_, err = guard.Get(ctx, resultstore.OwnerKey(2, "synthetic-session"), current)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized)
	source.PasswordEncrypted = "different-ciphertext"
	require.NoError(t, repo.Update(ctx, source))
	_, err = guard.Get(ctx, owner, current)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized)
	_, err = exact.GetExact(ctx, owner, current)
	require.ErrorIs(t, err, resultstore.ErrUnauthorized)
}
