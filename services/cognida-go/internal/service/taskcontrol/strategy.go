package taskcontrol

import (
	l "cognida/internal/model/lifecycle"
	policy "cognida/internal/model/strategy"
	d "cognida/internal/model/taskcontrol"
	strategies "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"errors"
	"sort"
)

// OrchestrationPlanDigest includes every structural and budget field, but no task
// ID, owner, timestamp or plan revision. It can compare two fixed identical DAGs.
func OrchestrationPlanDigest(plan PlanSpec) string { return policy.Digest(plan) }

// OrchestrationTrialBinding is read-only. Starting the task remains a separate
// command with normal capability/approval/source/budget enforcement.
func (s *Service) OrchestrationTrialBinding(ctx context.Context, p l.Principal, id string) (string, int, error) {
	scope := d.Scope{TenantID: p.TenantID, UserID: p.UserID}
	task, e := s.Get(ctx, scope, id)
	if e != nil {
		return "", 0, strategyTaskError(e)
	}
	if task.StartedAt != nil || (task.Status != d.StatusDraft && task.Status != d.StatusReady) {
		return "", 0, policy.ErrConflict
	}
	record, e := s.GetPlan(ctx, scope, id, task.CurrentPlanVersion)
	if e != nil {
		return "", 0, strategyTaskError(e)
	}
	plan, e := decodeStored[PlanSpec](record.Snapshot)
	if e != nil {
		return "", 0, policy.ErrInvalid
	}
	if _, e = s.Validate(ctx, scope, plan); e != nil {
		return "", 0, strategyTaskError(e)
	}
	// Benchmark trials may only produce governed candidates. A candidate strategy
	// cannot become an indirect permission to publish or perform external writes.
	for _, node := range plan.Nodes {
		capability := s.validator.capabilities[node.Capability]
		if capability.RequiresApproval {
			return "", 0, policy.ErrForbidden
		}
	}
	return OrchestrationPlanDigest(plan), plan.Budget.MaxConcurrency, nil
}

type orchestrationEvidence struct {
	TaskID       string               `json:"task_id"`
	Status       d.Status             `json:"status"`
	Policy       *policy.Snapshot     `json:"policy"`
	PlanDigest   string               `json:"plan_digest"`
	InputDigests map[string]string    `json:"input_digests"`
	Metrics      policy.Metrics       `json:"metrics"`
	Children     []orchestrationChild `json:"children"`
}
type orchestrationChild struct {
	NodeID   string `json:"node_id"`
	Attempt  int    `json:"attempt"`
	DomainID string `json:"domain_id"`
	Status   string `json:"status"`
}

// OrchestrationRegressionReader consumes actual terminal parents and bound child
// observations. It neither starts work nor fabricates a policy pin for old runs.
func (s *Service) OrchestrationRegressionReader(policies *strategies.Service) strategies.RegressionReader {
	return func(ctx context.Context, p l.Principal, baselineID, candidateID string) (policy.RegressionEvidence, error) {
		var out policy.RegressionEvidence
		load := func(id string) (orchestrationEvidence, error) {
			var result orchestrationEvidence
			scope := d.Scope{TenantID: p.TenantID, UserID: p.UserID}
			task, e := s.Get(ctx, scope, id)
			if e != nil {
				return result, e
			}
			if !task.Status.Terminal() || task.StartedAt == nil || task.FinishedAt == nil || task.FinishedAt.Before(*task.StartedAt) {
				return result, policy.ErrConflict
			}
			record, e := s.GetPlan(ctx, scope, id, task.ExecutionPlanVersion)
			if e != nil {
				return result, e
			}
			if record.CreatedAt.After(*task.StartedAt) {
				return result, policy.ErrConflict
			} // Replanned trials are not comparable fixed DAGs.
			plan, e := decodeStored[PlanSpec](record.Snapshot)
			if e != nil {
				return result, policy.ErrConflict
			}
			if _, e = s.Validate(ctx, scope, plan); e != nil {
				return result, e
			}
			digest := OrchestrationPlanDigest(plan)
			pin, e := policies.ReadOrchestrationPin(ctx, p, id, digest)
			if e != nil {
				return result, e
			}
			refs, e := decodeStored[[]l.RefID](task.InputRefs)
			if e != nil {
				return result, policy.ErrConflict
			}
			inputs := map[string]string{}
			for _, ref := range refs {
				if policies.Assets == nil {
					return result, policy.ErrForbidden
				}
				asset, _, e := policies.Assets.Resolve(ctx, p.TenantID, p.UserID, ref)
				if e != nil {
					return result, policy.ErrForbidden
				}
				inputs[ref.String()] = asset.Provenance.Digest
			}
			children, e := s.repo.ListChildren(ctx, scope, id)
			if e != nil {
				return result, e
			}
			latest := latestChildren(children, task.ExecutionPlanVersion)
			m := policy.Metrics{Samples: len(plan.Nodes), Cost: float64(max(1, task.FinishedAt.Sub(*task.StartedAt).Milliseconds()))}
			if m.Samples < 1 {
				return result, policy.ErrConflict
			}
			details := []orchestrationChild{}
			for _, node := range plan.Nodes {
				child, ok := latest[node.ID]
				status := "not_dispatched"
				if ok {
					if !childTerminal(child) {
						return result, policy.ErrConflict
					}
					status = child.DomainStatus
					if status == d.DomainSucceeded {
						m.Quality++
					} else {
						m.ErrorRate++
					}
					details = append(details, orchestrationChild{node.ID, child.Attempt, child.DomainID, status})
				} else {
					m.ErrorRate++
					details = append(details, orchestrationChild{NodeID: node.ID, Status: status})
				}
			}
			m.Quality /= float64(m.Samples)
			m.ErrorRate /= float64(m.Samples)
			sort.Slice(details, func(i, j int) bool { return details[i].NodeID < details[j].NodeID })
			result = orchestrationEvidence{task.ID, task.Status, pin, digest, inputs, m, details}
			return result, nil
		}
		base, e := load(baselineID)
		if e != nil {
			return out, strategyTaskError(e)
		}
		candidate, e := load(candidateID)
		if e != nil {
			return out, strategyTaskError(e)
		}
		if base.PlanDigest != candidate.PlanDigest || policy.Digest(base.InputDigests) != policy.Digest(candidate.InputDigests) {
			return out, policy.ErrConflict
		}
		evidence, _ := json.Marshal(map[string]any{"baseline": base, "candidate": candidate})
		return policy.RegressionEvidence{BaselineTaskID: baselineID, CandidateTaskID: candidateID, BaselineVersionID: base.Policy.VersionID, CandidateVersionID: candidate.Policy.VersionID, DatasetVersionID: "task-plan:" + base.PlanDigest, DatasetManifest: policy.Digest(base.InputDigests), ConfigurationDigest: base.PlanDigest, GraderVersion: "taskcontrol-node-outcomes-v1", QualityMetric: "successful_node_fraction", CostMetric: "parent_wall_time_ms_not_billing", Baseline: base.Metrics, Candidate: candidate.Metrics, Samples: evidence}, nil
	}
}

func strategyTaskError(e error) error {
	switch {
	case errors.Is(e, d.ErrForbidden):
		return policy.ErrForbidden
	case errors.Is(e, d.ErrNotFound):
		return policy.ErrNotFound
	case errors.Is(e, d.ErrConflict):
		return policy.ErrConflict
	case errors.Is(e, d.ErrInvalid):
		return policy.ErrInvalid
	}
	return e
}
