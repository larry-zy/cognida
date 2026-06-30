// Package telemetry provides OpenTelemetry integration for Agent framework.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// InstrumentationName is the name of the telemetry instrumentation.
	InstrumentationName = "link/internal/service/agent"

	// Span names
	SpanAgentChat       = "agent.chat"
	SpanAgentStream     = "agent.stream"
	SpanToolInvocation  = "agent.tool.invoke"
	SpanMiddlewareExec  = "agent.middleware.exec"
	SpanOrchestration   = "agent.orchestration"

	// Attribute keys
	AttrAgentName      = "agent.name"
	AttrToolName       = "tool.name"
	AttrToolInput      = "tool.input"
	AttrToolOutput     = "tool.output"
	AttrToolError      = "tool.error"
	AttrMessageLength  = "message.length"
	AttrIterationCount = "iteration.count"
	AttrStreamChunk    = "stream.chunk"
	AttrOrchestrationType = "orchestration.type"
	AttrMiddlewareName = "middleware.name"
)

// Tracer provides tracing capabilities for Agent operations.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer creates a new Tracer.
func NewTracer() *Tracer {
	return &Tracer{
		tracer: otel.Tracer(InstrumentationName),
	}
}

// StartSpan starts a new span for Agent chat operation.
func (t *Tracer) StartSpan(ctx context.Context, agentName, message string) (context.Context, *Span) {
	ctx, span := t.tracer.Start(ctx, SpanAgentChat,
		trace.WithAttributes(
			attribute.String(AttrAgentName, agentName),
			attribute.Int(AttrMessageLength, len(message)),
		),
	)
	return ctx, &Span{span: span}
}

// StartToolSpan starts a new span for tool invocation.
func (t *Tracer) StartToolSpan(ctx context.Context, agentName, toolName, input string) (context.Context, *Span) {
	ctx, span := t.tracer.Start(ctx, SpanToolInvocation,
		trace.WithAttributes(
			attribute.String(AttrAgentName, agentName),
			attribute.String(AttrToolName, toolName),
			attribute.String(AttrToolInput, truncateString(input, 500)),
		),
	)
	return ctx, &Span{span: span}
}

// StartMiddlewareSpan starts a new span for middleware execution.
func (t *Tracer) StartMiddlewareSpan(ctx context.Context, agentName, middlewareName string) (context.Context, *Span) {
	ctx, span := t.tracer.Start(ctx, SpanMiddlewareExec,
		trace.WithAttributes(
			attribute.String(AttrAgentName, agentName),
			attribute.String(AttrMiddlewareName, middlewareName),
		),
	)
	return ctx, &Span{span: span}
}

// StartOrchestrationSpan starts a new span for orchestration patterns.
func (t *Tracer) StartOrchestrationSpan(ctx context.Context, orchestrationType string) (context.Context, *Span) {
	ctx, span := t.tracer.Start(ctx, SpanOrchestration,
		trace.WithAttributes(
			attribute.String(AttrOrchestrationType, orchestrationType),
		),
	)
	return ctx, &Span{span: span}
}

// Span wraps an OpenTelemetry span with convenience methods.
type Span struct {
	span trace.Span
}

// SetAttributes sets attributes on the span.
func (s *Span) SetAttributes(attrs ...attribute.KeyValue) {
	s.span.SetAttributes(attrs...)
}

// SetAttribute sets a single attribute.
func (s *Span) SetAttribute(key string, value interface{}) {
	switch v := value.(type) {
	case string:
		s.span.SetAttributes(attribute.String(key, v))
	case int:
		s.span.SetAttributes(attribute.Int(key, v))
	case int64:
		s.span.SetAttributes(attribute.Int64(key, v))
	case float64:
		s.span.SetAttributes(attribute.Float64(key, v))
	case bool:
		s.span.SetAttributes(attribute.Bool(key, v))
	default:
		s.span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", v)))
	}
}

// AddEvent adds an event to the span.
func (s *Span) AddEvent(name string, attrs ...attribute.KeyValue) {
	s.span.AddEvent(name, trace.WithAttributes(attrs...))
}

// RecordError records an error on the span.
func (s *Span) RecordError(err error) {
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
}

// End ends the span.
func (s *Span) End() {
	s.span.End()
}

// EndWithTime ends the span with a specific time.
func (s *Span) EndWithTime(t time.Time) {
	s.span.End(trace.WithTimestamp(t))
}

// Context returns the span's context.
func (s *Span) Context() trace.SpanContext {
	return s.span.SpanContext()
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
