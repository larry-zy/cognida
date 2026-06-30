// Package orchestration provides composable patterns for combining multiple agents.
package orchestration

import (
	"context"
	"fmt"

	infraagent "link/internal/service/agent/framework"
)

// Sequential creates an agent that executes agents in sequence.
// The output of each agent is passed as input to the next.
func Sequential(agents ...infraagent.Agent) infraagent.Agent {
	if len(agents) == 0 {
		return &errorAgent{err: fmt.Errorf("Sequential: no agents provided")}
	}
	if len(agents) == 1 {
		return agents[0]
	}
	return &sequentialAgent{
		agents: agents,
	}
}

type sequentialAgent struct {
	agents []infraagent.Agent
}

func (s *sequentialAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	current := message
	var lastResponse *infraagent.Response
	var err error

	for i, a := range s.agents {
		lastResponse, err = a.Chat(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("Sequential: agent %d failed: %w", i, err)
		}

		// Pass content to next agent
		current = lastResponse.Content
	}

	return lastResponse, nil
}

func (s *sequentialAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 1)

	go func() {
		defer close(out)

		current := message
		for i, a := range s.agents {
			ch, err := a.Stream(ctx, current)
			if err != nil {
				out <- &infraagent.Chunk{
					Content: "",
					Done:    true,
					Metadata: map[string]interface{}{
						"error": fmt.Sprintf("agent %d failed: %v", i, err),
					},
				}
				return
			}

			// Stream chunks from this agent
			var buffer string
			for chunk := range ch {
				if chunk.Done {
					break
				}
				buffer += chunk.Content
				out <- chunk
			}

			current = buffer
		}

		out <- &infraagent.Chunk{Done: true}
	}()

	return out, nil
}

func (s *sequentialAgent) Name() string {
	names := make([]string, len(s.agents))
	for i, a := range s.agents {
		names[i] = a.Name()
	}
	return fmt.Sprintf("Sequential[%v]", names)
}

// errorAgent is an agent that always returns an error.
type errorAgent struct {
	err error
}

func (e *errorAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	return nil, e.err
}

func (e *errorAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	ch := make(chan *infraagent.Chunk, 1)
	close(ch)
	return ch, e.err
}

func (e *errorAgent) Name() string {
	return "ErrorAgent"
}
