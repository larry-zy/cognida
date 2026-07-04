// 委派信封与执行内核（Phase 7）：结构化信封 {goal, inputs, constraints{scope,max_rows}, return}
// 作为校验型契约——缺必填字段拒绝委派并回灌 LLM；每次委派按信封 scope 授予工具权限
// （非持久高权），且不得越过指挥官自身 scope；子代理按注册的默认上下文模式接收
// 隔离/摘要上下文（上下文防火墙），只回传 handle/摘要。
package framework

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainagent "link/internal/model/agent"
)

// ========================================
// 委派信封（校验型契约）
// ========================================

// DelegationInputs 委派输入：按需携带既有结果句柄 / SQL / 自然语言问题。
type DelegationInputs struct {
	ResultID string `json:"result_id,omitempty"`
	SQL      string `json:"sql,omitempty"`
	Question string `json:"question,omitempty"`
}

// DelegationConstraints 委派约束：scope 为必填（本次委派授予的权限级），
// max_rows 约束子代理结果规模。
type DelegationConstraints struct {
	Scope   string `json:"scope"`
	MaxRows int    `json:"max_rows,omitempty"`
}

// DelegationEnvelope 结构化委派信封。作为校验型契约：缺 goal 或
// constraints.scope 等必填字段时拒绝委派（错误回灌 LLM 自我修正）。
type DelegationEnvelope struct {
	AgentName   string                `json:"agent_name"`
	Goal        string                `json:"goal"`
	Inputs      DelegationInputs      `json:"inputs,omitempty"`
	Constraints DelegationConstraints `json:"constraints"`
	Return      string                `json:"return,omitempty"`
}

// validateEnvelope 校验委派信封必填字段；不以默认值放行（宁拒不猜）。
func validateEnvelope(env *DelegationEnvelope) error {
	if env.AgentName == "" {
		return fmt.Errorf("委派信封缺必填字段 agent_name")
	}
	if strings.TrimSpace(env.Goal) == "" {
		return fmt.Errorf("委派信封缺必填字段 goal：请声明该子任务要达成什么")
	}
	scope := env.Constraints.Scope
	if scope == "" {
		return fmt.Errorf("委派信封缺必填字段 constraints.scope：请显式声明本次委派授予的权限（read/write/etl），不以默认值放行")
	}
	if scope != ScopeRead && scope != ScopeWrite && scope != ScopeETL {
		return fmt.Errorf("constraints.scope 取值非法: %q（允许 read/write/etl）", scope)
	}
	if env.Constraints.MaxRows < 0 {
		return fmt.Errorf("constraints.max_rows 不能为负数: %d", env.Constraints.MaxRows)
	}
	return nil
}

// ========================================
// 委派审计（与 agent_operation_audit 串联）
// ========================================

// 委派结果状态。
const (
	delegationStatusSuccess  = "success"
	delegationStatusFailed   = "failed"
	delegationStatusRejected = "rejected"
)

// DelegationRecord 一次委派的留痕载荷：目标子代理、目标风险级（治理目录）、
// 授予 scope、结果状态与明细。
type DelegationRecord struct {
	Agent     string
	Goal      string
	Scope     string
	RiskClass string
	Status    string
	Detail    string
}

// delegationRecorder 可插拔委派审计（组合根经 SetDelegationRecorder 挂接，
// 与 agent_operation_audit 串联）；未挂接时仅落日志。
var delegationRecorder func(ctx context.Context, rec DelegationRecord)

// SetDelegationRecorder 挂接委派审计落地实现（组合根调用）。
func SetDelegationRecorder(fn func(ctx context.Context, rec DelegationRecord)) {
	delegationRecorder = fn
}

// recordDelegation 委派留痕：治理元数据（risk_class）+ 授予 scope + 状态。
func recordDelegation(ctx context.Context, registry *CollaborationRegistry, env *DelegationEnvelope, status, detail string) {
	riskClass := ""
	if gov := registry.GetGovernance(env.AgentName); gov != nil {
		riskClass = gov.RiskClass
	}
	if delegationRecorder != nil {
		delegationRecorder(ctx, DelegationRecord{
			Agent:     env.AgentName,
			Goal:      env.Goal,
			Scope:     env.Constraints.Scope,
			RiskClass: riskClass,
			Status:    status,
			Detail:    detail,
		})
	}
}

