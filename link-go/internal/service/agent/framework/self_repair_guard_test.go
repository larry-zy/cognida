package framework

import (
	"strings"
	"testing"
)

// TestFailureGuard_SignatureAndReplan 校验：同一签名达阈值触发一次再规划提示，且不重复注入。
func TestFailureGuard_SignatureAndReplan(t *testing.T) {
	g := newFailureGuard()

	// 首次失败：未达阈值，无再规划。
	g.recordFailure("sql_execute", "unknown_column")
	if note := g.replanNote(); note != "" {
		t.Fatalf("首次失败不应再规划: %q", note)
	}
	// 第二次同签名失败：达阈值，注入一次再规划提示。
	g.recordFailure("sql_execute", "unknown_column")
	note := g.replanNote()
	if note == "" || !strings.Contains(note, "sql_execute:unknown_column") {
		t.Fatalf("第二次同签名失败应再规划并含签名: %q", note)
	}
	// 再次查询：同签名已提示过，不重复注入。
	if again := g.replanNote(); again != "" {
		t.Fatalf("同签名不应重复注入: %q", again)
	}
}

// TestFailureGuard_DifferentKindsSeparateSignatures 校验不同 error_kind 各自独立计数。
func TestFailureGuard_DifferentKindsSeparateSignatures(t *testing.T) {
	g := newFailureGuard()
	g.recordFailure("sql_execute", "unknown_column")
	g.recordFailure("sql_execute", "syntax")
	// 两个不同签名各 1 次，均未达阈值。
	if note := g.replanNote(); note != "" {
		t.Fatalf("不同签名各 1 次不应再规划: %q", note)
	}
}

// TestFailureGuard_SuccessResetsTool 校验同工具任一成功清零其名下所有失败签名。
func TestFailureGuard_SuccessResetsTool(t *testing.T) {
	g := newFailureGuard()
	g.recordFailure("sql_execute", "unknown_column")
	g.recordSuccess("sql_execute") // 一次进展打破失败循环
	g.recordFailure("sql_execute", "unknown_column")
	// 计数从零重来，仅 1 次，不应再规划。
	if note := g.replanNote(); note != "" {
		t.Fatalf("成功后应清零该工具计数: %q", note)
	}
}

// TestFailureGuard_SuccessDoesNotResetOtherTools 校验成功仅清零本工具，不牵连其它工具。
func TestFailureGuard_SuccessDoesNotResetOtherTools(t *testing.T) {
	g := newFailureGuard()
	g.recordFailure("sql_execute", "syntax")
	g.recordFailure("sql_execute", "syntax")
	g.recordSuccess("data_export") // 无关工具成功
	note := g.replanNote()
	if note == "" || !strings.Contains(note, "sql_execute:syntax") {
		t.Fatalf("无关工具成功不应清零 sql_execute 计数: %q", note)
	}
}

// TestFailureGuard_WindDownThreshold 校验累计失败达阈值判定原地打转。
func TestFailureGuard_WindDownThreshold(t *testing.T) {
	g := newFailureGuard()
	for i := 0; i < windDownThreshold-1; i++ {
		g.recordFailure("sql_execute", "other")
	}
	if g.shouldWindDown() {
		t.Fatal("未达阈值不应收尾")
	}
	g.recordFailure("sql_execute", "other")
	if !g.shouldWindDown() {
		t.Fatal("达阈值应提前收尾")
	}
}

// TestFailureGuard_NilSafe 校验空 guard 各方法安全（防御性）。
func TestFailureGuard_NilSafe(t *testing.T) {
	var g *failureGuard
	g.recordFailure("t", "k") // 不应 panic
	g.recordSuccess("t")
	if g.replanNote() != "" || g.shouldWindDown() {
		t.Fatal("nil guard 应惰性安全")
	}
}

// TestFailureSignature 校验签名构造：有/无 error_kind。
func TestFailureSignature(t *testing.T) {
	if got := failureSignature("sql_execute", "syntax"); got != "sql_execute:syntax" {
		t.Fatalf("got %q", got)
	}
	if got := failureSignature("sql_execute", ""); got != "sql_execute" {
		t.Fatalf("空 kind 应退化为工具名, got %q", got)
	}
}
