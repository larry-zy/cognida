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

// TestInvokeTool_GateBlocksBeforeExecution 拦截发生在工具执行前：
// 被拒调用不触达底层工具，返回合成 tool_blocked ToolMessage（nil error 回灌 LLM）。
func TestInvokeTool_GateBlocksBeforeExecution(t *testing.T) {
	var calls []string
	a := &agentImpl{tools: []tool.BaseTool{
		&recordingTool{name: "sql_mutate", calls: &calls},
		&recordingTool{name: "sql_execute", calls: &calls},
	}}
	ctx := WithToolPolicy(context.Background(), &ToolPolicy{Scope: ScopeRead, Skill: "readonly-report"})

	out, err := a.invokeTool(ctx, schema.ToolCall{
		Function: schema.FunctionCall{Name: "sql_mutate", Arguments: "{}"},
	})
	if err != nil {
		t.Fatalf("拦截应以合成 ToolMessage 回灌而非报错: %v", err)
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
	out2, err2 := a.invokeTool(ctx, schema.ToolCall{
		Function: schema.FunctionCall{Name: "sql_execute", Arguments: "{}"},
	})
	if err2 != nil || out2 != "ok:sql_execute" {
		t.Errorf("放行工具应正常执行, out=%s err=%v", out2, err2)
	}

	// 未注入策略 = 不设门（既有 Agent 兼容）
	out3, err3 := a.invokeTool(context.Background(), schema.ToolCall{
		Function: schema.FunctionCall{Name: "sql_mutate", Arguments: "{}"},
	})
	if err3 != nil || out3 != "ok:sql_mutate" {
		t.Errorf("无策略时不设门, out=%s err=%v", out3, err3)
	}
}
