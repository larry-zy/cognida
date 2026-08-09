// Package types provides common cross-domain types
package common

// ========================================
// Model Source Types
// ========================================

// ModelSource 模型源类型
type ModelSource string

const (
	// ModelSourceLocal 本地模型
	ModelSourceLocal ModelSource = "local"
	// ModelSourceRemote 远程模型
	ModelSourceRemote ModelSource = "remote"
)
