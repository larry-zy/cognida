// Package tools 提供 Skill 调用工具
// 让 Agent 能够动态加载和注入 Skill 内容
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"link/internal/service/agent/skills"
)

// ========================================
// skill_invoke Tool
// ========================================

// skillInvokeTool skill_invoke 工具实现
type skillInvokeTool struct {
	skillManager skills.SkillManager
	integration  skills.SkillAgentIntegration
}

// NewSkillInvokeTool 创建 skill_invoke 工具
func NewSkillInvokeTool() (tool.InvokableTool, error) {
	return &skillInvokeTool{
		skillManager: skills.GetGlobalManager(),
		integration:  skills.NewSkillAgentIntegration(),
	}, nil
}

// Info 返回工具信息
func (t *skillInvokeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_invoke",
		Desc: "调用指定的 Skill（技能指导文档），将 Skill 的指导内容注入到当前上下文中。当需要特定领域的专业知识或指导时使用。参数：skill_name（必需）- Skill 名称；task（可选）- 当前任务描述，用于验证 Skill 是否适用。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill_name": {
				Type:     schema.String,
				Desc:     "（必需）要调用的 Skill 名称，如 code-review, rag-search, data-analysis",
				Required: true,
			},
			"task": {
				Type:     schema.String,
				Desc:     "（可选）当前任务描述，用于验证 Skill 是否适用",
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

	// 获取 skill_name
	skillName, ok := arguments["skill_name"].(string)
	if !ok || skillName == "" {
		return "", fmt.Errorf("missing required parameter: skill_name")
	}

	// 获取 Skill
	skill, exists := t.skillManager.GetSkill(skillName)
	if !exists {
		// 尝试模糊匹配
		matches := t.skillManager.SearchSkills(skillName)
		if len(matches) > 0 {
			skill = matches[0].Skill
		} else {
			return "", fmt.Errorf("skill not found: %s", skillName)
		}
	}

	// 格式化 Skill 内容
	skillContext, err := t.integration.FormatForAgent(ctx, skill)
	if err != nil {
		return "", fmt.Errorf("failed to format skill: %w", err)
	}

	// 构造结果
	result := map[string]interface{}{
		"success": true,
		"skill": map[string]interface{}{
			"name":        skill.Name,
			"description": skill.Description,
			"category":    skill.Category,
			"tags":        skill.Tags,
			"is_pure_guidance": skill.IsPureGuidance(),
		},
		"content": skillContext.FormattedContent,
		"instruction": fmt.Sprintf("Skill '%s' 已加载。请参考上述指导内容完成任务。", skill.Name),
	}

	// 如果有任务描述，验证 Skill 是否适用
	if task, ok := arguments["task"].(string); ok && task != "" {
		matches := t.skillManager.MatchSkillsForTask(task, 5)
		isRelevant := false
		for _, match := range matches {
			if match.Skill.Name == skill.Name {
				isRelevant = true
				result["relevance"] = match.Relevance
				result["match_reason"] = match.Reason
				break
			}
		}
		result["is_relevant"] = isRelevant
		if !isRelevant {
			result["warning"] = fmt.Sprintf("Skill '%s' 可能不完全匹配当前任务", skill.Name)
		}
	}

	// 序列化结果
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultBytes), nil
}

// ========================================
// skill_match Tool
// ========================================

// skillMatchTool skill_match 工具实现
type skillMatchTool struct {
	skillManager skills.SkillManager
}

// NewSkillMatchTool 创建 skill_match 工具
func NewSkillMatchTool() (tool.InvokableTool, error) {
	return &skillMatchTool{
		skillManager: skills.GetGlobalManager(),
	}, nil
}

// Info 返回工具信息
func (t *skillMatchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_match",
		Desc: "根据任务描述自动匹配最相关的 Skill。当不确定应该使用哪个 Skill 时使用。参数：task（必需）- 任务描述；limit（可选）- 返回结果数量，默认 3。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task": {
				Type:     schema.String,
				Desc:     "（必需）任务描述，用于匹配相关 Skill",
				Required: true,
			},
			"limit": {
				Type:     schema.Integer,
				Desc:     "（可选）返回结果数量，默认 3",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun 执行工具调用
func (t *skillMatchTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 解析参数
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &arguments); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// 获取 task
	task, ok := arguments["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("missing required parameter: task")
	}

	// 获取 limit
	limit := 3
	if limitVal, ok := arguments["limit"].(float64); ok && limitVal > 0 {
		limit = int(limitVal)
	}

	// 匹配 Skill
	matches := t.skillManager.MatchSkillsForTask(task, limit)

	if len(matches) == 0 {
		result := map[string]interface{}{
			"success": false,
			"message": "没有找到相关的 Skill",
			"task":    task,
		}
		resultBytes, _ := json.MarshalIndent(result, "", "  ")
		return string(resultBytes), nil
	}

	// 构造结果
	var matchedSkills []map[string]interface{}
	for _, match := range matches {
		matchedSkills = append(matchedSkills, map[string]interface{}{
			"name":          match.Skill.Name,
			"description":   match.Skill.Description,
			"category":      match.Skill.Category,
			"relevance":     match.Relevance,
			"reason":        match.Reason,
			"is_pure_guidance": match.Skill.IsPureGuidance(),
		})
	}

	result := map[string]interface{}{
		"success": true,
		"task":    task,
		"count":   len(matchedSkills),
		"matches": matchedSkills,
		"recommendation": formatRecommendation(matches),
	}

	// 序列化结果
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultBytes), nil
}

// formatRecommendation 格式化推荐信息
func formatRecommendation(matches []*skills.SkillMatchResult) string {
	if len(matches) == 0 {
		return "没有找到相关的 Skill"
	}

	topMatch := matches[0]
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("推荐使用 Skill: %s\n", topMatch.Skill.Name))
	sb.WriteString(fmt.Sprintf("相关度: %.2f\n", topMatch.Relevance))
	sb.WriteString(fmt.Sprintf("原因: %s\n", topMatch.Reason))

	if topMatch.Skill.IsPureGuidance() {
		sb.WriteString("\n这是一个纯指导性 Skill，提供专业知识但不调用工具。")
	} else {
		sb.WriteString("\n这个 Skill 可能会调用以下工具: ")
		if len(topMatch.Skill.AllowedTools) > 0 {
			sb.WriteString(strings.Join(topMatch.Skill.AllowedTools, ", "))
		} else {
			sb.WriteString("（未指定）")
		}
	}

	return sb.String()
}
