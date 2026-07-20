// Skill 硬工具门 + 会话 scope（Phase 6）。
//
// ToolPolicy 把 skill 的 allowed_tools/disallowed_tools 从文档提示升级为执行前
// 硬拦截，并在其上叠加会话级 scope（read / write / etl）最小权限：
//   - deny 优先：命中 Deny 即拒绝，即便同时出现在 Allow；
//   - 非空 Allow 即白名单模式：仅放行列表内工具；
//   - scope 校验：工具所需能力级别未被授予即拒绝；
// scope 与 skill 策略同为放行的必要条件，任一不过即拦截。
//
// 拦截点在 invokeTool 执行前（统一主干 run→execLoop→handleToolCall 覆盖 Chat/Stream 全部路径），
// 被拒调用不触达底层执行，以合成 {"error":"tool_blocked",...} ToolMessage 回灌
// LLM 自我修正，并同步留痕（日志 + 可插拔审计记录器）。
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	domainagent "link/internal/model/agent"
)

// ========================================
// 会话 scope（read < write < etl 权限阶梯）
// ========================================

// 会话工具 scope 级别：read 只读；write 含只读 + DML 写；etl 含前两者 + 派生对象。
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
	ScopeETL   = "etl"
)

// scopeLevels 权限阶梯：高级别蕴含低级别能力。未知/空 scope 按最小权限（read）处理。
var scopeLevels = map[string]int{
	ScopeRead:  1,
	ScopeWrite: 2,
	ScopeETL:   3,
}

// scopeLevel 返回 scope 的阶梯级别；空串/未知一律视为 read（最小权限兜底）。
func scopeLevel(scope string) int {
	if lv, ok := scopeLevels[scope]; ok {
		return lv
	}
	return scopeLevels[ScopeRead]
}

// toolScopeRequirements 各工具所需的最低 scope 能力；未列出的工具默认只读即可。
var toolScopeRequirements = map[string]string{
	"sql_mutate":  ScopeWrite, // DML 写库
	"data_export": ScopeWrite, // 数据外送（出域风险，只读会话不放行）
	"etl_run":     ScopeETL,   // 派生新对象
}

// RequiredToolScope 返回工具所需的最低 scope；未登记的工具默认 read。
func RequiredToolScope(tool string) string {
	if s, ok := toolScopeRequirements[tool]; ok {
		return s
	}
	return ScopeRead
}

// ========================================
// ToolPolicy 与放行判定
// ========================================

// 拦截原因（拦截留痕与合成 ToolMessage 共用的稳定标识）。
const (
	BlockReasonDisallowed     = "disallowed"        // 命中 skill disallowed_tools
	BlockReasonNotInAllowlist = "not-in-allowlist"  // 白名单模式下不在 allowed_tools
	BlockReasonScopeDenied    = "scope-denied"      // 会话 scope 未授予所需能力
	ReasonApprovalRequired    = "approval-required" // 策略标记为需人工审批（第三态，非拒绝）
)

// ToolVerdict 是执行前硬工具门的三值判定结果：放行 / 拒绝 / 需人工审批。
type ToolVerdict int

const (
	// VerdictAllow 放行：工具可直接执行。
	VerdictAllow ToolVerdict = iota
	// VerdictDeny 拒绝：deny/白名单/scope 硬拦截，工具不得执行，回灌 tool_blocked。
	VerdictDeny
	// VerdictApprovalRequired 需审批：工具不直接执行，进入人机确认流程（生成 pending_action）。
	VerdictApprovalRequired
)

// ToolPolicy 当前请求生效的工具策略：Allow/Deny 来自命中 skill 的
// allowed_tools/disallowed_tools，Scope 来自会话授权，Approve 为被会话/skill 标记
// 需人工审批的写类工具。nil 策略 = 不设门（兼容未启用硬工具门的既有 Agent）。
type ToolPolicy struct {
	Allow   []string // 非空即白名单模式：仅放行列表内工具
	Deny    []string // 黑名单：命中即拒绝（deny 优先）
	Approve []string // 需人工审批的工具：通过 deny/scope 后仍需人工确认才执行
	Scope   string   // 会话 scope：read / write / etl；空按 read 最小权限
	Skill   string   // 命中的 skill 名（留痕用；纯 scope 策略可为空）
}

// Permits 判定工具是否通过硬拦截（deny/白名单/scope）；不放行时返回稳定的拒绝原因标识。
// 判定顺序：deny 优先 → 白名单模式 → scope 校验，三者均为必要条件。
// 注意：Permits 只覆盖硬拦截二态，审批第三态由 Evaluate 在其之上叠加。
func (p *ToolPolicy) Permits(tool string) (bool, string) {
	if p == nil {
		return true, ""
	}
	for _, d := range p.Deny {
		if d == tool {
			return false, BlockReasonDisallowed
		}
	}
	if len(p.Allow) > 0 {
		inAllow := false
		for _, a := range p.Allow {
			if a == tool {
				inAllow = true
				break
			}
		}
		if !inAllow {
			return false, BlockReasonNotInAllowlist
		}
	}
	if scopeLevel(p.Scope) < scopeLevel(RequiredToolScope(tool)) {
		return false, BlockReasonScopeDenied
	}
	return true, ""
}

