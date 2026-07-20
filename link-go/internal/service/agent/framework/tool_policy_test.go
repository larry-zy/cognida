// 硬工具门单元测试（Phase 6 任务 7.5）：
// disallowed 拦截、白名单模式、deny 优先、只读会话拦写、scope 与策略同为必要条件，
// 以及 invokeTool 执行前拦截（合成 tool_blocked ToolMessage、不触达底层工具）。
package framework

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// TestToolPolicy_DisallowedBlocks disallowed_tools 命中即拦截（即便 scope 已授满）。
func TestToolPolicy_DisallowedBlocks(t *testing.T) {
	p := &ToolPolicy{Deny: []string{"sql_mutate"}, Scope: ScopeETL, Skill: "readonly-report"}

	ok, reason := p.Permits("sql_mutate")
	if ok || reason != BlockReasonDisallowed {
		t.Fatalf("disallowed 工具必须被拦截, ok=%v reason=%s", ok, reason)
	}
	if ok, _ := p.Permits("sql_execute"); !ok {
		t.Error("未被禁用的工具应放行")
	}
}

// TestToolPolicy_AllowlistMode 非空 allowed_tools 即白名单模式：仅放行列表内工具。
func TestToolPolicy_AllowlistMode(t *testing.T) {
	p := &ToolPolicy{Allow: []string{"sql_execute", "get_schema"}, Scope: ScopeETL}

	if ok, _ := p.Permits("sql_execute"); !ok {
		t.Error("白名单内工具应放行")
	}
	ok, reason := p.Permits("data_analysis")
	if ok || reason != BlockReasonNotInAllowlist {
		t.Errorf("白名单外工具必须拦截, ok=%v reason=%s", ok, reason)
	}
}

// TestToolPolicy_DenyWins 同一工具同时在 allow 与 deny → 以拒绝为准。
func TestToolPolicy_DenyWins(t *testing.T) {
	p := &ToolPolicy{
		Allow: []string{"sql_mutate", "sql_execute"},
		Deny:  []string{"sql_mutate"},
		Scope: ScopeETL,
	}

	ok, reason := p.Permits("sql_mutate")
	if ok || reason != BlockReasonDisallowed {
		t.Fatalf("deny 必须优先于 allow, ok=%v reason=%s", ok, reason)
	}
	if ok, _ := p.Permits("sql_execute"); !ok {
		t.Error("仅在 allow 的工具应放行")
	}
}

// TestToolPolicy_ReadScopeBlocksWrite 只读会话拦写：scope=read 下写/派生/导出全拦。
func TestToolPolicy_ReadScopeBlocksWrite(t *testing.T) {
	p := &ToolPolicy{Scope: ScopeRead}

	for _, tool := range []string{"sql_mutate", "etl_run", "data_export"} {
		ok, reason := p.Permits(tool)
		if ok || reason != BlockReasonScopeDenied {
			t.Errorf("只读会话必须拦截 %s, ok=%v reason=%s", tool, ok, reason)
		}
	}
	if ok, _ := p.Permits("sql_execute"); !ok {
		t.Error("只读会话应放行只读工具")
	}

	// 权限阶梯：write 放行 DML/导出但不放行 ETL；etl 全放行。
	pw := &ToolPolicy{Scope: ScopeWrite}
	if ok, _ := pw.Permits("sql_mutate"); !ok {
		t.Error("write scope 应放行 sql_mutate")
	}
	if ok, reason := pw.Permits("etl_run"); ok || reason != BlockReasonScopeDenied {
		t.Errorf("write scope 不应放行 etl_run, ok=%v reason=%s", ok, reason)
	}
	pe := &ToolPolicy{Scope: ScopeETL}
	if ok, _ := pe.Permits("etl_run"); !ok {
		t.Error("etl scope 应放行 etl_run")
	}

	// 空/未知 scope 按最小权限（read）兜底。
	p0 := &ToolPolicy{}
	if ok, reason := p0.Permits("sql_mutate"); ok || reason != BlockReasonScopeDenied {
		t.Errorf("空 scope 必须按 read 兜底拦写, ok=%v reason=%s", ok, reason)
	}
}

