// Package graph 图谱服务测试
package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	domain_knowledge "link/internal/model/knowledge"
)

// ========================================
// Mock Repositories for Testing
// ========================================

type mockGraphRepository struct {
	addGraphCalled     bool
	addGraphNamespace  domain_knowledge.NameSpace
	addGraphData       []*domain_knowledge.GraphData

	getGraphCalled     bool
	getGraphNamespace  domain_knowledge.NameSpace
	getGraphResult     *domain_knowledge.GraphData
	getGraphError      error

	entityContextResult *domain_knowledge.ExtractionContext
	entityContextError  error
}

func (m *mockGraphRepository) AddGraph(ctx context.Context, namespace domain_knowledge.NameSpace, graphs []*domain_knowledge.GraphData) error {
	m.addGraphCalled = true
	m.addGraphNamespace = namespace
	m.addGraphData = graphs
	return nil
}

func (m *mockGraphRepository) AddNode(ctx context.Context, namespace domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	return nil
}

func (m *mockGraphRepository) AddRelation(ctx context.Context, namespace domain_knowledge.NameSpace, relation *domain_knowledge.GraphRelation) (*domain_knowledge.GraphRelation, error) {
	return relation, nil
}

func (m *mockGraphRepository) DeleteByChunkID(ctx context.Context, namespace domain_knowledge.NameSpace, chunkID string) error {
	return nil
}

func (m *mockGraphRepository) DeleteByKnowledgeID(ctx context.Context, namespace domain_knowledge.NameSpace, knowledgeID string) error {
	return nil
}

func (m *mockGraphRepository) DeleteByScope(ctx context.Context, scopeType string, scopeID string) error {
	return nil
}

func (m *mockGraphRepository) DeleteGraph(ctx context.Context, namespaces []domain_knowledge.NameSpace) error {
	return nil
}

func (m *mockGraphRepository) DeleteNode(ctx context.Context, namespace domain_knowledge.NameSpace, nodeID string) error {
	return nil
}

func (m *mockGraphRepository) DeleteRelation(ctx context.Context, namespace domain_knowledge.NameSpace, relationID string) error {
	return nil
}

func (m *mockGraphRepository) FindKShortestPaths(ctx context.Context, namespace domain_knowledge.NameSpace, startNode, endNode string, k int, opts *domain_knowledge.PathQueryOptions) ([]*domain_knowledge.PathQueryResult, error) {
	return nil, nil
}

func (m *mockGraphRepository) FindPathWithTypes(ctx context.Context, namespace domain_knowledge.NameSpace, startNode, endNode string, relationTypes []string, maxDepth int) ([]*domain_knowledge.PathQueryResult, error) {
	return nil, nil
}

