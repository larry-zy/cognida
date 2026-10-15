package governance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func featureEngineInput() (d.Feature, l.DataAssetRef, l.Input) {
	ref := l.DataAssetRef{RefID: l.RefID{AssetID: uuid.NewString(), Version: 1}, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return d.Feature{Meta: d.Meta{ID: uuid.NewString(), Version: 1}, Name: "derived", Type: "numerical", SourceRefID: ref.RefID.String()}, ref, l.Input{Kind: "table", Columns: []string{"entity", "amount", "category", "event"}, Rows: [][]any{{"00123", "9007199254740993.123456789012345678", "  Gold  ", "2026-01-01T01:00:00Z"}, {"b", "0.2", "silver", "2026-01-01T02:00:00Z"}}}
}
func TestFeatureEngineDecimalAndCategories(t *testing.T) {
	f, ref, input := featureEngineInput()
	for _, test := range []struct{ name, typ, definition, want string }{
		{"field precision", "numerical", `{"kind":"field","field":"amount","entity_field":"entity"}`, `9007199254740993.123456789012345678`},
		{"decimal add", "numerical", `{"kind":"expression","entity_field":"entity","expression":{"op":"add","args":[{"op":"column","field":"amount"},{"op":"literal","value":0.1}]}}`, `9007199254740993.223456789012345678`},
		{"division semantics", "numerical", `{"kind":"expression","entity_field":"entity","expression":{"op":"div","args":[{"op":"literal","value":1},{"op":"literal","value":3}]}}`, `0.333333333333333333`},
		{"category transform", "categorical", `{"kind":"expression","entity_field":"entity","expression":{"op":"lower","args":[{"op":"trim","args":[{"op":"column","field":"category"}]}]}}`, `"gold"`},
		{"identifier preservation", "categorical", `{"kind":"field","field":"entity","entity_field":"entity"}`, `"00123"`},
		{"category mapping", "categorical", `{"kind":"expression","entity_field":"entity","expression":{"op":"category_map","args":[{"op":"column","field":"category"}],"mapping":{"  Gold  ":"premium"},"default":"standard"}}`, `"premium"`},
		{"round exact", "numerical", `{"kind":"expression","entity_field":"entity","expression":{"op":"round","args":[{"op":"literal","value":-1.255},{"op":"literal","value":2}]}}`, `-1.26`},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.Type = test.typ
			f.Definition = []byte(test.definition)
			values, _, err := computeFeatureValues(context.Background(), f, ref, input)
			require.NoError(t, err)
			require.Len(t, values, 2)
			require.Equal(t, "00123", values[0].EntityID)
			require.Equal(t, test.want, string(values[0].Value))
		})
	}
}
func TestFeatureWindowEntityBoundaryAndSameTime(t *testing.T) {
	f, ref, _ := featureEngineInput()
	f.Definition = []byte(`{"kind":"window_aggregate","field":"amount","entity_field":"entity","event_time_field":"event","window_seconds":3600,"aggregate":"sum"}`)
	input := l.Input{Kind: "table", Columns: []string{"entity", "amount", "event"}, Rows: [][]any{{"a", "1.1", "2026-01-01T00:00:00Z"}, {"a", "2.2", "2026-01-01T00:30:00Z"}, {"a", "3.3", "2026-01-01T01:00:00Z"}, {"a", "4.4", "2026-01-01T01:00:00Z"}, {"b", "100", "2026-01-01T01:00:00Z"}}}
	values, _, err := computeFeatureValues(context.Background(), f, ref, input)
	require.NoError(t, err)
	require.Len(t, values, 4)
	require.Equal(t, "1.1", string(values[0].Value))
	require.Equal(t, "3.3", string(values[1].Value))
	require.Equal(t, "9.9", string(values[2].Value))
	require.Equal(t, "100", string(values[3].Value))
	for _, aggregate := range []string{"count", "mean", "min", "max"} {
		var def d.FeatureDefinition
		require.NoError(t, json.Unmarshal(f.Definition, &def))
		def.Aggregate = aggregate
		f.Definition, _ = json.Marshal(def)
		got, _, e := computeFeatureValues(context.Background(), f, ref, input)
		require.NoError(t, e)
		want := map[string]string{"count": "3", "mean": "3.3", "min": "2.2", "max": "4.4"}[aggregate]
		require.Equal(t, want, string(got[2].Value))
	}
}
func TestFeatureEngineRejectsUnsafeAmbiguousOrUnboundedInput(t *testing.T) {
	f, ref, input := featureEngineInput()
	for _, raw := range []string{
		`{"kind":"embedding","field":"category"}`,
		`{"kind":"field","field":"amount","kind":"expression"}`,
		`{"kind":"field","field":"amount","script":"x"}`,
		`{"kind":"expression","expression":{"op":"eval","args":[]}}`,
		`{"kind":"expression","expression":{"op":"div","args":[{"op":"literal","value":1},{"op":"literal","value":0}]}}`,
		`{"kind":"expression","expression":{"op":"column","field":"missing"}}`,
		`{"kind":"expression","expression":{"op":"literal","value":1e999999999}}`,
		`{"kind":"expression","expression":{"op":"literal","value":1e-300}}`,
		`{"Kind":"field","field":"amount"}`,
		`{"kind":"window_aggregate","field":"amount","window_seconds":60,"aggregate":"sum"}`,
	} {
		f.Definition = []byte(raw)
		_, _, err := computeFeatureValues(context.Background(), f, ref, input)
		require.Error(t, err, raw)
		var issue *d.FeatureError
		require.ErrorAs(t, err, &issue, raw)
		require.ErrorIs(t, err, d.ErrInvalid)
	}
	f.Definition = []byte(`{"kind":"field","field":"amount","entity_field":"entity"}`)
	input.Rows[1][0] = "00123"
	_, _, err := computeFeatureValues(context.Background(), f, ref, input)
	require.ErrorContains(t, err, "duplicate_entity_time")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = computeFeatureValues(ctx, f, ref, input)
	require.True(t, errors.Is(err, context.Canceled))
}
func TestFeatureExpressionLimitsAndNullPolicy(t *testing.T) {
	expr := d.FeatureExpression{Op: "column", Field: "amount"}
	for i := 0; i < 10; i++ {
		expr = d.FeatureExpression{Op: "abs", Args: []d.FeatureExpression{expr}}
	}
	raw, _ := json.Marshal(d.FeatureDefinition{Kind: "expression", Expression: &expr})
	_, err := parseFeatureDefinition(raw)
	require.ErrorContains(t, err, "expression_limit")
	f, ref, input := featureEngineInput()
	f.Definition = []byte(`{"kind":"field","field":"amount"}`)
	input.Rows[0][1] = nil
	values, warnings, err := computeFeatureValues(context.Background(), f, ref, input)
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Len(t, warnings, 1)
	input.Rows = make([][]any, 1001)
	_, _, err = computeFeatureValues(context.Background(), f, ref, input)
	require.ErrorContains(t, err, "input_limit")
}

func TestFeatureVectorRejectsTrailingDataAndIdempotencyKeys(t *testing.T) {
	f, ref, input := featureEngineInput()
	f.Type = "vector"
	f.Definition = []byte(`{"kind":"field","field":"category"}`)
	input.Rows[0][2] = "[1,2] garbage"
	input.Rows[1][2] = "[3,4]"
	_, _, err := computeFeatureValues(context.Background(), f, ref, input)
	require.ErrorContains(t, err, "invalid_vector")
	for _, key := range []string{"", "中文", "has space", "newline\n"} {
		require.False(t, validFeatureKey(key), key)
	}
	require.True(t, validFeatureKey("governed:abc-123"))
}
