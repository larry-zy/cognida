// Package agent provides error types for multi-agent collaboration.
package framework

import (
	"fmt"
)

// ========================================
// Collaboration Errors
// ========================================

// Standard collaboration errors.
var (
	// ErrAgentNotFound is returned when an agent is not found in the registry.
	ErrAgentNotFound = fmt.Errorf("agent not found")

	// ErrAgentAlreadyRegistered is returned when trying to register an already registered agent.
	ErrAgentAlreadyRegistered = fmt.Errorf("agent already registered")
)

// CollabLoopError is returned when a collaboration loop is detected.
type CollabLoopError struct {
	Path     []string
	Target   string
	Message  string
}

// Error returns the error message with full loop path.
func (e *CollabLoopError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	pathStr := formatPath(e.Path)
	return fmt.Sprintf("collaboration loop detected: %s -> %s", pathStr, e.Target)
}

// NewCollabLoopError creates a new collaboration loop error.
func NewCollabLoopError(path []string, target string) *CollabLoopError {
	return &CollabLoopError{
		Path:   path,
		Target: target,
	}
}

// formatPath formats the delegation path for display.
func formatPath(path []string) string {
	if len(path) == 0 {
		return "<start>"
	}
	result := ""
	for i, p := range path {
		if i > 0 {
			result += " -> "
		}
		result += p
	}
	return result
}

// ========================================
// Error Helper Functions
// ========================================

// FormatAgentNotFoundError formats an agent not found error with available agents.
func FormatAgentNotFoundError(agentName string, availableAgents []AgentInfo) error {
	suggestions := "\nAvailable agents:\n"
	for _, a := range availableAgents {
		suggestions += fmt.Sprintf("  - %s: %s\n", a.Name, a.Description)
	}
	return fmt.Errorf("%w: '%s'%s", ErrAgentNotFound, agentName, suggestions)
}

// IsAgentNotFound checks if an error is an agent not found error.
func IsAgentNotFound(err error) bool {
	return err != nil && (err == ErrAgentNotFound || containsSubstring(err.Error(), "agent not found"))
}

// IsCollabLoopError checks if an error is a collaboration loop error.
func IsCollabLoopError(err error) bool {
	_, ok := err.(*CollabLoopError)
	return ok
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsInString(s, substr))
}

func containsInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
