package pendingaction

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newAction(owner string) *PendingAction {
	return &PendingAction{
		Owner: owner,
		Kind:  "mutate",
		SQL:   "UPDATE agent_etl_t SET a = 1",
		Params: map[string]interface{}{
			"idempotency_key": "k1",
		},
	}
}

func TestMemoryStore_PutConsume(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	action := newAction("1:sess")
	id, err := s.Put(ctx, action, DefaultTTL)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if id == "" || action.Token == "" {
		t.Fatalf("expected generated id/token, got id=%q token=%q", id, action.Token)
	}

	got, err := s.Consume(ctx, "1:sess", id, action.Token)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got.SQL != action.SQL || got.Kind != "mutate" {
		t.Fatalf("consumed action mismatch: %+v", got)
	}

	// 只能消费一次
	if _, err := s.Consume(ctx, "1:sess", id, action.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second consume should be ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_TokenMismatchConsumes(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	action := newAction("1:sess")
	id, _ := s.Put(ctx, action, DefaultTTL)

	// token 不匹配：返回 ErrTokenMismatch 且 action 立即失效（防暴力尝试）
	if _, err := s.Consume(ctx, "1:sess", id, "wrong-token"); !errors.Is(err, ErrTokenMismatch) {
		t.Fatalf("expected ErrTokenMismatch, got %v", err)
	}
	if _, err := s.Consume(ctx, "1:sess", id, action.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("action should be consumed after token mismatch, got %v", err)
	}
}

func TestMemoryStore_OwnerMismatchNotConsumed(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	action := newAction("1:sess")
	id, _ := s.Put(ctx, action, DefaultTTL)

	// 归属不符：视同不存在，且不消费
	if _, err := s.Consume(ctx, "2:other", id, action.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for owner mismatch, got %v", err)
	}
	// 原归属者仍可消费
	if _, err := s.Consume(ctx, "1:sess", id, action.Token); err != nil {
		t.Fatalf("original owner should still consume, got %v", err)
	}
}

func TestMemoryStore_Expired(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	action := newAction("1:sess")
	id, _ := s.Put(ctx, action, time.Minute)

	// 时间快进到过期后
	orig := nowUnix
	defer func() { nowUnix = orig }()
	nowUnix = func() int64 { return action.ExpiresAt + 1 }

	if _, err := s.Consume(ctx, "1:sess", id, action.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired action, got %v", err)
	}
}

func TestMemoryStore_NotFound(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Consume(context.Background(), "1:sess", "pa_missing", "tok"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
