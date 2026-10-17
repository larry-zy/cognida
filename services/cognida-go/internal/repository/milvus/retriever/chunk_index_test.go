package retriever

import (
	"context"
	"testing"

	"cognida/internal/model/knowledge"
	"github.com/stretchr/testify/require"
)

func TestChunkIndexQueryPreservesScopeEscapingAndPaging(t *testing.T) {
	opts, err := chunkIndexOptions(knowledge.ChunkIndexQuery{
		TenantID: 42, KBIDs: []string{`kb"x`, `kb\y`}, ChunkIDs: []string{"chunk\nline"},
		KnowledgeIDs: []string{"k1"}, EnabledOnly: true, Limit: 50, Offset: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{`tenant_id == 42 and kb_id in ["kb\"x","kb\\y"] and chunk_id in ["chunk\nline"] and knowledge_id in ["k1"] and is_enabled == true`}, opts.Expr)
	require.EqualValues(t, 50, opts.Limit)
	require.EqualValues(t, 10, opts.Offset)
	for _, q := range []knowledge.ChunkIndexQuery{{KBIDs: []string{"k"}}, {TenantID: 1}} {
		_, err = chunkIndexOptions(q)
		require.Error(t, err)
	}
}

func TestChunkIndexDomainMappingAndTypedNil(t *testing.T) {
	require.Equal(t, []*knowledge.IndexedChunk{{ChunkID: "c", KnowledgeID: "doc", KnowledgeBaseID: "kb", ChunkIndex: 2, Content: "exact 中文", IsEnabled: true, TokenCount: 9}}, indexedChunks([]*DocumentData{nil, {ChunkID: "c", KnowledgeID: "doc", KnowledgeBaseID: "kb", ChunkIndex: 2, Content: "exact 中文", IsEnabled: true, TokenCount: 9}}))
	require.NotNil(t, indexedChunks(nil))
	var reader knowledge.ChunkIndexReader = (*VectorRetriever)(nil)
	_, err := reader.QueryChunks(context.Background(), knowledge.ChunkIndexQuery{TenantID: 1, KBIDs: []string{"kb"}})
	require.Error(t, err)
}
