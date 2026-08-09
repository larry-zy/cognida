// Package telemetry —— 调用链追踪的运行期装配。
//
// 本文件把此前休眠的 OTel span 埋点真正点亮：注册一个 SDK TracerProvider
// 作为全局 provider（否则默认 no-op provider 会丢弃所有 span），并挂上
//   - contextTagProcessor：span 创建时从 Go context 盖上 request_id/session/租户/用户，
//     使 span 能与 audit_logs / 会话关联；
//   - mysqlSpanExporter：span 结束后经 BatchSpanProcessor 批量落到 trace_spans 表。
package telemetry

import (
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	agentctx "cognida/internal/model/agent"
	domaintrace "cognida/internal/model/trace"
)

// 由 contextTagProcessor 盖到 span 上、并被 exporter 读回的属性键。
const (
	attrRequestID = "request_id"
	attrSessionID = "session_id"
	attrTenantID  = "tenant_id"
	attrUserID    = "user_id"
	attrAgentName = "agent.name" // 与 observability/agent_tracing.go 的 AttrAgentName 对齐
)

// InitTracing 装配并注册全局 TracerProvider，返回优雅关闭函数（务必在退出前调用以 flush 批量队列）。
// repo 为空则直接返回 no-op 关闭函数，不改动全局 provider。
func InitTracing(repo domaintrace.Repository) (func(context.Context) error, error) {
	if repo == nil {
		return func(context.Context) error { return nil }, nil
	}

	exporter := &mysqlSpanExporter{repo: repo}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(&contextTagProcessor{}),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(3*time.Second),
			sdktrace.WithMaxExportBatchSize(200),
		),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", "cognida-go"),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// ========================================
// contextTagProcessor：创建 span 时从 ctx 盖标签
// ========================================

type contextTagProcessor struct{}

func (p *contextTagProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	if rid := agentctx.MustGetRequestID(parent); rid != "" {
		s.SetAttributes(attribute.String(attrRequestID, rid))
	}
	if sid := agentctx.MustGetSessionID(parent); sid != "" {
		s.SetAttributes(attribute.String(attrSessionID, sid))
	}
	if tid := agentctx.MustGetTenantID(parent); tid > 0 {
		s.SetAttributes(attribute.Int64(attrTenantID, tid))
	}
	if uid := agentctx.MustGetUserID(parent); uid > 0 {
		s.SetAttributes(attribute.Int64(attrUserID, uid))
	}
}

func (p *contextTagProcessor) OnEnd(sdktrace.ReadOnlySpan)            {}
func (p *contextTagProcessor) Shutdown(context.Context) error        { return nil }
func (p *contextTagProcessor) ForceFlush(context.Context) error      { return nil }

// ========================================
// mysqlSpanExporter：span → trace_spans 表
// ========================================

type mysqlSpanExporter struct {
	repo domaintrace.Repository
}

func (e *mysqlSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}
	out := make([]*domaintrace.Span, 0, len(spans))
	for _, s := range spans {
		out = append(out, convertSpan(s))
	}
	return e.repo.SaveBatch(ctx, out)
}

func (e *mysqlSpanExporter) Shutdown(context.Context) error { return nil }

// convertSpan 把 OTel ReadOnlySpan 折叠成领域 Span：从 attributes 回读盖上的
// request_id/session/租户/用户/agent 名，其余属性整体序列化进 attributes JSON。
func convertSpan(s sdktrace.ReadOnlySpan) *domaintrace.Span {
	sc := s.SpanContext()
	parentID := ""
	if p := s.Parent(); p.HasSpanID() {
		parentID = p.SpanID().String()
	}

	span := &domaintrace.Span{
		TraceID:      sc.TraceID().String(),
		SpanID:       sc.SpanID().String(),
		ParentSpanID: parentID,
		Name:         s.Name(),
		Kind:         s.SpanKind().String(),
		StartTime:    s.StartTime(),
		EndTime:      s.EndTime(),
		DurationMs:   s.EndTime().Sub(s.StartTime()).Milliseconds(),
		StatusCode:   statusString(s.Status().Code),
		StatusMsg:    s.Status().Description,
	}

	attrs := make(map[string]interface{}, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		key := string(kv.Key)
		val := kv.Value.AsInterface()
		attrs[key] = val
		switch key {
		case attrRequestID:
			span.RequestID = kv.Value.AsString()
		case attrSessionID:
			span.SessionID = kv.Value.AsString()
		case attrAgentName:
			span.AgentName = kv.Value.AsString()
		case attrTenantID:
			if v := kv.Value.AsInt64(); v > 0 {
				span.TenantID = &v
			}
		case attrUserID:
			if v := kv.Value.AsInt64(); v > 0 {
				span.UserID = &v
			}
		}
	}
	span.Attributes = mustJSON(attrs, "{}")
	span.Events = mustJSON(convertEvents(s.Events()), "[]")
	return span
}

type spanEvent struct {
	Name       string                 `json:"name"`
	Time       time.Time              `json:"time"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

func convertEvents(events []sdktrace.Event) []spanEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]spanEvent, 0, len(events))
	for _, ev := range events {
		attrs := make(map[string]interface{}, len(ev.Attributes))
		for _, kv := range ev.Attributes {
			attrs[string(kv.Key)] = kv.Value.AsInterface()
		}
		out = append(out, spanEvent{Name: ev.Name, Time: ev.Time, Attributes: attrs})
	}
	return out
}

func statusString(c otelcodes.Code) string {
	switch c {
	case otelcodes.Ok:
		return domaintrace.StatusOK
	case otelcodes.Error:
		return domaintrace.StatusError
	default:
		return domaintrace.StatusUnset
	}
}

func mustJSON(v interface{}, fallback string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(b)
}
