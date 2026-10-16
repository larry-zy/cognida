//go:build integration

package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cognida/internal/handler"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	repo "cognida/internal/repository/mysql"
	svc "cognida/internal/service/sandbox"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"net/http/httptest"
)

// This test claim is deliberately restricted to one fixture record. No global
// scanner/app worker is started against the shared integration database.
type sandboxCleanupClaim struct {
	d.Repository
	db *gorm.DB
	p  l.Principal
	id string
}

func (r *sandboxCleanupClaim) Claim(ctx context.Context, lease time.Duration) (*d.Record, error) {
	var out d.Record
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND state IN ?", r.id, r.p.TenantID, r.p.UserID, []d.State{d.StateRunning, d.StateQueued, d.StateCancelRequested, d.StateReconcileRequired}).Take(&out).Error; err != nil {
			return err
		}
		var micros int64
		if err := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&micros).Error; err != nil {
			return err
		}
		until := time.UnixMicro(micros).UTC().Add(lease)
		out.LeaseUntil, out.LeaseToken = &until, uuid.NewString()
		out.Version++
		return tx.Model(&d.Record{}).Where("id=? AND tenant_id=? AND user_id=?", r.id, r.p.TenantID, r.p.UserID).Updates(map[string]any{"version": out.Version, "lease_token": out.LeaseToken, "lease_until": until}).Error
	})
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &out, err
}

