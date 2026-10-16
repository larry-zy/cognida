// Package collectionadapter runs explicit single-URL snapshot collection.
package collectionadapter

import (
	d "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	"cognida/internal/service/collection"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

type Adapter struct {
	service *collection.Service
	assets  *lifecycle.Service
}

func New(service *collection.Service, assets *lifecycle.Service) (*Adapter, error) {
	if service == nil || assets == nil {
		return nil, m.ErrInvalid
	}
	service.SetGovernedAssets(assets)
	service.SetGovernedAuthorizer(assets.ValidateGovernedPrincipal)
	return &Adapter{service, assets}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: d.SnapshotProfile, BaseCalls: 3, MaxTimeoutSeconds: 120}
}
func (a *Adapter) Capability() tc.Capability {
	support := a.Support()
	return tc.Capability{ID: "collection.snapshot", Version: 1, Domain: "collection", ParameterKind: "collection", AllowEmptyInputs: true, InputKinds: []l.Kind{}, OutputKinds: []l.Kind{l.RawAsset, l.Result}, Tools: []string{"collection.snapshot"}, Permissions: []string{"read"}, Execution: &support}
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func (a *Adapter) Validate(ctx context.Context, p m.Scope, n tc.Node) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if p.Validate() != nil {
		return m.ErrForbidden
	}
	if !identifier.MatchString(n.ID) || n.Capability != "collection.snapshot" || n.CapabilityVersion != 1 || len(n.Inputs) != 0 || len(n.Tools) != 1 || n.Tools[0] != "collection.snapshot" || len(n.Permissions) != 1 || n.Permissions[0] != "read" || !tc.HasOnlyParameter(n.Parameters, "collection") || n.MaxAttempts != 1 || n.MaxCallsPerAttempt < 3 || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt < 0 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 120 {
		return m.ErrInvalid
	}
	if len(n.OutputKinds) != 2 || !((n.OutputKinds[0] == l.RawAsset && n.OutputKinds[1] == l.Result) || (n.OutputKinds[1] == l.RawAsset && n.OutputKinds[0] == l.Result)) {
		return m.ErrInvalid
	}
	params := n.Parameters.Collection
	if params.MaxResponseBytes < 0 || params.MaxResponseBytes > 2<<20 || (&d.Source{ID: "s", TaskID: "t", URL: params.URL}).Validate() != nil {
		return m.ErrInvalid
	}
	return nil
}
func (a *Adapter) ValidateInputs(ctx context.Context, p m.Scope, refs []l.RefID) error {
	if p.Validate() != nil {
		return m.ErrForbidden
	}
	if len(refs) == 0 {
		return mapped(a.assets.ValidateGovernedPrincipal(ctx, principal(p)))
	}
	return mapped(a.assets.ValidateGovernedInputs(ctx, principal(p), refs))
}
func principal(p m.Scope) l.Principal { return l.Principal{TenantID: p.TenantID, UserID: p.UserID} }
func mapped(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, context.Canceled):
		return context.Canceled
	case errors.Is(e, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(e, m.ErrConflict), errors.Is(e, l.ErrConflict):
		return m.ErrConflict
	case errors.Is(e, m.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		return m.ErrForbidden
	case errors.Is(e, m.ErrInvalid), errors.Is(e, l.ErrInvalid), errors.Is(e, d.ErrInvalidTask), errors.Is(e, d.ErrInvalidSource):
		return m.ErrInvalid
	case errors.Is(e, m.ErrNotFound), errors.Is(e, l.ErrNotFound):
		return m.ErrNotFound
	default:
		return errors.New("collection execution unavailable")
	}
}
func (a *Adapter) invocation(ctx context.Context, c tc.ExecutionCommand) (d.GovernedInvocation, error) {
	var out d.GovernedInvocation
	if e := a.Validate(ctx, c.Scope, c.Node); e != nil {
		return out, e
	}
	if c.TaskID == "" || c.PlanVersion < 1 || c.Attempt < 1 || c.Attempt > c.Node.MaxAttempts || len(c.InputRefs) != 0 || c.Deadline.IsZero() {
		return out, m.ErrInvalid
	}
	identity, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	if m.Digest(identity) != c.OperationKey {
		return out, m.ErrConflict
	}
	raw, _ := json.Marshal(c)
	raw, e := m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	if e != nil || m.Digest(raw) != c.PayloadHash {
		return out, m.ErrConflict
	}
	max := c.Node.Parameters.Collection.MaxResponseBytes
	if max == 0 {
		max = 2 << 20
	}
	return d.GovernedInvocation{OperationKey: c.OperationKey, PayloadHash: c.PayloadHash, Profile: d.SnapshotProfile, URL: c.Node.Parameters.Collection.URL, MaxResponseBytes: max, Deadline: c.Deadline, TimeoutSeconds: c.Node.TimeoutSeconds, CallsReserved: 3}, nil
}