// requiresApproval 判定工具是否被策略标记为需人工审批。
func (p *ToolPolicy) requiresApproval(tool string) bool {
	if p == nil {
		return false
	}
	for _, t := range p.Approve {
		if t == tool {
			return true
		}
	}
	return false
}

// Evaluate 是三值门：在 Permits 硬拦截之上叠加审批第三态。
// 判定顺序 SHALL 为 deny/scope（硬拒绝）优先 → 审批标记；deny 与 scope 拒绝优先于审批。
// 未被标记审批的工具，Evaluate 退化为 Permits 的放行/拒绝二态（向后兼容）。
func (p *ToolPolicy) Evaluate(tool string) (ToolVerdict, string) {
	if p == nil {
		return VerdictAllow, ""
	}
	if ok, reason := p.Permits(tool); !ok {
		return VerdictDeny, reason
	}
	if p.requiresApproval(tool) {
		return VerdictApprovalRequired, ReasonApprovalRequired
	}
	return VerdictAllow, ""
}

// ========================================
// Context 注入
// ========================================

// toolPolicyCtxKey 包内私有 ctx key，避免跨包冲突。
type toolPolicyCtxKey struct{}

// WithToolPolicy 将工具策略注入 Go context（入口 hook / 委派方调用）。
func WithToolPolicy(ctx context.Context, policy *ToolPolicy) context.Context {
	return context.WithValue(ctx, toolPolicyCtxKey{}, policy)
}

// ToolPolicyFromContext 取当前生效的工具策略；未注入返回 nil（不设门）。
func ToolPolicyFromContext(ctx context.Context) *ToolPolicy {
	policy, _ := ctx.Value(toolPolicyCtxKey{}).(*ToolPolicy)
	return policy
}

// ========================================
// 拦截留痕（日志 + 可插拔审计记录器）
// ========================================

// BlockedToolCall 一次被硬工具门/scope 拦截的调用（留痕载荷）。
type BlockedToolCall struct {
	Tool      string // 被拒工具名
	Reason    string // 拒绝原因（disallowed / not-in-allowlist / scope-denied）
	Skill     string // 生效 skill（可为空）
	Scope     string // 会话 scope
	SessionID string
	RequestID string
	TenantID  int64
}

// toolBlockRecorder 可插拔审计记录器：组合根（如操作工具注入点）可挂接
// agent_operation_audit 落库；未挂接时仅日志留痕。
var toolBlockRecorder func(ctx context.Context, call BlockedToolCall)

// SetToolBlockRecorder 挂接拦截审计记录器（组合根调用）。
func SetToolBlockRecorder(fn func(ctx context.Context, call BlockedToolCall)) {
	toolBlockRecorder = fn
}

// ========================================
// 写/导出审批第三态（approval_required）处理器
// ========================================

// ApprovalRequest 一次被判定为需人工审批的工具调用（供审批处理器构建 pending_action）。
type ApprovalRequest struct {
	Tool      string // 需审批的工具名（sql_mutate / data_export / etl_run）
	Arguments string // 原始 JSON 参数，供处理器解析构建待确认动作
	Skill     string // 生效 skill（可为空）
	Scope     string // 会话 scope
	SessionID string
	RequestID string
	TenantID  int64
}

// toolApprovalHandler 可插拔审批处理器：组合根（操作工具注入点）挂接后，
// 负责把需审批的工具调用转为待确认动作（复用 pendingaction/ConfirmToken/确认卡片），
// 并返回回灌 LLM 的「待人工确认」ToolMessage 载荷。未挂接时门降级为拒绝（宁拒不闯）。
var toolApprovalHandler func(ctx context.Context, req ApprovalRequest) (string, error)

// SetToolApprovalHandler 挂接写/导出审批处理器（组合根调用）。
func SetToolApprovalHandler(fn func(ctx context.Context, req ApprovalRequest) (string, error)) {
	toolApprovalHandler = fn
}

// recordBlockedToolCall 留痕一次拦截：必落日志，已挂接记录器则同步写审计。
func recordBlockedToolCall(ctx context.Context, policy *ToolPolicy, tool, reason string) {
	call := BlockedToolCall{
		Tool:      tool,
		Reason:    reason,
		Skill:     policy.Skill,
		Scope:     policy.Scope,
		SessionID: domainagent.MustGetSessionID(ctx),
		RequestID: domainagent.MustGetRequestID(ctx),
		TenantID:  domainagent.MustGetTenantID(ctx),
	}
	log.Printf("[工具门] 拦截: tool=%s reason=%s skill=%s scope=%s session=%s request=%s",
		call.Tool, call.Reason, call.Skill, call.Scope, call.SessionID, call.RequestID)
	if toolBlockRecorder != nil {
		toolBlockRecorder(ctx, call)
	}
}

