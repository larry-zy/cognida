//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/governance"
	k "cognida/internal/model/knowledge"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	life "cognida/internal/service/lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type publicationQueue struct {
	job.Queue
	jobs []job.Job
}

func (q *publicationQueue) Enqueue(_ context.Context, j job.Job) error {
	q.jobs = append(q.jobs, j)
	return nil
}
func (q *publicationQueue) drain(t *testing.T, run func(context.Context, job.Job) error) {
	t.Helper()
	for i := 0; len(q.jobs) > 0; i++ {
		require.Less(t, i, 20)
		j := q.jobs[0]
		q.jobs = q.jobs[1:]
		require.NoError(t, run(context.Background(), j))
	}
}

type publicationCompute struct{}

func (publicationCompute) Call(_ context.Context, op string, in l.Input, _ map[string]any, _ string) (life.ComputeResult, error) {
	result := map[string]any{}
	switch op {
	case "clean":
		result["data"] = in
	case "profile":
		result["overall_score"] = 100
	case "enrich":
	}
	return life.ComputeResult{SchemaVersion: "1", Method: "deterministic publication fixture", Result: result}, nil
}

// Actual MySQL, object store, lifecycle approvals/worker, governance rules/review
// and HTTP. The publisher fixture writes real knowledge/chunks and returns their
// digests; remote parsing/Milvus are deliberately outside this integration test.
func TestKnowledgeLabelsApprovedPublicationMySQLHTTP(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	q := &publicationQueue{}
	store, err := objects.NewLocal(t.TempDir())
	require.NoError(t, err)
	lr := repo.NewLifecycleRepository(f.db)
	var denied atomic.Bool
	kbID := uuid.NewString()
	require.NoError(t, f.db.Create(&repo.KnowledgeBaseModel{ID: kbID, TenantID: f.p.TenantID, UserID: f.p.UserID, Name: "publication bridge", Status: 1}).Error)
	lifecycle, err := life.NewService(lr, store, q, life.Config{Compute: publicationCompute{}, Authorize: func(_ context.Context, p l.Principal, _ string) error {
		if denied.Load() || p != f.p {
			return l.ErrForbidden
		}
		return nil
	}, Publish: func(ctx context.Context, p l.Principal, ref l.DataAssetRef, raw json.RawMessage, options map[string]any) (map[string]any, error) {
		var input l.Input
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		id := uuid.NewString()
		contentDigest := d.LabelContentDigest(input.Text)
		// One exact chunk here intentionally differs from the historical newline
		// reconstruction. Production uses the ingestion snapshot to bind it safely.
		chunkDigest := k.IngestionChunkDigest([]*k.Chunk{{Content: input.Text, IsEnabled: true}})
		doc := repo.KnowledgeModel{ID: id, TenantID: p.TenantID, UserID: p.UserID, KnowledgeBaseID: kbID, Type: "text", Title: "published support", EnableStatus: "enabled", ParseStatus: "completed", FileHash: contentDigest, IngestedChunkDigest: chunkDigest}
		if err := f.db.WithContext(ctx).Create(&doc).Error; err != nil {
			return nil, err
		}
		if err := f.db.WithContext(ctx).Create(&repo.ChunkModel{ID: uuid.NewString(), TenantID: p.TenantID, KnowledgeID: id, KnowledgeBaseID: kbID, Content: input.Text, IsEnabled: true}).Error; err != nil {
			return nil, err
		}
		return map[string]any{"publication_status": "published", "knowledge_id": id, "knowledge_base_id": kbID, "content_digest": contentDigest, "chunk_digest": chunkDigest}, nil
	}})
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	f.gs.Assets = lifecycle
	f.gs.Compute = publicationCompute{}
	f.gs.Queue = q
	NewLifecycleHandler(lifecycle).Register(f.router.Group("/api/lifecycle"))
	source, err := lifecycle.ImportWithKey(ctx, f.p, l.Input{Kind: "text", Text: "sales facts without a synthetic newline"}, "support", "publication-root")
	require.NoError(t, err)
	clean, err := lifecycle.Invoke(ctx, f.p, "data-cleaning", []l.RefID{source.RefID}, nil, "publication-clean", "")
	require.NoError(t, err)
	q.drain(t, lifecycle.Handle)
	clean, err = lr.FindStage(ctx, f.p.TenantID, clean.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, clean.Status)
	prepared, err := lifecycle.PrepareAsset(ctx, f.p, clean.Output.Refs[0].RefID, l.PreparationOptions{}, "publication-prepare", "")
	require.NoError(t, err)
	q.drain(t, lifecycle.Handle)
	prepared, err = lr.FindStage(ctx, f.p.TenantID, prepared.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, prepared.Status)
	preparedRef := prepared.Output.Refs[0]
	standard := f.standard(t, "tagging", []d.Label{{ID: "sales", Name: "sales", Keywords: []string{"sales"}}})
	task, err := f.gs.CreateTagging(ctx, f.p, d.TaggingTask{Name: "support tags", SourceRefID: preparedRef.AssetRefID, StandardVersionID: standard, Config: d.TaggingConfig{EnableTags: true}}, "publication-tagging")
	require.NoError(t, err)
	q.drain(t, f.gs.Handle)
	results, err := f.gs.Repo.List(ctx, f.p.TenantID, d.Filter{Kind: "tagging_result", ParentID: task.ID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, results.Items, 1)
	result := results.Items[0]
	approved, err := f.gs.ReviewTagging(ctx, f.p, result.ID, "approved", "human reviewed source", result.Version)
	require.NoError(t, err)
	path := "/tagging-results/" + result.ID
	targetPath := path + fmt.Sprintf("/apply-target?expected_version=%d", approved.Version)
	status, _ := f.request(t, "GET", targetPath, "", nil)
	require.Equal(t, 403, status, "raw/prepared has no legacy knowledge ancestor")
	publication, err := lifecycle.Invoke(ctx, f.p, "data-asset", []l.RefID{preparedRef.RefID}, map[string]any{"knowledge_base_id": kbID}, "publication-publish", "")
	require.NoError(t, err)
	q.drain(t, lifecycle.Handle)
	publication, err = lr.FindStage(ctx, f.p.TenantID, publication.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusApproval, publication.Status)
	var docs int64
	require.NoError(t, f.db.Table("knowledge").Where("tenant_id=?", f.p.TenantID).Count(&docs).Error)
	require.Zero(t, docs)
	approvalID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("approval:"+publication.ID)).String()
	status, _ = f.request(t, "POST", "/lifecycle/approvals/"+approvalID+"/resolve", "", map[string]any{"decision": "approved", "reason": "human publication approval"})
	require.Equal(t, 200, status)
	q.drain(t, lifecycle.Handle)
	publication, err = lr.FindStage(ctx, f.p.TenantID, publication.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, publication.Status)
	published := publication.Output.Refs[0]
	targetPath += "&publication_ref_id=" + published.AssetRefID
	status, raw := f.request(t, "GET", targetPath, "", nil)
	require.Equal(t, 200, status)
	var target d.KnowledgeLabelTarget
	require.NoError(t, json.Unmarshal(raw, &target))
	require.Equal(t, published.AssetRefID, target.PublicationRefID)
	in := d.ApplyKnowledgeLabels{ExpectedVersion: approved.Version, KnowledgeID: target.KnowledgeID, PublicationRefID: published.AssetRefID}
	status, raw = f.request(t, "POST", path+"/apply", "publication-apply", in)
	require.Equal(t, 200, status)
	var receipt d.KnowledgeLabelApplication
	require.NoError(t, json.Unmarshal(raw, &receipt))
	require.Equal(t, []string{"sales"}, receipt.Labels)
	status, raw = f.request(t, "POST", path+"/apply", "publication-apply", in)
	require.Equal(t, 200, status)
	require.Contains(t, string(raw), receipt.ID)
	for _, header := range []string{"X-Other-User", "X-Other-Tenant"} {
		status, _ = f.request(t, "GET", targetPath, "", nil, header)
		require.Contains(t, []int{403, 404}, status)
	}
	denied.Store(true)
	status, _ = f.request(t, "POST", path+"/apply", "revoked-publication", in)
	require.Equal(t, 403, status)
	denied.Store(false)
	// Approval and execution receipts are rechecked even for an existing label receipt.
	approval, err := lr.FindApproval(ctx, f.p.TenantID, approvalID, false)
	require.NoError(t, err)
	approval.Status = "rejected"
	require.NoError(t, lr.UpdateApproval(ctx, approval))
	status, _ = f.request(t, "GET", targetPath, "", nil)
	require.Equal(t, 403, status)
	approval.Status = "approved"
	require.NoError(t, lr.UpdateApproval(ctx, approval))
	// Another genuine publication of identical bytes has a different ancestry.
	other, err := lifecycle.ImportWithKey(ctx, f.p, l.Input{Kind: "text", Text: "sales facts without a synthetic newline"}, "other", "publication-other")
	require.NoError(t, err)
	otherStage, err := lifecycle.Invoke(ctx, f.p, "data-asset", []l.RefID{other.RefID}, map[string]any{"knowledge_base_id": kbID}, "publication-other-publish", "")
	require.NoError(t, err)
	q.drain(t, lifecycle.Handle)
	otherApproval := uuid.NewSHA1(uuid.NameSpaceOID, []byte("approval:"+otherStage.ID)).String()
	require.NoError(t, lifecycle.ResolveApproval(ctx, f.p, otherApproval, "approved", "other source"))
	q.drain(t, lifecycle.Handle)
	otherStage, err = lr.FindStage(ctx, f.p.TenantID, otherStage.ID, false)
	require.NoError(t, err)
	require.Equal(t, l.StatusSucceeded, otherStage.Status)
	in.PublicationRefID = otherStage.Output.Refs[0].AssetRefID
	status, _ = f.request(t, "POST", path+"/apply", "same-text-wrong-origin", in)
	require.Equal(t, 403, status)
	in.PublicationRefID = published.AssetRefID
	require.NoError(t, f.db.Table("chunks").Where("tenant_id=? AND knowledge_id=?", f.p.TenantID, target.KnowledgeID).Update("content", "changed after publication").Error)
	status, _ = f.request(t, "POST", path+"/apply", "changed-chunks", in)
	require.Equal(t, 409, status)
	var receipts int64
	require.NoError(t, f.db.Table("knowledge_label_applications").Where("tenant_id=?", f.p.TenantID).Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)
}
