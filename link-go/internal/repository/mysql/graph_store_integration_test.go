//go:build integration
// +build integration

// Package mysql: integration tests for the MySQL-backed graph store.
//
// These run against a real MySQL database and are gated behind the `integration`
// build tag (see CLAUDE.md 集成测试). Provide a DSN via MYSQL_DSN, e.g.:
//
//	MYSQL_DSN='root:password@tcp(localhost:3306)/link?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags=integration ./internal/repository/mysql/ -run TestGraphStore -v
//
// Each test isolates itself with a unique namespace and cleans up afterwards.
package mysql

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	domain_knowledge "link/internal/model/knowledge"
)

func newIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN not set; skipping graph store integration test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect mysql: %v", err)
	}
	return db
}

func newTestNamespace() domain_knowledge.NameSpace {
	// Unique-ish kb_id per run keeps tests isolated without a clock dependency.
	return domain_knowledge.NameSpace{TenantID: "it_tenant", KnowledgeBaseID: "it_kb_graphstore"}
}

func setupRepo(t *testing.T) (domain_knowledge.GraphRepository, domain_knowledge.NameSpace, func()) {
	t.Helper()
	db := newIntegrationDB(t)
	repo := NewGraphMetaRepository(db)
	ns := newTestNamespace()
	ctx := context.Background()
	if err := repo.InitIndexes(ctx); err != nil {
		t.Fatalf("InitIndexes: %v", err)
	}
	// Clean any residue from prior runs.
	_ = repo.DeleteGraph(ctx, []domain_knowledge.NameSpace{ns})
	cleanup := func() {
		_ = repo.DeleteGraph(ctx, []domain_knowledge.NameSpace{ns})
	}
	return repo, ns, cleanup
}

func sampleGraph() *domain_knowledge.GraphData {
	return &domain_knowledge.GraphData{
		Node: []*domain_knowledge.GraphNode{
			{ID: "n_alice", Name: "Alice", EntityType: "Person", Chunks: []string{"c1"}, Attributes: []string{"engineer"}},
			{ID: "n_bob", Name: "Bob", EntityType: "Person", Chunks: []string{"c1", "c2"}},
			{ID: "n_acme", Name: "Acme", EntityType: "Org", Chunks: []string{"c2"}},
		},
		Relation: []*domain_knowledge.GraphRelation{
			{ID: "r_ab", Source: "Alice", Target: "Bob", Type: "KNOWS", Strength: 0.5, Weight: 1, ChunkIDs: []string{"c1"}},
			{ID: "r_ba", Source: "Bob", Target: "Acme", Type: "WORKS_AT", Strength: 0.9, Weight: 1, ChunkIDs: []string{"c2"}},
		},
	}
}

func TestGraphStore_AddAndGetGraph(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}
	g, err := repo.GetGraph(ctx, ns)
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Node) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Node))
	}
	if len(g.Relation) != 2 {
		t.Errorf("expected 2 relations, got %d", len(g.Relation))
	}
}

func TestGraphStore_AddGraphMergesByName(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}
	// Re-add Alice with a new chunk; chunks should union, no duplicate node.
	merge := &domain_knowledge.GraphData{
		Node: []*domain_knowledge.GraphNode{
			{ID: "n_alice", Name: "Alice", EntityType: "Person", Chunks: []string{"c3"}},
		},
	}
	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{merge}); err != nil {
		t.Fatalf("AddGraph merge: %v", err)
	}
	g, err := repo.GetGraph(ctx, ns)
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Node) != 3 {
		t.Fatalf("expected still 3 nodes after merge, got %d", len(g.Node))
	}
	for _, n := range g.Node {
		if n.Name == "Alice" {
			if len(n.Chunks) != 2 {
				t.Errorf("expected Alice chunks unioned to 2, got %v", n.Chunks)
			}
		}
	}
}

func TestGraphStore_ShortestPath(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()
	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}
	path, err := repo.FindShortestPath(ctx, ns, "Alice", "Acme", nil)
	if err != nil {
		t.Fatalf("FindShortestPath: %v", err)
	}
	if path.Length != 2 {
		t.Errorf("expected length 2 (Alice-Bob-Acme), got %d", path.Length)
	}
}

func TestGraphStore_Neighbors(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()
	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}
	res, err := repo.GetNeighbors(ctx, ns, "Bob", nil)
	if err != nil {
		t.Fatalf("GetNeighbors: %v", err)
	}
	if res.Degree != 2 {
		t.Errorf("expected Bob degree 2, got %d", res.Degree)
	}
	if len(res.Neighbors) != 2 {
		t.Errorf("expected 2 neighbors, got %d", len(res.Neighbors))
	}
}

