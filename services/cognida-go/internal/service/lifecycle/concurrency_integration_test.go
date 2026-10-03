//go:build integration

package lifecycle

import (
	d "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type stageReadBarrier struct {
	d.Repository
	read, release chan struct{}
	once          *sync.Once
}

func (r *stageReadBarrier) Transaction(ctx context.Context, fn func(d.Repository) error) error {
	return r.Repository.Transaction(ctx, func(tx d.Repository) error {
		return fn(&stageReadBarrier{Repository: tx, read: r.read, release: r.release, once: r.once})
	})
}
func (r *stageReadBarrier) pause(ctx context.Context, stages []d.StageRun, err error) ([]d.StageRun, error) {
	if err != nil {
		return nil, err
	}
	r.once.Do(func() {
		close(r.read)
		select {
		case <-r.release:
		case <-ctx.Done():
		}
	})
	return stages, ctx.Err()
}
func (r *stageReadBarrier) ListStages(ctx context.Context, tenant int64, run string) ([]d.StageRun, error) {
	v, e := r.Repository.ListStages(ctx, tenant, run)
	return r.pause(ctx, v, e)
}
func (r *stageReadBarrier) LockStages(ctx context.Context, tenant int64, run string) ([]d.StageRun, error) {
	v, e := r.Repository.LockStages(ctx, tenant, run)
	return r.pause(ctx, v, e)
}
func twoStageTemplate(id string) d.PipelineDefinition {
	return d.PipelineDefinition{ID: id, Version: 1, Name: "concurrency fixture", MaxStages: 2, Enabled: true, Stages: []d.StageDefinition{{ID: "access", AgentID: "data-access", MaxAttempts: 3}, {ID: "quality", AgentID: "data-quality", DependsOn: []string{"access"}, AllowedGates: []string{d.GatePassed}, MaxAttempts: 3}}}
}
func TestLifecycleIntegrationCancellationCannotBeOverwrittenByReconcile(t *testing.T) {
	s, db, _, p, _ := integrationService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	template := twoStageTemplate("cancel-" + uuid.NewString())
	s.templates[template.ID] = template
	t.Cleanup(func() {
		require.NoError(t, db.Table("lifecycle_definitions").Where("id=?", template.ID).Delete(map[string]any{}).Error)
	})
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "cancel race"}, "fixture")
	require.NoError(t, e)
	run, e := s.StartRun(ctx, p, template.ID, []d.RefID{ref.RefID}, nil, "race", "race")
	require.NoError(t, e)
	base := s.repo
	stages, e := base.ListStages(ctx, p.TenantID, run.ID)
	require.NoError(t, e)
	var child string
	for i := range stages {
		if stages[i].StageKey == "access" {
			stages[i].Status = d.StatusSucceeded
			stages[i].Output = &d.Output{Refs: []d.DataAssetRef{ref}, Gate: d.GateResult{Status: d.GatePassed}}
			require.NoError(t, base.UpdateStage(ctx, &stages[i]))
		} else {
			child = stages[i].ID
		}
	}
	barrier := &stageReadBarrier{Repository: base, read: make(chan struct{}), release: make(chan struct{}), once: &sync.Once{}}
	reconcileService := *s
	reconcileService.repo = barrier
	reconciled := make(chan error, 1)
	go func() { reconciled <- reconcileService.reconcile(ctx, p.TenantID, run.ID) }()
	select {
	case <-barrier.read:
	case <-ctx.Done():
		t.Fatal("reconcile never read stages")
	}
	cancelled := make(chan error, 1)
	go func() { cancelled <- s.CancelStage(ctx, p, child) }()
	// On the unfixed implementation, cancellation commits while reconcile holds a
	// stale stage snapshot. With row locking it waits until that snapshot commits.
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		st, e := base.FindStage(ctx, p.TenantID, child, false)
		require.NoError(t, e)
		if st.Status == d.StatusCancelled {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(barrier.release)
	require.NoError(t, <-reconciled)
	require.NoError(t, <-cancelled)
	got, e := base.FindStage(ctx, p.TenantID, child, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusCancelled, got.Status, "reconcile must never resurrect an independently cancelled stage")
}
func TestLifecycleIntegrationRecoversPersistedTemplateVersion(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	template := twoStageTemplate("upgrade-" + uuid.NewString())
	s.templates[template.ID] = template
	t.Cleanup(func() {
		require.NoError(t, db.Table("lifecycle_definitions").Where("id=?", template.ID).Delete(map[string]any{}).Error)
	})
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "versioned run"}, "fixture")
	require.NoError(t, e)
	run, e := s.StartRun(ctx, p, template.ID, []d.RefID{ref.RefID}, nil, "upgrade", "upgrade")
	require.NoError(t, e)
	template.Version = 2
	template.Name = "new deployment"
	s.templates[template.ID] = template
	drain(t, s, q)
	recovered, e := s.repo.FindRun(ctx, p.TenantID, run.ID, false)
	require.NoError(t, e)
	require.Equal(t, 1, recovered.TemplateVersion)
	require.Equal(t, d.StatusSucceeded, recovered.Status)
}

