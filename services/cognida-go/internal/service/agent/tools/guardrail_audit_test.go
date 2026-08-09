// Package tools 测试组合根护栏审计留痕（guardrail-runtime 任务 6/7）。
package tools

import (
	"strings"
	"testing"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/agent/operations"
	"cognida/internal/service/agent/framework"
)

// TestGuardrailAuditStatus_Mapping 校验事件类型 → 审计状态映射：
// 拦截类 rejected、写审批 pending_confirm、脱敏类 success。
func TestGuardrailAuditStatus_Mapping(t *testing.T) {
	cases := map[string]string{
		framework.GuardrailEventInputBlocked:        operations.StatusRejected,
		framework.GuardrailEventJailbreakBlocked:    operations.StatusRejected,
		framework.GuardrailEventWriteApprovalNeeded: operations.StatusPendingConfirm,
		framework.GuardrailEventInputRedacted:       operations.StatusSuccess,
		framework.GuardrailEventOutputRedacted:      operations.StatusSuccess,
		framework.GuardrailEventToolOutputRedacted:  operations.StatusSuccess,
	}
	for evt, want := range cases {
		if got := guardrailAuditStatus(evt); got != want {
			t.Fatalf("guardrailAuditStatus(%q)=%q, want %q", evt, got, want)
		}
	}
}

// TestRecordGuardrailAudit_EachEventLandsOneRow 校验各类护栏事件经组合根记录器恰落一条
// type=guardrail 审计，含事件类型、命中工具、原因(Detail)与全链路 rid/session/tenant。
func TestRecordGuardrailAudit_EachEventLandsOneRow(t *testing.T) {
	o, _, audit, _, ctx := setupOperationTest(t)
	ctx = agentctx.WithRequestID(ctx, "rid-guard-audit")

	cases := []struct {
		evt        framework.GuardrailEvent
		wantStatus string
	}{
		{framework.GuardrailEvent{Type: framework.GuardrailEventInputBlocked, Detail: "输入不安全"}, operations.StatusRejected},
		{framework.GuardrailEvent{Type: framework.GuardrailEventJailbreakBlocked, Detail: "越狱意图"}, operations.StatusRejected},
		{framework.GuardrailEvent{Type: framework.GuardrailEventOutputRedacted, Detail: "输出脱敏"}, operations.StatusSuccess},
		{framework.GuardrailEvent{Type: framework.GuardrailEventToolOutputRedacted, Tool: "sql_execute", Detail: "工具观察脱敏"}, operations.StatusSuccess},
		{framework.GuardrailEvent{Type: framework.GuardrailEventWriteApprovalNeeded, Tool: "sql_mutate", Detail: "需人工审批"}, operations.StatusPendingConfirm},
	}

	for i, tc := range cases {
		o.recordGuardrailAudit(ctx, tc.evt)
		rec := audit.lastAudit(t)
		if rec.OperationType != operations.OpGuardrail {
			t.Fatalf("[%d] OperationType=%q, want %q", i, rec.OperationType, operations.OpGuardrail)
		}
		if rec.Status != tc.wantStatus {
			t.Fatalf("[%d] Status=%q, want %q", i, rec.Status, tc.wantStatus)
		}
		if rec.Result != tc.evt.Detail {
			t.Fatalf("[%d] Result(原因)=%q, want %q", i, rec.Result, tc.evt.Detail)
		}
		if rec.RequestID != "rid-guard-audit" {
			t.Fatalf("[%d] RequestID=%q, want rid-guard-audit", i, rec.RequestID)
		}
		if rec.SessionID != "sess-op" || rec.TenantID != 1 {
			t.Fatalf("[%d] session/tenant 透传错: session=%q tenant=%d", i, rec.SessionID, rec.TenantID)
		}
		// Params 应携事件类型，便于按事件维度检索。
		if !strings.Contains(rec.Params, tc.evt.Type) {
			t.Fatalf("[%d] Params 未携事件类型: %s", i, rec.Params)
		}
	}

	if len(audit.records) != len(cases) {
		t.Fatalf("每类事件应恰落一条审计，got %d want %d", len(audit.records), len(cases))
	}
}
