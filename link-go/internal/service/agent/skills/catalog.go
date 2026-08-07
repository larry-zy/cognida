package skills

import (
	"fmt"
	"sort"
	"strings"
)

// 渐进式披露（progressive disclosure）Level 1：把全局注册表中所有 Skill 的
// name + description(+ when_to_use) 汇成一段轻量目录，注入 agent 的 system prompt。
// LLM 据此自主判断是否需要某技能，需要时再用 skill_invoke 工具拉取完整指导（Level 2）。
// 这是 Anthropic Agent Skills 的成熟做法：元数据常驻、正文按需加载，选择交给模型推理，
// 而非词法匹配——词法匹配仅作 AutoInjectHook 的确定性兜底。

const (
	// maxCatalogSkills 目录最多展开多少个技能。目录随每次请求进 system prompt，而
	// SkillSink 会持续把会话经验沉淀成新技能（实验性），注册表只增不减；无上限会让
	// 每次请求的 prompt token 随技能数无界增长。超出者不展开，仅计数提示。
	maxCatalogSkills = 40
	// maxCatalogDescRunes 单技能描述在目录中的 rune 上限（超长描述截断，保住主体）。
	maxCatalogDescRunes = 160
	// maxCatalogWhenRunes 单技能「适用场景」在目录中的 rune 上限。
	maxCatalogWhenRunes = 120
)

// CatalogSection 生成"可用技能"目录文本块；全局无技能时返回空串，调用方据此决定是否拼接。
// 施加 token 预算：策展（非实验）技能优先展开，实验性（多为经验自动沉淀）靠后，
// 总数裁到 maxCatalogSkills，单条描述/场景按 rune 截断，避免目录随技能沉淀无界膨胀。
func CatalogSection() string {
	return renderCatalog(ListAllSkills())
}

// renderCatalog 是目录生成的纯函数核心（不触全局注册表，便于单测预算逻辑）。
func renderCatalog(all []*Skill) string {
	if len(all) == 0 {
		return ""
	}

	// 稳定优先级：策展技能在前、实验性在后，同组按名称（List 已按名排序，稳定排序即保序）。
	// 这样自动沉淀的实验性技能持续增多时，先被预算裁掉的是它们而非策展技能。
	ordered := make([]*Skill, len(all))
	copy(ordered, all)
	sort.SliceStable(ordered, func(i, j int) bool {
		return !ordered[i].Experimental && ordered[j].Experimental
	})

	shown := ordered
	if len(shown) > maxCatalogSkills {
		shown = shown[:maxCatalogSkills]
	}

	var b strings.Builder
	b.WriteString("\n\n## 可用技能（Skills）\n")
	b.WriteString("下列技能封装了特定任务的专业方法论。当用户任务落入某技能的适用场景时，" +
		"先调用 skill_invoke 工具加载其完整指导，再据此执行，不要凭空作答；" +
		"任务与任何技能都不相关时正常作答即可。\n\n")
	for _, s := range shown {
		b.WriteString(fmt.Sprintf("- **%s**：%s", s.Name, truncateCatalogText(s.Description, maxCatalogDescRunes)))
		if wt := strings.TrimSpace(s.WhenToUse); wt != "" {
			b.WriteString(fmt.Sprintf("（适用场景：%s）", truncateCatalogText(wt, maxCatalogWhenRunes)))
		}
		b.WriteString("\n")
	}
	if omitted := len(ordered) - len(shown); omitted > 0 {
		b.WriteString(fmt.Sprintf("\n（另有 %d 个技能未在目录展开，多为自动沉淀的实验性技能；如已知名称可直接用 skill_invoke 加载）\n", omitted))
	}
	b.WriteString("\n加载方式：调用工具 skill_invoke，参数 skill_name 取上面的技能名。\n")
	return b.String()
}

// truncateCatalogText 折叠为单行并按 rune 截断（保护 CJK），超限追加省略号。
func truncateCatalogText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	return string(rs[:maxRunes]) + "…"
}

// AugmentPromptWithCatalog 把技能目录附加到基础 system prompt 之后；无技能时原样返回。
func AugmentPromptWithCatalog(basePrompt string) string {
	return basePrompt + CatalogSection()
}
