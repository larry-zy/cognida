package framework

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

// mockChatModel is a mock implementation of BaseChatModel for testing.
type mockChatModel struct {
	generateFunc func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
}

func (m *mockChatModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, messages)
	}
	return &schema.Message{
		Role:    schema.Assistant,
		Content: "",
	}, nil
}

func (m *mockChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// Return empty reader for test using Pipe
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Close()
	return reader, nil
}

// TestNewLoggingMiddleware tests the logging middleware.
func TestNewLoggingMiddleware(t *testing.T) {
	model := &mockChatModel{
		generateFunc: func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
			return &schema.Message{
				Role:    schema.Assistant,
				Content: "Response",
			}, nil
		},
	}

	mw := NewLoggingMiddleware("test")
	assert.NotNil(t, mw)

	agent := &agentImpl{
		name:       "test-agent",
		model:      model,
		middleware: []Middleware{mw},
	}

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "Hello")

	assert.NoError(t, err)
	assert.Equal(t, "Response", resp.Content)
}

// TestNewMetricsMiddleware tests the metrics middleware.
func TestNewMetricsMiddleware(t *testing.T) {
	model := &mockChatModel{
		generateFunc: func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
			return &schema.Message{
				Role:    schema.Assistant,
				Content: "Response",
			}, nil
		},
	}

	mw := NewMetricsMiddleware("test")
	assert.NotNil(t, mw)

	agent := &agentImpl{
		name:       "test-agent",
		model:      model,
		middleware: []Middleware{mw},
	}

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "Hello")

	assert.NoError(t, err)
	assert.Equal(t, "Response", resp.Content)
	// Check that metrics were added
	assert.Contains(t, resp.Metadata, "test_duration_ms")
}

// TestChain tests chaining multiple middleware.
func TestChain(t *testing.T) {
	callOrder := make([]string, 0)

	first := &testMiddleware{
		name: "first",
		beforeFunc: func(ctx context.Context, message string) (context.Context, string, error) {
			callOrder = append(callOrder, "first-before")
			return ctx, message, nil
		},
		afterFunc: func(ctx context.Context, resp *Response) error {
			callOrder = append(callOrder, "first-after")
			return nil
		},
	}

	second := &testMiddleware{
		name: "second",
		beforeFunc: func(ctx context.Context, message string) (context.Context, string, error) {
			callOrder = append(callOrder, "second-before")
			return ctx, message, nil
		},
		afterFunc: func(ctx context.Context, resp *Response) error {
			callOrder = append(callOrder, "second-after")
			return nil
		},
	}

	model := &mockChatModel{
		generateFunc: func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
			callOrder = append(callOrder, "model")
			return &schema.Message{
				Role:    schema.Assistant,
				Content: "Response",
			}, nil
		},
	}

	chained := Chain(first, second)
	assert.NotNil(t, chained)

	agent := &agentImpl{
		name:       "test-agent",
		model:      model,
		middleware: []Middleware{chained},
	}

	ctx := context.Background()
	resp, err := agent.Chat(ctx, "Hello")

	assert.NoError(t, err)
	assert.Equal(t, "Response", resp.Content)

	// Verify call order: first-before, second-before, model, second-after, first-after
	expected := []string{"first-before", "second-before", "model", "second-after", "first-after"}
	assert.Equal(t, expected, callOrder)
}

// TestChain_Empty tests chaining with no middleware.
func TestChain_Empty(t *testing.T) {
	chained := Chain()
	assert.NotNil(t, chained)
	// Should still work even with empty chain
	ctx := context.Background()
	newCtx, msg, err := chained.Before(ctx, "test")
	assert.NoError(t, err)
	assert.Equal(t, "test", msg)
	assert.Equal(t, ctx, newCtx)

	resp := &Response{Content: "test"}
	assert.NoError(t, chained.After(ctx, resp))
}

// TestChain_Single tests chaining a single middleware.
func TestChain_Single(t *testing.T) {
	single := &testMiddleware{
		name: "single",
	}

	chained := Chain(single)
	assert.NotNil(t, chained)
	// Single middleware is wrapped in a chainMiddleware, but that should still work

	ctx := context.Background()
	_, msg, err := chained.Before(ctx, "test")
	assert.NoError(t, err)
	assert.Equal(t, "test", msg)
}

// testMiddleware is a configurable middleware for testing.
type testMiddleware struct {
	name       string
	beforeFunc func(ctx context.Context, message string) (context.Context, string, error)
	afterFunc  func(ctx context.Context, resp *Response) error
}

func (m *testMiddleware) Before(ctx context.Context, message string) (context.Context, string, error) {
	if m.beforeFunc != nil {
		return m.beforeFunc(ctx, message)
	}
	return ctx, message, nil
}

func (m *testMiddleware) After(ctx context.Context, resp *Response) error {
	if m.afterFunc != nil {
		return m.afterFunc(ctx, resp)
	}
	return nil
}
