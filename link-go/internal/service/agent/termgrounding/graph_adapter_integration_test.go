//go:build integration

// Package termgrounding 图谱接地层集成测试：真实 Neo4j 验证「模型内未命中的口语词
// 经知识图谱 SIMILAR_TO 桥接回落到受治理规范名」这条第二层接地主路。
//
// 运行：cd link-go && set -a && source .env && set +a && \
//   go test -tags=integration ./internal/service/agent/termgrounding/ -run Integration -v
package termgrounding

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"link/internal/model/knowledge"
	"link/internal/model/semantic"
	neo4jrepo "link/internal/repository/neo4j"
)

// itTenant 用独立测试租户，避免污染 dev(tenant=1) 的接地图谱，也便于清理。
const itTenant = int64(990001)

func newITGraphAdapter(t *testing.T) (*GraphAdapter, knowledge.GraphRepository) {
	t.Helper()
	uri := os.Getenv("NEO4J_URI")
	if uri == "" {
		uri = "bolt://localhost:7687"
	}
	user := os.Getenv("NEO4J_USERNAME")
	if user == "" {
		user = "neo4j"
	}
	// dbName 留空：与运行时 GraphAdapter 一致（cfg.Neo4j.DatabaseName 未设→""）。
	repo, err := neo4jrepo.NewNeo4jRepository(uri, user, os.Getenv("NEO4J_PASSWORD"), "")
	if err != nil {
		t.Skipf("Neo4j 不可用，跳过图谱接地集成测试: %v", err)
	}
	// GraphAdapter 用 kb_id=""（租户全域），与 cmd/server 接线一致。
	return NewGraphAdapter(repo, ""), repo
}

// TestGraphGroundingIntegration 验证：口语词「流水」不在模型内同义词中，
// 经图谱 SIMILAR_TO 桥接回落到受治理指标「营收」，Source 标注为 graph。
func TestGraphGroundingIntegration(t *testing.T) {
	adapter, repo := newITGraphAdapter(t)
	ctx := context.Background()
	ns := knowledge.NameSpace{TenantID: "990001", KnowledgeBaseID: ""}

	// 自建桥接数据：流水 —SIMILAR_TO→ 营收（营收须与下方 bundle 的规范名一致）。
	data := &knowledge.GraphData{
		Node: []*knowledge.GraphNode{
			{ID: "it_gt_liushui", Name: "流水", EntityType: "业务术语"},
			{ID: "it_gt_yingshou", Name: "营收", EntityType: "受治理规范名"},
		},
		Relation: []*knowledge.GraphRelation{
			{ID: "it_gt_rel_0", Source: "流水", Target: "营收",
				Type: string(knowledge.RelationTypeSimilarTo), Strength: 9},
		},
	}
	require.NoError(t, repo.AddGraph(ctx, ns, []*knowledge.GraphData{data}))
	t.Cleanup(func() {
		_ = repo.DeleteRelation(ctx, ns, "it_gt_rel_0")
		_ = repo.DeleteNode(ctx, ns, "it_gt_liushui")
		_ = repo.DeleteNode(ctx, ns, "it_gt_yingshou")
	})

	// 端口级：ResolveAliases 应把「流水」解析出别名「营收」。
	aliases, err := adapter.ResolveAliases(ctx, itTenant, "流水")
	require.NoError(t, err)
	assert.Contains(t, aliases, "营收", "图谱应把口语词『流水』解析到规范名『营收』")

	// 接地器级：模型内无「流水」同义词，须经图谱层回落且 Source=graph。
	bundle := &semantic.ModelBundle{
		Model: &semantic.SemanticModel{TenantID: itTenant, Name: "电商销售"},
		Metrics: []*semantic.Metric{
			{Name: "营收", Synonyms: []string{"revenue", "gmv"}},
		},
	}
	g := NewGrounder(adapter)
	res := g.Ground(ctx, itTenant, bundle, []string{"流水"})
	require.Len(t, res, 1)
	assert.Equal(t, "营收", res[0].Resolved, "『流水』应经图谱接地到『营收』")
	assert.False(t, res[0].Ambiguous)
	require.NotEmpty(t, res[0].Candidates)
	assert.Equal(t, SourceGraph, res[0].Candidates[0].Source, "命中来源应标注为 graph（可观测）")
	// Via 记录「在模型内命中的那个图谱别名」——此处经由邻居名『营收』回落，故 Via=营收。
	assert.Equal(t, "营收", res[0].Candidates[0].Via, "Via 应记录在模型内命中的图谱别名")
}
