//go:build integration

package wire

import (
	"bytes"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/lifecycle"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFeedbackKnowledgeImportAndExistingSourceRecheckMySQLHTTP(t *testing.T) {
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated DSN required")
	}
	cfg, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, e := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	conn, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 9172}
	ctx := context.Background()
	kb, base, candidate := uuid.NewString(), uuid.NewString(), uuid.NewString()
	r := mysql.NewFeedbackRepository(db)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM quality_feedback_events WHERE case_id IN (SELECT id FROM quality_feedback_cases WHERE tenant_id=?)", p.TenantID).Error)
		for _, table := range []string{"quality_feedback_publications", "quality_feedback_cases", "lifecycle_assets", "chunks", "knowledge", "knowledge_bases"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, conn.Close())
	})
	require.NoError(t, db.Create(&mysql.KnowledgeBaseModel{ID: kb, TenantID: p.TenantID, UserID: p.UserID, Name: "feedback source gate", Status: 1}).Error)
	for _, id := range []string{base, candidate} {
		require.NoError(t, db.Create(&mysql.KnowledgeModel{ID: id, TenantID: p.TenantID, UserID: p.UserID, KnowledgeBaseID: kb, Type: "text", Title: id, Source: "fixture", ParseStatus: "completed", EnableStatus: "enabled"}).Error)
		require.NoError(t, db.Create(&mysql.ChunkModel{ID: uuid.NewString(), TenantID: p.TenantID, KnowledgeBaseID: kb, KnowledgeID: id, Content: "candidate text", IsEnabled: true, ChunkType: "text"}).Error)
	}
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	guard := func(ctx context.Context, p l.Principal, kb, id string) error {
		c, v, e := r.FindKnowledge(ctx, p.TenantID, id)
		if e != nil {
			return e
		}
		if c == nil {
			return nil
		}
		if c.Status == "withdrawn" || c.Status == "invalidated" {
			return l.ErrForbidden
		}
		if v == nil {
			if c.ActiveVersionID != "" {
				return l.ErrForbidden
			}
			return nil
		}
		if c.ActiveVersionID != v.ID || v.Status != "approved" {
			return l.ErrForbidden
		}
		return nil
	}
	life, e := lifecycle.NewService(mysql.NewLifecycleRepository(db), objects, struct{ job.Queue }{}, lifecycle.Config{Authorize: func(context.Context, l.Principal, string) error { return nil }, Access: featureAccess(db, nil, nil, nil, objects, nil, nil, guard), ValidateGovernedOrigin: func(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
		return validateFeatureSourceOrigin(ctx, db, nil, nil, p, ref, nil, guard)
	}})
	require.NoError(t, e)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID); c.Next() })
	handler.NewLifecycleHandler(life).Register(router.Group("/lifecycle"))
	request := func(id string) (int, l.DataAssetRef) {
		raw, _ := json.Marshal(map[string]any{"source": lifecycle.SourceRequest{Kind: "knowledge", KnowledgeID: id}, "name": "import"})
		req := httptest.NewRequest("POST", "/lifecycle/assets", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var out struct {
			Data l.DataAssetRef `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return w.Code, out.Data
	}
	code, original := request(base)
	require.Equal(t, 201, code)
	require.Empty(t, original.Provenance.SourceVersion)
	c, e := r.CreateCase(ctx, &d.Case{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Status: "open", BaselineKnowledgeID: base}, "case", "digest")
	require.NoError(t, e)
	v, e := r.CreatePublication(ctx, &d.Publication{ID: uuid.NewString(), CaseID: c.ID, TenantID: p.TenantID, UserID: p.UserID, KnowledgeID: candidate, Status: "approved"}, "pub", "digest")
	require.NoError(t, e)

	code, _ = request(candidate)
	require.Equal(t, 403, code, "unactivated candidate cannot be reimported")
	require.NoError(t, db.Table("quality_feedback_cases").Where("id=?", c.ID).Update("active_version_id", v.ID).Error)
	code, imported := request(candidate)
	require.Equal(t, 201, code)
	require.Equal(t, "feedback:"+v.ID, imported.Provenance.SourceVersion)
	_, _, e = life.Resolve(ctx, p.TenantID, p.UserID, imported.RefID)
	require.NoError(t, e)
	require.NoError(t, db.Table("quality_feedback_cases").Where("id=?", c.ID).Updates(map[string]any{"status": "withdrawn", "active_version_id": ""}).Error)
	code, _ = request(candidate)
	require.Equal(t, 403, code)
	_, _, e = life.Resolve(ctx, p.TenantID, p.UserID, imported.RefID)
	require.ErrorIs(t, e, l.ErrForbidden)
	// This original private evidence reference remains only under its ordinary
	// owner/source rules; internal baseline validation does not recurse into publication state.
	_, _, e = life.Resolve(ctx, p.TenantID, p.UserID, original.RefID)
	require.NoError(t, e)
}
