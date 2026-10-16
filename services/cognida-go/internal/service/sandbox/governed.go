package sandbox

import (
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	"context"
	"encoding/json"
	"strings"
	"time"
)

// GovernedBinding freezes the parent dispatch envelope independently of the
// sandbox input/profile digest. It grants no additional execution permissions.
type GovernedBinding struct {
	CommandHash    string    `json:"command_hash"`
	ParentDeadline time.Time `json:"parent_deadline"`
}

func governedHash(hash string) bool {
	return len(hash) == 64 && digestPattern.MatchString("sha256:"+hash)
}
func (r *DockerRunner) GovernedPreflight(ctx context.Context, p l.Principal, in d.SubmitRequest) error {
	if e := r.authorize(ctx, p); e != nil {
		return e
	}
	v, e := r.validator.ValidateSubmission(ctx, p, in)
	if e != nil {
		return e
	}
	for _, input := range in.Inputs {
		if input.Name == "runner.py" {
			return d.ErrInvalid
		}
	}
	return r.host.Check(ctx, v.Profile.Image)
}
func (r *DockerRunner) LookupGoverned(ctx context.Context, p l.Principal, key, hash string) (*d.Record, error) {
	if p.Validate() != nil || !idempotencyKey.MatchString(key) || !governedHash(hash) {
		return nil, d.ErrInvalid
	}
	row, e := r.repo.FindByKey(ctx, p, key)
	if e != nil || row == nil {
		return row, e
	}
	var v ValidatedSubmission
	if json.Unmarshal(row.Submission, &v) != nil || v.Governed == nil || v.Governed.CommandHash != hash || ValidateReplay(p, row.Binding(), v.Binding) != nil {
		return nil, d.ErrConflict
	}
	return row, nil
}
func (r *DockerRunner) SubmitGoverned(ctx context.Context, p l.Principal, in d.SubmitRequest, hash string, deadline time.Time) (*d.Record, error) {
	if !governedHash(hash) || deadline.IsZero() {
		return nil, d.ErrInvalid
	}
	if prior, e := r.LookupGoverned(ctx, p, in.IdempotencyKey, hash); e != nil || prior != nil {
		return prior, e
	}
	if e := r.GovernedPreflight(ctx, p, in); e != nil {
		return nil, e
	}
	v, e := r.validator.ValidateSubmission(ctx, p, in)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	if !deadline.After(now) {
		return nil, d.ErrConflict
	}
	v.Governed = &GovernedBinding{CommandHash: hash, ParentDeadline: deadline}
	effective := now.Add(time.Duration(in.Resources.TimeoutSeconds) * time.Second)
	if deadline.Before(effective) {
		effective = deadline
	}
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	requestID, _ := agentctx.GetRequestID(ctx)
	return r.repo.Create(ctx, &d.Record{ID: v.Binding.ID, TenantID: p.TenantID, UserID: p.UserID, IdempotencyKey: in.IdempotencyKey, RequestSHA256: v.Binding.RequestSHA256, RequestID: requestID, Submission: raw, State: d.StateQueued, Version: 1, Deadline: effective, CreatedAt: now, UpdatedAt: now})
}

// CancelGoverned records absence as a terminal tombstone before a late Submit
// can create work. Source revocation must not prevent the owner stopping it.
func (r *DockerRunner) CancelGoverned(ctx context.Context, p l.Principal, in d.SubmitRequest, hash string, deadline time.Time) (*d.Record, error) {
	if !governedHash(hash) || deadline.IsZero() {
		return nil, d.ErrInvalid
	}
	if e := r.authorize(ctx, p); e != nil {
		return nil, e
	}
	row, e := r.LookupGoverned(ctx, p, in.IdempotencyKey, hash)
	if e != nil {
		return nil, e
	}
	if row != nil {
		return r.repo.RequestCancel(ctx, p, row.ID)
	}
	v, e := r.validator.validateSubmission(ctx, p, in, false)
	if e != nil {
		return nil, e
	}
	for _, input := range in.Inputs {
		if strings.EqualFold(input.Name, "runner.py") {
			return nil, d.ErrInvalid
		}
	}
	v.Governed = &GovernedBinding{CommandHash: hash, ParentDeadline: deadline}
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	requestID, _ := agentctx.GetRequestID(ctx)
	row, e = r.repo.Create(ctx, &d.Record{ID: v.Binding.ID, TenantID: p.TenantID, UserID: p.UserID, IdempotencyKey: in.IdempotencyKey, RequestSHA256: v.Binding.RequestSHA256, RequestID: requestID, Submission: raw, State: d.StateCancelled, CancelRequested: true, Version: 1, Deadline: deadline, CreatedAt: now, UpdatedAt: now})
	if e != nil {
		return nil, e
	}
	return r.repo.RequestCancel(ctx, p, row.ID)
}
