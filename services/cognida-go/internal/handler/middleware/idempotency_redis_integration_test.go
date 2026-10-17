//go:build integration

package middleware

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"os"
	"testing"
	"time"
)

func TestIdempotencyRealRedisAtomicBinding(t *testing.T) {
	addr := os.Getenv("COGNIDA_IDEMPOTENCY_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("COGNIDA_IDEMPOTENCY_TEST_REDIS_ADDR must explicitly select a test Redis")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Ping(context.Background()).Err())
	m := NewIdempotency(client)
	testBoundIdempotency(t, m)
	key := "idem:review:" + uuid.NewString()
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), key).Err()) })
	old := idemRecord{State: idemStateNew, Owner: "old", Digest: "old-body"}
	next := idemRecord{State: idemStateNew, Owner: "next", Digest: "next-body"}
	first, _, err := m.begin(context.Background(), key, old)
	require.NoError(t, err)
	require.True(t, first)
	require.NoError(t, client.PExpire(context.Background(), key, time.Millisecond).Err())
	require.Eventually(t, func() bool { return client.Exists(context.Background(), key).Val() == 0 }, time.Second, time.Millisecond)
	first, _, err = m.begin(context.Background(), key, next)
	require.NoError(t, err)
	require.True(t, first)
	old.State = idemStateDon
	m.complete(context.Background(), key, old)
	m.release(context.Background(), key, old.Owner)
	first, current, err := m.begin(context.Background(), key, old)
	require.NoError(t, err)
	require.False(t, first)
	require.Equal(t, next, *current)
}
