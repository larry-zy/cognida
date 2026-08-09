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

	infraagent "cognida/internal/service/agent/framework"
	"cognida/internal/service/agent/skills"
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
				Desc:     "（必需）要调用的 Skill 名称，如 text2sql-adhoc, doc-qa",
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

	// 可执行 skill（design D7）：命中 CanInvoke 且挂了 handler，则执行 handler 并回传其
	// 紧凑输出（result_id + 结论），复杂任务在 handler 内 inline 编排子代理、内部往返不回灌。
	// 无 handler 的 skill 完全维持既有「返回 markdown 指导」行为（向后兼容）。
	// 注意：handler 执行仍在同一 ctx 下——其经 InlineDelegate/注册表工具所触发的调用照旧
	// 过工具门与 scope 校验（skill-tool-policy 不被绕过）。
	if skill.CanInvoke && skill.Handler != nil {
		// 可执行 skill：handler 自管其内部工具编排（可能合法用到超出声明 allowed_tools 的工具），
		// 故此路径不激活白名单，避免 handler 内部调用被自身声明的窄工具面误拦。
		return t.runHandler(ctx, skill, arguments)
	}

	// 纯指导/文档 skill：LLM 将在本 skill 指导下继续用主循环工具完成任务 → 此刻显式激活其
	// 工具白名单/黑名单，把 skill 的 allowed/disallowed_tools 从文档提示升级为执行前硬约束。
	// 这是「显式激活」模型的挂载点：入口预匹配只定 scope（见 data_agent/tool_gate.go），
	// 真正收窄工具面只发生在 LLM 主动 skill_invoke 时。导航元工具永远豁免（framework 侧），
	// 保证 LLM 随时可再 skill_invoke 切换到别的 skill，不会被窄白名单锁死。
	// 空 allowed_tools 不进入白名单模式（纯指导 skill 无工具约束）。
	infraagent.ActivateSkillPolicy(ctx, skill.Name, skill.AllowedTools, skill.DisallowedTools)

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

// runHandler 执行可执行 skill 的 handler，并把结果包成与纯指导路径一致的信封回传。
// handler 内 panic 经 recover 兜底为普通 skill 错误（不崩整个 ReAct 循环）。
func (t *skillInvokeTool) runHandler(ctx context.Context, skill *skills.Skill, arguments map[string]interface{}) (output string, err error) {
	defer func() {
		if r := recover(); r != nil {
			// panic 兜底：转为普通错误回灌 LLM，交由护栏/修复纪律处置。
			output = ""
			err = fmt.Errorf("skill '%s' handler 执行异常: %v", skill.Name, r)
		}
	}()

	// handler 入参：透传本次调用参数（task 等），handler 自行取用其所需字段。
	inputJSON, mErr := json.Marshal(arguments)
	if mErr != nil {
		return "", fmt.Errorf("failed to marshal skill input: %w", mErr)
	}

	handlerOutput, hErr := skill.Handler(ctx, string(inputJSON))
	if hErr != nil {
		return "", fmt.Errorf("skill '%s' 执行失败: %w", skill.Name, hErr)
	}

	result := map[string]interface{}{
		"success": true,
		"skill": map[string]interface{}{
			"name":        skill.Name,
			"description": skill.Description,
			"category":    skill.Category,
			"executed":    true,
		},
		"output":      handlerOutput,
		"instruction": fmt.Sprintf("Skill '%s' 已执行完成，以下为其紧凑结论；请据此推进，勿重复其内部步骤。", skill.Name),
	}
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