func TestLifecycleIntegrationConcurrentIdempotencyAndWorkers(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	template := d.PipelineDefinition{ID: "workers-" + uuid.NewString(), Version: 1, Name: "concurrent workers", Enabled: true, MaxStages: 3, Stages: []d.StageDefinition{{ID: "left", AgentID: "data-access", MaxAttempts: 3}, {ID: "right", AgentID: "data-access", MaxAttempts: 3}, {ID: "join", AgentID: "data-quality", DependsOn: []string{"left", "right"}, AllowedGates: []string{d.GatePassed}, MaxAttempts: 3}}}
	s.templates[template.ID] = template
	t.Cleanup(func() {
		require.NoError(t, db.Table("lifecycle_definitions").Where("id=?", template.ID).Delete(map[string]any{}).Error)
	})
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "parallel invocation"}, "fixture")
	require.NoError(t, e)
	type result struct {
		run *d.PipelineRun
		err error
	}
	results := make(chan result, 12)
	for i := 0; i < 12; i++ {
		go func() {
			r, e := s.StartRun(ctx, p, template.ID, []d.RefID{ref.RefID}, nil, "shared-request", "shared-request")
			results <- result{r, e}
		}()
	}
	id := ""
	for i := 0; i < 12; i++ {
		r := <-results
		require.NoError(t, r.err)
		if id == "" {
			id = r.run.ID
		}
		require.Equal(t, id, r.run.ID)
	}
	size, e := q.ReadySize(ctx)
	require.NoError(t, e)
	require.EqualValues(t, 2, size, "each root stage enqueued once")
	workers := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			for {
				j, e := q.TryClaim(ctx)
				if e != nil {
					workers <- e
					return
				}
				if j == nil {
					workers <- nil
					return
				}
				if e = s.Handle(ctx, *j); e != nil {
					workers <- e
					return
				}
				if e = q.Ack(ctx, *j); e != nil {
					workers <- e
					return
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, <-workers)
	}
	drain(t, s, q)
	run, e := s.repo.FindRun(ctx, p.TenantID, id, false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, run.Status)
	stages, e := s.repo.ListStages(ctx, p.TenantID, id)
	require.NoError(t, e)
	require.Len(t, stages, 3)
	for _, stage := range stages {
		require.Equal(t, 1, stage.Attempt)
		require.Equal(t, d.StatusSucceeded, stage.Status)
	}
	var starts int64
	require.NoError(t, db.Table("lifecycle_events").Where("tenant_id=? AND run_id=? AND type='stage.started'", p.TenantID, id).Count(&starts).Error)
	require.EqualValues(t, 3, starts)
}

type scopedRunRecovery struct {
	d.Repository
	tenant int64
	ids    []string
}