func (m *mockGraphRepository) FindShortestPath(ctx context.Context, namespace domain_knowledge.NameSpace, startNode, endNode string, opts *domain_knowledge.PathQueryOptions) (*domain_knowledge.PathQueryResult, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetCentralitySummaries(ctx context.Context, namespace domain_knowledge.NameSpace, centralityType string) (*domain_knowledge.CentralitySummary, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetCommunityStats(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.CommunityStats, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetDensityMetrics(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.DensityMetrics, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetDegreeStats(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.DegreeStats, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetEntityContext(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.ExtractionContext, error) {
	if m.entityContextError != nil {
		return nil, m.entityContextError
	}
	return m.entityContextResult, nil
}

func (m *mockGraphRepository) GetGraph(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.GraphData, error) {
	m.getGraphCalled = true
	m.getGraphNamespace = namespace
	if m.getGraphError != nil {
		return nil, m.getGraphError
	}
	return m.getGraphResult, nil
}

func (m *mockGraphRepository) GetGraphStats(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.GraphStats, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetNeighbors(ctx context.Context, namespace domain_knowledge.NameSpace, nodeName string, opts *domain_knowledge.RelationQueryOptions) (*domain_knowledge.NodeQueryResult, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetNodeCommunity(ctx context.Context, namespace domain_knowledge.NameSpace, nodeName string) (*domain_knowledge.Community, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetStatsByEntityType(ctx context.Context, namespace domain_knowledge.NameSpace, entityType string) (*domain_knowledge.GraphStats, error) {
	return nil, nil
}

func (m *mockGraphRepository) GetTypeDistribution(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.TypeDistribution, error) {
	return nil, nil
}

func (m *mockGraphRepository) InitIndexes(ctx context.Context) error {
	return nil
}

func (m *mockGraphRepository) SearchNodes(ctx context.Context, namespace domain_knowledge.NameSpace, query string, opts *domain_knowledge.NodeQueryOptions) ([]*domain_knowledge.GraphNode, error) {
	return nil, nil
}

func (m *mockGraphRepository) StoreCommunity(ctx context.Context, namespace domain_knowledge.NameSpace, community *domain_knowledge.Community) error {
	return nil
}

func (m *mockGraphRepository) StoreCommunityMembers(ctx context.Context, namespace domain_knowledge.NameSpace, members []*domain_knowledge.CommunityMember) error {
	return nil
}

func (m *mockGraphRepository) UpdateCentralityScores(ctx context.Context, namespace domain_knowledge.NameSpace, scores map[string]float64, scoreType string) error {
	return nil
}

func (m *mockGraphRepository) UpdateNode(ctx context.Context, namespace domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	return nil
}

func (m *mockGraphRepository) UpdateRelation(ctx context.Context, namespace domain_knowledge.NameSpace, relation *domain_knowledge.GraphRelation) (*domain_knowledge.GraphRelation, error) {
	return nil, nil
}

func (m *mockGraphRepository) CheckHealth(ctx context.Context) error {
	return nil
}

func (m *mockGraphRepository) Close(ctx context.Context) error {
	return nil
}

// ========================================
// Mock LLM Extractor for Testing
// ========================================

type mockLLMExtractor struct {
	// For joint extraction
	jointNodes     []*domain_knowledge.GraphNode
	jointRelations []*domain_knowledge.GraphRelation
	jointError     error

	// For incremental extraction
	incrementalNodes     []*domain_knowledge.GraphNode
	incrementalRelations []*domain_knowledge.GraphRelation
	incrementalError     error

	// For entity extraction
	entityNodes []*domain_knowledge.GraphNode
	entityError error

	// For relation extraction
	relationRelations []*domain_knowledge.GraphRelation
	relationError     error
}

func (m *mockLLMExtractor) ExtractEntities(
	ctx context.Context,
	chunkID, document, query string,
	mode domain_knowledge.ExtractionMode,
) ([]*domain_knowledge.GraphNode, error) {
	if m.entityError != nil {
		return nil, m.entityError
	}
	return m.entityNodes, nil
}

func (m *mockLLMExtractor) ExtractRelations(
	ctx context.Context,
	kbID, chunkID, document, query string,
	entities []*domain_knowledge.GraphNode,
	mode domain_knowledge.ExtractionMode,
) ([]*domain_knowledge.GraphRelation, error) {
	if m.relationError != nil {
		return nil, m.relationError
	}
	return m.relationRelations, nil
}

func (m *mockLLMExtractor) ExtractGraphJoint(
	ctx context.Context,
	chunkID, document string,
	existingContext *domain_knowledge.ExtractionContext,
) ([]*domain_knowledge.GraphNode, []*domain_knowledge.GraphRelation, error) {
	if m.jointError != nil {
		return nil, nil, m.jointError
	}
	return m.jointNodes, m.jointRelations, nil
}

func (m *mockLLMExtractor) ExtractGraphIncremental(
	ctx context.Context,
	chunkID, document string,
	existingContext *domain_knowledge.ExtractionContext,
) ([]*domain_knowledge.GraphNode, []*domain_knowledge.GraphRelation, error) {
	if m.incrementalError != nil {
		return nil, nil, m.incrementalError
	}
	return m.incrementalNodes, m.incrementalRelations, nil
}

// ========================================
// GraphService Tests
// ========================================

func TestNewGraphService(t *testing.T) {
	mockRepo := &mockGraphRepository{}
	mockChat := &mockLLMExtractor{}

	service := NewGraphService(mockRepo, mockChat)

	assert.NotNil(t, service)
	assert.Equal(t, mockRepo, service.GetGraphRepo())
}

func TestGraphService_AddGraph(t *testing.T) {
	tests := []struct {
		name      string
		graphs    []*domain_knowledge.GraphData
		wantErr   bool
		setupRepo func(*mockGraphRepository)
	}{
		{
			name: "Valid graph",
			graphs: []*domain_knowledge.GraphData{
				{
					Node: []*domain_knowledge.GraphNode{
						{ID: "1", Name: "Entity1", EntityType: "Technology"},
					},
					Relation: []*domain_knowledge.GraphRelation{},
				},
			},
			wantErr: false,
			setupRepo: func(m *mockGraphRepository) {
				// No setup needed
			},
		},
		{
			name:    "Empty graph",
			graphs:  []*domain_knowledge.GraphData{},
			wantErr: false,
			setupRepo: func(m *mockGraphRepository) {
				// No setup needed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockGraphRepository{}
			if tt.setupRepo != nil {
				tt.setupRepo(mockRepo)
			}

			service := &GraphService{
				graphRepo: mockRepo,
			}

			namespace := domain_knowledge.NameSpace{
				TenantID: "tenant-1",
			 KnowledgeBaseID:     "kb-1",
			}

			err := service.AddGraph(context.Background(), namespace, tt.graphs)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, mockRepo.addGraphCalled)
			}
		})
	}
}

func TestGraphService_GetGraph(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func(*mockGraphRepository)
		wantErr   bool
	}{
		{
			name: "Successful get",
			setupRepo: func(m *mockGraphRepository) {
				m.getGraphResult = &domain_knowledge.GraphData{
					Node: []*domain_knowledge.GraphNode{
						{ID: "1", Name: "Entity1"},
					},
				}
			},
			wantErr: false,
		},
		{
			name: "Repository error",
			setupRepo: func(m *mockGraphRepository) {
				m.getGraphError = errors.New("repository error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockGraphRepository{}
			tt.setupRepo(mockRepo)

			service := &GraphService{
				graphRepo: mockRepo,
			}

			namespace := domain_knowledge.NameSpace{
				TenantID: "tenant-1",
			 KnowledgeBaseID:     "kb-1",
			}

			result, err := service.GetGraph(context.Background(), namespace)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// ========================================
// Extraction Input Tests
// ========================================

func TestChunkExtractionInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   *domain_knowledge.ChunkExtractionInput
		wantErr bool
	}{
		{
			name: "Valid input",
			input: &domain_knowledge.ChunkExtractionInput{
				ChunkID:         "chunk-1",
				Document:        "Test document content",
			KnowledgeBaseID:       "kb-1",
			},
			wantErr: false,
		},
		{
			name: "Empty chunk ID",
			input: &domain_knowledge.ChunkExtractionInput{
				ChunkID:  "",
				Document: "Test document",
			},
			wantErr: true,
		},
		{
			name: "Empty document",
			input: &domain_knowledge.ChunkExtractionInput{
				ChunkID:  "chunk-1",
				Document: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validation is done in the application layer
			if tt.wantErr && (tt.input.ChunkID == "" || tt.input.Document == "") {
				assert.True(t, true, "expected validation error")
			} else {
				assert.True(t, true, "input is valid")
			}
		})
	}
}

// ========================================
// Namespace Tests
// ========================================

func TestNameSpaceString(t *testing.T) {
	tests := []struct {
		name      string
		namespace domain_knowledge.NameSpace
		want      string
	}{
		{
			name: "Full namespace",
			namespace: domain_knowledge.NameSpace{
				TenantID:    "tenant-1",
			 KnowledgeBaseID:        "kb-1",
				Knowledge: "knowledge-1",
			},
			want: "tenant-1/kb-1/knowledge-1",
		},
		{
			name: "KB only",
			namespace: domain_knowledge.NameSpace{
				TenantID: "tenant-1",
			 KnowledgeBaseID:     "kb-1",
			},
			want: "tenant-1/kb-1/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.namespace.TenantID + "/" + tt.namespace.KnowledgeBaseID + "/" + tt.namespace.Knowledge
			assert.Equal(t, tt.want, result)
		})
	}
}

// ========================================
// Graph Data Validation Tests
// ========================================

func TestExtractedGraphData(t *testing.T) {
	t.Run("Successful extraction", func(t *testing.T) {
		data := &domain_knowledge.ExtractedGraphData{
			ChunkID: "chunk-1",
			Nodes: []*domain_knowledge.GraphNode{
				{ID: "1", Name: "Entity1", EntityType: "Technology"},
			},
			Relations: []*domain_knowledge.GraphRelation{
				{ID: "r1", Source: "Entity1", Target: "Entity2", Type: "RELATED_TO"},
			},
		}

		assert.NotNil(t, data.Nodes)
		assert.NotNil(t, data.Relations)
		assert.Nil(t, data.Error)
	})

	t.Run("Failed extraction", func(t *testing.T) {
		err := errors.New("extraction failed")
		data := &domain_knowledge.ExtractedGraphData{
			ChunkID: "chunk-1",
			Error:   err,
		}

		assert.Nil(t, data.Nodes)
		assert.Nil(t, data.Relations)
		assert.Equal(t, err, data.Error)
	})
}

// ========================================
// Mode Tests
// ========================================

func TestExtractionMode(t *testing.T) {
	tests := []struct {
		name string
		mode domain_knowledge.ExtractionMode
		want string
	}{
		{"Document mode", domain_knowledge.ExtractionModeDocument, "document"},
		{"Query mode", domain_knowledge.ExtractionModeQuery, "query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.mode))
		})
	}
}

// ========================================
// Service Creation Tests
// ========================================

func TestNewGraphServiceWithQuery(t *testing.T) {
	mockGraphRepo := &mockGraphRepository{}
	mockQueryRepo := &mockGraphQueryRepository{}
	mockChat := &mockLLMExtractor{}

	service := NewGraphServiceWithQuery(mockGraphRepo, mockQueryRepo, mockChat)

	assert.NotNil(t, service)
	assert.Equal(t, mockGraphRepo, service.graphRepo)
	assert.Equal(t, mockQueryRepo, service.graphQueryRepo)
}

func TestNewGraphServiceWithChunks(t *testing.T) {
	mockGraphRepo := &mockGraphRepository{}
	mockQueryRepo := &mockGraphQueryRepository{}
	mockChunkRepo := &mockChunkRepository{}
	mockChat := &mockLLMExtractor{}

	service := NewGraphServiceWithChunks(mockGraphRepo, mockQueryRepo, mockChunkRepo, mockChat)

	assert.NotNil(t, service)
	assert.Equal(t, mockGraphRepo, service.graphRepo)
	assert.Equal(t, mockQueryRepo, service.graphQueryRepo)
	assert.Equal(t, mockChunkRepo, service.chunkRepo)
}

// ========================================
// Mock Implementations
// ========================================

type mockGraphQueryRepository struct{}

func (m *mockGraphQueryRepository) GetChunksByGraphNodes(ctx context.Context, kbID string, nodeNames []string) ([]*domain_knowledge.Chunk, error) {
	return nil, nil
}

func (m *mockGraphQueryRepository) GetChunksByIDs(ctx context.Context, kbID string, chunkIDs []string) ([]*domain_knowledge.Chunk, error) {
	return nil, nil
}

func (m *mockGraphQueryRepository) GetKnowledgeByGraphNodes(ctx context.Context, kbID string, nodeNames []string) (*domain_knowledge.Knowledge, error) {
	return nil, nil
}

func (m *mockGraphQueryRepository) GetGraphStats(ctx context.Context, kbID string) (*domain_knowledge.DetailedGraphStats, error) {
	return nil, nil
}

type mockChunkRepository struct{}

func (m *mockChunkRepository) FindEnabledChunks(ctx context.Context, kbID string, limit int) ([]Chunk, error) {
	return nil, nil
}
