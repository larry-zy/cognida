// Package lifecycleadapter connects trusted task-control presets to the
// existing durable lifecycle controller. It never executes an agent prompt.
package lifecycleadapter

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"cognida/internal/service/lifecycle"
	tc "cognida/internal/service/taskcontrol"
)

type Adapter struct {
	service    *lifecycle.Service
	agent      string
	capability tc.Capability
	preset     lifecycle.GovernedPreset
}

var _ tc.ExecutionAdapter = (*Adapter)(nil)
var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func New(service *lifecycle.Service, capability string) (*Adapter, error) {
	if service == nil || !strings.HasPrefix(capability, "lifecycle.") {
		return nil, d.ErrInvalid
	}
	agent := strings.TrimPrefix(capability, "lifecycle.")
	preset, ok := lifecycle.GovernedPresetFor(agent)
	if !ok {
		return nil, d.ErrInvalid
	}
	for _, contract := range tc.LifecycleCapabilities(lifecycle.Contracts()) {
		if contract.ID == capability {
			return &Adapter{service, agent, contract, preset}, nil
		}
	}
	return nil, d.ErrInvalid
}

func (a *Adapter) Support() tc.ExecutionSupport {
	return tc.ExecutionSupport{Profile: a.preset.Profile, BaseCalls: a.preset.BaseCalls, CallsPerInput: 1, MaxTimeoutSeconds: a.preset.MaxTimeoutSeconds}
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] || (i > 0 && x[i] == x[i-1]) {
			return false
		}
	}
	return true
}
func hasKind(kinds []l.Kind, kind l.Kind) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

func (a *Adapter) Validate(ctx context.Context, scope d.Scope, node tc.Node) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := scope.Validate(); err != nil {
		return err
	}
	if !identifier.MatchString(node.ID) || node.Capability != a.capability.ID || node.CapabilityVersion != a.capability.Version || !equalSet(node.Tools, a.capability.Tools) || !equalSet(node.Permissions, a.capability.Permissions) || len(node.Inputs) < 1 || len(node.Inputs) > 16 || len(node.OutputKinds) < 1 || len(node.OutputKinds) > 16 || node.MaxAttempts < 1 || node.MaxAttempts > 5 || node.MaxCallsPerAttempt < len(node.Inputs)+a.preset.BaseCalls || node.MaxCallsPerAttempt > 128 || node.MaxCostMicrosPerAttempt < 0 || node.MaxCostMicrosPerAttempt > 1_000_000_000 || node.TimeoutSeconds < 1 || node.TimeoutSeconds > a.preset.MaxTimeoutSeconds {
		return d.ErrInvalid
	}
	data := len(a.capability.RequiredAnyInputKinds) == 0
	for _, input := range node.Inputs {
		if !hasKind(a.capability.InputKinds, input.Kind) || (input.Asset == nil) == (input.FromNode == "") || (input.Asset != nil && input.Asset.Validate() != nil) || (input.FromNode != "" && !identifier.MatchString(input.FromNode)) {
			return d.ErrInvalid
		}
		if hasKind(a.capability.RequiredAnyInputKinds, input.Kind) {
			data = true
		}
	}
	if !data {
		return d.ErrInvalid
	}
	seen := map[l.Kind]bool{}
	for _, kind := range node.OutputKinds {
		if !hasKind(a.capability.OutputKinds, kind) || seen[kind] {
			return d.ErrInvalid
		}
		if hasKind(a.capability.FirstInputPassthroughKinds, kind) {
			var first l.Kind
			for _, input := range node.Inputs {
				if hasKind(a.capability.FirstInputPassthroughKinds, input.Kind) {
					first = input.Kind
					break
				}
			}
			if first != kind {
				return d.ErrInvalid
			}
		}
		seen[kind] = true
	}
	return nil
}

func principal(scope d.Scope) l.Principal {
	return l.Principal{TenantID: scope.TenantID, UserID: scope.UserID}
}
func mapped(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, l.ErrForbidden), errors.Is(err, l.ErrExpired):
		return d.ErrForbidden
	case errors.Is(err, l.ErrNotFound):
		return d.ErrNotFound
	case errors.Is(err, l.ErrConflict):
		return d.ErrConflict
	case errors.Is(err, l.ErrInvalid):
		return d.ErrInvalid
	default:
		return errors.New("lifecycle execution service unavailable")
	}
}

func (a *Adapter) ValidateInputs(ctx context.Context, scope d.Scope, refs []l.RefID) error {
	return mapped(a.service.ValidateGovernedInputs(ctx, principal(scope), refs))
}

