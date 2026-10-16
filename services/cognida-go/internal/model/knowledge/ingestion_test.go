package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIngestionChunkDigestPreservesOverlapAndLabelContentOrder(t *testing.T) {
	deleted := time.Now()
	chunks := []*Chunk{
		{ID: "b", ChunkIndex: 1, Content: "overlap tail\r\n", IsEnabled: true},
		{ID: "z", ChunkIndex: 0, Content: "head overlap", IsEnabled: true},
		{ID: "a", ChunkIndex: 1, Content: "same index first", IsEnabled: true},
		{ID: "hidden", Content: "disabled", IsEnabled: false},
		{ID: "deleted", Content: "deleted", IsEnabled: true, DeletedAt: &deleted},
	}
	sum := sha256.Sum256([]byte("head overlap\nsame index first\noverlap tail\r\n\n"))
	require.Equal(t, hex.EncodeToString(sum[:]), IngestionChunkDigest(chunks))
	require.Equal(t, "b", chunks[0].ID, "digest must not reorder the processor's slice")
	changed := append([]*Chunk{}, chunks...)
	copy := *changed[0]
	copy.Content = "modified"
	changed[0] = &copy
	require.NotEqual(t, IngestionChunkDigest(chunks), IngestionChunkDigest(changed))
}