func TestGraphStore_Stats(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()
	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}
	stats, err := repo.GetGraphStats(ctx, ns)
	if err != nil {
		t.Fatalf("GetGraphStats: %v", err)
	}
	if stats.NodeCount != 3 || stats.RelationCount != 2 {
		t.Errorf("unexpected stats: nodes=%d rels=%d", stats.NodeCount, stats.RelationCount)
	}
	if stats.ChunkCount == 0 {
		t.Errorf("expected non-zero chunk count")
	}

	deg, err := repo.GetDegreeStats(ctx, ns)
	if err != nil {
		t.Fatalf("GetDegreeStats: %v", err)
	}
	if deg.MaxDegree != 2 {
		t.Errorf("expected max degree 2 (Bob), got %d", deg.MaxDegree)
	}
}

func TestGraphStore_DeleteByChunkID(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()
	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}
	// c1 is the only chunk of Alice -> Alice removed; Bob keeps c2.
	if err := repo.DeleteByChunkID(ctx, ns, "c1"); err != nil {
		t.Fatalf("DeleteByChunkID: %v", err)
	}
	g, err := repo.GetGraph(ctx, ns)
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	for _, n := range g.Node {
		if n.Name == "Alice" {
			t.Errorf("expected Alice removed after deleting its only chunk")
		}
	}
}

// TestGraphStore_ConcurrentAddGraphMerges verifies that concurrent AddGraph calls
// touching the same node/relation merge cleanly (no duplicate-key failure) thanks to
// the row lock taken during upsert.
func TestGraphStore_ConcurrentAddGraphMerges(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()

	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			g := &domain_knowledge.GraphData{
				Node: []*domain_knowledge.GraphNode{
					{ID: "n_shared", Name: "Shared", EntityType: "Person", Chunks: []string{fmt.Sprintf("c%d", idx)}},
				},
				Relation: []*domain_knowledge.GraphRelation{
					{ID: "r_shared", Source: "Shared", Target: "Shared", Type: "SELF", Weight: 1, ChunkIDs: []string{fmt.Sprintf("c%d", idx)}},
				},
			}
			errs[idx] = repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{g})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent AddGraph writer %d failed: %v", i, err)
		}
	}
	g, err := repo.GetGraph(ctx, ns)
	if err != nil {
		t.Fatalf("GetGraph: %v", err)
	}
	if len(g.Node) != 1 {
		t.Errorf("expected exactly 1 merged node, got %d", len(g.Node))
	}
	if len(g.Node) == 1 && len(g.Node[0].Chunks) != writers {
		t.Errorf("expected %d unioned chunks, got %d (%v)", writers, len(g.Node[0].Chunks), g.Node[0].Chunks)
	}
}

func TestGraphStore_CommunityAndCentrality(t *testing.T) {
	repo, ns, cleanup := setupRepo(t)
	defer cleanup()
	ctx := context.Background()
	if err := repo.AddGraph(ctx, ns, []*domain_knowledge.GraphData{sampleGraph()}); err != nil {
		t.Fatalf("AddGraph: %v", err)
	}

	if err := repo.StoreCommunity(ctx, ns, &domain_knowledge.Community{ID: "com1", Name: "team", Size: 2, Modularity: 0.4}); err != nil {
		t.Fatalf("StoreCommunity: %v", err)
	}
	if err := repo.StoreCommunityMembers(ctx, ns, []*domain_knowledge.CommunityMember{
		{CommunityID: "com1", EntityID: "n_alice", EntityName: "Alice", MembershipScore: 1},
		{CommunityID: "com1", EntityID: "n_bob", EntityName: "Bob", MembershipScore: 1},
	}); err != nil {
		t.Fatalf("StoreCommunityMembers: %v", err)
	}
	com, err := repo.GetNodeCommunity(ctx, ns, "Alice")
	if err != nil {
		t.Fatalf("GetNodeCommunity: %v", err)
	}
	if com.ID != "com1" {
		t.Errorf("expected com1, got %s", com.ID)
	}

	if err := repo.UpdateCentralityScores(ctx, ns, map[string]float64{"Bob": 0.9, "Alice": 0.3}, "pagerank"); err != nil {
		t.Fatalf("UpdateCentralityScores: %v", err)
	}
	cs, err := repo.GetCentralitySummaries(ctx, ns, "pagerank")
	if err != nil {
		t.Fatalf("GetCentralitySummaries: %v", err)
	}
	if len(cs.TopNodes) == 0 || cs.TopNodes[0].NodeName != "Bob" {
		t.Errorf("expected Bob highest pagerank, got %+v", cs.TopNodes)
	}
}
