// Package tools 提供 Skill 列表工具
// 让 Agent 能够发现和浏览可用的 Skill
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"cognida/internal/service/agent/skills"
	toolregistry "cognida/internal/service/agent/tools"
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
		Desc: "列出所有可用的 Skill（技能指导文档）。Skill 是包含特定领域知识和指导的 Markdown 文档，可以帮助 Agent 更好地完成特定任务。category 与 tags 条件取交集；tags 用逗号分隔，默认 any（OR），tag_match=all 时为 AND。标签区分大小写。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"category": {
				Type:     schema.String,
				Desc:     "（可选）按分类过滤 Skill，如: development, retrieval, data",
				Required: false,
			},
			"tags": {
				Type:     schema.String,
				Desc:     "（可选）按标签过滤，逗号分隔；默认任一标签匹配（OR），区分大小写，如: sql,analytics",
				Required: false,
			},
			"tag_match": {Type: schema.String, Desc: "any（默认 OR）或 all（AND）", Enum: []string{"any", "all"}},
		}),
	}, nil
}

// InvokableRun 执行工具调用
func (t *skillListTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	filter, err := toolregistry.ParseSkillListFilter(argumentsInJSON)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var filteredSkills []*skills.SkillInfo
	for _, skillInfo := range t.skillManager.ListSkillsInfo() {
		if skillInfo != nil && filter.Matches(skillInfo.Category, skillInfo.Tags) {
			filteredSkills = append(filteredSkills, skillInfo)
		}
	}
	sort.Slice(filteredSkills, func(i, j int) bool { return filteredSkills[i].Name < filteredSkills[j].Name })

	// 构造结果
	result := map[string]interface{}{
		"count":  len(filteredSkills),
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
			"name":             info.Name,
			"description":      info.Description,
			"category":         info.Category,
			"tags":             info.Tags,
			"is_pure_guidance": info.IsPureGuidance,
		}

		if info.WhenToUse != "" {
			skill["when_to_use"] = info.WhenToUse
		}

		formatted = append(formatted, skill)
	}

	return formatted
}
