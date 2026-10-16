// Package evaluationadapter binds a single immutable existing-answer table to
// the fixed deterministic offline graders. No model, prompt or code is accepted.
package evaluationadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"

	e "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"cognida/internal/service/evaluation"
	tc "cognida/internal/service/taskcontrol"
)

const CapabilityID = "evaluation.offline-text-v1"

type Adapter struct{ service *evaluation.OfflineService }

var _ tc.ExecutionAdapter = (*Adapter)(nil)
var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func New(service *evaluation.OfflineService) (*Adapter, error) {
	if service == nil {
		return nil, d.ErrInvalid
	}
	return &Adapter{service}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: e.OfflineProfile, BaseCalls: 5, CallsPerInput: 1, MaxTimeoutSeconds: 300}
}
func (a *Adapter) Capability() tc.Capability {
	support := a.Support()
	return tc.Capability{ID: CapabilityID, Version: 1, Domain: "evaluation", ParameterKind: "evaluation", InputKinds: []l.Kind{l.RawAsset, l.CleanedAsset, l.EnrichedAsset}, OutputKinds: []l.Kind{l.Result}, Tools: []string{"evaluation.offline-score"}, Permissions: []string{"read"}, Execution: &support}
}
func columns(p *tc.NodeParameters) (e.OfflineColumns, error) {
	if !tc.HasOnlyParameter(p, "evaluation") {
		return e.OfflineColumns{}, d.ErrInvalid
	}
	c := e.OfflineColumns{QuestionColumn: p.Evaluation.QuestionColumn, ReferenceAnswerColumn: p.Evaluation.ReferenceAnswerColumn, GeneratedAnswerColumn: p.Evaluation.GeneratedAnswerColumn}
	if c.Validate() != nil {
		return c, d.ErrInvalid
	}
	return c, nil
}
func (a *Adapter) Validate(ctx context.Context, p d.Scope, n tc.Node) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if !identifier.MatchString(n.ID) || n.Capability != CapabilityID || n.CapabilityVersion != 1 || len(n.Inputs) != 1 || len(n.OutputKinds) != 1 || n.OutputKinds[0] != l.Result || len(n.Tools) != 1 || n.Tools[0] != "evaluation.offline-score" || len(n.Permissions) != 1 || n.Permissions[0] != "read" || n.MaxAttempts != 1 || n.MaxCallsPerAttempt < 6 || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt != 0 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > 300 {
		return d.ErrInvalid
	}
	if _, err := columns(n.Parameters); err != nil {
		return err
	}
	input := n.Inputs[0]
	if input.Kind != l.RawAsset && input.Kind != l.CleanedAsset && input.Kind != l.EnrichedAsset {
		return d.ErrInvalid
	}
	if (input.Asset == nil) == (input.FromNode == "") || (input.Asset != nil && input.Asset.Validate() != nil) || (input.FromNode != "" && !identifier.MatchString(input.FromNode)) {
		return d.ErrInvalid
	}
	return nil
}
func scope(p d.Scope) e.OfflineScope { return e.OfflineScope{TenantID: p.TenantID, UserID: p.UserID} }
func mapped(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, e.ErrOfflineForbidden), errors.Is(err, l.ErrForbidden), errors.Is(err, l.ErrExpired):
		return d.ErrForbidden
	case errors.Is(err, e.ErrTaskNotFound), errors.Is(err, l.ErrNotFound):
		return d.ErrNotFound
	case errors.Is(err, e.ErrOfflineConflict), errors.Is(err, l.ErrConflict):
		return d.ErrConflict
	case errors.Is(err, e.ErrOfflineInvalid), errors.Is(err, l.ErrInvalid):
		return d.ErrInvalid
	default:
		return errors.New("offline evaluation service unavailable")
	}
}
func (a *Adapter) ValidateInputs(ctx context.Context, p d.Scope, refs []l.RefID) error {
	return mapped(a.service.ValidateInputs(ctx, scope(p), refs))
}
func (a *Adapter) request(ctx context.Context, cmd tc.ExecutionCommand) (e.OfflineRequest, error) {
	var out e.OfflineRequest
	if err := a.Validate(ctx, cmd.Scope, cmd.Node); err != nil {
		return out, err
	}
	if cmd.TaskID == "" || cmd.PlanVersion < 1 || cmd.Attempt < 1 || cmd.Attempt > cmd.Node.MaxAttempts || len(cmd.InputRefs) != 1 || cmd.InputRefs[0].Validate() != nil || cmd.Deadline.IsZero() || (cmd.Node.Inputs[0].Asset != nil && *cmd.Node.Inputs[0].Asset != cmd.InputRefs[0]) {
		return out, d.ErrInvalid
	}
	identity, _ := json.Marshal([]any{cmd.Scope.TenantID, cmd.Scope.UserID, cmd.TaskID, cmd.PlanVersion, cmd.Node.ID, cmd.Attempt})
	if d.Digest(identity) != cmd.OperationKey {
		return out, d.ErrConflict
	}
	body, err := json.Marshal(cmd)
	if err != nil {
		return out, d.ErrInvalid
	}
	body, err = d.CanonicalJSON(body, '{', d.MaxSnapshotBytes)
	if err != nil || d.Digest(body) != cmd.PayloadHash {
		return out, d.ErrConflict
	}
	c, _ := columns(cmd.Node.Parameters)
	out = e.OfflineRequest{OperationKey: cmd.OperationKey, PayloadHash: cmd.PayloadHash, Profile: e.OfflineProfile, InputRef: cmd.InputRefs[0], Columns: c, TimeoutSeconds: cmd.Node.TimeoutSeconds, Deadline: cmd.Deadline, RequestID: cmd.RequestID}
	return out, mapped(out.Validate())
}

