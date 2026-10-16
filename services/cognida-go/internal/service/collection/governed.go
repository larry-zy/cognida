package collection

import (
	d "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type GovernedAssets interface {
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
	ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error)
}

func (s *Service) SetGovernedAuthorizer(fn func(context.Context, l.Principal) error) {
	s.governedAuthorize = fn
}
func (s *Service) SetGovernedAssets(assets GovernedAssets) { s.governedAssets = assets }
func (s *Service) governedRepo() (d.GovernedRepository, error) {
	r, ok := s.repository.(d.GovernedRepository)
	if !ok {
		return nil, tc.ErrInvalid
	}
	return r, nil
}
func (s *Service) LookupGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation) (*d.Run, error) {
	if p.Validate() != nil {
		return nil, tc.ErrForbidden
	}
	if e := in.Validate(); e != nil {
		return nil, e
	}
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	run, e := r.FindGoverned(ctx, p, in.OperationKey)
	if e != nil || run == nil {
		return run, e
	}
	if !d.SameGoverned(*run.Governed, in) {
		return nil, tc.ErrConflict
	}
	return run, nil
}
func (s *Service) SubmitGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation, cancel bool) (*d.Run, error) {
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	if !cancel {
		if s.governedAuthorize == nil || s.governedAuthorize(ctx, p) != nil {
			return nil, tc.ErrForbidden
		}
		if s.artifacts == nil || s.governedAssets == nil || s.submitter == nil {
			return nil, tc.ErrInvalid
		}
		if !in.Deadline.After(time.Now()) {
			return nil, context.DeadlineExceeded
		}
	}
	run, e := r.SubmitGoverned(ctx, p, in, cancel)
	if e != nil {
		return nil, e
	}
	if !run.Status.IsTerminal() && !cancel {
		if e = s.submitter.Submit(ctx, run); e != nil {
			return nil, e
		}
	}
	return run, nil
}
func (s *Service) executeGoverned(ctx context.Context, task *d.Task, run *d.Run) error {
	defer func() { s.mu.Lock(); delete(s.cancels, run.ID); s.mu.Unlock() }()
	p := l.Principal{TenantID: task.TenantID, UserID: task.UserID}
	r, e := s.governedRepo()
	if e != nil {
		return e
	}
	if run.Governed == nil || run.Governed.Validate() != nil || s.governedAssets == nil || s.artifacts == nil {
		return tc.ErrInvalid
	}
	in := *run.Governed
	limit := run.StartedAt.Add(time.Duration(in.TimeoutSeconds) * time.Second)
	if in.Deadline.Before(limit) {
		limit = in.Deadline
	}
	work, cancel := context.WithDeadline(ctx, limit)
	defer cancel()
	finish := func(status d.RunStatus, refs []l.RefID, message string) error {
		final, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		if status == d.RunStatusFailed {
			items, e := s.repository.ListItems(final, p.TenantID, run.ID)
			if e != nil {
				return e
			}
			for _, item := range items {
				if item.Status == d.ItemStatusPending {
					item.Status = d.ItemStatusFailed
					item.ErrorReason = message
					item.UpdatedAt = time.Now().UTC()
					if e = s.repository.UpdateItem(final, item); e != nil {
						return e
					}
				}
			}
		}
		_, e := r.FinishGoverned(final, p, run.ID, status, refs, message)
		return e
	}
	check := func() error {
		if e := work.Err(); e != nil {
			return e
		}
		if s.governedAuthorize == nil || s.governedAuthorize(work, p) != nil {
			return tc.ErrForbidden
		}
		current, e := r.FindGoverned(work, p, in.OperationKey)
		if e != nil {
			return e
		}
		if current == nil {
			return tc.ErrNotFound
		}
		if current.Status.IsTerminal() {
			return context.Canceled
		}
		return nil
	}
	if e = check(); e != nil {
		if errors.Is(e, context.Canceled) {
			return nil
		}
		if errors.Is(e, context.DeadlineExceeded) {
			return finish(d.RunStatusFailed, nil, "deadline_exceeded")
		}
		return e
	}
	sources, e := s.repository.ListSources(work, p.TenantID, task.ID)
	if e != nil {
		return e
	}
	if len(sources) != 1 || sources[0].URL != in.URL {
		return tc.ErrConflict
	}
	items, e := s.repository.ListItems(work, p.TenantID, run.ID)
	if e != nil {
		return e
	}
	var item *d.Item
	if len(items) == 1 {
		item = items[0]
	} else if len(items) > 1 {
		return tc.ErrConflict
	}
	if item == nil {
		now := time.Now().UTC()
		item = &d.Item{ID: run.ID, TenantID: p.TenantID, TaskID: task.ID, RunID: run.ID, SourceID: sources[0].ID, SourceURL: in.URL, Status: d.ItemStatusPending, CreatedAt: now, UpdatedAt: now}
		if e = s.repository.CreateItem(work, item); e != nil {
			return e
		}
	}
	response, e := s.fetchOrRestore(work, task, item, sources[0])
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(work.Err(), context.DeadlineExceeded) {
			return finish(d.RunStatusFailed, nil, "deadline_exceeded")
		}
		if errors.Is(e, errArtifact) {
			return e
		}
		return finish(d.RunStatusFailed, nil, "collection_fetch_failed")
	}
	if e = check(); e != nil {
		if errors.Is(e, context.Canceled) {
			return nil
		}
		reason := "authorization_revoked"
		if errors.Is(e, context.DeadlineExceeded) {
			reason = "deadline_exceeded"
		}
		return finish(d.RunStatusFailed, nil, reason)
	}
	if response == nil || response.StatusCode < 200 || response.StatusCode >= 300 || int64(len(response.Body)) > in.MaxResponseBytes {
		return finish(d.RunStatusFailed, nil, "collection_http_failed")
	}
	item.CanonicalURL, item.ContentType, item.HTTPStatus = response.CanonicalURL, response.ContentType, response.StatusCode
	if e = s.persistRaw(work, item, response); e != nil {
		return e
	}
	content, e := s.normalize.Normalize(response.ContentType, response.Body)
	if e != nil || strings.TrimSpace(content) == "" {
		return finish(d.RunStatusFailed, nil, "collection_normalization_failed")
	}
	input := l.Input{Kind: "text", Text: content}
	if input.Validate() != nil {
		return finish(d.RunStatusFailed, nil, "collection_content_limit")
	}
	sum := sha256.Sum256([]byte(content))
	item.ContentHash = hex.EncodeToString(sum[:])
	if e = s.persistNormalized(work, item, content); e != nil {
		return e
	}
	if e = check(); e != nil {
		if errors.Is(e, context.Canceled) {
			return nil
		}
		reason := "authorization_revoked"
		if errors.Is(e, context.DeadlineExceeded) {
			reason = "deadline_exceeded"
		}
		return finish(d.RunStatusFailed, nil, reason)
	}
	policy := l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"}
	raw, _ := json.Marshal(input)
	producer := "collection-governed:" + run.ID
	asset, e := s.governedAssets.ImportDerived(work, p, l.RawAsset, producer, nil, raw, policy)
	if e != nil {
		return e
	}
	manifest := map[string]any{"schema_version": 1, "domain": "collection", "mode": "snapshot", "task_id": task.ID, "run_id": run.ID, "item_id": item.ID, "source_url": in.URL, "canonical_url": response.CanonicalURL, "content_hash": item.ContentHash, "raw_asset_ref": asset.RefID.String(), "knowledge_published": false}
	raw, _ = json.Marshal(manifest)
	result, e := s.governedAssets.ImportDerived(work, p, l.Result, producer, nil, raw, policy)
	if e != nil {
		return e
	}
	item.AssetRefID = asset.RefID.String()
	item.Status = d.ItemStatusAccepted
	item.UpdatedAt = time.Now().UTC()
	if e = s.repository.UpdateItem(work, item); e != nil {
		return e
	}
	return finish(d.RunStatusCompleted, []l.RefID{asset.RefID, result.RefID}, "")
}

