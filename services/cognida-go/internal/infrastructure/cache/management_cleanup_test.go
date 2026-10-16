package cache

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	d "cognida/internal/model/cache"
	r "cognida/internal/repository/redis"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type cleanupVectorFixture struct {
	d.CacheVectorRepository
	rows      []d.CacheVectorIdentity
	listError error
	delete    func(d.CacheVectorIdentity) (int64, error)
}

func (f *cleanupVectorFixture) ListCleanupCandidates(context.Context, int64, int64, int, int) ([]d.CacheVectorIdentity, error) {
	return f.rows, f.listError
}
func (f *cleanupVectorFixture) DeleteCleanupCandidate(_ context.Context, v d.CacheVectorIdentity) (int64, error) {
	return f.delete(v)
}

func cleanupFixture(t *testing.T) (*CacheManagementService, *cleanupVectorFixture, *r.CacheContentRepository, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	contents := r.NewCacheContentRepository(client)
	vectors := &cleanupVectorFixture{delete: func(d.CacheVectorIdentity) (int64, error) { return 1, nil }}
	return NewCacheManagementService(nil, vectors, contents), vectors, contents, client
}
func oldCache(id string) d.CacheVectorIdentity {
	return d.CacheVectorIdentity{CacheID: id, TenantID: 7, AgentType: "rag", CreatedAt: time.Now().Add(-time.Hour).Unix()}
}
func cacheEntry(v d.CacheVectorIdentity) *d.CacheEntry {
	return &d.CacheEntry{CacheID: v.CacheID, TenantID: v.TenantID, AgentType: v.AgentType, CreatedAt: v.CreatedAt, Response: "synthetic cached response"}
}

func TestOrphanCleanupUsesPrimaryHashesAndCountsActualDeletes(t *testing.T) {
	s, v, c, redis := cleanupFixture(t)
	ctx := context.Background()
	v.rows = []d.CacheVectorIdentity{oldCache("active"), oldCache("orphan"), oldCache("already_deleted")}
	require.NoError(t, c.SetContent(ctx, cacheEntry(v.rows[0]), time.Hour))
	// Secondary indexes are deliberately absent, just as after their TTL expires.
	require.NoError(t, redis.Del(ctx, "tenant:7:caches", "tenant:7:agent:rag:caches").Err())
	var deleted []string
	v.delete = func(row d.CacheVectorIdentity) (int64, error) {
		deleted = append(deleted, row.CacheID)
		if row.CacheID == "already_deleted" {
			return 0, nil
		}
		return 1, nil
	}
	result, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
	require.NoError(t, err)
	require.Equal(t, 3, result.Scanned)
	require.Equal(t, 1, result.Preserved)
	require.EqualValues(t, 1, result.Deleted)
	require.Equal(t, []string{"orphan", "already_deleted"}, deleted)
	for _, row := range v.rows {
		require.NoError(t, c.SetContent(ctx, cacheEntry(row), time.Hour), "successful cleanup releases its exact fence")
	}
}

func TestOrphanCleanupCompletesPreflightBeforeAnyDelete(t *testing.T) {
	for _, mode := range []string{"late page failure", "too many", "wrong tenant", "duplicate", "malformed hash", "wrong type", "redis unavailable", "malformed fence"} {
		t.Run(mode, func(t *testing.T) {
			s, v, _, client := cleanupFixture(t)
			ctx := context.Background()
			v.rows = []d.CacheVectorIdentity{oldCache("orphan"), oldCache("second")}
			req := d.OrphanCleanupRequest{TenantID: 7}
			calls := 0
			v.delete = func(d.CacheVectorIdentity) (int64, error) { calls++; return 1, nil }
			switch mode {
			case "late page failure":
				v.listError = errors.New("page failed")
			case "too many":
				req.MaxVectors = 1
			case "wrong tenant":
				v.rows[1].TenantID = 8
			case "duplicate":
				v.rows[1] = v.rows[0]
			case "malformed hash":
				require.NoError(t, client.HSet(ctx, "semantic_cache:second", "response", "content without ownership").Err())
			case "wrong type":
				require.NoError(t, client.Set(ctx, "semantic_cache:second", "wrong type", 0).Err())
			case "redis unavailable":
				require.NoError(t, client.Close())
			case "malformed fence":
				require.NoError(t, client.Set(ctx, "semantic_cache:cleanup:7:broken", "invalid", 0).Err())
			}
			result, err := s.CleanupOrphanVectors(ctx, req)
			require.Error(t, err)
			require.Zero(t, result.Deleted)
			require.Zero(t, calls)
		})
	}
	s, v, _, _ := cleanupFixture(t)
	v.rows = nil
	result, err := s.CleanupOrphanVectors(context.Background(), d.OrphanCleanupRequest{TenantID: 7})
	require.NoError(t, err)
	require.Zero(t, result.Deleted)
	_, err = s.CleanupOrphanVectors(context.Background(), d.OrphanCleanupRequest{})
	require.ErrorIs(t, err, d.ErrCleanupInvalid)
}

