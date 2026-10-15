package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"

	domain "cognida/internal/model/knowledge"
)

// DocumentIngestionLocker serializes a stable ingestion key across workers.
// The storage adapter owns lock lifetime; callers never hold a DB transaction
// open across parsing, embedding or graph calls.
type DocumentIngestionLocker interface {
	WithDocumentLock(context.Context, string, func() error) error
}

// DocumentIngestionTargetValidator lets collection validate even a duplicate
// that can reuse its previous receipt without invoking document processing.
type DocumentIngestionTargetValidator interface {
	ValidateIngestionTarget(context.Context, int64, string) error
}

func (s *documentProcessorService) ProcessDocument(ctx context.Context, tenantID, userID int64, req *ProcessDocumentRequest) (*ProcessDocumentResponse, error) {
	if req == nil || req.IdempotencyKey == "" {
		return s.processDocument(ctx, tenantID, userID, req)
	}
	if tenantID <= 0 || userID <= 0 || req.KnowledgeBaseID == "" || len(req.Content) == 0 {
		return nil, fmt.Errorf("idempotent ingestion requires authenticated identity, knowledge base and content")
	}
	// Check authorization even for an already completed result.
	if _, err := s.requireIngestionKnowledgeBase(ctx, req.KnowledgeBaseID, tenantID); err != nil {
		return nil, err
	}
	locker, ok := s.knowledgeRepo.(DocumentIngestionLocker)
	if !ok {
		return nil, fmt.Errorf("durable ingestion locking is unavailable")
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", tenantID, req.KnowledgeBaseID, req.IdempotencyKey)))
	id := uuid.NewSHA1(uuid.NameSpaceURL, digest[:]).String()
	contentDigest := sha256.Sum256(req.Content)
	contentHash := hex.EncodeToString(contentDigest[:])
	var result *ProcessDocumentResponse
	err := locker.WithDocumentLock(ctx, id, func() error {
		// The target may have been disabled while another worker held the lock.
		if _, err := s.requireIngestionKnowledgeBase(ctx, req.KnowledgeBaseID, tenantID); err != nil {
			return err
		}
		copy := *req
		copy.DocumentID, copy.creationID, copy.contentHash = "", id, contentHash
		existing, err := s.knowledgeRepo.FindByID(ctx, id)
		if err != nil && !errors.Is(err, domain.ErrKnowledgeNotFound) {
			return err
		}
		if existing != nil {
			if existing.DeletedAt != nil || !existing.IsEnabled() {
				return domain.ErrKnowledgeNotFound
			}
			if existing.TenantID != tenantID || existing.KnowledgeBaseID != req.KnowledgeBaseID || existing.FileHash != contentHash {
				return fmt.Errorf("ingestion idempotency key conflicts with existing content")
			}
			if existing.ParseStatus == domain.ParseStatusCompleted {
				result = &ProcessDocumentResponse{DocumentID: id, KnowledgeID: id, ParseStatus: existing.ParseStatus, StorageSize: existing.StorageSize}
				if existing.ChunkCount != nil {
					result.ChunkCount = *existing.ChunkCount
				}
				return nil
			}
			copy.DocumentID = id // resume the same document, never create a duplicate
		}
		result, err = s.processDocument(ctx, tenantID, userID, &copy)
		return err
	})
	return result, err
}
