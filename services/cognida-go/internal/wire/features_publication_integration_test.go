//go:build integration

package wire

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

func TestTextPublicationTargetTenantAndRevocation(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	require.Contains(t, dsn, "/cognida_review_")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	defer sql.Close()
	id := uuid.NewString()
	tenant := time.Now().UnixNano() & 0x1fffffffffffff
	require.NoError(t, db.Table("knowledge_bases").Create(map[string]any{"id": id, "tenant_id": tenant, "user_id": 991, "name": "review target", "status": 1}).Error)
	defer db.Table("knowledge_bases").Where("id=?", id).Delete(map[string]any{})
	validate := featurePublicationValidator(db, nil)
	ctx := context.Background()
	p := l.Principal{TenantID: tenant, UserID: 991}
	in := l.Input{Kind: "text", Text: "evidence"}
	require.ErrorIs(t, validate(ctx, p, in, nil), l.ErrInvalid)
	options := map[string]any{"knowledge_base_id": id}
	require.NoError(t, validate(ctx, p, in, options))
	require.ErrorIs(t, validate(ctx, l.Principal{TenantID: tenant + 1, UserID: 991}, in, options), l.ErrForbidden)
	require.NoError(t, db.Table("knowledge_bases").Where("id=?", id).Update("status", 0).Error)
	require.ErrorIs(t, validate(ctx, p, in, options), l.ErrForbidden)
	require.NoError(t, db.Table("knowledge_bases").Where("id=?", id).Updates(map[string]any{"status": 1, "deleted_at": time.Now()}).Error)
	require.ErrorIs(t, validate(ctx, p, in, options), l.ErrForbidden)
}
