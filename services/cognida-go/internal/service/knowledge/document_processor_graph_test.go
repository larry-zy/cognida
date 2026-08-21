package knowledge

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cognida/internal/infrastructure/id"
	domain_knowledge "cognida/internal/model/knowledge"
	domain_llm "cognida/internal/model/llm"
)

type scriptedGraphLLMClient struct {
	responses []*domain_llm.ChatResponse
	calls     int
}

func (c *scriptedGraphLLMClient) Chat(ctx context.Context, req *domain_llm.ChatRequest) (*domain_llm.ChatResponse, error) {
	if c.calls >= len(c.responses) {
		return nil, fmt.Errorf("unexpected LLM request %d", c.calls+1)
	}
	response := c.responses[c.calls]
	c.calls++
	return response, nil
}

func (c *scriptedGraphLLMClient) ChatStream(ctx context.Context, req *domain_llm.ChatRequest) (<-chan *domain_llm.ChatChunk, error) {
	return nil, nil
}

func (c *scriptedGraphLLMClient) GetModelInfo(ctx context.Context) (*domain_llm.ModelInfo, error) {
	return nil, nil
}

func (c *scriptedGraphLLMClient) SupportsTools() bool {
	return false
}

func (c *scriptedGraphLLMClient) SupportsStreaming() bool {
	return false
}

func TestExtractGraphData_RetriesTruncatedBatchAndMerges(t *testing.T) {
	llm := &scriptedGraphLLMClient{responses: []*domain_llm.ChatResponse{
		{Content: `{"nodes": [`, FinishReason: "length"},
		{Content: `{"nodes":[{"name":"Acme","entity_type":"组织"},{"name":"Beta","entity_type":"概念"}],"relations":[{"source":"Acme","target":"Beta","type":"RELATED_TO","strength":3}]}`},
		{Content: `{"nodes":[{"name":"acme","entity_type":"组织"},{"name":"beta","entity_type":"概念"}],"relations":[{"source":"acme","target":"beta","type":"RELATED_TO","strength":7}]}`},
	}}
	service := &documentProcessorService{llmClient: llm, idGenerator: id.NewIDGenerator()}
	chunks := []*domain_knowledge.Chunk{
		{ID: "c1", Content: "first"},
		{ID: "c2", Content: "second"},
		{ID: "c3", Content: "third"},
		{ID: "c4", Content: "fourth"},
	}

	graph, err := service.extractGraphData(context.Background(), chunks)
	require.NoError(t, err)
	assert.Equal(t, 3, llm.calls)
	assert.Len(t, graph.Node, 2)
	assert.Len(t, graph.Relation, 1)
	assert.ElementsMatch(t, []string{"c1", "c2", "c3", "c4"}, graph.Node[0].Chunks)
	assert.ElementsMatch(t, []string{"c1", "c2", "c3", "c4"}, graph.Relation[0].ChunkIDs)
	assert.Equal(t, 7.0, graph.Relation[0].Strength)
}

func TestExtractGraphData_ReturnsErrorWhenSingleChunkIsTruncated(t *testing.T) {
	llm := &scriptedGraphLLMClient{responses: []*domain_llm.ChatResponse{{Content: `{"nodes": [`, FinishReason: "length"}}}
	service := &documentProcessorService{llmClient: llm, idGenerator: id.NewIDGenerator()}

	_, err := service.extractGraphData(context.Background(), []*domain_knowledge.Chunk{{ID: "c1", Content: "only"}})
	require.Error(t, err)
	assert.Equal(t, 1, llm.calls)
}

func TestExtractGraphData_RetriesIncompleteJSONWithoutFinishReason(t *testing.T) {
	llm := &scriptedGraphLLMClient{responses: []*domain_llm.ChatResponse{
		{Content: `{"nodes":[{"name":"broken"`},
		{Content: `{"nodes":[{"name":"First","entity_type":"概念"}],"relations":[]}`},
		{Content: `{"nodes":[{"name":"Second","entity_type":"概念"}],"relations":[]}`},
	}}
	service := &documentProcessorService{llmClient: llm, idGenerator: id.NewIDGenerator()}

	graph, err := service.extractGraphData(context.Background(), []*domain_knowledge.Chunk{
		{ID: "c1", Content: "first"},
		{ID: "c2", Content: "second"},
		{ID: "c3", Content: "third"},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, llm.calls)
	assert.Len(t, graph.Node, 2)
}

func TestRebuildKnowledgeBaseGraph_PreservesOldGraphWhenExtractionFailed(t *testing.T) {
	repo := &mockGraphRepository{}
	service := &documentProcessorService{graphRepo: repo}
	namespace := domain_knowledge.NameSpace{TenantID: "1", KnowledgeBaseID: "kb-1"}

	err := service.replaceRebuiltGraph(context.Background(), namespace, nil, 1)
	require.NoError(t, err)
	assert.False(t, repo.replaceGraphCalled)
}

func TestRebuildKnowledgeBaseGraph_ReplacesOnceAfterSuccessfulExtraction(t *testing.T) {
	repo := &mockGraphRepository{}
	service := &documentProcessorService{graphRepo: repo}
	namespace := domain_knowledge.NameSpace{TenantID: "1", KnowledgeBaseID: "kb-1"}
	graphs := []*domain_knowledge.GraphData{{Node: []*domain_knowledge.GraphNode{{ID: "n1", Name: "Acme"}}}}

	err := service.replaceRebuiltGraph(context.Background(), namespace, graphs, 0)
	require.NoError(t, err)
	require.True(t, repo.replaceGraphCalled)
	require.Len(t, repo.replaceGraphData, 1)
	assert.Len(t, repo.replaceGraphData[0].Node, 1)
}
