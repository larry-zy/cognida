// Package skills 提供 Skill 中间件适配器
// 将 Skill 中间件适配到 Agent 框架的中间件接口
package skills

import (
	"context"

	"link/internal/service/agent/framework"
)

// ========================================
// SkillMiddlewareAdapter Skill 中间件适配器
// ========================================

// skillMiddlewareAdapter 将 SkillMiddleware 适配为 framework.Middleware
type skillMiddlewareAdapter struct {
	skillMiddleware SkillMiddleware
}

// NewSkillMiddlewareAdapter 创建 Skill 中间件适配器
// 将 AutoSkillMiddleware 适配为 Agent 框架可用的中间件
func NewSkillMiddlewareAdapter(middleware SkillMiddleware) framework.Middleware {
	return &skillMiddlewareAdapter{
		skillMiddleware: middleware,
	}
}

// Before 实现框架中间件接口
func (a *skillMiddlewareAdapter) Before(ctx context.Context, message string) (context.Context, string, error) {
	return a.skillMiddleware.BeforeExecution(ctx, message)
}

// After 实现框架中间件接口
func (a *skillMiddlewareAdapter) After(ctx context.Context, resp *framework.Response) error {
	// 将 framework.Response 转换为 interface{} 传递给 Skill 中间件
	// 由于 Skill 中间件的 AfterExecution 通常不修改响应，这里简化处理
	return a.skillMiddleware.AfterExecution(ctx, resp)
}

// ========================================
// 便捷函数
// ========================================

// NewAutoSkillMiddlewareForAgent 创建可直接用于 Agent 的自动 Skill 中间件
// 返回 framework.Middleware 接口，可直接添加到 Agent
func NewAutoSkillMiddlewareForAgent(opts ...MiddlewareOption) framework.Middleware {
	skillMiddleware := NewAutoSkillMiddleware(opts...)
	return NewSkillMiddlewareAdapter(skillMiddleware)
}

// NewManualSkillMiddlewareForAgent 创建手动 Skill 中间件
// Agent 需要通过工具调用来使用 Skill
func NewManualSkillMiddlewareForAgent() framework.Middleware {
	skillMiddleware := NewManualSkillMiddleware()
	return NewSkillMiddlewareAdapter(skillMiddleware)
}
