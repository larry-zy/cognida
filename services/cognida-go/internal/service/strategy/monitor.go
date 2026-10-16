package strategy

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/strategy"
	"context"
	"encoding/json"
	"errors"
)

type Observation struct {
	TaskID         string               `json:"task_id"`
	Metrics        d.Metrics            `json:"metrics"`
	Reasons        []string             `json:"reasons"`
	Recommendation string               `json:"recommendation"`
	Evidence       d.RegressionEvidence `json:"evidence"`
}

// Observe consumes a genuine completed evaluation, not caller-provided scores.
// A warning is only a suggestion. It cannot disable or roll back a policy.
func (s *Service) Observe(ctx context.Context, p l.Principal, slot, id, taskID, key string) (out *d.Event, err error) {
	if err = s.auth(ctx, p, "read"); err != nil {
		return
	}
	if !d.ValidKey(key) || taskID == "" {
		return nil, d.ErrInvalid
	}
	version, e := s.Read(ctx, p, id)
	if e != nil {
		return nil, e
	}
	reader := s.regressionReader(version.Kind)
	if reader == nil {
		return nil, d.ErrUnavailable
	}
	proof, e := reader(ctx, p, taskID, taskID)
	if e != nil {
		return nil, e
	}
	if proof.CandidateVersionID != id {
		return nil, d.ErrConflict
	}
	hash := d.Digest([]string{id, taskID})
	err = s.Repo.InSlot(ctx, p, slot, "", func(tx d.Tx) error {
		v, e := tx.Version(ctx, id)
		if e != nil {
			return e
		}
		if e = s.check(ctx, p, v, true); e != nil {
			return e
		}
		previous, e := replay(ctx, tx, "observation", key, hash)
		if e != nil {
			return e
		}
		if previous != nil {
			out = previous
			return nil
		}
		reasons := []string{}
		if proof.Candidate.Quality < v.Gates.MinimumQuality {
			reasons = append(reasons, "minimum_quality")
		}
		if proof.Candidate.ErrorRate > v.Gates.MaximumErrorRate {
			reasons = append(reasons, "error_rate")
		}
		// Compare against the fixed paired regression used by the final review,
		// rather than treating the new run as its own cost/quality baseline.
		decision, decisionErr := tx.Event(ctx, "approval_decision", id)
		if decisionErr == nil {
			var pointer struct {
				ApprovalID string `json:"approval_id"`
			}
			if json.Unmarshal(decision.Payload, &pointer) != nil {
				return d.ErrConflict
			}
			approval, err := tx.EventByID(ctx, pointer.ApprovalID)
			if err != nil {
				return err
			}
			approved, err := decode[d.Approval](approval)
			if err != nil {
				return d.ErrConflict
			}
			regression, err := tx.EventByID(ctx, approved.RegressionID)
			if err != nil {
				return err
			}
			fixed, err := decode[d.Regression](regression)
			if err != nil {
				return d.ErrConflict
			}
			if fixed.Evidence.DatasetVersionID != proof.DatasetVersionID || fixed.Evidence.DatasetManifest != proof.DatasetManifest || fixed.Evidence.ConfigurationDigest != proof.ConfigurationDigest || fixed.Evidence.GraderVersion != proof.GraderVersion || fixed.Evidence.QualityMetric != proof.QualityMetric || fixed.Evidence.CostMetric != proof.CostMetric {
				return d.ErrConflict
			}
			_, reasons = d.Compare(v.Gates, fixed.Evidence.Baseline, proof.Candidate)
		} else if !errors.Is(decisionErr, d.ErrNotFound) {
			return decisionErr
		}
		recommendation := "continue_monitoring"
		if len(reasons) > 0 {
			recommendation = "review_disable_or_rollback"
		}
		out, e = event(ctx, tx, "observation", key, hash, id, Observation{TaskID: taskID, Metrics: proof.Candidate, Reasons: reasons, Recommendation: recommendation, Evidence: proof})
		return e
	})
	return
}

// ConsumerInfo exposes only supported, explicitly wired consumption kinds.
func (s *Service) ConsumerInfo() map[string]string {
	out := map[string]string{}
	for k, v := range s.consumers {
		out[k] = v.Name
	}
	return out
}
func SnapshotData(v *d.Snapshot) json.RawMessage { raw, _ := json.Marshal(v); return raw }
