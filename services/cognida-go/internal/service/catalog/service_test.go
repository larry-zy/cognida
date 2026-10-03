package catalog

import (
	c "cognida/internal/model/catalog"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type profileRepository struct {
	c.Repository
	getCalls, columnCalls int
	gotIDs                []string
}

func (r *profileRepository) Get(context.Context, int64, string) (*c.AssetTable, error) {
	r.getCalls++
	return &c.AssetTable{ID: "source", DatasourceID: "ds", Columns: []c.AssetColumn{{Name: "id"}, {Name: "amount"}, {Name: "retired", Deprecated: true}}}, nil
}
func (r *profileRepository) List(context.Context, int64, c.Filter) (*c.Page[c.AssetTable], error) {
	return &c.Page[c.AssetTable]{Items: []c.AssetTable{{ID: "source"}, {ID: "one", TableName: "one"}, {ID: "two", TableName: "two"}, {ID: "retired", Deprecated: true}}, HasMore: true}, nil
}
func (r *profileRepository) ActiveColumnNames(_ context.Context, _ int64, ids []string) (map[string][]string, error) {
	r.columnCalls++
	r.gotIDs = ids
	return map[string][]string{"one": {"id"}, "two": {"ID", "amount"}}, nil
}

func TestProfileCardBatchesRelatedMetadataAndPreservesRanking(t *testing.T) {
	repo := &profileRepository{}
	card, err := NewService(repo, nil, nil, nil).ProfileCard(context.Background(), 1, "source")
	require.NoError(t, err)
	require.Equal(t, 1, repo.getCalls, "only load the requested asset's full payload, never per-candidate samples")
	require.Equal(t, 1, repo.columnCalls)
	require.Equal(t, []string{"one", "two"}, repo.gotIDs)
	require.Len(t, card.Related, 2)
	require.Equal(t, "two", card.Related[0].ID)
	require.Contains(t, card.Related[0].Reason, "2 个同名字段")
	require.Contains(t, card.Recommendations, "相关数据候选来自当前数据源前 100 个资产，不代表完整排序。")
}

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
