//go:build integration

package mysql

import (
	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	agent "cognida/internal/service/agent"
	"cognida/internal/service/researchrun"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func researchIT(t *testing.T) (*ResearchRunRepository, *gorm.DB, d.Scope) {
	_, db, p := taskControlIT(t)
	var version int
	require.NoError(t, db.Raw("SELECT version FROM schema_migrations LIMIT 1").Scan(&version).Error)
	require.GreaterOrEqual(t, version, 25)
	t.Cleanup(func() {
		require.NoError(t, db.Table("research_runs").Where("tenant_id = ?", p.TenantID).Delete(map[string]any{}).Error)
	})
	return NewResearchRunRepository(db), db, p
}
func researchInput() r.Submission {
	return r.Submission{OperationKey: d.Digest([]byte(uuid.NewString())), PayloadHash: d.Digest([]byte(uuid.NewString())), Profile: r.Profile, Request: r.Request{Query: "Synthetic evidence question", KBIDs: []string{"allowed-kb"}}, Deadline: time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond), TimeoutSeconds: 60, MaxCalls: r.MaxCalls}
}
func TestResearchRunIntegrationAtomicReceiptCancellationAndScope(t *testing.T) {
	repo, _, p := researchIT(t)
	ctx := context.Background()
	in := researchInput()
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, e := repo.Submit(ctx, p, in, false)
			errs <- e
			if e == nil {
				ids <- row.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	unique := map[string]bool{}
	var id string
	for id = range ids {
		unique[id] = true
	}
	require.Len(t, unique, 1)
	row, e := repo.Get(ctx, p, id)
	require.NoError(t, e)
	require.Equal(t, "queued", row.Status)
	changed := in
	changed.Request.Query = "different question"
	_, e = repo.Submit(ctx, p, changed, false)
	require.ErrorIs(t, e, d.ErrConflict)
	changed = in
	changed.PayloadHash = strings.Repeat("c", 64)
	_, e = repo.Submit(ctx, p, changed, false)
	require.ErrorIs(t, e, d.ErrConflict)
	for _, foreign := range []d.Scope{{TenantID: p.TenantID + 1, UserID: p.UserID}, {TenantID: p.TenantID, UserID: p.UserID + 1}} {
		_, e = repo.Get(ctx, foreign, id)
		require.ErrorIs(t, e, d.ErrNotFound)
		got, e := repo.Lookup(ctx, foreign, in.OperationKey, in.PayloadHash)
		require.NoError(t, e)
		require.Nil(t, got)
		_, e = repo.Cancel(ctx, foreign, id)
		require.ErrorIs(t, e, d.ErrNotFound)
	}
	tomb := researchInput()
	cancelled, e := repo.Submit(ctx, p, tomb, true)
	require.NoError(t, e)
	replay, e := repo.Submit(ctx, p, tomb, false)
	require.NoError(t, e)
	require.Equal(t, cancelled.ID, replay.ID)
	require.Equal(t, "cancelled", replay.Status)
	_, e = repo.Claim(ctx, p, replay.ID, "late-worker", time.Second)
	require.ErrorIs(t, e, d.ErrConflict)
}
func TestResearchRunIntegrationExpiredClaimNeverReplaysPaidCall(t *testing.T) {
	repo, db, p := researchIT(t)
	ctx := context.Background()
	in := researchInput()
	row, e := repo.Submit(ctx, p, in, false)
	require.NoError(t, e)
	claimed, e := repo.Claim(ctx, p, row.ID, "old", time.Second)
	require.NoError(t, e)
	require.Equal(t, "running", claimed.Status)
	checkpoint := json.RawMessage(`{"metadata":{"model_calls_used":2,"model_calls_reserved":2,"draft_repair":{"attempts":0,"status":"reserved","trigger":{"stage":"validating","code":"draft_required_fields"}}},"sources":[{"id":"S1","excerpt":"retained original evidence"}]}`)
	_, e = repo.Checkpoint(ctx, p, row.ID, "old", "repairing", 0.9, checkpoint)
	require.NoError(t, e)
	_, e = repo.Claim(ctx, p, row.ID, "second", time.Second)
	require.ErrorIs(t, e, d.ErrConflict)
	require.NoError(t, researchScope(db, p).Model(&r.Run{}).Where("id = ?", row.ID).Update("lease_until_us", 1).Error)
	recovered, e := repo.Claim(ctx, p, row.ID, "restart", time.Second)
	require.NoError(t, e)
	require.Equal(t, "failed", recovered.Status)
	require.Contains(t, recovered.Error, "research_interrupted_unconfirmed")
	// A crash after reservation may hide a paid in-flight request; no repair,
	// generation or review is automatically replayed on the original run.
	require.JSONEq(t, string(checkpoint), string(recovered.Report))
	_, e = repo.Complete(ctx, p, row.ID, "old", r.Completion{Status: "succeeded", Report: json.RawMessage(`{}`), OutputRefs: json.RawMessage(`[]`)})
	require.ErrorIs(t, e, d.ErrConflict)
	_, e = repo.Claim(ctx, p, row.ID, "again", time.Second)
	require.ErrorIs(t, e, d.ErrConflict)
	expired := researchInput()
	expired.Deadline = time.Now().Add(-time.Second)
	pending, e := repo.Submit(ctx, p, expired, false)
	require.NoError(t, e)
	done, e := repo.Claim(ctx, p, pending.ID, "worker", time.Second)
	require.NoError(t, e)
	require.Equal(t, "timed_out", done.Status)
}

type researchEngineFixture struct {
	calls atomic.Int32
	run   func(context.Context) (*agent.DeepResearchResponse, error)
}

func (e *researchEngineFixture) Available() bool { return true }
func (e *researchEngineFixture) ExecuteGoverned(ctx context.Context, _ *agent.DeepResearchRequest) (*agent.DeepResearchResponse, error) {
	e.calls.Add(1)
	return e.run(ctx)
}
func researchReportFixture() *agent.DeepResearchResponse {
	return &agent.DeepResearchResponse{Success: true, Query: "Synthetic evidence question", ExecutiveSummary: "Synthetic summary [S1]", DetailedAnalysis: "Synthetic analysis [S1]", Sources: []agent.ReportSourceDTO{{ID: "S1", Type: "rag", KnowledgeBaseID: "allowed-kb", KnowledgeID: "doc", ChunkID: "chunk", Excerpt: "Synthetic evidence from this isolated test."}}, Metadata: agent.ReportMetadataDTO{CitationChecked: true}}
}
func TestResearchRunIntegrationDurableReportAndRevocation(t *testing.T) {
	repo, _, p := researchIT(t)
	ctx := context.Background()
	var denied atomic.Bool
	var written atomic.Int32
	auth := func(_ context.Context, scope d.Scope, kbs []string, report *agent.DeepResearchResponse) error {
		if scope != p || denied.Load() || len(kbs) != 1 || kbs[0] != "allowed-kb" {
			return d.ErrForbidden
		}
		return nil
	}
	ref := l.RefID{AssetID: uuid.NewString(), Version: 1}
	store := func(context.Context, d.Scope, string, *agent.DeepResearchResponse) (l.RefID, error) {
		written.Add(1)
		return ref, nil
	}
	svc, e := researchrun.New(repo, auth, store)
	require.NoError(t, e)
	engine := &researchEngineFixture{run: func(context.Context) (*agent.DeepResearchResponse, error) { return researchReportFixture(), nil }}
	svc.SetEngine(engine)
	in := researchInput()
	row, e := svc.Submit(ctx, p, in)
	require.NoError(t, e)
	require.Zero(t, engine.calls.Load())
	require.NoError(t, svc.Process(ctx, p, row.ID))
	require.Equal(t, int32(1), engine.calls.Load())
	require.Equal(t, int32(1), written.Load())
	saved, e := svc.Read(ctx, p, row.ID)
	require.NoError(t, e)
	require.Equal(t, "succeeded", saved.Status)
	require.Contains(t, string(saved.Report), "Synthetic summary")
	restarted, e := researchrun.New(NewResearchRunRepository(repo.db), auth, store)
	require.NoError(t, e)
	restarted.SetEngine(engine)
	same, e := restarted.Lookup(ctx, p, in)
	require.NoError(t, e)
	require.Equal(t, row.ID, same.ID)
	require.ErrorIs(t, restarted.Process(ctx, p, row.ID), d.ErrConflict)
	require.Equal(t, int32(1), engine.calls.Load())
	_, e = restarted.Read(ctx, d.Scope{TenantID: p.TenantID, UserID: p.UserID + 1}, row.ID)
	require.ErrorIs(t, e, d.ErrNotFound)
	raw, _ := json.Marshal(researchReportFixture())
	asset := l.DataAssetRef{RefID: ref, Kind: l.Insight, ProducerID: "research-governed:" + row.ID, Provenance: l.Provenance{Digest: d.Digest(raw)}}
	require.NoError(t, restarted.ValidateGovernedOrigin(ctx, l.Principal{TenantID: p.TenantID, UserID: p.UserID}, asset))
	require.NoError(t, restarted.ValidateInputs(ctx, p, []l.RefID{ref}))
	denied.Store(true)
	_, e = restarted.Read(ctx, p, row.ID)
	require.ErrorIs(t, e, d.ErrForbidden)
	require.ErrorIs(t, restarted.ValidateGovernedOrigin(ctx, l.Principal{TenantID: p.TenantID, UserID: p.UserID}, asset), d.ErrForbidden)
	require.ErrorIs(t, restarted.ValidateInputs(ctx, p, []l.RefID{ref}), d.ErrForbidden)
}
func TestResearchRunIntegrationRunningCancelSuppressesLateReport(t *testing.T) {
	repo, _, p := researchIT(t)
	ctx := context.Background()
	entered := make(chan struct{})
	engine := &researchEngineFixture{run: func(ctx context.Context) (*agent.DeepResearchResponse, error) {
		close(entered)
		<-ctx.Done()
		return researchReportFixture(), nil
	}}
	var outputs atomic.Int32
	svc, e := researchrun.New(repo, func(context.Context, d.Scope, []string, *agent.DeepResearchResponse) error { return nil }, func(context.Context, d.Scope, string, *agent.DeepResearchResponse) (l.RefID, error) {
		outputs.Add(1)
		return l.RefID{AssetID: uuid.NewString(), Version: 1}, nil
	})
	require.NoError(t, e)
	svc.SetEngine(engine)
	row, e := svc.Submit(ctx, p, researchInput())
	require.NoError(t, e)
	finished := make(chan error, 1)
	go func() { finished <- svc.Process(ctx, p, row.ID) }()
	<-entered
	requested, e := svc.Cancel(ctx, p, row.ID)
	require.NoError(t, e)
	require.Equal(t, "cancel_requested", requested.Status)
	select {
	case e = <-finished:
		require.NoError(t, e)
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not settle")
	}
	done, e := svc.Read(ctx, p, row.ID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", done.Status)
	require.Zero(t, outputs.Load())
	require.Empty(t, done.Report)
	require.JSONEq(t, `[]`, string(done.OutputRefs))
	require.Equal(t, int32(1), engine.calls.Load())
}
func TestResearchRunIntegrationAuthorizationAndStorageFailuresDoNotReplay(t *testing.T) {
	for _, scenario := range []string{"revoked", "storage", "engine"} {
		t.Run(scenario, func(t *testing.T) {
			repo, _, p := researchIT(t)
			ctx := context.Background()
			var revoked atomic.Bool
			var writes atomic.Int32
			engine := &researchEngineFixture{run: func(context.Context) (*agent.DeepResearchResponse, error) {
				if scenario == "revoked" {
					revoked.Store(true)
				}
				if scenario == "engine" {
					return nil, errors.New("private upstream body")
				}
				return researchReportFixture(), nil
			}}
			svc, e := researchrun.New(repo, func(context.Context, d.Scope, []string, *agent.DeepResearchResponse) error {
				if revoked.Load() {
					return d.ErrForbidden
				}
				return nil
			}, func(context.Context, d.Scope, string, *agent.DeepResearchResponse) (l.RefID, error) {
				writes.Add(1)
				return l.RefID{}, errors.New("storage unavailable")
			})
			require.NoError(t, e)
			svc.SetEngine(engine)
			row, e := svc.Submit(ctx, p, researchInput())
			require.NoError(t, e)
			require.NoError(t, svc.Process(ctx, p, row.ID))
			stored, e := repo.Get(ctx, p, row.ID)
			require.NoError(t, e)
			require.Equal(t, "failed", stored.Status)
			require.NotContains(t, stored.Error, "private upstream")
			if scenario == "storage" {
				require.NotEmpty(t, stored.Report)
			} else {
				require.Empty(t, stored.Report)
			}
			require.ErrorIs(t, svc.Process(ctx, p, row.ID), d.ErrConflict)
			require.Equal(t, int32(1), engine.calls.Load())
			if scenario != "storage" {
				require.Zero(t, writes.Load())
			}
		})
	}
}

func TestResearchRunIntegrationCheckpointCancellationAndPartial(t *testing.T) {
	repo, _, p := researchIT(t)
	ctx := context.Background()
	row, e := repo.Submit(ctx, p, researchInput(), false)
	require.NoError(t, e)
	_, e = repo.Claim(ctx, p, row.ID, "worker", time.Minute)
	require.NoError(t, e)
	report := json.RawMessage(`{"sources":[{"id":"S1","excerpt":"saved before cancel"}],"success":false}`)
	_, e = repo.Checkpoint(ctx, p, row.ID, "worker", "evidence_collected", 0.6, report)
	require.NoError(t, e)
	_, e = repo.Cancel(ctx, p, row.ID)
	require.NoError(t, e)
	_, e = repo.Checkpoint(ctx, p, row.ID, "worker", "late", 0.9, json.RawMessage(`{"secret":"late"}`))
	require.ErrorIs(t, e, d.ErrConflict)
	done, e := repo.Complete(ctx, p, row.ID, "worker", r.Completion{Status: "succeeded", Report: json.RawMessage(`{"late":true}`), OutputRefs: json.RawMessage(`[]`)})
	require.NoError(t, e)
	require.Equal(t, "cancelled", done.Status)
	require.JSONEq(t, string(report), string(done.Report))
	require.NotContains(t, string(done.Report), "late")
	next, e := repo.Submit(ctx, p, researchInput(), false)
	require.NoError(t, e)
	_, e = repo.Claim(ctx, p, next.ID, "second", time.Minute)
	require.NoError(t, e)
	partial, e := repo.Complete(ctx, p, next.ID, "second", r.Completion{Status: "partial", Report: report, OutputRefs: json.RawMessage(`[]`)})
	require.NoError(t, e)
	require.True(t, partial.Terminal())
	require.NotEmpty(t, partial.Report)
}

func TestResearchRunIntegrationRenewMaintainsLeaseAndOwnerFence(t *testing.T) {
	repo, _, p := researchIT(t)
	ctx := context.Background()
	row, err := repo.Submit(ctx, p, researchInput(), false)
	require.NoError(t, err)
	claimed, err := repo.Claim(ctx, p, row.ID, "worker", time.Second)
	require.NoError(t, err)
	renewed, err := repo.Renew(ctx, p, row.ID, "worker", 15*time.Second)
	require.NoError(t, err)
	require.Greater(t, renewed.LeaseUntilUS, claimed.LeaseUntilUS)
	actual, err := repo.Get(ctx, p, row.ID)
	require.NoError(t, err)
	require.Equal(t, renewed.LeaseUntilUS, actual.LeaseUntilUS)
	_, err = repo.Renew(ctx, p, row.ID, "foreign-worker", 15*time.Second)
	require.ErrorIs(t, err, d.ErrConflict)
}
