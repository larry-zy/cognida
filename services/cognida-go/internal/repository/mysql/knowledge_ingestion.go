package mysql

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	d "cognida/internal/model/knowledge"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ingestionSHA(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (r *knowledgeRepository) FinalizeIngestion(ctx context.Context, in d.IngestionCompletion) error {
	if in.TenantID <= 0 || in.UserID <= 0 || in.KnowledgeID == "" || in.KnowledgeBaseID == "" || in.ChunkCount < 1 || in.ChunkCount > 100000 || !ingestionSHA(in.ChunkDigest) || (in.ContentDigest != "" && !ingestionSHA(in.ContentDigest)) || (in.ParseStatus != d.ParseStatusCompleted && in.ParseStatus != d.ParseStatusFailed) {
		return d.ErrInvalidDocument
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc KnowledgeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND knowledge_base_id=? AND deleted_at IS NULL", in.KnowledgeID, in.TenantID, in.UserID, in.KnowledgeBaseID).First(&doc).Error; err != nil {
			return err
		}
		if doc.FileHash != in.ExpectedFileHash || in.ContentDigest != "" && in.ContentDigest != in.ExpectedFileHash {
			return d.ErrIngestionConflict
		}
		var rows []ChunkModel
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("knowledge_id=? AND knowledge_base_id=? AND tenant_id=? AND deleted_at IS NULL", in.KnowledgeID, in.KnowledgeBaseID, in.TenantID).Order("chunk_index,id").Limit(in.ChunkCount + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != in.ChunkCount {
			return d.ErrIngestionConflict
		}
		chunks := make([]*d.Chunk, len(rows))
		for i := range rows {
			if !rows[i].IsEnabled {
				return d.ErrIngestionConflict
			}
			chunks[i] = rows[i].ToDomain()
		}
		if d.IngestionChunkDigest(chunks) != in.ChunkDigest {
			return d.ErrIngestionConflict
		}
		now := time.Now().UTC()
		updates := map[string]any{"parse_status": in.ParseStatus, "error_message": in.ErrorMessage, "chunk_count": in.ChunkCount, "updated_at": now, "ingested_chunk_digest": ""}
		if in.ParseStatus == d.ParseStatusCompleted {
			updates["processed_at"] = now
			if in.ContentDigest != "" {
				updates["ingested_chunk_digest"] = in.ChunkDigest
			}
		}
		return tx.Model(&KnowledgeModel{}).Where("id=? AND tenant_id=? AND user_id=? AND deleted_at IS NULL", in.KnowledgeID, in.TenantID, in.UserID).Updates(updates).Error
	})
}

var _ d.KnowledgeIngestionFinalizer = (*knowledgeRepository)(nil)

func (r *knowledgeRepository) BeginReingestion(ctx context.Context, in d.IngestionStart) error {
	if in.TenantID <= 0 || in.UserID <= 0 || in.KnowledgeID == "" || in.KnowledgeBaseID == "" || (in.ContentDigest != "" && !ingestionSHA(in.ContentDigest)) {
		return d.ErrInvalidDocument
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc KnowledgeModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND knowledge_base_id=? AND deleted_at IS NULL", in.KnowledgeID, in.TenantID, in.UserID, in.KnowledgeBaseID).First(&doc).Error; err != nil {
			return err
		}
		if doc.FileHash != in.PreviousContentDigest {
			return d.ErrIngestionConflict
		}
		updates := map[string]any{"ingested_chunk_digest": "", "parse_status": d.ParseStatusProcessing, "error_message": "", "processed_at": nil, "updated_at": time.Now().UTC()}
		if in.ContentDigest != "" {
			updates["file_hash"] = in.ContentDigest
		}
		return tx.Model(&KnowledgeModel{}).Where("id=? AND tenant_id=? AND user_id=?", in.KnowledgeID, in.TenantID, in.UserID).Updates(updates).Error
	})
}

var _ d.KnowledgeReingestionStarter = (*knowledgeRepository)(nil)
