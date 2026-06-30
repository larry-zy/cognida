package rag

import (
	"encoding/json"
	"testing"
)

// TestDocument 测试文档实体
func TestDocument(t *testing.T) {
	doc := &Document{
		ChunkID:      "chunk-123",
		KnowledgeID:  "kb-456",
	 KnowledgeBaseID:         "test-kb",
		Content:      "This is a test document content",
		Score:        0.95,
		MatchType:    "hybrid",
		ChunkIndex:   1,
		Metadata:     map[string]interface{}{"source": "test.pdf"},
	}

	if doc.ChunkID != "chunk-123" {
		t.Errorf("Expected ChunkID 'chunk-123', got '%s'", doc.ChunkID)
	}
	if doc.Score != 0.95 {
		t.Errorf("Expected Score 0.95, got %f", doc.Score)
	}
	if doc.MatchType != "hybrid" {
		t.Errorf("Expected MatchType 'hybrid', got '%s'", doc.MatchType)
	}
}

// TestDocumentJSONSerialization 测试文档JSON序列化
func TestDocumentJSONSerialization(t *testing.T) {
	doc := &Document{
		ChunkID:   "chunk-1",
		Content:   "Test content",
		Score:     0.85,
		MatchType: "vector",
		Metadata:  map[string]interface{}{"key": "value"},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var unmarshaled Document
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.ChunkID != doc.ChunkID {
		t.Errorf("ChunkID mismatch: got %q, want %q", unmarshaled.ChunkID, doc.ChunkID)
	}
	if unmarshaled.Content != doc.Content {
		t.Errorf("Content mismatch: got %q, want %q", unmarshaled.Content, doc.Content)
	}
}

// TestPipelineConfig 测试管道配置
func TestPipelineConfig(t *testing.T) {
	config := DefaultPipelineConfig()

	if config.TopK != 10 {
		t.Errorf("Expected default TopK 10, got %d", config.TopK)
	}
	if config.SimilarityThreshold != 0.5 {
		t.Errorf("Expected default SimilarityThreshold 0.5, got %f", config.SimilarityThreshold)
	}
	if config.RetrievalMode != "hybrid" {
		t.Errorf("Expected default RetrievalMode 'hybrid', got '%s'", config.RetrievalMode)
	}
	if config.EnableRerank != true {
		t.Errorf("Expected default EnableRerank true, got %v", config.EnableRerank)
	}
}

// TestRetrieveOptions 测试检索选项
func TestRetrieveOptions(t *testing.T) {
	opts := DefaultRetrieveOptions()

	if opts.TopK != 10 {
		t.Errorf("Expected default TopK 10, got %d", opts.TopK)
	}
	if opts.SimilarityThreshold != 0.5 {
		t.Errorf("Expected default SimilarityThreshold 0.5, got %f", opts.SimilarityThreshold)
	}
	if opts.RerankEnabled != true {
		t.Errorf("Expected default RerankEnabled true, got %v", opts.RerankEnabled)
	}
}

// TestRetrieveResponse 测试检索响应
func TestRetrieveResponse(t *testing.T) {
	resp := &RetrieveResponse{
		Results: []*Document{
			{
				ChunkID: "chunk-1",
				Content: "Content 1",
				Score:   0.9,
			},
			{
				ChunkID: "chunk-2",
				Content: "Content 2",
				Score:   0.8,
			},
		},
		Query:      "test query",
		TotalCount: 2,
		HasMore:    false,
		Latency:    100,
	}

	if len(resp.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(resp.Results))
	}
	if resp.TotalCount != 2 {
		t.Errorf("Expected TotalCount 2, got %d", resp.TotalCount)
	}
	if resp.HasMore != false {
		t.Errorf("Expected HasMore false, got %v", resp.HasMore)
	}
}

// TestGraphNode 测试图谱节点
func TestGraphNode(t *testing.T) {
	node := &GraphNode{
		ID:         "node-1",
		Name:       "测试实体",
		EntityType: "PERSON",
		Attributes: []string{"attr1", "attr2"},
		Chunks:     []string{"chunk-1", "chunk-2"},
		Properties: map[string]string{"key": "value"},
	}

	if node.ID != "node-1" {
		t.Errorf("Expected ID 'node-1', got '%s'", node.ID)
	}
	if node.EntityType != "PERSON" {
		t.Errorf("Expected EntityType 'PERSON', got '%s'", node.EntityType)
	}
	if len(node.Attributes) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(node.Attributes))
	}
}

