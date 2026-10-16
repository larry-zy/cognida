//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// The asset graph is a fixed fixture. Knowledge permissions/current bytes,
// governance records, review, label transactions, and HTTP are real MySQL/API.
// Browser acceptance separately executes the actual clean/prepare stages.
type knowledgeParentAssets struct {
	f      *reviewedFixture
	refs   map[string]l.DataAssetRef
	inputs map[string]l.Input
}

func (a *knowledgeParentAssets) Resolve(ctx context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if a.f.assets.deny || tenant != a.f.p.TenantID || user != a.f.p.UserID {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	ref, ok := a.refs[id.String()]
	if !ok {
		return l.DataAssetRef{}, nil, l.ErrNotFound
	}
	input := a.inputs[id.String()]
	if ref.Provenance.SourceType == "knowledge" {
		var count int64
		err := a.f.db.WithContext(ctx).Table("knowledge k").Joins("JOIN knowledge_bases b ON b.id=k.knowledge_base_id AND b.tenant_id=k.tenant_id").Where("k.tenant_id=? AND k.id=? AND k.deleted_at IS NULL AND k.enable_status='enabled' AND b.deleted_at IS NULL AND b.status=1 AND (b.is_public=1 OR b.user_id=?)", tenant, ref.Provenance.SourceID, user).Count(&count).Error
		if err != nil {
			return l.DataAssetRef{}, nil, err
		}
		if count != 1 {
			return l.DataAssetRef{}, nil, l.ErrForbidden
		}
		var chunks []repo.ChunkModel
		if err = a.f.db.WithContext(ctx).Where("tenant_id=? AND knowledge_id=? AND deleted_at IS NULL AND is_enabled=1", tenant, ref.Provenance.SourceID).Order("chunk_index,id").Find(&chunks).Error; err != nil {
			return l.DataAssetRef{}, nil, err
		}
		var content strings.Builder
		for _, chunk := range chunks {
			content.WriteString(chunk.Content)
			content.WriteByte('\n')
		}
		input = l.Input{Kind: "text", Text: content.String()}
	}
	raw, err := json.Marshal(input)
	return ref, raw, err
}
func (*knowledgeParentAssets) ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error) {
	return l.DataAssetRef{}, l.ErrInvalid
}