// TestToolPolicy_ScopeAndSkillBothRequired scope 与 skill 策略同为必要条件。
func TestToolPolicy_ScopeAndSkillBothRequired(t *testing.T) {
	// skill 白名单放行了写工具，但只读 scope 未授予 → 仍拦截
	p := &ToolPolicy{Allow: []string{"sql_mutate"}, Scope: ScopeRead}
	ok, reason := p.Permits("sql_mutate")
	if ok || reason != BlockReasonScopeDenied {
		t.Fatalf("skill 放行但 scope 未授予必须拦截, ok=%v reason=%s", ok, reason)
	}

	// scope 已授予写能力，但 skill 黑名单禁用 → 仍拦截
	p2 := &ToolPolicy{Deny: []string{"sql_mutate"}, Scope: ScopeWrite}
	ok2, reason2 := p2.Permits("sql_mutate")
	if ok2 || reason2 != BlockReasonDisallowed {
		t.Fatalf("scope 授予但 skill 禁用必须拦截, ok=%v reason=%s", ok2, reason2)
	}

	// 双双放行才执行
	p3 := &ToolPolicy{Allow: []string{"sql_mutate"}, Scope: ScopeWrite}
	if ok3, _ := p3.Permits("sql_mutate"); !ok3 {
		t.Error("skill 与 scope 双双放行时应执行")
	}
}

// TestToolPolicy_NilPermitsAll nil 策略 = 不设门（兼容未启用硬工具门的 Agent）。
func TestToolPolicy_NilPermitsAll(t *testing.T) {
	var p *ToolPolicy
	if ok, _ := p.Permits("sql_mutate"); !ok {
		t.Error("nil 策略应放行（不设门）")
	}
}

// TestToolPolicy_EvaluateThreeValued 三值门：deny/scope 优先于审批、未标记退化二态、
// 标记写类返回 approval_required。
func TestToolPolicy_EvaluateThreeValued(t *testing.T) {
	// 标记审批的写工具（scope 已授满）→ approval_required
	p := &ToolPolicy{Approve: []string{"sql_mutate"}, Scope: ScopeWrite}
	if v, reason := p.Evaluate("sql_mutate"); v != VerdictApprovalRequired || reason != ReasonApprovalRequired {
		t.Fatalf("标记审批的写工具应判 approval_required, v=%d reason=%s", v, reason)
	}

	// 未标记审批的工具 → 退化为放行二态
	if v, _ := p.Evaluate("sql_execute"); v != VerdictAllow {
		t.Errorf("未标记审批工具应放行, v=%d", v)
	}

	// deny 优先于审批：同时 deny 与 approve → 直接 Deny
	pd := &ToolPolicy{Deny: []string{"sql_mutate"}, Approve: []string{"sql_mutate"}, Scope: ScopeWrite}
	if v, reason := pd.Evaluate("sql_mutate"); v != VerdictDeny || reason != BlockReasonDisallowed {
		t.Fatalf("deny 必须优先于审批, v=%d reason=%s", v, reason)
	}

	// scope 拒绝优先于审批：只读会话 + 标记审批的写工具 → 直接 Deny(scope)
	ps := &ToolPolicy{Approve: []string{"sql_mutate"}, Scope: ScopeRead}
	if v, reason := ps.Evaluate("sql_mutate"); v != VerdictDeny || reason != BlockReasonScopeDenied {
		t.Fatalf("scope 拒绝必须优先于审批, v=%d reason=%s", v, reason)
	}

	// nil 策略 = 不设门 → 放行
	var pn *ToolPolicy
	if v, _ := pn.Evaluate("sql_mutate"); v != VerdictAllow {
		t.Errorf("nil 策略应放行, v=%d", v)
	}
}

