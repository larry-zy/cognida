// Package skills 提供 Skill 与 Agent 的集成
// 负责将 Skill 内容注入到 Agent 上下文中
package skills

import (
	"context"
	"fmt"
	"strings"
)

// ========================================
// SkillContext Skill 上下文
// ========================================

// SkillContext Skill 上下文，用于将 Skill 内容传递给 Agent
type SkillContext struct {
	// Skill 信息
	Skill *Skill

	// 格式化后的内容（用于注入到 Agent）
	FormattedContent string

	// 匹配原因（如何找到此 Skill）
	MatchReason string
}

// ========================================
// SkillAgentIntegration Skill 与 Agent 集成接口
// ========================================

// SkillAgentIntegration Skill 与 Agent 集成接口
type SkillAgentIntegration interface {
	// FormatForAgent 将 Skill 格式化为 Agent 可用的内容
	FormatForAgent(ctx context.Context, skill *Skill) (*SkillContext, error)

	// FormatMultipleForAgent 批量格式化多个 Skill
	FormatMultipleForAgent(ctx context.Context, skills []*Skill) ([]*SkillContext, error)

	// InjectIntoSystemMessage 将 Skill 内容注入到系统消息
	InjectIntoSystemMessage(systemMsg string, skillContext *SkillContext) string

	// BuildSystemPrompt 构建包含 Skill 的系统提示
	BuildSystemPrompt(basePrompt string, skillContexts []*SkillContext) string
}

// skillAgentIntegration Skill 与 Agent 集成实现
type skillAgentIntegration struct {
	// 格式化选项
	includeMetadata    bool
	includeReason     bool
	maxContentLength  int
}

// NewSkillAgentIntegration 创建 Skill 与 Agent 集成
func NewSkillAgentIntegration(opts ...IntegrationOption) SkillAgentIntegration {
	integration := &skillAgentIntegration{
		includeMetadata:   true,
		includeReason:     true,
		maxContentLength:  10000, // 最大内容长度
	}

	for _, opt := range opts {
		opt(integration)
	}

	return integration
}

// IntegrationOption 集成选项
type IntegrationOption func(*skillAgentIntegration)

// WithIncludeMetadata 包含元数据
func WithIncludeMetadata(include bool) IntegrationOption {
	return func(i *skillAgentIntegration) {
		i.includeMetadata = include
	}
}

// WithIncludeReason 包含匹配原因
func WithIncludeReason(include bool) IntegrationOption {
	return func(i *skillAgentIntegration) {
		i.includeReason = include
	}
}

// WithMaxContentLength 设置最大内容长度
func WithMaxContentLength(length int) IntegrationOption {
	return func(i *skillAgentIntegration) {
		i.maxContentLength = length
	}
}

// FormatForAgent 将 Skill 格式化为 Agent 可用的内容
func (i *skillAgentIntegration) FormatForAgent(ctx context.Context, skill *Skill) (*SkillContext, error) {
	var builder strings.Builder

	// 写入标题
	builder.WriteString(fmt.Sprintf("## Skill: %s\n\n", skill.Name))

	// 写入描述
	if skill.Description != "" {
		builder.WriteString(fmt.Sprintf("**Description:** %s\n\n", skill.Description))
	}

	// 写入使用时机
	if skill.WhenToUse != "" {
		builder.WriteString(fmt.Sprintf("**When to use:** %s\n\n", skill.WhenToUse))
	}

	// 写入元数据
	if i.includeMetadata {
		metadata := i.buildMetadata(skill)
		if metadata != "" {
			builder.WriteString("**Metadata:**\n")
			builder.WriteString(metadata)
			builder.WriteString("\n")
		}
	}

	// 写入匹配原因
	if i.includeReason && skill.Source != "" {
		// 从来源推断匹配原因
		reason := i.inferMatchReason(skill)
		if reason != "" {
			builder.WriteString(fmt.Sprintf("**Match reason:** %s\n\n", reason))
		}
	}

	// 写入内容
	content := skill.Content
	if i.maxContentLength > 0 && len(content) > i.maxContentLength {
		content = content[:i.maxContentLength] + "\n\n...(content truncated due to length)"
	}

	builder.WriteString("**Instructions:**\n\n")
	builder.WriteString(content)

	return &SkillContext{
		Skill:            skill,
		FormattedContent: builder.String(),
		MatchReason:      i.inferMatchReason(skill),
	}, nil
}

// FormatMultipleForAgent 批量格式化多个 Skill
func (i *skillAgentIntegration) FormatMultipleForAgent(ctx context.Context, skills []*Skill) ([]*SkillContext, error) {
	contexts := make([]*SkillContext, 0, len(skills))

	for _, skill := range skills {
		ctx, err := i.FormatForAgent(ctx, skill)
		if err != nil {
			return nil, fmt.Errorf("failed to format skill %s: %w", skill.Name, err)
		}
		contexts = append(contexts, ctx)
	}

	return contexts, nil
}

