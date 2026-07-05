package skills

import (
	"fmt"
	"strings"
)

// 渐进式披露（progressive disclosure）Level 1：把全局注册表中所有 Skill 的
// name + description(+ when_to_use) 汇成一段轻量目录，注入 agent 的 system prompt。
// LLM 据此自主判断是否需要某技能，需要时再用 skill_invoke 工具拉取完整指导（Level 2）。
// 这是 Anthropic Agent Skills 的成熟做法：元数据常驻、正文按需加载，选择交给模型推理，
// 而非词法匹配——词法匹配仅作 AutoInjectHook 的确定性兜底。

// CatalogSection 生成"可用技能"目录文本块；全局无技能时返回空串，调用方据此决定是否拼接。
func CatalogSection() string {
	all := ListAllSkills()
	if len(all) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## 可用技能（Skills）\n")
	b.WriteString("下列技能封装了特定任务的专业方法论。当用户任务落入某技能的适用场景时，" +
		"先调用 skill_invoke 工具加载其完整指导，再据此执行，不要凭空作答；" +
		"任务与任何技能都不相关时正常作答即可。\n\n")
	for _, s := range all {
		b.WriteString(fmt.Sprintf("- **%s**：%s", s.Name, strings.TrimSpace(s.Description)))
		if wt := strings.TrimSpace(s.WhenToUse); wt != "" {
			b.WriteString(fmt.Sprintf("（适用场景：%s）", wt))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n加载方式：调用工具 skill_invoke，参数 skill_name 取上面的技能名。\n")
	return b.String()
}

// AugmentPromptWithCatalog 把技能目录附加到基础 system prompt 之后；无技能时原样返回。
func AugmentPromptWithCatalog(basePrompt string) string {
	return basePrompt + CatalogSection()
}
