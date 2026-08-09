package framework

import (
	"context"
	"log"
	"time"
)

// Middleware defines the interface for agent middleware.
// Middleware can intercept and modify requests/responses.
type Middleware interface {
	// Before is called before the agent processes a message.
	// It can modify the context and message.
	Before(ctx context.Context, message string) (context.Context, string, error)

	// After is called after the agent generates a response.
	// It can modify the response.
	After(ctx context.Context, resp *Response) error
}

// MiddlewareFunc is a function adapter for Middleware.
type MiddlewareFunc struct {
	beforeFunc func(ctx context.Context, message string) (context.Context, string, error)
	afterFunc  func(ctx context.Context, resp *Response) error
}

// Before implements Middleware.Before.
func (m *MiddlewareFunc) Before(ctx context.Context, message string) (context.Context, string, error) {
	if m.beforeFunc != nil {
		return m.beforeFunc(ctx, message)
	}
	return ctx, message, nil
}

// After implements Middleware.After.
func (m *MiddlewareFunc) After(ctx context.Context, resp *Response) error {
	if m.afterFunc != nil {
		return m.afterFunc(ctx, resp)
	}
	return nil
}

// NewMiddleware creates a Middleware from functions.
func NewMiddleware(
	before func(ctx context.Context, message string) (context.Context, string, error),
	after func(ctx context.Context, resp *Response) error,
) Middleware {
	return &MiddlewareFunc{
		beforeFunc: before,
		afterFunc:  after,
	}
}

// loggingMiddleware logs requests and responses.
type loggingMiddleware struct {
	prefix string
	logger interface{ Printf(string, ...interface{}) }
}

// NewLoggingMiddleware creates a middleware that logs requests and responses.
func NewLoggingMiddleware(prefix string) Middleware {
	return &loggingMiddleware{
		prefix: prefix,
		logger: log.Default(),
	}
}

// Before implements Middleware.Before.
func (m *loggingMiddleware) Before(ctx context.Context, message string) (context.Context, string, error) {
	m.logger.Printf("[%s] Request: %s", m.prefix, message)
	return ctx, message, nil
}

// After implements Middleware.After.
func (m *loggingMiddleware) After(ctx context.Context, resp *Response) error {
	m.logger.Printf("[%s] Response: %d chars", m.prefix, len(resp.Content))
	return nil
}

// metricsMiddleware collects metrics on agent operations.
type metricsMiddleware struct {
	prefix string
	getMetrics func() map[string]interface{}
}

// NewMetricsMiddleware creates a middleware that collects metrics.
func NewMetricsMiddleware(prefix string) Middleware {
	return &metricsMiddleware{
		prefix: prefix,
		getMetrics: func() map[string]interface{} {
			return make(map[string]interface{})
		},
	}
}

// Before implements Middleware.Before.
func (m *metricsMiddleware) Before(ctx context.Context, message string) (context.Context, string, error) {
	// Store start time in context
	ctx = context.WithValue(ctx, metricsKey("start"), time.Now())
	return ctx, message, nil
}

// After implements Middleware.After.
func (m *metricsMiddleware) After(ctx context.Context, resp *Response) error {
	start, ok := ctx.Value(metricsKey("start")).(time.Time)
	if ok {
		duration := time.Since(start)
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]interface{})
		}
		resp.Metadata[m.prefix+"_duration_ms"] = duration.Milliseconds()
		resp.Metadata[m.prefix+"_request_size"] = len(resp.Content)
	}
	return nil
}

type metricsKey string

// Chain creates a middleware chain from multiple middleware.
// The Before methods are called in order, After methods in reverse order.
func Chain(middlewares ...Middleware) Middleware {
	return &chainMiddleware{middlewares: middlewares}
}

type chainMiddleware struct {
	middlewares []Middleware
}

// Before implements Middleware.Before.
func (c *chainMiddleware) Before(ctx context.Context, message string) (context.Context, string, error) {
	for _, mw := range c.middlewares {
		var err error
		ctx, message, err = mw.Before(ctx, message)
		if err != nil {
			return ctx, message, err
		}
	}
	return ctx, message, nil
}

// After implements Middleware.After.
func (c *chainMiddleware) After(ctx context.Context, resp *Response) error {
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		if err := c.middlewares[i].After(ctx, resp); err != nil {
			return err
		}
	}
	return nil
}
