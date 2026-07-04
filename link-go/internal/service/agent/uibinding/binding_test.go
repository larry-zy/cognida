package uibinding

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestMemoryStore_PutGet 基本读写往返。
func TestMemoryStore_PutGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	b := &Binding{
		Surface:   "sfc_test1",
		TenantID:  1,
		SessionID: "sess-1",
		ResultID:  "res_abc",
		Token:     NewToken(),
	}
	if err := s.Put(ctx, b, time.Minute); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}

	got, err := s.Get(ctx, "sfc_test1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ResultID != "res_abc" || got.Token != b.Token || got.TenantID != 1 || got.SessionID != "sess-1" {
		t.Errorf("绑定内容不一致: %+v", got)
	}
	if got.CreatedAt == 0 {
		t.Error("CreatedAt 未回填")
	}
}

// TestMemoryStore_NotFound 不存在的 surface 返回 ErrNotFound。
func TestMemoryStore_NotFound(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Get(context.Background(), "sfc_nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("期望 ErrNotFound, got %v", err)
	}
}

// TestMemoryStore_TTLExpiry 超会话 TTL 后返回 ErrNotFound（任务 4.8："会话已过期"降级依据）。
func TestMemoryStore_TTLExpiry(t *testing.T) {
	base := time.Now().Unix()
	nowUnix = func() int64 { return base }
	defer func() { nowUnix = func() int64 { return time.Now().Unix() } }()

	s := NewMemoryStore()
	ctx := context.Background()
	b := &Binding{Surface: "sfc_ttl", TenantID: 1, SessionID: "s", Token: "tk"}
	if err := s.Put(ctx, b, time.Hour); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}

	// TTL 内可取
	if _, err := s.Get(ctx, "sfc_ttl"); err != nil {
		t.Fatalf("TTL 内应可取回: %v", err)
	}

	// 时间推进超过 TTL → 惰性过期
	nowUnix = func() int64 { return base + int64(2*time.Hour/time.Second) }
	if _, err := s.Get(ctx, "sfc_ttl"); !errors.Is(err, ErrNotFound) {
		t.Errorf("超 TTL 期望 ErrNotFound, got %v", err)
	}
}

// TestMemoryStore_EmptySurfaceRejected surface 为空拒绝写入。
func TestMemoryStore_EmptySurfaceRejected(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Put(context.Background(), &Binding{}, time.Minute); err == nil {
		t.Error("空 surface 应拒绝")
	}
}

// TestSetGetStore 包级单例注入。
func TestSetGetStore(t *testing.T) {
	old := GetStore()
	defer SetStore(old)

	s := NewMemoryStore()
	SetStore(s)
	if GetStore() != Store(s) {
		t.Error("SetStore/GetStore 不一致")
	}
}
