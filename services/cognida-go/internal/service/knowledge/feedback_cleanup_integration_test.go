//go:build integration

package knowledge

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	f "cognida/internal/model/feedback"
	k "cognida/internal/model/knowledge"
	l "cognida/internal/model/lifecycle"
	repo "cognida/internal/repository/mysql"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFeedbackCleanupMySQLDeletedKBRetainsScopeAndRetriesProjection(t *testing.T) {
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	if dsn == "" {
		t.Skip("COGNIDA_DOMAIN_TEST_DSN required")
	}
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 71321}
	kbID, docID, chunkID := "20261016"+uuid.NewString()[:20], uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		for _, table := range []string{"chunks", "knowledge", "knowledge_bases"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, conn.Close())
	})
	require.NoError(t, db.Create(&repo.KnowledgeBaseModel{ID: kbID, TenantID: p.TenantID, UserID: p.UserID, Name: "cleanup fixture"}).Error)
	require.NoError(t, db.Create(&repo.KnowledgeModel{ID: docID, TenantID: p.TenantID, UserID: p.UserID, KnowledgeBaseID: kbID, Title: "candidate", Type: "text", Source: "feedback", ParseStatus: "completed", EnableStatus: "disabled"}).Error)
	require.NoError(t, db.Create(&repo.ChunkModel{ID: chunkID, TenantID: p.TenantID, KnowledgeBaseID: kbID, KnowledgeID: docID, Content: "candidate fixture"}).Error)
	kbs, docs, chunks := repo.NewKnowledgeBaseRepository(db, true), repo.NewKnowledgeRepository(db), repo.NewChunkRepository(db)
	events := []string{}
	vectors := &cleanupVectors{events: &events}
	graph := &cleanupGraph{events: &events}
	service := NewKnowledgeBaseService(kbs, nil, docs, chunks, repo.NewKnowledgeStatsQuerier(db), vectors, graph)
	projection := NewFeedbackProjection(kbs, docs, chunks, nil, nil, service, nil)
	ctx := context.Background()
	require.NoError(t, service.Delete(ctx, kbID, p.TenantID))
	require.Error(t, service.DeleteKnowledge(ctx, kbID, docID, p.TenantID), "public API must not bypass deleted-KB authorization")
	for _, wrong := range []l.Principal{{TenantID: p.TenantID + 1, UserID: p.UserID}, {TenantID: p.TenantID, UserID: p.UserID + 1}} {
		require.ErrorIs(t, projection.Remove(ctx, wrong, kbID, docID), f.ErrForbidden)
	}
	require.ErrorIs(t, projection.Remove(ctx, p, "another-kb", docID), f.ErrForbidden)
	require.Empty(t, events)
	// External projection ports are deterministic; document/chunk retention and
	// deletion below execute the real MySQL transaction, not mocked results.
	vectors.err = errors.New("projection delete unavailable")
	require.Error(t, projection.Remove(ctx, p, kbID, docID))
	_, err = docs.FindByID(ctx, docID)
	require.NoError(t, err)
	rows, err := chunks.FindByKnowledgeID(ctx, docID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	vectors.err, events = nil, nil
	require.NoError(t, projection.Remove(ctx, p, kbID, docID))
	require.Equal(t, []string{"vector", "graph"}, events)
	require.Equal(t, []string{chunkID}, graph.chunkIDs)
	require.Equal(t, kbID, graph.namespace.KnowledgeBaseID)
	_, err = docs.FindByID(ctx, docID)
	require.ErrorIs(t, err, k.ErrKnowledgeNotFound)
	rows, err = chunks.FindByKnowledgeID(ctx, docID, false)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, projection.Remove(ctx, p, kbID, docID), "cleanup replay must not resurrect or repeat work")
	require.Len(t, events, 2)
}
