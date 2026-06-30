// Package tools 提供 Skill 列表工具
// 让 Agent 能够发现和浏览可用的 Skill
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"link/internal/service/agent/skills"
)

// ========================================
// skill_list Tool
// ========================================

// skillListTool skill_list 工具实现
type skillListTool struct {
	skillManager skills.SkillManager
}

// NewSkillListTool 创建 skill_list 工具
func NewSkillListTool() (tool.InvokableTool, error) {
	return &skillListTool{
		skillManager: skills.GetGlobalManager(),
	}, nil
}

// Info 返回工具信息
func (t *skillListTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "skill_list",
		Desc: "列出所有可用的 Skill（技能指导文档）。Skill 是包含特定领域知识和指导的 Markdown 文档，可以帮助 Agent 更好地完成特定任务。可选参数：category（按分类过滤）、tags（按标签过滤，逗号分隔）。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"category": {
				Type:     schema.String,
				Desc:     "（可选）按分类过滤 Skill，如: development, retrieval, data",
				Required: false,
			},
			"tags": {
				Type:     schema.String,
				Desc:     "（可选）按标签过滤，多个标签用逗号分隔，如: sql,analytics",
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

	// 获取所有 Skill 信息
	allSkills := t.skillManager.ListSkillsInfo()

	// 过滤 Skill
	var filteredSkills []*skills.SkillInfo
	category, hasCategory := arguments["category"].(string)
	tagsStr, hasTags := arguments["tags"].(string)

	var tags []string
	if hasTags && tagsStr != "" {
		// 解析标签（逗号分隔）
		tags = parseTags(tagsStr)
	}

	for _, skillInfo := range allSkills {
		// 分类过滤
		if hasCategory && category != "" && skillInfo.Category != category {
			continue
		}

		// 标签过滤
		if hasTags && len(tags) > 0 {
			if !matchesTags(skillInfo.Tags, tags) {
				continue
			}
		}

		filteredSkills = append(filteredSkills, skillInfo)
	}

	// 构造结果
	result := map[string]interface{}{
		"count": len(filteredSkills),
		"skills": formatSkillList(filteredSkills),
	}

	// 序列化结果
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultBytes), nil
}

// formatSkillList 格式化 Skill 列表
func formatSkillList(skillInfos []*skills.SkillInfo) []map[string]interface{} {
	formatted := make([]map[string]interface{}, 0, len(skillInfos))

	for _, info := range skillInfos {
		skill := map[string]interface{}{
			"name":          info.Name,
			"description":   info.Description,
			"category":      info.Category,
			"tags":          info.Tags,
			"is_pure_guidance": info.IsPureGuidance,
		}

		if info.WhenToUse != "" {
			skill["when_to_use"] = info.WhenToUse
		}

		formatted = append(formatted, skill)
	}

	return formatted
}

// parseTags 解析标签字符串
func parseTags(tagsStr string) []string {
	var tags []string
	current := ""

	for _, ch := range tagsStr {
		if ch == ',' {
			if current != "" {
				tags = append(tags, current)
				current = ""
			}
		} else if ch != ' ' && ch != '\t' {
			current += string(ch)
		}
	}

	if current != "" {
		tags = append(tags, current)
	}

	return tags
}

// matchesTags 检查 Skill 标签是否匹配
func matchesTags(skillTags []string, filterTags []string) bool {
	if len(filterTags) == 0 {
		return true
	}

	skillTagMap := make(map[string]bool)
	for _, tag := range skillTags {
		skillTagMap[tag] = true
	}

	for _, filterTag := range filterTags {
		if skillTagMap[filterTag] {
			return true
		}
	}

	return false
}
