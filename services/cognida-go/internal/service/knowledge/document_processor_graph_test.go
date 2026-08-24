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

// rebuild 流程只依赖这两个仓储的读取能力；内嵌接口让 mock 保持最小，
// 未覆写的方法一旦被调用会 panic，测试失败即暴露越界依赖。
type rebuildKnowledgeRepo struct {
	domain_knowledge.KnowledgeRepository
	docs []*domain_knowledge.Knowledge
}

func (r *rebuildKnowledgeRepo) FindByKnowledgeBaseID(context.Context, string, *domain_knowledge.KnowledgeListQuery) ([]*domain_knowledge.Knowledge, int64, error) {
	return r.docs, int64(len(r.docs)), nil
}

type rebuildChunkRepo struct {
	domain_knowledge.ChunkRepository
	chunksByDoc map[string][]*domain_knowledge.Chunk
	failDoc     string
}

func (r *rebuildChunkRepo) FindByKnowledgeID(_ context.Context, knowledgeID string, _ bool) ([]*domain_knowledge.Chunk, error) {
	if r.failDoc == knowledgeID {
		return nil, fmt.Errorf("chunk store unavailable")
	}
	return r.chunksByDoc[knowledgeID], nil
}

func TestRebuildKnowledgeBaseGraph_PreservesOldGraphWhenExtractionFailed(t *testing.T) {
	repo := &mockGraphRepository{}
	service := &documentProcessorService{graphRepo: repo}
	namespace := domain_knowledge.NameSpace{TenantID: "1", KnowledgeBaseID: "kb-1"}
	merged := &domain_knowledge.GraphData{Node: []*domain_knowledge.GraphNode{{ID: "n1", Name: "Acme"}}}

	replaced, err := service.replaceRebuiltGraph(context.Background(), namespace, merged, 1)
	require.NoError(t, err)
	assert.False(t, replaced)
	assert.False(t, repo.replaceGraphCalled)
}

func TestRebuildKnowledgeBaseGraph_ReplacesOnceAfterSuccessfulExtraction(t *testing.T) {
	repo := &mockGraphRepository{}
	service := &documentProcessorService{graphRepo: repo}
	namespace := domain_knowledge.NameSpace{TenantID: "1", KnowledgeBaseID: "kb-1"}
	merged := &domain_knowledge.GraphData{Node: []*domain_knowledge.GraphNode{{ID: "n1", Name: "Acme"}}}

	replaced, err := service.replaceRebuiltGraph(context.Background(), namespace, merged, 0)
	require.NoError(t, err)
	require.True(t, replaced)
	require.True(t, repo.replaceGraphCalled)
	require.Len(t, repo.replaceGraphData, 1)
	assert.Len(t, repo.replaceGraphData[0].Node, 1)
}

func TestRebuildKnowledgeBaseGraph_SkipsReplacementWhenMergedGraphEmpty(t *testing.T) {
	repo := &mockGraphRepository{}
	service := &documentProcessorService{graphRepo: repo}
	namespace := domain_knowledge.NameSpace{TenantID: "1", KnowledgeBaseID: "kb-1"}

	replaced, err := service.replaceRebuiltGraph(context.Background(), namespace, &domain_knowledge.GraphData{}, 0)
	require.NoError(t, err)
	assert.False(t, replaced)
	assert.False(t, repo.replaceGraphCalled)
}

// 回归评审指出的问题：跨文档存在重复实体时，响应计数必须等于合并去重后
// 实际写入的数量，而不是逐文档原始计数的累加。
func TestRebuildKnowledgeBaseGraph_ReportsDeduplicatedCounts(t *testing.T) {
	llm := &scriptedGraphLLMClient{responses: []*domain_llm.ChatResponse{
		// doc-1 抽到 Acme、Beta
		{Content: `{"nodes":[{"name":"Acme","entity_type":"组织"},{"name":"Beta","entity_type":"概念"}],"relations":[{"source":"Acme","target":"Beta","type":"RELATED_TO","strength":3}]}`},
		// doc-2 抽到 acme（归一化后与 Acme 重复）、Gamma
		{Content: `{"nodes":[{"name":"acme","entity_type":"组织"},{"name":"Gamma","entity_type":"概念"}],"relations":[{"source":"acme","target":"Gamma","type":"RELATED_TO","strength":5}]}`},
	}}
	graphRepo := &mockGraphRepository{}
	service := &documentProcessorService{
		graphRepo:   graphRepo,
		llmClient:   llm,
		idGenerator: id.NewIDGenerator(),
		knowledgeRepo: &rebuildKnowledgeRepo{docs: []*domain_knowledge.Knowledge{
			{ID: "doc-1", ParseStatus: domain_knowledge.ParseStatusCompleted},
			{ID: "doc-2", ParseStatus: domain_knowledge.ParseStatusCompleted},
		}},
		chunkRepo: &rebuildChunkRepo{chunksByDoc: map[string][]*domain_knowledge.Chunk{
			"doc-1": {{ID: "c1", Content: "first"}},
			"doc-2": {{ID: "c2", Content: "second"}},
		}},
	}

	resp, err := service.RebuildKnowledgeBaseGraph(context.Background(), 1, "kb-1")
	require.NoError(t, err)

	assert.True(t, resp.GraphReplaced)
	assert.Equal(t, 2, resp.ProcessedDocuments)
	// 逐文档累加会得到 4 个节点；实际写入的是去重后的 Acme/Beta/Gamma = 3
	assert.Equal(t, 3, resp.TotalNodes)
	assert.Equal(t, 2, resp.TotalRelations)
	require.True(t, graphRepo.replaceGraphCalled)
	require.Len(t, graphRepo.replaceGraphData, 1)
	assert.Len(t, graphRepo.replaceGraphData[0].Node, 3)
	assert.Len(t, graphRepo.replaceGraphData[0].Relation, 2)
}

// 回归评审指出的"假成功"：存在失败文档时图谱未替换，响应必须如实反映，
// 不能带着节点计数伪装成已写入。
func TestRebuildKnowledgeBaseGraph_ZeroCountsWhenReplacementSkipped(t *testing.T) {
	llm := &scriptedGraphLLMClient{responses: []*domain_llm.ChatResponse{
		{Content: `{"nodes":[{"name":"Acme","entity_type":"组织"}],"relations":[]}`},
	}}
	graphRepo := &mockGraphRepository{}
	service := &documentProcessorService{
		graphRepo:   graphRepo,
		llmClient:   llm,
		idGenerator: id.NewIDGenerator(),
		knowledgeRepo: &rebuildKnowledgeRepo{docs: []*domain_knowledge.Knowledge{
			{ID: "doc-1", ParseStatus: domain_knowledge.ParseStatusCompleted},
			{ID: "doc-2", ParseStatus: domain_knowledge.ParseStatusCompleted},
		}},
		// doc-1 正常抽取；doc-2 读块失败制造 failed 文档
		chunkRepo: &rebuildChunkRepo{
			chunksByDoc: map[string][]*domain_knowledge.Chunk{
				"doc-1": {{ID: "c1", Content: "first"}},
			},
			failDoc: "doc-2",
		},
	}

	resp, err := service.RebuildKnowledgeBaseGraph(context.Background(), 1, "kb-1")
	require.NoError(t, err)

	assert.Equal(t, 1, resp.ProcessedDocuments)
	assert.Equal(t, 1, resp.FailedDocuments)
	assert.False(t, resp.GraphReplaced)
	assert.Equal(t, 0, resp.TotalNodes)
	assert.Equal(t, 0, resp.TotalRelations)
	assert.False(t, graphRepo.replaceGraphCalled)
}
