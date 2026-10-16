//go:build integration

package handler

import (
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strconv"
	"strings"
	"testing"
)

type cleaningQueue struct {
	job.Queue
	items []job.Job
}

func (q *cleaningQueue) Enqueue(_ context.Context, j job.Job) error {
	q.items = append(q.items, j)
	return nil
}

type cleaningFixtureCompute struct{}

func (cleaningFixtureCompute) Call(_ context.Context, op string, in d.Input, _ map[string]any, _ string) (svc.ComputeResult, error) {
	if op == "profile" {
		return svc.ComputeResult{Method: "fixture-quality", Result: map[string]any{"overall_score": 95}}, nil
	}
	out := in
	out.Rows = [][]any{append([]any(nil), in.Rows[1]...)}
	out.Rows[0][0] = strings.TrimSpace(out.Rows[0][0].(string))
	return svc.ComputeResult{Method: "fixture-clean", Result: map[string]any{"data": out, "source_row_indices": []int{1}, "diagnostics": map[string]any{"retention_rate": 0.5, "before": map[string]any{"row_count": 2}, "after": map[string]any{"row_count": 1}, "row_changes": []map[string]any{{"source_row": 0, "operations": []string{"trim", "dedup"}, "removed_by": "dedup"}, {"source_row": 1, "operations": []string{"trim"}, "removed_by": ""}}}}}, nil
}
func TestCleaningReviewMySQLHTTPPreviewImmutableDiffAndBinding(t *testing.T) {
	f := newReviewedFixture(t)
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	q := &cleaningQueue{}
	s, e := svc.NewService(repo.NewLifecycleRepository(f.db), objects, q, svc.Config{Compute: cleaningFixtureCompute{}, Authorize: func(_ context.Context, p d.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	h := NewLifecycleHandler(s)
	group := f.router.Group("/api/lifecycle")
	h.Register(group)
	h.RegisterCleaningReview(group)
	exact := "9007199254740993.123456789012345678"
	input := d.Input{Kind: "table", Columns: []string{"id", "amount"}, ColumnTypes: map[string]string{"id": "VARCHAR", "amount": "DECIMAL"}, Rows: [][]any{{" 00123 ", exact}, {" 00123 ", exact}}}
	source, e := s.ImportWithKey(context.Background(), f.p, input, "source", "source")
	require.NoError(t, e)
	options := map[string]any{"operations": []string{"trim", "dedup"}, "keep": "last"}
	code, raw := f.request(t, "POST", "/lifecycle/cleaning/preview", "", map[string]any{"input_ref": source.RefID, "options": options})
	require.Equal(t, 200, code)
	var preview svc.CleaningPreview
	require.NoError(t, json.Unmarshal(raw, &preview))
	require.False(t, preview.Saved)
	require.Len(t, preview.Differences.Items, 2)
	require.Equal(t, exact, preview.Differences.Items[0].Before[1])
	require.Empty(t, q.items)
	var count int64
	require.NoError(t, f.db.Table("lifecycle_assets").Where("tenant_id=?", f.p.TenantID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	options["preview_digest"] = preview.PlanDigest
	code, raw = f.request(t, "POST", "/lifecycle/invocations", "clean", map[string]any{"agent_id": "data-cleaning", "input_refs": []d.RefID{source.RefID}, "options": options})
	require.Equal(t, 201, code)
	require.Len(t, q.items, 1)
	require.NoError(t, s.Handle(context.Background(), q.items[0]))
	var stage d.StageRun
	require.NoError(t, json.Unmarshal(raw, &stage))
	stagePtr, e := repo.NewLifecycleRepository(f.db).FindStage(context.Background(), f.p.TenantID, stage.ID, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, stagePtr.Status)
	var output d.RefID
	for _, ref := range stagePtr.Output.Refs {
		if ref.Kind == d.CleanedAsset {
			output = ref.RefID
		}
	}
	require.NotEmpty(t, output.AssetID)
	path := "/lifecycle/assets/" + output.AssetID + "/cleaning-diff?version=" + strconv.Itoa(output.Version)
	code, raw = f.request(t, "GET", path, "", nil)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), exact)
	require.Contains(t, string(raw), `"00123"`)
	code, _ = f.request(t, "GET", path, "", nil, "X-Other-Tenant")
	require.Equal(t, 404, code)
	_, raw, e = s.Resolve(context.Background(), f.p.TenantID, f.p.UserID, source.RefID)
	require.NoError(t, e)
	var unchanged d.Input
	require.NoError(t, json.Unmarshal(raw, &unchanged))
	require.Equal(t, input, unchanged)
	options["keep"] = "first"
	code, _ = f.request(t, "POST", "/lifecycle/invocations", "different-plan", map[string]any{"agent_id": "data-cleaning", "input_refs": []d.RefID{source.RefID}, "options": options})
	require.Equal(t, 409, code)
	require.Len(t, q.items, 1)
}
