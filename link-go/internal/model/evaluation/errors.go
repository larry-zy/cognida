// Package evaluation 提供评测领域错误定义
package evaluation

import "errors"

// ========================================
// Domain Errors
// ========================================

var (
	// ErrTaskNotFound 任务不存在
	ErrTaskNotFound = errors.New("evaluation task not found")

	// ErrTaskAlreadyExists 任务已存在
	ErrTaskAlreadyExists = errors.New("evaluation task already exists")

	// ErrInvalidStatus 无效的状态转换
	ErrInvalidStatus = errors.New("invalid task status transition")

	// ErrInvalidEvalType 无效的评测类型
	ErrInvalidEvalType = errors.New("invalid evaluation type")

	// ErrDatasetNotFound 数据集不存在
	ErrDatasetNotFound = errors.New("dataset not found")

	// ErrDatasetTypeMismatch 数据集类型不匹配
	ErrDatasetTypeMismatch = errors.New("dataset type mismatch")

	// ErrDatasetAlreadyExists 数据集已存在
	ErrDatasetAlreadyExists = errors.New("dataset already exists")

	// ErrDatasetNameEmpty 数据集名称为空
	ErrDatasetNameEmpty = errors.New("dataset name cannot be empty")

	// ErrDatasetSampleNotFound 样本不存在
	ErrDatasetSampleNotFound = errors.New("dataset sample not found")

	// ErrInvalidDatasetType 无效的数据集类型
	ErrInvalidDatasetType = errors.New("invalid dataset type")

	// ErrRepository 仓储操作失败
	ErrRepository = errors.New("repository operation failed")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("invalid configuration")
)
