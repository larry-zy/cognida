package orchestration

import (
	"context"
	"fmt"

	infraagent "cognida/internal/service/agent/framework"
)

// Loop creates an agent that executes a body agent repeatedly.
// The loop continues until the continuation function returns false.
func Loop(bodyAgent infraagent.Agent, continuation func(*infraagent.Response) bool) infraagent.Agent {
	return &loopAgent{
		body:         bodyAgent,
		continuation: continuation,
		maxIter:      100, // Default max iterations
	}
}

type loopAgent struct {
	body         infraagent.Agent
	continuation func(*infraagent.Response) bool
	maxIter      int
}

func (l *loopAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	current := message
	var allResponses []*infraagent.Response
	var lastResponse *infraagent.Response
	var err error

	for i := 0; i < l.maxIter; i++ {
		lastResponse, err = l.body.Chat(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("Loop: iteration %d failed: %w", i, err)
		}

		allResponses = append(allResponses, lastResponse)

		// Check if we should continue
		if !l.continuation(lastResponse) {
			break
		}

		// Use this response as input for next iteration
		current = lastResponse.Content
	}

	if len(allResponses) >= l.maxIter {
		return l.aggregateResponses(allResponses), fmt.Errorf("Loop: exceeded max iterations (%d)", l.maxIter)
	}

	return l.aggregateResponses(allResponses), nil
}

func (l *loopAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 1)

	go func() {
		defer close(out)

		current := message
		for i := 0; i < l.maxIter; i++ {
			ch, err := l.body.Stream(ctx, current)
			if err != nil {
				out <- &infraagent.Chunk{
					Content: "",
					Done:    true,
					Metadata: map[string]interface{}{
						"error": fmt.Sprintf("iteration %d failed: %v", i, err),
					},
				}
				return
			}

			var buffer string
			var done bool
			for chunk := range ch {
				out <- chunk
				if chunk.Done {
					done = true
					break
				}
				buffer += chunk.Content
			}

			// Check continuation
			resp := &infraagent.Response{Content: buffer}
			if !l.continuation(resp) || done {
				return
			}

			current = buffer
		}

		out <- &infraagent.Chunk{
			Content: "",
			Done:    true,
			Metadata: map[string]interface{}{
				"warning": "max iterations reached",
			},
		}
	}()

	return out, nil
}

func (l *loopAgent) Name() string {
	return fmt.Sprintf("Loop(body: %s, max: %d)", l.body.Name(), l.maxIter)
}

// aggregateResponses combines multiple responses into one.
func (l *loopAgent) aggregateResponses(responses []*infraagent.Response) *infraagent.Response {
	if len(responses) == 0 {
		return &infraagent.Response{Content: ""}
	}
	if len(responses) == 1 {
		return responses[0]
	}

	content := ""
	for i, resp := range responses {
		content += fmt.Sprintf("--- Iteration %d ---\n%s\n\n", i+1, resp.Content)
	}

	return &infraagent.Response{
		Content: content,
		Metadata: map[string]interface{}{
			"iterations": len(responses),
		},
	}
}

// WithMaxIterations sets the maximum number of iterations for a loop infraagent.
func WithMaxIterations(max int) func(*loopAgent) {
	return func(l *loopAgent) {
		l.maxIter = max
	}
}

// Repeat executes an agent a fixed number of times.
func Repeat(agent infraagent.Agent, count int) infraagent.Agent {
	return &repeatAgent{
		agent: agent,
		count: count,
	}
}

type repeatAgent struct {
	agent infraagent.Agent
	count int
}

func (r *repeatAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	var allResponses []*infraagent.Response

	for i := 0; i < r.count; i++ {
		resp, err := r.agent.Chat(ctx, message)
		if err != nil {
			return nil, fmt.Errorf("Repeat: iteration %d failed: %w", i, err)
		}
		allResponses = append(allResponses, resp)
	}

	return r.aggregateResponses(allResponses), nil
}

func (r *repeatAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 10)

	go func() {
		defer close(out)

		for i := 0; i < r.count; i++ {
			ch, err := r.agent.Stream(ctx, message)
			if err != nil {
				out <- &infraagent.Chunk{
					Content: "",
					Done:    true,
					Metadata: map[string]interface{}{
						"error": fmt.Sprintf("iteration %d failed: %v", i, err),
					},
				}
				return
			}

			for chunk := range ch {
				out <- chunk
			}
		}

		out <- &infraagent.Chunk{Done: true}
	}()

	return out, nil
}

func (r *repeatAgent) Name() string {
	return fmt.Sprintf("Repeat(%s, %d)", r.agent.Name(), r.count)
}

func (r *repeatAgent) aggregateResponses(responses []*infraagent.Response) *infraagent.Response {
	if len(responses) == 0 {
		return &infraagent.Response{Content: ""}
	}
	if len(responses) == 1 {
		return responses[0]
	}

	content := ""
	for i, resp := range responses {
		content += fmt.Sprintf("--- Attempt %d ---\n%s\n\n", i+1, resp.Content)
	}

	return &infraagent.Response{
		Content: content,
		Metadata: map[string]interface{}{
			"attempts": len(responses),
		},
	}
}

// Retry executes an agent until it succeeds or max retries is reached.
func Retry(agent infraagent.Agent, maxRetries int, shouldRetry func(error) bool) infraagent.Agent {
	return &retryAgent{
		agent:      agent,
		maxRetries: maxRetries,
		shouldRetry: shouldRetry,
	}
}

type retryAgent struct {
	agent      infraagent.Agent
	maxRetries int
	shouldRetry func(error) bool
}

func (r *retryAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	var lastErr error

	for i := 0; i <= r.maxRetries; i++ {
		resp, err := r.agent.Chat(ctx, message)
		if err == nil {
			if i > 0 {
				if resp.Metadata == nil {
					resp.Metadata = make(map[string]interface{})
				}
				resp.Metadata["retries"] = i
			}
			return resp, nil
		}

		lastErr = err
		if r.shouldRetry == nil || !r.shouldRetry(err) {
			break
		}
	}

	return nil, fmt.Errorf("Retry: failed after %d attempts: %w", r.maxRetries+1, lastErr)
}

func (r *retryAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 1)

	go func() {
		defer close(out)

		for i := 0; i <= r.maxRetries; i++ {
			ch, err := r.agent.Stream(ctx, message)
			if err == nil {
				for chunk := range ch {
					out <- chunk
				}
				return
			}

			if r.shouldRetry == nil || !r.shouldRetry(err) {
				out <- &infraagent.Chunk{
					Content: "",
					Done:    true,
					Metadata: map[string]interface{}{
						"error": fmt.Sprintf("failed after %d attempts: %v", i+1, err),
					},
				}
				return
			}
		}
	}()

	return out, nil
}

func (r *retryAgent) Name() string {
	return fmt.Sprintf("Retry(%s, max: %d)", r.agent.Name(), r.maxRetries)
}
