// Package tools 提供 Skill 工具注册
package tools

import (
	"log"

	"link/internal/service/agent/tools"
)

// init 自动注册 Skill 工具到全局工具注册表
func init() {
	if err := RegisterSkillTools(); err != nil {
		log.Printf("[警告] Skill 工具注册失败: %v", err)
	}
}

// RegisterSkillTools 注册所有 Skill 工具
func RegisterSkillTools() error {
	// skill_list - 列出可用 Skill
	skillListTool, err := NewSkillListTool()
	if err != nil {
		return err
	}
	if err := tools.GlobalRegistry.Register("skill", skillListTool); err != nil {
		return err
	}

	// skill_invoke - 调用指定 Skill
	skillInvokeTool, err := NewSkillInvokeTool()
	if err != nil {
		return err
	}
	if err := tools.GlobalRegistry.Register("skill", skillInvokeTool); err != nil {
		return err
	}

	// skill_match - 匹配最相关的 Skill
	skillMatchTool, err := NewSkillMatchTool()
	if err != nil {
		return err
	}
	if err := tools.GlobalRegistry.Register("skill", skillMatchTool); err != nil {
		return err
	}

	log.Printf("[Skill Tools] 已注册 3 个 Skill 工具: skill_list, skill_invoke, skill_match")

	return nil
}
