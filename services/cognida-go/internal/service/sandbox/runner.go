package sandbox

import (
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type InputLoader func(context.Context, l.Principal, l.RefID) ([]byte, error)
type CandidateWriter func(context.Context, l.Principal, string, []l.RefID, []byte) (l.RefID, error)
type ScopeAuthorizer func(context.Context, l.Principal) error

type DockerRunner struct {
	repo      d.Repository
	validator *Validator
	host      *DockerHost
	load      InputLoader
	write     CandidateWriter
	authorize ScopeAuthorizer
}

func NewDockerRunner(repo d.Repository, v *Validator, host *DockerHost, load InputLoader, write CandidateWriter, authorize ScopeAuthorizer) (*DockerRunner, error) {
	if repo == nil || v == nil || host == nil || host.Root == "" || load == nil || write == nil || authorize == nil {
		return nil, d.ErrUnavailable
	}
	return &DockerRunner{repo, v, host, load, write, authorize}, nil
}
func (v ValidatedSubmission) BindingPrincipal() l.Principal {
	return l.Principal{TenantID: v.Binding.TenantID, UserID: v.Binding.UserID}
}
func (r *DockerRunner) Metadata(ctx context.Context, p l.Principal, ref l.RefID) (d.InputMetadata, error) {
	if e := r.authorize(ctx, p); e != nil {
		return d.InputMetadata{}, e
	}
	b, e := r.load(ctx, p, ref)
	if e != nil {
		return d.InputMetadata{}, e
	}
	hash := sha256.Sum256(b)
	return d.InputMetadata{TenantID: p.TenantID, Ref: ref, SHA256: "sha256:" + hex.EncodeToString(hash[:]), SizeBytes: int64(len(b))}, nil
}
func (r *DockerRunner) Submit(ctx context.Context, p l.Principal, in d.SubmitRequest) (d.Execution, error) {
	if e := r.authorize(ctx, p); e != nil {
		return d.Execution{}, e
	}
	v, e := r.validator.ValidateSubmission(ctx, p, in)
	if e != nil {
		return d.Execution{}, e
	}
	for _, input := range v.Request.Inputs {
		if input.Name == "runner.py" {
			return d.Execution{}, d.ErrInvalid
		}
	}
	if e = r.host.Check(ctx, v.Profile.Image); e != nil {
		return d.Execution{}, e
	}
	raw, _ := json.Marshal(v)
	now := time.Now().UTC()
	requestID, _ := agentctx.GetRequestID(ctx)
	row, e := r.repo.Create(ctx, &d.Record{ID: v.Binding.ID, TenantID: p.TenantID, UserID: p.UserID, IdempotencyKey: in.IdempotencyKey, RequestSHA256: v.Binding.RequestSHA256, RequestID: requestID, Submission: raw, State: d.StateQueued, Version: 1, Deadline: now.Add(time.Duration(in.Resources.TimeoutSeconds) * time.Second), CreatedAt: now, UpdatedAt: now})
	if e != nil {
		return d.Execution{}, e
	}
	return execution(row), nil
}
func execution(row *d.Record) d.Execution {
	return d.Execution{Binding: row.Binding(), State: row.State, ErrorCode: row.ErrorCode, UpdatedAt: row.UpdatedAt}
}
func (r *DockerRunner) read(ctx context.Context, p l.Principal, id string, checkSources bool) (*d.Record, ValidatedSubmission, error) {
	var v ValidatedSubmission
	if e := r.authorize(ctx, p); e != nil {
		return nil, v, e
	}
	row, e := r.repo.Get(ctx, p, id)
	if e != nil {
		return nil, v, e
	}
	if json.Unmarshal(row.Submission, &v) != nil || ValidateReplay(p, row.Binding(), v.Binding) != nil {
		return nil, v, d.ErrConflict
	}
	if checkSources {
		current, e := r.validator.ValidateSubmission(ctx, p, v.Request)
		if e != nil {
			return nil, v, e
		}
		if ValidateReplay(p, row.Binding(), current.Binding) != nil {
			return nil, v, d.ErrConflict
		}
	}
	return row, v, nil
}
func (r *DockerRunner) Get(ctx context.Context, p l.Principal, id string) (*d.Record, error) {
	row, _, e := r.read(ctx, p, id, true)
	return row, e
}
func (r *DockerRunner) List(ctx context.Context, p l.Principal, cursor string) ([]d.Record, string, error) {
	if e := r.authorize(ctx, p); e != nil {
		return nil, "", e
	}
	rows, next, e := r.repo.List(ctx, p, cursor, 20)
	if e != nil {
		return nil, "", e
	}
	safe := []d.Record{}
	for _, row := range rows {
		if _, _, e := r.read(ctx, p, row.ID, true); e == nil {
			safe = append(safe, row)
		}
	}
	return safe, next, nil
}
func (r *DockerRunner) Status(ctx context.Context, p l.Principal, ref d.ExecutionRef) (d.Execution, error) {
	row, _, e := r.read(ctx, p, ref.ID, true)
	if e != nil {
		return d.Execution{}, e
	}
	if row.Binding().ExecutionRef != ref {
		return d.Execution{}, d.ErrForbidden
	}
	return execution(row), nil
}
func (r *DockerRunner) Cancel(ctx context.Context, p l.Principal, ref d.ExecutionRef) (d.Execution, error) {
	// Revoking a source cannot revoke the owner's ability to stop resource use.
	row, _, e := r.read(ctx, p, ref.ID, false)
	if e != nil {
		return d.Execution{}, e
	}
	if row.Binding().ExecutionRef != ref {
		return d.Execution{}, d.ErrForbidden
	}
	row, e = r.repo.RequestCancel(ctx, p, ref.ID)
	if e != nil {
		return d.Execution{}, e
	}
	return execution(row), nil
}
func (r *DockerRunner) Collect(ctx context.Context, p l.Principal, ref d.ExecutionRef) (d.Manifest, error) {
	row, _, e := r.read(ctx, p, ref.ID, true)
	if e != nil {
		return d.Manifest{}, e
	}
	if row.Binding().ExecutionRef != ref || row.State != d.StateSucceeded {
		return d.Manifest{}, d.ErrConflict
	}
	var out d.Manifest
	e = json.Unmarshal(row.Manifest, &out)
	return out, e
}
func (r *DockerRunner) ValidateOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	parts := strings.Split(strings.TrimPrefix(ref.ProducerID, "sandbox-governed:"), "/")
	if len(parts) < 2 {
		return l.ErrForbidden
	}
	row, e := r.Get(ctx, p, parts[0])
	if e != nil || row.State != d.StateSucceeded {
		return l.ErrForbidden
	}
	var refs []l.RefID
	if json.Unmarshal(row.OutputRefs, &refs) != nil {
		return l.ErrForbidden
	}
	for _, v := range refs {
		if v == ref.RefID {
			return nil
		}
	}
	return l.ErrForbidden
}
func (r *DockerRunner) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 4; i++ {
				worked, e := r.Tick(ctx)
				if e != nil || !worked {
					break
				}
			}
		}
	}
}
func (r *DockerRunner) Tick(ctx context.Context) (bool, error) {
	row, e := r.repo.Claim(ctx, 2*time.Minute)
	if e != nil || row == nil {
		return false, e
	}
	p := row.Principal()
	ctx = agentctx.WithRequestID(ctx, row.RequestID)
	var v ValidatedSubmission
	if json.Unmarshal(row.Submission, &v) != nil {
		return true, r.finish(ctx, row, d.StateFailed, "invalid_binding")
	}
	if row.CancelRequested {
		return true, r.finish(ctx, row, d.StateCancelled, "cancelled")
	}
	if time.Now().After(row.Deadline) {
		state := d.StateTimedOut
		code := "timeout"
		if row.Phase == "launch_committed" {
			code = "deadline_or_launch_unknown"
		}
		return true, r.finish(ctx, row, state, code)
	}
	if _, _, e = r.read(ctx, p, row.ID, true); e != nil {
		return true, r.finish(ctx, row, d.StateFailed, "source_access_revoked")
	}
	if row.Phase == "collected" || (row.Phase == "cleanup_pending" && row.FinishState == d.StateSucceeded) {
		return true, r.finish(ctx, row, d.StateSucceeded, "")
	}
	if row.Phase == "cleanup_pending" {
		return true, r.finish(ctx, row, row.FinishState, row.FinishError)
	}
	if row.Phase == "" {
		inputs := map[string][]byte{}
		for _, input := range append([]d.InputRef{v.Request.Code}, v.Request.Inputs...) {
			b, err := r.load(ctx, p, input.Ref)
			if err != nil {
				return true, r.finish(ctx, row, d.StateFailed, "input_unavailable")
			}
			sum := sha256.Sum256(b)
			if "sha256:"+hex.EncodeToString(sum[:]) != input.SHA256 || int64(len(b)) != input.SizeBytes {
				return true, r.finish(ctx, row, d.StateFailed, "input_changed")
			}
			inputs[input.Name] = b
		}
		if e = r.host.Prepare(ctx, v, inputs); e != nil {
			return true, r.finish(ctx, row, d.StateFailed, "runtime_unavailable")
		}
		row.ExecID, e = r.host.CreateExec(ctx, v)
		if e != nil {
			return true, r.finish(ctx, row, d.StateFailed, "runtime_unavailable")
		}
		row.Phase = "launch_committed"
		row.State = d.StateRunning
		if e = r.repo.Save(ctx, row); e != nil {
			return true, e
		}
		// The persisted Engine exec ID is single-use. A replacement controller can
		// start this exact instance; the daemon rejects a second execution.
		current, err := r.repo.Get(ctx, p, row.ID)
		if err != nil || current.CancelRequested || !current.Deadline.After(time.Now()) {
			return true, err
		}
		_ = r.host.StartExec(ctx, row.ExecID)
		return true, nil
	}
	if e = r.host.StartExec(ctx, row.ExecID); e != nil && !errors.Is(e, d.ErrConflict) {
		return true, r.repo.Save(ctx, row)
	}
	ready, code, e := r.host.ReceiptExec(ctx, v, row.ExecID)
	if e != nil {
		return true, r.finish(ctx, row, d.StateFailed, "invalid_receipt")
	}
	if !ready {
		return true, r.repo.Save(ctx, row)
	}
	if code != 0 {
		state := d.StateFailed
		if code == 124 {
			state = d.StateTimedOut
		}
		return true, r.finish(ctx, row, state, fmt.Sprintf("code_exit_%d", code))
	}
	archive, e := r.host.Outputs(ctx, v)
	if e != nil {
		return true, r.finish(ctx, row, d.StateFailed, "output_unavailable")
	}
	manifest, files, e := CollectArchive(v, archive)
	if e != nil {
		return true, r.finish(ctx, row, d.StateFailed, "invalid_output")
	}
	// Recheck source authority after compute and before registering candidates.
	if _, _, e = r.read(ctx, p, row.ID, true); e != nil {
		return true, r.finish(ctx, row, d.StateFailed, "source_access_revoked")
	}
	parents := []l.RefID{v.Request.Code.Ref}
	for _, input := range v.Request.Inputs {
		parents = append(parents, input.Ref)
	}
	refs := []l.RefID{}
	for _, file := range files {
		ref, err := r.write(ctx, p, "sandbox-governed:"+row.ID+"/"+file.Artifact.Path, parents, file.Body)
		if err != nil {
			return true, r.finish(ctx, row, d.StateFailed, "candidate_registration_failed")
		}
		refs = append(refs, ref)
	}
	row.Manifest, _ = json.Marshal(manifest)
	row.OutputRefs, _ = json.Marshal(refs)
	row.Phase = "collected"
	return true, r.repo.Save(ctx, row)
}
func (r *DockerRunner) finish(ctx context.Context, row *d.Record, state d.State, code string) error {
	// Cancel/timeout may only be confirmed after the whole container is gone.
	cleanup := r.host.Remove(ctx, row.ID)
	if cleanup != nil {
		row.FinishState = state
		row.FinishError = code
		row.Phase = "cleanup_pending"
		row.State = d.StateReconcileRequired
		row.ErrorCode = "container_cleanup_unconfirmed"
		return r.repo.Save(ctx, row)
	}
	row.State = state
	row.ErrorCode = code
	if state != d.StateSucceeded {
		row.Manifest = nil
		row.OutputRefs = nil
	}
	e := r.repo.Save(ctx, row)
	if errors.Is(e, d.ErrConflict) {
		return e
	}
	return e
}
