package evaluation

import (
	agentctx "cognida/internal/model/agent"
	d "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	policy "cognida/internal/model/strategy"
	lifecycle "cognida/internal/service/lifecycle"
	strategies "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
)

type preparationTimesKey struct{}

func (s *SnapshotService) WithStrategies(v *strategies.Service, compute lifecycle.Compute) *SnapshotService {
	s.Strategies = v
	s.StrategyCompute = compute
	return s
}
func validateStrategyProfile(s *policy.Snapshot, profile string) error {
	switch s.Kind {
	case "retrieval":
		if profile != "rag" {
			return policy.ErrInvalid
		}
	case "cleaning", "skill", "playbook", "experience":
		if profile != "qa" {
			return policy.ErrInvalid
		}
	default:
		return policy.ErrUnavailable
	}
	return nil
}
func (s *SnapshotService) prepareStrategy(ctx context.Context, p l.Principal, snap *d.EvaluationSnapshot, pairs []*d.QAPair) (context.Context, []*d.QAPair, error) {
	if snap == nil || snap.Strategy == nil {
		return ctx, pairs, nil
	}
	if s == nil || s.Strategies == nil {
		return ctx, nil, policy.ErrUnavailable
	}
	if e := s.Strategies.ValidateSnapshot(ctx, p, snap.Strategy); e != nil {
		return ctx, nil, e
	}
	ctx = strategies.WithSnapshot(ctx, snap.Strategy)
	if snap.Strategy.Kind != "cleaning" {
		return ctx, pairs, nil
	}
	if s.StrategyCompute == nil {
		return ctx, nil, policy.ErrUnavailable
	}
	var opts map[string]any
	if json.Unmarshal(snap.Strategy.EffectiveBody, &opts) != nil {
		return ctx, nil, policy.ErrInvalid
	}
	computeOptions, e := lifecycle.CleaningComputeOptions(opts)
	if e != nil {
		return ctx, nil, e
	}
	out := make([]*d.QAPair, 0, len(pairs))
	elapsed := make([]int64, 0, len(pairs))
	for _, pair := range pairs {
		if e := s.Strategies.ValidateSnapshot(ctx, p, snap.Strategy); e != nil {
			return ctx, nil, e
		}
		started := time.Now()
		result, e := s.StrategyCompute.Call(ctx, "clean", l.Input{Kind: "text", Text: pair.Question}, computeOptions, agentctx.MustGetRequestID(ctx))
		if e != nil {
			return ctx, nil, e
		}
		raw, _ := json.Marshal(result.Result["data"])
		var cleaned l.Input
		if json.Unmarshal(raw, &cleaned) != nil || cleaned.Kind != "text" || cleaned.Validate() != nil {
			return ctx, nil, policy.ErrInvalid
		}
		threshold := 70.0
		if value, ok := opts["min_score"].(float64); ok {
			threshold = value
		}
		profile, e := s.StrategyCompute.Call(ctx, "profile", cleaned, map[string]any{}, agentctx.MustGetRequestID(ctx))
		if e != nil {
			return ctx, nil, e
		}
		score, ok := profile.Result["overall_score"].(float64)
		if number, numberOK := profile.Result["overall_score"].(json.Number); numberOK {
			score, e = number.Float64()
			ok = e == nil
		}
		if !ok || math.IsNaN(score) || score < threshold {
			return ctx, nil, policy.ErrGate
		}
		copy := *pair
		copy.Question = cleaned.Text
		out = append(out, &copy)
		elapsed = append(elapsed, max(1, time.Since(started).Milliseconds()))
	}
	return context.WithValue(ctx, preparationTimesKey{}, elapsed), out, nil
}

