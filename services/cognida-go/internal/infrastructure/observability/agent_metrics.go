// Package telemetry provides Prometheus metrics integration for Agent framework.
package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics provides Prometheus metrics for Agent operations.
type Metrics struct {
	meter metric.Meter

	// Counters
	requestCounter        metric.Int64Counter
	toolCallCounter       metric.Int64Counter
	errorCounter          metric.Int64Counter
	streamChunkCounter    metric.Int64Counter

	// Histograms
	latencyHistogram      metric.Float64Histogram
	toolLatencyHistogram  metric.Float64Histogram
	tokenHistogram        metric.Float64Histogram

	// Gauges
	activeRequestsGauge   metric.Int64UpDownCounter
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(InstrumentationName)

	m := &Metrics{meter: meter}

	// Initialize counters
	requestCounter, err := meter.Int64Counter(
		"agent.requests.total",
		metric.WithDescription("Total number of agent requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	m.requestCounter = requestCounter

	toolCallCounter, err := meter.Int64Counter(
		"agent.tool_calls.total",
		metric.WithDescription("Total number of tool calls"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	m.toolCallCounter = toolCallCounter

	errorCounter, err := meter.Int64Counter(
		"agent.errors.total",
		metric.WithDescription("Total number of errors"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	m.errorCounter = errorCounter

	streamChunkCounter, err := meter.Int64Counter(
		"agent.stream.chunks.total",
		metric.WithDescription("Total number of stream chunks"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	m.streamChunkCounter = streamChunkCounter

	// Initialize histograms
	latencyHistogram, err := meter.Float64Histogram(
		"agent.latency",
		metric.WithDescription("Agent request latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	m.latencyHistogram = latencyHistogram

	toolLatencyHistogram, err := meter.Float64Histogram(
		"agent.tool.latency",
		metric.WithDescription("Tool execution latency in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	m.toolLatencyHistogram = toolLatencyHistogram

	tokenHistogram, err := meter.Float64Histogram(
		"agent.tokens.total",
		metric.WithDescription("Total tokens processed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	m.tokenHistogram = tokenHistogram

	// Initialize gauges
	activeRequestsGauge, err := meter.Int64UpDownCounter(
		"agent.requests.active",
		metric.WithDescription("Number of active agent requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	m.activeRequestsGauge = activeRequestsGauge

	return m, nil
}

// RecordRequest records an agent request.
func (m *Metrics) RecordRequest(ctx context.Context, agentName string, success bool) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
		attribute.String("success", formatBool(success)),
	)
	m.requestCounter.Add(ctx, 1, attrs)
}

// RecordToolCall records a tool invocation.
func (m *Metrics) RecordToolCall(ctx context.Context, agentName, toolName string, success bool) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
		attribute.String(AttrToolName, toolName),
		attribute.String("success", formatBool(success)),
	)
	m.toolCallCounter.Add(ctx, 1, attrs)
}

// RecordError records an error.
func (m *Metrics) RecordError(ctx context.Context, agentName, errorType string) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
		attribute.String("error_type", errorType),
	)
	m.errorCounter.Add(ctx, 1, attrs)
}

// RecordLatency records request latency.
func (m *Metrics) RecordLatency(ctx context.Context, agentName string, duration time.Duration) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
	)
	m.latencyHistogram.Record(ctx, duration.Seconds(), attrs)
}

// RecordToolLatency records tool execution latency.
func (m *Metrics) RecordToolLatency(ctx context.Context, toolName string, duration time.Duration) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrToolName, toolName),
	)
	m.toolLatencyHistogram.Record(ctx, duration.Seconds(), attrs)
}

// RecordTokens records token usage.
func (m *Metrics) RecordTokens(ctx context.Context, agentName string, inputTokens, outputTokens int) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
		attribute.String("token_type", "input"),
	)
	m.tokenHistogram.Record(ctx, float64(inputTokens), attrs)

	attrs = metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
		attribute.String("token_type", "output"),
	)
	m.tokenHistogram.Record(ctx, float64(outputTokens), attrs)
}

// RecordStreamChunk records a stream chunk.
func (m *Metrics) RecordStreamChunk(ctx context.Context, agentName string) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
	)
	m.streamChunkCounter.Add(ctx, 1, attrs)
}

// IncrementActiveRequests increments the active requests gauge.
func (m *Metrics) IncrementActiveRequests(ctx context.Context, agentName string) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
	)
	m.activeRequestsGauge.Add(ctx, 1, attrs)
}

// DecrementActiveRequests decrements the active requests gauge.
func (m *Metrics) DecrementActiveRequests(ctx context.Context, agentName string) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrAgentName, agentName),
	)
	m.activeRequestsGauge.Add(ctx, -1, attrs)
}

// formatBool converts a bool to "true" or "false" string.
func formatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
