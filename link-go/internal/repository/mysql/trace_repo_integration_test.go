//go:build integration
// +build integration

// Package mysql: 调用链追踪仓储（trace_spans）真实 DB 集成测试。
//
// 针对真实 MySQL 运行，受 `integration` 构建标签门控（见 CLAUDE.md 集成测试）。
// 通过 MYSQL_DSN 提供 DSN，例如：
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestTraceRepo -v
//
// 覆盖：SaveBatch 幂等（span_id 冲突忽略）、GetSpansByTraceID 顺序、
// ListTraces 按 trace 聚合（根名/耗时/span 数/错误标记）、only_error HAVING 过滤、
// 租户隔离与分页。
package mysql

import (
	"context"
	"testing"
	"time"

	domaintrace "link/internal/model/trace"
)

func i64(v int64) *int64 { return &v }

func mkSpan(traceID, spanID, parent, name, status string, agent string, tenant int64, start time.Time, durMs int64) *domaintrace.Span {
	t := tenant
	return &domaintrace.Span{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parent,
		Name:         name,
		Kind:         "internal",
		StartTime:    start,
		EndTime:      start.Add(time.Duration(durMs) * time.Millisecond),
		DurationMs:   durMs,
		StatusCode:   status,
		RequestID:    "rid-" + traceID,
		SessionID:    "sid-" + traceID,
		TenantID:     &t,
		AgentName:    agent,
		Attributes:   "{}",
		Events:       "[]",
	}
}

func TestTraceRepoRoundTripAndAggregation(t *testing.T) {
	db := newIntegrationDB(t)
	if err := db.AutoMigrate(&TraceSpanModel{}); err != nil {
		t.Fatalf("automigrate trace_spans: %v", err)
	}
	repo := NewTraceRepository(db)
	ctx := context.Background()

	base := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	const tenant = int64(9911)
	const traceOK = "it_trace_ok_0001"
	const traceErr = "it_trace_err_0001"

	clean := func() {
		db.Where("trace_id IN ?", []string{traceOK, traceErr}).Delete(&TraceSpanModel{})
	}
	clean()
	t.Cleanup(clean)

	// traceOK：根 + 两个子 span，全 OK，根耗时 120ms。
	okSpans := []*domaintrace.Span{
		mkSpan(traceOK, "ok_root", "", "agent.chat", domaintrace.StatusOK, "data_agent", tenant, base, 120),
		mkSpan(traceOK, "ok_c1", "ok_root", "tool.sql", domaintrace.StatusOK, "data_agent", tenant, base.Add(10*time.Millisecond), 40),
		mkSpan(traceOK, "ok_c2", "ok_root", "tool.chart", domaintrace.StatusOK, "data_agent", tenant, base.Add(60*time.Millisecond), 30),
	}
	// traceErr：根 OK + 一个 ERROR 子 span。
	errSpans := []*domaintrace.Span{
		mkSpan(traceErr, "er_root", "", "agent.chat", domaintrace.StatusOK, "sql_agent", tenant, base.Add(5*time.Minute), 200),
		mkSpan(traceErr, "er_c1", "er_root", "tool.sql", domaintrace.StatusError, "sql_agent", tenant, base.Add(5*time.Minute+20*time.Millisecond), 50),
	}

	if err := repo.SaveBatch(ctx, okSpans); err != nil {
		t.Fatalf("SaveBatch ok: %v", err)
	}
	if err := repo.SaveBatch(ctx, errSpans); err != nil {
		t.Fatalf("SaveBatch err: %v", err)
	}
	// 幂等：重复投递同一批不应报错，也不应增加行数。
	if err := repo.SaveBatch(ctx, okSpans); err != nil {
		t.Fatalf("SaveBatch idempotent: %v", err)
	}

	// GetSpansByTraceID：按 start_time 升序返回 3 条。
	got, err := repo.GetSpansByTraceID(ctx, traceOK)
	if err != nil {
		t.Fatalf("GetSpansByTraceID: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("traceOK 期望 3 条 span（幂等去重后），得到 %d", len(got))
	}
	if got[0].SpanID != "ok_root" {
		t.Errorf("首条应为最早开始的 ok_root，得到 %s", got[0].SpanID)
	}

	// ListTraces：租户隔离下应看到两条 trace。
	q := &domaintrace.Query{TenantID: i64(tenant), Page: 1, PageSize: 20}
	list, total, err := repo.ListTraces(ctx, q)
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	if total != 2 {
		t.Fatalf("期望 2 条 trace，得到 total=%d", total)
	}
	byID := map[string]*domaintrace.TraceSummary{}
	for _, s := range list {
		byID[s.TraceID] = s
	}
	okSum := byID[traceOK]
	if okSum == nil {
		t.Fatalf("列表缺 traceOK")
	}
	if okSum.RootName != "agent.chat" {
		t.Errorf("根名应为 agent.chat，得到 %q", okSum.RootName)
	}
	if okSum.SpanCount != 3 {
		t.Errorf("span_count 应为 3，得到 %d", okSum.SpanCount)
	}
	if okSum.DurationMs != 120 {
		t.Errorf("根耗时应为 120ms，得到 %d", okSum.DurationMs)
	}
	if okSum.HasError {
		t.Errorf("traceOK 不应标记错误")
	}
	if okSum.StartTime.IsZero() {
		t.Errorf("start_time 不应为零值（聚合列扫描失败）")
	}
	if !okSum.EndTime.After(okSum.StartTime) {
		t.Errorf("end_time 应晚于 start_time，得到 start=%v end=%v", okSum.StartTime, okSum.EndTime)
	}
	if okSum.AgentName != "data_agent" {
		t.Errorf("agent_name 应为 data_agent，得到 %q", okSum.AgentName)
	}
	errSum := byID[traceErr]
	if errSum == nil || !errSum.HasError {
		t.Errorf("traceErr 应标记错误")
	}

	// only_error：仅返回含错误的 trace。
	q2 := &domaintrace.Query{TenantID: i64(tenant), OnlyError: true, Page: 1, PageSize: 20}
	list2, total2, err := repo.ListTraces(ctx, q2)
	if err != nil {
		t.Fatalf("ListTraces only_error: %v", err)
	}
	if total2 != 1 || len(list2) != 1 || list2[0].TraceID != traceErr {
		t.Fatalf("only_error 应仅返回 traceErr，得到 total=%d list=%d", total2, len(list2))
	}

	// 租户隔离：换一个租户应查不到。
	q3 := &domaintrace.Query{TenantID: i64(tenant + 1), Page: 1, PageSize: 20}
	_, total3, err := repo.ListTraces(ctx, q3)
	if err != nil {
		t.Fatalf("ListTraces other tenant: %v", err)
	}
	if total3 != 0 {
		t.Errorf("其他租户应查不到本测试数据，得到 total=%d", total3)
	}
}
