// Package chat provides Chat domain-specific error definitions
package conversation

import (
	"errors"
	"fmt"
)

var (
	// ErrSessionNotFound indicates a session could not be found
	ErrSessionNotFound = errors.New("session not found")

	// ErrSessionArchived indicates a session is archived
	ErrSessionArchived = errors.New("session is archived")

	// ErrMessageNotFound indicates a message could not be found
	ErrMessageNotFound = errors.New("message not found")

	// ErrInvalidSessionState indicates an invalid session state transition
	ErrInvalidSessionState = errors.New("invalid session state")

	// ErrEmptyMessage indicates an empty message content
	ErrEmptyMessage = errors.New("message content cannot be empty")

	// ErrSessionLimitExceeded indicates the user has exceeded session limit
	ErrSessionLimitExceeded = errors.New("session limit exceeded")
)

// ChatError represents a chat-specific domain error
type ChatError struct {
	Code    string
	Message string
	Err     error
}

// Error implements the error interface
func (e *ChatError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap implements the errors.Unwrap interface
func (e *ChatError) Unwrap() error {
	return e.Err
}

// NewChatError creates a new chat error
func NewChatError(code, message string, err error) *ChatError {
	return &ChatError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Error codes
const (
	ErrorCodeSessionNotFound     = "SESSION_NOT_FOUND"
	ErrorCodeSessionArchived     = "SESSION_ARCHIVED"
	ErrorCodeMessageNotFound     = "MESSAGE_NOT_FOUND"
	ErrorCodeInvalidSessionState = "INVALID_SESSION_STATE"
	ErrorCodeEmptyMessage        = "EMPTY_MESSAGE"
)

// Common error constructors
func SessionNotFoundError(id string, err error) *ChatError {
	return NewChatError(ErrorCodeSessionNotFound, fmt.Sprintf("session '%s' not found", id), err)
}

func MessageNotFoundError(id string) *ChatError {
	return NewChatError(ErrorCodeMessageNotFound, fmt.Sprintf("message '%s' not found", id), nil)
}

func InvalidSessionStateError(current, expected string) *ChatError {
	return NewChatError(ErrorCodeInvalidSessionState, fmt.Sprintf("invalid session state: expected %s, got %s", expected, current), nil)
}
