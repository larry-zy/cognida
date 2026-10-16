package knowledge

import (
	"context"
	"errors"
	"testing"

	f "cognida/internal/model/feedback"
	k "cognida/internal/model/knowledge"
	l "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

type cleanupDocuments struct {
	k.KnowledgeRepository
	doc *k.Knowledge
}

func (r *cleanupDocuments) FindByID(context.Context, string) (*k.Knowledge, error) {
	if r.doc == nil {
		return nil, k.ErrKnowledgeNotFound
	}
	return r.doc, nil
}

type cleanupVectors struct {
	k.VectorRepository
	events *[]string
	err    error
}

func (r *cleanupVectors) HasCollection(context.Context, int64) (bool, error) { return true, nil }
func (r *cleanupVectors) DeleteByKnowledgeID(context.Context, int64, string) error {
	*r.events = append(*r.events, "vector")
	return r.err
}

type cleanupGraph struct {
	k.GraphRepository
	events    *[]string
	namespace k.NameSpace
	chunkIDs  []string
}

func (r *cleanupGraph) DeleteByChunkIDs(_ context.Context, ns k.NameSpace, ids []string) error {
	*r.events = append(*r.events, "graph")
	r.namespace, r.chunkIDs = ns, append([]string{}, ids...)
	return nil
}

type cleanupChunks struct{ k.ChunkRepository }

func (cleanupChunks) FindByKnowledgeID(context.Context, string, bool) ([]*k.Chunk, error) {
	return []*k.Chunk{{ID: "candidate-chunk"}}, nil
}

type cleanupStats struct {
	k.KnowledgeStatsQuerier
	events *[]string
	docs   *cleanupDocuments
}

func (r *cleanupStats) DeleteKnowledgeWithChunks(context.Context, string, string) error {
	*r.events = append(*r.events, "mysql")
	r.docs.doc = nil
	return nil
}

func TestFeedbackCleanupSurvivesDeletedKBAndRetainsOwnerBinding(t *testing.T) {
	ctx := context.Background()
	p := l.Principal{TenantID: 42, UserID: 7}
	docs := &cleanupDocuments{doc: &k.Knowledge{ID: "candidate", TenantID: p.TenantID, UserID: p.UserID, KnowledgeBaseID: "20261016-deleted"}}
	events := []string{}
	vectors := &cleanupVectors{events: &events}
	graph := &cleanupGraph{events: &events}
	stats := &cleanupStats{events: &events, docs: docs}
	service := NewKnowledgeBaseService(newTenantFakeKBRepo(), nil, docs, cleanupChunks{}, stats, vectors, graph)
	projection := NewFeedbackProjection(nil, docs, nil, nil, nil, service, nil)
	for _, invalid := range []struct {
		who l.Principal
		kb  string
	}{
		{l.Principal{}, docs.doc.KnowledgeBaseID},
		{l.Principal{TenantID: 43, UserID: 7}, docs.doc.KnowledgeBaseID},
		{l.Principal{TenantID: 42, UserID: 8}, docs.doc.KnowledgeBaseID},
		{p, "another-kb"},
	} {
		require.ErrorIs(t, projection.Remove(ctx, invalid.who, invalid.kb, "candidate"), f.ErrForbidden)
		require.Empty(t, events)
	}
	// The public deletion API retains its active-KB authorization boundary.
	require.Error(t, service.DeleteKnowledge(ctx, docs.doc.KnowledgeBaseID, "candidate", p.TenantID))
	require.Empty(t, events)
	// A failed projection delete must preserve authoritative rows for retry.
	vectors.err = errors.New("projection refused")
	require.Error(t, projection.Remove(ctx, p, docs.doc.KnowledgeBaseID, "candidate"))
	require.NotNil(t, docs.doc)
	require.NotContains(t, events, "mysql")
	events = nil
	vectors.err = nil
	require.NoError(t, projection.Remove(ctx, p, docs.doc.KnowledgeBaseID, "candidate"))
	require.Equal(t, []string{"vector", "graph", "mysql"}, events)
	require.Equal(t, k.NameSpace{TenantID: "42", KnowledgeBaseID: "20261016-deleted", Knowledge: "candidate"}, graph.namespace)
	require.Equal(t, []string{"candidate-chunk"}, graph.chunkIDs)
	// Already removed records are successful idempotent cleanup, not new work.
	require.NoError(t, projection.Remove(ctx, p, "20261016-deleted", "candidate"))
	require.Len(t, events, 3)
}
