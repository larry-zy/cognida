//go:build integration

package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"cognida/internal/model/conversation"
	repo "cognida/internal/repository/mysql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Real repositories + HTTP handlers. Identity is a fixed authenticated fixture;
// login/token verification is outside these authorization regression tests.
func TestReviewSecurityMySQLMessageAndAuditHTTP(t *testing.T) {
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
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	user := int64(8719)
	sessionID := uuid.NewString()
	messageID := uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, db.Unscoped().Where("session_id=?", sessionID).Delete(&repo.MessageModel{}).Error)
		require.NoError(t, db.Unscoped().Where("id=?", sessionID).Delete(&repo.SessionModel{}).Error)
		require.NoError(t, db.Where("tenant_id=?", tenant).Delete(&repo.AuditLogModel{}).Error)
	})
	sr, mr := repo.NewSessionRepository(db), repo.NewMessageRepository(db)
	require.NoError(t, sr.Create(context.Background(), &conversation.Session{ID: sessionID, TenantID: tenant, UserID: user, Title: "scope fixture", AgentType: "default", Status: 1}))
	// Trusted Agent persistence deliberately has no HTTP principal and stays valid.
	require.NoError(t, mr.Create(context.Background(), &conversation.Message{ID: messageID, SessionID: sessionID, Role: "assistant", Content: "private fixture"}))
	for _, scope := range []struct {
		tenant, user int64
		allow        bool
	}{{tenant, user, true}, {tenant, user + 1, false}, {tenant + 1, user, false}, {0, user, false}, {tenant, 0, false}} {
		r := messageScopeRouter(mr, sr, scope.tenant, scope.user)
		for _, op := range []struct{ method, path, body string }{{"GET", "/messages/" + messageID, ""}, {"GET", "/messages?session_id=" + sessionID, ""}, {"POST", "/messages", fmt.Sprintf(`{"session_id":%q,"role":"user","content":"new"}`, sessionID)}, {"PUT", "/messages/" + messageID, `{"content":"owned edit"}`}} {
			w := httptest.NewRecorder()
			q := httptest.NewRequest(op.method, op.path, bytes.NewBufferString(op.body))
			q.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, q)
			want := 403
			if scope.allow {
				want = 200
				if op.method == "POST" {
					want = 201
				}
			}
			require.Equal(t, want, w.Code, w.Body.String())
		}
		if !scope.allow {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("DELETE", "/messages/"+messageID, nil))
			require.Equal(t, 403, w.Code)
		}
	}
	got, err := mr.FindByID(context.Background(), messageID)
	require.NoError(t, err)
	require.Equal(t, "owned edit", got.Content)
	// Parent deletion revokes reads and writes, while the historical message remains.
	require.NoError(t, sr.Delete(context.Background(), sessionID))
	r := messageScopeRouter(mr, sr, tenant, user)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/messages/"+messageID, nil))
	require.Equal(t, 403, w.Code)
	audit := repo.AuditLogModel{TenantID: &tenant, UserID: &user, Module: "review", Action: "test", Details: `{"private":"audit fixture"}`, Status: "success"}
	require.NoError(t, db.Create(&audit).Error)
	ar := repo.NewAuditRepository(db)
	for _, scope := range []struct {
		tenant int64
		status int
	}{{tenant, 200}, {tenant + 1, 404}, {0, 401}} {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("tenant_id", scope.tenant) })
		router.GET("/audit/:id", NewAuditHandler(ar, nil).GetAuditLog)
		rw := httptest.NewRecorder()
		router.ServeHTTP(rw, httptest.NewRequest("GET", fmt.Sprintf("/audit/%d", audit.ID), nil))
		require.Equal(t, scope.status, rw.Code)
		if scope.status != 200 {
			require.NotContains(t, rw.Body.String(), "audit fixture")
		}
	}
	_, err = ar.FindByID(context.Background(), tenant+1, audit.ID)
	require.Error(t, err)
	_, err = ar.FindByID(context.Background(), 0, audit.ID)
	require.Error(t, err)
}
