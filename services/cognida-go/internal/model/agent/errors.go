// Package agent provides Agent domain-specific error definitions
package agent

import (
	"errors"
	"fmt"
)

var (
	// ErrAgentNotFound indicates an agent could not be found
	ErrAgentNotFound = errors.New("agent not found")

	// ErrToolNotFound indicates a tool could not be found
	ErrToolNotFound = errors.New("tool not found")

	// ErrToolExecutionFailed indicates a tool execution failed
	ErrToolExecutionFailed = errors.New("tool execution failed")

	// ErrMaxIterationsExceeded indicates the agent exceeded maximum iterations
	ErrMaxIterationsExceeded = errors.New("max iterations exceeded")

	// ErrInvalidAgentConfig indicates the agent configuration is invalid
	ErrInvalidAgentConfig = errors.New("invalid agent config")

	// ErrAgentNotInitialized indicates the agent orchestrator is not initialized
	ErrAgentNotInitialized = errors.New("agent orchestrator not initialized")

	// ErrToolAlreadyRegistered indicates a tool is already registered
	ErrToolAlreadyRegistered = errors.New("tool already registered")

	// ErrToolNotEnabled indicates a tool is not enabled
	ErrToolNotEnabled = errors.New("tool not enabled")
)

// AgentError represents an agent-specific domain error
type AgentError struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface
func (e *AgentError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface
func (e *AgentError) Unwrap() error {
	return e.Err
}

// NewAgentError creates a new agent error
func NewAgentError(code, message string, err error) *AgentError {
	return &AgentError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Error codes
const (
	ErrorCodeAgentNotFound         = "AGENT_NOT_FOUND"
	ErrorCodeToolNotFound          = "TOOL_NOT_FOUND"
	ErrorCodeToolExecutionFailed   = "TOOL_EXECUTION_FAILED"
	ErrorCodeMaxIterationsExceeded = "MAX_ITERATIONS_EXCEEDED"
	ErrorCodeInvalidConfig         = "INVALID_AGENT_CONFIG"
)

// Common error constructors
func AgentNotFoundError(name string, err error) *AgentError {
	return NewAgentError(ErrorCodeAgentNotFound, fmt.Sprintf("agent '%s' not found", name), err)
}

func ToolNotFoundError(name string) *AgentError {
	return NewAgentError(ErrorCodeToolNotFound, fmt.Sprintf("tool '%s' not found", name), nil)
}

func ToolExecutionFailedError(tool string, err error) *AgentError {
	return NewAgentError(ErrorCodeToolExecutionFailed, fmt.Sprintf("tool '%s' execution failed", tool), err)
}
