package knowledge

import (
	"context"
	"fmt"

	k "cognida/internal/model/knowledge"
)

// IndexChunk uses the same embedding, BM25 fields and stable vector IDs as
// ingestion. The caller holds the document lock and removes old projections.
func (s *documentProcessorService) IndexChunk(ctx context.Context, chunk *k.Chunk) error {
	if s.embedder == nil || chunk == nil {
		return fmt.Errorf("chunk indexing is unavailable")
	}
	if err := s.vectorizeChunks(ctx, chunk.TenantID, chunk.KnowledgeBaseID, []*k.Chunk{chunk}); err != nil {
		return err
	}
	settings, err := s.settingRepo.FindByKnowledgeBaseID(ctx, chunk.KnowledgeBaseID)
	if err != nil {
		return err
	}
	if settings != nil && settings.GraphEnabled {
		_, _, err = s.extractGraph(ctx, chunk.TenantID, chunk.KnowledgeBaseID, chunk.KnowledgeID, []*k.Chunk{chunk})
	}
	return err
}

var _ k.ChunkIndexer = (*documentProcessorService)(nil)

// vectorCollectionKey preserves the collection mapping used by existing
// ingestion, including IDs with a decimal prefix followed by hex characters.
func vectorCollectionKey(kbID string) int64 {
	var key int64
	_, _ = fmt.Sscanf(kbID, "%d", &key)
	return key
}
