// Package tools 提供 Skill 工具实现（工具发现和推荐）
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

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
		Desc: "列出已注册工具及可信分类/标签。category 与标签条件取交集；多个 tags 默认 any（OR），tag_match=all 时为 AND。标签区分大小写。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"category": {
				Type:     schema.String,
				Desc:     "（可选）按分类过滤工具，如: sql, rag, web, kb, graph",
				Required: false,
			},
			"tags": {
				Type:     schema.String,
				Desc:     "（可选）注册标签，逗号分隔；默认匹配任一标签（OR），区分大小写。",
				Required: false,
			},
			"tag_match": {Type: schema.String, Desc: "any（默认 OR）或 all（AND）", Enum: []string{"any", "all"}},
		}),
	}, nil
}

// InvokableRun 执行工具调用
func (t *skillListTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	filter, err := ParseSkillListFilter(argumentsInJSON)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var tools []schema.ToolInfo
	for _, info := range t.reg.ListToolInfo() {
		metadata, ok := t.reg.ToolMetadata(info.Name)
		if !ok || !filter.Matches(metadata.Category, metadata.Tags) {
			continue
		}
		tools = append(tools, info)
	}

	// 构造结果
	result := map[string]interface{}{
		"count": len(tools),
		"tools": make([]map[string]interface{}, 0, len(tools)),
	}

	for _, info := range tools {
		metadata, _ := t.reg.ToolMetadata(info.Name)
		toolInfo := map[string]interface{}{
			"category":    metadata.Category,
			"tags":        metadata.Tags,
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

const maxSkillFilterTags = 32

// SkillListFilter is shared by document Skill discovery and tool discovery.
// Matching is case-sensitive, as in existing Skill metadata; category is ANDed
// with tags. The default any mode preserves the production Skill OR behavior.
type SkillListFilter struct {
	Category string
	Tags     []string
	TagMatch string
}

// ParseSkillListFilter accepts a bounded JSON object with exact known keys.
// Bad types/JSON/duplicates must not silently turn a filter into an unfiltered list.
func ParseSkillListFilter(input string) (SkillListFilter, error) {
	filter := SkillListFilter{TagMatch: "any"}
	invalid := func() (SkillListFilter, error) {
		return SkillListFilter{}, fmt.Errorf("invalid skill_list filter: expected category/tags strings and tag_match any or all")
	}
	if len(input) > 16*1024 || !utf8.ValidString(input) {
		return invalid()
	}
	dec := json.NewDecoder(strings.NewReader(input))
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		return invalid()
	}
	values := make(map[string]string)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return invalid()
		}
		key, ok := token.(string)
		if !ok {
			return invalid()
		}
		if key != "category" && key != "tags" && key != "tag_match" {
			return invalid()
		}
		if _, exists := values[key]; exists {
			return invalid()
		}
		token, err = dec.Token()
		if err != nil {
			return invalid()
		}
		value, ok := token.(string)
		if !ok {
			return invalid()
		}
		values[key] = value
	}
	if end, err := dec.Token(); err != nil || end != json.Delim('}') {
		return invalid()
	}
	if _, err := dec.Token(); err != io.EOF {
		return invalid()
	}
	if mode, ok := values["tag_match"]; ok {
		if mode != "any" && mode != "all" {
			return invalid()
		}
		filter.TagMatch = mode
	}
	filter.Category = strings.TrimSpace(values["category"])
	if filter.Category != "" && validateSkillLabel(filter.Category) != nil {
		return invalid()
	}
	raw := strings.TrimSpace(values["tags"])
	if raw == "" {
		return filter, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxSkillFilterTags {
		return invalid()
	}
	seen := make(map[string]bool)
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if validateSkillLabel(tag) != nil {
			return invalid()
		}
		if !seen[tag] {
			filter.Tags = append(filter.Tags, tag)
			seen[tag] = true
		}
	}
	return filter, nil
}

func validateSkillLabel(label string) error {
	if label == "" || len(label) > 128 || !utf8.ValidString(label) {
		return fmt.Errorf("invalid label")
	}
	for _, char := range label {
		if unicode.IsControl(char) || char == ',' || unicode.IsSpace(char) {
			return fmt.Errorf("invalid label")
		}
	}
	return nil
}

// Matches implements category intersection plus explicit any/all tag matching.
func (f SkillListFilter) Matches(category string, tags []string) bool {
	if f.Category != "" && f.Category != category {
		return false
	}
	if len(f.Tags) == 0 {
		return true
	}
	available := make(map[string]bool, len(tags))
	for _, tag := range tags {
		available[tag] = true
	}
	for _, tag := range f.Tags {
		if f.TagMatch == "any" && available[tag] {
			return true
		}
		if f.TagMatch == "all" && !available[tag] {
			return false
		}
	}
	return f.TagMatch == "all"
}
