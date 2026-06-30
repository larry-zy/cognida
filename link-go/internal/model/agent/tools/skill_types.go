// Package tools 提供 Agent 工具相关的领域类型定义。
//
// Skill 相关 DTO 由 service/agent/tools 下沉至此(model 层)，使得
// infrastructure 层(如 mcp 客户端)可直接依赖 model 而非反向依赖 service，
// 符合 handler → service → model ← repository/infrastructure 的依赖方向。
// 注意：此处仅为纯类型定义，Skill 的能力层(执行逻辑/工具实现)仍保留在
// service/agent/tools，不在迁移范围内。
package tools

// SkillInvokeRequest Skill 调用请求
type SkillInvokeRequest struct {
	SkillName  string                 `json:"skill_name"`
	Parameters map[string]interface{} `json:"parameters"`
	Timeout    int                    `json:"timeout,omitempty"`
}

// SkillInvokeResult Skill 调用结果
type SkillInvokeResult struct {
	Success  bool                   `json:"success"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Error    string                 `json:"error,omitempty"`
	Duration int64                  `json:"duration_ms"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SkillSource Skill 来源类型
type SkillSource string

const (
	SkillSourceBundled SkillSource = "bundled" // 系统内置
	SkillSourcePrivate SkillSource = "private" // 用户私有
	SkillSourcePublic  SkillSource = "public"  // 公开市场
	SkillSourcePlugin  SkillSource = "plugin"  // 插件
	SkillSourceUnknown SkillSource = "unknown" // 未知来源
)

// ContextMode 执行模式
type ContextMode string

const (
	ContextModeInline ContextMode = "inline" // 同步执行
	ContextModeFork   ContextMode = "fork"   // 异步子 Agent
)

// SkillInfo Skill 完整信息
type SkillInfo struct {
	// 核心标识
	Name        string `json:"name"`              // 全局唯一 ID
	Description string `json:"description"`       // 搜索排序依据
	Content     string `json:"content,omitempty"` // SKILL.md 正文
	Version     string `json:"version,omitempty"` // 版本号

	// 调用控制
	WhenToUse       string   `json:"when_to_use,omitempty"`       // 何时调用此 Skill
	AllowedTools    []string `json:"allowed_tools,omitempty"`     // 工具白名单
	DisallowedTools []string `json:"disallowed_tools,omitempty"` // 工具黑名单

	// 执行配置
	Model       string      `json:"model,omitempty"`        // 模型覆盖 ("inherit" = 继承)
	ContextMode ContextMode `json:"context_mode,omitempty"` // inline/fork
	Agent       string      `json:"agent,omitempty"`        // fork 模式的 SubAgent 类型

	// 模板参数
	ArgNames     []string `json:"arg_names,omitempty"`     // 模板占位符名称
	ArgumentHint string   `json:"argument_hint,omitempty"` // UI 使用提示

	// 元数据
	Source       SkillSource            `json:"source"`                  // Skill 来源
	Category     string                 `json:"category,omitempty"`      // 分类
	Tags         []string               `json:"tags,omitempty"`          // 标签
	Author       string                 `json:"author,omitempty"`        // 作者
	Deprecated   bool                   `json:"deprecated"`              // 是否废弃
	Experimental bool                   `json:"experimental"`            // 是否实验性
	Timeout      int                    `json:"timeout,omitempty"`       // 默认超时（秒）
	InputSchema  map[string]interface{} `json:"input_schema,omitempty"`  // 输入 Schema
	OutputSchema map[string]interface{} `json:"output_schema,omitempty"` // 输出 Schema

	// 扩展字段
	RawFrontmatter map[string]interface{} `json:"-"` // 保留自定义字段（不序列化）
}

// SkillListResult Skill 列表响应
type SkillListResult struct {
	Skills  []SkillInfo `json:"skills"`
	Count   int         `json:"count"`
	Success bool        `json:"success"`
}

// SkillListRequest Skill 列表查询请求
type SkillListRequest struct {
	Category string   `json:"category,omitempty"` // 按分类过滤
	Tags     []string `json:"tags,omitempty"`     // 按标签过滤
	Source   string   `json:"source,omitempty"`   // 按来源过滤
}
