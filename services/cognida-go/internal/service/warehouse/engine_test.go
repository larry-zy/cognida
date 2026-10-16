package warehouse

import (
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/warehouse"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func sampleDefinition() d.Definition {
	return d.Definition{Name: "订单区域营收", Subject: "销售", Grain: "每区域一行", GrainKeys: []string{"dim_region"}, FactNode: "orders", FactKeys: []string{"order_id"}, EventField: "event", UpdatePolicy: "upsert", DeletePolicy: "unsupported", LatePolicy: "reject_incremental_use_backfill", OutputNode: "region_sales", Nodes: []d.Node{
		{ID: "orders", Layer: "raw", Kind: "source", SourceRef: "00000000-0000-4000-8000-000000000001:1"},
		{ID: "customers", Layer: "raw", Kind: "source", SourceRef: "00000000-0000-4000-8000-000000000002:1"},
		{ID: "detail", Layer: "detail", Kind: "left_join_many_to_one", Inputs: []string{"orders", "customers"}, Join: &d.Join{LeftKeys: []string{"customer"}, RightKeys: []string{"customer"}, RightColumns: []string{"region"}, Prefix: "dim_"}},
		{ID: "region_sales", Layer: "summary", Kind: "aggregate", Inputs: []string{"detail"}, GroupBy: []string{"dim_region"}, Metrics: []d.Metric{{Name: "revenue", Field: "amount", Aggregate: "sum", Caliber: "按原始订单金额精确加总"}, {Name: "orders", Aggregate: "count", Caliber: "订单记录数"}}},
	}}
}
func sampleSources() map[string]l.Input {
	return map[string]l.Input{"orders": {Kind: "table", Columns: []string{"order_id", "customer", "amount", "event"}, ColumnTypes: map[string]string{"amount": "DECIMAL"}, Rows: [][]any{{"01", "A", "9007199254740993.123456789012345678", "2026-01-01T00:00:00Z"}, {"02", "A", "0.1", "2026-01-02T00:00:00Z"}, {"03", "B", "2", "2026-01-02T00:00:00Z"}}}, "customers": {Kind: "table", Columns: []string{"customer", "region"}, Rows: [][]any{{"A", "north"}, {"B", "south"}}}}
}
func TestWarehouseExactJoinAggregateAndActualLineage(t *testing.T) {
	r, e := execute(context.Background(), sampleDefinition(), sampleSources())
	require.NoError(t, e)
	require.Equal(t, 4, len(r.Stages))
	require.NoError(t, r.Output.Validate())
	require.NotContains(t, r.Output.ColumnTypes, "dim_region")
	require.Equal(t, 3, r.Stages[2].OutputRows)
	require.Contains(t, r.Lineage, d.ColumnLineage{Node: "region_sales", Column: "orders", InputNode: "detail", InputColumn: "order_id", Dependency: "row_set"})
	for _, line := range r.Lineage {
		require.NotEmpty(t, line.InputColumn)
	}
	graph := lineageGraph(&d.Run{ID: "run"}, r.Lineage)
	for _, node := range graph.Nodes {
		require.NotEqual(t, "detail.", node.Name)
	}
	require.Equal(t, json.Number("9007199254740993.223456789012345678"), r.Output.Rows[0][1])
	require.Equal(t, json.Number("2"), r.Output.Rows[0][2])
	require.Contains(t, r.Lineage, d.ColumnLineage{Node: "detail", Column: "dim_region", InputNode: "customers", InputColumn: "customer"})
	require.Contains(t, r.Lineage, d.ColumnLineage{Node: "region_sales", Column: "revenue", InputNode: "detail", InputColumn: "amount"})
}
func TestWarehouseRejectsGrainAmplificationAndCycles(t *testing.T) {
	s := sampleSources()
	v := s["customers"]
	v.Rows = append(v.Rows, []any{"A", "other"})
	s["customers"] = v
	_, e := execute(context.Background(), sampleDefinition(), s)
	require.ErrorContains(t, e, "duplicate_grain")
	def := sampleDefinition()
	def.Nodes[2].Inputs[0] = "region_sales"
	_, e = execute(context.Background(), def, sampleSources())
	require.ErrorContains(t, e, "cycle")
	def = sampleDefinition()
	def.Nodes[2].Kind = "sql"
	_, e = validate(def)
	require.ErrorContains(t, e, "unsupported_operator")
}
func TestWarehouseExplicitExactProjection(t *testing.T) {
	def := sampleDefinition()
	def.Nodes = []d.Node{def.Nodes[0], {ID: "detail", Kind: "projection", Layer: "detail", Inputs: []string{"orders"}, Columns: []d.Projection{{Name: "id", Type: "text", Expression: g.FeatureExpression{Op: "column", Field: "order_id"}}, {Name: "double", Type: "number", Expression: g.FeatureExpression{Op: "mul", Args: []g.FeatureExpression{{Op: "column", Field: "amount"}, {Op: "literal", Value: json.RawMessage(`2`)}}}}}}}
	def.OutputNode = "detail"
	def.GrainKeys = []string{"id"}
	r, e := execute(context.Background(), def, sampleSources())
	require.NoError(t, e)
	require.Equal(t, "01", r.Output.Rows[0][0])
	require.Equal(t, json.Number("18014398509481986.246913578024691356"), r.Output.Rows[0][1])
	def.Nodes[1].Columns[0].Expression.Field = "absent"
	_, e = execute(context.Background(), def, sampleSources())
	require.ErrorContains(t, e, "missing_column")
}
func TestWarehouseIncrementalBackfillNeverAdvancesOldHistory(t *testing.T) {
	def := sampleDefinition()
	base := sampleSources()["orders"]
	in := base
	in.Rows = [][]any{{"02", "A", "5.5", "2026-01-03T00:00:00Z"}}
	mark := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	merged, next, e := mergeFacts(def, base, in, d.RunRequest{Mode: "incremental"}, &mark)
	require.NoError(t, e)
	require.Len(t, merged.Rows, 3)
	require.Equal(t, "5.5", merged.Rows[1][2])
	require.True(t, next.After(mark))
	require.Equal(t, "0.1", base.Rows[1][2])
	in.Rows = [][]any{{"02", "A", "0.123456789012345678", "2026-01-01T00:00:00Z"}}
	_, _, e = mergeFacts(def, base, in, d.RunRequest{Mode: "incremental"}, &mark)
	require.ErrorContains(t, e, "late_record")
	from := mark.Add(-24 * time.Hour)
	merged, next, e = mergeFacts(def, base, in, d.RunRequest{Mode: "backfill", EventFrom: &from, EventTo: &mark}, &mark)
	require.NoError(t, e)
	require.Equal(t, mark, *next)
	require.Equal(t, "0.123456789012345678", merged.Rows[1][2])
	require.Equal(t, "0.1", base.Rows[1][2])
	in.Rows[0][3] = mark.Format(time.RFC3339)
	_, _, e = mergeFacts(def, base, in, d.RunRequest{Mode: "backfill", EventFrom: &from, EventTo: &mark}, &mark)
	require.ErrorContains(t, e, "backfill_range")
}

func TestWarehouseNumericDimensionKeysUseExactValueNotSpelling(t *testing.T) {
	def := sampleDefinition()
	sources := sampleSources()
	dims := sources["customers"]
	dims.ColumnTypes = map[string]string{"customer": "DECIMAL(20,2)"}
	dims.Rows = [][]any{{"1", "north"}, {"1.0", "south"}}
	sources["customers"] = dims
	_, e := execute(context.Background(), def, sources)
	require.ErrorContains(t, e, "duplicate_grain")
}

func TestWarehouseBrowserRoundtripPreservesNumericLiteral(t *testing.T) {
	def := sampleDefinition()
	def.Nodes[0].Columns = []d.Projection{{Expression: g.FeatureExpression{Op: "literal", Value: json.RawMessage(`9007199254740993.123456789012345678`)}}}
	normalized, e := canonicalDefinition(def)
	require.NoError(t, e)
	expr := normalized.Nodes[0].Columns[0].Expression
	require.Equal(t, "to_number", expr.Op)
	require.Equal(t, `"9007199254740993.123456789012345678"`, string(expr.Args[0].Value))
	again, e := canonicalDefinition(normalized)
	require.NoError(t, e)
	require.Equal(t, digest(normalized), digest(again))
}
