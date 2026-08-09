// Package telemetry 提供 Agent framework 的遥测装饰器。
//
// 该装饰器实现 framework.Agent 接口并包装另一个 Agent，是典型的装饰器模式，
// 概念上属于 Agent framework(service 层)而非基础设施层。原先置于
// infrastructure/observability 会造成 infra→service 反转，故迁至此处；
// 它依赖 infrastructure/observability 提供的 Tracer/Metrics(可观测性，横切关注点)。
package telemetry

import (
	"context"
	"time"

	obs "cognida/internal/infrastructure/observability"
	"cognida/internal/service/agent/framework"
)

// TelemetryMiddleware wraps an Agent with telemetry capabilities.
type TelemetryMiddleware struct {
	agent   framework.Agent
	tracer  *obs.Tracer
	metrics *obs.Metrics
}

// NewTelemetryMiddleware creates a new telemetry middleware.
func NewTelemetryMiddleware(a framework.Agent) *TelemetryMiddleware {
	return &TelemetryMiddleware{
		agent:   a,
		tracer:  obs.NewTracer(),
		metrics: obs.GetMetrics(),
	}
}

// Chat executes the agent with telemetry.
func (t *TelemetryMiddleware) Chat(ctx context.Context, message string) (*framework.Response, error) {
	startTime := time.Now()
	agentName := t.agent.Name()

	// Start tracing span
	ctx, span := t.tracer.StartSpan(ctx, agentName, message)
	defer span.End()

	// Increment active requests
	if t.metrics != nil {
		t.metrics.IncrementActiveRequests(ctx, agentName)
		defer t.metrics.DecrementActiveRequests(ctx, agentName)
	}

	// Execute the agent
	resp, err := t.agent.Chat(ctx, message)
	duration := time.Since(startTime)

	// Record metrics
	if t.metrics != nil {
		t.metrics.RecordLatency(ctx, agentName, duration)
		if err != nil {
			t.metrics.RecordError(ctx, agentName, "chat_error")
		} else {
			t.metrics.RecordRequest(ctx, agentName, true)

			// Record tool calls
			for _, tc := range resp.ToolCalls {
				success := tc.Error == nil
				t.metrics.RecordToolCall(ctx, agentName, tc.Name, success)
				if tc.Error != nil {
					span.SetAttribute(obs.AttrToolError, tc.Error.Error())
				}
			}

			// Record metadata
			if iter, ok := resp.Metadata["iterations"].(int); ok {
				span.SetAttribute(obs.AttrIterationCount, iter)
			}
		}
	}

	// Record error on span
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Add response attributes
	span.SetAttribute("response.content_length", len(resp.Content))
	span.SetAttribute("response.tool_calls", len(resp.ToolCalls))

	return resp, nil
}

// Stream executes the agent with streaming telemetry.
func (t *TelemetryMiddleware) Stream(ctx context.Context, message string) (<-chan *framework.Chunk, error) {
	startTime := time.Now()
	agentName := t.agent.Name()

	// Start tracing span
	ctx, span := t.tracer.StartSpan(ctx, agentName, message)

	// Increment active requests
	if t.metrics != nil {
		t.metrics.IncrementActiveRequests(ctx, agentName)
	}

	// Execute the agent
	chunkChan, err := t.agent.Stream(ctx, message)
	if err != nil {
		span.RecordError(err)
		span.End()
		if t.metrics != nil {
			t.metrics.DecrementActiveRequests(ctx, agentName)
			t.metrics.RecordError(ctx, agentName, "stream_error")
		}
		return nil, err
	}

	// Wrap the channel with telemetry
	resultChan := make(chan *framework.Chunk, 1)
	go func() {
		defer close(resultChan)
		defer span.End()
		if t.metrics != nil {
			defer t.metrics.DecrementActiveRequests(ctx, agentName)
		}

		chunkCount := 0
		for chunk := range chunkChan {
			chunkCount++
			if t.metrics != nil {
				t.metrics.RecordStreamChunk(ctx, agentName)
			}
			resultChan <- chunk
		}

		span.SetAttribute("stream.chunk_count", chunkCount)
		duration := time.Since(startTime)
		if t.metrics != nil {
			t.metrics.RecordLatency(ctx, agentName, duration)
		}
	}()

	return (<-chan *framework.Chunk)(resultChan), nil
}

// Name returns the agent name.
func (t *TelemetryMiddleware) Name() string {
	return t.agent.Name()
}

// WrapAgent wraps an agent with telemetry middleware.
func WrapAgent(a framework.Agent) framework.Agent {
	return NewTelemetryMiddleware(a)
}
