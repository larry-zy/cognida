package mysql

import (
	"context"
	"errors"
	"time"

	k "cognida/internal/model/knowledge"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *chunkRepository) EditChunk(ctx context.Context, chunk *k.Chunk) error {
	return r.mutateChunk(ctx, chunk, false)
}

func (r *chunkRepository) RemoveChunk(ctx context.Context, chunk *k.Chunk) error {
	return r.mutateChunk(ctx, chunk, true)
}

func (r *chunkRepository) mutateChunk(ctx context.Context, chunk *k.Chunk, remove bool) error {
	if chunk == nil || chunk.TenantID <= 0 || chunk.ID == "" || chunk.KnowledgeID == "" || chunk.KnowledgeBaseID == "" {
		return k.ErrInvalidDocument
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc KnowledgeModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND knowledge_base_id=? AND deleted_at IS NULL", chunk.KnowledgeID, chunk.TenantID, chunk.KnowledgeBaseID).First(&doc).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return k.ErrKnowledgeNotFound
		}
		if err != nil {
			return err
		}
		if doc.ParseStatus == k.ParseStatusProcessing || doc.ParseStatus == k.ParseStatusPending {
			return k.ErrIngestionConflict
		}
		q := tx.Model(&ChunkModel{}).Where("id=? AND tenant_id=? AND knowledge_base_id=? AND knowledge_id=? AND deleted_at IS NULL", chunk.ID, chunk.TenantID, chunk.KnowledgeBaseID, chunk.KnowledgeID)
		var result *gorm.DB
		if remove {
			result = q.Delete(&ChunkModel{})
		} else {
			result = q.Updates(map[string]any{"content": chunk.Content, "token_count": nil, "updated_at": chunk.UpdatedAt})
		}
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return k.ErrChunkNotFound
		}
		updates := map[string]any{"ingested_chunk_digest": "", "updated_at": time.Now().UTC()}
		if remove {
			updates["chunk_count"] = gorm.Expr("GREATEST(chunk_count - 1, 0)")
		}
		return tx.Model(&KnowledgeModel{}).Where("id=? AND tenant_id=?", chunk.KnowledgeID, chunk.TenantID).Updates(updates).Error
	})
}

var _ k.ChunkMutationRepository = (*chunkRepository)(nil)
