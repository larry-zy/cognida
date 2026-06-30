// Package neo4j 图谱仓储集成测试
package neo4j

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/joho/godotenv"

	"link/internal/model/knowledge"
)

func init() {
	// 加载 .env 文件
	_ = os.Setenv("NEO4J_URI", "bolt://localhost:7687")
	_ = os.Setenv("NEO4J_USERNAME", "neo4j")
	_ = os.Setenv("NEO4J_PASSWORD", "larry12345")
	_ = os.Setenv("NEO4J_DATABASE", "neo4j")
}

// getTestConfig 从环境变量获取测试配置
func getTestConfig() (uri, username, password, dbName string) {
	uri = os.Getenv("NEO4J_URI")
	username = os.Getenv("NEO4J_USERNAME")
	password = os.Getenv("NEO4J_PASSWORD")
	dbName = os.Getenv("NEO4J_DATABASE")

	if uri == "" {
		uri = "bolt://localhost:7687"
	}
	if username == "" {
		username = "neo4j"
	}
	if dbName == "" {
		dbName = "neo4j"
	}
	return
}

// setupTestRepo 创建测试用的仓储实例
func setupTestRepo(t *testing.T) *Neo4jRepository {
	uri, username, password, dbName := getTestConfig()

	t.Logf("Connecting to Neo4j: %s (user: %s, db: %s)", uri, username, dbName)

	repo, err := NewNeo4jRepository(uri, username, password, dbName)
	if err != nil {
		t.Skipf("Neo4j not available or connection failed: %v", err)
		return nil
	}

	return repo
}

// cleanupTestData 清理测试数据
func cleanupTestData(t *testing.T, ctx context.Context, repo *Neo4jRepository, namespace knowledge.NameSpace) {
	t.Log("Cleaning up test data...")

	// 删除测试命名空间下的所有数据
	err := repo.DeleteByScope(ctx, "kb_id", namespace.KnowledgeBaseID)
	if err != nil {
		t.Logf("Warning: cleanup failed: %v", err)
	}
}

// TestRealConnection 测试真实 Neo4j 连接
func TestRealConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()

	// 测试健康检查
	err := repo.CheckHealth(ctx)
	assert.NoError(t, err, "Health check should succeed")
}

// TestRealAddGraph 测试真实添加图谱数据
func TestRealAddGraph(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-" + t.Name(),
	}

	// 清理可能存在的旧数据
	cleanupTestData(t, ctx, repo, namespace)

	// 创建测试图谱数据
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{
				ID:         "node-1",
				Name:       "Python",
				
				EntityType: "Technology",
				Attributes: []string{"编程语言", "动态类型"},
				Chunks:     []string{"chunk-1"},
			},
			{
				ID:         "node-2",
				Name:       "Django",
				
				EntityType: "Technology",
				Attributes: []string{"Web框架"},
				Chunks:     []string{"chunk-1"},
			},
			{
				ID:         "node-3",
				Name:       "Flask",
				
				EntityType: "Technology",
				Attributes: []string{"微框架"},
				Chunks:     []string{"chunk-1"},
			},
		},
		Relation: []*knowledge.GraphRelation{
			{
				ID:          "rel-1",
				Source:      "Django",
				Target:      "Python",
				Type:        "RELATED_TO",
				Description: "Django 是 Python 的 Web 框架",
				Strength:    9.0,
				Weight:      8.5,
			},
			{
				ID:          "rel-2",
				Source:      "Flask",
				Target:      "Python",
				Type:        "RELATED_TO",
				Description: "Flask 是 Python 的微框架",
				Strength:    8.5,
				Weight:      8.0,
			},
		},
	}

	// 添加图谱
	t.Log("Adding graph data...")
	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err, "AddGraph should succeed")

	// 验证数据已添加
	t.Log("Retrieving graph data...")
	retrieved, err := repo.GetGraph(ctx, namespace)
	require.NoError(t, err, "GetGraph should succeed")

	assert.GreaterOrEqual(t, len(retrieved.Node), 3, "Should have at least 3 nodes")
	assert.GreaterOrEqual(t, len(retrieved.Relation), 2, "Should have at least 2 relations")

	t.Logf("Retrieved %d nodes and %d relations", len(retrieved.Node), len(retrieved.Relation))

	// 清理测试数据
	cleanupTestData(t, ctx, repo, namespace)
}

