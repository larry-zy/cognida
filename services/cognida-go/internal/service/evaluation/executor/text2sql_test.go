package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	domeval "cognida/internal/model/evaluation"
)

// fakeSQLRunner 以固定结果集实现 SQLRunner，供 Text2SQL 执行器测试（按 SQL 文本返回不同结果）。
type fakeSQLRunner struct {
	bySQL map[string]*SQLResultSet
	err   error
}

func (f *fakeSQLRunner) RunReadOnly(ctx context.Context, sql string) (*SQLResultSet, error) {
	if f.err != nil {
		return nil, f.err
	}
	if rs, ok := f.bySQL[sql]; ok {
		return rs, nil
	}
	return &SQLResultSet{}, nil
}

// TestText2SQLExecutor_CapturesSQLAndResultSets 验证执行器：写入金标准/生成 SQL，
// 并用 SQLRunner 采集双方结果集；生成出 SQL 即视为成功。
func TestText2SQLExecutor_CapturesSQLAndResultSets(t *testing.T) {
	svc := &fakeAgentService{result: &AgentChatResult{
		Answer:       "订单总额 1000",
		GeneratedSQL: "SELECT SUM(amount) FROM orders",
	}}
	runner := &fakeSQLRunner{bySQL: map[string]*SQLResultSet{
		"SELECT SUM(amount) FROM orders": {Columns: []string{"s"}, Rows: [][]interface{}{{1000}}},
		"SELECT SUM(amt) FROM orders":    {Columns: []string{"s"}, Rows: [][]interface{}{{999}}},
	}}
	exec := NewText2SQLExecutor(svc, runner, 5*time.Second)

	if exec.Type() != domeval.EvaluationTypeSQL {
		t.Fatalf("Type() = %q, want sql", exec.Type())
	}

	config := &EvaluationTaskConfig{AgentID: "text2sql", Type: domeval.EvaluationTypeSQL}
	// 金标准 SQL 存 reference_answer（数据集不新增列）；GoldSQL 为空回退它。
	dataset := []*QAPair{{Question: "订单总额", ReferenceAnswer: "SELECT SUM(amt) FROM orders"}}

	results, err := exec.Execute(context.Background(), config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	r := results[0]
	if !r.Success {
		t.Fatalf("expected success, got error %q", r.Error)
	}
	if r.GeneratedSQL != "SELECT SUM(amount) FROM orders" {
		t.Errorf("generated_sql = %q", r.GeneratedSQL)
	}
	if r.GoldSQL != "SELECT SUM(amt) FROM orders" {
		t.Errorf("gold_sql = %q, want 回退 reference_answer", r.GoldSQL)
	}
	if len(r.ResultSet) != 1 || r.ResultSet[0][0] != 1000 {
		t.Errorf("result_set = %v, want [[1000]]", r.ResultSet)
	}
	if len(r.GoldResultSet) != 1 || r.GoldResultSet[0][0] != 999 {
		t.Errorf("gold_result_set = %v, want [[999]]", r.GoldResultSet)
	}
}

// panicSQLRunner 的 RunReadOnly 恒 panic，供 B1 SQL 执行故障隔离测试。
type panicSQLRunner struct{}

func (panicSQLRunner) RunReadOnly(ctx context.Context, sql string) (*SQLResultSet, error) {
	panic("sql runner boom")
}

// TestText2SQLExecutor_RecoversFromRunnerPanic 验证只读 SQL 执行 panic 被 runSafely 隔离：
// 结果集缺席但该条仍按「已产出 SQL」成功，且不冒泡带崩整批（B1 故障隔离）。
func TestText2SQLExecutor_RecoversFromRunnerPanic(t *testing.T) {
	svc := &fakeAgentService{result: &AgentChatResult{GeneratedSQL: "SELECT 1"}}
	exec := NewText2SQLExecutor(svc, panicSQLRunner{}, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "text2sql", Type: domeval.EvaluationTypeSQL}
	dataset := []*QAPair{{Question: "Q", ReferenceAnswer: "SELECT gold"}}

	var results []*QAResult
	escaped := func() (panicked bool) {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		results, _ = exec.Execute(context.Background(), config, dataset)
		return false
	}()
	if escaped {
		t.Fatal("SQL 执行 panic 不应冒泡出 Execute")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("已产出 SQL 应判成功，实得 error=%q", results[0].Error)
	}
	if results[0].ResultSet != nil || results[0].GoldResultSet != nil {
		t.Error("panic 路径不应写入结果集")
	}
}

// TestText2SQLExecutor_PrefersGoldSQLField 验证 QAPair.GoldSQL 非空时优先于 ReferenceAnswer。
func TestText2SQLExecutor_PrefersGoldSQLField(t *testing.T) {
	svc := &fakeAgentService{result: &AgentChatResult{GeneratedSQL: "SELECT 1"}}
	exec := NewText2SQLExecutor(svc, nil, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "text2sql", Type: domeval.EvaluationTypeSQL}
	dataset := []*QAPair{{
		Question:        "Q",
		ReferenceAnswer: "SELECT ref",
		GoldSQL:         "SELECT gold",
	}}

	results, err := exec.Execute(context.Background(), config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if results[0].GoldSQL != "SELECT gold" {
		t.Errorf("gold_sql = %q, want SELECT gold（GoldSQL 优先）", results[0].GoldSQL)
	}
}

// TestText2SQLExecutor_NoSQLProducedIsFailure 验证 Agent 未产出 SQL 时该条记失败。
func TestText2SQLExecutor_NoSQLProducedIsFailure(t *testing.T) {
	svc := &fakeAgentService{result: &AgentChatResult{Answer: "无 SQL"}}
	exec := NewText2SQLExecutor(svc, nil, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "text2sql", Type: domeval.EvaluationTypeSQL}
	dataset := []*QAPair{{Question: "Q", ReferenceAnswer: "SELECT 1"}}

	results, err := exec.Execute(context.Background(), config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if results[0].Success {
		t.Fatal("expected failure when no SQL produced")
	}
}

// TestText2SQLExecutor_NilRunnerSkipsExecution 验证 sqlRunner 为 nil 时只算结构指标不执行 SQL。
func TestText2SQLExecutor_NilRunnerSkipsExecution(t *testing.T) {
	svc := &fakeAgentService{result: &AgentChatResult{GeneratedSQL: "SELECT 1"}}
	exec := NewText2SQLExecutor(svc, nil, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "text2sql", Type: domeval.EvaluationTypeSQL}
	dataset := []*QAPair{{Question: "Q", ReferenceAnswer: "SELECT 1"}}

	results, err := exec.Execute(context.Background(), config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !results[0].Success {
		t.Fatalf("expected success, got %q", results[0].Error)
	}
	if results[0].ResultSet != nil || results[0].GoldResultSet != nil {
		t.Error("nil runner 不应采集结果集")
	}
}

// TestText2SQLExecutor_RequiresAgentID 验证缺 AgentID 时整批报错（与 AgentExecutor 一致）。
func TestText2SQLExecutor_RequiresAgentID(t *testing.T) {
	exec := NewText2SQLExecutor(&fakeAgentService{}, nil, 5*time.Second)
	config := &EvaluationTaskConfig{Type: domeval.EvaluationTypeSQL}
	if _, err := exec.Execute(context.Background(), config, []*QAPair{{Question: "Q"}}); err == nil {
		t.Fatal("expected error when agent_id missing")
	}
}

// TestText2SQLExecutor_RunnerErrorDoesNotBlock 验证结果集执行失败不阻断（仍成功产出 SQL）。
func TestText2SQLExecutor_RunnerErrorDoesNotBlock(t *testing.T) {
	svc := &fakeAgentService{result: &AgentChatResult{GeneratedSQL: "SELECT 1"}}
	runner := &fakeSQLRunner{err: errors.New("db down")}
	exec := NewText2SQLExecutor(svc, runner, 5*time.Second)
	config := &EvaluationTaskConfig{AgentID: "text2sql", Type: domeval.EvaluationTypeSQL}
	dataset := []*QAPair{{Question: "Q", ReferenceAnswer: "SELECT 1"}}

	results, err := exec.Execute(context.Background(), config, dataset)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !results[0].Success {
		t.Fatal("SQL 执行失败不应影响「已产出 SQL」的成功判定")
	}
	if results[0].ResultSet != nil {
		t.Error("执行失败不应写入结果集")
	}
}
