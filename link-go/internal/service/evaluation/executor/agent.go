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
	// Chat 调用 Agent 进行对话
	Chat(ctx context.Context, agentID, message string) (string, error)
	// GetAgent 获取 Agent 配置
	GetAgent(ctx context.Context, agentID string) (*AgentInfo, error)
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
		}

		// 为每个 QA 设置单独的超时
		qaCtx, cancel := context.WithTimeout(ctx, e.timeout)
		answer, err := e.agentService.Chat(qaCtx, task.AgentID, qa.Question)
		cancel()

		if err != nil {
			results[i].Success = false
			results[i].Error = fmt.Sprintf("agent chat failed: %v", err)
			continue
		}

		results[i].GeneratedAnswer = answer
		results[i].Success = true
	}

	return results, nil
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
		}

		// 为每个 QA 设置单独的超时
		qaCtx, cancel := context.WithTimeout(ctx, e.timeout)
		answer, err := e.agentService.Chat(qaCtx, task.AgentID, qa.Question)
		cancel()

		if err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("agent chat failed: %v", err)
		} else {
			result.GeneratedAnswer = answer
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