func TestKnowledgeLabelsPreparedParentsMySQLHTTP(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	kbID, docID, secondDoc := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, f.db.Create(&repo.KnowledgeBaseModel{ID: kbID, TenantID: f.p.TenantID, UserID: f.p.UserID, Name: "prepared target", Status: 1}).Error)
	for _, id := range []string{docID, secondDoc} {
		require.NoError(t, f.db.Create(&repo.KnowledgeModel{ID: id, TenantID: f.p.TenantID, UserID: f.p.UserID, KnowledgeBaseID: kbID, Type: "text", Title: "knowledge source", EnableStatus: "enabled", ParseStatus: "completed"}).Error)
		require.NoError(t, f.db.Create(&repo.ChunkModel{ID: uuid.NewString(), TenantID: f.p.TenantID, KnowledgeID: id, KnowledgeBaseID: kbID, Content: "sales facts", IsEnabled: true}).Error)
	}
	graph := &knowledgeParentAssets{f: f, refs: map[string]l.DataAssetRef{}, inputs: map[string]l.Input{}}
	add := func(source, doc string, parents ...l.RefID) l.RefID {
		id := l.RefID{AssetID: uuid.NewString(), Version: 1}
		graph.refs[id.String()] = l.DataAssetRef{RefID: id, AssetRefID: id.String(), TenantID: f.p.TenantID, Kind: l.CleanedAsset, Parents: parents, Provenance: l.Provenance{SourceType: source, SourceID: doc}}
		graph.inputs[id.String()] = l.Input{Kind: "text", Text: "sales facts\n"}
		return id
	}
	original := add("knowledge", docID)
	cleaned := add("lifecycle", "", original)
	prepared := f.assets.ref
	prepared.Parents = []l.RefID{cleaned}
	prepared.Provenance.SourceType = "lifecycle"
	prepared.Provenance.Preparation.SourceRefID = cleaned.String()
	graph.refs[prepared.AssetRefID] = prepared
	graph.inputs[prepared.AssetRefID] = l.Input{Kind: "text", Text: "sales facts\n"}
	f.gs.Assets = graph
	standard := f.standard(t, "tagging", []d.Label{{ID: "sales", Name: "sales", Keywords: []string{"sales"}}})
	definition, err := f.gs.BindTaggingStandard(ctx, f.p, d.TaggingTask{Name: "prepared labels", SourceRefID: prepared.AssetRefID, StandardVersionID: standard, Config: d.TaggingConfig{EnableTags: true}})
	require.NoError(t, err)
	payload, err := json.Marshal(definition)
	require.NoError(t, err)
	task, err := f.gs.Repo.Create(ctx, &d.Record{Meta: d.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "tagging_task", Status: "completed", Payload: payload})
	require.NoError(t, err)
	payload, err = json.Marshal(d.TaggingResult{TaskID: task.ID, Content: "sales facts\n", Tags: []string{"sales"}, ReviewStatus: "pending"})
	require.NoError(t, err)
	result, err := f.gs.Repo.Create(ctx, &d.Record{Meta: d.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "tagging_result", ParentID: task.ID, Status: "pending", Payload: payload})
	require.NoError(t, err)
	path := "/tagging-results/" + result.ID
	code, _ := f.request(t, "GET", path+"/apply-target?expected_version=1", "", nil)
	require.Equal(t, 409, code)
	approved, err := f.gs.ReviewTagging(ctx, f.p, result.ID, "approved", "verified against original knowledge", result.Version)
	require.NoError(t, err)
	targetPath := path + fmt.Sprintf("/apply-target?expected_version=%d", approved.Version)
	code, raw := f.request(t, "GET", targetPath, "", nil)
	require.Equal(t, 200, code)
	var target d.KnowledgeLabelTarget
	require.NoError(t, json.Unmarshal(raw, &target))
	require.Equal(t, docID, target.KnowledgeID)
	require.Equal(t, result.ID, target.ResultID)
	require.Equal(t, prepared.AssetRefID, target.SourceRefID)
	require.Equal(t, d.LabelContentDigest("sales facts\n"), target.ContentDigest)
	require.NotContains(t, string(raw), "sales facts")
	code, _ = f.request(t, "GET", targetPath, "", nil, "X-Other-User")
	require.Equal(t, 403, code)
	in := d.ApplyKnowledgeLabels{KnowledgeID: secondDoc, ExpectedVersion: approved.Version}
	code, _ = f.request(t, "POST", path+"/apply", "wrong-target", in)
	require.Equal(t, 403, code)
	in.KnowledgeID = docID
	code, raw = f.request(t, "POST", path+"/apply", "prepared-apply", in)
	require.Equal(t, 200, code)
	var receipt d.KnowledgeLabelApplication
	require.NoError(t, json.Unmarshal(raw, &receipt))
	require.Equal(t, prepared.AssetRefID, receipt.SourceRefID)
	code, raw = f.request(t, "POST", path+"/apply", "prepared-apply", in)
	require.Equal(t, 200, code)
	require.Contains(t, string(raw), receipt.ID)
	labels, err := f.gs.KnowledgeLabels(ctx, f.p, docID)
	require.NoError(t, err)
	require.Equal(t, []string{"sales"}, labels)

	checkRejected := func(key string) {
		code, _ = f.request(t, "GET", targetPath, "", nil)
		require.Equal(t, 403, code, key)
		code, _ = f.request(t, "POST", path+"/apply", key, in)
		require.Equal(t, 403, code, key)
	}
	second := add("knowledge", secondDoc)
	changed := prepared
	changed.Parents = []l.RefID{cleaned, second}
	graph.refs[prepared.AssetRefID] = changed
	checkRejected("ambiguous")
	graph.refs[prepared.AssetRefID] = prepared
	graph.inputs[prepared.AssetRefID] = l.Input{Kind: "text", Text: "sales facts"} // Even newline changes are not relabelled against the original.
	checkRejected("changed-prepared")
	graph.inputs[prepared.AssetRefID] = l.Input{Kind: "text", Text: "sales facts\n"}
	require.NoError(t, f.db.Table("knowledge_bases").Where("tenant_id=? AND id=?", f.p.TenantID, kbID).Update("status", 0).Error)
	checkRejected("revoked")
	require.NoError(t, f.db.Table("knowledge_bases").Where("tenant_id=? AND id=?", f.p.TenantID, kbID).Update("status", 1).Error)
	require.NoError(t, f.db.Table("chunks").Where("tenant_id=? AND knowledge_id=?", f.p.TenantID, docID).Update("content", "modified knowledge").Error)
	checkRejected("changed-original")
	labels, err = f.gs.KnowledgeLabels(ctx, f.p, docID)
	require.NoError(t, err)
	require.Empty(t, labels)
	var receipts int64
	require.NoError(t, f.db.Table("knowledge_label_applications").Where("tenant_id=?", f.p.TenantID).Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)
}
