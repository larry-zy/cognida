//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	a "cognida/internal/model/agent"
	f "cognida/internal/model/feedback"
	k "cognida/internal/model/knowledge"
	"cognida/internal/model/rag"
	repo "cognida/internal/repository/mysql"
	feedbacksvc "cognida/internal/service/feedback"
	knowledge "cognida/internal/service/knowledge"
	pipeline "cognida/internal/service/knowledge/pipeline"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// External index is intentionally an in-memory port with stale rows. The source,
// ingestion-evidence invalidation and HTTP retrieval guard are production MySQL
// implementations. This is not a live Milvus/Neo4j acceptance test.
type chunkHTTPIndex struct {
	k.VectorRepository
	stale               *rag.Document
	current             *rag.Document
	removeErr, indexErr error
	removed, indexed    int
}

func (i *chunkHTTPIndex) HasCollection(context.Context, int64) (bool, error) { return true, nil }
func (i *chunkHTTPIndex) DeleteByChunkID(context.Context, int64, string) error {
	i.removed++
	if i.removeErr != nil {
		return i.removeErr
	}
	i.current = nil
	return nil
}
func (i *chunkHTTPIndex) IndexChunk(_ context.Context, c *k.Chunk) error {
	i.indexed++
	if i.indexErr != nil {
		return i.indexErr
	}
	i.current = &rag.Document{ChunkID: c.ID, KnowledgeID: c.KnowledgeID, KnowledgeBaseID: c.KnowledgeBaseID, Content: c.Content, Score: 1}
	return nil
}

type chunkHTTPRetriever struct{ index *chunkHTTPIndex }

func (r chunkHTTPRetriever) FullTextSearch(context.Context, string, string, int, map[string]string) ([]*rag.Document, error) {
	i := r.index
	out := []*rag.Document{i.stale}
	if i.current != nil {
		out = append(out, i.current)
	}
	return out, nil
}
func (r chunkHTTPRetriever) Search(context.Context, string, []float32, int, map[string]string) ([]*rag.Document, error) {
	return nil, nil
}

type chunkHTTPFeedbackRepo struct{ f.Repository }

func (chunkHTTPFeedbackRepo) FindKnowledge(context.Context, int64, string) (*f.Case, *f.Publication, error) {
	return nil, nil, nil
}

