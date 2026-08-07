// Package executor 提供评测执行器实现
package executor

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	agentctx "link/internal/model/agent"
	domeval "link/internal/model/evaluation"
)

// safeChat 包一层 recover 调用被测 Agent：单条 QA 触发的 panic（被测 Agent 内部/工具/LLM 客户端）
// 会被转成普通 error 返回，由调用方标记该条 Success=false 并继续下一条，避免 panic 冒泡带崩整批评测。
func safeChat(ctx context.Context, svc AgentService, agentID, message string) (res *AgentChatResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = fmt.Errorf("agent chat panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return svc.Chat(ctx, agentID, message)
}

// runSafely 执行 fn 并把其中的 panic 转成 error，供单条 QA 内的其它易崩操作（如只读 SQL 执行）
// 做故障隔离，同样避免单条 panic 带崩整批。
func runSafely(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}

// withItemRequestID 在任务级 rid 基础上派生「每条 QA」的子 rid（形如 <base>#<序号>），
// 使单条 Agent 会话在 audit_logs/Loki 中既可独立检索、又通过前缀归属同一评测任务。
// 上游未注入 rid 时原样返回 ctx，不强行造 rid（保持"无 rid 即不追踪"语义）。
func withItemRequestID(ctx context.Context, index int) context.Context {
	base, ok := agentctx.GetRequestID(ctx)
	if !ok || base == "" {
		return ctx
	}
	return agentctx.WithRequestID(ctx, fmt.Sprintf("%s#%d", base, index+1))
}

// 类型别名，简化使用
type (
	EvaluationTaskConfig = domeval.EvaluationTaskConfig
	QAPair              = domeval.QAPair
	QAResult            = domeval.QAResult
)

// AgentExecutor Agent 评测执行器
type AgentExecutor struct {
	agentService AgentService
	timeout      time.Duration
}

// AgentService Agent 服务接口
type AgentService interface {
	// Chat 调用 Agent 进行对话，返回答案与工具调用轨迹。
	// 轨迹用于 tool_selection/trajectory/step_efficiency 指标——业界共识是 Agent
	// 评测必须打分整条轨迹而非仅最终答案（选对工具≠传对参数≠不绕路）。
	Chat(ctx context.Context, agentID, message string) (*AgentChatResult, error)
	// GetAgent 获取 Agent 配置
	GetAgent(ctx context.Context, agentID string) (*AgentInfo, error)
}

// AgentChatResult 单轮 Agent 对话的富结果：最终答案 + 工具调用轨迹。
type AgentChatResult struct {
	// Answer 最终文本答案。
	Answer string
	// ToolsUsed 实际调用的工具名（按调用顺序，含重复），供 tool_selection/tool_order。
	ToolsUsed []string
	// Trajectory 步骤描述（工具名或步骤摘要），供 trajectory_match。
	Trajectory []string
	// TotalSteps 步骤总数（默认取工具调用数），供 step_efficiency。
	TotalSteps int
	// TokensUsed 本轮消耗的总 token 数（来自 Response.Metadata["tokens_used"]），供 tokens_used 指标。
	TokensUsed int
	// LLMCalls 本轮 LLM 调用次数（ReAct 迭代数，来自 Metadata["iterations"]），供 llm_calls 指标。
	LLMCalls int
	// GeneratedSQL Agent 本轮生成的 SQL（取最后一次 sql_execute 工具调用的语句），供 Text2SQL 评测。
	// 非 SQL 场景为空，不影响既有指标。
	GeneratedSQL string
}

// AgentInfo Agent 信息
type AgentInfo struct {
	ID          string
	Name        string
	Type        string
	Description string
}

// NewAgentExecutor 创建 Agent 执行器
func NewAgentExecutor(agentService AgentService, timeout time.Duration) *AgentExecutor {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &AgentExecutor{
		agentService: agentService,
		timeout:      timeout,
	}
}

// Type 返回执行器类型
func (e *AgentExecutor) Type() domeval.EvaluationType {
	return domeval.EvaluationTypeAgent
}

// Execute 执行 Agent 评测
func (e *AgentExecutor) Execute(ctx context.Context, task *EvaluationTaskConfig, dataset []*QAPair) ([]*QAResult, error) {
	// 验证配置
	if task.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required for agent evaluation")
	}

	// 检查 Agent 是否存在
	_, err := e.agentService.GetAgent(ctx, task.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s, error: %w", task.AgentID, err)
	}

	// 创建结果切片
	results := make([]*QAResult, len(dataset))

	// 为每个 QA 创建超时上下文
	for i, qa := range dataset {
		results[i] = &QAResult{
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
			ExpectedTools:   qa.ExpectedTools,
			ExpectedSteps:   qa.ExpectedSteps,
		}

		// 为每个 QA 设置单独的超时，并测量墙钟耗时（供 latency_ms 指标，失败路径也记录）
		// 派生每条 QA 的子 rid，使被测 Agent 的本轮运行可独立追踪。
		qaCtx, cancel := context.WithTimeout(withItemRequestID(ctx, i), e.timeout)
		// 把派生出的子 rid 落到结果上，供前端深链到该轮 trace 瀑布图（失败路径也保留，便于查错）。
		if rid, ok := agentctx.GetRequestID(qaCtx); ok {
			results[i].RequestID = rid
		}
		start := time.Now()
		chatResult, err := safeChat(qaCtx, e.agentService, task.AgentID, qa.Question)
		results[i].LatencyMs = time.Since(start).Milliseconds()
		cancel()

		if err != nil {
			results[i].Success = false
			results[i].Error = fmt.Sprintf("agent chat failed: %v", err)
			continue
		}

		applyAgentChatResult(results[i], chatResult)
		results[i].Success = true
	}

	return results, nil
}

