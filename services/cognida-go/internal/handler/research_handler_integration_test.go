//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cognida/internal/model/conversation"
	"cognida/internal/model/knowledge"
	mysqlrepo "cognida/internal/repository/mysql"
	agentuc "cognida/internal/service/agent"
	"cognida/internal/service/agent/adapters"
	"cognida/internal/service/agent/tools"
	knowledgeuc "cognida/internal/service/knowledge"
	pipeline "cognida/internal/service/knowledge/pipeline"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestResearchHTTPRealSessionAndKnowledgeAuthorization(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx := context.Background()
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	kbID, foreignKB, docID, chunkID, sessionID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		for _, table := range []string{"sessions", "chunks", "knowledge", "knowledge_bases"} {
			require.NoError(t, db.Table(table).Where("tenant_id IN ?", []int64{tenant, tenant + 1}).Delete(map[string]any{}).Error)
		}
	})
	kbs := mysqlrepo.NewKnowledgeBaseRepository(db, true)
	chunks := mysqlrepo.NewChunkRepository(db)
	docs := mysqlrepo.NewKnowledgeRepository(db)
	sessions := mysqlrepo.NewSessionRepository(db)
	require.NoError(t, kbs.Create(ctx, &knowledge.KnowledgeBase{ID: kbID, TenantID: tenant, UserID: 9, Name: "Research API test", Status: 1}))
	require.NoError(t, kbs.Create(ctx, &knowledge.KnowledgeBase{ID: foreignKB, TenantID: tenant + 1, UserID: 9, Name: "Foreign", Status: 1}))
	require.NoError(t, docs.Create(ctx, &knowledge.Knowledge{ID: docID, TenantID: tenant, UserID: 9, KnowledgeBaseID: kbID, Type: "text", Title: "Evidence", Source: "review", EnableStatus: "enabled", ParseStatus: "completed"}))
	require.NoError(t, chunks.Create(ctx, &knowledge.Chunk{ID: chunkID, TenantID: tenant, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "Verified evidence in the source document.", IsEnabled: true, ChunkType: "text"}))
	require.NoError(t, sessions.Create(ctx, &conversation.Session{ID: sessionID, TenantID: tenant, UserID: 9, AgentType: "deep_research", Title: "Review", Status: 1}))
	adapter := adapters.NewRAGRetrieverAdapter(knowledgeuc.NewRetrievalCapability(pipeline.NewRetriever(nil, chunks, nil, nil, nil, nil, nil), nil).WithEnabledFilter(docs), kbs)
	svc := agentuc.NewResearchService(nil, nil).WithDependencies(researchHTTPModel{}, researchHTTPRegistry{"rag_query": tools.NewRAGQueryTool(adapter)})
	h := NewAgentHandler(nil, svc, nil, nil, agentuc.NewAgentPersistenceService(sessions, nil), nil)
	gin.SetMode(gin.TestMode)
	request := func(owner, user int64, kb, path string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("tenant_id", owner); c.Set("user_id", user) })
		r.POST("/research", h.DeepResearch)
		r.POST("/stream", h.DeepResearchStream)
		payload, _ := json.Marshal(agentuc.DeepResearchRequest{Query: "evidence", KBIDs: []string{kb}, SessionID: sessionID})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, 403, request(tenant, 10, kbID, "/research").Code)
	require.Equal(t, 403, request(tenant+1, 9, kbID, "/stream").Code)
	w := request(tenant, 9, kbID, "/stream")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "event: complete")
	require.Contains(t, w.Body.String(), chunkID)
	require.Contains(t, w.Body.String(), `"citation_checked":true`)
	w = request(tenant, 9, foreignKB, "/research")
	require.Equal(t, 422, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "Verified evidence")
}
