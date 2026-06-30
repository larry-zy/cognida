// Package agent provides Agent infrastructure implementations.
package framework

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"link/internal/model/agent"
)

// ========================================
// AgentGetter 获取 Agent 实例的函数类型
// ========================================

// AgentGetter 函数类型，用于根据 agentID 获取 Agent 实例
type AgentGetter func(agentID string) (Agent, bool)

// ========================================
// RegistryAgentOrchestrator - 基于注册中心的 Agent 编排器
// ========================================

// registryAgentOrchestrator 使用 Agent 注册中心路由请求到对应的 Agent 实例
type registryAgentOrchestrator struct {
	mu           sync.RWMutex
	agents       map[string]Agent // agentID -> Agent 实例
	status       map[string]agent.AgentStatus
	agentGetter  AgentGetter      // 从外部获取 Agent 的函数
	lastError    error
}

// NewRegistryAgentOrchestrator 创建基于注册中心的编排器
// 可以传入一个可选的 agentGetter 函数来动态获取 Agent
func NewRegistryAgentOrchestrator(agentGetter ...AgentGetter) agent.AgentExecutor {
	o := &registryAgentOrchestrator{
		agents: make(map[string]Agent),
		status: make(map[string]agent.AgentStatus),
	}
	if len(agentGetter) > 0 {
		o.agentGetter = agentGetter[0]
	}
	return o
}

// RegisterAgent 注册一个 Agent 实例
func (o *registryAgentOrchestrator) RegisterAgent(agentID string, agentInstance Agent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agents[agentID] = agentInstance
	o.status[agentID] = agent.AgentStatusIdle
	log.Printf("[RegistryOrchestrator] Registered agent: id=%s, name=%s", agentID, agentInstance.Name())
}

// SetAgentGetter 设置 Agent 获取函数
func (o *registryAgentOrchestrator) SetAgentGetter(getter AgentGetter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentGetter = getter
}

// Execute 执行 Agent（实现 domain.AgentExecutor 接口）
func (o *registryAgentOrchestrator) Execute(ctx context.Context, agentID string, input string) (string, error) {
	// 获取 Agent 实例
	agentInstance, err := o.getAgent(agentID)
	if err != nil {
		return "", err
	}

	o.mu.Lock()
	o.status[agentID] = agent.AgentStatusRunning
	o.lastError = nil
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.status[agentID] = agent.AgentStatusIdle
		o.mu.Unlock()
	}()

	log.Printf("[RegistryOrchestrator] Execute: AgentID=%s, Input=%q", agentID, input)

	// 调用 Agent 的 Chat 方法
	response, err := agentInstance.Chat(ctx, input)
	if err != nil {
		o.mu.Lock()
		o.lastError = err
		o.mu.Unlock()
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	// 构建响应
	result := response.Content
	if len(response.ToolCalls) > 0 {
		// 如果有工具调用，添加工具调用信息到响应
		toolInfo := o.buildToolCallsInfo(response.ToolCalls)
		result = fmt.Sprintf("%s\n\n%s", result, toolInfo)
	}

	return result, nil
}

// ExecuteStream 流式执行 Agent（实现 domain.AgentExecutor 接口）
func (o *registryAgentOrchestrator) ExecuteStream(ctx context.Context, agentID string, input string) (<-chan string, error) {
	// 获取 Agent 实例
	agentInstance, err := o.getAgent(agentID)
	if err != nil {
		return nil, err
	}

	o.mu.Lock()
	o.status[agentID] = agent.AgentStatusRunning
	o.lastError = nil
	o.mu.Unlock()

	log.Printf("[RegistryOrchestrator] ExecuteStream: AgentID=%s, Input=%q", agentID, input)

	chunkChan := make(chan string, 16)

	go func() {
		defer close(chunkChan)
		defer func() {
			o.mu.Lock()
			o.status[agentID] = agent.AgentStatusIdle
			o.mu.Unlock()
		}()

		// 调用 Agent 的 Stream 方法
		chunkStream, err := agentInstance.Stream(ctx, input)
		if err != nil {
			log.Printf("[RegistryOrchestrator] Stream failed: %v", err)
			return
		}

		// 转发 Chunk 内容
		for chunk := range chunkStream {
			if chunk.Content != "" {
				select {
				case <-ctx.Done():
					return
				case chunkChan <- chunk.Content:
				}
			}

			// 处理工具调用事件
			if chunk.Metadata != nil {
				if event, ok := chunk.Metadata["event"].(string); ok {
					switch event {
					case "tool_call":
						// 工具调用事件
						if tc, ok := chunk.Metadata["tool_call"].(*ToolCallInStream); ok {
							toolMsg := o.formatToolCallEvent(tc)
							select {
							case <-ctx.Done():
								return
							case chunkChan <- toolMsg:
							}
						}
					case "tool_result":
						// 工具结果事件
						if tc, ok := chunk.Metadata["tool_call"].(*ToolCallInStream); ok {
							resultMsg := o.formatToolResultEvent(tc)
							select {
							case <-ctx.Done():
								return
							case chunkChan <- resultMsg:
							}
						}
					case "error":
						// 错误事件
						if errMsg, ok := chunk.Metadata["error"].(string); ok {
							errorMsg := fmt.Sprintf("\n\n❌ 错误: %s", errMsg)
							select {
							case <-ctx.Done():
								return
							case chunkChan <- errorMsg:
							}
						}
					}
				}
			}

			// 检查是否完成
			if chunk.Done {
				break
			}
		}
	}()

	return chunkChan, nil
}

