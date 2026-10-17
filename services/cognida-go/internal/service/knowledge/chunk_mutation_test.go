package knowledge

import (
	a "cognida/internal/model/agent"
	f "cognida/internal/model/feedback"
	k "cognida/internal/model/knowledge"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/rag"
	feedbacksvc "cognida/internal/service/feedback"
	ragpipeline "cognida/internal/service/knowledge/pipeline"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type mutationKB struct{ k.KnowledgeBaseRepository }

func (mutationKB) FindByIDForTenant(context.Context, string, int64) (*k.KnowledgeBase, error) {
	return &k.KnowledgeBase{ID: "kb", TenantID: 1, UserID: 1, Status: 1}, nil
}

type mutationDocuments struct{ k.KnowledgeRepository }

func (mutationDocuments) FindByID(context.Context, string) (*k.Knowledge, error) {
	return &k.Knowledge{ID: "doc", TenantID: 1, KnowledgeBaseID: "kb", ParseStatus: k.ParseStatusCompleted, EnableStatus: k.EnableStatusEnabled}, nil
}
func (mutationDocuments) FilterEnabledKnowledgeIDs(context.Context, []string) (map[string]bool, error) {
	return map[string]bool{"doc": true}, nil
}
func (mutationDocuments) WithDocumentLock(_ context.Context, _ string, fn func() error) error {
	return fn()
}

type mutationChunks struct {
	k.ChunkRepository
	chunk   *k.Chunk
	deleted bool
}

func (r *mutationChunks) FindByID(context.Context, string) (*k.Chunk, error) {
	if r.deleted {
		return nil, k.ErrChunkNotFound
	}
	c := *r.chunk
	return &c, nil
}
func (r *mutationChunks) EditChunk(_ context.Context, c *k.Chunk) error { r.chunk = c; return nil }
func (r *mutationChunks) RemoveChunk(context.Context, *k.Chunk) error   { r.deleted = true; return nil }

type mutationIndex struct {
	k.VectorRepository
	deleteErr, indexErr error
	removed, indexed    int
	content             string
}

func (r *mutationIndex) HasCollection(context.Context, int64) (bool, error) { return true, nil }
func (r *mutationIndex) DeleteByChunkID(context.Context, int64, string) error {
	r.removed++
	return r.deleteErr
}
func (r *mutationIndex) IndexChunk(_ context.Context, c *k.Chunk) error {
	r.indexed++
	if r.indexErr != nil {
		return r.indexErr
	}
	r.content = c.Content
	return nil
}

type mutationSearchIndex struct{}

func (r mutationSearchIndex) Search(context.Context, string, []float32, int, map[string]string) ([]*rag.Document, error) {
	return nil, nil
}
func (r mutationSearchIndex) FullTextSearch(context.Context, string, string, int, map[string]string) ([]*rag.Document, error) {
	// Simulate delayed Milvus visibility: the old row remains visible even after
	// delete succeeds. The source guard must independently reject that row.
	return []*rag.Document{{ChunkID: "chunk", KnowledgeID: "doc", KnowledgeBaseID: "kb", Content: "obsolete secret text", Score: 1}}, nil
}

type mutationFeedbackRepo struct{ f.Repository }

func (mutationFeedbackRepo) FindKnowledge(context.Context, int64, string) (*f.Case, *f.Publication, error) {
	return nil, nil, nil
}
func mutationFixture() (context.Context, *mutationChunks, *mutationIndex, KnowledgeBaseService) {
	ctx := a.WithUserID(a.WithTenantID(context.Background(), 1), 1)
	chunks := &mutationChunks{chunk: &k.Chunk{ID: "chunk", KnowledgeID: "doc", KnowledgeBaseID: "kb", TenantID: 1, Content: "obsolete secret text", IsEnabled: true}}
	index := &mutationIndex{}
	return ctx, chunks, index, NewKnowledgeBaseService(mutationKB{}, nil, mutationDocuments{}, chunks, nil, index, nil, index)
}
func TestChunkMutationRejectsStaleRetrieval(t *testing.T) {
	for _, op := range []string{"edit", "delete", "failed_reindex"} {
		t.Run(op, func(t *testing.T) {
			ctx, chunks, index, svc := mutationFixture()
			if op == "delete" {
				require.NoError(t, svc.DeleteChunk(ctx, "kb", 1, "chunk"))
			} else {
				if op == "failed_reindex" {
					index.indexErr = errors.New("embedding unavailable")
				}
				text := "replacement correct text"
				_, err := svc.UpdateChunk(ctx, "kb", 1, "chunk", &text)
				if op == "failed_reindex" {
					require.ErrorContains(t, err, "retry this edit")
				} else {
					require.NoError(t, err)
					require.Equal(t, text, index.content)
				}
			}
			require.Equal(t, 1, index.removed)
			guard := &feedbacksvc.Service{Repo: mutationFeedbackRepo{}, Projection: &FeedbackProjection{KBs: mutationKB{}, Documents: mutationDocuments{}, Chunks: chunks}}
			retriever := ragpipeline.NewRetriever(nil, chunks, nil, mutationSearchIndex{}, nil, nil, nil)
			retriever.(interface{ SetConsumptionGuard(rag.ConsumptionGuard) }).SetConsumptionGuard(guard)
			capability := NewRetrievalCapability(retriever, nil).WithEnabledFilter(mutationDocuments{})
			capability.SetConsumptionGuard(guard)
			out, err := capability.Retrieve(ctx, 1, []string{"kb"}, GovernedQuery{Query: "obsolete secret text", Mode: "bm25", TopK: 5})
			require.NoError(t, err)
			require.Empty(t, out.Chunks)
			if op == "failed_reindex" {
				index.indexErr = nil
				text := "replacement correct text"
				_, err = svc.UpdateChunk(ctx, "kb", 1, "chunk", &text)
				require.NoError(t, err)
				require.Equal(t, 2, index.indexed)
			}
		})
	}
}
func TestChunkDeleteProjectionFailureKeepsSourceForRetry(t *testing.T) {
	ctx, chunks, index, svc := mutationFixture()
	index.deleteErr = errors.New("index unavailable")
	require.Error(t, svc.DeleteChunk(ctx, "kb", 1, "chunk"))
	require.False(t, chunks.deleted)
	index.deleteErr = nil
	require.NoError(t, svc.DeleteChunk(ctx, "kb", 1, "chunk"))
	require.True(t, chunks.deleted)
}
func TestChunkCurrentSourceGuard(t *testing.T) {
	ctx, chunks, _, _ := mutationFixture()
	p := &FeedbackProjection{Chunks: chunks}
	hit := &rag.Document{ChunkID: "chunk", KnowledgeID: "doc", KnowledgeBaseID: "kb", Content: chunks.chunk.Content}
	who := l.Principal{TenantID: 1, UserID: 1}
	ok, err := p.CurrentChunk(ctx, who, hit)
	require.NoError(t, err)
	require.True(t, ok)
	for _, change := range []func(){func() { chunks.chunk.Content = "changed" }, func() { chunks.chunk.TenantID = 2 }, func() { chunks.chunk.KnowledgeID = "other" }, func() { chunks.chunk.KnowledgeBaseID = "other" }, func() { chunks.chunk.IsEnabled = false }} {
		backup := *chunks.chunk
		change()
		ok, err = p.CurrentChunk(ctx, who, hit)
		require.NoError(t, err)
		require.False(t, ok)
		*chunks.chunk = backup
	}
}

func TestVectorCollectionKeyPreservesHexIDSuffix(t *testing.T) {
	require.EqualValues(t, 20260704, vectorCollectionKey("20260704bfa0e3"))
	require.EqualValues(t, 42, vectorCollectionKey("42"))
	require.Zero(t, vectorCollectionKey("kb-uuid"))
	require.Zero(t, vectorCollectionKey(""))
}

type indexedProjection struct {
	k.VectorRepository
	kb   int64
	docs []*k.VectorDocument
}

func (r *indexedProjection) HasCollection(context.Context, int64) (bool, error) { return true, nil }
func (r *indexedProjection) Insert(_ context.Context, kb int64, docs []*k.VectorDocument) error {
	r.kb = kb
	r.docs = docs
	return nil
}

type chunkIndexSettings struct {
	k.KnowledgeBaseSettingRepository
	enabled bool
}

func (s chunkIndexSettings) FindByKnowledgeBaseID(context.Context, string) (*k.KnowledgeBaseSetting, error) {
	return &k.KnowledgeBaseSetting{GraphEnabled: s.enabled}, nil
}
func TestIndexChunkUsesCurrentTextForEmbeddingAndBM25(t *testing.T) {
	index := &indexedProjection{}
	embedder := &recordingEmbedder{}
	svc := &documentProcessorService{vectorRepo: index, embedder: embedder, settingRepo: chunkIndexSettings{}}
	chunk := &k.Chunk{ID: "current-chunk", KnowledgeID: "doc", KnowledgeBaseID: "20260704bfa0e3", TenantID: 1, Content: "updated text", IsEnabled: true}
	require.NoError(t, svc.IndexChunk(context.Background(), chunk))
	require.Equal(t, [][]string{{chunk.Content}}, embedder.batches)
	require.Len(t, index.docs, 1)
	require.EqualValues(t, 20260704, index.kb)
	require.Equal(t, chunk.Content, index.docs[0].Text)
	require.Equal(t, chunk.Content, index.docs[0].Content)
	require.Equal(t, stableVectorID(chunk.ID), index.docs[0].ID)
	require.Equal(t, chunk.KnowledgeBaseID, index.docs[0].KnowledgeBaseID)
	// Graph-enabled KBs cannot silently report completion if that projection
	// cannot be rebuilt. The same request remains retryable.
	svc.settingRepo = chunkIndexSettings{enabled: true}
	require.ErrorContains(t, svc.IndexChunk(context.Background(), chunk), "graph repository")
}
