//go:build integration

package sandboxadapter_test

import (
	"cognida/internal/infrastructure/job"
	objects "cognida/internal/infrastructure/objectstore"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	m "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/lifecycle"
	sb "cognida/internal/service/sandbox"
	tc "cognida/internal/service/taskcontrol"
	adapter "cognida/internal/service/taskcontrol/sandboxadapter"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type noQueue struct{ job.Queue }

func (noQueue) Enqueue(context.Context, job.Job) error { return errors.New("unexpected queue") }
func TestSandboxGovernedMySQLRealDockerRecoveryAndCancellation(t *testing.T) {
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	image := os.Getenv("COGNIDA_TEST_SANDBOX_IMAGE")
	if dsn == "" || image == "" {
		t.Skip("isolated MySQL and real digest-pinned Docker image required")
	}
	cfg, e := driver.ParseDSN(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, e := gorm.Open(gormmysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	sql, e := db.DB()
	require.NoError(t, e)
	p := l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 7331}
	ctx := context.Background()
	var revoked atomic.Bool
	t.Cleanup(func() {
		for _, table := range []string{"sandbox_runs", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	storage, e := objects.NewLocal(t.TempDir())
	require.NoError(t, e)
	var runner *sb.DockerRunner
	assets, e := lifecycle.NewService(repo.NewLifecycleRepository(db), storage, noQueue{}, lifecycle.Config{Authorize: func(_ context.Context, who l.Principal, _ string) error {
		if who != p || revoked.Load() {
			return l.ErrForbidden
		}
		return nil
	}, ValidateGovernedOrigin: func(ctx context.Context, who l.Principal, ref l.DataAssetRef) error {
		return runner.ValidateOrigin(ctx, who, ref)
	}})
	require.NoError(t, e)
	code, e := assets.ImportWithKey(ctx, p, l.Input{Kind: "text", Text: "import json;json.dump({'kind':'text','text':'governed Docker output'},open('/output/result.json','w'))"}, "test code", uuid.NewString())
	require.NoError(t, e)
	_, body, e := assets.Resolve(ctx, p.TenantID, p.UserID, code.RefID)
	require.NoError(t, e)
	hash := sha256.Sum256(body)
	limits := d.ResourceLimits{CPUMillis: 1000, MemoryBytes: 256 << 20, DiskBytes: 32 << 20, Processes: 32, TimeoutSeconds: 40, InputBytes: 8 << 20, OutputBytes: 8 << 20, OutputFiles: 16, LogBytes: 64 << 10}
	profile := d.Profile{ID: "python-test", Version: 1, Approved: true, IsolationLevel: d.ControlledDevelopment, Image: image, MaxResources: limits, AllowedMediaTypes: []string{"application/json", "text/plain"}, Isolation: d.IsolationRequirements{NonRoot: true, ReadOnlyRoot: true, DropCapabilities: true, NoNewPrivileges: true, SyscallFilter: true, IsolatedWorkspace: true, ReadOnlyInputs: true, NetworkDisabled: true}}
	load := func(ctx context.Context, who l.Principal, id l.RefID) ([]byte, error) {
		_, raw, e := assets.Resolve(ctx, who.TenantID, who.UserID, id)
		return raw, e
	}
	validator, e := sb.NewValidator([]d.Profile{profile}, func(ctx context.Context, who l.Principal, id l.RefID) (d.InputMetadata, error) {
		return runner.Metadata(ctx, who, id)
	})
	require.NoError(t, e)
	host := &sb.DockerHost{Root: t.TempDir()}
	repository := repo.NewSandboxRepository(db)
	writes := 0
	writer := func(ctx context.Context, who l.Principal, producer string, parents []l.RefID, raw []byte) (l.RefID, error) {
		writes++
		ref, e := assets.ImportDerived(ctx, who, l.CleanedAsset, producer, parents, raw, l.PolicySnapshot{Scopes: []string{"read"}, Version: "1"})
		return ref.RefID, e
	}
	authorize := func(_ context.Context, who l.Principal) error {
		if who != p {
			return d.ErrForbidden
		}
		return nil
	}
	runner, e = sb.NewDockerRunner(repository, validator, host, load, writer, authorize)
	require.NoError(t, e)
	a, e := adapter.New(runner, assets)
	require.NoError(t, e)
	cap := a.Capability()
	command := func(task string) tc.ExecutionCommand {
		c := tc.ExecutionCommand{Scope: m.Scope{TenantID: p.TenantID, UserID: p.UserID}, TaskID: task, PlanVersion: 1, Attempt: 1, Deadline: time.Now().Add(30 * time.Second), RequestID: "governed-sandbox-test", InputRefs: []l.RefID{code.RefID}, Node: tc.Node{ID: "sandbox", Capability: cap.ID, CapabilityVersion: 1, Inputs: []tc.InputBinding{{Asset: &code.RefID, Kind: code.Kind}}, OutputKinds: cap.OutputKinds, Tools: cap.Tools, Permissions: cap.Permissions, MaxAttempts: 1, MaxCallsPerAttempt: 19, TimeoutSeconds: 40, Parameters: &tc.NodeParameters{Sandbox: &tc.SandboxParameters{ProfileID: profile.ID, ProfileVersion: 1, ImageSHA256: image[strings.LastIndex(image, "@")+1:], Code: d.InputRef{Name: "code", Ref: code.RefID, SHA256: "sha256:" + hex.EncodeToString(hash[:]), SizeBytes: int64(len(body))}, Resources: limits}}}}
		raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
		c.OperationKey = m.Digest(raw)
		raw, _ = json.Marshal(c)
		raw, _ = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
		c.PayloadHash = m.Digest(raw)
		return c
	}
	c := command("success")
	require.NoError(t, a.Validate(ctx, c.Scope, c.Node))
	h, e := a.Submit(ctx, c)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, host.Remove(ctx, h.DomainID)) })
	replay, e := a.Submit(ctx, c)
	require.NoError(t, e)
	require.Equal(t, h, replay)
	row, e := repository.Get(ctx, p, h.DomainID)
	require.NoError(t, e)
	require.WithinDuration(t, c.Deadline, row.Deadline, time.Microsecond)
	require.Equal(t, c.RequestID, row.RequestID)
	_, e = runner.Tick(ctx)
	require.NoError(t, e)
	runner, e = sb.NewDockerRunner(repository, validator, host, load, writer, authorize)
	require.NoError(t, e)
	a, e = adapter.New(runner, assets)
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		_, e := runner.Tick(ctx)
		require.NoError(t, e)
		row, e := runner.Get(ctx, p, h.DomainID)
		require.NoError(t, e)
		if d.Terminal(row.State) {
			require.Equal(t, d.StateSucceeded, row.State, row.ErrorCode)
			return true
		}
		return false
	}, 25*time.Second, 150*time.Millisecond)
	observed, e := a.Inspect(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, observed.Status)
	require.Len(t, observed.OutputRefs, 1)
	require.Equal(t, 1, writes)
	pending := command("cancel")
	ph, e := a.Submit(ctx, pending)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, host.Remove(ctx, ph.DomainID)) })
	wrong := *ph
	wrong.DomainID = h.DomainID
	_, e = a.Cancel(ctx, pending, wrong)
	require.ErrorIs(t, e, m.ErrConflict)
	revoked.Store(true)
	_, e = a.Inspect(ctx, pending, *ph)
	require.ErrorIs(t, e, m.ErrForbidden)
	successful, e := a.Cancel(ctx, c, *h)
	require.NoError(t, e)
	require.Equal(t, m.DomainSucceeded, successful.Status)
	require.Empty(t, successful.OutputRefs)
	cancelObs, e := a.Cancel(ctx, pending, *ph)
	require.NoError(t, e)
	require.Equal(t, m.DomainRunning, cancelObs.Status)
	_, e = runner.Tick(ctx)
	require.NoError(t, e)
	row, e = repository.Get(ctx, p, ph.DomainID)
	require.NoError(t, e)
	require.Equal(t, d.StateCancelled, row.State)
	require.Empty(t, row.OutputRefs)
	tomb := command("late")
	th, e := a.CancelSubmission(ctx, tomb)
	require.NoError(t, e)
	late, e := a.Submit(ctx, tomb)
	require.NoError(t, e)
	require.Equal(t, th, late)
	row, e = repository.Get(ctx, p, th.DomainID)
	require.NoError(t, e)
	require.Equal(t, d.StateCancelled, row.State)
	require.Equal(t, 1, writes)
}
