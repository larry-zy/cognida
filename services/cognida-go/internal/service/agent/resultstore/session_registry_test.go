package resultstore

import (
	"sort"
	"testing"
	"time"
)

// LiveFetches：按 SQL 签名去重后返回本 owner 全部未过期取数；不同 SQL → 多份候选，供 data_analysis 判歧义。
func TestSessionRegistry_LiveFetches_Distinct(t *testing.T) {
	restore := nowUnix
	nowUnix = func() int64 { return 1000 }
	defer func() { nowUnix = restore }()

	reg := NewSessionRegistry(30 * time.Minute)
	owner := OwnerKey(1, "s")
	reg.RecordFetch(owner, SQLSignature("db1", "SELECT a"), "res-a")
	reg.RecordFetch(owner, SQLSignature("db1", "SELECT b"), "res-b")

	got := reg.LiveFetches(owner)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "res-a" || got[1] != "res-b" {
		t.Fatalf("LiveFetches = %v, want [res-a res-b]", got)
	}
}

// 同一 SQL 重复执行（去重复用同一 result_id）只计一次，不放大成多份候选。
func TestSessionRegistry_LiveFetches_DedupSameSQL(t *testing.T) {
	restore := nowUnix
	nowUnix = func() int64 { return 1000 }
	defer func() { nowUnix = restore }()

	reg := NewSessionRegistry(30 * time.Minute)
	owner := OwnerKey(1, "s")
	sig := SQLSignature("db1", "SELECT a")
	reg.RecordFetch(owner, sig, "res-a")
	reg.RecordFetch(owner, sig, "res-a") // 复用命中，签名与 id 均相同

	if got := reg.LiveFetches(owner); len(got) != 1 || got[0] != "res-a" {
		t.Fatalf("LiveFetches = %v, want [res-a]", got)
	}
}

// 过期取数不计入 live，并被惰性清理。
func TestSessionRegistry_LiveFetches_ExpiryEvicts(t *testing.T) {
	restore := nowUnix
	base := int64(1000)
	nowUnix = func() int64 { return base }
	defer func() { nowUnix = restore }()

	reg := NewSessionRegistry(60 * time.Second)
	owner := OwnerKey(1, "s")
	reg.RecordFetch(owner, SQLSignature("db1", "SELECT a"), "res-a")

	base += 120 // 越过 TTL
	if got := reg.LiveFetches(owner); len(got) != 0 {
		t.Fatalf("过期后 LiveFetches = %v, want 空", got)
	}
}

// nil 接收者 / 空 owner / 未知 owner 均安全返回空。
func TestSessionRegistry_LiveFetches_Safe(t *testing.T) {
	var nilReg *SessionRegistry
	if got := nilReg.LiveFetches("x"); got != nil {
		t.Errorf("nil 接收者 = %v, want nil", got)
	}
	reg := NewSessionRegistry(time.Minute)
	if got := reg.LiveFetches(""); got != nil {
		t.Errorf("空 owner = %v, want nil", got)
	}
	if got := reg.LiveFetches(OwnerKey(9, "none")); got != nil {
		t.Errorf("未知 owner = %v, want nil", got)
	}
}
