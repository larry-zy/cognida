package metricsql

import (
	"cognida/internal/model/semantic"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCompiledColumnDependenciesAndObserver(t *testing.T) {
	b := &semantic.ModelBundle{Model: &semantic.SemanticModel{ID: "m", TenantID: 1}, LogicalTables: []*semantic.LogicalTable{{ID: "orders", Name: "orders", PhysicalTable: "orders"}}, Measures: []*semantic.Measure{{Name: "Revenue", LogicalTableID: "orders", Expr: "amount", Aggregation: semantic.AggSum}}, Metrics: []*semantic.Metric{{Name: "GMV", Expr: "{Revenue}"}}, Dimensions: []*semantic.Dimension{{Name: "region", LogicalTableID: "orders", Expr: "region"}}}
	observed := make(chan *Result, 1)
	result, err := Build(b, Query{Metrics: []string{"GMV"}, Dimensions: []string{"region"}}, func(copy *semantic.ModelBundle, _ Query, r *Result) { copy.Model.ID = "mutated"; observed <- r })
	require.NoError(t, err)
	require.Contains(t, result.Dependencies, Dependency{"GMV", "orders", "amount"})
	require.Contains(t, result.Dependencies, Dependency{"GMV", "orders", "region"})
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("observer not called")
	}
	require.Equal(t, "m", b.Model.ID)
}

func TestTableDependencyAfterColumnMetric(t *testing.T) {
	b := &semantic.ModelBundle{Model: &semantic.SemanticModel{ID: "m", TenantID: 1}, LogicalTables: []*semantic.LogicalTable{{ID: "orders", Name: "orders", PhysicalTable: "orders"}}}
	deps := physicalDependencies(newIndex(b), []selectItem{{alias: "total", expr: "SUM(orders.amount)", tableID: "orders"}, {alias: "rows", expr: "COUNT(*)", tableID: "orders"}}, nil, nil, "")
	require.Contains(t, deps, Dependency{"rows", "orders", ""})
}
