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

// Register 注册执行器
func (r *ExecutorRegistry) Register(executor Executor) error {
	if executor == nil {
		return fmt.Errorf("executor is nil")
	}

	typ := executor.Type()
	if !typ.IsValid() {
		return fmt.Errorf("invalid executor type: %s", typ)
	}

	r.executors[typ] = executor
	return nil
}

// Get 获取执行器
func (r *ExecutorRegistry) Get(typ domeval.EvaluationType) (Executor, error) {
	executor, ok := r.executors[typ]
	if !ok {
		return nil, fmt.Errorf("executor not found for type: %s", typ)
	}
	return executor, nil
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