// StrategyRegression reads completed immutable evaluations. HTTP callers cannot
// supply metrics or claim a policy was consumed. Both runs must carry worker receipts.
func (s *Service) StrategyRegression(ctx context.Context, p l.Principal, baselineID, candidateID string) (policy.RegressionEvidence, error) {
	var out policy.RegressionEvidence
	load := func(id string) (*d.EvaluationSnapshot, []*d.EvaluationResult, error) {
		task, e := s.taskRepo.FindByID(ctx, id)
		if e != nil {
			return nil, nil, e
		}
		if task.TenantID != p.TenantID || task.UserID != p.UserID || task.DeletedAt != nil {
			return nil, nil, policy.ErrForbidden
		}
		if task.Status != d.TaskStatusCompleted {
			return nil, nil, policy.ErrConflict
		}
		if e = s.authorizeTaskRead(ctx, task); e != nil {
			return nil, nil, e
		}
		var snapshot d.EvaluationSnapshot
		if json.Unmarshal(task.Snapshot, &snapshot) != nil || snapshot.Strategy == nil || s.snapshots == nil || s.snapshots.Strategies == nil {
			return nil, nil, policy.ErrConflict
		}
		if e = s.snapshots.Strategies.ValidateSnapshot(ctx, p, snapshot.Strategy); e != nil {
			return nil, nil, e
		}
		rows, e := s.resultRepo.FindByTaskID(ctx, id)
		if e != nil {
			return nil, nil, e
		}
		if len(rows) != len(snapshot.QAPairs) || len(rows) == 0 {
			return nil, nil, policy.ErrConflict
		}
		for _, row := range rows {
			if row.Scores["strategy_applied"] != 1 || row.Scores["latency_ms"] <= 0 {
				return nil, nil, policy.ErrConflict
			}
		}
		return &snapshot, rows, nil
	}
	ctx = agentctx.WithTenantID(agentctx.WithUserID(ctx, p.UserID), p.TenantID)
	base, bRows, e := load(baselineID)
	if e != nil {
		return out, e
	}
	candidate, cRows, e := load(candidateID)
	if e != nil {
		return out, e
	}
	if base.DatasetVersionID != candidate.DatasetVersionID || base.DatasetManifest != candidate.DatasetManifest || base.GraderVersion != candidate.GraderVersion || policy.Digest(base.Target) != policy.Digest(candidate.Target) || policy.Digest(base.Configuration) != policy.Digest(candidate.Configuration) || policy.Digest(base.QAPairs) != policy.Digest(candidate.QAPairs) || base.Strategy.Kind != candidate.Strategy.Kind {
		return out, policy.ErrConflict
	}
	metric := "rouge_l"
	if base.Strategy.Kind == "retrieval" {
		metric = "recall"
	}
	summarize := func(rows []*d.EvaluationResult) (policy.Metrics, error) {
		m := policy.Metrics{Samples: len(rows)}
		for _, r := range rows {
			score, ok := r.Scores[metric]
			if !ok {
				if metric == "recall" && r.Recall != nil {
					score = *r.Recall
					ok = true
				}
				if metric == "rouge_l" && r.ROUGEL != nil {
					score = *r.ROUGEL
					ok = true
				}
			}
			if !ok || math.IsNaN(score) || score < 0 || score > 1 {
				return m, policy.ErrConflict
			}
			m.Quality += score
			m.Cost += r.Scores["latency_ms"]
			if !r.Success {
				m.ErrorRate++
			}
		}
		m.Quality /= float64(m.Samples)
		m.ErrorRate /= float64(m.Samples)
		return m, nil
	}
	b, e := summarize(bRows)
	if e != nil {
		return out, e
	}
	c, e := summarize(cRows)
	if e != nil {
		return out, e
	}
	type sample struct {
		QuestionDigest      string `json:"question_digest"`
		Baseline, Candidate *d.EvaluationResult
	}
	samples := make([]sample, 0, len(bRows))
	usedBase, usedCandidate := map[int]bool{}, map[int]bool{}
	for _, pair := range base.QAPairs {
		find := func(rows []*d.EvaluationResult, used map[int]bool) *d.EvaluationResult {
			for i, r := range rows {
				if !used[i] && r.Question == pair.Question && r.ReferenceAnswer == pair.ReferenceAnswer {
					used[i] = true
					return r
				}
			}
			return nil
		}
		br, cr := find(bRows, usedBase), find(cRows, usedCandidate)
		if br == nil || cr == nil {
			return out, policy.ErrConflict
		}
		samples = append(samples, sample{policy.Digest(strings.TrimSpace(pair.Question)), br, cr})
	}
	raw, _ := json.Marshal(samples)
	return policy.RegressionEvidence{BaselineTaskID: baselineID, CandidateTaskID: candidateID, BaselineVersionID: base.Strategy.VersionID, CandidateVersionID: candidate.Strategy.VersionID, DatasetVersionID: base.DatasetVersionID, DatasetManifest: base.DatasetManifest, ConfigurationDigest: policy.Digest([]json.RawMessage{base.Configuration, base.Target}), GraderVersion: base.GraderVersion, QualityMetric: metric, CostMetric: "wall_time_ms_not_billing", Baseline: b, Candidate: c, Samples: raw}, nil
}

func evaluationInheritedPolicy(c *EvaluationTaskConfig, kind string) json.RawMessage {
	if kind == "cleaning" {
		return lifecycle.DefaultCleaningEvaluationOptions()
	}
	if c.Type == "rag" {
		topK := 5
		if v, ok := c.Config["top_k"].(float64); ok && v > 0 {
			topK = int(v)
		}
		raw, _ := json.Marshal(policy.RetrievalPolicy{TopK: topK, ScoreThreshold: 0, Rerank: false})
		return raw
	}
	return json.RawMessage(`{"passthrough":true}`)
}
