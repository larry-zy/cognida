// Package tools provides infrastructure-level tool implementations for the Agent framework.
//
// This package contains tool adapters and factories for integrating external
// services with the Agent framework.
//
// The tools are implemented in service/agent/tools and use Service interfaces
// for business logic integration.
package tools

import (
	"github.com/cloudwego/eino/components/tool"

	"link/internal/model/agent"
)

// ========================================
// Eino Tool Adapter
// ========================================

// EinoToolAdapter adapts Eino framework tools to domain Tool interface.
type EinoToolAdapter struct{}

// NewEinoToolAdapter creates a new Eino tool adapter.
func NewEinoToolAdapter() *EinoToolAdapter {
	return &EinoToolAdapter{}
}

// ToDomainTool converts an Eino BaseTool to a domain Tool.
// This is a placeholder for future implementation.
func (a *EinoToolAdapter) ToDomainTool(einoTool tool.BaseTool) agent.Tool {
	// TODO: Implement proper conversion when domain.Tool is fully defined
	// For now, return a minimal domain Tool
	return agent.Tool{
		ID:          "placeholder",
		Name:        "placeholder",
		Description: "Placeholder for Eino tool conversion",
		Enabled:     true,
	}
}

// ToEinoTool converts a domain Tool to an Eino BaseTool.
// This is a placeholder for future implementation.
func (a *EinoToolAdapter) ToEinoTool(domainTool agent.Tool) tool.BaseTool {
	// TODO: Implement proper conversion when domain.Tool has full definition
	// For now, return nil
	return nil
}

// ========================================
// Tool Factories (Placeholder)
// ========================================

// These factory functions provide tool adapters for external services.

// NewRAGQueryTool creates a RAG query tool.
// TODO: Implement when needed for integration.
func NewRAGQueryTool() (tool.InvokableTool, error) {
	return nil, &ErrNotImplemented{Message: "RAG query tool not implemented"}
}

// NewGraphQueryTool creates a graph query tool.
// TODO: Implement when needed for integration.
func NewGraphQueryTool() (tool.InvokableTool, error) {
	return nil, &ErrNotImplemented{Message: "Graph query tool not implemented"}
}

// NewWebSearchTool creates a web search tool.
// TODO: Implement when needed for integration.
func NewWebSearchTool() (tool.InvokableTool, error) {
	return nil, &ErrNotImplemented{Message: "Web search tool not implemented"}
}

// ========================================
// Error Types
// ========================================

// ErrNotImplemented indicates a feature is not yet implemented.
type ErrNotImplemented struct {
	Message string
}

func (e *ErrNotImplemented) Error() string {
	return e.Message
}
