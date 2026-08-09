// Package reflection provides the domain interfaces for Agent self-reflection capability.
package reflection

import "context"

// ========================================
// Critic 评估器接口
// ========================================

// Critic 定义了评估 Agent 输出的接口。
// 可插拔设计支持不同的评估策略（LLM、规则、自定义等）。
type Critic interface {
	// Evaluate 评估给定任务的输出质量，返回批评结果。
	Evaluate(ctx context.Context, task, output string) (*CritiqueResult, error)

	// ShouldRefine 判断根据批评结果是否需要改进。
	ShouldRefine(result *CritiqueResult) bool
}

// DimensionEvaluator 维度评估器接口
// 支持自定义评估维度的具体逻辑
type DimensionEvaluator interface {
	// EvaluateDimension 评估单个维度
	EvaluateDimension(ctx context.Context, task, output string) (DimensionScore, error)
}
