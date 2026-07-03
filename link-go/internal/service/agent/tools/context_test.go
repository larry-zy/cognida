package tools

import (
	"context"
	"sync"
	"testing"

	"link/internal/model/knowledge"
)

// mockRAGService 模拟 RAG 服务
type mockRAGService struct{}

func (m *mockRAGService) Query(ctx interface{}, req *RAGQueryRequest) (*RAGQueryResult, error) {
	return &RAGQueryResult{
		Answer: "mock answer",
		Chunks: []DocumentChunk{},
		Count:  0,
	 KnowledgeBaseID:   req.KnowledgeBaseID,
		Query:  req.Query,
	}, nil
}

// mockGraphService 模拟图谱服务
type mockGraphService struct{}

func (m *mockGraphService) GraphQuery(ctx interface{}, req *GraphQueryRequest) (*GraphQueryResult, error) {
	return &GraphQueryResult{
		Answer:    "mock answer",
		Entities:  []GraphEntity{},
		Relations: []GraphRelation{},
		Count:     0,
	 KnowledgeBaseID:      req.KnowledgeBaseID,
		Query:     req.Query,
	}, nil
}

// mockKBRepo 模拟知识库仓库（实现完整接口用于测试）
type mockKBRepo struct{}

func (m *mockKBRepo) Create(ctx context.Context, kb *knowledge.KnowledgeBase) error {
	return nil
}

func (m *mockKBRepo) FindByID(ctx context.Context, id string) (*knowledge.KnowledgeBase, error) {
	return &knowledge.KnowledgeBase{ID: id, Name: "mock kb"}, nil
}

func (m *mockKBRepo) FindByTenantID(ctx context.Context, tenantID int64, page, pageSize int) ([]*knowledge.KnowledgeBase, int64, error) {
	return []*knowledge.KnowledgeBase{}, 0, nil
}

func (m *mockKBRepo) FindByUser(ctx context.Context, userID int64, page, pageSize int) ([]*knowledge.KnowledgeBase, int64, error) {
	return []*knowledge.KnowledgeBase{}, 0, nil
}

func (m *mockKBRepo) Update(ctx context.Context, kb *knowledge.KnowledgeBase) error {
	return nil
}

func (m *mockKBRepo) UpdateStats(ctx context.Context, knowledgeBaseID string, documentCount, chunkCount int, storageSize int64) error {
	return nil
}

func (m *mockKBRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockKBRepo) HardDelete(ctx context.Context, id string) error {
	return nil
}

func (m *mockKBRepo) Exists(ctx context.Context, id string) (bool, error) {
	return true, nil
}

func (m *mockKBRepo) GetStorageStats(ctx context.Context, tenantID int64) (totalSize, kbCount int64, err error) {
	return 0, 0, nil
}

// mockDataService 模拟数据查询服务
type mockDataService struct{}

func (m *mockDataService) NaturalLanguageToSQL(ctx interface{}, query string, schema interface{}) (string, error) {
	return "SELECT * FROM users LIMIT 10", nil
}

func (m *mockDataService) ExecuteQuery(ctx interface{}, sql string, params map[string]interface{}) (*SQLExecuteResult, error) {
	return &SQLExecuteResult{
		Columns:  []string{"id", "name"},
		Samples:  []map[string]interface{}{{"id": 1, "name": "test"}},
		RowCount: 1,
	}, nil
}

func (m *mockDataService) GetDatabaseSchema(ctx interface{}, dbID string) (interface{}, error) {
	return map[string]interface{}{
		"database": "test_db",
		"tables":   []interface{}{},
	}, nil
}

func (m *mockDataService) ValidateQuery(sql string) error {
	return nil
}

// TestToolContext_Concurrency 测试并发安全
func TestToolContext_Concurrency(t *testing.T) {
	ctx := NewToolContext()
	mockSvc := &mockRAGService{}

	// 并发写入
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx.WithRAGService(mockSvc)
		}()
	}
	wg.Wait()

	// 验证服务已设置
	if ctx.GetRAGService() == nil {
		t.Error("RAGService should not be nil after concurrent writes")
	}
}

// TestToolContext_Chaining 测试链式调用
func TestToolContext_Chaining(t *testing.T) {
	mockRAG := &mockRAGService{}
	mockGraph := &mockGraphService{}
	mockKB := &mockKBRepo{}
	mockData := &mockDataService{}

	ctx := NewToolContext().
		WithRAGService(mockRAG).
		WithGraphService(mockGraph).
		WithKBRepo(mockKB).
		WithDataService(mockData)

	if ctx.GetRAGService() == nil {
		t.Error("RAGService not set")
	}
	if ctx.GetGraphService() == nil {
		t.Error("GraphService not set")
	}
	if ctx.GetKBRepo() == nil {
		t.Error("KBRepo not set")
	}
	if ctx.GetDataService() == nil {
		t.Error("DataService not set")
	}
}

// TestToolContext_GlobalContext 测试全局上下文
func TestToolContext_GlobalContext(t *testing.T) {
	ctx1 := GetGlobalContext()
	ctx2 := GetGlobalContext()

	if ctx1 != ctx2 {
		t.Error("GlobalContext should return the same instance")
	}
}