// ========================================
// 委派执行内核（单次与并行共用）
// ========================================

// delegationTimeout 单次委派超时：需覆盖子代理自身 ReAct 循环
// （Report→Insight→Query 逐层嵌套时每层独立计时）。
const delegationTimeout = 180 * time.Second

// executeDelegation 执行一次经信封契约校验的委派：
//  1. 信封必填校验（缺字段拒绝，错误回灌）；
//  2. 循环委派（IsCyclic）与 MaxDepth 护栏；
//  3. scope 越权护栏：委派授予不得超过指挥官自身 scope；
//  4. 子代理 ctx 重建：克隆协作上下文（并行安全）+ 按注册默认模式设隔离级
//     + 以信封 scope 装配每次委派授予的工具门策略（结束即失效，非持久高权）；
//  5. 只回传子代理最终内容（handle/摘要），不回灌其内部工具往返。
func executeDelegation(ctx context.Context, registry *CollaborationRegistry, env *DelegationEnvelope) (string, error) {
	if err := validateEnvelope(env); err != nil {
		recordDelegation(ctx, registry, env, delegationStatusRejected, err.Error())
		return "", err
	}

	// 循环与深度护栏（沿用既有 CollaborationContext）
	collabCtx, hasCtx := domainagent.GetCollaborationContext(ctx)
	if hasCtx && collabCtx.IsCyclic(env.AgentName) {
		err := fmt.Errorf("检测到循环委派: %s → %s", collabCtx.ChainDescription(), env.AgentName)
		recordDelegation(ctx, registry, env, delegationStatusRejected, err.Error())
		return "", err
	}
	if hasCtx && collabCtx.IsDepthExceeded() {
		err := fmt.Errorf("超过最大委派深度: %d", collabCtx.MaxDepth)
		recordDelegation(ctx, registry, env, delegationStatusRejected, err.Error())
		return "", err
	}

	targetAgent, err := registry.GetByName(env.AgentName)
	if err != nil {
		return "", fmt.Errorf("agent '%s' not found. Available agents:\n%s",
			env.AgentName, formatAgentInfos(registry.ListWithDescriptions()))
	}

	// scope 越权护栏：委派授予不得超过指挥官自身 scope（指挥官无策略时不设门，兼容旧路径）
	if commander := ToolPolicyFromContext(ctx); commander != nil &&
		scopeLevel(env.Constraints.Scope) > scopeLevel(commander.Scope) {
		err := fmt.Errorf("委派 scope 越权: 会话 scope=%s 不能授予子代理 scope=%s",
			commander.Scope, env.Constraints.Scope)
		recordDelegation(ctx, registry, env, delegationStatusRejected, err.Error())
		return "", err
	}

	// 子代理 ctx 重建：克隆协作上下文（并行 fan-out 下互不干扰），
	// 上下文模式取该子代理注册的默认值（上下文防火墙）。
	childCtx := ctx
	if hasCtx {
		childCollab := collabCtx.Clone()
		childCollab.AddDelegate(env.AgentName)
		childCollab.Mode = registry.GetContextMode(env.AgentName)
		childCtx = domainagent.WithCollaborationContext(childCtx, childCollab)
	} else {
		// 无协作上下文时退化为路径追踪（旧兼容）
		if pathContains(getDelegatePath(childCtx), env.AgentName) {
			return "", NewCollabLoopError(getDelegatePath(childCtx), env.AgentName)
		}
		childCtx = withDelegatePath(childCtx, env.AgentName)
	}

	// 每次委派授予：以信封 scope 装配子代理工具门策略，随本次委派 ctx 生命周期
	// 结束而失效（不跨委派累积持久高权）。
	childCtx = WithToolPolicy(childCtx, &ToolPolicy{Scope: env.Constraints.Scope})
	childCtx = domainagent.WithToolScope(childCtx, env.Constraints.Scope)

	childCtx, cancel := context.WithTimeout(childCtx, delegationTimeout)
	defer cancel()

	message := buildEnvelopeMessage(registry.GetContextMode(env.AgentName), collabCtx, hasCtx, env)

	startTime := time.Now()
	resp, err := targetAgent.Chat(childCtx, message)
	duration := time.Since(startTime)

	if err != nil {
		if hasCtx {
			collabCtx.StoreResult(env.AgentName, &domainagent.TaskResult{
				AgentName: env.AgentName,
				Error:     err,
				Duration:  duration,
				StartTime: startTime,
				EndTime:   startTime.Add(duration),
			})
		}
		recordDelegation(ctx, registry, env, delegationStatusFailed, err.Error())
		return "", fmt.Errorf("delegation to agent '%s' failed: %w", env.AgentName, err)
	}

	if hasCtx {
		collabCtx.StoreResult(env.AgentName, &domainagent.TaskResult{
			AgentName: env.AgentName,
			Content:   resp.Content,
			Duration:  duration,
			StartTime: startTime,
			EndTime:   startTime.Add(duration),
		})
	}
	recordDelegation(ctx, registry, env, delegationStatusSuccess, "")

	// 只回传子代理最终内容（约定为 handle/摘要），不含其内部工具往返
	return resp.Content, nil
}