// Real MySQL + HTTP + runner/Save paths. Docker absence/removal failure is an
// explicit CLI fixture; this is not additional live-container isolation proof.
func TestSandboxMySQLHTTPConfirmedCleanupConverges(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"), "isolated migrated review database required")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 971}
	t.Cleanup(func() {
		require.NoError(t, db.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID).Delete(&d.Record{}).Error)
	})
	bin := t.TempDir()
	failFile := filepath.Join(bin, "unconfirmed")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n[ \"$1\" = container ] || exit 64\n[ ! -e \"$SANDBOX_TEST_REMOVE_FAIL\" ] || exit 1\nexit 0\n"), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SANDBOX_TEST_REMOVE_FAIL", failFile)
	storage := repo.NewSandboxRepository(db)
	ctx := context.Background()
	makeRecord := func(t *testing.T, phase string) (*d.Record, *svc.Validator, *bool) {
		t.Helper()
		allowed := true
		in := request()
		in.IdempotencyKey = uuid.NewString()
		v, err := svc.NewValidator([]d.Profile{profile()}, func(_ context.Context, who l.Principal, ref l.RefID) (d.InputMetadata, error) {
			if who != p || !allowed {
				return d.InputMetadata{}, d.ErrForbidden
			}
			for _, input := range append([]d.InputRef{in.Code}, in.Inputs...) {
				if input.Ref == ref {
					return d.InputMetadata{TenantID: p.TenantID, Ref: ref, SHA256: input.SHA256, SizeBytes: input.SizeBytes}, nil
				}
			}
			return d.InputMetadata{}, d.ErrForbidden
		})
		require.NoError(t, err)
		validated, err := v.ValidateSubmission(ctx, p, in)
		require.NoError(t, err)
		raw, err := json.Marshal(validated)
		require.NoError(t, err)
		now := time.Now().UTC()
		row, err := storage.Create(ctx, &d.Record{ID: validated.Binding.ID, TenantID: p.TenantID, UserID: p.UserID, IdempotencyKey: in.IdempotencyKey, RequestSHA256: validated.Binding.RequestSHA256, Submission: raw, State: d.StateRunning, Phase: phase, ExecID: strings.Repeat("a", 64), Version: 1, Deadline: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now})
		require.NoError(t, err)
		return row, v, &allowed
	}
	for _, tc := range []struct {
		name, phase                              string
		cancel, expired, revoked, cleanupFailure bool
		want                                     d.State
	}{
		{"cancel launched", "launch_committed", true, false, false, false, d.StateCancelled},
		{"cancel collected", "collected", true, false, false, false, d.StateCancelled},
		{"deadline launched", "launch_committed", false, true, false, false, d.StateTimedOut},
		{"source revoked launched", "launch_committed", false, false, true, false, d.StateFailed},
		{"unconfirmed cleanup retries", "launch_committed", true, false, false, true, d.StateCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, validator, allowed := makeRecord(t, tc.phase)
			if tc.expired {
				require.NoError(t, db.Model(row).Update("deadline", time.Now().Add(-time.Second)).Error)
			}
			if tc.phase == "collected" {
				require.NoError(t, db.Model(row).Updates(map[string]any{"output_refs": `[{"asset_id":"11111111-1111-4111-8111-111111111111","version":1}]`, "manifest": `{}`}).Error)
			}
			scoped := &sandboxCleanupClaim{Repository: storage, db: db, p: p, id: row.ID}
			writes := 0
			runner, err := svc.NewDockerRunner(scoped, validator, &svc.DockerHost{Root: t.TempDir()}, func(context.Context, l.Principal, l.RefID) ([]byte, error) { return nil, d.ErrForbidden }, func(context.Context, l.Principal, string, []l.RefID, []byte) (l.RefID, error) {
				writes++
				return l.RefID{}, d.ErrForbidden
			}, func(_ context.Context, who l.Principal) error {
				if who != p {
					return d.ErrForbidden
				}
				return nil
			})
			require.NoError(t, err)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", p.TenantID); c.Set("user_id", p.UserID) })
			handler.NewSandboxHandler(runner, nil).Register(router.Group("/api"))
			if tc.cancel {
				body, err := json.Marshal(row.Binding().ExecutionRef)
				require.NoError(t, err)
				w := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/api/sandbox/runs/"+row.ID+"/cancel", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				require.Equal(t, 200, w.Code, w.Body.String())
			}
			*allowed = !tc.revoked
			if tc.cleanupFailure {
				require.NoError(t, os.WriteFile(failFile, nil, 0600))
			}
			worked, err := runner.Tick(ctx)
			require.NoError(t, err)
			require.True(t, worked)
			if tc.cleanupFailure {
				pending, err := storage.Get(ctx, p, row.ID)
				require.NoError(t, err)
				require.Equal(t, d.StateReconcileRequired, pending.State)
				require.Equal(t, "cleanup_pending", pending.Phase)
				require.Equal(t, "container_cleanup_unconfirmed", pending.ErrorCode)
				require.NoError(t, os.Remove(failFile))
				worked, err = runner.Tick(ctx)
				require.NoError(t, err)
				require.True(t, worked)
			}
			stored, err := storage.Get(ctx, p, row.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.State)
			require.Empty(t, stored.Manifest)
			require.Empty(t, stored.OutputRefs)
			require.Zero(t, writes)
			worked, err = runner.Tick(ctx)
			require.NoError(t, err)
			require.False(t, worked, "confirmed cleanup must stop being claimed")
		})
	}
	t.Run("late publication and stale worker remain fenced", func(t *testing.T) {
		row, _, _ := makeRecord(t, "launch_committed")
		scoped := &sandboxCleanupClaim{Repository: storage, db: db, p: p, id: row.ID}
		old, err := scoped.Claim(ctx, time.Minute)
		require.NoError(t, err)
		_, err = storage.RequestCancel(ctx, p, row.ID)
		require.NoError(t, err)
		old.State = d.StateCancelled
		require.ErrorIs(t, storage.Save(ctx, old), d.ErrConflict, "cancel invalidates the old claim")
		current, err := scoped.Claim(ctx, time.Minute)
		require.NoError(t, err)
		current.State, current.Phase = d.StateSucceeded, "collected"
		current.OutputRefs = json.RawMessage(`[]`)
		require.ErrorIs(t, storage.Save(ctx, current), d.ErrConflict)
		current, err = scoped.Claim(ctx, time.Minute)
		require.NoError(t, err)
		require.NoError(t, db.Model(&d.Record{}).Where("id=? AND tenant_id=?", row.ID, p.TenantID).Update("lease_until", time.Now().Add(-time.Second)).Error)
		current.State = d.StateCancelled
		require.ErrorIs(t, storage.Save(ctx, current), d.ErrConflict, "even confirmed cleanup needs a current lease")
		stored, err := storage.Get(ctx, p, row.ID)
		require.NoError(t, err)
		require.Equal(t, d.StateRunning, stored.State)
		require.Empty(t, stored.OutputRefs)
	})
	t.Run("expired deadline still prevents launch and success", func(t *testing.T) {
		row, _, _ := makeRecord(t, "launch_committed")
		require.NoError(t, db.Model(&d.Record{}).Where("id=? AND tenant_id=?", row.ID, p.TenantID).Update("deadline", time.Now().Add(-time.Second)).Error)
		scoped := &sandboxCleanupClaim{Repository: storage, db: db, p: p, id: row.ID}
		for _, state := range []d.State{d.StateRunning, d.StateSucceeded} {
			current, err := scoped.Claim(ctx, time.Minute)
			require.NoError(t, err)
			current.State = state
			require.ErrorIs(t, storage.Save(ctx, current), d.ErrConflict)
		}
		current, err := scoped.Claim(ctx, time.Minute)
		require.NoError(t, err)
		current.State = d.StateTimedOut
		require.NoError(t, storage.Save(ctx, current))
		stored, err := storage.Get(ctx, p, row.ID)
		require.NoError(t, err)
		require.Equal(t, d.StateTimedOut, stored.State)
		require.Empty(t, stored.OutputRefs)
	})
}
