package apisource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	d "cognida/internal/model/apisource"
	"cognida/internal/model/job"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/objectstore"
	"cognida/internal/service/collection/fetcher"
)

func (s *Service) Handle(ctx context.Context, j job.Job) error {
	var item delivery
	if j.Type != d.JobType || json.Unmarshal(j.Payload, &item) != nil || item.ID != j.ID {
		return d.ErrInvalid
	}
	p := l.Principal{TenantID: item.TenantID, UserID: item.UserID}
	run, e := s.repo.Claim(ctx, p, item.ID, 30*time.Second)
	if errors.Is(e, d.ErrConflict) {
		return nil
	}
	if e != nil || run.Terminal() {
		return e
	}
	work, cancel := context.WithDeadline(ctx, run.Deadline)
	s.mu.Lock()
	s.active[run.ID] = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); delete(s.active, run.ID); s.mu.Unlock() }()
	fail := func(cause error) error { return s.finishFailure(ctx, p, run, cause) }
	if run.Error != "" && run.Error != "pages_complete" {
		return fail(errSnapshot)
	}
	for run.Error != "pages_complete" {
		credential, e := s.check(work, p, run)
		if e != nil {
			return fail(e)
		}
		if len(run.Pages) >= run.Config.Limits.MaxPages || run.Bytes >= run.Config.Limits.MaxBytes || run.Requests >= run.Config.Limits.MaxRequests {
			return fail(errLimit)
		}
		endpoint, e := s.pageURL(run)
		if e != nil {
			return fail(e)
		}
		reserved, e := s.repo.ReserveRequest(work, p, run.ID, run.Epoch)
		if e != nil {
			return fail(e)
		}
		run = reserved
		request, cancelRequest := context.WithTimeout(work, 20*time.Second)
		response, e := s.fetch.FetchJSON(request, endpoint, credential, run.Config.Limits.MaxPageBytes)
		cancelRequest()
		if e != nil {
			return fail(e)
		}
		if int64(len(response.Body))+run.Bytes > run.Config.Limits.MaxBytes {
			return fail(errLimit)
		}
		cp, e := s.parsePage(run, response.Body)
		if e != nil {
			return fail(e)
		}
		// Reject reflected credentials, including JSON-escaped representations,
		// before storing either the original page or normalized records.
		secret := strings.TrimPrefix(credential.Value, "Bearer ")
		if containsSecret(response.Body, secret) {
			return fail(fetcher.ErrAuthentication)
		}
		snapshot := response.Body
		// Raw source bytes are retained exactly; page URL and headers are not stored.
		cp.Page.ObjectKey = fmt.Sprintf("api-ingestion/%d/%d/%s/pages/%d/%s.json", p.TenantID, p.UserID, run.ID, cp.Page.Number, cp.Page.Digest)
		if _, e = s.store.Put(work, objectstore.PutRequest{Key: cp.Page.ObjectKey, Body: bytes.NewReader(snapshot), Size: int64(len(snapshot)), ContentType: "application/json"}); e != nil {
			return fail(errors.New("page snapshot persistence failed"))
		}
		if _, e = s.check(work, p, run); e != nil {
			return fail(e)
		}
		next, e := s.repo.Checkpoint(work, p, run.ID, run.Epoch, cp)
		if e != nil {
			return fail(e)
		}
		run = next
	}
	if _, e = s.check(work, p, run); e != nil {
		return fail(e)
	}
	refs, e := s.artifacts(work, p, run, true)
	if e != nil {
		return fail(e)
	}
	_, e = s.repo.Finish(work, p, run.ID, run.Epoch, "completed", "", false, refs)
	if e != nil {
		return fail(e)
	}
	return nil
}
func containsSecret(raw []byte, secret string) bool {
	if strings.Contains(string(raw), secret) {
		return true
	}
	root, e := decode(raw)
	if e != nil {
		return false
	}
	var walk func(any) bool
	walk = func(value any) bool {
		switch v := value.(type) {
		case string:
			return strings.Contains(v, secret)
		case map[string]any:
			for key, child := range v {
				if strings.Contains(key, secret) || walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range v {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(root)
}
func (s *Service) finishFailure(parent context.Context, p l.Principal, run *d.Run, cause error) error {
	// Cancellation and lost leases cannot change the current owner's receipt.
	final, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	current, e := s.repo.GetRun(final, p, run.ID)
	if e != nil {
		return e
	}
	if current.Terminal() || current.Epoch != run.Epoch {
		return nil
	}
	run = current
	status, message, retryable := "failed", "API source request failed", false
	switch {
	case errors.Is(cause, errSnapshot):
		message = "source snapshot cannot be continued; start a new attempt"
	case errors.Is(cause, errLimit):
		message = "read limit reached before complete snapshot"
	case errors.Is(cause, errRecord):
		message = "JSON record, pagination or stable key contract failed"
	case errors.Is(cause, fetcher.ErrCursorExpired):
		message = "source cursor expired; start a new snapshot attempt"
	case errors.Is(cause, fetcher.ErrAuthentication) || errors.Is(cause, d.ErrForbidden):
		message = "source or credential authorization unavailable"
	case errors.Is(cause, fetcher.ErrUnsupportedType):
		message = "source did not return JSON"
	case errors.Is(cause, fetcher.ErrResponseTooLarge):
		message = "source response exceeded byte limit"
	case errors.Is(cause, fetcher.ErrUnsafeDestination) || errors.Is(cause, fetcher.ErrInvalidURL):
		message = "source destination is not allowed"
	case errors.Is(cause, context.DeadlineExceeded) || !run.Deadline.After(time.Now()):
		message = "source deadline exceeded"
		status = "timed_out"
	default:
		retryable = (len(run.Pages) == 0 || run.SnapshotCiphertext != "") && run.Requests < run.Config.Limits.MaxRequests && run.Deadline.After(time.Now())
	}
	if len(run.Pages) > 0 || retryable {
		status = "partial"
	}
	if !run.Deadline.After(time.Now()) {
		retryable = false
	}
	// Retryable partial runs keep their checkpoint without registering an output
	// whose deterministic ID might later represent different final bytes.
	var refs []l.RefID
	if !retryable && len(run.Pages) > 0 {
		if _, e = s.check(final, p, run); e == nil {
			refs, e = s.artifacts(final, p, run, false)
			if e != nil {
				refs = nil
			}
		}
	}
	_, e = s.repo.Finish(final, p, run.ID, run.Epoch, status, message, retryable, refs)
	if errors.Is(e, d.ErrConflict) {
		return nil
	}
	return e
}
func (s *Service) artifacts(ctx context.Context, p l.Principal, run *d.Run, complete bool) ([]l.RefID, error) {
	if _, e := s.check(ctx, p, run); e != nil {
		return nil, e
	}
	policy := l.PolicySnapshot{Sensitivity: "unknown", Scopes: []string{"read"}, Version: "1"}
	producer := "api-ingestion:" + run.ID
	refs := []l.RefID{}
	if len(run.Records) > 0 {
		input, e := table(run.Records)
		if e != nil {
			return nil, e
		}
		raw, _ := json.Marshal(input)
		asset, e := s.assets.ImportDerived(ctx, p, l.RawAsset, producer, nil, raw, policy)
		if e != nil {
			return nil, e
		}
		refs = append(refs, asset.RefID)
	}
	// No URL, query cursor, credential value or opaque token appears in manifest.
	manifest := map[string]any{"schema_version": 1, "source_type": "authenticated_json_api", "source_id": run.SourceID, "source_version": run.SourceVersion, "run_id": run.ID, "complete": complete, "candidate_only": true, "knowledge_published": false, "consistency": run.Consistency, "snapshot_digest": run.SnapshotDigest, "pages": run.Pages, "record_count": run.RecordCount, "duplicate_count": run.DuplicateCount, "deleted_count": run.DeletedCount, "bytes": run.Bytes, "watermark_before": run.WatermarkBefore, "watermark_after": run.WatermarkAfter, "raw_asset_refs": refs, "deletion_detection": "only explicit tombstones; absent rows are not deletions"}
	raw, _ := json.Marshal(manifest)
	artifact, e := s.assets.ImportDerived(ctx, p, l.Result, producer, nil, raw, policy)
	if e != nil {
		return nil, e
	}
	return append(refs, artifact.RefID), nil
}
func (s *Service) Exhausted(ctx context.Context, j job.Job, _ error) error {
	var item delivery
	if json.Unmarshal(j.Payload, &item) != nil || j.ID != item.ID {
		return d.ErrInvalid
	}
	p := l.Principal{TenantID: item.TenantID, UserID: item.UserID}
	run, e := s.repo.Claim(ctx, p, item.ID, 30*time.Second)
	if e != nil || run.Terminal() {
		return e
	}
	return s.finishFailure(ctx, p, run, errors.New("delivery retries exhausted"))
}
