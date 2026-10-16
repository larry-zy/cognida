// Package huggingfaceadapter binds the existing static HF importer to the
// durable task runtime. It has no network or alternate asset registration path.
package huggingfaceadapter

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	hf "cognida/internal/service/huggingface"
	tc "cognida/internal/service/taskcontrol"
)

type Adapter struct{ s *hf.Service }

var nodeID = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func New(s *hf.Service) (*Adapter, error) {
	if s == nil {
		return nil, m.ErrInvalid
	}
	return &Adapter{s: s}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: d.GovernedProfile, BaseCalls: d.GovernedCalls, MaxTimeoutSeconds: 120}
}
func (a *Adapter) Capability() tc.Capability {
	support := a.Support()
	return tc.Capability{ID: "huggingface.import", Version: 1, Domain: "huggingface", ParameterKind: "huggingface", AllowEmptyInputs: true, InputKinds: []l.Kind{}, OutputKinds: []l.Kind{l.RawAsset}, Tools: []string{"huggingface.import"}, Permissions: []string{"read"}, Execution: &support}
}
func principal(scope m.Scope) l.Principal {
	return l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}
}
func validate(n tc.Node) error {
	if !nodeID.MatchString(n.ID) || n.Capability != "huggingface.import" || n.CapabilityVersion != 1 || !tc.HasOnlyParameter(n.Parameters, "huggingface") || len(n.Inputs) != 0 || n.MaxAttempts != 1 || n.MaxCallsPerAttempt < d.GovernedCalls || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt != 0 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 120 || len(n.Tools) != 1 || n.Tools[0] != "huggingface.import" || len(n.Permissions) != 1 || n.Permissions[0] != "read" || len(n.OutputKinds) != 1 || n.OutputKinds[0] != l.RawAsset {
		return m.ErrInvalid
	}
	if n.Parameters.HuggingFace.Request.Validate() != nil {
		return m.ErrInvalid
	}
	return nil
}
func (a *Adapter) Validate(ctx context.Context, scope m.Scope, n tc.Node) error {
	if scope.Validate() != nil {
		return m.ErrForbidden
	}
	if e := validate(n); e != nil {
		return e
	}
	return mapped(a.s.ValidateGovernedRequest(ctx, principal(scope), n.Parameters.HuggingFace.Request))
}
func (a *Adapter) ValidateInputs(ctx context.Context, scope m.Scope, refs []l.RefID) error {
	if scope.Validate() != nil {
		return m.ErrForbidden
	}
	if len(refs) != 0 {
		return m.ErrInvalid
	}
	return mapped(a.s.ValidateGovernedPrincipal(ctx, principal(scope)))
}
func (a *Adapter) ValidateOutputs(ctx context.Context, scope m.Scope, refs []l.RefID) error {
	if scope.Validate() != nil {
		return m.ErrForbidden
	}
	return mapped(a.s.ValidateOutputs(ctx, principal(scope), refs))
}
func command(c tc.ExecutionCommand) (d.GovernedInvocation, error) {
	if c.Scope.Validate() != nil {
		return d.GovernedInvocation{}, m.ErrForbidden
	}
	if validate(c.Node) != nil || c.TaskID == "" || c.PlanVersion < 1 || c.Attempt != 1 || len(c.InputRefs) != 0 || c.Deadline.IsZero() {
		return d.GovernedInvocation{}, m.ErrInvalid
	}
	raw, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	if m.Digest(raw) != c.OperationKey {
		return d.GovernedInvocation{}, m.ErrConflict
	}
	raw, _ = json.Marshal(c)
	canonical, e := m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	if e != nil || m.Digest(canonical) != c.PayloadHash {
		return d.GovernedInvocation{}, m.ErrConflict
	}
	in := d.GovernedInvocation{OperationKey: c.OperationKey, PayloadHash: c.PayloadHash, TaskID: c.TaskID, PlanVersion: c.PlanVersion, NodeID: c.Node.ID, Request: c.Node.Parameters.HuggingFace.Request, Deadline: c.Deadline, TimeoutSeconds: c.Node.TimeoutSeconds, RequestID: c.RequestID}
	if in.Validate() != nil {
		return in, m.ErrInvalid
	}
	return in, nil
}