// blockedToolResult 合成被拒调用的 ToolMessage 载荷：回灌 LLM 说明拦截原因，
// 引导其换用允许的工具或降低操作级别自我修正。
func blockedToolResult(policy *ToolPolicy, tool, reason string) string {
	var hint string
	switch reason {
	case BlockReasonDisallowed:
		hint = fmt.Sprintf("工具 %s 被当前 skill 策略禁用，请改用其他允许的工具完成任务", tool)
	case BlockReasonNotInAllowlist:
		hint = fmt.Sprintf("当前 skill 处于白名单模式，工具 %s 不在 allowed_tools 内", tool)
	case BlockReasonScopeDenied:
		hint = fmt.Sprintf("会话 scope 为 %s，未授予工具 %s 所需的 %s 能力；请改走只读路径或告知用户需要更高授权",
			policy.Scope, tool, RequiredToolScope(tool))
	case ReasonApprovalRequired:
		hint = fmt.Sprintf("工具 %s 需人工审批，但审批通道未启用，操作被拒绝；请告知用户需人工确认后方可执行", tool)
	default:
		hint = fmt.Sprintf("工具 %s 被策略拦截", tool)
	}
	payload := map[string]interface{}{
		"error":  "tool_blocked",
		"tool":   tool,
		"reason": reason,
		"skill":  policy.Skill,
		"scope":  policy.Scope,
		"detail": hint,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"error":"tool_blocked","tool":%q,"reason":%q}`, tool, reason)
	}
	return string(b)
}

// gateToolCall 执行前硬工具门（三值）：放行返回 ("", true)；拒绝/需审批时返回合成
// ToolMessage 载荷（第二返回值 false 表示不放行、不执行底层工具）。所有工具执行路径
// （统一主干 run→handleToolCall→invokeTool）统一经此门。arguments 为原始 JSON 参数，
// 仅在需审批时用于构建待确认动作。
func gateToolCall(ctx context.Context, tool, arguments string) (string, bool) {
	policy := ToolPolicyFromContext(ctx)
	if policy == nil {
		return "", true
	}
	switch verdict, reason := policy.Evaluate(tool); verdict {
	case VerdictAllow:
		return "", true
	case VerdictApprovalRequired:
		return handleApprovalRequired(ctx, policy, tool, arguments), false
	default: // VerdictDeny
		recordBlockedToolCall(ctx, policy, tool, reason)
		return blockedToolResult(policy, tool, reason), false
	}
}

// handleApprovalRequired 处理需人工审批的工具调用：委托审批处理器生成待确认动作并
// 返回「待人工确认」ToolMessage 载荷回灌 LLM。未挂接处理器时降级为拒绝（宁拒不闯），
// 以硬拦截留痕并回灌 tool_blocked。
func handleApprovalRequired(ctx context.Context, policy *ToolPolicy, tool, arguments string) string {
	if toolApprovalHandler == nil {
		// 无审批通道：宁拒不闯，按硬拦截留痕并回灌 tool_blocked。
		recordBlockedToolCall(ctx, policy, tool, ReasonApprovalRequired)
		return blockedToolResult(policy, tool, ReasonApprovalRequired)
	}
	payload, err := toolApprovalHandler(ctx, ApprovalRequest{
		Tool:      tool,
		Arguments: arguments,
		Skill:     policy.Skill,
		Scope:     policy.Scope,
		SessionID: domainagent.MustGetSessionID(ctx),
		RequestID: domainagent.MustGetRequestID(ctx),
		TenantID:  domainagent.MustGetTenantID(ctx),
	})
	if err != nil {
		// 审批通道异常：不闯，回灌错误说明由 LLM 转达用户。
		log.Printf("[工具门] 审批处理失败: tool=%s err=%v", tool, err)
		return approvalErrorResult(tool, err)
	}
	// 统一护栏留痕（任务 6.2）：写审批已生成待确认动作，与拦截/脱敏事件同源落审计。
	recordGuardrailEvent(ctx, GuardrailEventWriteApprovalNeeded, tool, "策略级写审批：已生成 pending_action 待人工确认")
	return payload
}

// approvalErrorResult 审批处理异常时的合成 ToolMessage：不执行工具，说明需人工确认但暂不可用。
func approvalErrorResult(tool string, err error) string {
	payload := map[string]interface{}{
		"error":  "approval_unavailable",
		"tool":   tool,
		"detail": fmt.Sprintf("工具 %s 需人工审批，但审批通道暂不可用：%v。请稍后重试或联系管理员。", tool, err),
	}
	b, mErr := json.Marshal(payload)
	if mErr != nil {
		return fmt.Sprintf(`{"error":"approval_unavailable","tool":%q}`, tool)
	}
	return string(b)
}
