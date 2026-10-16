//go:build integration

package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"cognida/internal/infrastructure/id"
	"cognida/internal/model/docreader"
	d "cognida/internal/model/knowledge"
	svc "cognida/internal/service/knowledge"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	dbmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ingestionEvidenceDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	config, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(config.DBName, "cognida_review_"), "explicit isolated review DB required")
	db, err := gorm.Open(dbmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	var version uint
	var dirty bool
	require.NoError(t, sqlDB.QueryRow("SELECT version,dirty FROM schema_migrations").Scan(&version, &dirty))
	require.GreaterOrEqual(t, version, uint(39))
	require.False(t, dirty)
	return db
}

type evidenceOverlapReader struct{ calls int }

func (r *evidenceOverlapReader) ParseDocument(_ context.Context, in *docreader.ParseDocumentRequest) (*docreader.ParseDocumentResponse, error) {
	r.calls++
	text := string(in.Content)
	if text == "" {
		text = "fresh URL source"
	}
	return &docreader.ParseDocumentResponse{Success: true, Text: text, Metadata: &docreader.DocumentMetadata{Format: "txt"}}, nil
}
func (*evidenceOverlapReader) ChunkDocument(_ context.Context, in *docreader.ChunkDocumentRequest) (*docreader.ChunkDocumentResponse, error) {
	chunks := []*docreader.Chunk{{Text: in.Text, Index: 0}}
	if in.Text == "alpha overlap omega" {
		chunks = []*docreader.Chunk{{Text: "overlap omega", Index: 1}, {Text: "alpha overlap", Index: 0}}
	}
	return &docreader.ChunkDocumentResponse{Chunks: chunks, TotalCount: len(chunks)}, nil
}
func evidenceHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func TestKnowledgeIngestionMySQLHistoricalEvidenceReplayAndReprocess(t *testing.T) {
	db := ingestionEvidenceDB(t)
	ctx := context.Background()
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	kbID := uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, db.Where("tenant_id=? AND knowledge_base_id=?", tenant, kbID).Delete(&ChunkModel{}).Error)
		require.NoError(t, db.Where("tenant_id=? AND knowledge_base_id=?", tenant, kbID).Delete(&KnowledgeModel{}).Error)
		require.NoError(t, db.Where("tenant_id=? AND id=?", tenant, kbID).Delete(&KnowledgeBaseModel{}).Error)
	})
	kbs := NewKnowledgeBaseRepository(db, true)
	docs := NewKnowledgeRepository(db)
	chunks := NewChunkRepository(db)
	require.NoError(t, kbs.Create(ctx, &d.KnowledgeBase{ID: kbID, TenantID: tenant, UserID: 42, Name: "evidence fixture", Status: 1}))
	reader := &evidenceOverlapReader{}
	processor := svc.NewDocumentProcessorService(kbs, nil, docs, chunks, nil, nil, reader, nil, nil, id.NewIDGenerator())
	req := &svc.ProcessDocumentRequest{KnowledgeBaseID: kbID, Title: "overlap evidence", Content: []byte("alpha overlap omega"), IdempotencyKey: uuid.NewString()}
	first, err := processor.ProcessDocument(ctx, tenant, 42, req)
	require.NoError(t, err)
	require.Equal(t, evidenceHash(string(req.Content)), first.ContentDigest)
	require.Equal(t, evidenceHash("alpha overlap\noverlap omega\n"), first.ChunkDigest)
	require.NotEqual(t, first.ContentDigest, first.ChunkDigest)
	stored, err := docs.FindByID(ctx, first.KnowledgeID)
	require.NoError(t, err)
	require.Equal(t, first.ChunkDigest, stored.IngestedChunkDigest)
	content, err := knowledgeLabelContent(db, tenant, first.KnowledgeID)
	require.NoError(t, err)
	require.Equal(t, first.ChunkDigest, evidenceHash(content), "same exact label content serialization")
	stale := *stored
	stale.IngestedChunkDigest = ""
	stale.Title = "legitimate metadata edit"
	require.NoError(t, docs.Update(ctx, &stale))
	stored, err = docs.FindByID(ctx, first.KnowledgeID)
	require.NoError(t, err)
	require.Equal(t, first.ChunkDigest, stored.IngestedChunkDigest)
	require.NoError(t, db.Model(&ChunkModel{}).Where("tenant_id=? AND knowledge_id=? AND chunk_index=0", tenant, first.KnowledgeID).Update("content", "modified by editor").Error)
	replayed, err := processor.ProcessDocument(ctx, tenant, 42, req)
	require.NoError(t, err)
	require.Equal(t, first.ChunkDigest, replayed.ChunkDigest, "completed replay cannot bless later chunk edits")
	require.Equal(t, 1, reader.calls, "completed replay does not parse or chunk")
	content, err = knowledgeLabelContent(db, tenant, first.KnowledgeID)
	require.NoError(t, err)
	require.NotEqual(t, replayed.ChunkDigest, evidenceHash(content))
	_, err = processor.ProcessDocument(ctx, tenant, 43, req)
	require.Error(t, err)
	// An explicit same-owner reprocess creates a new binding, not old evidence.
	reprocess := *req
	reprocess.IdempotencyKey = ""
	reprocess.DocumentID = first.KnowledgeID
	reprocess.Content = []byte("new document")
	updated, err := processor.ProcessDocument(ctx, tenant, 42, &reprocess)
	require.NoError(t, err)
	require.Equal(t, evidenceHash("new document"), updated.ContentDigest)
	require.Equal(t, evidenceHash("new document\n"), updated.ChunkDigest)
	_, err = processor.ProcessDocument(ctx, tenant, 42, req)
	require.Error(t, err, "old content/key cannot replay the new ingestion")
	// A URL parse without the original downloaded bytes cannot inherit an old hash.
	reprocess.Content = nil
	reprocess.URL = "https://example.invalid/fixture"
	urlResult, err := processor.ProcessDocument(ctx, tenant, 42, &reprocess)
	require.NoError(t, err)
	require.Empty(t, urlResult.ContentDigest)
	require.Empty(t, urlResult.ChunkDigest)
	stored, err = docs.FindByID(ctx, first.KnowledgeID)
	require.NoError(t, err)
	require.Equal(t, evidenceHash("new document"), stored.FileHash, "file metadata is not erased")
	require.Empty(t, stored.IngestedChunkDigest, "unverified downloaded bytes cannot inherit prior evidence")
	// Ordinary upload first persists the file's hash, then passes FilePath and
	// DocumentID to the processor. The reader fixture avoids opening host files.
	uploadID := uuid.NewString()
	uploadHash := evidenceHash("original uploaded file bytes")
	require.NoError(t, docs.Create(ctx, &d.Knowledge{ID: uploadID, TenantID: tenant, UserID: 42, KnowledgeBaseID: kbID, Type: "text", Title: "uploaded document", Source: "upload", FileHash: uploadHash, ParseStatus: d.ParseStatusProcessing, EnableStatus: d.EnableStatusEnabled}))
	uploadResult, err := processor.ProcessDocument(ctx, tenant, 42, &svc.ProcessDocumentRequest{KnowledgeBaseID: kbID, DocumentID: uploadID, Title: "uploaded document", FilePath: "/synthetic-upload-reader-fixture.txt", FileType: "txt"})
	require.NoError(t, err)
	require.Equal(t, d.ParseStatusCompleted, uploadResult.ParseStatus)
	require.Empty(t, uploadResult.ContentDigest)
	require.Empty(t, uploadResult.ChunkDigest)
	uploaded, err := docs.FindByID(ctx, uploadID)
	require.NoError(t, err)
	require.Equal(t, uploadHash, uploaded.FileHash, "ordinary upload deduplication retains its metadata")
	require.Empty(t, uploaded.IngestedChunkDigest)

}