func (a *Adapter) invocation(ctx context.Context, cmd tc.ExecutionCommand) (lifecycle.GovernedInvocation, error) {
	var out lifecycle.GovernedInvocation
	if err := a.Validate(ctx, cmd.Scope, cmd.Node); err != nil {
		return out, err
	}
	if cmd.TaskID == "" || cmd.PlanVersion < 1 || cmd.Attempt < 1 || cmd.Attempt > cmd.Node.MaxAttempts || len(cmd.InputRefs) != len(cmd.Node.Inputs) || cmd.Deadline.IsZero() {
		return out, d.ErrInvalid
	}
	seen := map[string]bool{}
	for i, ref := range cmd.InputRefs {
		if ref.Validate() != nil || seen[ref.String()] || (cmd.Node.Inputs[i].Asset != nil && *cmd.Node.Inputs[i].Asset != ref) {
			return out, d.ErrInvalid
		}
		seen[ref.String()] = true
	}
	identity, _ := json.Marshal([]any{cmd.Scope.TenantID, cmd.Scope.UserID, cmd.TaskID, cmd.PlanVersion, cmd.Node.ID, cmd.Attempt})
	if d.Digest(identity) != cmd.OperationKey {
		return out, d.ErrConflict
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return out, d.ErrInvalid
	}
	raw, err = d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
	if err != nil || d.Digest(raw) != cmd.PayloadHash {
		return out, d.ErrConflict
	}
	out = lifecycle.GovernedInvocation{OperationKey: cmd.OperationKey, PayloadHash: cmd.PayloadHash, AgentID: a.agent, Profile: a.preset.Profile, InputRefs: cmd.InputRefs, TimeoutSeconds: cmd.Node.TimeoutSeconds, MaxCalls: cmd.Node.MaxCallsPerAttempt, CallsReserved: len(cmd.InputRefs) + a.preset.BaseCalls, Deadline: cmd.Deadline}
	return out, nil
}

type receipt struct {
	Domain       string `json:"domain"`
	StageID      string `json:"stage_id"`
	OperationKey string `json:"operation_key"`
	PayloadHash  string `json:"payload_hash"`
}

func handle(cmd tc.ExecutionCommand, stage *l.StageRun) *tc.ExecutionHandle {
	if stage == nil {
		return nil
	}
	raw, _ := json.Marshal(receipt{"lifecycle", stage.ID, cmd.OperationKey, cmd.PayloadHash})
	return &tc.ExecutionHandle{DomainID: stage.ID, Receipt: raw}
}
func validateHandle(cmd tc.ExecutionCommand, h tc.ExecutionHandle) error {
	if h.DomainID == "" {
		return d.ErrInvalid
	}
	want, _ := json.Marshal(receipt{"lifecycle", h.DomainID, cmd.OperationKey, cmd.PayloadHash})
	canonical, err := d.CanonicalJSON(h.Receipt, '{', d.MaxReceiptBytes)
	want, _ = d.CanonicalJSON(want, '{', d.MaxReceiptBytes)
	if err != nil || string(canonical) != string(want) {
		return d.ErrConflict
	}
	return nil
}

func (a *Adapter) Submit(ctx context.Context, cmd tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := a.invocation(ctx, cmd)
	if err != nil {
		return nil, err
	}
	for i, ref := range cmd.InputRefs {
		asset, _, err := a.service.Resolve(ctx, cmd.Scope.TenantID, cmd.Scope.UserID, ref)
		if err != nil {
			if errors.Is(err, l.ErrConflict) || errors.Is(err, l.ErrNotFound) || errors.Is(err, l.ErrExpired) {
				return nil, d.ErrForbidden
			}
			return nil, mapped(err)
		}
		if asset.Kind != cmd.Node.Inputs[i].Kind {
			return nil, d.ErrInvalid
		}
	}
	stage, err := a.service.InvokeGoverned(ctx, principal(cmd.Scope), in, cmd.RequestID)
	return handle(cmd, stage), mapped(err)
}
func (a *Adapter) Lookup(ctx context.Context, cmd tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := a.invocation(ctx, cmd)
	if err != nil {
		return nil, err
	}
	stage, err := a.service.LookupGoverned(ctx, principal(cmd.Scope), in)
	return handle(cmd, stage), mapped(err)
}
func (a *Adapter) CancelSubmission(ctx context.Context, cmd tc.ExecutionCommand) (*tc.ExecutionHandle, error) {
	in, err := a.invocation(ctx, cmd)
	if err != nil {
		return nil, err
	}
	stage, err := a.service.CancelGovernedSubmission(ctx, principal(cmd.Scope), in, cmd.RequestID)
	return handle(cmd, stage), mapped(err)
}

func observation(in lifecycle.GovernedInvocation, stage *l.StageRun) *tc.DomainObservation {
	out := &tc.DomainObservation{Status: stage.Status, OutputRefs: []l.RefID{}, ObservedAt: stage.UpdatedAt, DomainKind: "lifecycle", CallsReserved: in.CallsReserved}
	if lifecycle.GovernedTimedOut(stage) {
		out.Status = d.DomainTimedOut
	}
	if stage.Error != "" {
		out.Error = "lifecycle preset did not complete; inspect the authorized domain record"
	}
	if stage.Output != nil {
		for _, ref := range stage.Output.Refs {
			out.OutputRefs = append(out.OutputRefs, ref.RefID)
		}
	}
	return out
}
func (a *Adapter) Inspect(ctx context.Context, cmd tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	in, err := a.invocation(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err = validateHandle(cmd, h); err != nil {
		return nil, err
	}
	stage, err := a.service.InspectGoverned(ctx, principal(cmd.Scope), in, h.DomainID)
	if err != nil {
		return nil, mapped(err)
	}
	return observation(in, stage), nil
}
func (a *Adapter) Cancel(ctx context.Context, cmd tc.ExecutionCommand, h tc.ExecutionHandle) (*tc.DomainObservation, error) {
	in, err := a.invocation(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err = validateHandle(cmd, h); err != nil {
		return nil, err
	}
	stage, err := a.service.CancelGoverned(ctx, principal(cmd.Scope), in, h.DomainID)
	if err != nil {
		return nil, mapped(err)
	}
	return observation(in, stage), nil
}
