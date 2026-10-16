//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	l "cognida/internal/model/lifecycle"
	policy "cognida/internal/model/strategy"
	d "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	strategies "cognida/internal/service/strategy"
	tasks "cognida/internal/service/taskcontrol"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type orchestrationSources struct{ fixture *supervisorHTTPFixture }

func (a orchestrationSources) Resolve(_ context.Context, tenant, user int64, ref l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	f := a.fixture
	if f.revoked.Load() || tenant != f.owner.TenantID || user != f.owner.UserID || ref != f.ref {
		return l.DataAssetRef{}, nil, policy.ErrForbidden
	}
	return l.DataAssetRef{RefID: ref, TenantID: tenant, Kind: l.RawAsset, Provenance: l.Provenance{Digest: "fixed-synthetic-source"}}, json.RawMessage(`{"kind":"text","text":"synthetic"}`), nil
}

// Real MySQL, HTTP trial/start APIs, controller/outbox and persisted child state;
// the domain adapter is an explicit deterministic fixture, not live Python work.
func TestStrategyMySQLOrchestrationTrialPinsActualControllerAndParentPair(t *testing.T) {
	f, r, domain := runtimeHTTPFixture(t)
	ctx := context.Background()
	p := l.Principal{TenantID: f.owner.TenantID, UserID: f.owner.UserID}
	t.Cleanup(func() {
		for _, table := range []string{"strategy_events", "strategy_versions", "strategy_heads"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	f.plan.Budget.MaxConcurrency = 2
	f.budget = f.plan.Budget
	second := f.plan.Nodes[0]
	second.ID = "second"
	f.plan.Nodes = append(f.plan.Nodes, second)
	s := strategies.NewService(repo.NewStrategyRepository(f.db), orchestrationSources{f}, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }, map[string]strategies.Consumer{"orchestration": {Name: "taskcontrol.concurrency.v1", Baseline: strategies.DefaultOrchestrationStrategy}})
	s.WithOrchestration(f.service.OrchestrationTrialBinding, f.service.OrchestrationRegressionReader(s))
	r.WithOrchestration(func(ctx context.Context, scope d.Scope, id string, plan tasks.PlanSpec) (int, error) {
		n, _, e := s.ResolveOrchestration(ctx, l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}, id, tasks.OrchestrationPlanDigest(plan), plan.Budget.MaxConcurrency)
		return n, e
	})
	NewStrategyHandler(s).Register(f.router.Group("/api/v1"))
	// First consumption freezes trusted defaults even before any admin registration.
	_, initial, e := s.ResolveOrchestration(ctx, p, "probe", tasks.OrchestrationPlanDigest(f.plan), 2)
	require.NoError(t, e)
	require.NotNil(t, initial)
	base, e := s.Read(ctx, p, initial.VersionID)
	require.NoError(t, e)
	candidate, e := s.CreateCandidate(ctx, p, "orchestration.default", strategies.CandidateInput{Kind: "orchestration", BaselineID: base.ID, Body: json.RawMessage(`{"max_parallel":1}`), EvidenceRefs: []string{f.ref.String()}}, "candidate")
	require.NoError(t, e)
	ids := []string{}
	for index, version := range []*policy.Version{base, candidate} {
		task, e := f.service.CreateDraft(ctx, f.owner, tasks.DraftInput{Goal: "fixed controlled trial", InputRefs: []l.RefID{f.ref}, Budget: f.budget}, uuid.NewString(), "trial")
		require.NoError(t, e)
		_, e = f.service.AppendPlan(ctx, f.owner, task.ID, tasks.AppendPlanInput{ExpectedVersion: task.Version, Plan: f.plan})
		require.NoError(t, e)
		task, e = f.repo.GetTask(ctx, f.owner, task.ID)
		require.NoError(t, e)
		before := f.counts()
		raw, _ := json.Marshal(map[string]string{"task_id": task.ID})
		req := httptest.NewRequest("POST", "/api/v1/strategies/orchestration.default/versions/"+version.ID+"/trials", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		trialKey := uuid.NewString()
		req.Header.Set("Idempotency-Key", trialKey)
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, req)
		require.Equal(t, 200, response.Code, response.Body.String())
		require.Equal(t, before, f.counts())
		_, e = s.PrepareOrchestrationTrial(ctx, p, version.ID, task.ID, trialKey)
		require.NoError(t, e, "same full intent replays its fixed pin")
		otherVersion := candidate.ID
		if otherVersion == version.ID {
			otherVersion = base.ID
		}
		_, e = s.PrepareOrchestrationTrial(ctx, p, otherVersion, task.ID, trialKey)
		require.ErrorIs(t, e, policy.ErrConflict, "request key cannot be rebound to a different version")
		runtimeStart(t, f, *task)
		require.NoError(t, runtimeStep(t, f, r, task.ID))
		children, e := f.repo.ListChildren(ctx, f.owner, task.ID)
		require.NoError(t, e)
		require.Len(t, children, 2-index, "actual persisted dispatch slots follow fixed policy")
		for step := 0; step < 20 && !runtimeTask(t, f, task.ID).Status.Terminal(); step++ {
			require.NoError(t, runtimeStep(t, f, r, task.ID))
			domain.finish(d.DomainSucceeded)
		}
		require.Equal(t, d.StatusCompleted, runtimeTask(t, f, task.ID).Status)
		fixed, e := s.ReadOrchestrationPin(ctx, p, task.ID, tasks.OrchestrationPlanDigest(f.plan))
		require.NoError(t, e)
		require.Equal(t, version.ID, fixed.VersionID)
		_, e = s.PrepareOrchestrationTrial(ctx, p, candidate.ID, task.ID, "late-trial")
		require.Error(t, e, "already started task cannot change its trial")
		ids = append(ids, task.ID)
	}
	proof, e := f.service.OrchestrationRegressionReader(s)(ctx, p, ids[0], ids[1])
	require.NoError(t, e)
	require.Equal(t, base.ID, proof.BaselineVersionID)
	require.Equal(t, candidate.ID, proof.CandidateVersionID)
	require.Equal(t, 2, proof.Candidate.Samples)
	require.Equal(t, float64(1), proof.Candidate.Quality)
	require.Greater(t, proof.Candidate.Cost, float64(0))
	require.Equal(t, "successful_node_fraction", proof.QualityMetric)
	_, e = s.Regress(ctx, p, "orchestration.default", candidate.ID, strategies.RegressionInput{BaselineTaskID: ids[0], CandidateTaskID: ids[1]}, "actual-pair")
	require.NoError(t, e) // May fail cost gate; storing truth never implies approval.
	f.revoked.Store(true)
	_, e = f.service.OrchestrationRegressionReader(s)(ctx, p, ids[0], ids[1])
	require.Error(t, e)
}