func TestChunkMutationMySQLHTTPGuardsStaleProjectionAndRetry(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN required")
	}
	require.True(t, strings.Contains(dsn, "/cognida_review_"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	for _, mode := range []string{"edit", "delete", "index_failure", "delete_failure"} {
		t.Run(mode, func(t *testing.T) {
			tenant := time.Now().UnixNano() & 0x1fffffffffffff
			user := int64(8719)
			kbID, docID, chunkID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			t.Cleanup(func() {
				for _, table := range []string{"chunks", "knowledge", "knowledge_bases"} {
					require.NoError(t, db.Table(table).Where("tenant_id=?", tenant).Delete(map[string]any{}).Error)
				}
			})
			require.NoError(t, db.Create(&repo.KnowledgeBaseModel{ID: kbID, TenantID: tenant, UserID: user, Name: "chunk mutation", Status: 1}).Error)
			chunkCount := 1
			require.NoError(t, db.Create(&repo.KnowledgeModel{ID: docID, TenantID: tenant, UserID: user, KnowledgeBaseID: kbID, Title: "fixture", Type: "text", Source: "manual", ParseStatus: k.ParseStatusCompleted, EnableStatus: k.EnableStatusEnabled, ChunkCount: &chunkCount, IngestedChunkDigest: strings.Repeat("a", 64)}).Error)
			require.NoError(t, db.Create(&repo.ChunkModel{ID: chunkID, TenantID: tenant, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "obsolete source text", IsEnabled: true}).Error)
			kbs, docs, chunks := repo.NewKnowledgeBaseRepository(db, true), repo.NewKnowledgeRepository(db), repo.NewChunkRepository(db)
			index := &chunkHTTPIndex{stale: &rag.Document{ChunkID: chunkID, KnowledgeID: docID, KnowledgeBaseID: kbID, Content: "obsolete source text", Score: 1}}
			service := knowledge.NewKnowledgeBaseService(kbs, nil, docs, chunks, repo.NewKnowledgeStatsQuerier(db), index, nil, index)
			guard := &feedbacksvc.Service{Repo: chunkHTTPFeedbackRepo{}, Projection: &knowledge.FeedbackProjection{KBs: kbs, Documents: docs, Chunks: chunks}}
			retriever := pipeline.NewRetriever(nil, chunks, nil, chunkHTTPRetriever{index}, nil, nil, nil)
			retriever.(interface{ SetConsumptionGuard(rag.ConsumptionGuard) }).SetConsumptionGuard(guard)
			capability := knowledge.NewRetrievalCapability(retriever, nil).WithEnabledFilter(docs)
			capability.SetConsumptionGuard(guard)
			h := NewKnowledgeBaseHandler(service, nil, capability, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				scope := tenant
				if c.GetHeader("X-Other-Tenant") == "1" {
					scope++
				}
				c.Set("tenant_id", scope)
				c.Set("user_id", user)
				c.Request = c.Request.WithContext(a.WithUserID(a.WithTenantID(c.Request.Context(), scope), user))
			})
			router.PUT("/knowledge-bases/:id/chunks/:cid", h.UpdateChunk)
			router.DELETE("/knowledge-bases/:id/chunks/:cid", h.DeleteChunk)
			router.POST("/knowledge/search", h.SearchKnowledge)
			request := func(method, path string, body any, foreign bool) *httptest.ResponseRecorder {
				raw, _ := json.Marshal(body)
				q := httptest.NewRequest(method, path, bytes.NewReader(raw))
				q.Header.Set("Content-Type", "application/json")
				if foreign {
					q.Header.Set("X-Other-Tenant", "1")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, q)
				return w
			}
			search := func() []map[string]any {
				w := request("POST", "/knowledge/search", map[string]any{"kb_ids": []string{kbID}, "query": "source text", "retrieval_mode": "bm25", "top_k": 5}, false)
				require.Equal(t, 200, w.Code, w.Body.String())
				var response struct {
					Data struct {
						Items []map[string]any `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				return response.Data.Items
			}
			require.Len(t, search(), 1)
			path := "/knowledge-bases/" + kbID + "/chunks/" + chunkID
			denied := request("PUT", path, map[string]any{"content": "unauthorized change"}, true)
			require.GreaterOrEqual(t, denied.Code, 400)
			require.Zero(t, index.removed)
			bad := request("PUT", path, map[string]any{"content": "   "}, false)
			require.GreaterOrEqual(t, bad.Code, 400)
			require.Zero(t, index.removed)
			if mode == "delete" || mode == "delete_failure" {
				if mode == "delete_failure" {
					index.removeErr = errors.New("synthetic index delete unavailable")
					failed := request("DELETE", path, nil, false)
					require.GreaterOrEqual(t, failed.Code, 500)
					source, e := chunks.FindByID(context.Background(), chunkID)
					require.NoError(t, e)
					require.Equal(t, "obsolete source text", source.Content)
					require.Len(t, search(), 1)
					index.removeErr = nil
				}
				w := request("DELETE", path, nil, false)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Empty(t, search())
				_, e := chunks.FindByID(context.Background(), chunkID)
				require.ErrorIs(t, e, k.ErrChunkNotFound)
			} else {
				if mode == "index_failure" {
					index.indexErr = errors.New("synthetic embedding unavailable")
				}
				w := request("PUT", path, map[string]any{"content": "replacement source text"}, false)
				if mode == "index_failure" {
					require.GreaterOrEqual(t, w.Code, 500)
					source, e := chunks.FindByID(context.Background(), chunkID)
					require.NoError(t, e)
					require.Equal(t, "replacement source text", source.Content)
					require.Empty(t, search(), "stale index row must not survive source edit")
					index.indexErr = nil
					w = request("PUT", path, map[string]any{"content": "replacement source text"}, false)
				}
				require.Equal(t, 200, w.Code, w.Body.String())
				items := search()
				require.Len(t, items, 1)
				require.Equal(t, "replacement source text", items[0]["content"])
			}
			doc, e := docs.FindByID(context.Background(), docID)
			require.NoError(t, e)
			require.Empty(t, doc.IngestedChunkDigest)
		})
	}
}
