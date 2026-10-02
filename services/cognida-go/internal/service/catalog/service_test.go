package catalog

import (
	c "cognida/internal/model/catalog"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProfilePreservesNullAndBoundsEvidence(t *testing.T) {
	cols := []c.AssetColumn{{Name: "value"}, {Name: "absent"}}
	applyProfile(cols, []string{"value"}, [][]any{{nil}, {""}, {"x"}, {"x"}})
	require.True(t, cols[0].Profiled)
	require.Equal(t, .25, *cols[0].NullRate)
	require.Equal(t, int64(2), *cols[0].DistinctCount)
	require.Equal(t, 4, cols[0].SampleSize)
	require.False(t, cols[1].Profiled)
	require.Nil(t, cols[1].NullRate)
}

func TestProfileCardDoesNotInventQuality(t *testing.T) {
	missing := describeAsset(c.AssetTable{ID: "a", Columns: []c.AssetColumn{{Name: "email"}}})
	require.Nil(t, missing.QualityScore)
	require.NotEmpty(t, missing.Risks)
	rate := .25
	measured := describeAsset(c.AssetTable{ID: "a", Columns: []c.AssetColumn{{Name: "amount", Profiled: true, SampleSize: 4, NullRate: &rate}, {Name: "empty", Profiled: true, SampleSize: 0, NullRate: &rate}}})
	require.Equal(t, 75.0, *measured.QualityScore)
	require.Equal(t, 1, measured.ProfiledColumns)
	require.Len(t, measured.Risks, 1)
}
