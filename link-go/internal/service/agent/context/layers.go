package context

// 分层动态提示词的规范四层与优先级。数值越大越重要，超预算时越晚被裁剪。
// 系统层与安全层 Pinned，MUST 永不被裁剪——这是「超预算裁剪必须保留安全策略层」的落地。
const (
	// LayerSystem 系统约束层（角色、总目标、硬规则）。
	LayerSystem = "system"
	// LayerSafety 安全策略层（会话 scope、危险操作红线、工具门禁）。
	LayerSafety = "safety"
	// LayerCapability 能力与工具层（当前可用工具契约）。
	LayerCapability = "capability"
	// LayerSkillPlaybook 命中的 skill playbook 层。
	LayerSkillPlaybook = "skill_playbook"
	// LayerMemory 会话记忆层（历史摘要、样本快照等低优先内容）。
	LayerMemory = "memory"

	priSystem     = 100
	prioritySafe  = 95
	priCapability = 70
	priSkill      = 50
	priMemory     = 20
)

// SystemLayer 构造系统约束层（Pinned）。
func SystemLayer(content string) Layer {
	return Layer{Name: LayerSystem, Content: content, Priority: priSystem, Pinned: true}
}

// SafetyLayer 构造安全策略层（Pinned）。超预算裁剪时必须保留本层。
func SafetyLayer(content string) Layer {
	return Layer{Name: LayerSafety, Content: content, Priority: prioritySafe, Pinned: true}
}

// CapabilityLayer 构造能力与工具契约层。
func CapabilityLayer(content string) Layer {
	return Layer{Name: LayerCapability, Content: content, Priority: priCapability}
}

// SkillPlaybookLayer 构造命中的 skill playbook 层。
func SkillPlaybookLayer(content string) Layer {
	return Layer{Name: LayerSkillPlaybook, Content: content, Priority: priSkill}
}

// MemoryLayer 构造会话记忆层（历史摘要/样本），最先被裁剪。
func MemoryLayer(content string) Layer {
	return Layer{Name: LayerMemory, Content: content, Priority: priMemory}
}
