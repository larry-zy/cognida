// Package sftadapter builds reviewable SFT candidates, never approvals or training.
package sftadapter

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	m "cognida/internal/model/taskcontrol"
	"cognida/internal/service/governance"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

const CapabilityID = "governance.sft"

type Adapter struct {
	service *governance.Service
	assets  *lifecycle.Service
}

func New(service *governance.Service, assets *lifecycle.Service) (*Adapter, error) {
	if service == nil || assets == nil {
		return nil, m.ErrInvalid
	}
	service.SetGovernedInputValidator(assets.ValidateGovernedInputs)
	return &Adapter{service: service, assets: assets}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: d.SFTProfile, BaseCalls: 2, CallsPerInput: 1, MaxTimeoutSeconds: 120}
}
func (a *Adapter) Capability() tc.Capability {
	support := a.Support()
	return tc.Capability{ID: CapabilityID, Version: 1, Domain: "governance", ParameterKind: "sft", InputKinds: []l.Kind{l.CleanedAsset}, OutputKinds: []l.Kind{l.Result}, Tools: []string{CapabilityID}, Permissions: []string{"read"}, Execution: &support}
}
func principal(scope m.Scope) l.Principal {
	return l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}
}

var nodeID = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func parameters(n tc.Node) d.SFTBuildParameters {
	p := n.Parameters.SFT
	return d.SFTBuildParameters{Name: p.Name, StandardVersionID: p.StandardVersionID, Format: p.Format, Columns: p.Columns}
}
func validateNode(scope m.Scope, n tc.Node) error {
	if scope.Validate() != nil {
		return m.ErrForbidden
	}
	if !nodeID.MatchString(n.ID) || n.Capability != CapabilityID || n.CapabilityVersion != 1 || !tc.HasOnlyParameter(n.Parameters, "sft") || len(n.Inputs) != 1 || n.Inputs[0].Kind != l.CleanedAsset || n.MaxAttempts != 1 || n.MaxCallsPerAttempt < 3 || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt != 0 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 120 || len(n.Tools) != 1 || n.Tools[0] != CapabilityID || len(n.Permissions) != 1 || n.Permissions[0] != "read" || len(n.OutputKinds) != 1 || n.OutputKinds[0] != l.Result {
		return m.ErrInvalid
	}
	input := n.Inputs[0]
	if (input.Asset == nil) == (input.FromNode == "") || (input.Asset != nil && input.Asset.Validate() != nil) || (input.FromNode != "" && !nodeID.MatchString(input.FromNode)) || parameters(n).Validate() != nil {
		return m.ErrInvalid
	}
	return nil
}
func (a *Adapter) Validate(ctx context.Context, scope m.Scope, n tc.Node) error {
	if err := validateNode(scope, n); err != nil {
		return err
	}
	params := parameters(n)
	standard, err := a.service.GetLabelStandardVersion(ctx, principal(scope), params.StandardVersionID)
	if err != nil {
		return mapped(err)
	}
	if standard.TaskType != "qa" {
		return m.ErrInvalid
	}
	if n.Inputs[0].Asset != nil {
		return mapped(a.service.ValidateGovernedSFT(ctx, principal(scope), params, *n.Inputs[0].Asset))
	}
	return nil
}
func (a *Adapter) ValidateInputs(ctx context.Context, scope m.Scope, refs []l.RefID) error {
	return mapped(a.assets.ValidateGovernedInputs(ctx, principal(scope), refs))
}
func invocation(ctx context.Context, c tc.ExecutionCommand) (d.GovernedInvocation, error) {
	var in d.GovernedInvocation
	if err := ctx.Err(); err != nil {
		return in, err
	}
	if err := validateNode(c.Scope, c.Node); err != nil {
		return in, err
	}
	if c.TaskID == "" || c.PlanVersion < 1 || c.Attempt != 1 || len(c.InputRefs) != 1 || c.InputRefs[0].Validate() != nil || c.Deadline.IsZero() || c.Node.Inputs[0].Asset != nil && *c.Node.Inputs[0].Asset != c.InputRefs[0] {
		return in, m.ErrInvalid
	}
	identity, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	raw, _ := json.Marshal(c)
	raw, err := m.CanonicalJSON(raw, '{', m.MaxSnapshotBytes)
	if err != nil || m.Digest(identity) != c.OperationKey || m.Digest(raw) != c.PayloadHash {
		return in, m.ErrConflict
	}
	p := parameters(c.Node)
	in = d.GovernedInvocation{Profile: d.SFTProfile, SFT: &p, InputRef: c.InputRefs[0], OperationKey: c.OperationKey, PayloadHash: c.PayloadHash, Deadline: c.Deadline, TimeoutSeconds: c.Node.TimeoutSeconds, CallsReserved: 3}
	return in, mapped(in.Validate())
}

