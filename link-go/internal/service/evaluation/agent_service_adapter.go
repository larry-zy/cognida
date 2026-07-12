// Package evaluation 提供评测系统应用层实现
package evaluation

import (
	"context"
	"fmt"

	agentframework "link/internal/service/agent/framework"
	"link/internal/service/evaluation/executor"
)

// agentServiceAdapter 将 Agent 注册中心适配为评测执行器所需的 executor.AgentService。
// 复用与 RegistryAgentHandler 同源的注册实例（default / rag_assistant / chat_assistant /
// text2sql / data_agent 等），使"评测运行中的 Agent"与前端 ListAgents 展示的列表一致。
type agentServiceAdapter struct {
	registry agentframework.AgentInstanceRegistry
}

// NewAgentServiceAdapter 基于 Agent 实例注册表构建评测用的 AgentService。
func NewAgentServiceAdapter(registry agentframework.AgentInstanceRegistry) executor.AgentService {
	return &agentServiceAdapter{registry: registry}
}

// Chat 取运行中的 Agent 实例执行单轮对话，返回答案与工具调用轨迹。
// 从 framework.Response.ToolCalls 按调用顺序抽取工具名，供轨迹类指标打分。
func (a *agentServiceAdapter) Chat(ctx context.Context, agentID, message string) (*executor.AgentChatResult, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("agent registry not configured")
	}
	inst, ok := a.registry.GetInstance(agentID)
	if !ok {
		return nil, fmt.Errorf("agent instance not found: %s", agentID)
	}
	resp, err := inst.Chat(ctx, message)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &executor.AgentChatResult{}, nil
	}
	tools := make([]string, 0, len(resp.ToolCalls))
	for _, tc := range resp.ToolCalls {
		if tc == nil {
			continue
		}
		tools = append(tools, tc.Name)
	}
	return &executor.AgentChatResult{
		Answer:     resp.Content,
		ToolsUsed:  tools,
		Trajectory: tools, // 首版轨迹即工具调用序列；后续可纳入思考步骤
		TotalSteps: len(tools),
	}, nil
}

// GetAgent 从注册中心读取 Agent 元信息，用于评测前校验 Agent 是否存在。
func (a *agentServiceAdapter) GetAgent(ctx context.Context, agentID string) (*executor.AgentInfo, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("agent registry not configured")
	}
	def, err := a.registry.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return &executor.AgentInfo{
		ID:          def.ID,
		Name:        def.Name,
		Type:        string(def.Type),
		Description: def.Description,
	}, nil
}
