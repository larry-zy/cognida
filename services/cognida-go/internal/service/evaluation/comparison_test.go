package evaluation

import (
	d "cognida/internal/model/evaluation"
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPairedComparisonGateAndMissingEvidence(t *testing.T) {
	base, next := []*d.EvaluationResult{}, []*d.EvaluationResult{}
	for i := 0; i < 25; i++ {
		base = append(base, &d.EvaluationResult{Question: fmt.Sprint(i), ReferenceAnswer: "ref", Success: true, Scores: map[string]float64{"answer_accuracy": .4}})
		next = append(next, &d.EvaluationResult{Question: fmt.Sprint(i), ReferenceAnswer: "ref", Success: true, Scores: map[string]float64{"answer_accuracy": .8}})
	}
	in := ComparisonRequest{Metric: "answer_accuracy", HigherIsBetter: true}
	out, e := compareResults(in, base, next)
	require.NoError(t, e)
	require.Equal(t, "eligible_for_review", out.Gate)
	require.InDelta(t, .4, out.Improvement, 1e-10)
	out, e = compareResults(in, next, base)
	require.NoError(t, e)
	require.Equal(t, "regression", out.Gate)
	out, e = compareResults(in, base, next[:24])
	require.NoError(t, e)
	require.Equal(t, "insufficient_evidence", out.Gate)
	require.Equal(t, 1, out.MissingPairs)
	next[0].ReferenceAnswer = "changed answer"
	out, e = compareResults(in, base, next)
	require.NoError(t, e)
	require.Equal(t, 2, out.MissingPairs)
	_, e = compareResults(in, append(base, base[0]), next)
	require.ErrorIs(t, e, d.ErrInvalidConfig)
	_, ok := resultMetric(&d.EvaluationResult{ID: 12, Success: true}, "id")
	require.False(t, ok)
}
func TestComparisonRejectsForeignTaskAndDatasetDrift(t *testing.T) {
	tr := newMockTaskRepo()
	rr := newMockResultRepo()
	s := NewService(nil, tr, rr, nil, nil, nil)
	for _, id := range []string{"base", "next"} {
		tr.tasks[id] = &d.EvaluationTask{ID: id, TenantID: 1, UserID: 2, Status: d.TaskStatusCompleted, DatasetID: "set", Type: d.EvaluationTypeAgent}
	}
	in := ComparisonRequest{BaselineID: "base", CandidateID: "next", Metric: "answer_accuracy", HigherIsBetter: true}
	_, e := s.Compare(context.Background(), 2, 2, in)
	require.ErrorIs(t, e, d.ErrTaskNotFound)
	_, e = s.Compare(context.Background(), 1, 3, in)
	require.ErrorIs(t, e, d.ErrTaskNotFound)
	tr.tasks["next"].DatasetID = "different"
	_, e = s.Compare(context.Background(), 1, 2, in)
	require.ErrorIs(t, e, d.ErrInvalidConfig)
}