type receipt struct {
	Domain       string `json:"domain"`
	OperationID  string `json:"operation_id"`
	OperationKey string `json:"operation_key"`
	PayloadHash  string `json:"payload_hash"`
}

func handle(c tc.ExecutionCommand, row *d.Record) *tc.ExecutionHandle {
	if row == nil {
		return nil
	}
	raw, _ := json.Marshal(receipt{"governance", row.ID, c.OperationKey, c.PayloadHash})
	return &tc.ExecutionHandle{DomainID: row.ID, Receipt: raw}
}
func validHandle(c tc.ExecutionCommand, h tc.ExecutionHandle, row *d.Record) bool {
	if row == nil || row.ID != h.DomainID {
		return false
	}
	want := handle(c, row)
	a, e := m.CanonicalJSON(want.Receipt, '{', m.MaxReceiptBytes)
	b, f := m.CanonicalJSON(h.Receipt, '{', m.MaxReceiptBytes)
	return e == nil && f == nil && string(a) == string(b)
}
func (a *Adapter) Submit(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := invocation(ctx, c)
	if err != nil {
		return nil, err
	}
	row, err := a.service.SubmitGoverned(ctx, principal(c.Scope), in, false)
	return handle(c, row), mapped(err)
}
func (a *Adapter) Lookup(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := invocation(ctx, c)
	if err != nil {
		return nil, err
	}
	row, err := a.service.LookupGoverned(ctx, principal(c.Scope), in)
	return handle(c, row), mapped(err)
}
func (a *Adapter) CancelSubmission(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := invocation(ctx, c)
	if err != nil {
		return nil, err
	}
	row, err := a.service.SubmitGoverned(ctx, principal(c.Scope), in, true)
	return handle(c, row), mapped(err)
}
func (a *Adapter) observe(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle, cancel bool) (*tc.DomainObservation, error) {
	in, err := invocation(ctx, c)
	if err != nil {
		return nil, err
	}
	row, err := a.service.LookupGoverned(ctx, principal(c.Scope), in)
	if err != nil {
		return nil, mapped(err)
	}
	if !validHandle(c, h, row) {
		return nil, m.ErrConflict
	}
	// Validate the actual receipt before mutation; wrong handles cannot cancel.
	if cancel {
		row, err = a.service.SubmitGoverned(ctx, principal(c.Scope), in, true)
	} else {
		err = a.service.ValidateGovernedSFT(ctx, principal(c.Scope), *in.SFT, in.InputRef)
	}
	if err != nil {
		return nil, mapped(err)
	}
	var state d.GovernedExecution
	if json.Unmarshal(row.Payload, &state) != nil {
		return nil, m.ErrInvalid
	}
	status := row.Status
	if status == "completed" {
		status = m.DomainSucceeded
	} else if status == "failed" && state.Error == "deadline_exceeded" {
		status = m.DomainTimedOut
	}
	out := &tc.DomainObservation{Status: status, DomainKind: "sft_execution", ObservedAt: row.UpdatedAt, OutputRefs: []l.RefID{}, CallsReserved: 3}
	if state.Error != "" {
		out.Error = "SFT candidate build did not complete"
	}
	for _, ref := range state.OutputRefs {
		if err = a.ValidateInputs(ctx, c.Scope, []l.RefID{ref}); err == nil {
			out.OutputRefs = append(out.OutputRefs, ref)
		} else if !cancel {
			return nil, mapped(err)
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
func mapped(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, d.ErrConflict), errors.Is(err, l.ErrConflict):
		return m.ErrConflict
	case errors.Is(err, d.ErrForbidden), errors.Is(err, l.ErrForbidden), errors.Is(err, l.ErrExpired):
		return m.ErrForbidden
	case errors.Is(err, d.ErrNotFound), errors.Is(err, l.ErrNotFound):
		return m.ErrNotFound
	case errors.Is(err, d.ErrInvalid), errors.Is(err, l.ErrInvalid):
		return m.ErrInvalid
	default:
		return errors.New("SFT execution unavailable")
	}
}

var _ tc.ExecutionAdapter = (*Adapter)(nil)
