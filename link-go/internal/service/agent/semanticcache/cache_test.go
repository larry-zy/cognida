package semanticcache

import (
	"context"
	"testing"
	"time"

	"link/internal/service/agent/metricsql"
)

func sampleQuery() metricsql.Query {
	return metricsql.Query{
		Metrics:    []string{"营收"},
		Dimensions: []string{"区域"},
		Filters:    []metricsql.Filter{{Field: "区域", Op: metricsql.OpEq, Values: []string{"华东"}}},
		Limit:      10,
	}
}

func TestBuildKey_StableAndNormalized(t *testing.T) {
	k1 := BuildKey(1, "sales", 3, sampleQuery())
	// 名称大小写/空白规范化后应产生同键。
	q2 := sampleQuery()
	q2.Metrics = []string{"  营收 "}
	q2.Dimensions = []string{"区域"}
	k2 := BuildKey(1, "SALES", 3, q2)
	if k1 != k2 {
		t.Errorf("expected normalized keys equal:\n  %s\n  %s", k1, k2)
	}
}

func TestBuildKey_VersionInvalidates(t *testing.T) {
	k3 := BuildKey(1, "sales", 3, sampleQuery())
	k4 := BuildKey(1, "sales", 4, sampleQuery())
	if k3 == k4 {
		t.Errorf("version bump must change key: %s == %s", k3, k4)
	}
}

func TestBuildKey_TenantIsolated(t *testing.T) {
	if BuildKey(1, "sales", 3, sampleQuery()) == BuildKey(2, "sales", 3, sampleQuery()) {
		t.Errorf("different tenants must not share key")
	}
}

func TestMemoryCache_PutGetHitMiss(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache()
	key := BuildKey(1, "sales", 3, sampleQuery())

	if _, ok, err := c.Get(ctx, key); err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}

	if err := c.Put(ctx, key, &Entry{SQL: "SELECT 1", ModelVersion: 3, Verified: true}, DefaultTTL); err != nil {
		t.Fatalf("put: %v", err)
	}
	e, ok, err := c.Get(ctx, key)
	if err != nil || !ok {
		t.Fatalf("expected hit, got ok=%v err=%v", ok, err)
	}
	if e.SQL != "SELECT 1" || !e.Verified {
		t.Errorf("unexpected entry: %+v", e)
	}

	// 版本 bump → 新键 → miss（旧受信记录不再命中）。
	if _, ok, _ := c.Get(ctx, BuildKey(1, "sales", 4, sampleQuery())); ok {
		t.Errorf("version-bumped key must miss")
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache()
	key := "k"
	if err := c.Put(ctx, key, &Entry{SQL: "SELECT 1"}, 1*time.Millisecond); err != nil {
		t.Fatalf("put: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, ok, _ := c.Get(ctx, key); ok {
		t.Errorf("expected TTL expiry miss")
	}
}
