package executor

import (
	"context"
	"testing"

	domeval "cognida/internal/model/evaluation"
)

// stubExecutor 仅用于注册表路由测试：只关心 Type()，Execute 不做实事。
type stubExecutor struct {
	typ domeval.EvaluationType
}

func (s *stubExecutor) Execute(context.Context, *domeval.EvaluationTaskConfig, []*domeval.QAPair) ([]*domeval.QAResult, error) {
	return nil, nil
}

func (s *stubExecutor) Type() domeval.EvaluationType { return s.typ }

// TestRegistryRoutesFourTypes 验证 qa/llm/rag/agent 四类型均能取到执行器，
// 其中 qa 与 llm 互为别名、路由到同一实例。
func TestRegistryRoutesFourTypes(t *testing.T) {
	r := NewExecutorRegistry()

	qaExec := &stubExecutor{typ: domeval.EvaluationTypeQA}
	ragExec := &stubExecutor{typ: domeval.EvaluationTypeRAG}
	agentExec := &stubExecutor{typ: domeval.EvaluationTypeAgent}

	for _, e := range []Executor{qaExec, ragExec, agentExec} {
		if err := r.Register(e); err != nil {
			t.Fatalf("register %s failed: %v", e.Type(), err)
		}
	}

	cases := []struct {
		typ  domeval.EvaluationType
		want Executor
	}{
		{domeval.EvaluationTypeQA, qaExec},
		{domeval.EvaluationTypeLLM, qaExec}, // llm 命中 qa 的别名
		{domeval.EvaluationTypeRAG, ragExec},
		{domeval.EvaluationTypeAgent, agentExec},
	}
	for _, c := range cases {
		got, err := r.Get(c.typ)
		if err != nil {
			t.Fatalf("Get(%s) unexpected error: %v", c.typ, err)
		}
		if got != c.want {
			t.Fatalf("Get(%s) = %v, want %v", c.typ, got.Type(), c.want.Type())
		}
	}
}

// TestRegistryLLMAliasFromLLMRegistration 验证反向别名：若先注册 llm 执行器，
// 用历史别名 qa 查询同样命中。
func TestRegistryLLMAliasFromLLMRegistration(t *testing.T) {
	r := NewExecutorRegistry()
	llmExec := &stubExecutor{typ: domeval.EvaluationTypeLLM}
	if err := r.Register(llmExec); err != nil {
		t.Fatalf("register llm failed: %v", err)
	}
	got, err := r.Get(domeval.EvaluationTypeQA)
	if err != nil {
		t.Fatalf("Get(qa) unexpected error: %v", err)
	}
	if got != llmExec {
		t.Fatalf("Get(qa) did not route to llm executor")
	}
}

// TestRegistryUnknownType 未知类型返回可诊断错误（含已注册类型清单）。
func TestRegistryUnknownType(t *testing.T) {
	r := NewExecutorRegistry()
	if err := r.Register(&stubExecutor{typ: domeval.EvaluationTypeQA}); err != nil {
		t.Fatalf("register qa failed: %v", err)
	}
	_, err := r.Get(domeval.EvaluationType("does-not-exist"))
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}
