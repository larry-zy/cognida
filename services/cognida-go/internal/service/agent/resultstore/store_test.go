package resultstore

import (
	"context"
	"testing"
	"time"
)

func sampleResult(owner string, n int) *Result {
	rows := make([]map[string]interface{}, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, map[string]interface{}{
			"id":     i,
			"region": "R",
			"amount": float64(i) * 1.5,
		})
	}
	return &Result{
		Owner:   owner,
		Columns: []string{"id", "region", "amount"},
		Rows:    rows,
	}
}

func TestMemoryStore_PutGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	owner := OwnerKey(1, "sess-a")

	id, err := s.Put(ctx, sampleResult(owner, 3), DefaultTTL)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty result_id")
	}

	got, err := s.Get(ctx, owner, id)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(got.Rows))
	}
}

func TestMemoryStore_CrossOwnerRejected(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	ownerA := OwnerKey(1, "sess-a")
	ownerB := OwnerKey(1, "sess-b")

	id, err := s.Put(ctx, sampleResult(ownerA, 2), DefaultTTL)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}

	// 同库不同会话读取应被拒
	if _, err := s.Get(ctx, ownerB, id); err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestMemoryStore_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	if _, err := s.Get(ctx, OwnerKey(1, "x"), "rs_missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Expiry(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	owner := OwnerKey(1, "sess-a")

	// 冻结时间：写入时刻 t0，TTL 1 分钟
	base := time.Now().Unix()
	restore := nowUnix
	nowUnix = func() int64 { return base }
	defer func() { nowUnix = restore }()

	id, err := s.Put(ctx, sampleResult(owner, 1), time.Minute)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}

	// 快进 2 分钟后应过期
	nowUnix = func() int64 { return base + 120 }
	if _, err := s.Get(ctx, owner, id); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after expiry, got %v", err)
	}
}

func TestBuildEnvelope_SampleCapAndAggregates(t *testing.T) {
	r := sampleResult(OwnerKey(1, "s"), 100)
	r.ResultID = "rs_x"

	env := BuildEnvelope(r, 20)

	if env.RowCount != 100 {
		t.Fatalf("expected row_count 100, got %d", env.RowCount)
	}
	if len(env.Samples) != 20 {
		t.Fatalf("expected 20 samples (capped), got %d", len(env.Samples))
	}
	if !env.Truncated {
		t.Fatal("expected Truncated=true when samples < rows")
	}
	if env.Dtypes["amount"] != "number" {
		t.Fatalf("expected amount dtype number, got %s", env.Dtypes["amount"])
	}
	if env.Dtypes["region"] != "string" {
		t.Fatalf("expected region dtype string, got %s", env.Dtypes["region"])
	}
	amountAgg, ok := env.Aggregates["amount"].(map[string]interface{})
	if !ok {
		t.Fatal("expected numeric aggregates for amount")
	}
	if amountAgg["count"].(int) != 100 {
		t.Fatalf("expected amount count 100, got %v", amountAgg["count"])
	}
}

func TestBuildEnvelope_EmptyResult(t *testing.T) {
	r := &Result{ResultID: "rs_e", Columns: []string{"id"}, Rows: nil}
	env := BuildEnvelope(r, 20)
	if env.RowCount != 0 {
		t.Fatalf("expected row_count 0, got %d", env.RowCount)
	}
	if env.Truncated {
		t.Fatal("empty result should not be truncated")
	}
	if len(env.Samples) != 0 {
		t.Fatalf("expected 0 samples, got %d", len(env.Samples))
	}
}

func TestBuildEnvelope_NumericStrings(t *testing.T) {
	// SQL 驱动常把数值列以字符串（[]byte→string）返回，应仍识别为 number 并可聚合。
	r := &Result{
		ResultID: "rs_s",
		Columns:  []string{"v"},
		Rows: []map[string]interface{}{
			{"v": "10"}, {"v": "20"}, {"v": "30"},
		},
	}
	env := BuildEnvelope(r, 20)
	if env.Dtypes["v"] != "number" {
		t.Fatalf("expected v dtype number, got %s", env.Dtypes["v"])
	}
	agg := env.Aggregates["v"].(map[string]interface{})
	if agg["sum"].(float64) != 60 {
		t.Fatalf("expected sum 60, got %v", agg["sum"])
	}
	if agg["max"].(float64) != 30 {
		t.Fatalf("expected max 30, got %v", agg["max"])
	}
}
