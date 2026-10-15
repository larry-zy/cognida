//go:build integration

package knowledge

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	domain "cognida/internal/model/knowledge"
	repository "cognida/internal/repository/mysql"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCancelledDocumentMySQLPersistsFailedReceipt(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated review database required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	tenant, id := time.Now().UnixNano()&0x1fffffffffffff, uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM knowledge WHERE tenant_id = ? AND id = ?", tenant, id).Error)
	})
	require.NoError(t, db.Exec("INSERT INTO knowledge (id,tenant_id,knowledge_base_id,user_id,type,title,source,parse_status) VALUES (?,?,'timeout-fixture',1,'text','timeout fixture','collection','processing')", id, tenant).Error)
	repo := repository.NewKnowledgeRepository(db)
	svc := &documentProcessorService{knowledgeRepo: repo}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, svc.recordCancelledProcessing(ctx, id, context.DeadlineExceeded), context.DeadlineExceeded)
	doc, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, domain.ParseStatusFailed, doc.ParseStatus)
	require.Contains(t, doc.ErrorMessage, "context deadline exceeded")
}
