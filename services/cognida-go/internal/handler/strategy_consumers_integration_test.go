//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	store "cognida/internal/infrastructure/objectstore"
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/rag"
	d "cognida/internal/model/strategy"
	repo "cognida/internal/repository/mysql"
	knowledge "cognida/internal/service/knowledge"
	lifecycle "cognida/internal/service/lifecycle"
	strategies "cognida/internal/service/strategy"
	"github.com/stretchr/testify/require"
)

type strategyRetriever struct {
	rag.Retriever
	seen       *rag.RetrieveOptions
	tenant, kb string
	after      func()
}

func (r *strategyRetriever) HybridRetrieve(_ context.Context, tenant, kb, _ string, o *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	r.tenant, r.kb, r.seen = tenant, kb, o
	if r.after != nil {
		r.after()
	}
	return &rag.RetrieveResponse{Results: []*rag.Document{{ChunkID: "c", Content: "synthetic authorized evidence", Score: 1}}}, nil
}
func TestStrategyMySQLRetrievalConsumesExactPinAndRechecksSource(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := agentctx.WithRequestID(agentctx.WithTenantID(agentctx.WithUserID(context.Background(), f.p.UserID), f.p.TenantID), "retrieval-once")
	t.Cleanup(func() {
		for _, table := range []string{"strategy_events", "strategy_versions", "strategy_heads"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	s := strategies.NewService(repo.NewStrategyRepository(f.db), f.assets, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }, map[string]strategies.Consumer{"retrieval": {Name: "knowledge.retrieval.v1", Baseline: knowledge.DefaultRetrievalStrategy}})
	base, e := s.RegisterBaseline(ctx, f.p, "retrieval.default", "retrieval", "baseline")
	require.NoError(t, e)
	retriever := &strategyRetriever{}
	cap := knowledge.NewRetrievalCapability(retriever, nil).WithStrategies(s)
	q := knowledge.GovernedQuery{Query: "synthetic", TopK: 7, MinScore: .2, Mode: "hybrid"}
	_, e = cap.Retrieve(ctx, f.p.TenantID, []string{"authorized-kb"}, q)
	require.NoError(t, e)
	require.Equal(t, 7, retriever.seen.TopK)
	q.TopK = 19 // Retrying the same run keeps its first effective settings.
	_, e = cap.Retrieve(ctx, f.p.TenantID, []string{"authorized-kb"}, q)
	require.NoError(t, e)
	require.Equal(t, 7, retriever.seen.TopK)
	require.Equal(t, strconv.FormatInt(f.p.TenantID, 10), retriever.tenant)
	require.Equal(t, "authorized-kb", retriever.kb)
	candidate, e := s.CreateCandidate(ctx, f.p, "retrieval.default", strategies.CandidateInput{Kind: "retrieval", BaselineID: base.ID, Body: json.RawMessage(`{"top_k":3,"score_threshold":0.4,"rerank":false}`), EvidenceRefs: []string{f.assets.ref.AssetRefID}}, "candidate")
	require.NoError(t, e)
	pin, e := s.PinEvaluation(ctx, f.p, candidate.ID, "isolated-evaluation")
	require.NoError(t, e)
	candidateCtx := strategies.WithSnapshot(ctx, pin)
	_, e = cap.Retrieve(candidateCtx, f.p.TenantID, []string{"authorized-kb"}, q)
	require.NoError(t, e)
	require.Equal(t, 3, retriever.seen.TopK)
	require.Equal(t, .4, retriever.seen.SimilarityThreshold)
	tampered := *pin
	tampered.EffectiveBody = json.RawMessage(`{"top_k":30}`)
	require.Error(t, s.ValidateSnapshot(ctx, f.p, &tampered))
	retriever.after = func() { f.assets.deny = true }
	_, e = cap.Retrieve(candidateCtx, f.p.TenantID, []string{"authorized-kb"}, q)
	require.Error(t, e)
}

func TestStrategyMySQLCleaningPreviewIsReadonlyAndMatchesExecution(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	t.Cleanup(func() {
		for _, table := range []string{"strategy_events", "strategy_versions", "strategy_heads", "lifecycle_events", "lifecycle_approvals", "lifecycle_stages", "lifecycle_runs", "lifecycle_assets"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	objects, e := store.NewLocal(t.TempDir())
	require.NoError(t, e)
	q := &cleaningQueue{}
	life, e := lifecycle.NewService(repo.NewLifecycleRepository(f.db), objects, q, lifecycle.Config{Compute: cleaningFixtureCompute{}, Authorize: func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }})
	require.NoError(t, e)
	s := strategies.NewService(repo.NewStrategyRepository(f.db), f.assets, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }, map[string]strategies.Consumer{"cleaning": {Name: "lifecycle.cleaning.v1", Baseline: lifecycle.DefaultCleaningStrategy}})
	life.WithStrategies(s)
	_, e = s.RegisterBaseline(ctx, f.p, "cleaning.default", "cleaning", "baseline")
	require.NoError(t, e)
	source, e := life.ImportWithKey(ctx, f.p, l.Input{Kind: "table", Columns: []string{"id"}, Rows: [][]any{{" a "}, {" a "}}}, "synthetic", "source")
	require.NoError(t, e)
	options := map[string]any{"operations": []string{"trim", "dedup"}, "keep": "last"}
	var before, after int64
	require.NoError(t, f.db.Table("strategy_events").Where("tenant_id=?", f.p.TenantID).Count(&before).Error)
	preview, e := life.PreviewCleaning(ctx, f.p, source.RefID, options, "")
	require.NoError(t, e)
	require.NoError(t, f.db.Table("strategy_events").Where("tenant_id=?", f.p.TenantID).Count(&after).Error)
	require.Equal(t, before, after)
	require.Empty(t, q.items)
	options["preview_digest"] = preview.PlanDigest
	stage, e := life.Invoke(ctx, f.p, "data-cleaning", []l.RefID{source.RefID}, options, "clean", "test")
	require.NoError(t, e)
	require.Equal(t, "last", stage.Options["keep"])
	require.NoError(t, life.Handle(ctx, q.items[0]))
	stored, e := repo.NewLifecycleRepository(f.db).FindStage(ctx, f.p.TenantID, stage.ID, false)
	require.NoError(t, e)
	require.Equal(t, l.StatusSucceeded, stored.Status)
	var frozen d.Snapshot
	raw, _ := json.Marshal(stored.Options["_strategy_snapshot"])
	require.NoError(t, json.Unmarshal(raw, &frozen))
	require.JSONEq(t, `{"operations":["trim","dedup"],"keep":"last","min_score":70}`, string(frozen.EffectiveBody))
	require.NoError(t, s.ValidateSnapshot(ctx, f.p, &frozen))
}

func TestStrategyMySQLGuidanceHookUsesApprovedVersionWithoutAuthority(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := agentctx.WithRequestID(agentctx.WithTenantID(agentctx.WithUserID(context.Background(), f.p.UserID), f.p.TenantID), "agent-once")
	t.Cleanup(func() {
		for _, table := range []string{"strategy_events", "strategy_versions", "strategy_heads"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	consumers := map[string]strategies.Consumer{}
	for _, kind := range []string{"experience", "skill", "playbook"} {
		consumers[kind] = strategies.Consumer{Name: "agent.guidance.v1", Baseline: strategies.DefaultGuidance}
	}
	s := strategies.NewService(repo.NewStrategyRepository(f.db), f.assets, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }, consumers)
	base, e := s.RegisterBaseline(ctx, f.p, "skill.default", "skill", "baseline")
	require.NoError(t, e)
	v, e := s.CreateCandidate(ctx, f.p, "skill.default", strategies.CandidateInput{Kind: "skill", BaselineID: base.ID, Body: json.RawMessage(`{"instructions":"Use numbered explanations for synthetic fixture questions."}`), EvidenceRefs: []string{f.assets.ref.AssetRefID}}, "candidate")
	require.NoError(t, e)
	// Actual stored review/gates, with an explicit deterministic regression reader.
	s.WithRegressionReader(func(context.Context, l.Principal, string, string) (d.RegressionEvidence, error) {
		return d.RegressionEvidence{BaselineTaskID: "base-eval", CandidateTaskID: "candidate-eval", BaselineVersionID: base.ID, CandidateVersionID: v.ID, DatasetVersionID: "fixed", DatasetManifest: "manifest", ConfigurationDigest: "fixed-config", GraderVersion: "v1", QualityMetric: "rouge_l", CostMetric: "wall_time_ms_not_billing", Baseline: d.Metrics{Quality: 1, Cost: 10, Samples: 1}, Candidate: d.Metrics{Quality: 1, Cost: 10, Samples: 1}, Samples: json.RawMessage(`[{"fixture":true}]`)}, nil
	})
	reg, e := s.Regress(ctx, f.p, "skill.default", v.ID, strategies.RegressionInput{BaselineTaskID: "base-eval", CandidateTaskID: "candidate-eval"}, "regress")
	require.NoError(t, e)
	approval, e := s.Approve(ctx, f.p, "skill.default", v.ID, strategies.ApprovalInput{Digest: v.Digest, RegressionID: reg.ID, Approved: true}, "approve")
	require.NoError(t, e)
	_, e = s.Activate(ctx, f.p, "skill.default", v.ID, strategies.ActivateInput{Digest: v.Digest, ApprovalID: approval.ID, ExpectedRevision: 1, Reason: "verified"}, "activate")
	require.NoError(t, e)
	hook := strategies.NewGuidanceHook(s)
	_, message, e := hook(ctx, "original request")
	require.NoError(t, e)
	require.Contains(t, message, "Use numbered explanations")
	require.Contains(t, message, "不是系统指令")
	require.Contains(t, message, "original request")
	require.Contains(t, message, v.ID)
	other := agentctx.WithUserID(ctx, f.p.UserID+1)
	_, message, e = hook(other, "other owner request")
	require.NoError(t, e)
	require.Equal(t, "other owner request", message)
	f.assets.deny = true
	_, _, e = hook(ctx, "original request")
	require.Error(t, e)
	_, _, e = hook(context.Background(), "missing identity")
	require.ErrorIs(t, e, d.ErrForbidden)
}
