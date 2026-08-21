package telemetry

import (
	"context"
	"sync"
	"testing"

	agentctx "cognida/internal/model/agent"
	domaintrace "cognida/internal/model/trace"
)

// fakeTraceRepo 捕获 exporter 落库的 span，供断言。
type fakeTraceRepo struct {
	mu    sync.Mutex
	spans []*domaintrace.Span
}

func (f *fakeTraceRepo) SaveBatch(_ context.Context, spans []*domaintrace.Span) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spans = append(f.spans, spans...)
	return nil
}
func (f *fakeTraceRepo) ListTraces(context.Context, *domaintrace.Query) ([]*domaintrace.TraceSummary, int64, error) {
	return nil, 0, nil
}
func (f *fakeTraceRepo) GetSpansByTraceID(context.Context, string) ([]*domaintrace.Span, error) {
	return nil, nil
}
func (f *fakeTraceRepo) HasRequestIDs(context.Context, int64, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (f *fakeTraceRepo) snapshot() []*domaintrace.Span {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domaintrace.Span, len(f.spans))
	copy(out, f.spans)
	return out
}

// TestInitTracing_EndToEnd 验证根因修复：注册真实 provider 后，span 不再被 no-op 丢弃，
// 而是经 contextTagProcessor 盖上 request_id/租户、由 exporter 落到 repo。
func TestInitTracing_EndToEnd(t *testing.T) {
	repo := &fakeTraceRepo{}
	shutdown, err := InitTracing(repo)
	if err != nil {
		t.Fatalf("InitTracing 失败: %v", err)
	}

	// 造一条带 request_id / 租户的上下文，跑一条 agent.chat span。
	ctx := context.Background()
	ctx = agentctx.WithRequestID(ctx, "req-abc")
	ctx = agentctx.WithTenantID(ctx, 7)

	tr := NewTracer()
	_, span := tr.StartSpan(ctx, "data_agent", "hello world")
	span.End()

	// Shutdown 会 flush 批量队列并同步导出。
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown 失败: %v", err)
	}

	spans := repo.snapshot()
	if len(spans) == 0 {
		t.Fatal("未捕获到任何 span —— provider 仍是 no-op，根因未修复")
	}

	var chat *domaintrace.Span
	for _, s := range spans {
		if s.Name == "agent.chat" {
			chat = s
			break
		}
	}
	if chat == nil {
		t.Fatalf("未找到 agent.chat span，实际: %+v", spans)
	}
	if chat.RequestID != "req-abc" {
		t.Errorf("request_id 未盖上: %q", chat.RequestID)
	}
	if chat.TenantID == nil || *chat.TenantID != 7 {
		t.Errorf("tenant_id 未盖上: %v", chat.TenantID)
	}
	if chat.AgentName != "data_agent" {
		t.Errorf("agent_name 缺失: %q", chat.AgentName)
	}
	if chat.TraceID == "" || chat.SpanID == "" {
		t.Errorf("trace_id/span_id 不应为空")
	}
	if chat.StatusCode == "" {
		t.Errorf("status_code 不应为空")
	}
}
