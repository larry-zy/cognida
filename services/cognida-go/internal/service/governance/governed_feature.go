package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"errors"
	"time"
)

// The feature service owns version validation and atomic materialization. This
// interface keeps the execution envelope independent of its compute internals.
type governedFeatures interface {
	ValidateBoundFeatureVersion(context.Context, l.Principal, string, int, string) error
	MaterializeGovernedFeature(context.Context, l.Principal, string, int, string) (*d.FeatureMaterialization, error)
	LookupFeatureMaterialization(context.Context, l.Principal, string) (*d.FeatureMaterialization, error)
}

func (s *Service) ValidateGovernedFeature(ctx context.Context, p l.Principal, id string, version int, source l.RefID) error {
	port, ok := any(s).(governedFeatures)
	if !ok || s.governedInputs == nil {
		return d.ErrInvalid
	}
	if e := s.governedInputs(ctx, p, []l.RefID{source}); e != nil {
		return e
	}
	return port.ValidateBoundFeatureVersion(ctx, p, id, version, source.String())
}
func (s *Service) runGovernedFeature(ctx context.Context, record *d.Record, state d.GovernedExecution) error {
	// A persisted job is checked before source authorization, preserving recovery
	// of real partial commits even after withdrawal of the source permission.
	if jobs, err := s.featureJobs(); err == nil {
		existing, err := jobs.FindFeatureJobByKey(ctx, record.TenantID, record.UserID, "governed-job:"+record.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			return s.runGovernedFeatureJob(ctx, record, state)
		}
	}
	embedding, err := s.isEmbeddingFeature(ctx, l.Principal{TenantID: record.TenantID, UserID: record.UserID}, state.Invocation.FeatureID, state.Invocation.FeatureVersion)
	if err == nil && embedding {
		return s.runGovernedFeatureJob(ctx, record, state)
	}

	port, ok := any(s).(governedFeatures)
	if !ok {
		return d.ErrInvalid
	}
	r, e := s.governedRepo()
	if e != nil {
		return e
	}
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	in := state.Invocation
	key := "governed:" + record.ID
	end := record.CreatedAt.Add(time.Duration(in.TimeoutSeconds) * time.Second)
	if in.Deadline.Before(end) {
		end = in.Deadline
	}
	work, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	finish := func(receipt *d.FeatureMaterialization, cause error) error {
		return s.finishGovernedFeature(ctx, record, state, receipt, cause)
	}
	// Scope-only receipt lookup comes before renewed authorization: committed
	// writes remain true even if their source was revoked after commit.
	receipt, e := port.LookupFeatureMaterialization(ctx, p, key)
	if e != nil {
		return e
	}
	if receipt != nil {
		return finish(receipt, nil)
	}
	current, e := r.UpdateGoverned(ctx, p, record.ID, func(op *d.Record, _ *d.GovernedExecution, _ d.Repository) error {
		if op.Status == "queued" {
			op.Status = "running"
		}
		return nil
	})
	if e != nil {
		return e
	}
	if current.Status == "cancelled" || current.Status == "failed" || current.Status == "completed" {
		return nil
	}
	if current.Status == "cancel_requested" {
		// FeatureMaterialize locks this same operation row before its first commit;
		// a pending cancellation therefore fences every late first write.
		receipt, e = port.LookupFeatureMaterialization(ctx, p, key)
		if e != nil {
			return e
		}
		return finish(receipt, nil)
	}
	if e = s.ValidateGovernedFeature(work, p, in.FeatureID, in.FeatureVersion, in.InputRef); e != nil {
		return finish(nil, e)
	}
	receipt, e = port.MaterializeGovernedFeature(work, p, in.FeatureID, in.FeatureVersion, record.ID)
	if e != nil {
		final, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		committed, lookupErr := port.LookupFeatureMaterialization(final, p, key)
		if lookupErr != nil {
			return lookupErr
		}
		if committed != nil {
			return finish(committed, nil)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return finish(nil, e)
	}
	return finish(receipt, nil)
}

// finishGovernedFeature holds the same operation lock as first materialization.
// An unlocked absent lookup is never enough to declare that a write did not occur.
func (s *Service) finishGovernedFeature(ctx context.Context, record *d.Record, state d.GovernedExecution, known *d.FeatureMaterialization, cause error) error {
	r, e := s.governedRepo()
	if e != nil {
		return e
	}
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	in := state.Invocation
	final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, e = r.UpdateGoverned(final, p, record.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
		receipts, ok := repo.(d.FeatureMaterializationRepository)
		if !ok {
			return d.ErrInvalid
		}
		receipt, err := receipts.FeatureReceipt(final, p.TenantID, p.UserID, "governed:"+op.ID)
		if err != nil && !errors.Is(err, d.ErrNotFound) {
			return err
		}
		if known != nil && receipt == nil {
			return d.ErrConflict
		}
		// Preserve authoritative success even when a previous worker observed no
		// receipt before another worker committed under this lock.
		status, reason := "failed", "feature_materialization_failed"
		if receipt != nil {
			if receipt.FeatureID != in.FeatureID || receipt.Version != in.FeatureVersion || receipt.SourceRefID != in.InputRef.String() {
				return d.ErrConflict
			}
			id, err := l.ParseRefID(receipt.OutputRefID)
			if err != nil {
				return err
			}
			if op.Status == "completed" && len(st.OutputRefs) == 1 && st.OutputRefs[0] == id {
				return nil
			}
			st.OutputRefs = []l.RefID{id}
			status, reason = "completed", ""
		} else {
			if op.Status == "completed" {
				return d.ErrConflict
			}
			if op.Status == "cancelled" || op.Status == "failed" {
				return nil
			}
			if op.Status == "cancel_requested" {
				status, reason = "cancelled", ""
			} else if errors.Is(cause, context.DeadlineExceeded) {
				reason = "deadline_exceeded"
			}
		}
		task, err := repo.Get(final, p.TenantID, st.DomainKind, st.DomainID)
		if err != nil {
			return err
		}
		if task.UserID != p.UserID || task.ParentID != op.ID {
			return d.ErrConflict
		}
		task.Status = status
		if err = repo.Update(final, task, task.Version); err != nil {
			return err
		}
		op.Status, st.Error = status, reason
		return nil
	})
	return e
}