// applyAgentChatResult 把 Agent 富结果写入评测结果（答案 + 轨迹），供 Python 侧计算
// tool_selection/trajectory/step_efficiency。chatResult 为 nil 时安全跳过。
func applyAgentChatResult(result *QAResult, chatResult *AgentChatResult) {
	if chatResult == nil {
		return
	}
	result.GeneratedAnswer = chatResult.Answer
	result.ToolsUsed = chatResult.ToolsUsed
	result.Trajectory = chatResult.Trajectory
	result.TotalSteps = chatResult.TotalSteps
	result.TokensUsed = chatResult.TokensUsed
	result.LLMCalls = chatResult.LLMCalls
}

// ExecuteSequential 顺序执行 Agent 评测（用于调试）
func (e *AgentExecutor) ExecuteSequential(ctx context.Context, task *EvaluationTaskConfig, dataset []*QAPair) ([]*QAResult, error) {
	// 验证配置
	if task.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required for agent evaluation")
	}

	// 检查 Agent 是否存在
	_, err := e.agentService.GetAgent(ctx, task.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s, error: %w", task.AgentID, err)
	}

	// 创建结果切片
	results := make([]*QAResult, 0, len(dataset))

	// 顺序执行
	for i, qa := range dataset {
		result := &QAResult{
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
			ExpectedTools:   qa.ExpectedTools,
			ExpectedSteps:   qa.ExpectedSteps,
		}

		// 为每个 QA 设置单独的超时，并测量墙钟耗时（供 latency_ms 指标）
		qaCtx, cancel := context.WithTimeout(withItemRequestID(ctx, i), e.timeout)
		if rid, ok := agentctx.GetRequestID(qaCtx); ok {
			result.RequestID = rid
		}
		start := time.Now()
		chatResult, err := safeChat(qaCtx, e.agentService, task.AgentID, qa.Question)
		result.LatencyMs = time.Since(start).Milliseconds()
		cancel()

		if err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("agent chat failed: %v", err)
		} else {
			applyAgentChatResult(result, chatResult)
			result.Success = true
		}

		results = append(results, result)
	}

	return results, nil
}

// GetTimeout 获取超时时间
func (e *AgentExecutor) GetTimeout() time.Duration {
	return e.timeout
}

// SetTimeout 设置超时时间
func (e *AgentExecutor) SetTimeout(timeout time.Duration) {
	e.timeout = timeout
}

// GetAgentName 获取 Agent 名称（用于日志）
func (e *AgentExecutor) GetAgentName(ctx context.Context, agentID string) string {
	info, err := e.agentService.GetAgent(ctx, agentID)
	if err != nil {
		return agentID
	}
	return info.Name
}