type receipt struct {
	Domain       string `json:"domain"`
	ID           string `json:"id"`
	OperationKey string `json:"operation_key"`
	PayloadHash  string `json:"payload_hash"`
}

func handle(c tc.ExecutionCommand, r *d.Import) *tc.ExecutionHandle {
	if r == nil {
		return nil
	}
	raw, _ := json.Marshal(receipt{"huggingface", r.ID, c.OperationKey, c.PayloadHash})
	return &tc.ExecutionHandle{DomainID: r.ID, Receipt: raw}
}
func (a *Adapter) Submit(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, e := command(c)
	if e != nil {
		return nil, e
	}
	r, e := a.s.SubmitGoverned(ctx, principal(c.Scope), in)
	return handle(c, r), mapped(e)
}
func (a *Adapter) Lookup(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, e := command(c)
	if e != nil {
		return nil, e
	}
	r, e := a.s.LookupGoverned(ctx, principal(c.Scope), in)
	return handle(c, r), mapped(e)
}
func (a *Adapter) CancelSubmission(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, e := command(c)
	if e != nil {
		return nil, e
	}
	r, e := a.s.CancelGoverned(ctx, principal(c.Scope), in)
	return handle(c, r), mapped(e)
}
func (a *Adapter) observe(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle, cancel bool) (*tc.DomainObservation, error) {
	in, e := command(c)
	if e != nil {
		return nil, e
	}
	r, e := a.s.LookupGoverned(ctx, principal(c.Scope), in)
	if e != nil {
		return nil, mapped(e)
	}
	if r == nil || r.ID != h.DomainID {
		return nil, m.ErrConflict
	}
	want, _ := m.CanonicalJSON(handle(c, r).Receipt, '{', m.MaxReceiptBytes)
	got, e := m.CanonicalJSON(h.Receipt, '{', m.MaxReceiptBytes)
	if e != nil || string(want) != string(got) {
		return nil, m.ErrConflict
	}
	if cancel {
		r, e = a.s.CancelGoverned(ctx, principal(c.Scope), in)
		if e != nil {
			return nil, mapped(e)
		}
	}
	refs, e := a.s.GovernedOutputRefs(ctx, principal(c.Scope), in, r)
	if e != nil {
		mappedErr := mapped(e)
		if !cancel || (!errors.Is(mappedErr, m.ErrForbidden) && !errors.Is(mappedErr, m.ErrNotFound)) {
			return nil, mappedErr
		}
		refs = []l.RefID{}
	}
	status := m.DomainRunning
	switch r.Status {
	case "queued":
		status = m.DomainQueued
	case "succeeded":
		status = m.DomainSucceeded
	case "failed":
		status = m.DomainFailed
	case "timed_out":
		status = m.DomainTimedOut
	case "cancelled":
		status = m.DomainCancelled
	case "running", "cancel_requested":
	default:
		return nil, m.ErrConflict
	}
	return &tc.DomainObservation{Status: status, OutputRefs: refs, Error: r.ErrorCode, ObservedAt: r.UpdatedAt, DomainKind: "huggingface_import", CallsReserved: d.GovernedCalls}, nil
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
	case errors.Is(e, context.Canceled):
		return context.Canceled
	case errors.Is(e, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(e, d.ErrInvalid), errors.Is(e, l.ErrInvalid):
		return m.ErrInvalid
	case errors.Is(e, d.ErrConflict), errors.Is(e, l.ErrConflict):
		return m.ErrConflict
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		return m.ErrForbidden
	case errors.Is(e, d.ErrNotFound), errors.Is(e, l.ErrNotFound):
		return m.ErrNotFound
	default:
		return errors.New("huggingface execution unavailable")
	}
}

var _ tc.ExecutionAdapter = (*Adapter)(nil)
