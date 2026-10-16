//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "cognida/internal/model/knowledge"
	mysqlrepo "cognida/internal/repository/mysql"
	graphrepo "cognida/internal/repository/neo4j"
	knowledgeapp "cognida/internal/service/knowledge"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This is a real MySQL + Neo4j HTTP test. It requires an explicitly provisioned
// review database and graph endpoint; it neither starts services nor migrates.
func TestGraphRelationStatsMySQLNeo4jHTTP(t *testing.T) {
	dsn, uri := os.Getenv("COGNIDA_GRAPH_TEST_DSN"), os.Getenv("COGNIDA_GRAPH_TEST_NEO4J_URI")
	if dsn == "" || uri == "" {
		t.Skip("set COGNIDA_GRAPH_TEST_DSN and COGNIDA_GRAPH_TEST_NEO4J_URI for the isolated graph integration environment")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	parsedURI, err := url.Parse(uri)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, parsedURI.Hostname(), "explicit local graph test endpoint required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(os.Getenv("COGNIDA_GRAPH_TEST_NEO4J_USER"), os.Getenv("COGNIDA_GRAPH_TEST_NEO4J_PASSWORD"), ""))
	require.NoError(t, err)
	require.NoError(t, driver.VerifyConnectivity(ctx))
	t.Cleanup(func() { require.NoError(t, driver.Close(context.Background())) })
	database := os.Getenv("COGNIDA_GRAPH_TEST_NEO4J_DATABASE")
	repo, err := graphrepo.NewNeo4jRepositoryFromDriver(driver, database)
	require.NoError(t, err)
	cypher := func(ctx context.Context, query string, params map[string]any) {
		t.Helper()
		session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: database})
		defer session.Close(ctx)
		result, err := session.Run(ctx, query, params)
		require.NoError(t, err)
		_, err = result.Consume(ctx)
		require.NoError(t, err)
	}
	tenant, user := time.Now().UnixNano()&0x1fffffffffffff, int64(91)
	otherTenant := tenant + 1
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	kbID, emptyKB, privateKB, foreignKB := ids[0], ids[1], ids[2], ids[3]
	tenantText, otherTenantText := strconv.FormatInt(tenant, 10), strconv.FormatInt(otherTenant, 10)
	t.Cleanup(func() {
		require.NoError(t, db.Where("tenant_id IN ? AND id IN ?", []int64{tenant, otherTenant}, ids).Delete(&mysqlrepo.KnowledgeBaseModel{}).Error)
	})
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cypher(cleanupCtx, `MATCH (n:Entity) WHERE n.tenant_id IN $tenants AND n.kb_id IN $ids DETACH DELETE n`, map[string]any{"tenants": []string{tenantText, otherTenantText}, "ids": ids})
	})
	for _, kb := range []mysqlrepo.KnowledgeBaseModel{
		{ID: kbID, TenantID: tenant, UserID: user, Name: "relation stats owner", Status: 1},
		{ID: emptyKB, TenantID: tenant, UserID: user, Name: "relation stats empty", Status: 1},
		{ID: privateKB, TenantID: tenant, UserID: user + 1, Name: "relation stats private", Status: 1},
		{ID: foreignKB, TenantID: otherTenant, UserID: user, Name: "relation stats foreign", Status: 1, IsPublic: true},
	} {
		require.NoError(t, db.Create(&kb).Error)
	}
	graph := &domain.GraphData{Node: []*domain.GraphNode{{ID: uuid.NewString(), Name: "A"}, {ID: uuid.NewString(), Name: "B"}}}
	// More than the visualization's 1000-edge limit: statistics must include all.
	for i := 0; i < 1001; i++ {
		graph.Relation = append(graph.Relation, &domain.GraphRelation{ID: uuid.NewString(), Source: "A", Target: "B", Type: "owns", Strength: 2.5, Weight: 4, ChunkIDs: []string{"chunk,one", "chunk\"two"}})
	}
	nativeID := uuid.NewString()
	graph.Relation = append(graph.Relation,
		&domain.GraphRelation{ID: nativeID, Source: "A", Target: "B", Type: "owns", Strength: 4.75, Weight: 2, ChunkIDs: []string{"native"}},
		&domain.GraphRelation{ID: uuid.NewString(), Source: "A", Target: "A", Type: "self", Strength: -1.2, Weight: 1},
	)
	ns := domain.NameSpace{TenantID: tenantText, KnowledgeBaseID: kbID}
	require.NoError(t, repo.AddGraph(ctx, ns, []*domain.GraphData{graph}))
	params := map[string]any{"tenant": tenantText, "otherTenant": otherTenantText, "kb": kbID, "otherKB": emptyKB, "nativeID": nativeID}
	cypher(ctx, `MATCH ()-[r:RELATION {id:$nativeID,tenant_id:$tenant,kb_id:$kb}]->() SET r.chunk_ids = ['native']`, params)
	cypher(ctx, `MATCH (a:Entity {name:'A',tenant_id:$tenant,kb_id:$kb}), (b:Entity {name:'B',tenant_id:$tenant,kb_id:$kb})
	CREATE (a)-[:RELATION {tenant_id:$tenant,kb_id:$kb}]->(b)
	CREATE (a)-[:RELATION {tenant_id:$otherTenant,kb_id:$kb,type:'wrong_edge_tenant'}]->(b)
	CREATE (a)-[:RELATION {tenant_id:$tenant,kb_id:$otherKB,type:'wrong_edge_kb'}]->(b)
	CREATE (a)-[:OTHER {tenant_id:$tenant,kb_id:$kb}]->(b)
	CREATE (foreign:Entity {name:'foreign',tenant_id:$otherTenant,kb_id:$kb})
	CREATE (other:Entity {name:'other',tenant_id:$tenant,kb_id:$otherKB})
	CREATE (a)-[:RELATION {tenant_id:$tenant,kb_id:$kb,type:'wrong_target_tenant'}]->(foreign)
	CREATE (foreign)-[:RELATION {tenant_id:$tenant,kb_id:$kb,type:'wrong_source_tenant'}]->(b)
	CREATE (a)-[:RELATION {tenant_id:$tenant,kb_id:$kb,type:'wrong_target_kb'}]->(other)
	CREATE (other)-[:RELATION {tenant_id:$tenant,kb_id:$kb,type:'wrong_source_kb'}]->(b)`, params)
	svc := knowledgeapp.NewGraphService(repo, nil)
	svc.SetKnowledgeBaseRepository(mysqlrepo.NewKnowledgeBaseRepository(db, true))
	router := graphStatsAPIRouter(svc, tenant, user, true)
	readStats := func(id string, status int) knowledgeapp.RelationStatsDTO {
		t.Helper()
		w := graphStatsAPIRequest(router, id)
		require.Equal(t, status, w.Code, w.Body.String())
		var response struct {
			Data knowledgeapp.RelationStatsDTO `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		return response.Data
	}
	stats := readStats(kbID, 200)
	require.Equal(t, 1004, stats.TotalRelations)
	require.InDelta(t, (1001*2.5+4.75-1.2)/1004, stats.AverageStrength, 1e-12)
	require.InDelta(t, float64(4007)/1004, stats.AverageWeight, 1e-12)
	require.InDelta(t, float64(2003)/1004, stats.AverageChunkCount, 1e-12)
	require.Equal(t, map[string]int{"2": 1001, "4": 1, "-1": 1, "0": 1}, stats.StrengthDistribution)
	require.Equal(t, map[string]int{"owns": 1002, "self": 1, "": 1}, stats.TypeDistribution)
	// The real single-edge HTTP path must persist scope on the relationship,
	// not just on its endpoints, and immediately appear in the aggregate.
	created := graphStatsAPICreateRelation(router, kbID, "A")
	require.Equal(t, 200, created.Code, created.Body.String())
	stats = readStats(kbID, 200)
	require.Equal(t, 1005, stats.TotalRelations)
	require.Equal(t, 1, stats.TypeDistribution["api-created"])
	require.Equal(t, 1, stats.StrengthDistribution["5"])
	require.InDelta(t, (1001*2.5+4.75-1.2+5)/1005, stats.AverageStrength, 1e-12)
	require.InDelta(t, (4007+3.95)/1005, stats.AverageWeight, 1e-12)
	require.InDelta(t, float64(2006)/1005, stats.AverageChunkCount, 1e-12)
	missing := graphStatsAPICreateRelation(router, kbID, "does-not-exist")
	require.Equal(t, 404, missing.Code, missing.Body.String())
	for _, scope := range []struct {
		tenant, user int64
		status       int
	}{{tenant, user + 1, 403}, {otherTenant, user, 404}} {
		denied := graphStatsAPICreateRelation(graphStatsAPIRouter(svc, scope.tenant, scope.user, true), kbID, "A")
		require.Equal(t, scope.status, denied.Code, denied.Body.String())
		require.Equal(t, stats, readStats(kbID, 200), "denied creates must not mutate the graph")
	}
	// Public grants still allow reads only: a different owner cannot inject a
	// relation into an otherwise visible graph.
	require.NoError(t, db.Model(&mysqlrepo.KnowledgeBaseModel{}).Where("id = ? AND tenant_id = ?", kbID, tenant).Update("is_public", true).Error)
	sharedReader := graphStatsAPIRouter(svc, tenant, user+1, true)
	require.Equal(t, 200, graphStatsAPIRequest(sharedReader, kbID).Code)
	require.Equal(t, 403, graphStatsAPICreateRelation(sharedReader, kbID, "A").Code)
	require.Equal(t, stats, readStats(kbID, 200))
	empty := readStats(emptyKB, 200)
	require.Equal(t, knowledgeapp.RelationStatsDTO{StrengthDistribution: map[string]int{}, TypeDistribution: map[string]int{}}, empty)
	readStats(privateKB, 403)
	readStats(foreignKB, 404)
	readStats(uuid.NewString(), 404)
	// Public sharing is still bounded by the verified tenant.
	require.NoError(t, db.Model(&mysqlrepo.KnowledgeBaseModel{}).Where("id = ? AND tenant_id = ?", privateKB, tenant).Update("is_public", true).Error)
	readStats(privateKB, 200)
	// Current SQL authorization is consulted on every statistics request.
	require.NoError(t, db.Model(&mysqlrepo.KnowledgeBaseModel{}).Where("id = ? AND tenant_id = ?", kbID, tenant).Update("status", 0).Error)
	readStats(kbID, 404)
	require.NoError(t, db.Model(&mysqlrepo.KnowledgeBaseModel{}).Where("id = ? AND tenant_id = ?", kbID, tenant).Updates(map[string]any{"status": 1, "deleted_at": time.Now()}).Error)
	readStats(kbID, 404)
	require.NoError(t, db.Model(&mysqlrepo.KnowledgeBaseModel{}).Where("id = ? AND tenant_id = ?", kbID, tenant).Update("deleted_at", nil).Error)
	// A damaged persisted relationship is an error, never a fabricated zero.
	cypher(ctx, `MATCH ()-[r:RELATION {id:$nativeID,tenant_id:$tenant,kb_id:$kb}]->() SET r.chunk_ids = 'not JSON'`, params)
	readStats(kbID, 500)
	cypher(ctx, `MATCH ()-[r:RELATION {id:$nativeID,tenant_id:$tenant,kb_id:$kb}]->() SET r.chunk_ids = ['native']`, params)
	require.Equal(t, stats, readStats(kbID, 200))
	t.Log(fmt.Sprintf("real MySQL authorization + Neo4j HTTP: %d exact relations, cross-scope edges excluded, empty/public/revoked/corrupt cases verified", stats.TotalRelations))
}
