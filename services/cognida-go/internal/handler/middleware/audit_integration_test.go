//go:build integration

package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"cognida/internal/repository/mysql"
	auditsvc "cognida/internal/service/audit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuditQueryRedactionMySQLIntegration(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	require.NotEmpty(t, dsn, "MYSQL_DSN must target a migrated isolated test database")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	requestID := "audit-redaction-" + uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, db.Where("request_id=?", requestID).Delete(&mysql.AuditLogModel{}).Error)
	})
	writer := auditsvc.NewWriter(mysql.NewAuditRepository(db), auditsvc.WriterConfig{BufferSize: 8, BatchSize: 8, FlushInterval: time.Hour})
	t.Cleanup(func() { require.NoError(t, writer.Close(context.Background())) })
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("request_id", requestID)
		c.Set("tenant_id", int64(10001))
		c.Set("user_id", int64(10002))
		c.Next()
	})
	router.Use(NewAuditMiddleware(writer).Apply())
	router.GET("/api/v1/lifecycle/runs/:id/events", func(c *gin.Context) {
		require.Equal(t, "fixture-not-a-real-jwt", c.Query("token"))
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "event: completed\ndata: {}\n\n")
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/lifecycle/runs/fixture/events?token=fixture-not-a-real-jwt&cursor=17&page_size=10", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, writer.Close(context.Background()))
	var stored mysql.AuditLogModel
	require.NoError(t, db.Where("request_id=?", requestID).First(&stored).Error)
	require.Equal(t, "GET /api/v1/lifecycle/runs/:id/events", stored.Action)
	require.NotContains(t, stored.Details, "fixture-not-a-real-jwt")
	var details map[string]any
	require.NoError(t, json.Unmarshal([]byte(stored.Details), &details))
	query, err := url.ParseQuery(details["query"].(string))
	require.NoError(t, err)
	require.Equal(t, redactedQueryValue, query.Get("token"))
	require.Equal(t, "17", query.Get("cursor"))
	require.Equal(t, "10", query.Get("page_size"))
}
