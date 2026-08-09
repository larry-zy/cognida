// Package semantic 提供指标语义层领域错误定义。
package semantic

import "errors"

var (
	// ErrModelNotFound 语义模型不存在。
	ErrModelNotFound = errors.New("semantic model not found")

	// ErrModelNotActive 语义模型未生效（非 active 状态不参与查询主路径）。
	ErrModelNotActive = errors.New("semantic model not active")

	// ErrMetricNotFound 指标不存在。
	ErrMetricNotFound = errors.New("semantic metric not found")

	// ErrNotCovered 语义模型未覆盖目标库表/指标，调用方应回退词法 NL2SQL。
	ErrNotCovered = errors.New("semantic model does not cover target")

	// ErrRepository 仓储操作失败。
	ErrRepository = errors.New("semantic repository operation failed")
)
