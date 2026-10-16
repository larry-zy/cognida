//go:build integration

package handler

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/strategy"
	repo "cognida/internal/repository/mysql"
	lifecycle "cognida/internal/service/lifecycle"
	svc "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

// Genuine MySQL+HTTP, with an explicit trusted regression-reader fixture. Actual
// worker consumption is tested separately; this does not claim live-model E2E.
func TestStrategyMySQLHTTPVersionsGatesPinsAndRollback(t *testing.T) {
	f := newReviewedFixture(t)
	ctx := context.Background()
	t.Cleanup(func() {
		for _, table := range []string{"strategy_events", "strategy_versions", "strategy_heads"} {
			require.NoError(t, f.db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
	})
	s := svc.NewService(repo.NewStrategyRepository(f.db), f.assets, func(_ context.Context, p l.Principal, _ string) error { return p.Validate() }, map[string]svc.Consumer{"cleaning": {Name: "lifecycle.cleaning.v1", Baseline: lifecycle.DefaultCleaningStrategy}})
	NewStrategyHandler(s).Register(f.router.Group("/api"))
	path := "/strategies/cleaning.default"
	code, raw := f.request(t, "POST", path+"/baseline", "baseline", map[string]any{"kind": "cleaning"})
	require.Equal(t, 200, code)
	var base d.Version
	require.NoError(t, json.Unmarshal(raw, &base))
	pin, e := s.Pin(ctx, f.p, "cleaning.default", "existing-run")
	require.NoError(t, e)
	require.Equal(t, base.ID, pin.VersionID)
	in := svc.CandidateInput{Kind: "cleaning", BaselineID: base.ID, Body: json.RawMessage(`{"operations":["trim","normalize_ws"],"keep":"first","min_score":70}`), EvidenceRefs: []string{f.assets.ref.AssetRefID}}
	code, raw = f.request(t, "POST", path+"/versions", "candidate", in)
	require.Equal(t, 200, code)
	var candidate d.Version
	require.NoError(t, json.Unmarshal(raw, &candidate))
	require.Equal(t, 2, candidate.Number)
	code, replayed := f.request(t, "POST", path+"/versions", "candidate", in)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(raw), string(replayed))
	in.Body = json.RawMessage(`{"operations":["trim"],"keep":"first","min_score":70}`)
	code, _ = f.request(t, "POST", path+"/versions", "candidate", in)
	require.Equal(t, 409, code)
	head, e := s.Head(ctx, f.p, "cleaning.default")
	require.NoError(t, e)
	require.Equal(t, base.ID, head.ActiveID)
	for _, header := range []string{"X-Other-User", "X-Other-Tenant"} {
		code, _ = f.request(t, "GET", path+"/versions/"+candidate.ID, "", nil, header)
		require.Equal(t, 404, code)
	}
	proof := d.RegressionEvidence{BaselineTaskID: "baseline-task", CandidateTaskID: "candidate-task", BaselineVersionID: base.ID, CandidateVersionID: candidate.ID, DatasetVersionID: "fixed-version", DatasetManifest: "exact-manifest", ConfigurationDigest: "model-grader-config", GraderVersion: "v1", QualityMetric: "rouge_l", CostMetric: "wall_time_ms_not_billing", Baseline: d.Metrics{Quality: .9, Cost: 100, Samples: 1}, Candidate: d.Metrics{Quality: .1, Cost: 100, Samples: 1}, Samples: json.RawMessage(`[{"reference":"actual fixture"}]`)}
	proofDenied := false
	s.WithRegressionReader(func(_ context.Context, _ l.Principal, b, c string) (d.RegressionEvidence, error) {
		if proofDenied {
			return d.RegressionEvidence{}, d.ErrForbidden
		}
		if b == c {
			observed := proof
			observed.BaselineTaskID = b
			observed.CandidateTaskID = c
			observed.BaselineVersionID = candidate.ID
			observed.Candidate.Cost = 201
			return observed, nil
		}
		return proof, nil
	})
	target := path + "/versions/" + candidate.ID
	regInput := svc.RegressionInput{BaselineTaskID: "baseline-task", CandidateTaskID: "candidate-task"}
	code, raw = f.request(t, "POST", target+"/regressions", "bad-regression", regInput)
	require.Equal(t, 200, code)
	var reg d.Event
	require.NoError(t, json.Unmarshal(raw, &reg))
	code, _ = f.request(t, "POST", target+"/approvals", "bad-approval", svc.ApprovalInput{Digest: candidate.Digest, RegressionID: reg.ID, Approved: true})
	require.Equal(t, 409, code)
	proof.Candidate.Quality = .95
	code, raw = f.request(t, "POST", target+"/regressions", "good-regression", regInput)
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(raw, &reg))
	code, raw = f.request(t, "POST", target+"/approvals", "good-approval", svc.ApprovalInput{Digest: candidate.Digest, RegressionID: reg.ID, Approved: true})
	require.Equal(t, 200, code)
	var approval d.Event
	require.NoError(t, json.Unmarshal(raw, &approval))
	activate := svc.ActivateInput{Digest: candidate.Digest, ApprovalID: approval.ID, ExpectedRevision: head.Revision, Reason: "passed paired fixed regression"}
	code, raw = f.request(t, "POST", target+"/activate", "activate", activate)
	require.Equal(t, 200, code)
	code, replayed = f.request(t, "POST", target+"/activate", "activate", activate)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(raw), string(replayed))
	code, _ = f.request(t, "POST", target+"/approvals", "conflicting-review", svc.ApprovalInput{Digest: candidate.Digest, RegressionID: reg.ID, Approved: false})
	require.Equal(t, 409, code) // A final review is immutable; corrections require a new version.
	code, observed := f.request(t, "POST", target+"/observations", "observe", map[string]string{"task_id": "monitor-task"})
	require.Equal(t, 200, code)
	require.Contains(t, string(observed), "cost_regression")
	code, _ = f.request(t, "GET", path+"/versions?page_size=1", "", nil)
	require.Equal(t, 200, code)
	code, _ = f.request(t, "GET", path+"/events?page_size=1", "", nil)
	require.Equal(t, 200, code)
	proofDenied = true
	code, _ = f.request(t, "GET", path+"/events", "", nil)
	require.Equal(t, 403, code)
	proofDenied = false
	newPin, e := s.Pin(ctx, f.p, "cleaning.default", "new-run")
	require.NoError(t, e)
	require.Equal(t, candidate.ID, newPin.VersionID)
	oldPin, e := s.Pin(ctx, f.p, "cleaning.default", "existing-run")
	require.NoError(t, e)
	require.Equal(t, pin.VersionID, oldPin.VersionID)
	require.NotEqual(t, pin.CacheIdentity, newPin.CacheIdentity)
	head, e = s.Head(ctx, f.p, "cleaning.default")
	require.NoError(t, e)
	code, _ = f.request(t, "POST", path+"/versions/"+base.ID+"/activate", "rollback", svc.ActivateInput{Digest: base.Digest, ExpectedRevision: head.Revision, Rollback: true, Reason: "authorized return to baseline"})
	require.Equal(t, 200, code)
	rollbackPin, e := s.Pin(ctx, f.p, "cleaning.default", "after-rollback")
	require.NoError(t, e)
	require.Equal(t, base.ID, rollbackPin.VersionID)
	require.NoError(t, s.ValidateSnapshot(ctx, f.p, newPin))
	f.assets.deny = true
	require.Error(t, s.ValidateSnapshot(ctx, f.p, newPin))
	code, _ = f.request(t, "POST", target+"/activate", "activate", activate)
	require.Equal(t, 403, code)
	code, _ = f.request(t, "POST", target+"/disable", "disable", map[string]string{"reason": "source revoked"})
	require.Equal(t, 200, code)
	f.assets.deny = false
	require.ErrorIs(t, s.ValidateSnapshot(ctx, f.p, newPin), d.ErrConflict)
}
