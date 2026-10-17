package knowledge

import "context"

// ChunkMutationRepository changes source text and invalidates its ingestion
// evidence atomically. It must never recreate a concurrently deleted chunk.
type ChunkMutationRepository interface {
	EditChunk(context.Context, *Chunk) error
	RemoveChunk(context.Context, *Chunk) error
}

// ChunkIndexer rebuilds searchable projections from the committed source.
type ChunkIndexer interface {
	IndexChunk(context.Context, *Chunk) error
}
