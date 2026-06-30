package orchestration

import (
	"context"
	"testing"

	"link/internal/service/agent/framework"

	"github.com/stretchr/testify/assert"
)

// TestLoop_Continue tests Loop that continues iterating.
func TestLoop_Continue(t *testing.T) {
	callCount := 0
	innerAgent := &mockAgentForOrchestration{
		name: "inner",
		response: "response",
	}

	loopAgent := Loop(
		innerAgent,
		func(resp *framework.Response) bool {
			callCount++
			// Continue for 2 iterations
			return callCount < 2
		},
	)

	ctx := context.Background()
	resp, err := loopAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Contains(t, resp.Content, "response")
}

// TestLoop_StopImmediately tests Loop that stops immediately.
func TestLoop_StopImmediately(t *testing.T) {
	callCount := 0
	innerAgent := &mockAgentForOrchestration{
		name: "inner",
		response: "response",
	}

	loopAgent := Loop(
		innerAgent,
		func(resp *framework.Response) bool {
			callCount++
			return false // Stop immediately
		},
	)

	ctx := context.Background()
	resp, err := loopAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.Contains(t, resp.Content, "response")
}

// TestLoop_MaxIterations tests Loop reaching max iterations.
func TestLoop_MaxIterations(t *testing.T) {
	// This test would run to the default max of 100 iterations
	// Let's test with a custom max using WithMaxIterations
	callCount := 0
	innerAgent := &mockAgentForOrchestration{
		name: "inner",
		response: "response",
	}

	loopAgent := Loop(
		innerAgent,
		func(resp *framework.Response) bool {
			callCount++
			return true // Always continue
		},
	)

	ctx := context.Background()
	_, err := loopAgent.Chat(ctx, "test")

	// Should stop at max iterations (100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations")
}

// TestFunc tests Func wrapper for simple functions.
func TestFunc(t *testing.T) {
	funcAgent := Func(func(ctx context.Context, message string) (*framework.Response, error) {
		return &framework.Response{
			Content: "func response: " + message,
		}, nil
	})

	ctx := context.Background()
	resp, err := funcAgent.Chat(ctx, "hello")

	assert.NoError(t, err)
	assert.Equal(t, "func response: hello", resp.Content)
	assert.Equal(t, "FuncAgent", funcAgent.Name())
}

// TestFunc_Stream tests Func with streaming fallback.
func TestFunc_Stream(t *testing.T) {
	funcAgent := Func(func(ctx context.Context, message string) (*framework.Response, error) {
		return &framework.Response{Content: "response"}, nil
	})

	ctx := context.Background()
	ch, err := funcAgent.Stream(ctx, "test")

	assert.NoError(t, err)
	assert.NotNil(t, ch)

	// Should receive the response from Chat via streaming
	chunk := <-ch
	assert.Equal(t, "response", chunk.Content)
}

// TestRepeat tests Repeat executing an agent multiple times.
func TestRepeat(t *testing.T) {
	innerAgent := &mockAgentForOrchestration{
		name:     "inner",
		response: "response",
	}

	repeatAgent := Repeat(innerAgent, 3)

	ctx := context.Background()
	resp, err := repeatAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Contains(t, resp.Content, "response")
	// Should have 3 attempts in the aggregated response
	assert.Contains(t, resp.Content, "Attempt 1")
	assert.Contains(t, resp.Content, "Attempt 2")
	assert.Contains(t, resp.Content, "Attempt 3")
}

// TestRetry_SuccessOnFirstTry tests Retry with immediate success.
func TestRetry_SuccessOnFirstTry(t *testing.T) {
	callCount := 0

	trackingAgent := Func(func(ctx context.Context, message string) (*framework.Response, error) {
		callCount++
		return &framework.Response{Content: "response"}, nil
	})

	retryAgent := Retry(trackingAgent, 3, func(err error) bool {
		return true
	})

	ctx := context.Background()
	resp, err := retryAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.Equal(t, "response", resp.Content)
}

// TestRetry_NeedsRetry tests Retry with failure then success.
func TestRetry_NeedsRetry(t *testing.T) {
	callCount := 0
	attemptsBeforeSuccess := 2

	innerAgent := Func(func(ctx context.Context, message string) (*framework.Response, error) {
		callCount++
		if callCount < attemptsBeforeSuccess {
			return nil, assert.AnError
		}
		return &framework.Response{Content: "success"}, nil
	})

	retryAgent := Retry(innerAgent, 3, func(err error) bool {
		return true // Always retry
	})

	ctx := context.Background()
	resp, err := retryAgent.Chat(ctx, "test")

	assert.NoError(t, err)
	assert.Equal(t, attemptsBeforeSuccess, callCount)
	assert.Equal(t, "success", resp.Content)
}
