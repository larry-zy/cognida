package adapters

import (
	"cognida/internal/model/knowledge"
	"cognida/internal/model/rag"
	"cognida/internal/service/agent/tools"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type chunkConsumptionGate struct{ err error }

func (chunkConsumptionGate) Prepare(context.Context, string, string) error { return nil }
func (g chunkConsumptionGate) Filter(context.Context, string, string, []*rag.Document) ([]*rag.Document, error) {
	return nil, g.err
}

func TestChunkFetchAppliesGuardEvenWithEnabledOnlyFalse(t *testing.T) {
	for _, err := range []error{nil, errors.New("publication unavailable")} {
		q := &fakeChunkQuerier{docs: []*knowledge.IndexedChunk{{ChunkID: "old", KnowledgeID: "retired", KnowledgeBaseID: "kb", Content: "private"}}}
		a := newChunkAdapter(q, &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb")}}).WithConsumptionGuard(chunkConsumptionGate{err})
		result, e := a.FetchChunks(chunkCtx(7, "kb"), &tools.FetchChunksRequest{KnowledgeIDs: []string{"retired"}, Limit: 20, EnabledOnly: false})
		if err != nil {
			require.Error(t, e)
			require.Nil(t, result)
		} else {
			require.NoError(t, e)
			require.Empty(t, result.Chunks)
		}
	}
}

// The current SQL content guard can approve one of multiple stale index rows
// sharing an ID. Approval must not leak to another version or owner tuple.
type exactChunkGate struct{}

func (exactChunkGate) Prepare(context.Context, string, string) error { return nil }
func (exactChunkGate) Filter(_ context.Context, _, _ string, docs []*rag.Document) ([]*rag.Document, error) {
	for _, doc := range docs {
		if doc.KnowledgeID == "current-doc" && doc.KnowledgeBaseID == "kb" && doc.Content == "current text" {
			return []*rag.Document{doc}, nil
		}
	}
	return nil, nil
}

func TestChunkFetchGuardBindsExactContentAndOwnershipWithDuplicateIDs(t *testing.T) {
	q := &fakeChunkQuerier{docs: []*knowledge.IndexedChunk{
		{ChunkID: "same", KnowledgeID: "current-doc", KnowledgeBaseID: "kb", Content: "old private text"},
		{ChunkID: "same", KnowledgeID: "other-doc", KnowledgeBaseID: "kb", Content: "current text"},
		{ChunkID: "same", KnowledgeID: "current-doc", KnowledgeBaseID: "other-kb", Content: "current text"},
		{ChunkID: "same", KnowledgeID: "current-doc", KnowledgeBaseID: "kb", Content: "current text", ChunkIndex: 2, TokenCount: 4},
	}}
	a := newChunkAdapter(q, &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb")}}).WithConsumptionGuard(exactChunkGate{})
	result, err := a.FetchChunks(chunkCtx(7, "kb"), &tools.FetchChunksRequest{ChunkIDs: []string{"same"}, Limit: 20})
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)
	require.Equal(t, []tools.FetchedChunk{{ChunkID: "same", KnowledgeID: "current-doc", KBID: "kb", Content: "current text", ChunkIndex: 2, TokenCount: 4}}, result.Chunks)
}
