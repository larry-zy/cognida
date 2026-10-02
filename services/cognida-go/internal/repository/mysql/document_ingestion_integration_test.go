//go:build integration

package mysql

import (
	"context"
	"testing"

	"cognida/internal/infrastructure/id"
	"cognida/internal/model/docreader"
	domain "cognida/internal/model/knowledge"
	svc "cognida/internal/service/knowledge"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type ingestionReader struct{}

func (ingestionReader) ParseDocument(_ context.Context, r *docreader.ParseDocumentRequest) (*docreader.ParseDocumentResponse, error) {
	return &docreader.ParseDocumentResponse{Success: true, Text: string(r.Content), Metadata: &docreader.DocumentMetadata{Format: "txt"}}, nil
}
func (ingestionReader) ChunkDocument(_ context.Context, r *docreader.ChunkDocumentRequest) (*docreader.ChunkDocumentResponse, error) {
	return &docreader.ChunkDocumentResponse{Chunks: []*docreader.Chunk{{Text: r.Text, EndPos: len(r.Text)}}, TotalCount: 1}, nil
}

func TestDocumentIngestionReplayAndResumeReuseOneKnowledge(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()
	kbID := uuid.NewString()
	const tenant = int64(9876501)
	kbs := NewKnowledgeBaseRepository(db, true)
	docs := NewKnowledgeRepository(db)
	chunks := NewChunkRepository(db)
	t.Cleanup(func() {
		db.Where("knowledge_base_id = ?", kbID).Delete(&ChunkModel{})
		db.Where("knowledge_base_id = ?", kbID).Delete(&KnowledgeModel{})
		db.Where("id = ?", kbID).Delete(&KnowledgeBaseModel{})
	})
	require.NoError(t, kbs.Create(ctx, &domain.KnowledgeBase{ID: kbID, TenantID: tenant, UserID: 1, Name: "ingestion review"}))
	processor := svc.NewDocumentProcessorService(kbs, nil, docs, chunks, nil, nil, ingestionReader{}, nil, nil, id.NewIDGenerator())
	req := &svc.ProcessDocumentRequest{KnowledgeBaseID: kbID, Title: "stable content", Content: []byte("a durable document"), IdempotencyKey: "stable-content"}
	first, err := processor.ProcessDocument(ctx, tenant, 1, req)
	require.NoError(t, err)
	require.NotEmpty(t, first.KnowledgeID)
	second, err := processor.ProcessDocument(ctx, tenant, 1, req)
	require.NoError(t, err)
	require.Equal(t, first.KnowledgeID, second.KnowledgeID)
	// A process died after writing the document but before marking it complete.
	require.NoError(t, docs.UpdateParseStatus(ctx, first.KnowledgeID, domain.ParseStatusProcessing, ""))
	resumed, err := processor.ProcessDocument(ctx, tenant, 1, req)
	require.NoError(t, err)
	require.Equal(t, first.KnowledgeID, resumed.KnowledgeID)
	var count int64
	require.NoError(t, db.Model(&KnowledgeModel{}).Where("knowledge_base_id = ?", kbID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	kb, err := kbs.FindByIDForTenant(ctx, kbID, tenant)
	require.NoError(t, err)
	require.Equal(t, 1, kb.DocumentCount)
	require.Equal(t, 1, kb.ChunkCount)
}
