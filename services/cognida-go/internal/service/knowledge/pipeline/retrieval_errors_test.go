package rag

import (
	"cognida/internal/model/knowledge"
	domainrag "cognida/internal/model/rag"
	"context"
	"errors"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/stretchr/testify/require"
	"testing"
)

type failingIndex struct {
	vectorErr, keywordErr error
	docs                  []*domainrag.Document
}

func (m failingIndex) Search(context.Context, string, []float32, int, map[string]string) ([]*domainrag.Document, error) {
	return m.docs, m.vectorErr
}
func (m failingIndex) FullTextSearch(context.Context, string, string, int, map[string]string) ([]*domainrag.Document, error) {
	return m.docs, m.keywordErr
}

type retrievalChunks struct {
	knowledge.ChunkRepository
	err    error
	chunks []*knowledge.Chunk
}

func (r retrievalChunks) FindEnabledChunks(context.Context, string, int) ([]*knowledge.Chunk, error) {
	return r.chunks, r.err
}

type retrievalEmbedder struct {
	embedding.Embedder
	err error
}

func (e retrievalEmbedder) EmbedStrings(context.Context, []string, ...embedding.Option) ([][]float64, error) {
	return [][]float64{{1}}, e.err
}
func TestRetrievalBackendFailuresAndFallbackWarnings(t *testing.T) {
	failure := errors.New("backend unavailable")
	ret := NewRetriever(nil, retrievalChunks{err: failure}, retrievalEmbedder{err: failure}, failingIndex{vectorErr: failure, keywordErr: failure}, nil, nil, nil)
	for _, method := range []func(context.Context, string, string, string, *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error){ret.VectorRetrieve, ret.BM25Retrieve, ret.HybridRetrieve} {
		_, err := method(context.Background(), "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
		require.ErrorIs(t, err, failure)
	}
	ret = NewRetriever(nil, retrievalChunks{chunks: []*knowledge.Chunk{{ID: "c", Content: "proof", KnowledgeBaseID: "kb"}}}, retrievalEmbedder{err: failure}, failingIndex{vectorErr: failure, keywordErr: failure}, nil, nil, nil)
	out, err := ret.HybridRetrieve(context.Background(), "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
	require.NoError(t, err)
	require.NotEmpty(t, out.Results)
	require.NotEmpty(t, out.Warnings)
	require.Equal(t, []string{"database_lexical"}, out.Results[0].Sources)
	ret = NewRetriever(nil, nil, retrievalEmbedder{}, failingIndex{vectorErr: failure, docs: []*domainrag.Document{{ChunkID: "c", Content: "proof", Score: 1}}}, nil, nil, nil)
	out, err = ret.HybridRetrieve(context.Background(), "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
	require.NoError(t, err)
	require.Len(t, out.Results, 1)
	require.NotEmpty(t, out.Warnings)
	require.Equal(t, []string{"milvus_bm25"}, out.Results[0].Sources)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ret.HybridRetrieve(ctx, "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
	require.ErrorIs(t, err, context.Canceled)
}

func TestRetrievalSourcesDescribeActualSuccessfulBackends(t *testing.T) {
	doc := &domainrag.Document{ChunkID: "c", Content: "proof", Score: 1, Sources: []string{"untrusted_metadata_source"}}
	ret := NewRetriever(nil, nil, retrievalEmbedder{}, failingIndex{docs: []*domainrag.Document{doc}}, nil, nil, nil)
	out, err := ret.VectorRetrieve(context.Background(), "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
	require.NoError(t, err)
	require.Equal(t, []string{"milvus_vector"}, out.Results[0].Sources)
	out, err = ret.HybridRetrieve(context.Background(), "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
	require.NoError(t, err)
	require.Equal(t, []string{"milvus_vector", "milvus_bm25"}, out.Results[0].Sources)
	require.Equal(t, []string{"untrusted_metadata_source"}, doc.Sources, "parallel retrieval must not mutate shared source documents")
	ret = NewRetriever(nil, retrievalChunks{chunks: []*knowledge.Chunk{{ID: "c", Content: "proof", KnowledgeBaseID: "kb"}}}, retrievalEmbedder{}, failingIndex{docs: []*domainrag.Document{}}, nil, nil, nil)
	out, err = ret.VectorRetrieve(context.Background(), "7", "kb", "proof", &domainrag.RetrieveOptions{TopK: 5})
	require.NoError(t, err)
	require.Equal(t, []string{"database_lexical"}, out.Results[0].Sources, "an empty successful index lookup does not label SQL results as Milvus")
}
