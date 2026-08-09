// Package agent provides type aliases for compatibility with tests
package agent

import (
	"context"
)

// Type aliases for infrastructure/agent types
// These are needed for test compatibility during the migration

// Agent is an alias for the Agent interface from infrastructure
type Agent = interface {
	Chat(ctx context.Context, message string) (*Response, error)
	Stream(ctx context.Context, message string) (<-chan *Chunk, error)
	Name() string
}

// Response is a placeholder - actual type comes from infrastructure/agent
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Metadata  map[string]interface{}
}

// Chunk is a placeholder - actual type comes from infrastructure/agent
type Chunk struct {
	Content  string
	Done     bool
	Metadata map[string]interface{}
}

// ToolCall represents a tool call
type ToolCall struct {
	Name   string
	Input  map[string]interface{}
	Output string
}

// These type definitions are simplified versions for test compatibility
// The full types are defined in internal/infrastructure/agent/eino_agent.go
