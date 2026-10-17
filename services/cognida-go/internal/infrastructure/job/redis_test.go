package job

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testQueue(t *testing.T) *RedisQueue {
	t.Helper()
	s := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { require.NoError(t, c.Close()) })
	return NewRedisQueueWithPrefix(c, "test:jobs")
}
func TestRedisQueueRecoveryRetryAndAck(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	j := Job{ID: "run-1", Type: "collection.run"}
	require.NoError(t, q.Enqueue(ctx, j))
	claimed, err := q.Claim(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, j.ID, claimed.ID)
	require.Equal(t, j.Type, claimed.Type)
	n, err := q.Recover(ctx)
	require.NoError(t, err)
	require.Zero(t, n, "a live delivery must not be stolen")
	require.NoError(t, q.client.ZAdd(ctx, q.leases(), redis.Z{Score: 0, Member: claimed.Delivery.(*deliveryReceipt).raw}).Err())
	n, err = q.Recover(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	claimed, err = q.Claim(ctx, time.Second)
	require.NoError(t, err)
	require.NoError(t, q.Retry(ctx, *claimed, 0))
	n, err = q.PromoteDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	claimed, err = q.Claim(ctx, time.Second)
	require.NoError(t, err)
	require.Equal(t, 1, claimed.Attempt)
	require.NoError(t, q.Ack(ctx, *claimed))
	require.NoError(t, q.Enqueue(ctx, j))
}

func TestRedisQueueSchedulesAndPromotesNewJobs(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	j := Job{ID: "scheduled-1", Type: "test"}
	require.NoError(t, q.Schedule(ctx, j, 0))
	n, err := q.PromoteDue(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	claimed, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.Equal(t, j.ID, claimed.ID)
	require.NoError(t, q.Ack(ctx, *claimed))
}

func TestRedisQueueFencesStaleAcknowledgement(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	j := Job{ID: "fenced", Type: "test"}
	require.NoError(t, q.Enqueue(ctx, j))
	old, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.NoError(t, q.client.ZAdd(ctx, q.leases(), redis.Z{Score: 0, Member: old.Delivery.(*deliveryReceipt).raw}).Err())
	_, err = q.Recover(ctx)
	require.NoError(t, err)
	current, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.Error(t, q.Ack(ctx, *old))
	require.Error(t, q.Retry(ctx, *old, 0))
	require.NoError(t, q.Heartbeat(ctx, *current))
	require.NoError(t, q.Ack(ctx, *current))
}
func TestRedisQueuePurgeAllowsReenqueue(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	j := Job{ID: "purged", Type: "test"}
	require.NoError(t, q.Enqueue(ctx, j))
	require.NoError(t, q.Purge(ctx))
	require.NoError(t, q.Enqueue(ctx, j))
	claimed, err := q.TryClaim(ctx)
	require.NoError(t, err)
	require.NotNil(t, claimed)
}
func TestRedisQueueEnqueueFailureDoesNotPoisonMarker(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	j := Job{ID: "retry-enqueue", Type: "test"}
	require.NoError(t, q.client.Set(ctx, q.ready(), "wrong-type", 0).Err())
	require.Error(t, q.Enqueue(ctx, j))
	require.Zero(t, q.client.Exists(ctx, q.marker(j.ID)).Val())
	require.NoError(t, q.client.Del(ctx, q.ready()).Err())
	require.NoError(t, q.Enqueue(ctx, j))
}
