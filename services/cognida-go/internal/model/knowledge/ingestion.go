package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
)

var ErrIngestionConflict = errors.New("knowledge ingestion evidence conflicts with stored document")

// IngestionCompletion contains only processor-owned fields. The repository
// verifies the source hash and persisted chunks while locking the document.
type IngestionCompletion struct {
	TenantID, UserID                                         int64
	KnowledgeID, KnowledgeBaseID, ContentDigest, ChunkDigest string
	// ExpectedFileHash is the current file metadata used for the commit CAS.
	// ContentDigest is nonempty only when this ingestion supplied actual bytes.
	ExpectedFileHash          string
	ChunkCount                int
	ParseStatus, ErrorMessage string
}

// KnowledgeIngestionFinalizer atomically binds the original input to the exact
// chunks that were ingested. Replays read this evidence; they never regenerate it.
type KnowledgeIngestionFinalizer interface {
	FinalizeIngestion(context.Context, IngestionCompletion) error
}

type IngestionStart struct {
	TenantID, UserID                                                   int64
	KnowledgeID, KnowledgeBaseID, PreviousContentDigest, ContentDigest string
}

// KnowledgeReingestionStarter invalidates the old evidence before removing any
// chunks. An unavailable new byte digest is empty, never inherited as evidence;
// existing upload file metadata may still be retained for deduplication.
type KnowledgeReingestionStarter interface {
	BeginReingestion(context.Context, IngestionStart) error
}

// IngestionChunkDigest matches knowledge-label content: enabled live chunks in
// (chunk_index, id) order, each including one trailing newline. It does not try
// to reverse overlap, normalize whitespace, or reconstruct the original input.
func IngestionChunkDigest(chunks []*Chunk) string {
	ordered := make([]*Chunk, 0, len(chunks))
	for _, c := range chunks {
		if c != nil && c.IsEnabled && c.DeletedAt == nil {
			ordered = append(ordered, c)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ChunkIndex != ordered[j].ChunkIndex {
			return ordered[i].ChunkIndex < ordered[j].ChunkIndex
		}
		return ordered[i].ID < ordered[j].ID
	})
	h := sha256.New()
	for _, c := range ordered {
		_, _ = h.Write([]byte(c.Content))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
