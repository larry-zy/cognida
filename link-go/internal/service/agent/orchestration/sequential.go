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

	// 逐段累积工具调用轨迹：Sequential 只把每段的 Content 透传给下一段，
	// 但最终返回的 lastResponse 仅是末段（如 Reflect）的响应，其自身通常无工具调用。
	// 若不累积，Plan/Execute 段真实发生的 get_schema/sql_execute 轨迹会被丢弃，
	// 导致下游（评测轨迹指标、orchestrator.Execute 的工具信息）读到空 ToolCalls。
	var trajectory []*infraagent.ToolCall

	for i, a := range s.agents {
		lastResponse, err = a.Chat(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("Sequential: agent %d failed: %w", i, err)
		}
		if lastResponse == nil {
			return nil, fmt.Errorf("Sequential: agent %d returned nil response", i)
		}

		trajectory = append(trajectory, lastResponse.ToolCalls...)

		// Pass content to next agent
		current = lastResponse.Content
	}

	// 用跨段合并后的完整轨迹覆盖末段响应的 ToolCalls，保序（Plan→Execute→Reflect）。
	if lastResponse != nil {
		lastResponse.ToolCalls = trajectory
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