func (r scopedRunRecovery) RecoverableRuns(ctx context.Context, _ int) ([]d.PipelineRun, error) {
	out := []d.PipelineRun{}
	for _, id := range r.ids {
		run, e := r.Repository.FindRun(ctx, r.tenant, id, false)
		if e != nil {
			return nil, e
		}
		out = append(out, *run)
	}
	return out, nil
}
func (r scopedRunRecovery) ExpiredApprovals(context.Context, time.Time, int) ([]d.Approval, error) {
	return nil, nil
}
func (r scopedRunRecovery) PendingStages(ctx context.Context, _ int) ([]d.StageRun, error) {
	out := []d.StageRun{}
	for _, id := range r.ids {
		rows, e := r.Repository.ListStages(ctx, r.tenant, id)
		if e != nil {
			return nil, e
		}
		for _, row := range rows {
			if row.Status == d.StatusQueued || row.Status == d.StatusRunning {
				out = append(out, row)
			}
		}
	}
	return out, nil
}
func TestLifecycleIntegrationInvalidDefinitionDoesNotBlockOtherRecovery(t *testing.T) {
	s, db, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, e := s.Import(ctx, p, d.Input{Kind: "text", Text: "recovery isolation"}, "fixture")
	require.NoError(t, e)
	ids := []string{}
	definitions := []string{}
	for _, kind := range []string{"missing", "incompatible", "malformed", "healthy"} {
		template := twoStageTemplate(kind + "-" + uuid.NewString())
		s.templates[template.ID] = template
		definitions = append(definitions, template.ID)
		run, e := s.StartRun(ctx, p, template.ID, []d.RefID{ref.RefID}, nil, kind, kind)
		require.NoError(t, e)
		ids = append(ids, run.ID)
	}
	t.Cleanup(func() {
		require.NoError(t, db.Table("lifecycle_definitions").Where("id IN ?", definitions).Delete(map[string]any{}).Error)
	})
	require.NoError(t, db.Table("lifecycle_definitions").Where("id=?", definitions[0]).Delete(map[string]any{}).Error)
	require.NoError(t, db.Exec("UPDATE lifecycle_definitions SET body=JSON_SET(body,'$.stages[0].agent_id','unregistered-agent') WHERE id=?", definitions[1]).Error)
	require.NoError(t, db.Exec("UPDATE lifecycle_definitions SET body=JSON_SET(body,'$.version','broken') WHERE id=?", definitions[2]).Error)
	s.repo = scopedRunRecovery{Repository: s.repo, tenant: p.TenantID, ids: ids}
	jobs, e := s.PendingJobs(ctx, 100)
	require.NoError(t, e)
	require.Len(t, jobs, 1, "bad definitions must not stop healthy queued stages")
	for _, id := range ids[:3] {
		run, e := s.repo.FindRun(ctx, p.TenantID, id, false)
		require.NoError(t, e)
		require.Equal(t, d.StatusQuarantined, run.Status)
		require.Contains(t, run.Error, "persisted pipeline definition")
	}
	drain(t, s, q)
	run, e := s.repo.FindRun(ctx, p.TenantID, ids[3], false)
	require.NoError(t, e)
	require.Equal(t, d.StatusSucceeded, run.Status)
}

func TestLifecycleIntegrationDerivedTablesKeepExactNumbers(t *testing.T) {
	s, _, q, p, _ := integrationService(t)
	ctx := context.Background()
	ref, err := s.Import(ctx, p, d.Input{Kind: "table", Columns: []string{"entity", "amount"}, Rows: [][]any{{json.Number("9007199254740993"), json.Number("10.25")}}}, "exact")
	require.NoError(t, err)
	run, err := s.StartRun(ctx, p, "web-knowledge-to-insight", []d.RefID{ref.RefID}, nil, "exact", "exact")
	require.NoError(t, err)
	drain(t, s, q)
	approvals, err := s.repo.ListApprovals(ctx, p.TenantID, run.ID)
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	require.NoError(t, s.ResolveApproval(ctx, p, approvals[0].ID, "approved", "precision reviewed"))
	drain(t, s, q)
	stages, err := s.repo.ListStages(ctx, p.TenantID, run.ID)
	require.NoError(t, err)
	seen := map[d.Kind]bool{}
	for _, stage := range stages {
		require.Equal(t, d.StatusSucceeded, stage.Status)
		for _, out := range stage.Output.Refs {
			if out.Kind != d.CleanedAsset && out.Kind != d.EnrichedAsset && out.Kind != d.PublishedAsset {
				continue
			}
			_, body, err := s.Resolve(ctx, p.TenantID, p.UserID, out.RefID)
			require.NoError(t, err)
			var input d.Input
			require.NoError(t, json.Unmarshal(body, &input))
			require.Equal(t, json.Number("9007199254740993"), input.Rows[0][0])
			require.Equal(t, json.Number("10.25"), input.Rows[0][1])
			seen[out.Kind] = true
		}
	}
	require.True(t, seen[d.CleanedAsset])
	require.True(t, seen[d.EnrichedAsset])
	require.True(t, seen[d.PublishedAsset])
}
