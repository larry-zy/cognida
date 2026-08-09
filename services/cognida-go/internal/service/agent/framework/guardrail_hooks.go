// 护栏运行时接入（agent-guardrail-runtime）。
//
// 把既有 GuardrailService（CheckInput/CheckJailbreak/CheckOutput/SanitizeInput/
// SanitizeOutput）装配进 Agent 的 ReAct 主链路，复用既有 Hook 挂载缝：
//   - 输入护栏 → BeforeHook（越狱检测 + 输入检查；不安全中止或脱敏放行），置于 before 链最前；
//   - 输出护栏 → AfterHook（最终输出检查；命中脱敏或替换安全兜底），置于 after 链最后；
//   - 启用输出护栏的流式会话强制走缓冲交付（AfterHook 仅在缓冲路径收敛，见 eino_agent.go）。
//
// 护栏默认全关：未装配任何护栏 Hook 时，Agent 构建与执行行为逐字节不变（零回归）。
package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	domainagent "cognida/internal/model/agent"
	domainguardrail "cognida/internal/model/guardrail"
)

// ========================================
// 护栏配置（会话/agent 级，各开关独立，默认全关）
// ========================================

// InputGuardrailConfig 输入护栏配置。零值即最小行为（全关），由组合根按需开启。
type InputGuardrailConfig struct {
	// EnableJailbreak 启用越狱检测：判定为攻击意图即中止（不可脱敏）。
	EnableJailbreak bool
	// EnableInputCheck 启用通用输入检查（PII/注入等）。
	EnableInputCheck bool
	// Sanitize 命中不安全输入时的处置：true 经 SanitizeInput 脱敏改写后放行；
	// false 直接中止本次执行（返回 error）。
	Sanitize bool
	// FilterOptions 输入检查选项；nil 用 DefaultFilterOptions。
	FilterOptions *domainguardrail.FilterOptions
	// JailbreakOptions 越狱检测选项；nil 用 DefaultJailbreakOptions。
	JailbreakOptions *domainguardrail.JailbreakOptions
}

// OutputGuardrailConfig 输出护栏配置。
type OutputGuardrailConfig struct {
	// OutputOptions 输出检查选项（含幻觉/PII 开关）；nil 用 DefaultOutputFilterOptions。
	OutputOptions *domainguardrail.OutputFilterOptions
	// FallbackText 无法脱敏时替换的安全兜底文案；空则用内置默认兜底。
	FallbackText string
}

// defaultOutputFallback 输出护栏无法脱敏时的安全兜底文案。
const defaultOutputFallback = "抱歉，本次回答包含无法安全展示的内容，已被安全护栏拦截。请调整问题后重试。"

// ========================================
// 护栏中止错误
// ========================================

// GuardrailViolationError 护栏判定输入不安全/越狱时的中止错误，经 BeforeHook error
// 契约传播，使 Agent 不进入 ReAct 循环。Stage 标识命中的护栏阶段。
type GuardrailViolationError struct {
	Stage  string // jailbreak / input
	Detail string
}

func (e *GuardrailViolationError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return fmt.Sprintf("guardrail blocked at %s", e.Stage)
}

// IsGuardrailViolation 判定 err 是否为护栏中止错误。
func IsGuardrailViolation(err error) bool {
	_, ok := err.(*GuardrailViolationError)
	return ok
}

// ========================================
// 护栏留痕（复用与工具门同源的可插拔记录器范式）
// ========================================

// 护栏事件类型（统一留痕稳定标识）。
const (
	GuardrailEventInputBlocked        = "input_blocked"
	GuardrailEventJailbreakBlocked    = "jailbreak_blocked"
	GuardrailEventInputRedacted       = "input_redacted"
	GuardrailEventOutputRedacted      = "output_redacted"
	GuardrailEventToolOutputRedacted  = "tool_output_redacted"
	GuardrailEventWriteApprovalNeeded = "write_approval_required"
)

// GuardrailEvent 一次护栏拦截/脱敏/审批事件的留痕载荷。
type GuardrailEvent struct {
	Type      string // 见 GuardrailEvent* 常量
	Tool      string // 逐工具脱敏时的工具名（可空）
	Detail    string // 原因/说明
	SessionID string
	RequestID string
	TenantID  int64
}

// guardrailRecorder 可插拔护栏审计记录器：组合根挂接后落审计；未挂接仅日志留痕。
var guardrailRecorder func(ctx context.Context, evt GuardrailEvent)

// SetGuardrailRecorder 挂接护栏审计记录器（组合根调用）。
func SetGuardrailRecorder(fn func(ctx context.Context, evt GuardrailEvent)) {
	guardrailRecorder = fn
}