type receipt struct {
	Domain       string `json:"domain"`
	RunID        string `json:"run_id"`
	OperationKey string `json:"operation_key"`
	PayloadHash  string `json:"payload_hash"`
}

func handle(c tc.ExecutionCommand, r *d.Run) *tc.ExecutionHandle {
	if r == nil {
		return nil
	}
	raw, _ := json.Marshal(receipt{"collection", r.ID, c.OperationKey, c.PayloadHash})
	return &tc.ExecutionHandle{DomainID: r.ID, Receipt: raw}
}
func validateHandle(c tc.ExecutionCommand, h tc.ExecutionHandle) error {
	raw, _ := json.Marshal(receipt{"collection", h.DomainID, c.OperationKey, c.PayloadHash})
	a, e := m.CanonicalJSON(raw, '{', m.MaxReceiptBytes)
	b, f := m.CanonicalJSON(h.Receipt, '{', m.MaxReceiptBytes)
	if h.DomainID == "" || e != nil || f != nil || string(a) != string(b) {
		return m.ErrConflict
	}
	return nil
}
func (a *Adapter) Submit(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, e := a.invocation(ctx, c)
	if e != nil {
		return nil, e
	}
	r, e := a.service.SubmitGoverned(ctx, principal(c.Scope), in, false)
	return handle(c, r), mapped(e)
}
func (a *Adapter) Lookup(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, e := a.invocation(ctx, c)
	if e != nil {
		return nil, e
	}
	r, e := a.service.LookupGoverned(ctx, principal(c.Scope), in)
	return handle(c, r), mapped(e)
}
func (a *Adapter) CancelSubmission(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, e := a.invocation(ctx, c)
	if e != nil {
		return nil, e
	}
	r, e := a.service.SubmitGoverned(ctx, principal(c.Scope), in, true)
	return handle(c, r), mapped(e)
}
func (a *Adapter) observe(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle, cancel bool) (*tc.DomainObservation, error) {
	in, e := a.invocation(ctx, c)
	if e != nil {
		return nil, e
	}
	if e = validateHandle(c, h); e != nil {
		return nil, e
	}
	if !cancel {
		if e := a.ValidateInputs(ctx, c.Scope, nil); e != nil {
			return nil, e
		}
	}
	var r *d.Run
	if cancel {
		r, e = a.service.SubmitGoverned(ctx, principal(c.Scope), in, true)
	} else {
		r, e = a.service.LookupGoverned(ctx, principal(c.Scope), in)
	}
	if e != nil {
		return nil, mapped(e)
	}
	if r == nil {
		return nil, m.ErrNotFound
	}
	if r.ID != h.DomainID {
		return nil, m.ErrConflict
	}
	status := m.DomainRunning
	switch r.Status {
	case d.RunStatusCompleted:
		status = m.DomainSucceeded
	case d.RunStatusFailed:
		status = m.DomainFailed
		if r.ErrorMessage == "deadline_exceeded" {
			status = m.DomainTimedOut
		}
	case d.RunStatusCancelled:
		status = m.DomainCancelled
	}
	out := &tc.DomainObservation{Status: status, OutputRefs: []l.RefID{}, ObservedAt: r.UpdatedAt, DomainKind: "collection", CallsReserved: 3}
	if r.ErrorMessage != "" {
		out.Error = "collection snapshot did not complete"
	}
	for _, ref := range r.OutputRefs {
		if a.ValidateInputs(ctx, c.Scope, []l.RefID{ref}) == nil {
			out.OutputRefs = append(out.OutputRefs, ref)
		} else if !cancel {
			return nil, m.ErrForbidden
		}
	}
	return out, nil
}
func (a *Adapter) Inspect(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	return a.observe(ctx, c, h, false)
}
func (a *Adapter) Cancel(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	return a.observe(ctx, c, h, true)
}

var _ tc.ExecutionAdapter = (*Adapter)(nil)
