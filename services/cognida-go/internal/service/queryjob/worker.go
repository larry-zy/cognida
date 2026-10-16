package queryjob

import (
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/queryjob"
	"context"
	"errors"
	"sync"
	"time"
)

// Start owns no server process and stops when its caller's context is cancelled.
func (s *Service) Start(ctx context.Context) {
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			items, e := s.repo.Runnable(ctx, 4)
			if e != nil {
				continue
			}
			var wg sync.WaitGroup
			for _, item := range items {
				if ctx.Err() != nil {
					break
				}
				wg.Add(1)
				go func(item d.Item) { defer wg.Done(); _ = s.Tick(ctx, item.Scope, item.ID) }(item)
			}
			wg.Wait()
		}
	}
}
func (s *Service) Tick(ctx context.Context, p l.Principal, id string) error {
	r, e := s.repo.Claim(ctx, p, id)
	if e != nil {
		return e
	}
	epoch := r.Epoch
	defer func() {
		end, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.repo.Release(end, p, id, epoch)
	}()
	save := func(fn func(*d.Record, time.Time) error) error {
		next, e := s.repo.Change(ctx, p, id, epoch, fn)
		if e == nil {
			r = next
		}
		return e
	}
	note := func(message string) error {
		return save(func(v *d.Record, _ time.Time) error {
			v.Job.Error = message
			v.Job.Status = "reconcile_required"
			return nil
		})
	}
	if r.Phase == "queued" && (r.Job.CancelRequested || !time.Now().Before(r.Job.Deadline)) {
		return save(func(v *d.Record, now time.Time) error {
			v.Job.Status = "cancelled"
			if !now.Before(v.Job.Deadline) {
				v.Job.Status = "timed_out"
			}
			v.Phase = "finished"
			return nil
		})
	}
	a, source, e := s.provider.AcquireQuery(ctx, p.TenantID, r.Job.Input.DataSourceID)
	if e != nil || source == nil || sourceVersion(source) != r.Job.SourceVersion {
		return note("source_connection_unavailable_or_changed")
	}
	adapter, ok := a.(ds.AsyncQueryAdapter)
	if !ok {
		return note("async_adapter_unavailable")
	}
	prepared, hasPrepared := a.(ds.PreparedAsyncQueryAdapter)
	operation, e := s.open(r.Operation)
	if e != nil {
		return note("operation_handle_unavailable")
	}
	prepareID, e := s.open(r.Prepared)
	if e != nil {
		return note("prepared_handle_unavailable")
	}
	request := ds.QueryRequest{SQL: r.Job.Input.SQL, MaxRows: r.Job.Input.MaxRows, MaxBytes: r.Job.Input.MaxBytes, Timeout: time.Duration(r.Job.Input.TimeoutSeconds) * time.Second, MaxScanBytes: r.Job.Input.MaxScanBytes, OperationID: r.Job.ID}
	// Cleanup follows committed result truth, including after a concurrent cancel.
	cleanup := func(final string) error {
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var e error
		if operation != "" {
			e = adapter.Cancel(call, operation)
		} else if prepareID != "" && hasPrepared {
			e = prepared.AbandonPrepared(call, prepareID)
		}
		// BigQuery jobs.cancel acknowledges a request; only a subsequent terminal
		// status confirms cleanup. Hive CloseSession invalidates its handle.
		remoteState := ""
		if e == nil && operation != "" && r.Job.Engine == ds.TypeBigQuery {
			remoteState, e = adapter.Status(call, operation)
			if e == nil && remoteState != "succeeded" && remoteState != "failed" && remoteState != "cancelled" && remoteState != "timed_out" {
				e = errors.New("remote cancellation still pending")
			}
		}
		if e != nil {
			return save(func(v *d.Record, _ time.Time) error {
				if remoteState != "" {
					v.Job.RemoteStatus = remoteState
				}
				v.Job.CleanupPending = true
				v.Job.Status = "reconcile_required"
				v.Job.Error = "remote_cleanup_unconfirmed"
				return nil
			})
		}
		return save(func(v *d.Record, _ time.Time) error {
			if remoteState != "" {
				v.Job.RemoteStatus = remoteState
			}
			if v.Phase == "result_committed" && v.Job.RemoteStatus == "succeeded" {
				final = "succeeded"
			}
			v.Job.CleanupPending = false
			v.Job.Status = final
			v.Phase = "finished"
			return nil
		})
	}
	if r.Phase == "result_committed" {
		return cleanup("succeeded")
	}
	authorized := s.authorized(ctx, p, r) == nil
	if r.Job.CancelRequested || !authorized || !time.Now().Before(r.Job.Deadline) {
		final := "cancelled"
		if !authorized {
			final = "failed"
		}
		if !time.Now().Before(r.Job.Deadline) {
			final = "timed_out"
		}
		// Once dispatch may have happened, absence of its handle is not cancellation.
		if r.Phase == "submitting" && hasPrepared && operation == "" {
			final = "aborted_unknown"
		}
		if r.Phase == "submitting" && !hasPrepared && operation == "" {
			lookup, ok := a.(ds.SubmissionLookup)
			if !ok {
				return note("submission_lookup_unavailable")
			}
			call, cancel := context.WithTimeout(ctx, 5*time.Second)
			handle, state, found, err := lookup.LookupSubmission(call, request)
			cancel()
			if err != nil {
				return note("submission_lookup_unavailable")
			}
			if !found {
				return note("cancel_requested_submission_not_yet_visible")
			}
			sealed, err := s.seal(handle)
			if err != nil {
				return err
			}
			operation = handle
			if err = save(func(v *d.Record, _ time.Time) error {
				v.Operation = sealed
				v.Job.RemoteStatus = state
				v.Phase = "submitted"
				return nil
			}); err != nil {
				return err
			}
		}
		if operation != "" {
			call, cancel := context.WithTimeout(ctx, 3*time.Second)
			state, err := adapter.Status(call, operation)
			cancel()
			if err == nil {
				_ = save(func(v *d.Record, _ time.Time) error { v.Job.RemoteStatus = state; return nil })
			}
		}
		if !authorized {
			_ = save(func(v *d.Record, _ time.Time) error { v.Job.Error = "source_authorization_revoked"; return nil })
		}
		return cleanup(final)
	}
	if r.Phase == "preparing" { // Prepare's response itself was lost: never execute SQL.
		return save(func(v *d.Record, _ time.Time) error {
			v.Job.Status = "aborted_unknown"
			v.Job.Error = "prepared_session_response_lost_no_statement_replay"
			v.Phase = "finished"
			return nil
		})
	}
	gate := func(phase string) error {
		return save(func(v *d.Record, now time.Time) error {
			if v.Job.CancelRequested || !now.Before(v.Job.Deadline) {
				return d.ErrConflict
			}
			v.Phase = phase
			v.Job.Status = "running"
			return nil
		})
	}
	if r.Phase == "queued" && hasPrepared {
		if e = gate("preparing"); e != nil {
			return e
		}
		call, cancel := context.WithDeadline(ctx, r.Job.Deadline)
		handle, err := prepared.Prepare(call, request)
		cancel()
		if err != nil {
			return save(func(v *d.Record, _ time.Time) error {
				v.Job.Status = "failed"
				v.Job.Error = "remote_prepare_failed"
				v.Phase = "finished"
				return nil
			})
		}
		sealed, err := s.seal(handle)
		if err != nil {
			return err
		}
		prepareID = handle
		if e = save(func(v *d.Record, _ time.Time) error { v.Prepared = sealed; v.Phase = "prepared"; return nil }); e != nil {
			end, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = prepared.AbandonPrepared(end, handle)
			return e
		}
	}
	if r.Phase == "submitting" && hasPrepared && operation == "" {
		return cleanup("aborted_unknown")
	}
	if r.Phase == "queued" || r.Phase == "prepared" || r.Phase == "submitting" && operation == "" {
		if e = gate("submitting"); e != nil {
			return e
		}
		call, cancel := context.WithDeadline(ctx, r.Job.Deadline)
		var handle string
		if hasPrepared {
			handle, e = prepared.SubmitPrepared(call, prepareID, request)
		} else {
			handle, e = adapter.Submit(call, request)
		}
		cancel()
		if handle != "" {
			sealed, err := s.seal(handle)
			if err != nil {
				return err
			}
			operation = handle
			if err = save(func(v *d.Record, _ time.Time) error { v.Operation = sealed; v.Phase = "submitted"; return nil }); err != nil {
				return err
			}
		}
		if e != nil {
			if hasPrepared && handle == "" {
				return cleanup("aborted_unknown")
			}
			return note("submission_outcome_requires_lookup")
		}
		if operation == "" {
			return note("submission_missing_handle")
		}
	}
	if e = gate("submitted"); e != nil {
		return e
	}
	call, cancel := context.WithDeadline(ctx, r.Job.Deadline)
	state, e := adapter.Status(call, operation)
	cancel()
	if e != nil {
		return note("remote_status_unavailable")
	}
	if e = save(func(v *d.Record, _ time.Time) error { v.Job.RemoteStatus = state; return nil }); e != nil {
		return e
	}
	if state == "running" || state == "queued" {
		return nil
	}
	if state != "succeeded" {
		if state == "cancelled" || state == "failed" || state == "timed_out" {
			return cleanup(state)
		}
		return note("remote_state_unknown")
	}
	cursor, e := s.open(r.Cursor)
	if e != nil {
		return note("page_cursor_unavailable")
	}
	call, cancel = context.WithDeadline(ctx, r.Job.Deadline)
	part, next, e := adapter.Page(call, operation, cursor, min(1000, r.Job.Input.MaxRows+1))
	cancel()
	if e != nil {
		return note("result_page_unavailable")
	}
	if part == nil || len(part.Columns) < 1 || len(part.Columns) > 512 {
		return note("result_schema_invalid")
	}
	complete := next == ""
	encoded, e := s.seal(next)
	if e != nil {
		return e
	}
	if e = save(func(v *d.Record, now time.Time) error {
		if v.Job.CancelRequested || !now.Before(v.Job.Deadline) {
			return d.ErrConflict
		}
		if v.Result == nil {
			v.Result = &ds.QueryResult{Columns: part.Columns, ColumnTypes: part.ColumnTypes, Rows: []map[string]any{}, ExecutedSQL: part.ExecutedSQL, SourceVersion: part.SourceVersion}
			v.Job.Bytes = resultBytes(map[string]any{"columns": part.Columns, "column_types": part.ColumnTypes, "rows": []any{}})
			if v.Job.Bytes > v.Job.Input.MaxBytes {
				return d.ErrInvalid
			}
		} else if d.Digest(v.Result.Columns) != d.Digest(part.Columns) || d.Digest(v.Result.ColumnTypes) != d.Digest(part.ColumnTypes) {
			return d.ErrConflict
		}
		for _, row := range part.Rows {
			size := resultBytes(row)
			if len(v.Result.Rows) > 0 {
				size++
			}
			if len(v.Result.Rows) >= v.Job.Input.MaxRows || v.Job.Bytes+size > v.Job.Input.MaxBytes {
				v.Job.Truncated = true
				complete = true
				break
			}
			v.Result.Rows = append(v.Result.Rows, row)
			v.Job.Bytes += size
		}
		v.Job.RowCount = len(v.Result.Rows)
		v.Job.Truncated = v.Job.Truncated || part.Truncated
		if !complete {
			hash := d.Digest(next)
			for _, old := range v.SeenCursors {
				if old == hash {
					return d.ErrConflict
				}
			}
			if len(v.SeenCursors) >= 100 {
				return d.ErrConflict
			}
			v.SeenCursors = append(v.SeenCursors, hash)
			v.Cursor = encoded
		}
		if complete {
			v.Phase = "result_committed"
			v.Job.CleanupPending = true
		}
		return nil
	}); e != nil {
		return e
	}
	if complete {
		return cleanup("succeeded")
	}
	return nil
}
