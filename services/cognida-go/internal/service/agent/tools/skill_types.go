// Package tools 提供 Skill 调用相关类型定义。
//
// Skill DTO 已下沉至 model 层(internal/model/agent/tools)，此处保留类型/常量
// 别名做向后兼容：能力层(skill_tool.go 等)继续以 tools.SkillInfo 等本地名引用，
// 无需改动；同时 infrastructure 层改为直接依赖 model 层，消除 infra→service 反转。
package tools

import modeltools "cognida/internal/model/agent/tools"

// Skill 相关类型别名（指向 model 层同一类型，赋值完全兼容）
type (
	SkillInvokeRequest = modeltools.SkillInvokeRequest
	SkillInvokeResult  = modeltools.SkillInvokeResult
	SkillSource        = modeltools.SkillSource
	ContextMode        = modeltools.ContextMode
	SkillInfo          = modeltools.SkillInfo
	SkillListResult    = modeltools.SkillListResult
	SkillListRequest   = modeltools.SkillListRequest
)

// Skill 来源 / 执行模式常量（再导出 model 层常量）
const (
	SkillSourceBundled = modeltools.SkillSourceBundled
	SkillSourcePrivate = modeltools.SkillSourcePrivate
	SkillSourcePublic  = modeltools.SkillSourcePublic
	SkillSourcePlugin  = modeltools.SkillSourcePlugin
	SkillSourceUnknown = modeltools.SkillSourceUnknown

	ContextModeInline = modeltools.ContextModeInline
	ContextModeFork   = modeltools.ContextModeFork
)
