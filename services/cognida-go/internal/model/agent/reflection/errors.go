// Package reflection defines domain errors for reflection capability.
package reflection

import "errors"

// ========================================
// 领域错误
// ========================================

var (
	// ErrReflectionDisabled 反思功能未启用
	ErrReflectionDisabled = errors.New("reflection is disabled")

	// ErrMaxIterationsReached 达到最大迭代次数
	ErrMaxIterationsReached = errors.New("max iterations reached")

	// ErrInvalidMaxIterations 无效的最大迭代次数
	ErrInvalidMaxIterations = errors.New("max iterations must be between 1 and 10")

	// ErrInvalidCriticType 无效的 Critic 类型
	ErrInvalidCriticType = errors.New("critic type must be 'llm' or 'rule'")

	// ErrCriticEvaluationFailed Critic 评估失败
	ErrCriticEvaluationFailed = errors.New("critic evaluation failed")

	// ErrMemoryNotFound 未找到相关记忆
	ErrMemoryNotFound = errors.New("no relevant memory found")

	// ErrMemoryStoreFailed 存储记忆失败
	ErrMemoryStoreFailed = errors.New("failed to store memory")
)
