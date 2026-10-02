//go:build integration

package wire

import (
	"cognida/internal/config"
	"cognida/internal/infrastructure/job"
	store "cognida/internal/infrastructure/objectstore"
	d "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/lifecycle"
	"context"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Explicit opt-in: retain only the requested fixture for the UI owner to inspect
// and subsequently delete. Never consumes the application's shared job queue.
func TestScopedPublicationFixture(t *testing.T) {
	raw := os.Getenv("LIFECYCLE_FIXTURE_REF")
	if raw == "" {
		t.Skip("explicit UI fixture only")
	}
	tenant, err := strconv.ParseInt(os.Getenv("LIFECYCLE_FIXTURE_TENANT"), 10, 64)
	require.NoError(t, err)
	user, err := strconv.ParseInt(os.Getenv("LIFECYCLE_FIXTURE_USER"), 10, 64)
	require.NoError(t, err)
	ref, err := d.ParseRefID(raw)
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.Open(os.Getenv("MYSQL_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sql.Close()) })
	ctx := context.Background()
	p := d.Principal{TenantID: tenant, UserID: user}
	authorize := featureAuthorizer(db)
	require.NoError(t, authorize(ctx, p, "admin"), "fixture principal must be a real administrator")
	cfg := config.LoadObjectStorageConfig()
	if cfg.Backend == "local" && !filepath.IsAbs(cfg.LocalDir) {
		cfg.LocalDir = filepath.Join("../..", cfg.LocalDir)
	}
	objects, err := store.NewFromConfig(cfg)
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: os.Getenv("REDIS_PASSWORD")})
	require.NoError(t, client.Ping(ctx).Err())
	prefix := "cognida:scoped-publication:" + uuid.NewString()
	t.Cleanup(func() {
		var cursor uint64
		for {
			keys, next, e := client.Scan(ctx, cursor, prefix+":*", 100).Result()
			require.NoError(t, e)
			if len(keys) > 0 {
				require.NoError(t, client.Del(ctx, keys...).Err())
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		require.NoError(t, client.Close())
	})
	q := job.NewRedisQueueWithPrefix(client, prefix)
	repo := mysql.NewLifecycleRepository(db)
	svc, err := lifecycle.NewService(repo, objects, q, lifecycle.Config{Authorize: authorize, Publish: featurePublication(nil, nil)})
	require.NoError(t, err)
	source, _, err := svc.Resolve(ctx, tenant, user, ref)
	require.NoError(t, err)
	require.Equal(t, d.RawAsset, source.Kind)
	stage, err := svc.Invoke(ctx, p, "data-asset", []d.RefID{ref}, nil, "ui-publication:"+uuid.NewString(), "ui-publication")
	require.NoError(t, err)
	execute := func() {
		j, e := q.TryClaim(ctx)
		require.NoError(t, e)
		require.NotNil(t, j)
		require.NoError(t, svc.Handle(ctx, *j))
		require.NoError(t, q.Ack(ctx, *j))
	}
	execute()
	approval, err := repo.FindApproval(ctx, tenant, uuid.NewSHA1(uuid.NameSpaceOID, []byte("approval:"+stage.ID)).String(), false)
	require.NoError(t, err)
	require.NotNil(t, approval)
	require.NoError(t, svc.ResolveApproval(ctx, p, approval.ID, "approved", "Directed UI verification by existing administrator"))
	execute()
	completed, err := repo.FindStage(ctx, tenant, stage.ID, false)
	require.NoError(t, err)
	require.Equal(t, d.StatusSucceeded, completed.Status)
	require.Len(t, completed.Output.Refs, 1)
	t.Logf("UI_FIXTURE invocation=%s approval=%s published_ref=%s", stage.ID, approval.ID, completed.Output.Refs[0].RefID.String())
}
