package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"time"
)

func (s *Service) ValidateGovernedSFT(ctx context.Context, p l.Principal, params d.SFTBuildParameters, source l.RefID) error {
	if params.Validate() != nil || source.Validate() != nil {
		return d.ErrInvalid
	}
	if s.governedInputs == nil {
		return d.ErrForbidden
	}
	if err := s.governedInputs(ctx, p, []l.RefID{source}); err != nil {
		return recordSourceError(err)
	}
	_, _, err := s.buildSFTDataset(ctx, p, params.Bind(source))
	return err
}

func sftCommitAllowed(ctx context.Context, record *d.Record, state *d.GovernedExecution, repo d.Repository) error {
	if record.Status != "queued" && record.Status != "running" {
		return context.Canceled
	}
	clock, ok := repo.(d.GovernedCommitClock)
	if !ok {
		return d.ErrInvalid
	}
	now, err := clock.GovernedNow(ctx)
	if err != nil {
		return err
	}
	if state.ExecutionDeadline == nil || state.ExecutionDeadline.After(state.Invocation.Deadline) {
		return d.ErrInvalid
	}
	if !now.Before(*state.ExecutionDeadline) {
		return context.DeadlineExceeded
	}
	return ctx.Err()
}

func (s *Service) failGovernedSFT(ctx context.Context, record *d.Record, reason string) error {
	repo, err := s.governedRepo()
	if err != nil {
		return err
	}
	final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = repo.UpdateGoverned(final, l.Principal{TenantID: record.TenantID, UserID: record.UserID}, record.ID, func(op *d.Record, st *d.GovernedExecution, _ d.Repository) error {
		if op.Status == "completed" || op.Status == "cancelled" || op.Status == "failed" {
			return nil
		}
		// Candidate and completed receipt commit together. A failed build cannot
		// leave a partially reviewable dataset or erase an already committed one.
		op.Status, st.Error = "failed", reason
		return nil
	})
	return err
}

func (s *Service) runGovernedSFT(ctx context.Context, record *d.Record, state d.GovernedExecution) error {
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	in := state.Invocation
	repo, err := s.governedRepo()
	if err != nil {
		return err
	}
	if state.ExecutionDeadline == nil || state.ExecutionDeadline.After(in.Deadline) {
		return s.failGovernedSFT(ctx, record, "invalid_execution_deadline")
	}
	work, cancel := context.WithDeadline(ctx, *state.ExecutionDeadline)
	defer cancel()
	fail := func(err error) error {
		// Shutdown keeps queued/running durable work recoverable. A domain
		// deadline is terminal; it is not a generic host cancellation.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(work.Err(), context.DeadlineExceeded) {
			return s.failGovernedSFT(ctx, record, "deadline_exceeded")
		}
		return s.failGovernedSFT(ctx, record, "sft_candidate_validation_failed")
	}
	current, err := repo.UpdateGoverned(work, p, record.ID, func(op *d.Record, st *d.GovernedExecution, tx d.Repository) error {
		if err := sftCommitAllowed(work, op, st, tx); err != nil {
			return err
		}
		op.Status = "running"
		return nil
	})
	if errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return nil // Cancellation or another worker completed under the same lock.
	}
	if err != nil {
		return fail(err)
	}
	if current.Status != "running" {
		return nil
	}
	if s.governedInputs == nil {
		return fail(d.ErrForbidden)
	}
	if err = s.governedInputs(work, p, []l.RefID{in.InputRef}); err != nil {
		return fail(err)
	}
	ref, dataset, err := s.buildSFTDataset(work, p, in.SFT.Bind(in.InputRef))
	if err != nil {
		return fail(err)
	}
	manifest := map[string]any{
		"schema_version": 1, "domain": "governance", "profile": d.SFTProfile,
		"dataset_id": state.DomainID, "dataset_kind": d.SFTDatasetKind,
		"source_ref_id": in.InputRef.String(), "standard_version_id": dataset.StandardVersionID,
		"standard_digest": dataset.StandardDigest, "content_digest": dataset.ContentDigest,
		"row_count": dataset.RowCount, "split_counts": dataset.SplitCounts,
		"review_required": true, "dataset_published": false, "training_started": false,
	}
	policy := ref.Policy
	policy.Approved = false
	output, err := s.Assets.ImportDerived(work, p, l.Result, "governance-governed:"+record.ID, []l.RefID{in.InputRef}, mustLabelJSON(manifest), policy)
	if err != nil {
		if work.Err() != nil {
			return fail(work.Err())
		}
		return err // Transient storage failure remains recoverable, with fixed IDs.
	}
	// The manifest is unreadable until the origin receipt commits. Recheck the
	// complete source immediately before the atomic candidate/receipt commit.
	if err = s.governedInputs(work, p, []l.RefID{in.InputRef}); err != nil {
		return fail(err)
	}
	candidate := &d.Record{Meta: d.Meta{ID: state.DomainID, TenantID: p.TenantID, UserID: p.UserID}, Kind: d.SFTDatasetKind, ParentID: record.ID, Status: "draft", IdempotencyKey: record.ID + ":sft", Payload: mustLabelJSON(dataset)}
	if err = s.authorizeSFT(work, p, candidate); err != nil {
		return fail(err)
	}
	_, err = repo.UpdateGoverned(work, p, record.ID, func(op *d.Record, st *d.GovernedExecution, tx d.Repository) error {
		if err := sftCommitAllowed(work, op, st, tx); err != nil {
			return err
		}
		if !d.SameGoverned(st.Invocation, in) || st.DomainID != candidate.ID || st.DomainKind != d.SFTDatasetKind {
			return d.ErrConflict
		}
		created, err := tx.Create(work, candidate)
		if err != nil {
			return err
		}
		if created.ID != candidate.ID || created.UserID != p.UserID || created.ParentID != op.ID || !d.JSONEqual(created.Payload, candidate.Payload) {
			return d.ErrConflict
		}
		op.Status, st.Error = "completed", ""
		st.OutputRefs = []l.RefID{output.RefID}
		st.StandardDigest = dataset.StandardDigest
		return nil
	})
	if errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fail(err)
	}
	return err
}
