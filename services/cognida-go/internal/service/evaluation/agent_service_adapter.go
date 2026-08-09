// Package evaluation 提供评测系统应用层实现
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	agentframework "cognida/internal/service/agent/framework"
	"cognida/internal/service/evaluation/executor"
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
		// 运行时基础指标：token 用量与 LLM 调用次数来自框架回填的 Response.Metadata
		TokensUsed: metadataInt(resp.Metadata, "tokens_used"),
		LLMCalls:   metadataInt(resp.Metadata, "iterations"),
		// Text2SQL 评测：取最后一次 sql_execute 调用的 SQL 作为「被测生成 SQL」。
		GeneratedSQL: extractGeneratedSQL(resp.ToolCalls),
	}, nil
}

// extractGeneratedSQL 从工具调用序列中抽取被测 Agent 生成的 SQL。
// 取最后一次 sql_execute 调用（一问可能多次试错，末次即最终答案所依据的查询）：
// 优先入参 Input["sql"]；缺失时回退解析工具输出 JSON 的 executed_sql（含自动补的 LIMIT）。
// 非 SQL 场景无 sql_execute 调用，返回空串。
func extractGeneratedSQL(toolCalls []*agentframework.ToolCall) string {
	for i := len(toolCalls) - 1; i >= 0; i-- {
		tc := toolCalls[i]
		if tc == nil || tc.Name != "sql_execute" {
			continue
		}
		if tc.Input != nil {
			if s, ok := tc.Input["sql"].(string); ok && s != "" {
				return s
			}
		}
		if tc.Output != "" {
			var out struct {
				ExecutedSQL string `json:"executed_sql"`
			}
			if err := json.Unmarshal([]byte(tc.Output), &out); err == nil && out.ExecutedSQL != "" {
				return out.ExecutedSQL
			}
		}
		return "" // 命中末次 sql_execute 但取不到 SQL，不再向前找（末次即准）
	}
	return ""
}

// metadataInt 从 Response.Metadata 安全读取整型指标（缺失或类型不符返回 0）。
// 框架 fillMetadata 以 int 写入 tokens_used/iterations，此处兼容常见数值类型。
func metadataInt(meta map[string]interface{}, key string) int {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
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
