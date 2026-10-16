package governance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	d "cognida/internal/model/governance"
	"github.com/stretchr/testify/require"
)

func TestLabelBenchmarkSelectionIsFixedAndNERUnseen(t *testing.T) {
	labels := []d.Label{{ID: "a", Name: "alpha"}, {ID: "b", Name: "beta"}}
	train := []LabelBenchmarkSample{{ID: "train", Split: "train", Text: "seen", Label: "a"}}
	test := []LabelBenchmarkSample{}
	for i := 0; i < 70; i++ {
		label := "a"
		if i%2 == 1 {
			label = "b"
		}
		test = append(test, LabelBenchmarkSample{ID: fmt.Sprint(i), Split: "test", Text: fmt.Sprint("unique", i), Label: label})
	}
	test[0].Text = "seen"
	first, err := SelectLabelBenchmarkModelSamples("classification", labels, train, test)
	require.NoError(t, err)
	require.Len(t, first, 4)
	for i, j := 0, len(test)-1; i < j; i, j = i+1, j-1 {
		test[i], test[j] = test[j], test[i]
	}
	again, err := SelectLabelBenchmarkModelSamples("classification", labels, train, test)
	require.NoError(t, err)
	require.Equal(t, first, again)
	// Public test splits may omit declared classes. Keep those classes in the
	// score schema, select only real held-out rows, never backfill from train.
	sparse, err := SelectLabelBenchmarkModelSamples("classification", append(labels, d.Label{ID: "missing", Name: "absent"}), train, []LabelBenchmarkSample{{ID: "one", Split: "test", Text: "one", Label: "a"}})
	require.NoError(t, err)
	require.Len(t, sparse, 1)
	require.Equal(t, "one", sparse[0].ID)
	ner, err := SelectLabelBenchmarkModelSamples("named_entity", labels, train, test)
	require.NoError(t, err)
	require.Len(t, ner, 64)
	for _, s := range ner {
		require.False(t, s.TrainTextOverlap)
		require.NotEqual(t, LabelBenchmarkTextDigest("seen"), s.TextSHA256)
	}
}
func TestLabelBenchmarkPreflightBeforeCallsAndBoundedConcurrency(t *testing.T) {
	labels := []d.Label{{ID: "a", Name: "alpha"}}
	var active, maxActive, calls atomic.Int32
	predict := func(_ context.Context, in LabelBenchmarkInput) (LabelBenchmarkPrediction, error) {
		calls.Add(1)
		n := active.Add(1)
		for {
			old := maxActive.Load()
			if n <= old || maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
		return LabelBenchmarkPrediction{Annotation: d.Annotation{Labels: []string{"a"}}, DecisionStatus: "matched"}, nil
	}
	rows := []LabelBenchmarkSample{}
	for i := 0; i < 8; i++ {
		rows = append(rows, LabelBenchmarkSample{ID: fmt.Sprint(i), Split: "test", Text: "alpha", Label: "a"})
	}
	r, err := EvaluateLabelBenchmark(context.Background(), "classification", labels, rows, predict, 2)
	require.NoError(t, err)
	require.Equal(t, int32(8), calls.Load())
	require.LessOrEqual(t, maxActive.Load(), int32(2))
	require.Equal(t, 1.0, r.MicroF1)
	calls.Store(0)
	entity := d.Entity{Label: "a", Start: 0, End: 1, Text: "a"}
	_, err = EvaluateLabelBenchmark(context.Background(), "named_entity", labels, []LabelBenchmarkSample{{ID: "bad", Split: "test", Text: "alpha", Entities: []d.Entity{entity, entity}}}, predict, 2)
	require.Error(t, err)
	require.Zero(t, calls.Load())
}

func TestLabelBenchmarkFitsTrainingOnlyAndUsesProductionRules(t *testing.T) {
	schema := []d.Label{{ID: "a", Name: "first"}, {ID: "b", Name: "second"}}
	train := []LabelBenchmarkSample{{ID: "a1", Split: "train", Text: "refund bank fee", Label: "a"}, {ID: "a2", Split: "train", Text: "refund bank charge", Label: "a"}, {ID: "b1", Split: "train", Text: "replace old card", Label: "b"}, {ID: "b2", Split: "train", Text: "replace card now", Label: "b"}}
	fitted, err := FitLabelBenchmarkRules("classification", schema, train)
	require.NoError(t, err)
	repeated, err := FitLabelBenchmarkRules("classification", schema, []LabelBenchmarkSample{train[3], train[2], train[1], train[0]})
	require.NoError(t, err)
	require.Equal(t, fitted, repeated)
	pred, err := NewLabelBenchmarkRulePredictor("classification", fitted)
	require.NoError(t, err)
	actual, err := pred(context.Background(), LabelBenchmarkInput{Kind: "classification", Text: "refund charge"})
	require.NoError(t, err)
	expected, provenance, err := candidateFor(d.AnnotationTask{Type: "classification", Labels: fitted}, annotationSource{Content: "refund charge"})
	require.NoError(t, err)
	require.Equal(t, expected, actual.Annotation)
	require.Equal(t, provenance.DecisionStatus, actual.DecisionStatus)
	train[0].Split = "test"
	_, err = FitLabelBenchmarkRules("classification", schema, train)
	require.ErrorContains(t, err, "held-out")
}
func TestLabelBenchmarkChineseNgramsAreFixedAndLiteral(t *testing.T) {
	schema := []d.Label{{ID: "a", Name: "退款"}, {ID: "b", Name: "查询"}}
	rows := []LabelBenchmarkSample{{Split: "train", Text: "我要退款", Label: "a"}, {Split: "train", Text: "现在退款", Label: "a"}, {Split: "train", Text: "查询天气", Label: "b"}, {Split: "train", Text: "天气如何", Label: "b"}}
	fitted, err := FitLabelBenchmarkRules("classification", schema, rows, "zh-CN")
	require.NoError(t, err)
	require.Contains(t, fitted[0].Keywords, "退款")
	require.Contains(t, fitted[1].Keywords, "天气")
	require.NotContains(t, benchmarkTerms("银行，余额", "zh-CN"), "行余")
}
func TestLabelBenchmarkClassificationErrorsAndAbstentionStayInDenominator(t *testing.T) {
	schema := []d.Label{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}}
	rows := []LabelBenchmarkSample{{ID: "ok", Split: "test", Text: "one", Label: "a"}, {ID: "wrong", Split: "test", Text: "two", Label: "b"}, {ID: "ambiguous", Split: "test", Text: "three", Label: "a"}, {ID: "invalid", Split: "test", Text: "four", Label: "b"}, {ID: "offline", Split: "test", Text: "five", Label: "a"}}
	calls := 0
	report, err := EvaluateLabelBenchmark(context.Background(), "classification", schema, rows, func(_ context.Context, in LabelBenchmarkInput) (LabelBenchmarkPrediction, error) {
		calls++
		p := LabelBenchmarkPrediction{DecisionStatus: "matched", Annotation: d.Annotation{Labels: []string{"a"}}}
		switch in.ID {
		case "ambiguous":
			p.Annotation.Labels = []string{"a", "b"}
		case "invalid":
			p.Annotation.Labels = []string{"invented"}
		case "offline":
			return p, errors.New("private provider error")
		}
		return p, nil
	})
	require.NoError(t, err)
	require.Equal(t, 5, calls)
	require.Equal(t, 5, report.Samples)
	require.Equal(t, 2, report.Covered)
	require.Equal(t, 3, report.Abstained)
	require.Equal(t, 1, report.Ambiguous)
	require.Equal(t, 1, report.ContractErrors)
	require.Equal(t, 1, report.ExecutionErrors)
	require.InDelta(t, .2, *report.Accuracy, 1e-12)
	require.InDelta(t, .5, report.MicroPrecision, 1e-12)
	require.InDelta(t, .2, report.MicroRecall, 1e-12)
	require.InDelta(t, 2.0/7, report.MicroF1, 1e-12)
	require.InDelta(t, .4/3, report.MacroF1, 1e-12)
	require.InDelta(t, .2, report.MacroF1SupportedOnly, 1e-12)
	require.Equal(t, 2, report.LabelsWithGoldSupport)
	require.Nil(t, report.Usage.CostUSD)
	require.Equal(t, "execution_error", report.Outcomes[4].ErrorCode)
}
func TestLabelBenchmarkNERUsesExactUnicodeSpansAndScoresPartialDrafts(t *testing.T) {
	schema := []d.Label{{ID: "person", Name: "person"}, {ID: "place", Name: "place"}}
	rows := []LabelBenchmarkSample{{ID: "r", Split: "test", Text: "李明去巴黎", Entities: []d.Entity{{Label: "person", Start: 0, End: 2, Text: "李明"}, {Label: "place", Start: 3, End: 5, Text: "巴黎"}}}}
	report, err := EvaluateLabelBenchmark(context.Background(), "named_entity", schema, rows, func(context.Context, LabelBenchmarkInput) (LabelBenchmarkPrediction, error) {
		return LabelBenchmarkPrediction{DecisionStatus: "uncertain", Annotation: d.Annotation{Entities: []d.Entity{{ID: "e", Label: "person", Start: 0, End: 2, Text: "李明"}}}}, nil
	})
	require.NoError(t, err)
	require.InDelta(t, 1.0, report.MicroPrecision, 1e-12)
	require.InDelta(t, .5, report.MicroRecall, 1e-12)
	require.InDelta(t, 2.0/3, report.MicroF1, 1e-12)
	require.Equal(t, 1, report.Abstained)
	require.Zero(t, report.Covered)
	require.Nil(t, report.Accuracy)
	report, err = EvaluateLabelBenchmark(context.Background(), "named_entity", schema, rows, func(context.Context, LabelBenchmarkInput) (LabelBenchmarkPrediction, error) {
		return LabelBenchmarkPrediction{DecisionStatus: "matched", Annotation: d.Annotation{Entities: []d.Entity{{ID: "e", Label: "person", Start: 0, End: 6, Text: "李明"}}}}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.ContractErrors)
	require.Zero(t, report.MicroF1)
}
func TestLabelBenchmarkCancelledRunDoesNotCallPredictorAndKeepsRows(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := EvaluateLabelBenchmark(ctx, "classification", []d.Label{{ID: "a", Name: "A"}}, []LabelBenchmarkSample{{ID: "x", Split: "test", Text: "x", Label: "a"}}, func(context.Context, LabelBenchmarkInput) (LabelBenchmarkPrediction, error) {
		t.Fatal("cancelled callback invoked")
		return LabelBenchmarkPrediction{}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, report.ExecutionErrors)
	require.Equal(t, "cancelled", report.Outcomes[0].ErrorCode)
}
func TestLabelBenchmarkReaderRejectsInvalidAndRepeatedIDs(t *testing.T) {
	row := `{"id":"a","split":"test","text":"hello","label":"one"}`
	rows, err := ReadLabelBenchmarkJSONL(strings.NewReader(row))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	for _, s := range []string{"", row + "\n" + row, `{"id":"a","split":"hidden","text":"hi"}`, "\n"} {
		_, err = ReadLabelBenchmarkJSONL(strings.NewReader(s))
		require.Error(t, err)
	}
}
