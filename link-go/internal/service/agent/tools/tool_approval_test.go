// Package tools 测试策略级写审批（agent-guardrail-runtime 任务 4）：
// 被标记审批的写类工具经 handleWriteApproval 生成 pending_action（复用 sql_mutate
// 危险级同一确认通道），人工确认后经 ExecuteConfirmed* 恢复执行；与红线/行数阈值门叠加。
package tools

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"link/internal/model/agent/operations"
	"link/internal/service/agent/framework"
	"link/internal/service/agent/resultstore"
)

// decodePayload 解析 handleWriteApproval 回灌 LLM 的待确认载荷。
func decodePayload(t *testing.T, raw string) approvalPendingPayload {
	t.Helper()
	var p approvalPendingPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("待确认载荷必须是 JSON: %v (raw=%s)", err, raw)
	}
	if p.Status != operations.StatusPendingConfirm {
		t.Fatalf("载荷 status 应为 pending_confirm, got %q", p.Status)
	}
	if p.PendingActionID == "" || p.ConfirmToken == "" {
		t.Fatalf("载荷应含 pending_action_id + confirm_token, got %+v", p)
	}
	return p
}

// ========================================
// 生成 pending_action + 确认后 resume（三工具各一条端到端）
// ========================================

func TestHandleWriteApproval_SQLMutate_GeneratesPendingAndConfirms(t *testing.T) {
	o, mock, audit, pending, ctx := setupOperationTest(t)

	out, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{
		Tool:      "sql_mutate",
		Arguments: `{"sql":"UPDATE orders SET status='done' WHERE id=7","idempotency_key":"appr-mut"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := decodePayload(t, out)
	if p.Tool != "sql_mutate" || p.Target != "orders" {
		t.Fatalf("payload mismatch: %+v", p)
	}
	// 生成 pending_action 也必须留痕 pending_confirm。
	last := audit.lastAudit(t)
	if last.Status != operations.StatusPendingConfirm || last.OperationType != operations.OpMutate {
		t.Fatalf("audit mismatch: %+v", last)
	}

	// 暂存动作可被正确 owner+token 消费，且承载 mutate resume 所需的 SQL/Params。
	owner := resultstore.OwnerKey(1, "sess-op")
	action, err := pending.Consume(ctx, owner, p.PendingActionID, p.ConfirmToken)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if action.Kind != operations.OpMutate || action.SQL != "UPDATE orders SET status='done' WHERE id=7" {
		t.Fatalf("pending action mismatch: %+v", action)
	}
	if key, _ := action.Params["idempotency_key"].(string); key != "appr-mut" {
		t.Fatalf("pending action should carry idempotency_key, got %v", action.Params)
	}
	if approved, _ := action.Params["approval"].(bool); !approved {
		t.Fatalf("pending action should be flagged approval, got %v", action.Params)
	}

	// 确认后经既有 ExecuteConfirmedMutation 恢复执行（框架门在审批阶段已放行）。
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE orders SET status='done' WHERE id=7")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := o.ExecuteConfirmedMutation(ctx, action)
	if err != nil {
		t.Fatalf("confirm-resume mutate: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowsAffected != 1 {
		t.Fatalf("expected success 1 row, got %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db expectations: %v", err)
	}
}

func TestHandleWriteApproval_ETLRun_GeneratesPendingAndConfirms(t *testing.T) {
	o, mock, audit, pending, ctx := setupOperationTest(t)

	out, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{
		Tool:      "etl_run",
		Arguments: `{"target_table":"agent_etl_appr","sql":"SELECT id FROM sales"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := decodePayload(t, out)
	if p.Tool != "etl_run" || p.Target != "agent_etl_appr" {
		t.Fatalf("payload mismatch: %+v", p)
	}
	if last := audit.lastAudit(t); last.Status != operations.StatusPendingConfirm || last.OperationType != operations.OpETL {
		t.Fatalf("audit mismatch: %+v", last)
	}

	owner := resultstore.OwnerKey(1, "sess-op")
	action, err := pending.Consume(ctx, owner, p.PendingActionID, p.ConfirmToken)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if action.Kind != operations.OpETL {
		t.Fatalf("expected etl kind, got %s", action.Kind)
	}
	// etl 恢复靠暂存的原始参数 JSON 重建请求。
	if raw, _ := action.Params["arguments"].(string); !strings.Contains(raw, "agent_etl_appr") {
		t.Fatalf("pending action should carry raw arguments, got %v", action.Params)
	}

	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `agent_etl_appr` AS (SELECT id FROM sales)")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM `agent_etl_appr`")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(17))

	res, err := o.ExecuteConfirmedETL(ctx, action)
	if err != nil {
		t.Fatalf("confirm-resume etl: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowCount != 17 {
		t.Fatalf("expected success 17 rows, got %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("db expectations: %v", err)
	}
}

func TestHandleWriteApproval_DataExport_GeneratesPendingAndConfirms(t *testing.T) {
	o, _, audit, pending, ctx := setupOperationTest(t)
	id := setupExportStore(t, o, ctx)

	out, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{
		Tool:      "data_export",
		Arguments: `{"result_id":"` + id + `","format":"csv"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := decodePayload(t, out)
	if p.Tool != "data_export" || p.Target != id {
		t.Fatalf("payload mismatch: %+v", p)
	}
	if last := audit.lastAudit(t); last.Status != operations.StatusPendingConfirm || last.OperationType != operations.OpExport {
		t.Fatalf("audit mismatch: %+v", last)
	}

	owner := resultstore.OwnerKey(1, "sess-op")
	action, err := pending.Consume(ctx, owner, p.PendingActionID, p.ConfirmToken)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if action.Kind != operations.OpExport {
		t.Fatalf("expected export kind, got %s", action.Kind)
	}

	res, err := o.ExecuteConfirmedExport(ctx, action)
	if err != nil {
		t.Fatalf("confirm-resume export: %v", err)
	}
	if res.Status != operations.StatusSuccess || res.RowCount != 2 {
		t.Fatalf("expected success 2 rows, got %+v", res)
	}
}

// ========================================
// 未标记写行为不变（不经审批处理器的路径由 operation_tools_test 覆盖）
// 这里补审批处理器自身的错误分支：宁拒不闯 + 参数缺失 + 未知工具
// ========================================

func TestHandleWriteApproval_NoPendingStoreRejects(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)
	o.cfg.Pending = nil // 宁拒不闯：审批通道不可用

	_, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{
		Tool:      "sql_mutate",
		Arguments: `{"sql":"UPDATE orders SET a=1","idempotency_key":"k"}`,
	})
	if err == nil {
		t.Fatalf("无待确认存储时审批必须报错（门降级为不执行）")
	}
}

func TestHandleWriteApproval_UnknownToolRejects(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)
	if _, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{
		Tool:      "web_search",
		Arguments: `{"query":"x"}`,
	}); err == nil {
		t.Fatalf("未知工具不支持写审批，应报错")
	}
}

func TestHandleWriteApproval_MissingRequiredFields(t *testing.T) {
	o, _, _, _, ctx := setupOperationTest(t)

	cases := []struct {
		name string
		tool string
		args string
	}{
		{"sql_mutate 缺 idempotency_key", "sql_mutate", `{"sql":"UPDATE orders SET a=1"}`},
		{"etl_run 缺 target_table", "etl_run", `{"sql":"SELECT 1"}`},
		{"data_export 缺 result_id", "data_export", `{"format":"csv"}`},
		{"sql_mutate 参数非法 JSON", "sql_mutate", `{bad`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{Tool: c.tool, Arguments: c.args}); err == nil {
				t.Fatalf("缺必填字段/非法参数应报错")
			}
		})
	}
}

// ========================================
// 双门叠加（任务 4.2）：策略级审批 + sql_mutate 内建红线/阈值门互不排斥。
// 审批在框架门（执行前）放行到人工确认；确认后 ExecuteConfirmedMutation 仍复核红线，
// 命中即拒绝——两道门都必须通过才真正落库。
// ========================================

func TestHandleWriteApproval_RedlineStillBlockedAfterConfirm(t *testing.T) {
	o, _, audit, pending, ctx := setupOperationTest(t)

	// 第一道门：策略级审批。审批不检视 SQL 内容，照常生成 pending_action。
	out, err := o.handleWriteApproval(ctx, framework.ApprovalRequest{
		Tool:      "sql_mutate",
		Arguments: `{"sql":"DELETE FROM users WHERE id=1","idempotency_key":"appr-red"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := decodePayload(t, out)

	owner := resultstore.OwnerKey(1, "sess-op")
	action, err := pending.Consume(ctx, owner, p.PendingActionID, p.ConfirmToken)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	// 第二道门：确认后仍复核红线（users 为红线表）→ 拒绝，未落库。
	res, err := o.ExecuteConfirmedMutation(ctx, action)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != operations.StatusRejected || !strings.Contains(res.Message, "红线") {
		t.Fatalf("确认后红线门应拦截, got %+v", res)
	}
	if audit.lastAudit(t).Status != operations.StatusRejected {
		t.Fatalf("红线拦截必须留痕")
	}
}
