package taskcontrol

import (
	"context"
	"encoding/json"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
)

type ReplanInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	Plan            PlanSpec `json:"plan"`
	Reason          string   `json:"reason"`
}

// Replan changes only unreserved future nodes. It does not execute a node,
// approve a capability, expand scope, reset elapsed time or erase domain truth.
func (r *Runtime) Replan(ctx context.Context, p d.Scope, id string, in ReplanInput, key string) (*PlanResult, error) {
	repo, ok := r.repo.(d.ReplanRepository)
	if !ok || !r.enabled {
		return nil, d.ErrInvalid
	}
	if in.ExpectedVersion < 1 || !d.ValidText(key, 200) || !d.ValidText(in.Reason, 2000) {
		return nil, d.ErrInvalid
	}
	task, e := r.tasks.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	validation, e := r.tasks.Validate(ctx, p, in.Plan)
	if e != nil {
		return nil, e
	}
	roots, e := decodeStored[[]l.RefID](task.InputRefs)
	if e != nil {
		return nil, e
	}
	allowed := map[string]bool{}
	for _, ref := range roots {
		allowed[ref.String()] = true
	}
	for _, ref := range validation.InputRefs {
		if !allowed[ref.String()] {
			return nil, d.ErrForbidden
		}
	}
	for _, node := range in.Plan.Nodes {
		adapter, exists := r.adapters[node.Capability]
		if !exists {
			return nil, d.ErrInvalid
		}
		if e = adapter.Validate(ctx, p, node); e != nil {
			return nil, e
		}
		if e = r.validateRootInputs(ctx, adapter, p, roots); e != nil {
			return nil, e
		}
	}
	raw, e := json.Marshal(in.Plan)
	if e != nil {
		return nil, e
	}
	hashRaw, _ := json.Marshal([]any{in.ExpectedVersion, in.Reason, in.Plan})
	hash := d.Digest(hashRaw)
	operation := d.Digest([]byte(key))
	prior, e := repo.LookupReplan(ctx, p, id, operation, hash)
	if e != nil {
		return nil, e
	}
	if prior != nil {
		return &PlanResult{Plan: prior, Validation: validation}, nil
	}
	if task.Version != in.ExpectedVersion || task.StartedAt == nil || task.CancelRequestedAt != nil || (task.Status != d.StatusRunning && task.Status != d.StatusReconcileRequired) {
		return nil, d.ErrConflict
	}
	old, e := r.plan(ctx, p, task)
	if e != nil {
		return nil, e
	}
	if old.Budget != in.Plan.Budget {
		return nil, &ValidationError{Issues: []Issue{{Field: "budget", Code: "replan_budget_fixed", Message: "running replan must preserve the approved cumulative budget and deadline"}}}
	}
	children, e := r.repo.ListChildren(ctx, p, id)
	if e != nil {
		return nil, e
	}
	for _, child := range children {
		before, ok := nodeByID(old, child.NodeID)
		after, kept := nodeByID(in.Plan, child.NodeID)
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(after)
		if !ok || !kept || string(a) != string(b) {
			return nil, &ValidationError{Issues: []Issue{{NodeID: child.NodeID, Field: "nodes", Code: "node_already_reserved", Message: "reserved, in-flight and completed nodes must remain unchanged"}}}
		}
	}
	if _, e = r.orchestrationLimit(ctx, p, id, in.Plan); e != nil {
		return nil, e
	}
	plan, e := repo.ReplanExecution(ctx, p, id, in.ExpectedVersion, d.ReplanCommand{Snapshot: raw, IdempotencyKey: operation, RequestHash: hash, Reason: in.Reason})
	if e != nil {
		return nil, e
	}
	return &PlanResult{Plan: plan, Validation: validation}, nil
}
