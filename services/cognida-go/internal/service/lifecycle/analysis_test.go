package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type goalCompute func(context.Context, string, d.Input, map[string]any, string) (ComputeResult, error)

func (f goalCompute) Call(c context.Context, op string, in d.Input, o map[string]any, r string) (ComputeResult, error) {
	return f(c, op, in, o, r)
}

func TestAnalyzeGoalPreservesStatisticsAndCarriesGoal(t *testing.T) {
	input := d.Input{Kind: "table", Columns: []string{"sales"}, Rows: [][]any{{10}, {20}}}
	called := false
	compute := goalCompute(func(ctx context.Context, op string, in d.Input, opts map[string]any, requestID string) (ComputeResult, error) {
		called = true
		require.Equal(t, "analyze", op)
		require.Equal(t, "解释销售差异", opts["prompt"])
		require.Equal(t, "asset:1", opts["source_ref"])
		require.Equal(t, "trace-goal", requestID)
		require.Equal(t, float64(15), opts["statistics"].(map[string]any)["columns"].([]map[string]any)[0]["mean"])
		return ComputeResult{Method: "goal-directed-llm-v1", Result: map[string]any{"narrative": "销售均值15", "verification_status": "citations_checked_interpretation_unverified", "columns": []any{"fabricated"}}}, nil
	})
	out, err := AnalyzeGoal(context.Background(), compute, input, " 解释销售差异 ", "asset:1", "trace-goal")
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "销售均值15", out["narrative"])
	require.Equal(t, float64(15), out["columns"].([]map[string]any)[0]["mean"])
	plain, err := AnalyzeGoal(context.Background(), nil, input, " ", "asset:1", "")
	require.NoError(t, err)
	require.NotContains(t, plain, "narrative")
	_, err = AnalyzeGoal(context.Background(), nil, input, "real goal", "asset:1", "")
	require.Error(t, err)
	_, err = AnalyzeGoal(context.Background(), nil, input, strings.Repeat("字", 4001), "asset:1", "")
	require.ErrorIs(t, err, d.ErrInvalid)
}
func TestAnalyzeGoalRejectsFailureAndInvalidInterpretation(t *testing.T) {
	input := d.Input{Kind: "text", Text: "evidence"}
	for _, result := range []ComputeResult{{}, {Result: map[string]any{"narrative": "unverified"}}} {
		_, err := AnalyzeGoal(context.Background(), goalCompute(func(context.Context, string, d.Input, map[string]any, string) (ComputeResult, error) {
			return result, nil
		}), input, "goal", "asset:1", "")
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AnalyzeGoal(ctx, goalCompute(func(c context.Context, _ string, _ d.Input, _ map[string]any, _ string) (ComputeResult, error) {
		return ComputeResult{}, c.Err()
	}), input, "goal", "asset:1", "")
	require.ErrorIs(t, err, context.Canceled)
}
func TestPipelineOptionsRejectMisspelledAndProtectedStages(t *testing.T) {
	def := DefaultTemplate()
	for _, opts := range []map[string]any{{"prompt": "silently ignored"}, {"stage_options": []any{}}, {"stage_options": map[string]any{"unknown": map[string]any{}}}, {"stage_options": map[string]any{def.Stages[0].ID: map[string]any{"tenant_id": 9}}}} {
		require.True(t, errors.Is(validatePipelineOptions(def, opts), d.ErrInvalid))
	}
	require.NoError(t, validatePipelineOptions(def, map[string]any{"stage_options": map[string]any{def.Stages[0].ID: map[string]any{"prompt": "goal"}}}))
	for _, opts := range []map[string]any{{"min_score": 101.0}, {"min_score": "90"}} {
		require.Error(t, validateExpertOptions("data-quality", opts))
	}
	require.Error(t, validateExpertOptions("data-analysis", map[string]any{"prompt": false}))
}