// ValidateGovernedOrigin proves the exact immutable output was committed by this
// owner's real snapshot run. A matching parent or a producer prefix alone is insufficient.
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	prefix := "collection-governed:"
	if !strings.HasPrefix(ref.ProducerID, prefix) || p.Validate() != nil || ref.TenantID != p.TenantID {
		return l.ErrForbidden
	}
	id := strings.TrimPrefix(ref.ProducerID, prefix)
	task, e := s.repository.FindTask(ctx, p.TenantID, id)
	if e != nil || task.UserID != p.UserID || task.Mode != d.SnapshotMode {
		return l.ErrForbidden
	}
	run, e := s.repository.FindLatestRun(ctx, p.TenantID, id)
	if e != nil || run.ID != id || run.Governed == nil || run.Status != d.RunStatusCompleted {
		return l.ErrForbidden
	}
	found := false
	for _, output := range run.OutputRefs {
		if output == ref.RefID {
			found = true
		}
	}
	if !found {
		return l.ErrForbidden
	}
	items, e := s.repository.ListItems(ctx, p.TenantID, id)
	if e != nil || len(items) != 1 || items[0].Status != d.ItemStatusAccepted || items[0].ContentHash == "" || items[0].NormalizedObjectKey == "" {
		return l.ErrForbidden
	}
	if len(ref.Parents) != 0 || ref.Provenance.SourceType != "lifecycle" || ref.Provenance.SourceID != ref.ProducerID {
		return l.ErrForbidden
	}
	return nil
}