// recordGuardrailEvent 留痕一次护栏事件：必落日志，已挂接记录器则同步写审计。
func recordGuardrailEvent(ctx context.Context, evtType, tool, detail string) {
	evt := GuardrailEvent{
		Type:      evtType,
		Tool:      tool,
		Detail:    detail,
		SessionID: domainagent.MustGetSessionID(ctx),
		RequestID: domainagent.MustGetRequestID(ctx),
		TenantID:  domainagent.MustGetTenantID(ctx),
	}
	log.Printf("[护栏] event=%s tool=%s detail=%s session=%s request=%s",
		evt.Type, evt.Tool, evt.Detail, evt.SessionID, evt.RequestID)
	if guardrailRecorder != nil {
		guardrailRecorder(ctx, evt)
	}
}

// ========================================
// 逐工具输出护栏（post-invoke，第二阶段：任务 5）
// ========================================

// ToolOutputHook 逐工具输出护栏缝：工具返回后、回灌 LLM 前对观察做脱敏。
// 返回可能被脱敏改写的观察；未命中或未启用时原样返回（零开销、逐字节不变）。
type ToolOutputHook func(ctx context.Context, toolName, output string) string

// isResultEnvelope 判定工具观察是否为 result_id 引用信封：含非空 result_id 键的 JSON 对象。
// 信封只承载引用（原始行数据留在服务端，不内联 PII），故直接放行以保结构不被破坏（任务 5.3）。
func isResultEnvelope(output string) bool {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(output), &obj); err != nil || obj == nil {
		return false
	}
	rid, _ := obj["result_id"].(string)
	return rid != ""
}

// newToolOutputGuardrailHook 把输出护栏装配为逐工具 post-invoke 脱敏缝。
// 对纯文本观察跑 CheckOutput，命中经 SanitizeOutput 脱敏（仅改写命中片段）后回灌；
// result_id 引用信封原样放行（保信封结构，任务 5.3）；护栏自身异常不阻断（fail-open，保留原观察）。
// gs 为 nil 时返回原样放行的空 hook（零回归）。
func newToolOutputGuardrailHook(gs domainguardrail.GuardrailService, cfg OutputGuardrailConfig) ToolOutputHook {
	return func(ctx context.Context, toolName, output string) string {
		if gs == nil || output == "" {
			return output
		}
		// result_id 信封：结构化引用，PII 不内联，放行以免破坏信封结构。
		if isResultEnvelope(output) {
			return output
		}
		opts := cfg.OutputOptions
		if opts == nil {
			opts = domainguardrail.DefaultOutputFilterOptions()
		}
		res, err := gs.CheckOutput(ctx, output, "", opts)
		if err != nil {
			log.Printf("[护栏] 工具输出检查异常（放行原观察）: tool=%s err=%v", toolName, err)
			return output
		}
		if res == nil || res.IsSafe {
			return output
		}
		sanitized, sErr := gs.SanitizeOutput(ctx, output, "", opts)
		if sErr != nil || sanitized == "" {
			// 脱敏失败：不丢弃工具观察（保留原文），仅留痕告警（工具观察非最终输出，
			// 不做安全兜底替换，避免误伤后续 ReAct 推理所需的中间结果）。
			log.Printf("[护栏] 工具输出脱敏失败（放行原观察）: tool=%s err=%v", toolName, sErr)
			return output
		}
		recordGuardrailEvent(ctx, GuardrailEventToolOutputRedacted, toolName, "工具观察含敏感内容，已脱敏后回灌")
		return sanitized
	}
}

// ========================================
// 组合根护栏装配（会话/agent 级配置，各开关独立，默认全关 → 零回归）
// ========================================

// GuardrailWiring 是组合根下发的护栏装配配置：持有 GuardrailService 与三路独立开关
// （输入 / 最终输出 / 逐工具输出）。Service 为 nil 或三开关皆关时视为「全关」，
// Decorator() 返回 nil（恒等装配），Builder 构建行为逐字节不变（零回归，任务 7.1/7.2）。
type GuardrailWiring struct {
	// Service 护栏能力实现；nil 时整体关闭。
	Service domainguardrail.GuardrailService
	// Input 输入护栏配置（越狱 / 输入检查子开关在其内部）。
	Input InputGuardrailConfig
	// Output 输出护栏配置（最终输出与逐工具输出共用一套脱敏选项/兜底）。
	Output OutputGuardrailConfig
	// EnableInput 启用输入护栏（BeforeHook 最先把关）。
	EnableInput bool
	// EnableOutput 启用最终输出护栏（AfterHook 最后把关；触发流式强制缓冲）。
	EnableOutput bool
	// EnableToolOutput 启用逐工具输出脱敏（post-invoke，任务 5）。
	EnableToolOutput bool
}

// GuardrailDecorator 把护栏 Hook 按会话/agent 级开关装配进任意 Builder。
// 组合根构造一次后下发给各 Agent 构建缝（含 preset 内部 Builder），保证装配口径统一。
// 零值（nil）为恒等装配：未启用任何护栏的 agent 不获得任何护栏 Hook（零回归，任务 7.2）。
type GuardrailDecorator func(*Builder) *Builder

// Apply 对 Builder 应用护栏装配；接收者为 nil 时原样返回（恒等，零回归）。
func (d GuardrailDecorator) Apply(b *Builder) *Builder {
	if d == nil || b == nil {
		return b
	}
	return d(b)
}

