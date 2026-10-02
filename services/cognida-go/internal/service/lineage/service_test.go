package lineage

import (
	"cognida/internal/model/semantic"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOutputIdentityPreservesActualAssetAndKind(t *testing.T) {
	b := &semantic.ModelBundle{Metrics: []*semantic.Metric{{ID: "metric-id", Name: "value"}}, Dimensions: []*semantic.Dimension{{ID: "dimension-id", Name: "value"}}}
	kind, m := outputIdentity(b, "value", true)
	require.Equal(t, "Metric", kind)
	require.Equal(t, "metric-id", m)
	kind, d := outputIdentity(b, "value", false)
	require.Equal(t, "Dimension", kind)
	require.Equal(t, "dimension-id", d)
	require.NotEqual(t, stable("Metric", "model", m), stable("Dimension", "model", d))
}
