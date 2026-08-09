// Package skills 提供 Skill 便捷函数
// 简化 Skill 系统的使用
package skills

import (
	"context"
	"fmt"
	"strings"
)

// ========================================
// 便捷函数
// ========================================

// FindSkill 根据名称查找 Skill
// 支持精确匹配和模糊匹配
func FindSkill(name string) (*Skill, error) {
	manager := GetGlobalManager()

	// 精确匹配
	if skill, exists := manager.GetSkill(name); exists {
		return skill, nil
	}

	// 模糊搜索
	matches := manager.SearchSkills(name)
	if len(matches) > 0 {
		return matches[0].Skill, nil
	}

	return nil, fmt.Errorf("skill not found: %s", name)
}

// FindSkillsByCategory 按分类查找 Skill
func FindSkillsByCategory(category string) []*Skill {
	manager := GetGlobalManager()
	return manager.GetRegistry().ListByCategory(category)
}

// FindSkillsByTags 按标签查找 Skill
func FindSkillsByTags(tags ...string) []*Skill {
	manager := GetGlobalManager()

	allSkills := manager.ListSkills()
	var result []*Skill

	for _, skill := range allSkills {
		if skill.MatchesTags(tags) {
			result = append(result, skill)
		}
	}

	return result
}

// GetSkillContent 获取 Skill 格式化后的内容
// 便捷函数，用于直接获取 Skill 内容字符串
func GetSkillContent(name string) (string, error) {
	skill, err := FindSkill(name)
	if err != nil {
		return "", err
	}

	integration := NewSkillAgentIntegration()
	ctx, err := integration.FormatForAgent(context.Background(), skill)
	if err != nil {
		return "", err
	}

	return ctx.FormattedContent, nil
}

// GetSkillNames 获取所有 Skill 名称
func GetSkillNames() []string {
	manager := GetGlobalManager()
	skills := manager.ListSkills()

	names := make([]string, len(skills))
	for i, skill := range skills {
		names[i] = skill.Name
	}

	return names
}

// GetCategories 获取所有分类
func GetCategories() []string {
	manager := GetGlobalManager()
	return manager.GetRegistry().Categories()
}

// ========================================
// Skill 构建器
// ========================================

// SkillBuilder Skill 构建器
type SkillBuilder struct {
	skill *Skill
}

// NewSkillBuilder 创建 Skill 构建器
func NewSkillBuilder(name, description string) *SkillBuilder {
	return &SkillBuilder{
		skill: &Skill{
			Name:        name,
			Description: description,
			Tags:        []string{},
		},
	}
}

// WithWhenToUse 设置使用时机
func (b *SkillBuilder) WithWhenToUse(whenToUse string) *SkillBuilder {
	b.skill.WhenToUse = whenToUse
	return b
}

// WithCategory 设置分类
func (b *SkillBuilder) WithCategory(category string) *SkillBuilder {
	b.skill.Category = category
	return b
}

// WithTags 设置标签
func (b *SkillBuilder) WithTags(tags ...string) *SkillBuilder {
	b.skill.Tags = tags
	return b
}

// WithContent 设置内容
func (b *SkillBuilder) WithContent(content string) *SkillBuilder {
	b.skill.Content = content
	return b
}

// WithAllowedTools 设置允许的工具
func (b *SkillBuilder) WithAllowedTools(tools ...string) *SkillBuilder {
	b.skill.AllowedTools = tools
	return b
}

// WithDisallowedTools 设置禁止的工具
func (b *SkillBuilder) WithDisallowedTools(tools ...string) *SkillBuilder {
	b.skill.DisallowedTools = tools
	return b
}

// WithDisableModelInvocation 设置禁用模型调用（纯指导性）
func (b *SkillBuilder) WithDisableModelInvocation(disable bool) *SkillBuilder {
	b.skill.DisableModelInvocation = disable
	return b
}

// WithVersion 设置版本
func (b *SkillBuilder) WithVersion(version string) *SkillBuilder {
	b.skill.Version = version
	return b
}

// WithAuthor 设置作者
func (b *SkillBuilder) WithAuthor(author string) *SkillBuilder {
	b.skill.Author = author
	return b
}

// WithExperimental 设置实验性
func (b *SkillBuilder) WithExperimental(experimental bool) *SkillBuilder {
	b.skill.Experimental = experimental
	return b
}

// Build 构建 Skill
func (b *SkillBuilder) Build() *Skill {
	return b.skill
}

// Register 注册到全局管理器
func (b *SkillBuilder) Register() error {
	manager := GetGlobalManager()
	return manager.GetRegistry().Register(b.skill)
}

// ========================================
// Skill 快速创建函数
// ========================================

// CreatePureGuidanceSkill 创建纯指导性 Skill
func CreatePureGuidanceSkill(name, description, content string) *Skill {
	return &Skill{
		Name:                   name,
		Description:            description,
		Content:                content,
		DisableModelInvocation: true,
	}
}

// CreateToolBasedSkill 创建基于工具的 Skill
func CreateToolBasedSkill(name, description, content string, allowedTools []string) *Skill {
	return &Skill{
		Name:         name,
		Description:  description,
		Content:      content,
		AllowedTools: allowedTools,
	}
}

// ========================================
// Skill 内容生成
// ========================================

// GenerateSkillPrompt 生成 Skill 提示
// 用于将 Skill 内容转换为 Agent 可用的提示格式
func GenerateSkillPrompt(skillNames []string) (string, error) {
	integration := NewSkillAgentIntegration()

	var skillContexts []*SkillContext

	for _, name := range skillNames {
		skill, err := FindSkill(name)
		if err != nil {
			return "", fmt.Errorf("skill not found: %s", name)
		}

		ctx, err := integration.FormatForAgent(context.Background(), skill)
		if err != nil {
			return "", fmt.Errorf("failed to format skill %s: %w", name, err)
		}

		skillContexts = append(skillContexts, ctx)
	}

	var sb strings.Builder
	sb.WriteString("--- 可用指导 ---\n\n")

	for i, ctx := range skillContexts {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		sb.WriteString(ctx.FormattedContent)
	}

	sb.WriteString("\n--- 指导结束 ---\n")

	return sb.String(), nil
}

// ========================================
// Skill 验证
// ========================================

// ValidateSkill 验证 Skill 定义
func ValidateSkill(skill *Skill) error {
	if skill == nil {
		return fmt.Errorf("skill is nil")
	}

	if skill.Name == "" {
		return fmt.Errorf("skill name is required")
	}

	if !IsValidSkillName(skill.Name) {
		return fmt.Errorf("invalid skill name: %s", skill.Name)
	}

	if skill.Description == "" {
		return fmt.Errorf("skill description is required")
	}

	if skill.Content == "" {
		return fmt.Errorf("skill content is required")
	}

	return nil
}

// ValidateSkillFile 验证 SKILL.md 文件
// 返回 Skill 和验证错误
func ValidateSkillFile(path string) (*Skill, []error, error) {
	loader := NewSkillLoader()

	skill, err := loader.LoadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load skill file: %w", err)
	}

	var validationErrors []error

	if err := ValidateSkill(skill); err != nil {
		validationErrors = append(validationErrors, err)
	}

	return skill, validationErrors, nil
}