// Decorator 依据开关构造装配器；全关时返回 nil（恒等），使默认关闭路径零回归。
func (w GuardrailWiring) Decorator() GuardrailDecorator {
	if w.Service == nil || (!w.EnableInput && !w.EnableOutput && !w.EnableToolOutput) {
		return nil
	}
	return func(b *Builder) *Builder {
		if w.EnableInput {
			b = b.WithInputGuardrail(w.Service, w.Input)
		}
		if w.EnableOutput {
			b = b.WithOutputGuardrail(w.Service, w.Output)
		}
		if w.EnableToolOutput {
			b = b.WithToolOutputGuardrail(w.Service, w.Output)
		}
		return b
	}
}

// ========================================
// Hook 工厂
// ========================================

// newInputGuardrailHook 把输入护栏装配为 BeforeHook：先越狱检测再输入检查。
// 越狱/不安全（且未开启脱敏）→ 返回 GuardrailViolationError 中止；开启脱敏且命中 →
// 经 SanitizeInput 改写 message 放行。gs 为 nil 时返回放行空 hook（防御）。
func newInputGuardrailHook(gs domainguardrail.GuardrailService, cfg InputGuardrailConfig) BeforeHook {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		if gs == nil {
			return ctx, message, nil
		}

		// 1) 越狱检测：攻击意图不可脱敏，直接中止。
		if cfg.EnableJailbreak {
			opts := cfg.JailbreakOptions
			if opts == nil {
				opts = domainguardrail.DefaultJailbreakOptions()
			}
			if jr, err := gs.CheckJailbreak(ctx, message, opts); err == nil && jr != nil && jr.IsJailbreak {
				recordGuardrailEvent(ctx, GuardrailEventJailbreakBlocked, "",
					fmt.Sprintf("attack_type=%s severity=%d", jr.AttackType, jr.Severity))
				return ctx, message, &GuardrailViolationError{
					Stage:  "jailbreak",
					Detail: "输入被判定为越狱/攻击意图，已拒绝执行",
				}
			}
		}

		// 2) 通用输入检查：不安全时按 cfg.Sanitize 决定中止或脱敏放行。
		if cfg.EnableInputCheck {
			opts := cfg.FilterOptions
			if opts == nil {
				opts = domainguardrail.DefaultFilterOptions()
			}
			res, err := gs.CheckInput(ctx, message, opts)
			if err != nil {
				// 护栏自身异常：不阻断正常业务（fail-open），仅留痕告警。
				log.Printf("[护栏] 输入检查异常（放行）: %v", err)
				return ctx, message, nil
			}
			if res != nil && !res.IsSafe {
				if cfg.Sanitize {
					sanitized, sErr := gs.SanitizeInput(ctx, message, opts)
					if sErr == nil && sanitized != "" {
						recordGuardrailEvent(ctx, GuardrailEventInputRedacted, "", "输入含敏感片段，已脱敏后放行")
						return ctx, sanitized, nil
					}
					// 脱敏失败退化为中止（宁拒不闯）。
				}
				recordGuardrailEvent(ctx, GuardrailEventInputBlocked, "", "输入被判定为不安全，已拒绝执行")
				return ctx, message, &GuardrailViolationError{
					Stage:  "input",
					Detail: "输入内容被安全护栏判定为不安全，已拒绝执行",
				}
			}
		}

		return ctx, message, nil
	}
}

// newOutputGuardrailHook 把输出护栏装配为 AfterHook：对最终回答跑 CheckOutput（含幻觉/PII），
// 命中经 SanitizeOutput 脱敏，脱敏失败则替换安全兜底文案。护栏自身异常不阻断（fail-open）。
func newOutputGuardrailHook(gs domainguardrail.GuardrailService, cfg OutputGuardrailConfig) AfterHook {
	return func(ctx context.Context, resp *Response) error {
		if gs == nil || resp == nil || resp.Content == "" {
			return nil
		}
		opts := cfg.OutputOptions
		if opts == nil {
			opts = domainguardrail.DefaultOutputFilterOptions()
		}
		res, err := gs.CheckOutput(ctx, resp.Content, "", opts)
		if err != nil {
			log.Printf("[护栏] 输出检查异常（放行原文）: %v", err)
			return nil
		}
		if res == nil || res.IsSafe {
			return nil
		}

		// 命中：优先脱敏，脱敏失败替换安全兜底。
		sanitized, sErr := gs.SanitizeOutput(ctx, resp.Content, "", opts)
		if sErr == nil && sanitized != "" {
			resp.Content = sanitized
		} else {
			fallback := cfg.FallbackText
			if fallback == "" {
				fallback = defaultOutputFallback
			}
			resp.Content = fallback
		}
		recordGuardrailEvent(ctx, GuardrailEventOutputRedacted, "", "最终输出含敏感内容，已脱敏或替换兜底")
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]interface{})
		}
		resp.Metadata["output_guardrail"] = "redacted"
		return nil
	}
}
