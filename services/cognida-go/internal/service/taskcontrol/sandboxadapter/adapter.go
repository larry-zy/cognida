// Package sandboxadapter binds governed tasks to the existing durable Docker runner.
package sandboxadapter

import (
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	m "cognida/internal/model/taskcontrol"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/sandbox"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
)

type Adapter struct {
	runner *sandbox.DockerRunner
	assets *lifecycle.Service
}

func New(runner *sandbox.DockerRunner, assets *lifecycle.Service) (*Adapter, error) {
	if runner == nil || assets == nil {
		return nil, m.ErrInvalid
	}
	return &Adapter{runner, assets}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: "sandbox.controlled-development-v1", BaseCalls: 18, CallsPerInput: 1, MaxTimeoutSeconds: 60}
}
func (a *Adapter) Capability() tc.Capability {
	s := a.Support()
	return tc.Capability{ID: "sandbox.execute", Version: 1, Domain: "sandbox", ParameterKind: "sandbox", InputKinds: []l.Kind{l.RawAsset, l.CleanedAsset, l.EnrichedAsset, l.Result, l.PublishedAsset}, OutputKinds: []l.Kind{l.CleanedAsset}, Tools: []string{"sandbox.execute"}, Permissions: []string{"sandbox"}, Execution: &s}
}
func principal(scope m.Scope) l.Principal {
	return l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}
}
func request(n tc.Node, key string) (d.SubmitRequest, error) {
	if !tc.HasOnlyParameter(n.Parameters, "sandbox") {
		return d.SubmitRequest{}, m.ErrInvalid
	}
	p := n.Parameters.Sandbox
	if n.Capability != "sandbox.execute" || n.CapabilityVersion != 1 || n.MaxAttempts != 1 || n.MaxCostMicrosPerAttempt != 0 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 60 || p.Resources.TimeoutSeconds != int64(n.TimeoutSeconds) || p.Resources.OutputFiles < 1 || p.Resources.OutputFiles > 16 || len(n.Inputs) < 1 || len(n.Inputs) > 16 || n.MaxCallsPerAttempt < 18+len(n.Inputs) || n.MaxCallsPerAttempt > 64 || len(n.Tools) != 1 || n.Tools[0] != "sandbox.execute" || len(n.Permissions) != 1 || n.Permissions[0] != "sandbox" || len(n.OutputKinds) != 1 || n.OutputKinds[0] != l.CleanedAsset {
		return d.SubmitRequest{}, m.ErrInvalid
	}
	refs := append([]d.InputRef{p.Code}, p.Inputs...)
	if len(refs) != len(n.Inputs) {
		return d.SubmitRequest{}, m.ErrInvalid
	}
	seen := map[string]bool{}
	for i, input := range n.Inputs {
		if input.Asset == nil || input.FromNode != "" || input.Asset.Validate() != nil || *input.Asset != refs[i].Ref || seen[input.Asset.String()] {
			return d.SubmitRequest{}, m.ErrInvalid
		}
		seen[input.Asset.String()] = true
	}
	return d.SubmitRequest{SchemaVersion: d.SchemaVersion, IdempotencyKey: key, ProfileID: p.ProfileID, ProfileVersion: p.ProfileVersion, ImageSHA256: p.ImageSHA256, Code: p.Code, Inputs: p.Inputs, Resources: p.Resources}, nil
}
func (a *Adapter) Validate(ctx context.Context, scope m.Scope, n tc.Node) error {
	if scope.Validate() != nil {
		return m.ErrForbidden
	}
	req, e := request(n, "governed-preflight")
	if e != nil {
		return e
	}
	for _, input := range n.Inputs {
		ref, _, e := a.assets.Resolve(ctx, scope.TenantID, scope.UserID, *input.Asset)
		if e != nil {
			return mapped(e)
		}
		if ref.Kind != input.Kind {
			return m.ErrInvalid
		}
	}
	return mapped(a.runner.GovernedPreflight(ctx, principal(scope), req))
}
func (a *Adapter) ValidateInputs(ctx context.Context, scope m.Scope, refs []l.RefID) error {
	return mapped(a.assets.ValidateGovernedInputs(ctx, principal(scope), refs))
}
func (a *Adapter) command(c tc.ExecutionCommand) (d.SubmitRequest, error) {
	req, e := request(c.Node, c.OperationKey)
	if e != nil {
		return req, e
	}
	if c.Scope.Validate() != nil || c.TaskID == "" || c.PlanVersion < 1 || c.Attempt != 1 || c.Deadline.IsZero() || len(c.InputRefs) != len(c.Node.Inputs) {
		return req, m.ErrInvalid
	}
	for i, ref := range c.InputRefs {
		if ref != *c.Node.Inputs[i].Asset {
			return req, m.ErrInvalid
		}
	}
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	if m.Digest(raw) != c.OperationKey {
		return req, m.ErrConflict
	}
	raw, _ = json.Marshal(c)
	raw, e = m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	if e != nil || m.Digest(raw) != c.PayloadHash {
		return req, m.ErrConflict
	}
	return req, nil
}
func handle(c tc.ExecutionCommand, row *d.Record) *tc.ExecutionHandle {
	if row == nil {
		return nil
	}
	raw, _ := json.Marshal(map[string]string{"domain": "sandbox", "id": row.ID, "request_sha256": row.RequestSHA256, "operation_key": c.OperationKey, "payload_hash": c.PayloadHash})
	return &tc.ExecutionHandle{DomainID: row.ID, Receipt: raw}
}
func (a *Adapter) Submit(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	req, e := a.command(c)
	if e != nil {
		return nil, e
	}
	if prior, e := a.runner.LookupGoverned(ctx, principal(c.Scope), c.OperationKey, c.PayloadHash); e != nil || prior != nil {
		return handle(c, prior), mapped(e)
	}
	if e = a.Validate(ctx, c.Scope, c.Node); e != nil {
		return nil, e
	}
	ctx = agentctx.WithRequestID(ctx, c.RequestID)
	row, e := a.runner.SubmitGoverned(ctx, principal(c.Scope), req, c.PayloadHash, c.Deadline)
	return handle(c, row), mapped(e)
}
func (a *Adapter) Lookup(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	if _, e := a.command(c); e != nil {
		return nil, e
	}
	row, e := a.runner.LookupGoverned(ctx, principal(c.Scope), c.OperationKey, c.PayloadHash)
	return handle(c, row), mapped(e)
}
func (a *Adapter) CancelSubmission(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	req, e := a.command(c)
	if e != nil {
		return nil, e
	}
	ctx = agentctx.WithRequestID(ctx, c.RequestID)
	row, e := a.runner.CancelGoverned(ctx, principal(c.Scope), req, c.PayloadHash, c.Deadline)
	return handle(c, row), mapped(e)
}
func (a *Adapter) observe(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle, cancel bool) (*tc.DomainObservation, error) {
	req, e := a.command(c)
	if e != nil {
		return nil, e
	}
	row, e := a.runner.LookupGoverned(ctx, principal(c.Scope), c.OperationKey, c.PayloadHash)
	if e != nil {
		return nil, mapped(e)
	}
	if row == nil || row.ID != h.DomainID {
		return nil, m.ErrConflict
	}
	expected := handle(c, row)
	left, e := m.CanonicalJSON(expected.Receipt, '{', m.MaxReceiptBytes)
	right, f := m.CanonicalJSON(h.Receipt, '{', m.MaxReceiptBytes)
	if e != nil || f != nil || string(left) != string(right) {
		return nil, m.ErrConflict
	}
	if cancel {
		row, e = a.runner.CancelGoverned(ctx, principal(c.Scope), req, c.PayloadHash, c.Deadline)
	} else {
		row, e = a.runner.Get(ctx, principal(c.Scope), row.ID)
	}
	if e != nil {
		return nil, mapped(e)
	}
	status := domainStatus(row.State)
	refs := []l.RefID{}
	switch row.State {
	case d.StateSucceeded:
		if json.Unmarshal(row.OutputRefs, &refs) != nil {
			return nil, m.ErrConflict
		}
		if e = a.ValidateInputs(ctx, c.Scope, refs); e != nil {
			if !cancel || (!errors.Is(e, m.ErrForbidden) && !errors.Is(e, m.ErrNotFound)) {
				return nil, e
			}
			refs = []l.RefID{}
		}
	}
	return &tc.DomainObservation{Status: status, OutputRefs: refs, ObservedAt: row.UpdatedAt, DomainKind: "sandbox_run", CallsReserved: c.Node.MaxCallsPerAttempt, Error: row.ErrorCode}, nil
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
	case errors.Is(e, d.ErrInvalid), errors.Is(e, d.ErrUnsupported), errors.Is(e, l.ErrInvalid):
		return m.ErrInvalid
	case errors.Is(e, d.ErrConflict), errors.Is(e, l.ErrConflict):
		return m.ErrConflict
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		return m.ErrForbidden
	case errors.Is(e, l.ErrNotFound):
		return m.ErrNotFound
	}
	return e
}

var _ tc.ExecutionAdapter = (*Adapter)(nil)

// Cleanup/reconciliation is ongoing work, even when the script already exited.
func domainStatus(state d.State) string {
	switch state {
	case d.StateSucceeded:
		return m.DomainSucceeded
	case d.StateCancelled:
		return m.DomainCancelled
	case d.StateTimedOut:
		return m.DomainTimedOut
	case d.StateFailed:
		return m.DomainFailed
	default:
		return m.DomainRunning
	}
}
