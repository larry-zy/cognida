//go:build integration

package job

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// This test uses an existing local Redis service and a unique namespace. It
// never flushes a database or removes keys belonging to another test/process.
func TestRedisQueueRealDeliveryLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: os.Getenv("REDIS_PASSWORD"), DB: 0})
	prefix := "cognida:integration:" + uuid.NewString()
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var cursor uint64
		for {
			keys, next, err := client.Scan(cleanup, cursor, prefix+":*", 100).Result()
			if err != nil {
				t.Errorf("scan isolated queue cleanup: %v", err)
				break
			}
			if len(keys) > 0 {
				if err := client.Del(cleanup, keys...).Err(); err != nil {
					t.Errorf("delete isolated queue keys: %v", err)
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		require.NoError(t, client.Close())
	})
	require.NoError(t, client.Ping(ctx).Err(), "local Redis must be available for integration tests")
	q := NewRedisQueueWithPrefix(client, prefix)
	j := Job{ID: "durable-run", Type: "collection.run"}
	// Concurrent idempotent submissions must create precisely one delivery.
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- q.Enqueue(ctx, j) }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	size, err := q.ReadySize(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, size)
	old, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, old)
	recovered, err := q.Recover(ctx)
	require.NoError(t, err)
	require.Zero(t, recovered, "live lease cannot be stolen by another worker")
	// Force only our delivery's lease to expire; do not wait for a real minute.
	require.NoError(t, client.ZAdd(ctx, q.leases(), redis.Z{Score: 0, Member: old.raw}).Err())
	recovered, err = q.Recover(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	current, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Error(t, q.Ack(ctx, *old))
	require.Error(t, q.Retry(ctx, *old, 0))
	require.Error(t, q.Heartbeat(ctx, *old))
	require.NoError(t, q.Heartbeat(ctx, *current))
	require.NoError(t, q.Retry(ctx, *current, 0))
	promoted, err := q.PromoteDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, promoted)
	retried, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, retried)
	require.Equal(t, 1, retried.Attempt)
	require.NoError(t, q.Ack(ctx, *retried))
	require.Zero(t, client.Exists(ctx, q.marker(j.ID)).Val(), "terminal acknowledgement clears idempotency marker")
	require.Zero(t, client.LLen(ctx, q.processing()).Val())
	require.NoError(t, q.Enqueue(ctx, j))
	size, err = q.ReadySize(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, size)
	// A Redis type failure cannot leave an orphan marker that blocks recovery.
	require.NoError(t, q.Purge(ctx))
	require.NoError(t, client.Set(ctx, q.ready(), "wrong-type", 0).Err())
	require.Error(t, q.Enqueue(ctx, Job{ID: "failed-submit", Type: "test"}))
	require.Zero(t, client.Exists(ctx, q.marker("failed-submit")).Val())
	require.NoError(t, client.Del(ctx, q.ready()).Err())
	require.NoError(t, q.Schedule(ctx, Job{ID: "scheduled", Type: "test"}, 0))
	promoted, err = q.PromoteDue(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, promoted)
}
