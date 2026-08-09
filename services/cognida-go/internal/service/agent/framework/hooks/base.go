// Package hooks provides Agent lifecycle hook implementations.
// These hooks implement the domain.HookService interface.
package hooks

import (
	"context"
	"fmt"
	"time"

	"cognida/internal/model/agent"
)

// ========================================
// LLMClient LLM 调用接口
// ========================================

// LLMClient defines LLM calling interface for hook implementations.
// This is an infrastructure-level interface for LLM integration.
type LLMClient interface {
	Chat(ctx context.Context, messages []Message) (string, error)
}

// Message represents an LLM message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ========================================
// BaseHook - Hook 通用实现
// ========================================

// BaseHook provides common HookService functionality.
// It implements the domain.HookService interface.
type BaseHook struct {
	enabled      bool
	name         string
	llm          LLMClient
	timeout      time.Duration
	errorHandler func(error) error
}

// Ensure BaseHook implements domain.HookService
var _ agent.HookService = (*BaseHook)(nil)

// NewBaseHook creates a new BaseHook instance.
func NewBaseHook(name string, llm LLMClient) *BaseHook {
	return &BaseHook{
		enabled: false,
		name:    name,
		llm:     llm,
		timeout: 30 * time.Second,
		errorHandler: func(err error) error {
			// 默认错误处理器：记录但不中断
			return fmt.Errorf("hook %s error: %w", name, err)
		},
	}
}

// Enable enables the hook.
func (h *BaseHook) Enable() *BaseHook {
	h.enabled = true
	return h
}

// Disable disables the hook.
func (h *BaseHook) Disable() *BaseHook {
	h.enabled = false
	return h
}

// IsEnabled checks if the hook is enabled.
func (h *BaseHook) IsEnabled() bool {
	return h.enabled
}

// WithTimeout sets the timeout for hook execution.
func (h *BaseHook) WithTimeout(d time.Duration) *BaseHook {
	h.timeout = d
	return h
}

// WithErrorHandler sets a custom error handler.
func (h *BaseHook) WithErrorHandler(fn func(error) error) *BaseHook {
	h.errorHandler = fn
	return h
}

// Before implements domain.HookService.Before.
// Default implementation returns the context and message unchanged if enabled.
func (h *BaseHook) Before(ctx context.Context, message string) (context.Context, string, error) {
	if !h.IsEnabled() {
		return ctx, message, nil
	}
	// Base implementation does nothing - override in specific hooks
	return ctx, message, nil
}

// After implements domain.HookService.After.
// Default implementation does nothing if enabled.
func (h *BaseHook) After(ctx context.Context, resp interface{}) error {
	if !h.IsEnabled() {
		return nil
	}
	// Base implementation does nothing - override in specific hooks
	return nil
}

// SafeExecute provides common error recovery and timeout handling.
func (h *BaseHook) SafeExecute(ctx context.Context, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("hook panic: %v", r)
		}
		if err != nil && h.errorHandler != nil {
			err = h.errorHandler(err)
		}
	}()

	// Apply timeout
	if h.timeout > 0 {
		var cancel context.CancelFunc
		_, cancel = context.WithTimeout(ctx, h.timeout)
		defer cancel()
	}

	return fn()
}

// ========================================
// Legacy helper functions (for backward compatibility)
// ========================================

// ToBeforeHook converts a function to the BeforeHook signature.
// Deprecated: Use direct implementation of domain.HookService instead.
func (h *BaseHook) ToBeforeHook(fn func(context.Context, string) (context.Context, string, error)) func(context.Context, string) (context.Context, string, error) {
	return func(ctx context.Context, message string) (context.Context, string, error) {
		if !h.IsEnabled() {
			return ctx, message, nil
		}
		return fn(ctx, message)
	}
}

// ToAfterHook converts a function to the AfterHook signature.
// Deprecated: Use direct implementation of domain.HookService instead.
func (h *BaseHook) ToAfterHook(fn func(context.Context, interface{}) error) func(context.Context, interface{}) error {
	return func(ctx context.Context, resp interface{}) error {
		if !h.IsEnabled() {
			return nil
		}
		return h.SafeExecute(ctx, func() error {
			return fn(ctx, resp)
		})
	}
}
