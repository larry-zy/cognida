//go:build integration

package resultstore

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 运行：REDIS_ADDR=localhost:6379 go test -tags=integration ./internal/service/agent/resultstore/ -v
func newRedisForTest(t *testing.T) *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis 不可用 (%s)，跳过集成测试: %v", addr, err)
	}
	return client
}

func TestRedisStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	client := newRedisForTest(t)
	defer client.Close()

	s := NewRedisStore(client)
	owner := OwnerKey(7, "sess-int")

	id, err := s.Put(ctx, sampleResult(owner, 5000), 30*time.Second)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}
	t.Cleanup(func() { client.Del(ctx, "agent:resultstore:"+id) })

	got, err := s.Get(ctx, owner, id)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if len(got.Rows) != 5000 {
		t.Fatalf("expected 5000 rows round-tripped, got %d", len(got.Rows))
	}

	// 信封只应携带样本，不含全量行
	env := BuildEnvelope(got, DefaultSampleRows)
	if len(env.Samples) != DefaultSampleRows {
		t.Fatalf("expected %d samples, got %d", DefaultSampleRows, len(env.Samples))
	}
	if env.RowCount != 5000 {
		t.Fatalf("expected row_count 5000, got %d", env.RowCount)
	}
}

func TestRedisStore_CrossOwnerRejected(t *testing.T) {
	ctx := context.Background()
	client := newRedisForTest(t)
	defer client.Close()

	s := NewRedisStore(client)
	id, err := s.Put(ctx, sampleResult(OwnerKey(7, "sess-a"), 3), 30*time.Second)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}
	t.Cleanup(func() { client.Del(ctx, "agent:resultstore:"+id) })

	if _, err := s.Get(ctx, OwnerKey(7, "sess-b"), id); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestRedisStore_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	client := newRedisForTest(t)
	defer client.Close()

	s := NewRedisStore(client)
	owner := OwnerKey(7, "sess-ttl")
	id, err := s.Put(ctx, sampleResult(owner, 2), 1*time.Second)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)
	if _, err := s.Get(ctx, owner, id); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after TTL, got %v", err)
	}
}
