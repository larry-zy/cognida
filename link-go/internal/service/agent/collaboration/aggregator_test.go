package collaboration

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeChatModel is a deterministic stand-in for an LLM, used in tests.
type fakeChatModel struct {
	reply string
	err   error
	calls int
}

func (f *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return schema.AssistantMessage(f.reply, nil), nil
}

func (f *fakeChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (f *fakeChatModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func execResult(contents map[string]string) *ExecutionResult {
	subTasks := make(map[string]*SubTaskResult, len(contents))
	for id, c := range contents {
		subTasks[id] = &SubTaskResult{Content: c}
	}
	return &ExecutionResult{
		PlanID:     "plan-1",
		SubTasks:   subTasks,
		TotalTasks: len(contents),
	}
}

func TestDetectConflicts_HeuristicContradiction(t *testing.T) {
	a := NewResultAggregator(nil, StrategySynthesize)
	res := execResult(map[string]string{
		"t1": "The deployment is safe and ready for the production release.",
		"t2": "The deployment is not safe and not ready for production.",
	})

	conflicts := a.detectConflicts(context.Background(), res)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.Type != ConflictTypeContradiction {
		t.Errorf("expected contradiction type, got %s", c.Type)
	}
	if c.Source1 == "" || c.Source2 == "" {
		t.Errorf("expected sources populated, got %q / %q", c.Source1, c.Source2)
	}
	if c.Severity <= 0.5 {
		t.Errorf("expected severity above base 0.5 for overlapping topic, got %f", c.Severity)
	}
}

func TestDetectConflicts_NoConflictSamePolarity(t *testing.T) {
	a := NewResultAggregator(nil, StrategySynthesize)
	res := execResult(map[string]string{
		"t1": "The sky appears blue during a clear day.",
		"t2": "The sky looks blue when the day is clear.",
	})
	if conflicts := a.detectConflicts(context.Background(), res); len(conflicts) != 0 {
		t.Fatalf("expected no conflict, got %d", len(conflicts))
	}
}

func TestDetectConflicts_DifferentTopicsNoConflict(t *testing.T) {
	a := NewResultAggregator(nil, StrategySynthesize)
	// Opposite polarity but unrelated topics -> not a contradiction.
	res := execResult(map[string]string{
		"t1": "Photosynthesis converts sunlight into chemical energy.",
		"t2": "The quarterly invoice was not delivered to accounting.",
	})
	if conflicts := a.detectConflicts(context.Background(), res); len(conflicts) != 0 {
		t.Fatalf("expected no conflict for different topics, got %d", len(conflicts))
	}
}

func TestDetectConflicts_LLMPath(t *testing.T) {
	llm := &fakeChatModel{reply: "```json\n[{\"source1\":\"t1\",\"source2\":\"t2\",\"type\":\"inconsistency\",\"description\":\"differ on date\",\"severity\":0.8}]\n```"}
	a := NewResultAggregator(llm, StrategySynthesize)
	res := execResult(map[string]string{
		"t1": "Launch is on March 1.",
		"t2": "Launch is on April 1.",
	})

	conflicts := a.detectConflicts(context.Background(), res)
	if llm.calls != 1 {
		t.Fatalf("expected LLM to be called once, got %d", llm.calls)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict from LLM, got %d", len(conflicts))
	}
	if conflicts[0].Type != ConflictTypeInconsistency {
		t.Errorf("expected inconsistency, got %s", conflicts[0].Type)
	}
	if conflicts[0].Content1 == "" || conflicts[0].Content2 == "" {
		t.Errorf("expected content backfilled from task IDs")
	}
}

func TestDetectConflicts_LLMErrorFallsBackToHeuristic(t *testing.T) {
	llm := &fakeChatModel{err: context.DeadlineExceeded}
	a := NewResultAggregator(llm, StrategySynthesize)
	res := execResult(map[string]string{
		"t1": "The migration succeeded without data loss.",
		"t2": "The migration did not succeed and caused data loss.",
	})
	conflicts := a.detectConflicts(context.Background(), res)
	if len(conflicts) != 1 {
		t.Fatalf("expected heuristic fallback to find 1 conflict, got %d", len(conflicts))
	}
}

func TestVote_MajorityWins(t *testing.T) {
	a := NewResultAggregator(nil, StrategyVote)
	res := execResult(map[string]string{
		"t1": "Paris is the capital of France, a major European city.",
		"t2": "The capital of France is Paris, a large city in Europe.",
		"t3": "The result depends on quantum entanglement between photons.",
	})

	resp, err := a.Aggregate(context.Background(), res, "What is the capital of France?")
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}
	if got := resp.Metadata["winning_votes"]; got != 2 {
		t.Errorf("expected winning_votes 2, got %v", got)
	}
	if got := resp.Metadata["total_votes"]; got != 3 {
		t.Errorf("expected total_votes 3, got %v", got)
	}
	if resp.SelectedSource != "t1" && resp.SelectedSource != "t2" {
		t.Errorf("expected winner from majority cluster, got %s", resp.SelectedSource)
	}
	if !strings.Contains(strings.ToLower(resp.Content), "paris") {
		t.Errorf("expected consensus content about Paris, got %q", resp.Content)
	}
}

func TestVote_SingleVoter(t *testing.T) {
	a := NewResultAggregator(nil, StrategyVote)
	res := execResult(map[string]string{"only": "The single answer."})
	resp, err := a.Aggregate(context.Background(), res, "q")
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}
	if resp.Content != "The single answer." {
		t.Errorf("unexpected content: %q", resp.Content)
	}
	if resp.SelectedSource != "only" {
		t.Errorf("expected selected source 'only', got %s", resp.SelectedSource)
	}
}

func TestVote_SynthesizesWinningClusterWithLLM(t *testing.T) {
	llm := &fakeChatModel{reply: "Synthesized consensus answer about Paris."}
	a := NewResultAggregator(llm, StrategyVote)
	res := execResult(map[string]string{
		"t1": "Paris is the capital of France, a major European city.",
		"t2": "The capital of France is Paris, a large city in Europe.",
	})
	resp, err := a.Aggregate(context.Background(), res, "capital?")
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}
	if llm.calls == 0 {
		t.Error("expected LLM to be used to synthesize the winning cluster")
	}
	if resp.Content != "Synthesized consensus answer about Paris." {
		t.Errorf("expected synthesized content, got %q", resp.Content)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"```json\n[1,2]\n```":   "[1,2]",
		"prefix [\"a\"] suffix": "[\"a\"]",
		"{\"k\":1}":             "{\"k\":1}",
		"```\n{\"x\":2}\n```":   "{\"x\":2}",
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJaccardSimilarity(t *testing.T) {
	a := tokenSet("alpha beta gamma")
	b := tokenSet("beta gamma delta")
	got := jaccardSimilarity(a, b)
	// intersection {beta,gamma}=2, union {alpha,beta,gamma,delta}=4 -> 0.5
	if got < 0.49 || got > 0.51 {
		t.Errorf("expected ~0.5, got %f", got)
	}
	if jaccardSimilarity(tokenSet(""), tokenSet("")) != 0 {
		t.Error("expected 0 for empty sets")
	}
}