// TestRealSearchNodes 测试真实节点搜索
func TestRealSearchNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-search",
	}

	// 清理可能存在的旧数据
	cleanupTestData(t, ctx, repo, namespace)

	// 添加测试数据
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "n1", Name: "Python", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n2", Name: "Java", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n3", Name: "Go", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n4", Name: "Django", EntityType: "Technology", Chunks: []string{"c2"}},
		},
		Relation: []*knowledge.GraphRelation{},
	}

	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err)

	// 测试搜索
	t.Log("Searching for nodes with 'Python'...")
	nodes, err := repo.SearchNodes(ctx, namespace, "Python", &knowledge.NodeQueryOptions{
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Greater(t, len(nodes), 0, "Should find nodes matching 'Python'")

	t.Logf("Found %d nodes", len(nodes))
	for _, node := range nodes {
		t.Logf("  - %s (%s): %s", node.Name, node.EntityType, node.Name)
	}

	// 测试按实体类型过滤
	t.Log("Searching for Technology nodes...")
	nodes, err = repo.SearchNodes(ctx, namespace, "", &knowledge.NodeQueryOptions{
		EntityTypes: []string{"Technology"},
		Limit:       10,
	})
	require.NoError(t, err)
	assert.Greater(t, len(nodes), 0, "Should find Technology nodes")

	// 清理
	cleanupTestData(t, ctx, repo, namespace)
}

// TestRealSearchNodesLimit 验证 SearchNodes 的结果集上限语义（针对默认上限兜底修复）：
//  1. 显式 Limit 必须被严格遵守；
//  2. 当 opts 非空但 Limit<=0 时，不再退化为无 LIMIT 的全量扫描，
//     而是应用默认上限，且查询正常返回不报错。
func TestRealSearchNodesLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID:        "test-tenant",
		KnowledgeBaseID: "test-kb-search-limit",
	}

	cleanupTestData(t, ctx, repo, namespace)

	// 构造多个共享子串 "Lang" 的节点，便于用 CONTAINS 命中多行。
	nodes := make([]*knowledge.GraphNode, 0, 6)
	for i, name := range []string{"LangA", "LangB", "LangC", "LangD", "LangE", "LangF"} {
		nodes = append(nodes, &knowledge.GraphNode{
			ID:         "ln-" + name,
			Name:       name,
			EntityType: "Technology",
			Chunks:     []string{"c1"},
		})
		_ = i
	}
	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{{Node: nodes}})
	require.NoError(t, err)

	// 1) 显式 Limit=2 必须严格生效。
	limited, err := repo.SearchNodes(ctx, namespace, "Lang", &knowledge.NodeQueryOptions{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, limited, 2, "显式 Limit 应严格限制返回数量")

	// 2) Limit<=0（仅指定实体类型过滤）不应报错，且受默认上限保护正常返回。
	unbounded, err := repo.SearchNodes(ctx, namespace, "Lang", &knowledge.NodeQueryOptions{
		EntityTypes: []string{"Technology"},
	})
	require.NoError(t, err, "Limit<=0 时应走默认上限而非生成非法/无界查询")
	assert.GreaterOrEqual(t, len(unbounded), 6, "应返回全部 6 个匹配节点（默认上限 100 之内）")
	assert.LessOrEqual(t, len(unbounded), defaultSearchNodeLimit, "返回数量不应超过默认上限")

	// 3) Offset + Limit 组合（SKIP 必须在 LIMIT 之前，否则 Cypher 语法错误）。
	paged, err := repo.SearchNodes(ctx, namespace, "Lang", &knowledge.NodeQueryOptions{Limit: 2, Offset: 2})
	require.NoError(t, err, "Offset+Limit 组合应生成合法 Cypher（SKIP 在 LIMIT 之前）")
	assert.Len(t, paged, 2, "分页第二页应返回 2 个节点")

	cleanupTestData(t, ctx, repo, namespace)
}

