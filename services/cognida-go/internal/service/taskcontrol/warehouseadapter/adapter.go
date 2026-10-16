package warehouseadapter

import (
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	d "cognida/internal/model/warehouse"
	tc "cognida/internal/service/taskcontrol"
	"cognida/internal/service/warehouse"
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Adapter struct{ s *warehouse.Service }

func New(s *warehouse.Service) (*Adapter, error) {
	if s == nil || s.Assets == nil || s.Repo == nil {
		return nil, m.ErrInvalid
	}
	return &Adapter{s}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: "warehouse.managed-v1", BaseCalls: 102, CallsPerInput: 1, MaxTimeoutSeconds: 120}
}
func (a *Adapter) Capability() tc.Capability {
	s := a.Support()
	return tc.Capability{ID: "warehouse.transform", Version: 1, Domain: "warehouse", ParameterKind: "warehouse", InputKinds: []l.Kind{l.RawAsset, l.CleanedAsset, l.EnrichedAsset, l.PublishedAsset, l.Result}, OutputKinds: []l.Kind{l.Result}, Tools: []string{"warehouse.transform"}, Permissions: []string{"read"}, Execution: &s}
}
func request(n tc.Node) (d.RunRequest, error) {
	if !tc.HasOnlyParameter(n.Parameters, "warehouse") {
		return d.RunRequest{}, m.ErrInvalid
	}
	p := n.Parameters.Warehouse
	r := d.RunRequest{ModelID: p.ModelID, ModelVersion: p.Version, Mode: p.Mode, FactSourceRef: p.FactSourceRef}
	for value, dest := range map[string]**time.Time{p.EventFrom: &r.EventFrom, p.EventTo: &r.EventTo} {
		if value != "" {
			at, e := time.Parse(time.RFC3339Nano, value)
			if e != nil {
				return r, m.ErrInvalid
			}
			*dest = &at
		}
	}
	if r.ModelID == "" || r.ModelVersion < 1 {
		return r, m.ErrInvalid
	}
	switch r.Mode {
	case "full":
		if r.EventFrom != nil || r.EventTo != nil {
			return r, m.ErrInvalid
		}
	case "incremental":
		if r.FactSourceRef == "" || r.EventFrom != nil || r.EventTo != nil {
			return r, m.ErrInvalid
		}
	case "backfill":
		if r.FactSourceRef == "" || r.EventFrom == nil || r.EventTo == nil || !r.EventTo.After(*r.EventFrom) {
			return r, m.ErrInvalid
		}
	default:
		return r, m.ErrInvalid
	}
	return r, nil
}
func (a *Adapter) validate(n tc.Node) error {
	if _, e := request(n); e != nil {
		return e
	}
	if n.Capability != "warehouse.transform" || n.CapabilityVersion != 1 || n.MaxAttempts != 1 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 120 || n.MaxCostMicrosPerAttempt != 0 || len(n.Inputs) < 1 || len(n.Inputs) > 9 || (n.MaxCallsPerAttempt < 102+len(n.Inputs) || n.MaxCallsPerAttempt > 128) || len(n.Tools) != 1 || n.Tools[0] != "warehouse.transform" || len(n.Permissions) != 1 || n.Permissions[0] != "read" || len(n.OutputKinds) != 1 || n.OutputKinds[0] != l.Result {
		return m.ErrInvalid
	}
	seen := map[string]bool{}
	for _, input := range n.Inputs {
		if input.Asset == nil || input.FromNode != "" || input.Asset.Validate() != nil || seen[input.Asset.String()] {
			return m.ErrInvalid
		}
		seen[input.Asset.String()] = true
	}
	return nil
}
func p(scope m.Scope) l.Principal { return l.Principal{TenantID: scope.TenantID, UserID: scope.UserID} }
func (a *Adapter) Validate(ctx context.Context, scope m.Scope, n tc.Node) error {
	if scope.Validate() != nil {
		return m.ErrForbidden
	}
	if e := a.validate(n); e != nil {
		return e
	}
	req, _ := request(n)
	model, e := a.s.GetModel(ctx, p(scope), req.ModelID, req.ModelVersion)
	if e != nil {
		return mapped(e)
	}
	expected := map[string]bool{}
	for _, node := range model.Definition.Nodes {
		if node.Kind == "source" {
			expected[node.SourceRef] = true
		}
	}
	if req.FactSourceRef != "" {
		expected[req.FactSourceRef] = true
	}
	if len(expected) != len(n.Inputs) {
		return m.ErrInvalid
	}
	for _, input := range n.Inputs {
		if !expected[input.Asset.String()] {
			return m.ErrInvalid
		}
		ref, _, e := a.s.Assets.Resolve(ctx, scope.TenantID, scope.UserID, *input.Asset)
		if e != nil {
			return mapped(e)
		}
		if ref.Kind != input.Kind {
			return m.ErrInvalid
		}
	}
	return mapped(a.s.Preflight(ctx, p(scope), req))
}
func (a *Adapter) ValidateInputs(ctx context.Context, scope m.Scope, refs []l.RefID) error {
	return mapped(a.s.Assets.ValidateGovernedInputs(ctx, p(scope), refs))
}
func (a *Adapter) command(c tc.ExecutionCommand) (d.RunRequest, error) {
	if c.Scope.Validate() != nil || c.TaskID == "" || c.PlanVersion < 1 || c.Attempt != 1 || c.Deadline.IsZero() || a.validate(c.Node) != nil || len(c.InputRefs) != len(c.Node.Inputs) {
		return d.RunRequest{}, m.ErrInvalid
	}
	for i, ref := range c.InputRefs {
		if ref != *c.Node.Inputs[i].Asset {
			return d.RunRequest{}, m.ErrInvalid
		}
	}
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	if m.Digest(raw) != c.OperationKey {
		return d.RunRequest{}, m.ErrConflict
	}
	raw, _ = json.Marshal(c)
	canonical, e := m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	if e != nil || m.Digest(canonical) != c.PayloadHash {
		return d.RunRequest{}, m.ErrConflict
	}
	return request(c.Node)
}
func handle(c tc.ExecutionCommand, r *d.Run) *tc.ExecutionHandle {
	if r == nil {
		return nil
	}
	raw, _ := json.Marshal(map[string]string{"domain": "warehouse", "id": r.ID, "operation_key": c.OperationKey, "payload_hash": c.PayloadHash})
	return &tc.ExecutionHandle{DomainID: r.ID, Receipt: raw}
}
func (a *Adapter) Submit(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	req, e := a.command(c)
	if e != nil {
		return nil, e
	}
	if e = a.Validate(ctx, c.Scope, c.Node); e != nil {
		return nil, e
	}
	ctx = agentctx.WithRequestID(ctx, c.RequestID)
	deadline := c.Deadline
	bounded := time.Now().Add(time.Duration(c.Node.TimeoutSeconds) * time.Second)
	if bounded.Before(deadline) {
		deadline = bounded
	}
	r, e := a.s.SubmitGoverned(ctx, p(c.Scope), req, c.OperationKey, c.PayloadHash, deadline)
	return handle(c, r), mapped(e)
}
func (a *Adapter) Lookup(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	if _, e := a.command(c); e != nil {
		return nil, e
	}
	r, e := a.s.LookupGoverned(ctx, p(c.Scope), c.OperationKey, c.PayloadHash)
	return handle(c, r), mapped(e)
}
func (a *Adapter) CancelSubmission(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	req, e := a.command(c)
	if e != nil {
		return nil, e
	}
	r, e := a.s.CancelGoverned(ctx, p(c.Scope), req, c.OperationKey, c.PayloadHash, c.Deadline)
	return handle(c, r), mapped(e)
}
func (a *Adapter) observe(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle, cancel bool) (*tc.DomainObservation, error) {
	req, e := a.command(c)
	if e != nil {
		return nil, e
	}
	r, e := a.s.LookupGoverned(ctx, p(c.Scope), c.OperationKey, c.PayloadHash)
	if e != nil {
		return nil, mapped(e)
	}
	if r == nil || r.ID != h.DomainID {
		return nil, m.ErrConflict
	}
	expected := handle(c, r)
	aJSON, e := m.CanonicalJSON(expected.Receipt, '{', m.MaxReceiptBytes)
	bJSON, f := m.CanonicalJSON(h.Receipt, '{', m.MaxReceiptBytes)
	if e != nil || f != nil || string(aJSON) != string(bJSON) {
		return nil, m.ErrConflict
	}
	if cancel {
		r, e = a.s.CancelGoverned(ctx, p(c.Scope), req, c.OperationKey, c.PayloadHash, c.Deadline)
		if e != nil {
			return nil, mapped(e)
		}
	} else if e = a.ValidateInputs(ctx, c.Scope, c.InputRefs); e != nil {
		return nil, e
	}
	status := m.DomainRunning
	refs := []l.RefID{}
	switch r.Status {
	case "awaiting_approval", "approved":
		status = m.DomainSucceeded
		ref, e := l.ParseRefID(r.OutputRef)
		if e != nil {
			return nil, m.ErrConflict
		}
		if e = a.ValidateInputs(ctx, c.Scope, []l.RefID{ref}); e != nil {
			if !cancel || (!errors.Is(e, m.ErrForbidden) && !errors.Is(e, m.ErrNotFound)) {
				return nil, e
			}
		} else {
			refs = append(refs, ref)
		}
	case "failed":
		status = m.DomainFailed
	case "cancelled":
		status = m.DomainCancelled
	}
	return &tc.DomainObservation{Status: status, OutputRefs: refs, ObservedAt: r.UpdatedAt, DomainKind: "warehouse_run", CallsReserved: c.Node.MaxCallsPerAttempt, Error: r.Error}, nil
}
func (a *Adapter) Inspect(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	return a.observe(ctx, c, h, false)
}
func (a *Adapter) Cancel(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	return a.observe(ctx, c, h, true)
}
func mapped(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, d.ErrInvalid), errors.Is(e, l.ErrInvalid):
		return m.ErrInvalid
	case errors.Is(e, d.ErrConflict), errors.Is(e, l.ErrConflict):
		return m.ErrConflict
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		return m.ErrForbidden
	case errors.Is(e, d.ErrNotFound), errors.Is(e, l.ErrNotFound):
		return m.ErrNotFound
	}
	return e
}

var _ tc.ExecutionAdapter = (*Adapter)(nil)