// InjectIntoSystemMessage 将 Skill 内容注入到系统消息
func (i *skillAgentIntegration) InjectIntoSystemMessage(systemMsg string, skillContext *SkillContext) string {
	var builder strings.Builder

	// 原系统消息
	if systemMsg != "" {
		builder.WriteString(systemMsg)
		builder.WriteString("\n\n")
	}

	// Skill 内容
	builder.WriteString("--- Active Skill Instructions ---\n\n")
	builder.WriteString(skillContext.FormattedContent)

	return builder.String()
}

// BuildSystemPrompt 构建包含 Skill 的系统提示
func (i *skillAgentIntegration) BuildSystemPrompt(basePrompt string, skillContexts []*SkillContext) string {
	var builder strings.Builder

	// 基础提示
	if basePrompt != "" {
		builder.WriteString(basePrompt)
		builder.WriteString("\n\n")
	}

	// 如果有 Skill，添加分隔符
	if len(skillContexts) > 0 {
		builder.WriteString("--- Active Skills ---\n")
		builder.WriteString("The following skills are available for this task:\n\n")

		// 添加每个 Skill
		for i, ctx := range skillContexts {
			if i > 0 {
				builder.WriteString("\n---\n\n")
			}
			builder.WriteString(ctx.FormattedContent)
		}

		builder.WriteString("\n--- End of Skills ---\n\n")
	}

	return builder.String()
}

// ========================================
// 辅助方法
// ========================================

// buildMetadata 构建元数据字符串
func (i *skillAgentIntegration) buildMetadata(skill *Skill) string {
	if !i.includeMetadata {
		return ""
	}

	var metadata []string

	if skill.Category != "" {
		metadata = append(metadata, fmt.Sprintf("- Category: %s", skill.Category))
	}

	if len(skill.Tags) > 0 {
		metadata = append(metadata, fmt.Sprintf("- Tags: %s", strings.Join(skill.Tags, ", ")))
	}

	if skill.Author != "" {
		metadata = append(metadata, fmt.Sprintf("- Author: %s", skill.Author))
	}

	if skill.Version != "" {
		metadata = append(metadata, fmt.Sprintf("- Version: %s", skill.Version))
	}

	if skill.Experimental {
		metadata = append(metadata, "- Status: Experimental")
	}

	if skill.IsPureGuidance() {
		metadata = append(metadata, "- Type: Pure Guidance (no tools)")
	} else {
		if len(skill.AllowedTools) > 0 {
			metadata = append(metadata, fmt.Sprintf("- Allowed Tools: %s", strings.Join(skill.AllowedTools, ", ")))
		}
		if len(skill.DisallowedTools) > 0 {
			metadata = append(metadata, fmt.Sprintf("- Disallowed Tools: %s", strings.Join(skill.DisallowedTools, ", ")))
		}
	}

	return strings.Join(metadata, "\n")
}

// inferMatchReason 推断匹配原因
func (i *skillAgentIntegration) inferMatchReason(skill *Skill) string {
	if skill.WhenToUse != "" {
		return skill.WhenToUse
	}

	if skill.Description != "" {
		return skill.Description
	}

	return "Skill matched based on task description"
}

// ========================================
// Skill 注入辅助函数
// ========================================

// InjectSkillsIntoPrompt 将 Skills 注入到提示中
// 便捷函数，用于快速将 Skills 注入到系统提示
func InjectSkillsIntoPrompt(basePrompt string, skills []*Skill) string {
	integration := NewSkillAgentIntegration()

	contexts, err := integration.FormatMultipleForAgent(context.Background(), skills)
	if err != nil {
		// 格式化失败，返回原提示
		return basePrompt
	}

	return integration.BuildSystemPrompt(basePrompt, contexts)
}

// GetSkillContextForTask 根据任务获取 Skill 上下文
// 便捷函数，用于根据任务描述获取相关的 Skill
func GetSkillContextForTask(task string, limit int) ([]*SkillContext, error) {
	manager := GetGlobalManager()

	// 匹配 Skill
	matches := manager.MatchSkillsForTask(task, limit)

	// 转换为 Skill 列表
	skills := make([]*Skill, 0, len(matches))
	for _, match := range matches {
		skills = append(skills, match.Skill)
	}

	// 格式化
	integration := NewSkillAgentIntegration()
	return integration.FormatMultipleForAgent(context.Background(), skills)
}

// ========================================
// Skill 工具调用支持
// ========================================

// SkillToolInvocation Skill 工具调用信息
type SkillToolInvocation struct {
	SkillName   string                 `json:"skill_name"`
	Parameters  map[string]interface{} `json:"parameters"`
	Reason      string                 `json:"reason"`
}

// GetSkillToolInvocations 从 Skill 获取工具调用信息
// 如果 Skill 定义了 allowed_tools，返回相关工具调用信息
func GetSkillToolInvocations(skill *Skill, task string) []SkillToolInvocation {
	if skill.IsPureGuidance() {
		return nil
	}

	// 这里可以根据 Skill 的 allowed_tools 和任务描述
	// 生成工具调用建议
	// TODO: 实现更智能的工具调用推断

	return nil
}
