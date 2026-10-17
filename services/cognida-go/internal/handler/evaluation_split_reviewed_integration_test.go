//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	d "cognida/internal/model/evaluation"
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	ev "cognida/internal/service/evaluation"
	gov "cognida/internal/service/governance"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type splitSourceAssets struct{ sources map[l.RefID]*reviewedAssets }

func (a *splitSourceAssets) Resolve(ctx context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	v := a.sources[id]
	if v == nil {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	return v.Resolve(ctx, tenant, user, id)
}
func (a *splitSourceAssets) ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error) {
	return l.DataAssetRef{}, l.ErrInvalid
}

// Real manual annotation edit/review -> HTTP version publication -> MySQL
// immutable version -> fixed evaluation snapshot. Source imports and identity
// are explicit fixtures; this test does not claim live model/source execution.
func TestReviewedSplitMySQLHTTPRejectsHumanEditedQALeakage(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	sources := &splitSourceAssets{sources: map[l.RefID]*reviewedAssets{}}
	f.gs = gov.NewService(repo.NewGovernanceRepository(f.db), sources, nil, nil, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() })
	f.vs = ev.NewVersionService(repo.NewDatasetVersionRepository(f.db), f.gs, sources, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", f.p.TenantID); c.Set("user_id", f.p.UserID) })
	NewDatasetVersionHandler(f.vs).Register(router.Group("/api"))
	f.router = router
	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := cutoff.Add(time.Hour)
	in := d.PublishDatasetVersion{DatasetID: "review-" + uuid.NewString(), Name: "Human reviewed split", Profile: "qa", Split: d.DatasetSplit{Algorithm: "source_time_v1", TrainBefore: &cutoff, ValidationBefore: &end}}
	for i, text := range []string{"How are monthly invoices issued to enterprise subscribers?", "Which birds inhabit the northern wetlands during winter?"} {
		ref := f.assets.ref
		ref.RefID = l.RefID{AssetID: uuid.NewString(), Version: 1}
		ref.AssetRefID = ref.RefID.String()
		ref.CreatedAt = cutoff.Add(-time.Hour)
		if i == 1 {
			ref.CreatedAt = end.Add(time.Hour)
		}
		f.assets = &reviewedAssets{ref: ref, input: l.Input{Kind: "table", Columns: []string{"q", "a"}, Rows: [][]any{{text, "Original source answer"}}}}
		sources.sources[ref.RefID] = f.assets
		task, item, version := f.approvedQA(t)
		edited, err := f.gs.UpdateAnnotation(ctx, f.p, task, item, g.Annotation{QA: &g.QAPair{Question: "How do I reset my password?", ReferenceAnswer: "Use the settings page"}}, version)
		require.NoError(t, err)
		approved, err := f.gs.ReviewAnnotation(ctx, f.p, task, item, "approved", "human checked", edited.Version)
		require.NoError(t, err)
		in.Samples = append(in.Samples, d.SampleSelection{TaskID: task, ItemID: item, ExpectedVersion: approved.Version})
	}
	status, _ := f.request(t, "POST", "/evaluation/dataset-versions", uuid.NewString(), in)
	require.Equal(t, 400, status)
	var count int64
	require.NoError(t, f.db.Table("evaluation_dataset_versions").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.Zero(t, count)
	// A different answer cannot conceal an identical evaluation question.
	selected := &in.Samples[1]
	edited, err := f.gs.UpdateAnnotation(ctx, f.p, selected.TaskID, selected.ItemID, g.Annotation{QA: &g.QAPair{Question: "How do I reset my password!", ReferenceAnswer: "Another answer"}}, selected.ExpectedVersion)
	require.NoError(t, err)
	approved, err := f.gs.ReviewAnnotation(ctx, f.p, selected.TaskID, selected.ItemID, "approved", "human checked", edited.Version)
	require.NoError(t, err)
	selected.ExpectedVersion = approved.Version
	status, _ = f.request(t, "POST", "/evaluation/dataset-versions", uuid.NewString(), in)
	require.Equal(t, 400, status)
	edited, err = f.gs.UpdateAnnotation(ctx, f.p, selected.TaskID, selected.ItemID, g.Annotation{QA: &g.QAPair{Question: "Which wetland birds migrate in winter?", ReferenceAnswer: "Species in the source"}}, selected.ExpectedVersion)
	require.NoError(t, err)
	approved, err = f.gs.ReviewAnnotation(ctx, f.p, selected.TaskID, selected.ItemID, "approved", "human checked", edited.Version)
	require.NoError(t, err)
	selected.ExpectedVersion = approved.Version
	status, raw := f.request(t, "POST", "/evaluation/dataset-versions", uuid.NewString(), in)
	require.Equal(t, 200, status)
	var version d.DatasetVersion
	require.NoError(t, json.Unmarshal(raw, &version))
	snapshots := ev.NewSnapshotService(f.vs, nil, func(context.Context, l.Principal, *ev.EvaluationTaskConfig) (json.RawMessage, error) {
		return json.RawMessage(`{"model":"fixture"}`), nil
	})
	snapshot, err := snapshots.Freeze(ctx, f.p, &ev.EvaluationTaskConfig{DatasetID: in.DatasetID, Type: ev.EvaluationTypeQA}, version.ID)
	require.NoError(t, err)
	require.Len(t, snapshot.QAPairs, 1)
	require.Equal(t, "Which wetland birds migrate in winter?", snapshot.QAPairs[0].Question)
}
