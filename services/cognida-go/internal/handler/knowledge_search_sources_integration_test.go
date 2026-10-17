//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	repository "cognida/internal/repository/mysql"
	knowledge "cognida/internal/service/knowledge"
	pipeline "cognida/internal/service/knowledge/pipeline"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestKnowledgeSearchMySQLFallbackSourcesAPI(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	tenant, kbID, docID := time.Now().UnixNano()&0x1fffffffffffff, uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		for _, table := range []string{"chunks", "knowledge", "knowledge_bases"} {
			require.NoError(t, db.Table(table).Where("tenant_id = ?", tenant).Delete(map[string]any{}).Error)
		}
	})
	require.NoError(t, db.Create(&repository.KnowledgeBaseModel{ID: kbID, TenantID: tenant, UserID: 1, Name: "source provenance fixture", Status: 1}).Error)
	require.NoError(t, db.Exec("INSERT INTO knowledge (id,tenant_id,knowledge_base_id,user_id,type,title,source,parse_status) VALUES (?,?,?,1,'text','source fixture','collection','completed')", docID, tenant, kbID).Error)
	require.NoError(t, db.Create(&repository.ChunkModel{ID: uuid.NewString(), TenantID: tenant, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "provenance proof", IsEnabled: true}).Error)
	kbRepo := repository.NewKnowledgeBaseRepository(db, true)
	kbService := knowledge.NewKnowledgeBaseService(kbRepo, nil, nil, nil, nil, nil, nil)
	retrieval := knowledge.NewRetrievalCapability(pipeline.NewRetriever(nil, repository.NewChunkRepository(db), nil, nil, nil, nil, nil), nil)
	filter, ok := repository.NewKnowledgeRepository(db).(knowledge.EnabledKnowledgeFilter)
	require.True(t, ok)
	retrieval.WithEnabledFilter(filter)
	h := NewKnowledgeBaseHandler(kbService, nil, retrieval, nil)
	scope := tenant
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", scope); c.Next() })
	router.POST("/search", h.SearchKnowledge)
	search := func() struct {
		Data struct {
			Items []struct {
				KnowledgeID string   `json:"knowledge_id"`
				Sources     []string `json:"sources"`
			}
			Warnings []string
		}
	} {
		req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(fmt.Sprintf(`{"kb_ids":[%q],"query":"provenance proof","retrieval_mode":"vector"}`, kbID)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Data struct {
				Items []struct {
					KnowledgeID string   `json:"knowledge_id"`
					Sources     []string `json:"sources"`
				}
				Warnings []string
			}
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return body
	}
	result := search()
	require.Len(t, result.Data.Items, 1)
	require.Equal(t, docID, result.Data.Items[0].KnowledgeID)
	require.Equal(t, []string{"database_lexical"}, result.Data.Items[0].Sources)
	require.Contains(t, strings.Join(result.Data.Warnings, " "), "lexical database fallback")
	scope = tenant + 1
	require.Empty(t, search().Data.Items, "sources must not expose cross-tenant evidence")
	scope = tenant
	require.NoError(t, db.Exec("UPDATE knowledge SET enable_status='disabled' WHERE id=? AND tenant_id=?", docID, tenant).Error)
	require.Empty(t, search().Data.Items, "disabled evidence must be removed before returning sources")
}