// TestGateToolCall_ApprovalDegradesToBlock 未挂接审批处理器时，approval_required 降级为
// 硬拦截（宁拒不闯）：回灌 tool_blocked、不触达底层工具。
func TestGateToolCall_ApprovalDegradesToBlock(t *testing.T) {
	prev := toolApprovalHandler
	toolApprovalHandler = nil // 显式无处理器
	defer func() { toolApprovalHandler = prev }()

	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Approve: []string{"sql_mutate"}, Scope: ScopeWrite})
	out, ok := gateToolCall(ctx, "sql_mutate", `{"sql":"UPDATE t SET a=1","idempotency_key":"k1"}`)
	if ok {
		t.Fatal("无审批处理器时需审批调用不得放行")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("降级载荷必须是 JSON: %v (out=%s)", err, out)
	}
	if payload["error"] != "tool_blocked" || payload["reason"] != ReasonApprovalRequired {
		t.Errorf("无审批通道应降级为 tool_blocked(approval-required): %s", out)
	}
}

// TestGateToolCall_ApprovalHandlerInvoked 挂接审批处理器时，approval_required 调用委托
// 处理器生成待确认载荷回灌，且不放行底层执行；处理器收到原始参数与工具名。
func TestGateToolCall_ApprovalHandlerInvoked(t *testing.T) {
	prev := toolApprovalHandler
	var gotReq ApprovalRequest
	toolApprovalHandler = func(ctx context.Context, req ApprovalRequest) (string, error) {
		gotReq = req
		return `{"status":"pending_confirm","confirm_token":"tok-1"}`, nil
	}
	defer func() { toolApprovalHandler = prev }()

	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Approve: []string{"data_export"}, Scope: ScopeWrite, Skill: "export-skill"})
	out, ok := gateToolCall(ctx, "data_export", `{"query":"SELECT 1"}`)
	if ok {
		t.Fatal("需审批调用不得直接放行执行")
	}
	if gotReq.Tool != "data_export" || gotReq.Arguments != `{"query":"SELECT 1"}` || gotReq.Skill != "export-skill" {
		t.Errorf("审批处理器未收到正确请求: %+v", gotReq)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("待确认载荷必须是 JSON: %v (out=%s)", err, out)
	}
	if payload["status"] != "pending_confirm" || payload["confirm_token"] != "tok-1" {
		t.Errorf("应回灌处理器生成的待确认载荷: %s", out)
	}
}

// TestGateToolCall_ApprovalRecordsGuardrailEvent 审批放行到人工确认时，须统一护栏留痕
// write_approval_required（与输入拦截/输出脱敏事件同源），供组合根落审计（任务 6.2）。
func TestGateToolCall_ApprovalRecordsGuardrailEvent(t *testing.T) {
	prevH := toolApprovalHandler
	toolApprovalHandler = func(ctx context.Context, req ApprovalRequest) (string, error) {
		return `{"status":"pending_confirm","confirm_token":"tok"}`, nil
	}
	defer func() { toolApprovalHandler = prevH }()

	prevR := guardrailRecorder
	var events []GuardrailEvent
	guardrailRecorder = func(ctx context.Context, evt GuardrailEvent) { events = append(events, evt) }
	defer func() { guardrailRecorder = prevR }()

	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Approve: []string{"sql_mutate"}, Scope: ScopeWrite})
	if _, ok := gateToolCall(ctx, "sql_mutate", `{"sql":"UPDATE t SET a=1","idempotency_key":"k"}`); ok {
		t.Fatal("需审批调用不得直接放行")
	}
	if len(events) != 1 {
		t.Fatalf("审批放行应恰好留痕一条护栏事件, got %d", len(events))
	}
	if events[0].Type != GuardrailEventWriteApprovalNeeded || events[0].Tool != "sql_mutate" {
		t.Fatalf("护栏事件类型/工具不符: %+v", events[0])
	}
}