// TestRealGetNeighbors 测试真实邻居查询
func TestRealGetNeighbors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-neighbors",
	}

	// 清理
	cleanupTestData(t, ctx, repo, namespace)

	// 创建图结构: A -> B -> C
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "a", Name: "A", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "b", Name: "B", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "c", Name: "C", EntityType: "Technology", Chunks: []string{"c1"}},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "r1", Source: "A", Target: "B", Type: "CONNECTED_TO", Strength: 8.0, Weight: 7.5},
			{ID: "r2", Source: "B", Target: "C", Type: "CONNECTED_TO", Strength: 8.0, Weight: 7.5},
		},
	}

	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err)

	// 查询 A 的邻居
	t.Log("Getting neighbors of 'A'...")
	result, err := repo.GetNeighbors(ctx, namespace, "A", &knowledge.RelationQueryOptions{
		Limit: 10,
	})
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(result.Neighbors), 1, "A should have at least 1 neighbor")
	assert.GreaterOrEqual(t, len(result.Relations), 1, "A should have at least 1 relation")

	t.Logf("Found %d neighbors and %d relations", len(result.Neighbors), len(result.Relations))

	// 清理
	cleanupTestData(t, ctx, repo, namespace)
}

// TestRealGraphStats 测试真实图谱统计
func TestRealGraphStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-stats",
	}

	// 清理
	cleanupTestData(t, ctx, repo, namespace)

	// 添加测试数据
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "n1", Name: "A", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n2", Name: "B", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n3", Name: "C", EntityType: "Organization", Chunks: []string{"c1"}},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "r1", Source: "A", Target: "B", Type: "RELATED_TO", Strength: 8.0, Weight: 7.5},
			{ID: "r2", Source: "A", Target: "C", Type: "BELONGS_TO", Strength: 9.0, Weight: 8.5},
		},
	}

	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err)

	// 获取统计信息
	t.Log("Getting graph stats...")
	stats, err := repo.GetGraphStats(ctx, namespace)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, stats.NodeCount, int64(3), "Should have at least 3 nodes")
	assert.GreaterOrEqual(t, stats.RelationCount, int64(2), "Should have at least 2 relations")

	t.Logf("Graph stats: Nodes=%d, Relations=%d, Chunks=%d",
		stats.NodeCount, stats.RelationCount, stats.ChunkCount)

	// 获取类型分布
	t.Log("Getting type distribution...")
	typeDist, err := repo.GetTypeDistribution(ctx, namespace)
	require.NoError(t, err)

	t.Logf("Type distribution: %+v", typeDist.RelationTypeDistribution)

	// 清理
	cleanupTestData(t, ctx, repo, namespace)
}

// TestRealGetEntityContext 测试获取实体上下文（增量提取）
func TestRealGetEntityContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-context",
	}

	// 清理
	cleanupTestData(t, ctx, repo, namespace)

	// 添加一些现有数据
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "n1", Name: "Python", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n2", Name: "Django",  Chunks: []string{"c1"}},
			{ID: "n3", Name: "Google",  Chunks: []string{"c1"}},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "r1", Source: "Django", Target: "Python", Type: "RELATED_TO", Strength: 9.0},
		},
	}

	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err)

	// 获取实体上下文
	t.Log("Getting entity context for incremental extraction...")
	context, err := repo.GetEntityContext(ctx, namespace)
	require.NoError(t, err)

	assert.Equal(t, namespace.KnowledgeBaseID, context.KnowledgeBaseID)
	assert.GreaterOrEqual(t, len(context.ExistingEntities), 3, "Should have existing entities")
	assert.Greater(t, len(context.EntityTypes), 0, "Should have entity type distribution")
	assert.Greater(t, len(context.RelationTypes), 0, "Should have relation type distribution")

	t.Logf("Entity context: %d entities, %d types, %d relation types",
		len(context.ExistingEntities), len(context.EntityTypes), len(context.RelationTypes))
	t.Logf("Sample entities: %+v", context.SampleEntities)
	t.Logf("Entity types: %+v", context.EntityTypes)
	t.Logf("Relation types: %+v", context.RelationTypes)

	// 清理
	cleanupTestData(t, ctx, repo, namespace)
}