func TestOrphanCleanupInvalidDeleteReceiptAndPartialFailure(t *testing.T) {
	for _, count := range []int64{-1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			s, v, c, _ := cleanupFixture(t)
			ctx := context.Background()
			v.rows = []d.CacheVectorIdentity{oldCache("a"), oldCache("b")}
			v.delete = func(row d.CacheVectorIdentity) (int64, error) {
				if row.CacheID == "a" {
					return 1, nil
				}
				return count, nil
			}
			result, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
			require.ErrorIs(t, err, d.ErrCleanupReconcile)
			require.EqualValues(t, 1, result.Deleted)
			require.NoError(t, c.SetContent(ctx, cacheEntry(v.rows[0]), time.Hour))
			require.ErrorIs(t, c.SetContent(ctx, cacheEntry(v.rows[1]), time.Hour), d.ErrCacheWriteFenced)
		})
	}
}

func TestOrphanCleanupAtomicFenceBlocksLateWriterAndAllowsDefiniteRecovery(t *testing.T) {
	s, v, c, _ := cleanupFixture(t)
	ctx := context.Background()
	row := oldCache("race")
	v.rows = []d.CacheVectorIdentity{row}
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	v.delete = func(d.CacheVectorIdentity) (int64, error) {
		close(entered)
		<-release
		return 0, &d.CacheDeleteError{Cause: errors.New("server rejected"), NotApplied: true}
	}
	go func() { _, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7}); finished <- err }()
	<-entered
	require.ErrorIs(t, c.SetContent(ctx, cacheEntry(row), time.Hour), d.ErrCacheWriteFenced)
	close(release)
	require.Error(t, <-finished)
	require.ErrorIs(t, c.SetContent(ctx, cacheEntry(row), time.Hour), d.ErrCacheWriteFenced)
	v.delete = func(d.CacheVectorIdentity) (int64, error) { return 1, nil }
	result, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Deleted)
	require.NoError(t, c.SetContent(ctx, cacheEntry(row), time.Hour))
}

func TestOrphanCleanupUnknownOutcomeCannotBeReportedAsCompleted(t *testing.T) {
	s, v, c, _ := cleanupFixture(t)
	ctx := context.Background()
	row := oldCache("unknown")
	v.rows = []d.CacheVectorIdentity{row}
	calls := 0
	v.delete = func(d.CacheVectorIdentity) (int64, error) {
		calls++
		return 0, errors.New("connection lost after submit")
	}
	result, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
	require.ErrorIs(t, err, d.ErrCleanupReconcile)
	require.Zero(t, result.Deleted)
	v.rows = nil // deletion may have committed despite the lost acknowledgement
	_, err = s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
	require.ErrorIs(t, err, d.ErrCleanupReconcile)
	require.Equal(t, 1, calls)
	require.ErrorIs(t, c.SetContent(ctx, cacheEntry(row), time.Hour), d.ErrCacheWriteFenced)
	other := row
	other.TenantID = 8
	require.NoError(t, c.SetContent(ctx, cacheEntry(other), time.Hour), "another tenant's marker must not block this owner")
}

func TestOrphanCleanupRecoversDefiniteFailureAfterVectorDisappears(t *testing.T) {
	s, v, c, _ := cleanupFixture(t)
	ctx := context.Background()
	row := oldCache("gone_after_rejection")
	f, err := c.ClaimCleanup(ctx, row)
	require.NoError(t, err)
	require.NoError(t, c.FinishCleanup(ctx, *f, "failed"))
	v.rows = nil
	v.delete = func(actual d.CacheVectorIdentity) (int64, error) { require.Equal(t, row, actual); return 0, nil }
	result, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
	require.NoError(t, err)
	require.Zero(t, result.Deleted)
	require.NoError(t, c.SetContent(ctx, cacheEntry(row), time.Hour))
}

func TestOrphanCleanupPreservesContentAppearingAfterPreflight(t *testing.T) {
	s, v, c, _ := cleanupFixture(t)
	ctx := context.Background()
	row := oldCache("new_content")
	v.rows = []d.CacheVectorIdentity{row}
	s.contentRepo = &appearingContent{CacheContentRepository: c, CacheCleanupContentRepository: c, write: func() { require.NoError(t, c.SetContent(ctx, cacheEntry(row), time.Hour)) }}
	v.delete = func(d.CacheVectorIdentity) (int64, error) { t.Fatal("live content deleted"); return 0, nil }
	result, err := s.CleanupOrphanVectors(ctx, d.OrphanCleanupRequest{TenantID: 7})
	require.NoError(t, err)
	require.Zero(t, result.Deleted)
	require.Equal(t, 1, result.Preserved)
}

type appearingContent struct {
	d.CacheContentRepository
	d.CacheCleanupContentRepository
	write func()
}

func (c *appearingContent) ClaimCleanup(ctx context.Context, v d.CacheVectorIdentity) (*d.CacheCleanupFence, error) {
	c.write()
	return c.CacheCleanupContentRepository.ClaimCleanup(ctx, v)
}
