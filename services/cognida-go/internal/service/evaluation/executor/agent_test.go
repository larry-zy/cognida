package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentctx "cognida/internal/model/agent"
	domeval "cognida/internal/model/evaluation"
)

// fakeAgentService 以固定返回值实现 AgentService，供执行器指标采集测试。
type fakeAgentService struct {
	result   *AgentChatResult
	err      error
	delay    time.Duration
	panicMsg string // 非空时 Chat 触发 panic，供 B1 故障隔离测试
}

func (f *fakeAgentService) Chat(ctx context.Context, agentID, message string) (*AgentChatResult, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.panicMsg != "" {
		panic(f.panicMsg)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f *fakeAgentService) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	return &AgentInfo{ID: agentID, Name: "fake"}, nil
}

// TestAgentExecutor_CapturesRuntimeMetrics 验证 Execute 采集耗时并透传 Token/LLM 调用次数。
func TestAgentExecutor_CapturesRuntimeMetrics(t *testing.T) {
	svc := &fakeAgentService{
		result: &AgentChatResult{Answer: "A", TokensUsed: 321, LLMCalls: 5},
		delay:  10 * time.Millisecond,
	}
	exec := NewAgentExecutor(svc, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "agent-1", Type: domeval.EvaluationTypeAgent}
	dataset := []*QAPair{{Question: "Q"}}

	results, err := exec.Execute(context.Background(), config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Success {
		t.Fatalf("expected success, got error %q", r.Error)
	}
	if r.LatencyMs < 10 {
		t.Errorf("latency_ms = %d, want >= 10 (含 10ms 延迟)", r.LatencyMs)
	}
	if r.TokensUsed != 321 {
		t.Errorf("tokens_used = %d, want 321", r.TokensUsed)
	}
	if r.LLMCalls != 5 {
		t.Errorf("llm_calls = %d, want 5", r.LLMCalls)
	}
}

// TestAgentExecutor_CapturesPerItemRequestID 验证每条 QA 派生并落上子 rid（<base>#<序号>），
// 供前端深链到该轮 trace 瀑布图；失败路径也应保留 rid 以便查错。
func TestAgentExecutor_CapturesPerItemRequestID(t *testing.T) {
	okSvc := &fakeAgentService{result: &AgentChatResult{Answer: "A"}}
	exec := NewAgentExecutor(okSvc, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "agent-1", Type: domeval.EvaluationTypeAgent}
	dataset := []*QAPair{{Question: "Q1"}, {Question: "Q2"}}

	ctx := agentctx.WithRequestID(context.Background(), "eval-abc123-def456")
	results, err := exec.Execute(ctx, config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if results[0].RequestID != "eval-abc123-def456#1" {
		t.Errorf("item0 request_id = %q, want eval-abc123-def456#1", results[0].RequestID)
	}
	if results[1].RequestID != "eval-abc123-def456#2" {
		t.Errorf("item1 request_id = %q, want eval-abc123-def456#2", results[1].RequestID)
	}

	// 失败路径也应带上 rid
	errSvc := &fakeAgentService{err: errors.New("boom")}
	execErr := NewAgentExecutor(errSvc, 5*time.Second)
	failResults, _ := execErr.Execute(ctx, config, []*QAPair{{Question: "Q"}})
	if failResults[0].Success {
		t.Fatal("expected failure result")
	}
	if failResults[0].RequestID != "eval-abc123-def456#1" {
		t.Errorf("失败路径 request_id = %q, want eval-abc123-def456#1", failResults[0].RequestID)
	}

	// 无 rid 上游（裸 ctx）不应造 rid，保持「无 rid 即不追踪」语义
	bare, _ := exec.Execute(context.Background(), config, []*QAPair{{Question: "Q"}})
	if bare[0].RequestID != "" {
		t.Errorf("裸 ctx request_id = %q, want empty", bare[0].RequestID)
	}
}

// TestAgentExecutor_RecoversFromPanic 验证单条 QA 触发的 panic 被 safeChat 隔离：
// 该条标记失败并继续下一条，不冒泡带崩整批（B1 故障隔离）。
func TestAgentExecutor_RecoversFromPanic(t *testing.T) {
	svc := &fakeAgentService{panicMsg: "kaboom"}
	exec := NewAgentExecutor(svc, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "agent-1", Type: domeval.EvaluationTypeAgent}

	var results []*QAResult
	done := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.New("panic escaped Execute")
			}
		}()
		var e error
		results, e = exec.Execute(context.Background(), config, []*QAPair{{Question: "Q1"}, {Question: "Q2"}})
		return e
	}()
	if done != nil {
		t.Fatalf("Execute 应吞掉单条 panic，实得: %v", done)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Success {
			t.Errorf("item%d 应因 panic 失败", i)
		}
		if !strings.Contains(r.Error, "panic") {
			t.Errorf("item%d error=%q，应含 panic 信息", i, r.Error)
		}
	}
}

// TestAgentExecutor_RecordsLatencyOnError 验证失败路径仍记录耗时。
func TestAgentExecutor_RecordsLatencyOnError(t *testing.T) {
	svc := &fakeAgentService{err: errors.New("boom"), delay: 10 * time.Millisecond}
	exec := NewAgentExecutor(svc, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "agent-1", Type: domeval.EvaluationTypeAgent}

	results, err := exec.Execute(context.Background(), config, []*QAPair{{Question: "Q"}})
	if err != nil {
		t.Fatalf("Execute should not fail on per-QA error: %v", err)
	}
	if results[0].Success {
		t.Fatal("expected failure result")
	}
	if results[0].LatencyMs < 10 {
		t.Errorf("失败路径 latency_ms = %d, want >= 10", results[0].LatencyMs)
	}
}