// TestRealDeleteByScope 测试按范围删除
func TestRealDeleteByScope(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-delete",
	}

	// 确保清理
	cleanupTestData(t, ctx, repo, namespace)

	// 添加数据
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "n1", Name: "Test1", EntityType: "Technology", Chunks: []string{"c1"}},
			{ID: "n2", Name: "Test2", EntityType: "Technology", Chunks: []string{"c1"}},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "r1", Source: "Test1", Target: "Test2", Type: "RELATED_TO"},
		},
	}

	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err)

	// 验证数据存在
	retrieved, _ := repo.GetGraph(ctx, namespace)
	assert.Greater(t, len(retrieved.Node), 0, "Data should exist before deletion")

	// 删除数据
	t.Log("Deleting data by scope (kb_id)...")
	err = repo.DeleteByScope(ctx, "kb_id", namespace.KnowledgeBaseID)
	require.NoError(t, err)

	// 验证数据已删除
	retrieved, err = repo.GetGraph(ctx, namespace)
	require.NoError(t, err)
	assert.Equal(t, 0, len(retrieved.Node), "All nodes should be deleted")
	assert.Equal(t, 0, len(retrieved.Relation), "All relations should be deleted")

	t.Log("Data successfully deleted")
}

// TestRealEndToEnd 完整的端到端测试
func TestRealEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	repo := setupTestRepo(t)
	if repo == nil {
		return
	}
	defer repo.Close(context.Background())

	ctx := context.Background()
	namespace := knowledge.NameSpace{
		TenantID: "test-tenant",
	 KnowledgeBaseID:     "test-kb-e2e",
	}

	// 清理
	cleanupTestData(t, ctx, repo, namespace)

	t.Log("=== End-to-End Test ===")

	// 1. 添加图谱
	t.Log("Step 1: Adding graph...")
	graphData := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "p1", Name: "Python", EntityType: "Technology", Attributes: []string{"动态类型"}, Chunks: []string{"doc1"}},
			{ID: "p2", Name: "Django", EntityType: "Technology", Attributes: []string{"Web框架", "MTV"}, Chunks: []string{"doc1"}},
			{ID: "p3", Name: "Flask", EntityType: "Technology", Attributes: []string{"微框架"}, Chunks: []string{"doc1"}},
			{ID: "o1", Name: "DSF", EntityType: "Organization", Chunks: []string{"doc1"}},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "r1", Source: "Django", Target: "Python", Type: "RELATED_TO", Description: "Django基于Python", Strength: 9.5, Weight: 9.0},
			{ID: "r2", Source: "Flask", Target: "Python", Type: "RELATED_TO", Description: "Flask基于Python", Strength: 9.0, Weight: 8.5},
			{ID: "r3", Source: "Django", Target: "DSF", Type: "BELONGS_TO", Description: "Django由DSF维护", Strength: 8.0, Weight: 7.5},
		},
	}

	err := repo.AddGraph(ctx, namespace, []*knowledge.GraphData{graphData})
	require.NoError(t, err)
	t.Logf("  Added %d nodes and %d relations", len(graphData.Node), len(graphData.Relation))

	// 2. 获取图谱
	t.Log("Step 2: Retrieving graph...")
	retrieved, err := repo.GetGraph(ctx, namespace)
	require.NoError(t, err)
	t.Logf("  Retrieved %d nodes and %d relations", len(retrieved.Node), len(retrieved.Relation))

	// 3. 搜索节点
	t.Log("Step 3: Searching nodes...")
	nodes, err := repo.SearchNodes(ctx, namespace, "Python", &knowledge.NodeQueryOptions{Limit: 10})
	require.NoError(t, err)
	t.Logf("  Found %d nodes matching 'Python'", len(nodes))

	// 4. 查询邻居
	t.Log("Step 4: Getting neighbors...")
	neighbors, err := repo.GetNeighbors(ctx, namespace, "Django", &knowledge.RelationQueryOptions{Limit: 10})
	require.NoError(t, err)
	t.Logf("  Django has %d direct neighbors", len(neighbors.Neighbors))

	// 5. 获取统计
	t.Log("Step 5: Getting statistics...")
	stats, err := repo.GetGraphStats(ctx, namespace)
	require.NoError(t, err)
	t.Logf("  Stats: %d nodes, %d relations", stats.NodeCount, stats.RelationCount)

	// 6. 获取实体上下文
	t.Log("Step 6: Getting entity context...")
	entityCtx, err := repo.GetEntityContext(ctx, namespace)
	require.NoError(t, err)
	t.Logf("  Context: %d entities, %d types", len(entityCtx.ExistingEntities), len(entityCtx.EntityTypes))

	// 7. 删除数据
	t.Log("Step 7: Cleaning up...")
	err = repo.DeleteByScope(ctx, "kb_id", namespace.KnowledgeBaseID)
	require.NoError(t, err)
	t.Log("  Cleanup complete")

	t.Log("=== End-to-End Test PASSED ===")
}
