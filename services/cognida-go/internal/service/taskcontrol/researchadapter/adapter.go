// Package researchadapter connects a bounded real research engine to durable
// task dispatch without treating a generated plan or HTTP stream as completion.
package researchadapter

import (
	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	"cognida/internal/service/researchrun"
	tc "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"reflect"
)

const CapabilityID = "research.deep-research"

type Adapter struct{ s *researchrun.Service }

var _ tc.ExecutionAdapter = (*Adapter)(nil)

func Capability() tc.Capability {
	support := tc.ExecutionSupport{Profile: r.Profile, BaseCalls: r.MaxCalls, MaxTimeoutSeconds: r.MaxTimeoutSeconds}
	return tc.Capability{Execution: &support, ID: CapabilityID, Version: 1, Domain: "research", InputKinds: []l.Kind{}, OutputKinds: []l.Kind{l.Insight}, Tools: []string{"rag_query", "research_generate"}, Permissions: []string{"read"}, ParameterKind: "research", AllowEmptyInputs: true}
}
func New(s *researchrun.Service) (*Adapter, error) {
	if s == nil {
		return nil, d.ErrInvalid
	}
	return &Adapter{s}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: r.Profile, BaseCalls: r.MaxCalls, CallsPerInput: 0, MaxTimeoutSeconds: r.MaxTimeoutSeconds}
}
func request(node tc.Node) (r.Request, error) {
	cap := Capability()
	if node.Capability != cap.ID || node.CapabilityVersion != 1 || len(node.Inputs) != 0 || !reflect.DeepEqual(node.OutputKinds, cap.OutputKinds) || !same(node.Tools, cap.Tools) || !same(node.Permissions, cap.Permissions) || node.MaxAttempts != 1 || node.MaxCallsPerAttempt < r.MaxCalls || node.MaxCallsPerAttempt > 128 || node.MaxCostMicrosPerAttempt != 0 || node.TimeoutSeconds < 1 || node.TimeoutSeconds > r.MaxTimeoutSeconds || node.Parameters == nil || node.Parameters.Research == nil {
		return r.Request{}, d.ErrInvalid
	}
	// Validate the union by marshaling: unrelated domains cannot be smuggled in.
	raw, e := json.Marshal(node.Parameters)
	if e != nil {
		return r.Request{}, d.ErrInvalid
	}
	var parts map[string]json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 1 {
		return r.Request{}, d.ErrInvalid
	}
	p := node.Parameters.Research
	req := r.Request{Query: p.Query, KBIDs: append([]string(nil), p.KBIDs...), GraphEnabled: p.GraphEnabled, EnableWebSearch: p.EnableWebSearch}
	return req, req.Validate()
}
func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range a {
		if seen[x] {
			return false
		}
		seen[x] = true
	}
	for _, x := range b {
		if !seen[x] {
			return false
		}
	}
	return true
}
func (a *Adapter) Validate(ctx context.Context, p d.Scope, n tc.Node) error {
	req, err := request(n)
	if err != nil {
		return err
	}
	return a.s.Validate(ctx, p, req)
}
func (a *Adapter) ValidateInputs(ctx context.Context, p d.Scope, refs []l.RefID) error {
	return a.s.ValidateInputs(ctx, p, refs)
}
func command(c tc.ExecutionCommand) (r.Submission, error) {
	req, err := request(c.Node)
	if err != nil {
		return r.Submission{}, err
	}
	if c.Scope.Validate() != nil || c.Attempt != 1 || len(c.InputRefs) != 0 || !d.ValidText(c.TaskID, 36) || c.PlanVersion < 1 {
		return r.Submission{}, d.ErrInvalid
	}
	identity, _ := json.Marshal([]any{c.Scope.TenantID, c.Scope.UserID, c.TaskID, c.PlanVersion, c.Node.ID, c.Attempt})
	if d.Digest(identity) != c.OperationKey {
		return r.Submission{}, d.ErrConflict
	}
	body, _ := json.Marshal(c)
	body, e := d.CanonicalJSON(body, '{', d.MaxSnapshotBytes)
	if e != nil || d.Digest(body) != c.PayloadHash {
		return r.Submission{}, d.ErrConflict
	}
	in := r.Submission{RequestID: c.RequestID, OperationKey: c.OperationKey, PayloadHash: c.PayloadHash, Profile: r.Profile, Request: req, Deadline: c.Deadline, TimeoutSeconds: c.Node.TimeoutSeconds, MaxCalls: c.Node.MaxCallsPerAttempt, MaxCostMicros: c.Node.MaxCostMicrosPerAttempt}
	return in, in.Validate()
}
func handle(row *r.Run) *tc.ExecutionHandle {
	if row == nil {
		return nil
	}
	raw, _ := json.Marshal(struct{ ID, OperationKey, PayloadHash, Profile string }{row.ID, row.OperationKey, row.PayloadHash, r.Profile})
	return &tc.ExecutionHandle{DomainID: row.ID, Receipt: raw}
}
func (a *Adapter) bound(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (r.Submission, *r.Run, error) {
	in, err := command(c)
	if err != nil {
		return in, nil, err
	}
	row, err := a.s.Lookup(ctx, c.Scope, in)
	if err != nil {
		return in, nil, err
	}
	if row == nil {
		return in, nil, d.ErrNotFound
	}
	expected := handle(row)
	actual, e := d.CanonicalJSON(h.Receipt, '{', 4096)
	canonical, f := d.CanonicalJSON(expected.Receipt, '{', 4096)
	if h.DomainID != row.ID || e != nil || f != nil || string(actual) != string(canonical) {
		return in, nil, d.ErrConflict
	}
	return in, row, nil
}
func (a *Adapter) Submit(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := command(c)
	if err != nil {
		return nil, err
	}
	row, err := a.s.Submit(ctx, c.Scope, in)
	return handle(row), err
}
func (a *Adapter) Lookup(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := command(c)
	if err != nil {
		return nil, err
	}
	row, err := a.s.Lookup(ctx, c.Scope, in)
	return handle(row), err
}
func (a *Adapter) CancelSubmission(ctx context.Context, c tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := command(c)
	if err != nil {
		return nil, err
	}
	row, err := a.s.CancelSubmission(ctx, c.Scope, in)
	return handle(row), err
}
func observation(row *r.Run) *tc.DomainObservation {
	var refs []l.RefID
	_ = json.Unmarshal(row.OutputRefs, &refs)
	if refs == nil {
		refs = []l.RefID{}
	}
	status := row.Status
	if status == "partial" {
		status = d.DomainQuarantined
	}
	if status == "cancel_requested" {
		status = d.DomainRunning
	}
	return &tc.DomainObservation{Status: status, OutputRefs: refs, Error: row.Error, ObservedAt: row.UpdatedAt, DomainKind: "research", CallsReserved: r.MaxCalls}
}
func (a *Adapter) Inspect(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	_, row, err := a.bound(ctx, c, h)
	if err != nil {
		return nil, err
	}
	row, err = a.s.Read(ctx, c.Scope, row.ID)
	if err != nil {
		return nil, err
	}
	return observation(row), nil
}
func (a *Adapter) Cancel(ctx context.Context, c tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	_, _, err := a.bound(ctx, c, h)
	if err != nil {
		return nil, err
	}
	row, err := a.s.Cancel(ctx, c.Scope, h.DomainID)
	if err != nil {
		return nil, err
	}
	if _, e := a.s.Read(ctx, c.Scope, row.ID); e != nil {
		row.OutputRefs = json.RawMessage(`[]`)
	}
	return observation(row), nil
}
