package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"testing"

	domain "cognida/internal/model/knowledge"
)

type replayKnowledgeRepo struct {
	domain.KnowledgeRepository
	doc       *domain.Knowledge
	locked    bool
	lockCalls int
}

func (r *replayKnowledgeRepo) WithDocumentLock(_ context.Context, _ string, fn func() error) error {
	r.locked = true
	r.lockCalls++
	defer func() { r.locked = false }()
	return fn()
}
func (r *replayKnowledgeRepo) FindByID(_ context.Context, id string) (*domain.Knowledge, error) {
	if !r.locked {
		return nil, fmt.Errorf("read outside ingestion lock")
	}
	if r.doc.ID != id {
		return nil, domain.ErrKnowledgeNotFound
	}
	return r.doc, nil
}

func TestDocumentIngestionReplayReturnsPreviouslyCommittedDocument(t *testing.T) {
	ctx := context.Background()
	idHash := sha256.Sum256([]byte("1:kb:stable-key"))
	id := uuid.NewSHA1(uuid.NameSpaceURL, idHash[:]).String()
	bodyHash := sha256.Sum256([]byte("already ingested"))
	count := 3
	repo := &replayKnowledgeRepo{doc: &domain.Knowledge{ID: id, TenantID: 1, UserID: 2, KnowledgeBaseID: "kb", FileHash: hex.EncodeToString(bodyHash[:]), ParseStatus: domain.ParseStatusCompleted, EnableStatus: domain.EnableStatusEnabled, ChunkCount: &count}}
	repo.doc.IngestedChunkDigest = "historical-digest-not-recomputed"
	kbRepo := newTenantFakeKBRepo()
	kbRepo.kbs["kb"] = &domain.KnowledgeBase{ID: "kb", TenantID: 1, Status: 1}
	svc := &documentProcessorService{kbRepo: kbRepo, knowledgeRepo: repo}
	req := &ProcessDocumentRequest{KnowledgeBaseID: "kb", IdempotencyKey: "stable-key", Content: []byte("already ingested")}
	for i := 0; i < 2; i++ {
		result, err := svc.ProcessDocument(ctx, 1, 2, req)
		if err != nil {
			t.Fatal(err)
		}
		if result.KnowledgeID != id || result.ChunkCount != 3 {
			t.Fatalf("unexpected replay: %+v", result)
		}
		if result.ContentDigest != repo.doc.FileHash || result.ChunkDigest != repo.doc.IngestedChunkDigest {
			t.Fatal("replay must return only persisted ingestion evidence without reading current chunks")
		}
	}
	if repo.lockCalls != 2 {
		t.Fatal("replays must be serialized")
	}
	if _, err := svc.ProcessDocument(ctx, 1, 3, req); err == nil {
		t.Fatal("another user's key must not return an ingestion proof")
	}
	repo.doc.IngestedChunkDigest = ""
	legacy, err := svc.ProcessDocument(ctx, 1, 2, req)
	if err != nil || legacy.ChunkDigest != "" {
		t.Fatal("legacy replay must not backfill evidence from current chunks")
	}
	repo.doc.EnableStatus = domain.EnableStatusDisabled
	if result, err := svc.ProcessDocument(ctx, 1, 2, req); !errors.Is(err, domain.ErrKnowledgeNotFound) || result != nil {
		t.Fatalf("disabled document replay: result=%+v, err=%v", result, err)
	}
	if repo.doc.IsEnabled() {
		t.Fatal("replay must not re-enable a disabled document")
	}
	repo.doc.EnableStatus = domain.EnableStatusEnabled
	req.Content = []byte("different payload")
	if _, err := svc.ProcessDocument(ctx, 1, 2, req); err == nil {
		t.Fatal("conflicting idempotency key accepted")
	}
	if _, err := svc.ProcessDocument(ctx, 2, 2, req); err == nil {
		t.Fatal("cross-tenant replay accepted")
	}
}