// GetStatus 获取 Agent 状态（实现 domain.AgentExecutor 接口）
func (o *registryAgentOrchestrator) GetStatus(agentID string) (agent.AgentStatus, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	status, ok := o.status[agentID]
	if !ok {
		return agent.AgentStatusIdle, nil
	}

	return status, nil
}

// getAgent 获取 Agent 实例
func (o *registryAgentOrchestrator) getAgent(agentID string) (Agent, error) {
	// 如果 agentID 为空，使用默认 agent
	if agentID == "" {
		agentID = "default"
	}

	// 首先检查本地注册的 agents
	o.mu.RLock()
	agentInstance, ok := o.agents[agentID]
	o.mu.RUnlock()

	if ok {
		return agentInstance, nil
	}

	// 尝试使用 agentGetter 获取
	if o.agentGetter != nil {
		if agentInstance, found := o.agentGetter(agentID); found {
			// 缓存到本地
			o.mu.Lock()
			o.agents[agentID] = agentInstance
			o.status[agentID] = agent.AgentStatusIdle
			o.mu.Unlock()
			return agentInstance, nil
		}
	}

	return nil, fmt.Errorf("agent not found: %s", agentID)
}

// buildToolCallsInfo 构建工具调用信息
func (o *registryAgentOrchestrator) buildToolCallsInfo(toolCalls []*ToolCall) string {
	var sb strings.Builder
	sb.WriteString("\n🔧 工具调用:\n")
	for _, tc := range toolCalls {
		sb.WriteString(fmt.Sprintf("  - %s", tc.Name))
		if tc.Error != nil {
			sb.WriteString(fmt.Sprintf(" ❌ (%s)", tc.Error))
		} else {
			sb.WriteString(" ✅")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatToolCallEvent 格式化工具调用事件
func (o *registryAgentOrchestrator) formatToolCallEvent(tc *ToolCallInStream) string {
	inputStr := ""
	if tc.Input != nil {
		inputStr = fmt.Sprintf("%v", tc.Input)
	}
	return fmt.Sprintf("\n\n🔧 调用工具: %s\n📝 参数: %s", tc.Name, inputStr)
}

// formatToolResultEvent 格式化工具结果事件
func (o *registryAgentOrchestrator) formatToolResultEvent(tc *ToolCallInStream) string {
	if tc.Error != "" {
		return fmt.Sprintf("\n\n❌ 工具执行失败: %s\n错误: %s", tc.Name, tc.Error)
	}
	// 截断过长的输出
	output := tc.Output
	if len(output) > 500 {
		output = output[:500] + "... (已截断)"
	}
	return fmt.Sprintf("\n\n✅ 工具执行成功: %s\n📊 结果: %s", tc.Name, output)
}

// ========================================
// 全局单例
// ========================================

var (
	globalRegistryOrchestrator agent.AgentExecutor
	registryOrchestratorOnce   sync.Once
)

// GetGlobalRegistryOrchestrator 获取全局注册中心编排器
func GetGlobalRegistryOrchestrator() agent.AgentExecutor {
	registryOrchestratorOnce.Do(func() {
		globalRegistryOrchestrator = NewRegistryAgentOrchestrator()
	})
	return globalRegistryOrchestrator
}

// SetGlobalAgentGetter 设置全局 Agent 获取函数
func SetGlobalAgentGetter(getter AgentGetter) {
	if orchestrator, ok := globalRegistryOrchestrator.(*registryAgentOrchestrator); ok {
		orchestrator.SetAgentGetter(getter)
	}
}
