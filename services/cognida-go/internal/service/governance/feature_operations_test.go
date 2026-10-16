package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestEmbeddingUsesPinnedProviderAndValidatesBeforeCall(t *testing.T) {
	f, ref, in := featureEngineInput()
	f.Type = "vector"
	f.Definition = []byte(`{"kind":"embedding","field":"category","entity_field":"entity","embedding":{"model_id":"provider/model","revision":"trusted-v1","dimension":2}}`)
	s := &Service{}
	calls := 0
	_, _, e := s.computeFeature(context.Background(), f, ref, in)
	require.Error(t, e)
	s.SetFeatureEmbedding("provider/model", "trusted-v1", func(ctx context.Context, texts []string) ([][]float64, error) {
		calls++
		require.Equal(t, []string{"  Gold  ", "silver"}, texts)
		return [][]float64{{.2, .8}, {.5, .5}}, nil
	})
	values, _, e := s.computeFeature(context.Background(), f, ref, in)
	require.NoError(t, e)
	require.Equal(t, 1, calls)
	require.Equal(t, "[0.2,0.8]", string(values[0].Value))
	in.Rows[1][2] = 42
	_, _, e = s.computeFeature(context.Background(), f, ref, in)
	require.Error(t, e)
	require.Equal(t, 1, calls, "validate entire batch before paid request")
	s.SetFeatureEmbedding("provider/model", "", func(context.Context, []string) ([][]float64, error) {
		t.Fatal("missing revision called model")
		return nil, nil
	})
	_, _, e = s.computeFeature(context.Background(), f, ref, in)
	require.Error(t, e)
}
func TestEmbeddingRejectsInvalidRealResponse(t *testing.T) {
	for _, vector := range [][]float64{{0, 0}, {1}, {math.NaN(), 1}, {math.Inf(1), 1}} {
		f, ref, in := featureEngineInput()
		in.Rows = in.Rows[:1]
		f.Type = "vector"
		f.Definition = []byte(`{"kind":"embedding","field":"category","embedding":{"model_id":"model","revision":"v1","dimension":2}}`)
		s := &Service{}
		s.SetFeatureEmbedding("model", "v1", func(context.Context, []string) ([][]float64, error) { return [][]float64{vector}, nil })
		_, _, e := s.computeFeature(context.Background(), f, ref, in)
		require.Error(t, e)
	}
}
func TestDriftRealShapeCategoricalMissingAndInsufficient(t *testing.T) {
	s := d.DriftSampling{Limit: 100, Minimum: 4, Seed: "fixed"}
	a := l.Input{Kind: "table", Columns: []string{"amount", "category"}, Rows: [][]any{}}
	b := a
	for i := 0; i < 20; i++ {
		a.Rows = append(a.Rows, []any{json.Number("9007199254740993.000000000000000001"), "a"})
		b.Rows = append(b.Rows, []any{json.Number("9007199254740993.000000000000000002"), "b"})
	}
	metrics, na, nb, changed, e := compareDrift(a, b, s, .2)
	require.NoError(t, e)
	require.True(t, changed)
	require.Equal(t, 20, na)
	require.Equal(t, 20, nb)
	for _, m := range metrics {
		if m.Method == "empirical_ks" || m.Method == "categorical_total_variation" {
			require.Equal(t, 1.0, *m.Effect)
		}
	}
	metrics, _, _, changed, e = compareDrift(a, a, s, .2)
	require.NoError(t, e)
	require.False(t, changed)
	for _, m := range metrics {
		if m.Effect != nil {
			require.Zero(t, *m.Effect)
		}
	}
	b.Rows = b.Rows[:2]
	metrics, _, _, _, e = compareDrift(a, b, s, .2)
	require.NoError(t, e)
	require.Condition(t, func() bool {
		for _, m := range metrics {
			if m.Method == "empirical_ks" {
				return m.Status == "insufficient" && m.Effect == nil
			}
		}
		return false
	})
	b = a
	b.Columns = []string{"new", "category"}
	metrics, _, _, changed, e = compareDrift(a, b, s, .2)
	require.NoError(t, e)
	require.True(t, changed)
	require.Equal(t, "schema", metrics[0].Method)
}
func TestDriftDeterministicSamplesAreActualRows(t *testing.T) {
	in := l.Input{Rows: [][]any{}}
	for i := 0; i < 200; i++ {
		in.Rows = append(in.Rows, []any{i})
	}
	s := d.DriftSampling{Limit: 20, Minimum: 4, Seed: "seed"}
	sample := driftSample(in, s)
	require.Equal(t, sample, driftSample(in, s))
	require.Len(t, sample, 20)
	seen := map[int]bool{}
	for _, row := range sample {
		n := row[0].(int)
		require.False(t, seen[n])
		seen[n] = true
		require.Equal(t, in.Rows[n], row)
	}
	s.Seed = "another"
	require.NotEqual(t, sample, driftSample(in, s))
}

func TestDriftSQLTypeClassificationNeverGuessesSubstrings(t *testing.T) {
	for _, kind := range []string{"POINT", "INTERVAL", "NUMBER|STRING", "VARCHAR", "BOOLEAN"} {
		require.False(t, driftIsNumeric(kind), kind)
	}
	for _, kind := range []string{"NUMBER", "BIGINT UNSIGNED", "DECIMAL(38,18)", "Nullable(Decimal64(4))"} {
		require.True(t, driftIsNumeric(kind), kind)
	}
}
