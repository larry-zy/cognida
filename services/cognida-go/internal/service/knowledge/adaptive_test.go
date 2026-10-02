package knowledge

import (
	"cognida/internal/model/rag"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdaptiveRetrievalSelectsAndBoundsFallback(t *testing.T) {
	fake := &capFakeRetriever{hybrid: []*rag.Document{doc("a", "error code explanation", .1)}}
	result, e := NewRetrievalCapability(fake, nil).Retrieve(context.Background(), 1, []string{"kb"}, GovernedQuery{Query: "ERR_402", Mode: "auto"})
	require.NoError(t, e)
	require.True(t, result.HasAnswer)
	require.Len(t, result.Attempts, 2)
	require.Equal(t, "bm25", result.Attempts[0].Mode)
	require.Equal(t, "hybrid", result.Attempts[1].Mode)
	require.Equal(t, 2, fake.calls)
	fake = &capFakeRetriever{}
	result, e = NewRetrievalCapability(fake, nil).Retrieve(context.Background(), 1, []string{"kb"}, GovernedQuery{Query: "如何改善退款流程", Mode: "auto"})
	require.NoError(t, e)
	require.False(t, result.HasAnswer)
	require.Len(t, result.Attempts, 2)
	require.Equal(t, "vector", result.Attempts[1].Mode)
}

type failingEnabledFilter struct{}

func (failingEnabledFilter) FilterEnabledKnowledgeIDs(context.Context, []string) (map[string]bool, error) {
	return nil, errors.New("offline")
}
func TestGovernedRetrievalNeverLeaksDisabledContentOnDatabaseFailure(t *testing.T) {
	fake := &capFakeRetriever{hybrid: []*rag.Document{{ChunkID: "a", KnowledgeID: "known", Content: "removed content", Score: 1}}}
	result, e := NewRetrievalCapability(fake, nil).WithEnabledFilter(failingEnabledFilter{}).Retrieve(context.Background(), 1, []string{"kb"}, GovernedQuery{Query: "x", Mode: "hybrid"})
	require.NoError(t, e)
	require.Empty(t, result.Chunks)
	require.False(t, result.HasAnswer)
}
func TestAdaptiveChunkingUsesStructure(t *testing.T) {
	strategy, size, overlap := adaptiveChunkPlan("first\n\nsecond\n\nthird")
	require.Equal(t, "paragraph", strategy)
	require.Less(t, overlap, size)
	strategy, _, _ = adaptiveChunkPlan("introduction\n# heading")
	require.Equal(t, "recursive", strategy)
}

func TestChunkConfigPersistsAndValidatesPatch(t *testing.T) {
	strategy := "auto"
	config, e := mergeChunkConfig("", &strategy, nil, nil)
	require.NoError(t, e)
	require.JSONEq(t, `{"strategy":"auto","chunk_size":0,"chunk_overlap":0}`, config)
	size, overlap := 700, 70
	config, e = mergeChunkConfig(config, nil, &size, &overlap)
	require.NoError(t, e)
	require.JSONEq(t, `{"strategy":"auto","chunk_size":700,"chunk_overlap":70}`, config)
	overlap = 800
	_, e = mergeChunkConfig(config, nil, nil, &overlap)
	require.True(t, IsValidationError(e))
}
