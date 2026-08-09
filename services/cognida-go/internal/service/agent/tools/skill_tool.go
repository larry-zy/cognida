// Package tools 提供 Skill 工具实现（工具发现和推荐）
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ========================================
// skill_invoke 工具 - 智能工具调用
// ========================================

// skillInvokeTool skill_invoke 工具实现
type skillInvokeTool struct {
	reg *ToolRegistry
}

// NewSkillInvokeTool 创建 skill_invoke 工具；reg 工具注册表经参数注入，用于按名查找并调用工具。
func NewSkillInvokeTool(reg *ToolRegistry) (tool.InvokableTool, error) {
	return &skillInvokeTool{reg: reg}, nil
}

// Info 返回工具信息
func (t *skillInvokeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_invoke",
		Desc: "智能工具调用器 - 根据任务描述自动选择并执行最合适的工具。支持直接指定工具名，或让系统自动选择。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"tool_name": {
				Type:     schema.String,
				Desc:     "（可选）指定要调用的工具名称，如 sql_execute、rag_query、web_search 等。如果不指定，系统将根据任务自动选择。",
				Required: false,
			},
			"task": {
				Type:     schema.String,
				Desc:     "（必需）任务描述，说明需要完成什么任务，用于工具选择",
				Required: true,
			},
			"parameters": {
				Type:     schema.String,
				Desc:     "（可选）传递给工具的参数（JSON 字符串）",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun 执行工具调用
func (t *skillInvokeTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 解析参数
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &arguments); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// 获取任务描述
	task, ok := arguments["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("missing required parameter: task")
	}

	// 获取指定的工具名（可选）
	var toolName string
	if name, ok := arguments["tool_name"].(string); ok && name != "" {
		toolName = name
	} else {
		// 根据任务自动选择工具
		toolName = t.selectToolForTask(task)
	}

	// 获取参数（可选）
	params := make(map[string]interface{})
	if paramsStr, ok := arguments["parameters"].(string); ok && paramsStr != "" {
		if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
			return "", fmt.Errorf("failed to parse parameters: %w", err)
		}
	}

	// 从注册表获取工具
	toolImpl, ok := t.reg.Get(toolName)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}

	// 将参数序列化为 JSON（InvokableRun 期望的格式）
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal parameters: %w", err)
	}

	// 执行工具 - 直接类型断言
	invokable, ok := toolImpl.(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error)
	})
	if !ok {
		return "", fmt.Errorf("tool %s is not invokable", toolName)
	}

	result, err := invokable.InvokableRun(ctx, string(paramsJSON), opts...)
	if err != nil {
		return "", fmt.Errorf("tool execution failed: %w", err)
	}

	return result, nil
}

// selectToolForTask 根据任务描述选择最合适的工具
func (t *skillInvokeTool) selectToolForTask(task string) string {
	taskLower := strings.ToLower(task)

	// 工具选择规则（基于关键词匹配）
	switch {
	case // SQL 相关
		strings.Contains(taskLower, "sql") ||
		strings.Contains(taskLower, "query") ||
		strings.Contains(taskLower, "database") ||
		strings.Contains(taskLower, "表") ||
		strings.Contains(taskLower, "数据查询"):
		return "sql_execute"

	case // RAG 相关
		strings.Contains(taskLower, "rag") ||
		strings.Contains(taskLower, "检索") ||
		strings.Contains(taskLower, "知识库") ||
		strings.Contains(taskLower, "搜索") && strings.Contains(taskLower, "向量"):
		return "rag_query"

	case // Web 搜索相关
		strings.Contains(taskLower, "web") ||
		strings.Contains(taskLower, "网络") ||
		strings.Contains(taskLower, "互联网") ||
		strings.Contains(taskLower, "百度") ||
		strings.Contains(taskLower, "谷歌"):
		return "web_search"

	case // 知识库相关
		strings.Contains(taskLower, "kb") ||
		strings.Contains(taskLower, "知识") && strings.Contains(taskLower, "库"):
		// kb_select 已随工具重设计移除；知识库检索统一走 rag_query（范围由会话入口选定并强制）。
		return "rag_query"

	case // 图谱相关
		strings.Contains(taskLower, "graph") ||
		strings.Contains(taskLower, "图谱") ||
		strings.Contains(taskLower, "关系"):
		return "graph_query"

	default:
		// 默认返回 web_search
		return "web_search"
	}
}

// ========================================
// skill_list 工具 - 列出可用工具
// ========================================

// skillListTool skill_list 工具实现
type skillListTool struct {
	reg *ToolRegistry
}

// NewSkillListTool 创建 skill_list 工具；reg 工具注册表经参数注入，用于列举可用工具。
func NewSkillListTool(reg *ToolRegistry) (tool.InvokableTool, error) {
	return &skillListTool{reg: reg}, nil
}

// Info 返回工具信息
func (t *skillListTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_list",
		Desc: "列出所有可用的工具及其描述。可选参数：category（按分类过滤）、tags（按标签过滤）。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"category": {
				Type:     schema.String,
				Desc:     "（可选）按分类过滤工具，如: sql, rag, web, kb, graph",
				Required: false,
			},
			"tags": {
				Type:     schema.String,
				Desc:     "（可选）按标签过滤（未实现）",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun 执行工具调用
func (t *skillListTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 解析参数
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &arguments); err != nil {
		// 参数为空或解析失败，使用默认行为
		arguments = make(map[string]interface{})
	}

	var tools []schema.ToolInfo

	// 获取分类过滤参数
	category, hasCategory := arguments["category"].(string)

	if hasCategory && category != "" {
		// 按分类获取工具
		for _, toolName := range t.reg.ListToolsByGroup(category) {
			if info, ok := t.reg.GetToolInfo(toolName); ok {
				tools = append(tools, info)
			}
		}
	} else {
		// 获取所有工具
		tools = t.reg.ListToolInfo()
	}

	// 构造结果
	result := map[string]interface{}{
		"count": len(tools),
		"tools": make([]map[string]interface{}, 0, len(tools)),
	}

	for _, info := range tools {
		toolInfo := map[string]interface{}{
			"name":        info.Name,
			"description": info.Desc,
		}
		if info.ParamsOneOf != nil {
			toolInfo["parameters"] = info.ParamsOneOf
		}
		result["tools"] = append(result["tools"].([]map[string]interface{}), toolInfo)
	}

	// 序列化结果
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultBytes), nil
}
