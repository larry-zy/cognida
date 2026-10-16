// Package governanceadapter generates real, unapproved governance candidates.
package governanceadapter

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
	"strings"
	"time"
)

type Adapter struct {
	service             *governance.Service
	assets              *lifecycle.Service
	capability, profile string
}

func New(service *governance.Service, assets *lifecycle.Service, capability string) (*Adapter, error) {
	if service == nil || assets == nil {
		return nil, m.ErrInvalid
	}
	profile, ok := map[string]string{"governance.tagging": d.TaggingProfile, "governance.classification": d.ClassificationProfile, "governance.named_entity": d.EntityProfile, "governance.qa": d.QAProfile, "governance.feature-engineering": d.FeatureProfile}[capability]
	if !ok {
		return nil, m.ErrInvalid
	}
	service.SetGovernedInputValidator(assets.ValidateGovernedInputs)
	return &Adapter{service, assets, capability, profile}, nil
}
func (a *Adapter) Support() tc.ExecutionSupport {
	if a.profile == d.FeatureProfile {
		return tc.ExecutionSupport{Profile: a.profile, BaseCalls: 2, CallsPerInput: 1, MaxTimeoutSeconds: 300}
	}
	return tc.ExecutionSupport{Profile: a.profile, BaseCalls: 33, CallsPerInput: 1, MaxTimeoutSeconds: 120}
}
func (a *Adapter) Capability() tc.Capability {
	support := a.Support()
	kind := "governance"
	inputs := []l.Kind{l.CleanedAsset}
	if a.profile == d.FeatureProfile {
		kind = "feature"
		inputs = []l.Kind{l.RawAsset, l.CleanedAsset, l.EnrichedAsset, l.PublishedAsset, l.Result}
	}
	return tc.Capability{ID: a.capability, Version: 1, Domain: "governance", ParameterKind: kind, InputKinds: inputs, OutputKinds: []l.Kind{l.Result}, Tools: []string{a.capability}, Permissions: []string{"read"}, Execution: &support}
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

func (a *Adapter) validateNode(ctx context.Context, p m.Scope, n tc.Node) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if p.Validate() != nil {
		return m.ErrForbidden
	}
	support := a.Support()
	if !identifier.MatchString(n.ID) || n.Capability != a.capability || n.CapabilityVersion != 1 || len(n.Inputs) != 1 || len(n.Tools) != 1 || n.Tools[0] != a.capability || len(n.Permissions) != 1 || n.Permissions[0] != "read" || n.Parameters == nil || n.MaxAttempts < 1 || n.MaxAttempts > 5 || n.MaxCallsPerAttempt < support.BaseCalls+1 || n.MaxCallsPerAttempt > 128 || n.MaxCostMicrosPerAttempt < 0 || n.TimeoutSeconds < 1 || n.TimeoutSeconds > support.MaxTimeoutSeconds || len(n.OutputKinds) != 1 || n.OutputKinds[0] != l.Result {
		return m.ErrInvalid
	}
	input := n.Inputs[0]
	if (input.Asset == nil) == (input.FromNode == "") || (input.Asset != nil && input.Asset.Validate() != nil) || (input.FromNode != "" && !identifier.MatchString(input.FromNode)) {
		return m.ErrInvalid
	}
	switch input.Kind {
	case l.RawAsset, l.CleanedAsset, l.EnrichedAsset, l.PublishedAsset:
	case l.Result:
		if a.profile != d.FeatureProfile {
			return m.ErrInvalid
		}
	default:
		return m.ErrInvalid
	}
	probe := a.nodeInvocation(n, l.RefID{AssetID: "00000000-0000-4000-8000-000000000001", Version: 1})
	probe.OperationKey = strings.Repeat("a", 64)
	probe.PayloadHash = strings.Repeat("b", 64)
	probe.Deadline = time.Now().Add(time.Minute)
	if a.profile == d.FeatureProfile {
		if !tc.HasOnlyParameter(n.Parameters, "feature") || n.MaxAttempts != 1 {
			return m.ErrInvalid
		}
	} else {
		if !tc.HasOnlyParameter(n.Parameters, "governance") {
			return m.ErrInvalid
		}
	}
	return mapped(probe.Validate())
}
func (a *Adapter) Validate(ctx context.Context, p m.Scope, n tc.Node) error {
	if e := a.validateNode(ctx, p, n); e != nil {
		return e
	}
	if a.profile == d.FeatureProfile {
		params := n.Parameters.Feature
		source := ""
		if n.Inputs[0].Asset != nil {
			source = n.Inputs[0].Asset.String()
		}
		calls, embedding, err := a.service.FeatureExecutionRequirementsForSource(ctx, principal(p), params.FeatureID, params.Version, source)
		if err != nil {
			return mapped(err)
		}
		if embedding && n.Inputs[0].Asset == nil {
			calls = 34
		} // unknown upstream, reserve the governed 1000-row ceiling
		if n.MaxCallsPerAttempt < calls || (embedding && n.MaxCostMicrosPerAttempt != 0) {
			return m.ErrInvalid
		}
		if n.Inputs[0].Asset != nil {
			return mapped(a.service.ValidateGovernedFeature(ctx, principal(p), params.FeatureID, params.Version, *n.Inputs[0].Asset))
		}
		return nil
	}
	params := n.Parameters.Governance
	if params.StandardVersionID == "" || n.Inputs[0].Kind != l.CleanedAsset {
		return m.ErrInvalid
	}
	standard, err := a.service.GetLabelStandardVersion(ctx, principal(p), params.StandardVersionID)
	if err != nil {
		return mapped(err)
	}
	expected := a.nodeInvocation(n, l.RefID{}).AnnotationType()
	if a.profile == d.TaggingProfile {
		expected = "tagging"
	}
	if standard.TaskType != expected {
		return m.ErrInvalid
	}
	if n.Inputs[0].Asset == nil {
		return nil
	}
	in := a.nodeInvocation(n, *n.Inputs[0].Asset)
	if a.profile == d.TaggingProfile {
		_, err = a.service.BindTaggingStandard(ctx, principal(p), d.TaggingTask{SourceRefID: in.InputRef.String(), StandardVersionID: in.StandardVersionID, Config: d.TaggingConfig{LabelSchema: in.Schema()}})
	} else {
		_, err = a.service.BindAnnotationStandard(ctx, principal(p), d.AnnotationTask{SourceRefID: in.InputRef.String(), StandardVersionID: in.StandardVersionID, Type: in.AnnotationType(), Labels: in.Schema(), QAMapping: in.QAMapping})
	}
	return mapped(err)
}
func (a *Adapter) nodeInvocation(n tc.Node, ref l.RefID) d.GovernedInvocation {
	in := d.GovernedInvocation{Profile: a.profile, InputRef: ref, TimeoutSeconds: n.TimeoutSeconds, CallsReserved: a.Support().BaseCalls + 1}
	if n.Parameters != nil && n.Parameters.Feature != nil {
		in.FeatureID = n.Parameters.Feature.FeatureID
		in.FeatureVersion = n.Parameters.Feature.Version
		in.CallsReserved = n.MaxCallsPerAttempt
	}
	if n.Parameters != nil && n.Parameters.Governance != nil {
		params := n.Parameters.Governance
		in.StandardVersionID = params.StandardVersionID
		in.Labels = params.Labels
		in.LabelSchema = params.LabelSchema
		in.QAMapping = params.QAMapping
	}
	return in
}
func (a *Adapter) ValidateInputs(ctx context.Context, p m.Scope, refs []l.RefID) error {
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
	case errors.Is(e, d.ErrConflict), errors.Is(e, m.ErrConflict), errors.Is(e, l.ErrConflict):
		return m.ErrConflict
	case errors.Is(e, d.ErrForbidden), errors.Is(e, m.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, l.ErrExpired):
		return m.ErrForbidden
	case errors.Is(e, d.ErrInvalid), errors.Is(e, m.ErrInvalid), errors.Is(e, l.ErrInvalid):
		return m.ErrInvalid
	case errors.Is(e, d.ErrNotFound), errors.Is(e, m.ErrNotFound), errors.Is(e, l.ErrNotFound):
		return m.ErrNotFound
	default:
		return errors.New("governance execution unavailable")
	}
}
func (a *Adapter) invocation(ctx context.Context, c tc.ExecutionCommand) (d.GovernedInvocation, error) {
	var out d.GovernedInvocation
	if e := a.validateNode(ctx, c.Scope, c.Node); e != nil {
		return out, e
	}
	if c.TaskID == "" || c.PlanVersion < 1 || c.Attempt < 1 || c.Attempt > c.Node.MaxAttempts || len(c.InputRefs) != 1 || c.InputRefs[0].Validate() != nil || c.Deadline.IsZero() || (c.Node.Inputs[0].Asset != nil && *c.Node.Inputs[0].Asset != c.InputRefs[0]) {
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
	out = a.nodeInvocation(c.Node, c.InputRefs[0])
	out.OperationKey = c.OperationKey
	out.PayloadHash = c.PayloadHash
	out.Deadline = c.Deadline
	return out, nil
}

type receipt struct {
	Domain       string `json:"domain"`
	OperationID  string `json:"operation_id"`
	OperationKey string `json:"operation_key"`
	PayloadHash  string `json:"payload_hash"`
}

func handle(c tc.ExecutionCommand, r *d.Record) *tc.ExecutionHandle {
	if r == nil {
		return nil
	}
	raw, _ := json.Marshal(receipt{"governance", r.ID, c.OperationKey, c.PayloadHash})
	return &tc.ExecutionHandle{DomainID: r.ID, Receipt: raw}
}
func validateHandle(c tc.ExecutionCommand, h tc.ExecutionHandle) error {
	raw, _ := json.Marshal(receipt{"governance", h.DomainID, c.OperationKey, c.PayloadHash})
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
	ref, _, e := a.assets.Resolve(ctx, c.Scope.TenantID, c.Scope.UserID, in.InputRef)
	if e != nil {
		return nil, mapped(e)
	}
	if ref.Kind != c.Node.Inputs[0].Kind {
		return nil, m.ErrInvalid
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
		if e = a.ValidateInputs(ctx, c.Scope, c.InputRefs); e != nil {
			return nil, e
		}
	}
	var r *d.Record
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
	var state d.GovernedExecution
	if json.Unmarshal(r.Payload, &state) != nil {
		return nil, m.ErrInvalid
	}
	status := r.Status
	if status == "cancel_requested" {
		status = m.DomainRunning
	}
	switch status {
	case "completed":
		status = m.DomainSucceeded
	case "failed":
		status = m.DomainFailed
		if state.Error == "deadline_exceeded" {
			status = m.DomainTimedOut
		}
	}
	kind := "governance_execution"
	if in.Profile == d.FeatureProfile {
		kind = "feature_execution"
	}
	out := &tc.DomainObservation{Status: status, OutputRefs: []l.RefID{}, ObservedAt: r.UpdatedAt, DomainKind: kind, CallsReserved: in.CallsReserved}
	if state.Error != "" {
		out.Error = "governance candidate generation did not complete"
	}
	for _, ref := range state.OutputRefs {
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
