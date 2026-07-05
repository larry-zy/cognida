// Package agent provides Agent infrastructure implementations.
package framework

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/tool"

	"link/internal/model/agent"
)

// ========================================
// ToolExecutor - Domain ToolExecutor 接口实现
// ========================================

// toolExecutorImpl implements the domain.ToolExecutor interface.
// It executes Eino framework tools.
type toolExecutorImpl struct {
	registry ToolRegistry
}

// Ensure toolExecutorImpl implements domain.ToolExecutor
var _ agent.ToolExecutor = (*toolExecutorImpl)(nil)

// NewToolExecutor creates a new tool executor.
func NewToolExecutor(registry ToolRegistry) agent.ToolExecutor {
	return &toolExecutorImpl{
		registry: registry,
	}
}

// Execute runs a tool synchronously and returns the result.
func (e *toolExecutorImpl) Execute(ctx context.Context, toolName string, input string) (string, error) {
	// Get the registry implementation to access Eino tools
	registryImpl, ok := e.registry.(*ToolRegistryImpl)
	if !ok {
		return "", fmt.Errorf("invalid registry type")
	}

	einoTool, exists := registryImpl.GetEinoTool(toolName)
	if !exists {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}

	if !e.registry.IsEnabled(toolName) {
		return "", fmt.Errorf("tool not enabled: %s", toolName)
	}

	// Execute based on tool type
	switch t := einoTool.(type) {
	case tool.InvokableTool:
		output, err := t.InvokableRun(ctx, input)
		if err != nil {
			return "", fmt.Errorf("tool execution failed: %w", err)
		}
		return output, nil

	case tool.StreamableTool:
		stream, err := t.StreamableRun(ctx, input)
		if err != nil {
			return "", fmt.Errorf("tool stream failed: %w", err)
		}
		defer stream.Close()

		// Collect stream output
		var result strings.Builder
		for {
			chunk, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				return "", fmt.Errorf("stream recv failed: %w", err)
			}
			result.WriteString(chunk)
		}
		return result.String(), nil

	default:
		return "", fmt.Errorf("unsupported tool type: %T", t)
	}
}

// ExecuteWithStream runs a tool with streaming output.
func (e *toolExecutorImpl) ExecuteWithStream(ctx context.Context, toolName string, input string) (<-chan string, error) {
	// Get the registry implementation to access Eino tools
	registryImpl, ok := e.registry.(*ToolRegistryImpl)
	if !ok {
		return nil, fmt.Errorf("invalid registry type")
	}

	einoTool, exists := registryImpl.GetEinoTool(toolName)
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	if !e.registry.IsEnabled(toolName) {
		return nil, fmt.Errorf("tool not enabled: %s", toolName)
	}

	// For streamable tools, return a wrapper channel
	if streamableTool, ok := einoTool.(tool.StreamableTool); ok {
		stream, err := streamableTool.StreamableRun(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("tool stream failed: %w", err)
		}

		// Create output channel
		outputChan := make(chan string, 16)

		go func() {
			defer close(outputChan)
			defer stream.Close()

			for {
				chunk, err := stream.Recv()
				if err != nil {
					if err == io.EOF {
						return
					}
					// Send error as message (or handle differently)
					outputChan <- fmt.Sprintf("[Error: %v]", err)
					return
				}
				outputChan <- chunk
			}
		}()

		return outputChan, nil
	}

	// For invokable tools, simulate streaming
	if invokableTool, ok := einoTool.(tool.InvokableTool); ok {
		outputChan := make(chan string, 1)

		go func() {
			defer close(outputChan)

			output, err := invokableTool.InvokableRun(ctx, input)
			if err != nil {
				outputChan <- fmt.Sprintf("[Error: %v]", err)
				return
			}
			outputChan <- output
		}()

		return outputChan, nil
	}

	return nil, fmt.Errorf("unsupported tool type: %T", einoTool)
}
