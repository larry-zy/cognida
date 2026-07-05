// Package tools 提供 Skill 工具注册
package tools

import (
	"log"

	"link/internal/service/agent/tools"
)

// RegisterSkillTools 注册所有 Skill 工具到给定注册表，由组合根（cmd/server）在
// NewToolRegistry 之后显式调用，替代旧的 init() 导入副作用注册。
// 注意：需在注册表构造之后调用——skill_list/skill_invoke 与 tools 包内置的
// 工具发现版同名，此处按名覆盖，保持旧加载顺序下"Skill 系统版生效"的语义。
func RegisterSkillTools(reg *tools.ToolRegistry) error {
	if reg == nil {
		return nil
	}

	// skill_list - 列出可用 Skill
	skillListTool, err := NewSkillListTool()
	if err != nil {
		return err
	}
	if err := reg.Register("skill", skillListTool); err != nil {
		return err
	}

	// skill_invoke - 调用指定 Skill
	skillInvokeTool, err := NewSkillInvokeTool()
	if err != nil {
		return err
	}
	if err := reg.Register("skill", skillInvokeTool); err != nil {
		return err
	}

	// skill_match - 匹配最相关的 Skill
	skillMatchTool, err := NewSkillMatchTool()
	if err != nil {
		return err
	}
	if err := reg.Register("skill", skillMatchTool); err != nil {
		return err
	}

	log.Printf("[Skill Tools] 已注册 3 个 Skill 工具: skill_list, skill_invoke, skill_match")

	return nil
}
