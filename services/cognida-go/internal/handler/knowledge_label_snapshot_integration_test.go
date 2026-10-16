//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"testing"

	g "cognida/internal/model/governance"
	repo "cognida/internal/repository/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Lifecycle provenance is verified separately by the publication resolver.
// This test exercises the final atomic MySQL guard with trusted prepared input.
func TestPublishedKnowledgeLabelSnapshotMySQL(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	kbID, docID := uuid.NewString(), uuid.NewString()
	content := "original technical support documentation"
	chunkContent := "original technical\ntechnical support documentation\n"
	sourceDigest, chunkDigest := g.LabelContentDigest(content), g.LabelContentDigest(chunkContent)
	require.NoError(t, f.db.Create(&repo.KnowledgeBaseModel{ID: kbID, TenantID: f.p.TenantID, UserID: f.p.UserID, Name: "publication target", Status: 1}).Error)
	require.NoError(t, f.db.Create(&repo.KnowledgeModel{ID: docID, TenantID: f.p.TenantID, UserID: f.p.UserID, KnowledgeBaseID: kbID, Type: "text", Title: "support", EnableStatus: "enabled", ParseStatus: "completed", FileHash: sourceDigest, IngestedChunkDigest: chunkDigest}).Error)
	for index, text := range []string{"original technical", "technical support documentation"} {
		require.NoError(t, f.db.Create(&repo.ChunkModel{ID: uuid.NewString(), TenantID: f.p.TenantID, KnowledgeID: docID, KnowledgeBaseID: kbID, ChunkIndex: index, Content: text, IsEnabled: true}).Error)
	}
	payload, err := json.Marshal(g.TaggingResult{Content: content, Tags: []string{"support"}, ReviewStatus: "approved", ReviewedBy: f.p.UserID})
	require.NoError(t, err)
	record, err := f.gs.Repo.Create(ctx, &g.Record{Meta: g.Meta{TenantID: f.p.TenantID, UserID: f.p.UserID}, Kind: "tagging_result", Status: "approved", Payload: payload})
	require.NoError(t, err)
	// Match the service contract: review evidence comes from the persisted JSON
	// representation, which MySQL canonicalizes when it stores a JSON column.
	record, err = f.gs.Repo.Get(ctx, f.p.TenantID, "tagging_result", record.ID)
	require.NoError(t, err)
	prepared := g.PreparedKnowledgeLabels{
		ApplyKnowledgeLabels: g.ApplyKnowledgeLabels{ResultID: record.ID, ExpectedVersion: record.Version, KnowledgeID: docID, PublicationRefID: uuid.NewString() + ":1"},
		SourceRefID:          f.assets.ref.AssetRefID, Content: content, SchemeDigest: g.LabelContentDigest("standard"), ReviewDigest: g.LabelContentDigest(string(record.Payload)), IdempotencyKey: "apply-publication",
		PublicationContentDigest: sourceDigest, PublicationChunkDigest: chunkDigest, Labels: []string{"support"},
	}
	labels := repo.NewKnowledgeLabelRepository(f.db)
	for _, change := range []func(*g.PreparedKnowledgeLabels){
		func(in *g.PreparedKnowledgeLabels) { in.PublicationContentDigest = "" },
		func(in *g.PreparedKnowledgeLabels) { in.PublicationChunkDigest = "" },
		func(in *g.PreparedKnowledgeLabels) { in.Content += " modified" },
		func(in *g.PreparedKnowledgeLabels) {
			in.PublicationChunkDigest = g.LabelContentDigest("wrong snapshot")
		},
		func(in *g.PreparedKnowledgeLabels) { in.PublicationRefID = "" },
	} {
		input := prepared
		change(&input)
		_, err = labels.ApplyApproved(ctx, f.p, input)
		require.ErrorIs(t, err, g.ErrConflict)
	}
	for field, wrong := range map[string]string{"file_hash": g.LabelContentDigest("changed source"), "ingested_chunk_digest": "", "parse_status": "processing"} {
		original := map[string]string{"file_hash": sourceDigest, "ingested_chunk_digest": chunkDigest, "parse_status": "completed"}[field]
		require.NoError(t, f.db.Table("knowledge").Where("id=?", docID).Update(field, wrong).Error)
		_, err = labels.ApplyApproved(ctx, f.p, prepared)
		require.ErrorIs(t, err, g.ErrConflict)
		require.NoError(t, f.db.Table("knowledge").Where("id=?", docID).Update(field, original).Error)
	}
	application, err := labels.ApplyApproved(ctx, f.p, prepared)
	require.NoError(t, err)
	require.Equal(t, chunkDigest, application.ContentDigest, "receipt binds the actual published chunks, including overlap")
	replay, err := labels.ApplyApproved(ctx, f.p, prepared)
	require.NoError(t, err)
	require.Equal(t, application.ID, replay.ID)
	active, err := labels.Labels(ctx, f.p, docID)
	require.NoError(t, err)
	require.Equal(t, []string{"support"}, active)
	require.NoError(t, f.db.Table("chunks").Where("knowledge_id=? AND chunk_index=0", docID).Update("content", "modified after publication").Error)
	_, err = labels.ApplyApproved(ctx, f.p, prepared)
	require.ErrorIs(t, err, g.ErrConflict, "even an idempotent replay must recheck the live document")
	active, err = labels.Labels(ctx, f.p, docID)
	require.NoError(t, err)
	require.Empty(t, active, "changed chunks deactivate previously applied labels")
}
