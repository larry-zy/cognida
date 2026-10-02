//go:build integration

package handler

import (
	"bytes"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/knowledge"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestKnowledgeAdaptiveSettingsHTTPPersistence(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn)
	db, e := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { sql.Close() })
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	id := ""
	t.Cleanup(func() {
		if id != "" {
			require.NoError(t, db.Where("knowledge_base_id=?", id).Delete(&repo.KnowledgeBaseSettingModel{}).Error)
			require.NoError(t, db.Where("id=?", id).Delete(&repo.KnowledgeBaseModel{}).Error)
		}
	})
	s := svc.NewKnowledgeBaseService(repo.NewKnowledgeBaseRepository(db, false), repo.NewKnowledgeBaseSettingRepository(db), nil, nil, nil, nil, nil)
	h := &KnowledgeBaseHandler{knowledgeBaseService: s}
	api := gin.New()
	api.Use(func(c *gin.Context) {
		who := tenant
		if c.GetHeader("X-Other") == "true" {
			who++
		}
		c.Set("tenant_id", who)
		c.Set("user_id", int64(1))
	})
	api.POST("/kb", h.CreateKnowledgeBase)
	api.PUT("/kb/:id", h.UpdateKnowledgeBase)
	api.GET("/kb/:id", h.GetKnowledgeBase)
	call := func(method, path string, body any, other string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Other", other)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	decode := func(w *httptest.ResponseRecorder) svc.KnowledgeBaseResponse {
		var out struct {
			Data svc.KnowledgeBaseResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out.Data
	}
	w := call("POST", "/kb", map[string]any{"name": "adaptive fixture", "chunk_strategy": "auto"}, "")
	require.Equal(t, 201, w.Code, w.Body.String())
	created := decode(w)
	id = created.ID
	require.NotEmpty(t, id)
	require.Equal(t, "auto", created.Setting.ChunkStrategy)
	require.Positive(t, created.Setting.ID)
	w = call("PUT", "/kb/"+id, map[string]any{"chunk_strategy": "sentence", "chunk_size": 900, "chunk_overlap": 90}, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "sentence", decode(w).Setting.ChunkStrategy)
	w = call("GET", "/kb/"+id, nil, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 900, decode(w).Setting.ChunkSize)
	w = call("PUT", "/kb/"+id, map[string]any{"chunk_overlap": 600}, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 600, decode(w).Setting.ChunkOverlap)
	w = call("PUT", "/kb/"+id, map[string]any{"name": "should-not-persist", "chunk_size": 200}, "")
	require.Equal(t, 400, w.Code, w.Body.String())
	w = call("GET", "/kb/"+id, nil, "")
	require.Equal(t, "adaptive fixture", decode(w).Name)
	require.Equal(t, 900, decode(w).Setting.ChunkSize)
	w = call("PUT", "/kb/"+id, map[string]any{"chunk_overlap": 0}, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, 0, decode(w).Setting.ChunkOverlap)

	w = call("PUT", "/kb/"+id, map[string]any{"chunk_strategy": "not-real"}, "")
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Equal(t, 404, call("GET", "/kb/"+id, nil, "true").Code)
}