type receipt struct {
	DomainID     string `json:"domain_id"`
	OperationKey string `json:"operation_key"`
	PayloadHash  string `json:"payload_hash"`
}

func handle(run *e.OfflineRun) *tc.ExecutionHandle {
	if run == nil {
		return nil
	}
	body, _ := json.Marshal(receipt{run.ID, run.Request.OperationKey, run.Request.PayloadHash})
	return &tc.ExecutionHandle{DomainID: run.ID, Receipt: body}
}
func checkHandle(cmd tc.ExecutionCommand, h tc.ExecutionHandle) error {
	if len(h.Receipt) > d.MaxReceiptBytes {
		return d.ErrConflict
	}
	decoder := json.NewDecoder(bytes.NewReader(h.Receipt))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return d.ErrConflict
	}
	keys := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || keys[name] {
			return d.ErrConflict
		}
		keys[name] = true
		var value string
		if decoder.Decode(&value) != nil {
			return d.ErrConflict
		}
	}
	var r receipt
	if json.Unmarshal(h.Receipt, &r) != nil || h.DomainID == "" || r.DomainID != h.DomainID || r.OperationKey != cmd.OperationKey || r.PayloadHash != cmd.PayloadHash {
		return d.ErrConflict
	}
	raw, _ := json.Marshal(r)
	canonical, e := d.CanonicalJSON(raw, '{', d.MaxReceiptBytes)
	actual, f := d.CanonicalJSON(h.Receipt, '{', d.MaxReceiptBytes)
	if e != nil || f != nil || string(canonical) != string(actual) {
		return d.ErrConflict
	}
	return nil
}
func (a *Adapter) Submit(ctx context.Context, cmd tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	r, err := a.request(ctx, cmd)
	if err != nil {
		return nil, err
	}
	run, err := a.service.Submit(ctx, scope(cmd.Scope), r)
	return handle(run), mapped(err)
}
func (a *Adapter) Lookup(ctx context.Context, cmd tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	r, err := a.request(ctx, cmd)
	if err != nil {
		return nil, err
	}
	run, err := a.service.Lookup(ctx, scope(cmd.Scope), r)
	return handle(run), mapped(err)
}
func (a *Adapter) CancelSubmission(ctx context.Context, cmd tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	r, err := a.request(ctx, cmd)
	if err != nil {
		return nil, err
	}
	run, err := a.service.CancelSubmission(ctx, scope(cmd.Scope), r)
	return handle(run), mapped(err)
}
func observation(run *e.OfflineRun) *tc.DomainObservation {
	if run == nil {
		return nil
	}
	out := &tc.DomainObservation{Status: run.Status, DomainKind: "evaluation", Error: run.Error, ObservedAt: run.UpdatedAt, CallsReserved: 6}
	if run.OutputRef != nil {
		out.OutputRefs = []l.RefID{*run.OutputRef}
	}
	return out
}
func (a *Adapter) Inspect(ctx context.Context, cmd tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	r, err := a.request(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err = checkHandle(cmd, h); err != nil {
		return nil, err
	}
	run, err := a.service.Inspect(ctx, scope(cmd.Scope), r, h.DomainID)
	return observation(run), mapped(err)
}
func (a *Adapter) Cancel(ctx context.Context, cmd tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	r, err := a.request(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err = checkHandle(cmd, h); err != nil {
		return nil, err
	}
	run, err := a.service.Cancel(ctx, scope(cmd.Scope), r, h.DomainID)
	return observation(run), mapped(err)
}