// buildEnvelopeMessage 按上下文模式构建子代理输入：
// isolated 只含信封所载必要输入（不泄漏指挥官全历史）；
// summary 附原始问题/摘要/已执行链路。
func buildEnvelopeMessage(mode domainagent.CollaborationContextMode, collabCtx *domainagent.CollaborationContext, hasCtx bool, env *DelegationEnvelope) string {
	var b strings.Builder

	if hasCtx && mode == domainagent.ContextModeSummary {
		b.WriteString("## 协作上下文\n\n")
		if collabCtx.OriginalQuery != "" {
			b.WriteString(fmt.Sprintf("**原始问题**: %s\n\n", collabCtx.OriginalQuery))
		}
		if collabCtx.Summary != "" {
			b.WriteString(fmt.Sprintf("**对话摘要**: %s\n\n", collabCtx.Summary))
		}
		if len(collabCtx.DelegateChain) > 0 {
			b.WriteString(fmt.Sprintf("**已执行流程**: %s\n\n", collabCtx.ChainDescription()))
		}
	}

	b.WriteString("## 委派任务\n\n")
	b.WriteString(fmt.Sprintf("**目标**: %s\n", env.Goal))
	if env.Inputs.ResultID != "" {
		b.WriteString(fmt.Sprintf("**输入 result_id**: %s\n", env.Inputs.ResultID))
	}
	if env.Inputs.SQL != "" {
		b.WriteString(fmt.Sprintf("**输入 SQL**: %s\n", env.Inputs.SQL))
	}
	if env.Inputs.Question != "" {
		b.WriteString(fmt.Sprintf("**输入问题**: %s\n", env.Inputs.Question))
	}
	b.WriteString(fmt.Sprintf("**权限 scope**: %s\n", env.Constraints.Scope))
	if env.Constraints.MaxRows > 0 {
		b.WriteString(fmt.Sprintf("**结果行数上限**: %d\n", env.Constraints.MaxRows))
	}
	if env.Return != "" {
		b.WriteString(fmt.Sprintf("**期望回传**: %s\n", env.Return))
	}
	b.WriteString("\n请在上述约束内完成任务，只回传紧凑结论（如 result_id + 摘要），不要罗列内部执行过程。")
	return b.String()
}

// pathContains 判断路径追踪中是否已包含目标（旧兼容循环检测）。
func pathContains(path []string, target string) bool {
	for _, name := range path {
		if name == target {
			return true
		}
	}
	return false
}

// formatAgentInfos 格式化 agent 列表（错误提示与工具描述共用）。
func formatAgentInfos(agents []AgentInfo) string {
	var sb strings.Builder
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", a.Name, a.Description))
	}
	return sb.String()
}
