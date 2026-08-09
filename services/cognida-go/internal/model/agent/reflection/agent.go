// Package reflection provides the domain interfaces for Agent self-reflection capability.
package reflection

import "context"

// ========================================
// ReflectionAgent 反思 Agent 接口
// ========================================

// ReflectionAgent 定义了带反思能力的 Agent 接口。
// 它包装原有的 Agent，在输出后进行自我评估和改进。
type ReflectionAgent interface {
	// ChatWithReflection 执行带反思的对话。
	// 返回经过反思改进后的响应。
	ChatWithReflection(ctx context.Context, task string) (*ReflectionResult, error)

	// IsEnabled 检查反思功能是否启用。
	IsEnabled() bool
}

// ReflectionResult 反思结果
type ReflectionResult struct {
	Content      string                 // 最终输出内容
	Iterations   int                    // 实际迭代次数
	InitialScore float64                // 初始评分
	FinalScore   float64                // 最终评分
	UsedMemory   bool                   // 是否使用了历史记忆
	FromCache    bool                   // 是否来自缓存
	Metadata     map[string]interface{} // 附加元数据
}
