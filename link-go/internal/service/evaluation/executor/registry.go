// Package executor 提供评测执行器实现
package executor

import (
	"context"
	"fmt"

	domeval "link/internal/model/evaluation"
)

// ========================================
// Executor Interface
// ========================================

// Executor 评测执行器接口
type Executor interface {
	// Execute 执行评测
	Execute(ctx context.Context, task *domeval.EvaluationTaskConfig, dataset []*domeval.QAPair) ([]*domeval.QAResult, error)

	// Type 返回执行器类型
	Type() domeval.EvaluationType
}

// ========================================
// Executor Registry
// ========================================

// ExecutorRegistry 执行器注册表
type ExecutorRegistry struct {
	executors map[domeval.EvaluationType]Executor
}

// NewExecutorRegistry 创建执行器注册表
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[domeval.EvaluationType]Executor),
	}
}

// Register 注册执行器。
// 注册键与 executor.Type() 一致，同时为其别名类型（如 qa <-> llm）登记同一实例，
// 使 Worker 用任务原始 type 查找时 qa / llm 都能命中同一执行器。
func (r *ExecutorRegistry) Register(executor Executor) error {
	if executor == nil {
		return fmt.Errorf("executor is nil")
	}

	typ := executor.Type()
	if !typ.IsValid() {
		return fmt.Errorf("invalid executor type: %s", typ)
	}

	r.executors[typ] = executor
	for _, alias := range aliasKeys(typ) {
		if _, exists := r.executors[alias]; !exists {
			r.executors[alias] = executor
		}
	}
	return nil
}

// Get 获取执行器。找不到时回退到归一化后的类型（qa -> llm）再试一次，
// 保证历史别名与规范类型都能路由到同一执行器。
func (r *ExecutorRegistry) Get(typ domeval.EvaluationType) (Executor, error) {
	if executor, ok := r.executors[typ]; ok {
		return executor, nil
	}
	if norm := typ.Normalize(); norm != typ {
		if executor, ok := r.executors[norm]; ok {
			return executor, nil
		}
	}
	return nil, fmt.Errorf("executor not found for type %q (registered: %v)", typ, r.List())
}

// aliasKeys 返回与给定类型互为别名、应解析到同一执行器的其它注册键。
func aliasKeys(typ domeval.EvaluationType) []domeval.EvaluationType {
	switch typ {
	case domeval.EvaluationTypeQA:
		return []domeval.EvaluationType{domeval.EvaluationTypeLLM}
	case domeval.EvaluationTypeLLM:
		return []domeval.EvaluationType{domeval.EvaluationTypeQA}
	}
	return nil
}

// MustGet 获取执行器，不存在则 panic
func (r *ExecutorRegistry) MustGet(typ domeval.EvaluationType) Executor {
	executor, err := r.Get(typ)
	if err != nil {
		panic(err)
	}
	return executor
}

// List 列出已注册的执行器类型
func (r *ExecutorRegistry) List() []domeval.EvaluationType {
	types := make([]domeval.EvaluationType, 0, len(r.executors))
	for typ := range r.executors {
		types = append(types, typ)
	}
	return types
}

// Has 检查是否有指定类型的执行器
func (r *ExecutorRegistry) Has(typ domeval.EvaluationType) bool {
	_, ok := r.executors[typ]
	return ok
}
