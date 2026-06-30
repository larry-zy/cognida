// Package skills 提供 Skill 中间件
// 负责在 Agent 执行时自动匹配和注入相关 Skill
package skills

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// ========================================
// SkillMiddleware Skill 中间件
// ========================================

// SkillMiddleware Skill 中间件接口
type SkillMiddleware interface {
	// BeforeExecution 在 Agent 执行前匹配和注入 Skill
	BeforeExecution(ctx context.Context, message string) (context.Context, string, error)

	// AfterExecution 在 Agent 执行后清理（可选）
	AfterExecution(ctx context.Context, result interface{}) error
}

// ========================================
// AutoSkillMiddleware 自动 Skill 匹配中间件
// ========================================

// AutoSkillMiddleware 自动匹配和注入 Skill 的中间件
type AutoSkillMiddleware struct {
	skillManager SkillManager
	integration   SkillAgentIntegration
	enabled       bool
	maxSkills     int // 最多注入的 Skill 数量
}

// NewAutoSkillMiddleware 创建自动 Skill 中间件
func NewAutoSkillMiddleware(opts ...MiddlewareOption) SkillMiddleware {
	m := &AutoSkillMiddleware{
		skillManager: GetGlobalManager(),
		integration:  NewSkillAgentIntegration(),
		enabled:      true,
		maxSkills:    3, // 默认最多注入 3 个 Skill
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// MiddlewareOption 中间件选项
type MiddlewareOption func(*AutoSkillMiddleware)

// WithSkillManager 使用指定的 Skill 管理器
func WithSkillManager(manager SkillManager) MiddlewareOption {
	return func(m *AutoSkillMiddleware) {
		m.skillManager = manager
	}
}

// WithSkillIntegration 使用指定的集成器
func WithSkillIntegration(integration SkillAgentIntegration) MiddlewareOption {
	return func(m *AutoSkillMiddleware) {
		m.integration = integration
	}
}

// WithMaxSkills 设置最多注入的 Skill 数量
func WithMaxSkills(max int) MiddlewareOption {
	return func(m *AutoSkillMiddleware) {
		m.maxSkills = max
	}
}

// WithEnabled 设置是否启用
func WithEnabled(enabled bool) MiddlewareOption {
	return func(m *AutoSkillMiddleware) {
		m.enabled = enabled
	}
}

// BeforeExecution 在 Agent 执行前匹配和注入 Skill
func (m *AutoSkillMiddleware) BeforeExecution(ctx context.Context, message string) (context.Context, string, error) {
	if !m.enabled {
		return ctx, message, nil
	}

	// 匹配相关 Skill
	matches := m.skillManager.MatchSkillsForTask(message, m.maxSkills)

	if len(matches) == 0 {
		// 没有匹配的 Skill，直接返回
		return ctx, message, nil
	}

	// 提取 Skill
	matchedSkills := make([]*Skill, 0, len(matches))
	for _, match := range matches {
		if match.Relevance > 0.3 { // 相关度阈值
			matchedSkills = append(matchedSkills, match.Skill)
		}
	}

	if len(matchedSkills) == 0 {
		return ctx, message, nil
	}

	// 格式化 Skill 内容
	skillContexts, err := m.integration.FormatMultipleForAgent(ctx, matchedSkills)
	if err != nil {
		log.Printf("[SkillMiddleware] 格式化 Skill 失败: %v", err)
		return ctx, message, nil
	}

	// 构建 Skill 前缀
	skillPrefix := m.buildSkillPrefix(skillContexts)

	// 将 Skill 注入到上下文
	ctx = contextWithSkills(ctx, skillContexts)

	// 返回增强后的消息
	enhancedMessage := skillPrefix + "\n\n" + message

	log.Printf("[SkillMiddleware] 已注入 %d 个 Skill: %s",
		len(skillContexts), formatSkillNames(matchedSkills))

	return ctx, enhancedMessage, nil
}

// AfterExecution 在 Agent 执行后清理（暂无操作）
func (m *AutoSkillMiddleware) AfterExecution(ctx context.Context, result interface{}) error {
	return nil
}

// buildSkillPrefix 构建 Skill 前缀
func (m *AutoSkillMiddleware) buildSkillPrefix(skillContexts []*SkillContext) string {
	var sb strings.Builder

	sb.WriteString("--- 相关指导 ---\n\n")

	for i, ctx := range skillContexts {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}

		// 简化版本：只包含关键信息
		sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", ctx.Skill.Name, ctx.Skill.Description))

		if ctx.Skill.WhenToUse != "" {
			sb.WriteString(fmt.Sprintf("*适用场景*: %s\n\n", ctx.Skill.WhenToUse))
		}

		// 包含关键指导内容
		content := ctx.FormattedContent
		if len(content) > 2000 {
			// 截断过长内容
			content = content[:2000] + "\n\n...(内容过长，已截断)"
		}
		sb.WriteString(content)
	}

	sb.WriteString("\n--- 指导结束 ---")

	return sb.String()
}

// ========================================
// 上下文管理
// ========================================

// contextKey 上下文 key 类型
type contextKey int

const (
	skillsContextKey contextKey = iota
)

// contextWithSkills 将 Skills 添加到上下文
func contextWithSkills(ctx context.Context, skillContexts []*SkillContext) context.Context {
	return context.WithValue(ctx, skillsContextKey, skillContexts)
}

// SkillsFromContext 从上下文获取 Skills
func SkillsFromContext(ctx context.Context) []*SkillContext {
	if val := ctx.Value(skillsContextKey); val != nil {
		if contexts, ok := val.([]*SkillContext); ok {
			return contexts
		}
	}
	return nil
}

// ========================================
// 辅助函数
// ========================================

// formatSkillNames 格式化 Skill 名称
func formatSkillNames(skills []*Skill) string {
	names := make([]string, len(skills))
	for i, skill := range skills {
		names[i] = skill.Name
	}
	return strings.Join(names, ", ")
}

// ========================================
// ManualSkillMiddleware 手动 Skill 选择中间件
// ========================================

// ManualSkillMiddleware 手动选择 Skill 的中间件
// Agent 需要显式调用 skill_invoke 工具来加载 Skill
type ManualSkillMiddleware struct {
	enabled bool
}

// NewManualSkillMiddleware 创建手动 Skill 中间件
func NewManualSkillMiddleware() SkillMiddleware {
	return &ManualSkillMiddleware{
		enabled: true,
	}
}

// BeforeExecution 不自动注入，等待 Agent 调用 skill_invoke
func (m *ManualSkillMiddleware) BeforeExecution(ctx context.Context, message string) (context.Context, string, error) {
	if !m.enabled {
		return ctx, message, nil
	}

	// 不做任何注入，Agent 需要通过工具调用 skill_invoke
	return ctx, message, nil
}

// AfterExecution 不做任何清理
func (m *ManualSkillMiddleware) AfterExecution(ctx context.Context, result interface{}) error {
	return nil
}