// TestGraphRelation 测试图谱关系
func TestGraphRelation(t *testing.T) {
	relation := &GraphRelation{
		ID:             "rel-1",
		Source:         "node-1",
		Target:         "node-2",
		Type:           "KNOWS",
		Strength:       8.5,
		Weight:         0.9,
		ChunkIDs:       []string{"chunk-1"},
		Properties:     map[string]string{"since": "2020"},
		CombinedDegree: 5,
	}

	if relation.Source != "node-1" {
		t.Errorf("Expected Source 'node-1', got '%s'", relation.Source)
	}
	if relation.Target != "node-2" {
		t.Errorf("Expected Target 'node-2', got '%s'", relation.Target)
	}
	if relation.Type != "KNOWS" {
		t.Errorf("Expected Type 'KNOWS', got '%s'", relation.Type)
	}
	if relation.Strength != 8.5 {
		t.Errorf("Expected Strength 8.5, got %f", relation.Strength)
	}
}

// TestConversationMessage 测试对话消息
func TestConversationMessage(t *testing.T) {
	msg := ConversationMessage{
		Role:    "user",
		Content: "What is RAG?",
	}

	if msg.Role != "user" {
		t.Errorf("Expected Role 'user', got '%s'", msg.Role)
	}
	if msg.Content != "What is RAG?" {
		t.Errorf("Expected Content 'What is RAG?', got '%s'", msg.Content)
	}
}

// TestRAGContext 测试RAG上下文
func TestRAGContext(t *testing.T) {
	ctx := &RAGContext{
		Query:          "test query",
		RewrittenQuery: "rewritten query",
		SubQueries:     []string{"sub1", "sub2"},
		Documents: []*Document{
			{ChunkID: "chunk-1", Content: "doc1"},
		},
		ConversationHistory: []ConversationMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	if ctx.Query != "test query" {
		t.Errorf("Expected Query 'test query', got '%s'", ctx.Query)
	}
	if len(ctx.SubQueries) != 2 {
		t.Errorf("Expected 2 sub-queries, got %d", len(ctx.SubQueries))
	}
	if len(ctx.Documents) != 1 {
		t.Errorf("Expected 1 document, got %d", len(ctx.Documents))
	}
	if len(ctx.ConversationHistory) != 1 {
		t.Errorf("Expected 1 history message, got %d", len(ctx.ConversationHistory))
	}
}

// TestStrengthenedQuery 测试增强后的查询
func TestStrengthenedQuery(t *testing.T) {
	sq := &StrengthenedQuery{
		OriginalQuery:  "original",
		RewrittenQuery: "rewritten",
		SubQueries:     []string{"sub1", "sub2"},
		RewriteApplied: true,
		SplitApplied:   true,
		ProcessingTime: 100,
	}

	queries := sq.GetQueriesForRetrieve()
	// 应该返回: sub1, sub2, rewritten, original
	if len(queries) != 4 {
		t.Errorf("Expected 4 queries, got %d", len(queries))
	}
}

// TestStrengthenedQuery_Empty 测试空增强查询
func TestStrengthenedQuery_Empty(t *testing.T) {
	sq := &StrengthenedQuery{
		OriginalQuery: "original",
		// 没有重写和拆分
	}

	queries := sq.GetQueriesForRetrieve()
	// 应该只返回原查询
	if len(queries) != 1 {
		t.Errorf("Expected 1 query, got %d", len(queries))
	}
	if queries[0] != "original" {
		t.Errorf("Expected query 'original', got '%s'", queries[0])
	}
}

// TestGraphData 测试图谱数据
func TestGraphData(t *testing.T) {
	data := &GraphData{
		Node: []*GraphNode{
			{ID: "node-1", Name: "Entity 1"},
			{ID: "node-2", Name: "Entity 2"},
		},
		Relation: []*GraphRelation{
			{ID: "rel-1", Source: "node-1", Target: "node-2", Type: "RELATED"},
		},
	}

	if len(data.Node) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(data.Node))
	}
	if len(data.Relation) != 1 {
		t.Errorf("Expected 1 relation, got %d", len(data.Relation))
	}
}

// TestSearchTrace 测试检索追踪
func TestSearchTrace(t *testing.T) {
	trace := &SearchTrace{
		VectorResultCount: 10,
		BM25ResultCount:   5,
		GraphResultCount:  2,
		RerankedCount:     8,
		VectorLatency:     100,
		BM25Latency:       50,
		GraphLatency:      30,
		RerankLatency:     20,
		RetrievalDetails: []RetrievalStep{
			{
				StepType:    "vector_search",
				Description: "Vector search completed",
				Latency:     100,
				ResultCount: 10,
			},
		},
	}

	if trace.VectorResultCount != 10 {
		t.Errorf("Expected VectorResultCount 10, got %d", trace.VectorResultCount)
	}
	// TotalLatency 可以通过计算各延迟之和得出
	totalLatency := trace.VectorLatency + trace.BM25Latency + trace.GraphLatency + trace.RerankLatency
	if totalLatency != 200 { // 100 + 50 + 30 + 20
		t.Errorf("Expected total latency 200, got %d", totalLatency)
	}
	if len(trace.RetrievalDetails) != 1 {
		t.Errorf("Expected 1 detail step, got %d", len(trace.RetrievalDetails))
	}
}
