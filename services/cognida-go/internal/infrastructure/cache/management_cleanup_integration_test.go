//go:build integration

package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	d "cognida/internal/model/cache"
	m "cognida/internal/repository/milvus"
	r "cognida/internal/repository/redis"
	"github.com/google/uuid"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// This test uses only explicitly configured test services and a random owned
// collection. Fault tests wrap the real repository, not the remote protocol.
func TestOrphanCleanupRedisMilvusIntegration(t *testing.T) {
	ra, ma := os.Getenv("COGNIDA_CACHE_TEST_REDIS_ADDR"), os.Getenv("COGNIDA_CACHE_TEST_MILVUS_ADDR")
	if ra == "" || ma == "" {
		t.Skip("explicit isolated Redis and Milvus test addresses required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	redisClient := redis.NewClient(&redis.Options{Addr: ra, Password: os.Getenv("COGNIDA_CACHE_TEST_REDIS_PASSWORD")})
	defer redisClient.Close()
	require.NoError(t, redisClient.Ping(ctx).Err())
	client, err := milvusclient.New(ctx, &milvusclient.ClientConfig{Address: ma, APIKey: os.Getenv("COGNIDA_CACHE_TEST_MILVUS_TOKEN")})
	require.NoError(t, err)
	defer client.Close(ctx)
	prefix := strings.ReplaceAll(uuid.NewString(), "-", "")
	name := "cognida_cache_review_" + prefix
	vectors, err := m.NewCacheVectorRepositoryForCollection(client, name, 4)
	require.NoError(t, err)
	require.NoError(t, vectors.CreateCacheCollection(ctx))
	var keys []string
	defer func() {
		clean, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		require.NoError(t, client.DropCollection(clean, milvusclient.NewDropCollectionOption(name)))
		if len(keys) > 0 {
			require.NoError(t, redisClient.Del(clean, keys...).Err())
		}
	}()
	idx, err := client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(name, "vector", index.NewFlatIndex(entity.COSINE)))
	require.NoError(t, err)
	require.NoError(t, idx.Await(ctx))
	require.NoError(t, vectors.LoadCacheCollection(ctx))
	tenant := int64(time.Now().UnixNano()%1000000000000 + 1000000000000)
	contents := r.NewCacheContentRepository(redisClient)
	s := NewCacheManagementService(nil, vectors, contents)
	add := func(label string, tid int64, old bool, live bool) d.CacheVectorIdentity {
		at := time.Now().Unix()
		if old {
			at -= 3600
		}
		v := d.CacheVectorIdentity{CacheID: prefix + "_" + label, TenantID: tid, AgentType: "rag", CreatedAt: at}
		entry := cacheEntry(v)
		entry.Vector = []float32{1, 0, 0, 0}
		// Strong queries read the growing segment. Avoid the unrelated legacy
		// Insert helper's per-row Flush, which is rate-limited by real Milvus.
		_, insertErr := client.Insert(ctx, milvusclient.NewColumnBasedInsertOption(name).
			WithColumns(column.NewColumnVarChar("cache_id", []string{v.CacheID}),
				column.NewColumnFloatVector("vector", 4, [][]float32{entry.Vector}),
				column.NewColumnInt64("tenant_id", []int64{tid}),
				column.NewColumnVarChar("agent_type", []string{"rag"}),
				column.NewColumnVarChar("kb_scope", []string{""}),
				column.NewColumnInt64("created_at", []int64{at})))
		require.NoError(t, insertErr)
		if live {
			require.NoError(t, contents.SetContent(ctx, entry, time.Hour))
		}
		keys = append(keys, "semantic_cache:"+v.CacheID, fmt.Sprintf("semantic_cache:cleanup:%d:%s", tid, v.CacheID), fmt.Sprintf("tenant:%d:caches", tid), fmt.Sprintf("tenant:%d:agent:rag:caches", tid))
		return v
	}
	active := add("active", tenant, true, true)
	expired := add("expired", tenant, true, true)
	orphan := add("orphan", tenant, true, false)
	fresh := add("fresh", tenant, false, false)
	other := add("other", tenant+1, true, false)
	require.NoError(t, redisClient.PExpire(ctx, "semantic_cache:"+expired.CacheID, time.Millisecond).Err())
	require.Eventually(t, func() bool { return redisClient.Exists(ctx, "semantic_cache:"+expired.CacheID).Val() == 0 }, time.Second, 10*time.Millisecond)
	// The active hash remains authoritative after the secondary indexes disappear.
	require.NoError(t, redisClient.Del(ctx, fmt.Sprintf("tenant:%d:caches", tenant), fmt.Sprintf("tenant:%d:agent:rag:caches", tenant)).Err())
	req := d.OrphanCleanupRequest{TenantID: tenant, BatchSize: 2, MaxVectors: 2}
	result, err := s.CleanupOrphanVectors(ctx, req)
	require.ErrorIs(t, err, d.ErrCleanupIncomplete)
	require.Zero(t, result.Deleted)
	rows, err := vectors.ListCleanupCandidates(ctx, tenant, time.Now().Unix(), 100, 2)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	// A corrupt primary hash in the last candidate fails the entire preflight.
	require.NoError(t, redisClient.Set(ctx, "semantic_cache:"+orphan.CacheID, "wrong type", time.Hour).Err())
	req.MaxVectors = 100
	result, err = s.CleanupOrphanVectors(ctx, req)
	require.Error(t, err)
	require.Zero(t, result.Deleted)
	require.NoError(t, redisClient.Del(ctx, "semantic_cache:"+orphan.CacheID).Err())
	result, err = s.CleanupOrphanVectors(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 3, result.Scanned)
	require.Equal(t, 1, result.Preserved)
	require.EqualValues(t, 2, result.Deleted)
	rows, err = vectors.ListCleanupCandidates(ctx, tenant, time.Now().Unix(), 100, 2)
	require.NoError(t, err)
	require.ElementsMatch(t, []d.CacheVectorIdentity{active, fresh}, rows)
	rows, err = vectors.ListCleanupCandidates(ctx, tenant+1, time.Now().Unix(), 100, 2)
	require.NoError(t, err)
	require.Equal(t, []d.CacheVectorIdentity{other}, rows)
	// Real Redis races against a held deletion. The first adapter response is an
	// injected definite rejection; the subsequent retry performs a real RPC.
	race := add("race", tenant, true, false)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	fault := &cleanupVectorFixture{CacheVectorRepository: vectors, rows: []d.CacheVectorIdentity{race}, delete: func(d.CacheVectorIdentity) (int64, error) {
		close(entered)
		<-release
		return 0, &d.CacheDeleteError{Cause: errors.New("injected rejection"), NotApplied: true}
	}}
	fs := NewCacheManagementService(nil, fault, contents)
	go func() { _, e := fs.CleanupOrphanVectors(ctx, req); done <- e }()
	<-entered
	require.ErrorIs(t, contents.SetContent(ctx, cacheEntry(race), time.Hour), d.ErrCacheWriteFenced)
	close(release)
	require.Error(t, <-done)
	result, err = s.CleanupOrphanVectors(ctx, req)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Deleted)
	require.NoError(t, contents.SetContent(ctx, cacheEntry(race), time.Hour))
	// A definite failure can later reconcile an already absent vector. This
	// verifies the real server's zero count and releases only the same receipt.
	absent := add("absent", tenant, true, false)
	fence, err := contents.ClaimCleanup(ctx, absent)
	require.NoError(t, err)
	require.NoError(t, contents.FinishCleanup(ctx, *fence, "failed"))
	n, err := vectors.DeleteCleanupCandidate(ctx, absent)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	result, err = s.CleanupOrphanVectors(ctx, req)
	require.NoError(t, err)
	require.Zero(t, result.Deleted)
	require.NoError(t, contents.SetContent(ctx, cacheEntry(absent), time.Hour))
	// The delete really commits, but its acknowledgement is injected as lost.
	// No automatic reissue or successful count is permitted in this case.
	unknown := add("unknown", tenant, true, false)
	fault.rows = []d.CacheVectorIdentity{unknown}
	calls := 0
	fault.delete = func(v d.CacheVectorIdentity) (int64, error) {
		calls++
		_, e := vectors.DeleteCleanupCandidate(ctx, v)
		require.NoError(t, e)
		return 0, errors.New("injected lost acknowledgement")
	}
	result, err = fs.CleanupOrphanVectors(ctx, req)
	require.ErrorIs(t, err, d.ErrCleanupReconcile)
	require.Zero(t, result.Deleted)
	_, err = s.CleanupOrphanVectors(ctx, req)
	require.ErrorIs(t, err, d.ErrCleanupReconcile)
	require.Equal(t, 1, calls)
	require.ErrorIs(t, contents.SetContent(ctx, cacheEntry(unknown), time.Hour), d.ErrCacheWriteFenced)
	rows, err = vectors.ListCleanupCandidates(ctx, tenant, time.Now().Unix(), 100, 2)
	require.NoError(t, err)
	require.ElementsMatch(t, []d.CacheVectorIdentity{active, fresh}, rows)
}
