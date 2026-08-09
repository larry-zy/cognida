package framework

import (
	"context"
	"errors"
	"testing"
)

// TestInlineDelegate_NoRegistryDegrades 校验：未注入注册表时 InlineDelegate 返回明确的
// ErrNoCollabRegistry（而非 panic），且不牵连其它路径。
func TestInlineDelegate_NoRegistryDegrades(t *testing.T) {
	if HasCollaborationRegistry(context.Background()) {
		t.Fatal("裸 ctx 不应含注册表")
	}
	_, err := InlineDelegate(context.Background(), DelegationEnvelope{
		AgentName:   "sql_author",
		Goal:        "取数",
		Constraints: DelegationConstraints{Scope: ScopeRead},
	})
	if !errors.Is(err, ErrNoCollabRegistry) {
		t.Fatalf("未注入应返回 ErrNoCollabRegistry, got %v", err)
	}
}

// TestWithCollaborationRegistry_InjectAndRead 校验注入后可被 handler 侧取用。
func TestWithCollaborationRegistry_InjectAndRead(t *testing.T) {
	reg := NewCollaborationRegistry()
	ctx := WithCollaborationRegistry(context.Background(), reg)
	if !HasCollaborationRegistry(ctx) {
		t.Fatal("注入后应可见注册表")
	}
	got, ok := collabRegistryFromContext(ctx)
	if !ok || got != reg {
		t.Fatalf("取用注册表异常: ok=%v got=%p want=%p", ok, got, reg)
	}
}

// TestWithCollaborationRegistry_NilNoop 校验 nil 注册表注入为 no-op，取用方仍安全降级。
func TestWithCollaborationRegistry_NilNoop(t *testing.T) {
	ctx := WithCollaborationRegistry(context.Background(), nil)
	if HasCollaborationRegistry(ctx) {
		t.Fatal("nil 注入不应标记为具备注册表")
	}
}

// TestInlineDelegate_InvalidEnvelopeRejected 校验：注入注册表后，信封契约仍被校验
// （缺 scope 拒绝），即 InlineDelegate 复用同一委派内核而非绕过护栏。
func TestInlineDelegate_InvalidEnvelopeRejected(t *testing.T) {
	reg := NewCollaborationRegistry()
	ctx := WithCollaborationRegistry(context.Background(), reg)
	_, err := InlineDelegate(ctx, DelegationEnvelope{AgentName: "x", Goal: "g"}) // 缺 scope
	if err == nil || errors.Is(err, ErrNoCollabRegistry) {
		t.Fatalf("缺 scope 应被信封校验拒绝, got %v", err)
	}
}