func TestKnowledgeIngestionMySQLAtomicScopeAndChunkEvidence(t *testing.T) {
	db := ingestionEvidenceDB(t)
	ctx := context.Background()
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	docID, kbID := uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, db.Where("tenant_id=? AND knowledge_id=?", tenant, docID).Delete(&ChunkModel{}).Error)
		require.NoError(t, db.Where("tenant_id=? AND id=?", tenant, docID).Delete(&KnowledgeModel{}).Error)
	})
	repo := NewKnowledgeRepository(db)
	finalizer := repo.(d.KnowledgeIngestionFinalizer)
	starter := repo.(d.KnowledgeReingestionStarter)
	create := &d.Knowledge{ID: docID, TenantID: tenant, UserID: 42, KnowledgeBaseID: kbID, Type: "text", Title: "evidence", Source: "fixture", FileHash: evidenceHash("raw"), ParseStatus: d.ParseStatusProcessing, EnableStatus: d.EnableStatusEnabled, Description: "concurrent metadata", IngestedChunkDigest: ""}
	require.NoError(t, repo.Create(ctx, create))
	chunk := &d.Chunk{ID: uuid.NewString(), TenantID: tenant, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "raw", IsEnabled: true}
	require.NoError(t, NewChunkRepository(db).Create(ctx, chunk))
	in := d.IngestionCompletion{TenantID: tenant, UserID: 42, KnowledgeID: docID, KnowledgeBaseID: kbID, ContentDigest: create.FileHash, ExpectedFileHash: create.FileHash, ChunkDigest: d.IngestionChunkDigest([]*d.Chunk{chunk}), ChunkCount: 1, ParseStatus: d.ParseStatusCompleted}
	for _, mutate := range []func(*d.IngestionCompletion){func(v *d.IngestionCompletion) { v.TenantID++ }, func(v *d.IngestionCompletion) { v.UserID++ }, func(v *d.IngestionCompletion) { v.KnowledgeBaseID = uuid.NewString() }, func(v *d.IngestionCompletion) { v.ContentDigest = evidenceHash("other") }, func(v *d.IngestionCompletion) { v.ChunkDigest = evidenceHash("other chunks") }, func(v *d.IngestionCompletion) { v.ChunkCount = 2 }} {
		bad := in
		mutate(&bad)
		require.Error(t, finalizer.FinalizeIngestion(ctx, bad))
		row, err := repo.FindByID(ctx, docID)
		require.NoError(t, err)
		require.Empty(t, row.IngestedChunkDigest)
		require.Equal(t, d.ParseStatusProcessing, row.ParseStatus)
	}
	require.NoError(t, db.Model(&KnowledgeModel{}).Where("id=? AND tenant_id=?", docID, tenant).Updates(map[string]any{"title": "edited title", "description": "edited description", "enable_status": d.EnableStatusDisabled}).Error)
	require.NoError(t, finalizer.FinalizeIngestion(ctx, in))
	row, err := repo.FindByID(ctx, docID)
	require.NoError(t, err)
	require.Equal(t, in.ChunkDigest, row.IngestedChunkDigest)
	require.Equal(t, "edited title", row.Title)
	require.Equal(t, "edited description", row.Description)
	require.Equal(t, d.EnableStatusDisabled, row.EnableStatus)
	require.NoError(t, starter.BeginReingestion(ctx, d.IngestionStart{TenantID: tenant, UserID: 42, KnowledgeID: docID, KnowledgeBaseID: kbID, PreviousContentDigest: in.ContentDigest, ContentDigest: evidenceHash("next")}))
	row, err = repo.FindByID(ctx, docID)
	require.NoError(t, err)
	require.Empty(t, row.IngestedChunkDigest)
	require.Nil(t, row.ProcessedAt)
	require.Equal(t, d.ParseStatusProcessing, row.ParseStatus)
	require.ErrorIs(t, finalizer.FinalizeIngestion(ctx, in), d.ErrIngestionConflict, "old completion cannot rebind new ingestion")
	// A failed new index cannot regain the previous successful evidence.
	in.ContentDigest = evidenceHash("next")
	in.ExpectedFileHash = in.ContentDigest
	in.ParseStatus = d.ParseStatusFailed
	in.ErrorMessage = "index failed"
	require.NoError(t, finalizer.FinalizeIngestion(ctx, in))
	row, err = repo.FindByID(ctx, docID)
	require.NoError(t, err)
	require.Empty(t, row.IngestedChunkDigest)
	require.Equal(t, d.ParseStatusFailed, row.ParseStatus)
}
