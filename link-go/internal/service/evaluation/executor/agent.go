// Package executor 提供评测执行器实现
package executor

import (
	"context"
	"fmt"
	"time"

	domeval "link/internal/model/evaluation"
)

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
		qaCtx, cancel := context.WithTimeout(ctx, e.timeout)
		start := time.Now()
		chatResult, err := e.agentService.Chat(qaCtx, task.AgentID, qa.Question)
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
	for _, qa := range dataset {
		result := &QAResult{
			Question:        qa.Question,
			ReferenceAnswer: qa.ReferenceAnswer,
			RelevantPIDs:    qa.RelevantPIDs,
			ExpectedTools:   qa.ExpectedTools,
			ExpectedSteps:   qa.ExpectedSteps,
		}

		// 为每个 QA 设置单独的超时，并测量墙钟耗时（供 latency_ms 指标）
		qaCtx, cancel := context.WithTimeout(ctx, e.timeout)
		start := time.Now()
		chatResult, err := e.agentService.Chat(qaCtx, task.AgentID, qa.Question)
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
