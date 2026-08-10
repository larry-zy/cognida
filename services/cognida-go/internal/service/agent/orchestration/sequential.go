// Package orchestration provides composable patterns for combining multiple agents.
package orchestration

import (
	"context"
	"fmt"

	"cognida/internal/pkg/safego"
	infraagent "cognida/internal/service/agent/framework"
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

	// 同理累积各段的运行时用量：末段 Metadata 只反映该段（如 Reflect）的 tokens_used/
	// iterations，会大幅低估整条管道的真实 LLM 调用次数与 token 消耗。跨段求和后写回，
	// 使评测运行时指标（llm_calls/tokens_used）反映 Plan+Execute+Reflect 全流程。
	var totalTokens, totalIterations int

	for i, a := range s.agents {
		lastResponse, err = a.Chat(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("Sequential: agent %d failed: %w", i, err)
		}
		if lastResponse == nil {
			return nil, fmt.Errorf("Sequential: agent %d returned nil response", i)
		}

		trajectory = append(trajectory, lastResponse.ToolCalls...)
		totalTokens += metaInt(lastResponse.Metadata, "tokens_used")
		totalIterations += metaInt(lastResponse.Metadata, "iterations")

		// Pass content to next agent
		current = lastResponse.Content
	}

	// 用跨段合并后的完整轨迹覆盖末段响应的 ToolCalls，保序（Plan→Execute→Reflect）。
	// 并把跨段求和后的运行时用量写回末段 Metadata（保留末段 terminated_by/partial 等其它键）。
	if lastResponse != nil {
		lastResponse.ToolCalls = trajectory
		if lastResponse.Metadata == nil {
			lastResponse.Metadata = make(map[string]interface{})
		}
		lastResponse.Metadata["tokens_used"] = totalTokens
		lastResponse.Metadata["iterations"] = totalIterations
	}

	return lastResponse, nil
}

// metaInt 从段响应 Metadata 中安全读取整型用量指标（缺失或类型不符按 0 计）。
// 框架 fillMetadata 以 int 写入 tokens_used/iterations，此处兼容常见数值类型。
func metaInt(meta map[string]interface{}, key string) int {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func (s *sequentialAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	out := make(chan *infraagent.Chunk, 1)

	go func() {
		defer safego.Recover("sequential-stream")
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
