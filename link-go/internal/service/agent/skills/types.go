// Package skills 提供 Skill 系统类型定义
// Skill 是 Markdown 文档（SKILL.md），不是 Go 代码
// Skill 可以是纯指导性的（无工具调用），也可以调用工具
package skills

import "time"

// ========================================
// Skill 核心类型
// ========================================

// Skill 表示一个完整的 Skill 定义
type Skill struct {
	// 核心标识（来自 frontmatter）
	Name        string `json:"name"`        // 必需：全局唯一名称
	Description string `json:"description"` // 必需：描述，用于搜索匹配

	// 调用控制
	WhenToUse        string   `json:"when_to_use,omitempty"`        // 何时使用此 Skill
	AllowedTools     []string `json:"allowed_tools,omitempty"`      // 允许的工具白名单
	DisallowedTools []string `json:"disallowed_tools,omitempty"`    // 禁止的工具黑名单

	// 执行配置
	Model       string `json:"model,omitempty"`       // 模型覆盖 ("inherit" = 继承 Agent 配置)
	ContextMode string `json:"context_mode,omitempty"` // 执行模式
	Effort      int    `json:"effort,omitempty"`      // 努力程度 (1-10)

	// 特殊标志
	DisableModelInvocation bool `json:"disable_model_invocation,omitempty"` // 禁用模型调用（纯指导性 Skill）

	// 内容
	Content string `json:"content,omitempty"` // Markdown 正文（frontmatter 之后的内容）

	// 支持文件
	SupportingFiles *SupportingFiles `json:"supporting_files,omitempty"` // scripts/, references/, examples/ 等

	// 元数据
	Source      string    `json:"source,omitempty"`      // 来源路径
	Version     string    `json:"version,omitempty"`     // 版本号
	Author      string    `json:"author,omitempty"`      // 作者
	Tags        []string  `json:"tags,omitempty"`        // 标签
	Category    string    `json:"category,omitempty"`    // 分类
	Deprecated  bool      `json:"deprecated,omitempty"`  // 是否废弃
	Experimental bool     `json:"experimental,omitempty"` // 是否实验性
	LoadedAt    time.Time `json:"loaded_at"`             // 加载时间

	// 原始 frontmatter（保留未解析字段）
	RawFrontmatter map[string]interface{} `json:"-"`
}

// IsPureGuidance 检查是否为纯指导性 Skill（无工具调用）
func (s *Skill) IsPureGuidance() bool {
	return s.DisableModelInvocation || (len(s.AllowedTools) == 0 && len(s.DisallowedTools) == 0)
}

// HasTools 检查是否使用工具
func (s *Skill) HasTools() bool {
	return !s.IsPureGuidance()
}

// MatchesTags 检查 Skill 是否匹配指定标签
func (s *Skill) MatchesTags(tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	tagMap := make(map[string]bool)
	for _, tag := range s.Tags {
		tagMap[tag] = true
	}
	for _, tag := range tags {
		if tagMap[tag] {
			return true
		}
	}
	return false
}

// MatchesCategory 检查 Skill 是否匹配指定分类
func (s *Skill) MatchesCategory(category string) bool {
	if category == "" {
		return true
	}
	return s.Category == category
}

// ========================================
// SupportingFiles Skill 支持文件
// ========================================

// SupportingFiles 表示 Skill 的支持文件目录
type SupportingFiles struct {
	Scripts    []string `json:"scripts,omitempty"`    // 可执行脚本
	References []string `json:"references,omitempty"` // 参考文档
	Examples   []string `json:"examples,omitempty"`   // 示例
	Assets     []string `json:"assets,omitempty"`     // 资源文件（图片等）
}

// ========================================
// Frontmatter Skill Frontmatter 结构
// ========================================

// Frontmatter 表示 SKILL.md 的 YAML frontmatter
// 解析后映射到 Skill 结构
type Frontmatter struct {
	Name                   string   `yaml:"name"`
	Description            string   `yaml:"description"`
	WhenToUse              string   `yaml:"when_to_use,omitempty"`
	AllowedTools           []string `yaml:"allowed_tools,omitempty"`
	DisallowedTools        []string `yaml:"disallowed_tools,omitempty"`
	Model                  string   `yaml:"model,omitempty"`
	ContextMode            string   `yaml:"context_mode,omitempty"`
	Effort                 int      `yaml:"effort,omitempty"`
	DisableModelInvocation bool     `yaml:"disable_model_invocation,omitempty"`
	Version                string   `yaml:"version,omitempty"`
	Author                 string   `yaml:"author,omitempty"`
	Tags                   []string `yaml:"tags,omitempty"`
	Category               string   `yaml:"category,omitempty"`
	Deprecated             bool     `yaml:"deprecated,omitempty"`
	Experimental           bool     `yaml:"experimental,omitempty"`
}

// ========================================
// SkillListRequest/Response
// ========================================

// SkillListRequest Skill 列表查询请求
type SkillListRequest struct {
	Category string   `json:"category,omitempty"` // 按分类过滤
	Tags     []string `json:"tags,omitempty"`     // 按标签过滤
	Source   string   `json:"source,omitempty"`   // 按来源过滤
	IncludeDeprecated bool `json:"include_deprecated,omitempty"` // 是否包含已废弃
}

// SkillListResponse Skill 列表响应
type SkillListResponse struct {
	Skills  []*SkillInfo `json:"skills"`
	Count   int          `json:"count"`
	Success bool         `json:"success"`
}

// SkillInfo Skill 简要信息（用于列表展示）
type SkillInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	WhenToUse    string   `json:"when_to_use,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Category     string   `json:"category,omitempty"`
	IsPureGuidance bool   `json:"is_pure_guidance"`
}

// ========================================
// SkillLoadResult Skill 加载结果
// ========================================

// SkillLoadResult Skill 加载结果
type SkillLoadResult struct {
	Skills   []*Skill `json:"skills"`
	Success  bool     `json:"success"`
	Errors   []string `json:"errors,omitempty"`
	Count    int      `json:"count"`
}

// ========================================
// SkillMatchResult Skill 匹配结果
// ========================================

// SkillMatchResult Skill 匹配结果（用于根据任务描述查找相关 Skill）
type SkillMatchResult struct {
	Skill      *Skill `json:"skill"`
	Relevance  float64 `json:"relevance"` // 相关度分数 0-1
	Reason     string `json:"reason"`     // 匹配原因
}
