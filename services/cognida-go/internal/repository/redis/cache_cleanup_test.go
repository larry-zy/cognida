package redis

import (
	"context"
	"testing"
	"time"

	d "cognida/internal/model/cache"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCleanupFenceExactIdentityAndToken(t *testing.T) {
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer c.Close()
	r := NewCacheContentRepository(c)
	ctx := context.Background()
	v := d.CacheVectorIdentity{TenantID: 9007199254740993, CacheID: "scoped", AgentType: "rag", CreatedAt: 9007199254740995}
	f, err := r.ClaimCleanup(ctx, v)
	require.NoError(t, err)
	require.NotNil(t, f)
	listed, err := r.ListCleanupFences(ctx, v.TenantID, 1, 1)
	require.NoError(t, err)
	require.Equal(t, []d.CacheCleanupFence{*f}, listed)
	_, err = r.ClaimCleanup(ctx, v)
	require.ErrorIs(t, err, d.ErrCleanupBusy)
	wrong := *f
	wrong.Token = "other"
	require.ErrorIs(t, r.FinishCleanup(ctx, wrong, "deleted"), d.ErrCleanupReconcile)
	wrong = *f
	wrong.Identity.CreatedAt++
	require.ErrorIs(t, r.FinishCleanup(ctx, wrong, "deleted"), d.ErrCleanupReconcile)
	require.NoError(t, r.FinishCleanup(ctx, *f, "failed"))
	f2, err := r.ClaimCleanup(ctx, v)
	require.NoError(t, err)
	require.NotEqual(t, f.Token, f2.Token)
	require.ErrorIs(t, r.FinishCleanup(ctx, *f, "deleted"), d.ErrCleanupReconcile)
	require.NoError(t, r.FinishCleanup(ctx, *f2, "unknown"))
	_, err = r.ClaimCleanup(ctx, v)
	require.ErrorIs(t, err, d.ErrCleanupReconcile)
	entry := &d.CacheEntry{CacheID: v.CacheID, TenantID: v.TenantID, AgentType: v.AgentType, CreatedAt: 100}
	require.ErrorIs(t, r.SetContent(ctx, entry, time.Hour), d.ErrCacheWriteFenced)
	entry.TenantID++
	require.NoError(t, r.SetContent(ctx, entry, time.Hour))
	_, err = r.CleanupContentPresent(ctx, v)
	require.ErrorIs(t, err, d.ErrCacheOwnership)
}

func TestCleanupFenceEnumerationIsScopedCompleteAndBounded(t *testing.T) {
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer c.Close()
	r := NewCacheContentRepository(c)
	ctx := context.Background()
	for _, v := range []d.CacheVectorIdentity{{TenantID: 7, CacheID: "a", AgentType: "rag", CreatedAt: 100}, {TenantID: 7, CacheID: "b", AgentType: "rag", CreatedAt: 100}, {TenantID: 8, CacheID: "c", AgentType: "rag", CreatedAt: 100}} {
		_, err := r.ClaimCleanup(ctx, v)
		require.NoError(t, err)
	}
	rows, err := r.ListCleanupFences(ctx, 7, 10, 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	rows, err = r.ListCleanupFences(ctx, 7, 1, 1)
	require.ErrorIs(t, err, d.ErrCleanupIncomplete)
	require.Nil(t, rows)
	require.NoError(t, c.Set(ctx, "semantic_cache:cleanup:7:broken", "not-json", 0).Err())
	rows, err = r.ListCleanupFences(ctx, 7, 10, 1)
	require.ErrorIs(t, err, d.ErrCacheOwnership)
	require.Nil(t, rows)
}

func TestCleanupFenceMalformedRecordsFailClosed(t *testing.T) {
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer c.Close()
	r := NewCacheContentRepository(c)
	ctx := context.Background()
	v := d.CacheVectorIdentity{TenantID: 7, CacheID: "scoped", AgentType: "rag", CreatedAt: 100}
	for _, raw := range []string{`broken`, `null`, `[]`, `{"identity":{}}`, `{"identity":{"cache_id":"scoped","tenant_id":"8","agent_type":"rag","created_at":"100"},"state":"failed"}`} {
		require.NoError(t, c.Set(ctx, cleanupFenceKey(v), raw, 0).Err())
		f, err := r.ClaimCleanup(ctx, v)
		require.Error(t, err)
		require.Nil(t, f)
	}
	require.NoError(t, c.Del(ctx, cleanupFenceKey(v)).Err())
	require.NoError(t, c.Set(ctx, r.buildCacheKey(v.CacheID), "wrong type", 0).Err())
	f, err := r.ClaimCleanup(ctx, v)
	require.Error(t, err)
	require.Nil(t, f)
}
