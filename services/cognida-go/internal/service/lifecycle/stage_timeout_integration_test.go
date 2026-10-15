//go:build integration

package lifecycle

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	storage "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/lifecycle"
	repository "cognida/internal/repository/mysql"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStageTimeoutMySQLPersistsTerminalFailure(t *testing.T) {
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
	p := d.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 1}
	id := uuid.NewString()
	t.Cleanup(func() {
		for _, table := range []string{"lifecycle_events", "lifecycle_stages"} {
			require.NoError(t, db.Table(table).Where("tenant_id = ?", p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	objects, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	// Hide intermediate queued/running stages from the live background worker;
	// commit only the failed state and inspect it through a fresh repository.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		repo := repository.NewLifecycleRepository(tx)
		s, err := NewService(repo, objects, &timeoutQueue{}, Config{StageTimeout: 25 * time.Millisecond, Adapters: map[string]d.Adapter{"data-asset": adapterFunc(func(ctx context.Context, _ d.StageRequest) (d.Output, error) {
			<-ctx.Done()
			return d.Output{}, ctx.Err()
		})}})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		st := d.StageRun{ID: id, TenantID: p.TenantID, UserID: p.UserID, AgentID: "data-asset", StageKey: "publish", Status: d.StatusQueued, Approved: true, MaxAttempts: 2, IdempotencyKey: id, CreatedAt: now, UpdatedAt: now}
		if err = repo.CreateStage(ctx, &st); err != nil {
			return err
		}
		j, err := stageJob(st)
		if err != nil {
			return err
		}
		require.ErrorIs(t, s.handle(ctx, j), context.DeadlineExceeded)
		got, err := repo.FindStage(ctx, p.TenantID, id, false)
		require.NoError(t, err)
		require.Equal(t, d.StatusQueued, got.Status)
		return s.handle(ctx, j)
	}))
	repo := repository.NewLifecycleRepository(db)
	got, err := repo.FindStage(ctx, p.TenantID, id, false)
	require.NoError(t, err)
	require.Equal(t, d.StatusFailed, got.Status)
	require.Equal(t, 2, got.Attempt)
	require.Contains(t, got.Error, "context deadline exceeded")
	require.Nil(t, got.Output)
	events, err := repo.Events(ctx, p.TenantID, "", 0)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, "stage.failed", events[len(events)-1].Type)
	_, err = repo.FindStage(ctx, p.TenantID+1, id, false)
	require.ErrorIs(t, err, d.ErrNotFound)
}
