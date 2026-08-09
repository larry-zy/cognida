//go:build integration

// Package handler 图谱 HTTP 接口集成测试（真实 Neo4j）
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainGraph "cognida/internal/model/knowledge"
	neo4jrepo "cognida/internal/repository/neo4j"
	knowledgeapp "cognida/internal/service/knowledge"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// setupGraphAPI 构建 handler → GraphService → 真实 Neo4j 的最小 gin 路由
func setupGraphAPI(t *testing.T) (*gin.Engine, *neo4jrepo.Neo4jRepository, string) {
	uri := envOr("NEO4J_URI", "bolt://localhost:7687")
	user := envOr("NEO4J_USERNAME", "neo4j")
	pass := os.Getenv("NEO4J_PASSWORD")
	db := envOr("NEO4J_DATABASE", "neo4j")

	repo, err := neo4jrepo.NewNeo4jRepository(uri, user, pass, db)
	if err != nil {
		t.Skipf("Neo4j 不可用，跳过 API 集成测试: %v", err)
	}

	svc := knowledgeapp.NewGraphService(repo, nil)
	h := NewGraphHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.GET("/graph/stats", h.GetStats)
	api.GET("/knowledge-bases/:id/graph/nodes/:nodeId", h.GetNodeDetail)

	return r, repo, "test-kb-graph-api"
}

// graphEnvelope 解析统一响应
type graphEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func seedGraph(t *testing.T, repo *neo4jrepo.Neo4jRepository, ns domainGraph.NameSpace) {
	ctx := t.Context()
	// 清理旧数据
	_ = repo.DeleteByScope(ctx, "kb_id", ns.KnowledgeBaseID)

	// 结构：A-B-C 链 + 孤立节点 D → 2 个连通分量、1 个孤立节点
	graph := &domainGraph.GraphData{
		Node: []*domainGraph.GraphNode{
			{ID: "a", Name: "A", EntityType: "Technology", Chunks: []string{"ck1"}},
			{ID: "b", Name: "B", EntityType: "Technology", Chunks: []string{"ck1"}},
			{ID: "c", Name: "C", EntityType: "Technology", Chunks: []string{"ck2"}},
			{ID: "d", Name: "D", EntityType: "Other"},
		},
		Relation: []*domainGraph.GraphRelation{
			{ID: "r1", Source: "A", Target: "B", Type: "REL", Strength: 5, Weight: 4, ChunkIDs: []string{"ck1"}},
			{ID: "r2", Source: "B", Target: "C", Type: "REL", Strength: 5, Weight: 6},
		},
	}
	require.NoError(t, repo.AddGraph(ctx, ns, []*domainGraph.GraphData{graph}))
}

// TestGraphStatsAPI 校验 GET /graph/stats 返回真实统计（非全零）
func TestGraphStatsAPI(t *testing.T) {
	r, repo, kb := setupGraphAPI(t)
	defer repo.Close(t.Context())
	ns := domainGraph.NameSpace{TenantID: "1", KnowledgeBaseID: kb}
	seedGraph(t, repo, ns)
	defer repo.DeleteByScope(t.Context(), "kb_id", kb)

	req := httptest.NewRequest("GET", "/api/v1/graph/stats?kb_id="+kb, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env graphEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))

	var stats knowledgeapp.GraphStatsDTO
	require.NoError(t, json.Unmarshal(env.Data, &stats))

	assert.Equal(t, 4, stats.NodeCount)
	assert.Equal(t, 2, stats.RelationCount)
	assert.Equal(t, 1, stats.IsolatedNodes, "D 应为孤立节点")
	assert.Equal(t, 2, stats.ComponentCount, "{A,B,C} 与 {D} 共两个连通分量")
	assert.Equal(t, 2, stats.MaxDegree, "B 度数为 2")
	assert.InDelta(t, 5.0, stats.AvgWeight, 0.001, "平均权重 (4+6)/2=5")
}

// TestNodeDetailAPI 校验 GET .../graph/nodes/:nodeId 返回中心节点 + 双向邻居 + 关系
func TestNodeDetailAPI(t *testing.T) {
	r, repo, kb := setupGraphAPI(t)
	defer repo.Close(t.Context())
	ns := domainGraph.NameSpace{TenantID: "1", KnowledgeBaseID: kb}
	seedGraph(t, repo, ns)
	defer repo.DeleteByScope(t.Context(), "kb_id", kb)

	// 查询中间节点 B：应双向找到 A(入) 与 C(出)
	req := httptest.NewRequest("GET", "/api/v1/knowledge-bases/"+kb+"/graph/nodes/B", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var env graphEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))

	var detail struct {
		Node      *domainGraph.GraphNode   `json:"node"`
		Neighbors []*domainGraph.GraphNode `json:"neighbors"`
		Relations []*domainGraph.GraphRelation `json:"relations"`
		Degree    int                      `json:"degree"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &detail))

	require.NotNil(t, detail.Node)
	assert.Equal(t, "B", detail.Node.Name)
	assert.Equal(t, "b", detail.Node.ID, "中心节点 id 应加载")
	assert.Equal(t, 2, detail.Degree)

	names := map[string]bool{}
	for _, nb := range detail.Neighbors {
		names[nb.Name] = true
	}
	assert.True(t, names["A"], "邻居应含入边来源 A（双向）")
	assert.True(t, names["C"], "邻居应含出边目标 C")
}
