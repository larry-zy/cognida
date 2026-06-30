package orchestration

import (
	"context"
	"fmt"
	"sync"

	infraagent "link/internal/service/agent/framework"
)

// Parallel creates an agent that executes multiple agents concurrently.
// All agents receive the same message and responses are aggregated.
func Parallel(agents ...infraagent.Agent) infraagent.Agent {
	if len(agents) == 0 {
		return &errorAgent{err: fmt.Errorf("Parallel: no agents provided")}
	}
	if len(agents) == 1 {
		return agents[0]
	}
	return &parallelAgent{
		agents: agents,
	}
}

type parallelAgent struct {
	agents []infraagent.Agent
}

func (p *parallelAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	type result struct {
		index   int
		response *infraagent.Response
		err     error
	}

	results := make(chan result, len(p.agents))
	var wg sync.WaitGroup

	// Execute all agents concurrently
	for i, a := range p.agents {
		wg.Add(1)
		go func(index int, a infraagent.Agent) {
			defer wg.Done()
			resp, err := a.Chat(ctx, message)
			results <- result{index: index, response: resp, err: err}
		}(i, a)
	}

	// Wait for all to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	responses := make([]*infraagent.Response, len(p.agents))
	errors := make([]error, 0)

	for r := range results {
		if r.err != nil {
			errors = append(errors, fmt.Errorf("agent %d: %w", r.index, r.err))
		} else {
			responses[r.index] = r.response
		}
	}

	// Build aggregated response
	aggregated := &infraagent.Response{
		Content:  p.aggregateContent(responses),
		Metadata: map[string]interface{}{
			"responses": len(responses),
			"errors":    len(errors),
		},
	}

	// Add tool calls from all responses
	for _, resp := range responses {
		if resp != nil {
			aggregated.ToolCalls = append(aggregated.ToolCalls, resp.ToolCalls...)
		}
	}

	// If there were errors, include them in metadata
	if len(errors) > 0 {
		errMsgs := make([]string, len(errors))
		for i, e := range errors {
			errMsgs[i] = e.Error()
		}
		aggregated.Metadata["errors_detail"] = errMsgs
	}

	return aggregated, nil
}

func (p *parallelAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 10)
	var wg sync.WaitGroup

	// Start all agents streaming
	for i, a := range p.agents {
		wg.Add(1)
		go func(index int, a infraagent.Agent) {
			defer wg.Done()

			ch, err := a.Stream(ctx, message)
			if err != nil {
				out <- &infraagent.Chunk{
					Content: "",
					Done:    false,
					Metadata: map[string]interface{}{
						"agent": index,
						"error": err.Error(),
					},
				}
				return
			}

			for chunk := range ch {
				chunk.Metadata = map[string]interface{}{
					"agent": index,
				}
				if chunk.Metadata == nil {
					chunk.Metadata = make(map[string]interface{})
				}
				chunk.Metadata["agent"] = index
				out <- chunk
			}
		}(i, a)
	}

	// Close channel when all agents complete
	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

func (p *parallelAgent) Name() string {
	names := make([]string, len(p.agents))
	for i, a := range p.agents {
		names[i] = a.Name()
	}
	return fmt.Sprintf("Parallel[%v]", names)
}

// aggregateContent combines content from multiple responses.
func (p *parallelAgent) aggregateContent(responses []*infraagent.Response) string {
	var result string
	for i, resp := range responses {
		if resp != nil && resp.Content != "" {
			result += fmt.Sprintf("[Agent %d]: %s\n\n", i, resp.Content)
		}
	}
	return result
}