// TestGateToolCall_ApprovalErrorNoGuardrailEvent 审批处理器异常时回灌 approval_unavailable，
// 不得留痕 write_approval_required（未生成 pending_action）。
func TestGateToolCall_ApprovalErrorNoGuardrailEvent(t *testing.T) {
	prevH := toolApprovalHandler
	toolApprovalHandler = func(ctx context.Context, req ApprovalRequest) (string, error) {
		return "", context.DeadlineExceeded
	}
	defer func() { toolApprovalHandler = prevH }()

	prevR := guardrailRecorder
	var events []GuardrailEvent
	guardrailRecorder = func(ctx context.Context, evt GuardrailEvent) { events = append(events, evt) }
	defer func() { guardrailRecorder = prevR }()

	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Approve: []string{"sql_mutate"}, Scope: ScopeWrite})
	out, ok := gateToolCall(ctx, "sql_mutate", `{"sql":"UPDATE t SET a=1","idempotency_key":"k"}`)
	if ok {
		t.Fatal("审批异常不得放行")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(out), &payload); err != nil || payload["error"] != "approval_unavailable" {
		t.Fatalf("审批异常应回灌 approval_unavailable: %s", out)
	}
	if len(events) != 0 {
		t.Fatalf("审批异常不应留痕护栏事件, got %d", len(events))
	}
}

// TestInvokeTool_GateBlocksBeforeExecution 拦截发生在工具执行前：
// 被拒调用不触达底层工具，返回合成 tool_blocked ToolMessage（nil error 回灌 LLM）。
func TestInvokeTool_GateBlocksBeforeExecution(t *testing.T) {
	var calls []string
	a := &agentImpl{tools: []tool.BaseTool{
		&recordingTool{name: "sql_mutate", calls: &calls},
		&recordingTool{name: "sql_execute", calls: &calls},
	}}
	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Scope: ScopeRead, Skill: "readonly-report"})

	out, synthetic, err := a.invokeTool(ctx, schema.ToolCall{
		Function: schema.FunctionCall{Name: "sql_mutate", Arguments: "{}"},
	})
	if err != nil {
		t.Fatalf("拦截应以合成 ToolMessage 回灌而非报错: %v", err)
	}
	if !synthetic {
		t.Errorf("被拒调用应标记 synthetic=true（跳过逐工具输出护栏）")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("合成载荷必须是 JSON: %v (out=%s)", err, out)
	}
	if payload["error"] != "tool_blocked" || payload["tool"] != "sql_mutate" ||
		payload["reason"] != BlockReasonScopeDenied || payload["skill"] != "readonly-report" {
		t.Errorf("合成 tool_blocked 载荷不符: %s", out)
	}
	if len(calls) != 0 {
		t.Errorf("被拒调用不得触达底层工具: %v", calls)
	}

	// 放行路径照常执行
	out2, synthetic2, err2 := a.invokeTool(ctx, schema.ToolCall{
		Function: schema.FunctionCall{Name: "sql_execute", Arguments: "{}"},
	})
	if err2 != nil || out2 != "ok:sql_execute" {
		t.Errorf("放行工具应正常执行, out=%s err=%v", out2, err2)
	}
	if synthetic2 {
		t.Errorf("放行工具的真实观察不应标记 synthetic（须过逐工具输出护栏）")
	}

	// 未注入策略 = 不设门（既有 Agent 兼容）
	out3, synthetic3, err3 := a.invokeTool(context.Background(), schema.ToolCall{
		Function: schema.FunctionCall{Name: "sql_mutate", Arguments: "{}"},
	})
	if err3 != nil || out3 != "ok:sql_mutate" {
		t.Errorf("无策略时不设门, out=%s err=%v", out3, err3)
	}
	if synthetic3 {
		t.Errorf("无策略放行的真实观察不应标记 synthetic")
	}
}
