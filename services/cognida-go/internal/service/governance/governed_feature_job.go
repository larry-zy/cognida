package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"time"
)

func (s *Service) FeatureExecutionRequirements(ctx context.Context, p l.Principal, id string, version int) (int, bool, error) {
	return s.FeatureExecutionRequirementsForSource(ctx, p, id, version, "")
}

func (s *Service) FeatureExecutionRequirementsForSource(ctx context.Context, p l.Principal, id string, version int, sourceRef string) (int, bool, error) {
	f, e := s.featureVersion(ctx, p, id, version)
	if e != nil {
		return 0, false, e
	}
	def, e := parseFeatureDefinition(f.Definition)
	if e != nil {
		return 0, false, e
	}
	if def.Kind != "embedding" {
		return 3, false, nil
	}
	if e = s.checkFeatureEmbedding(def); e != nil {
		return 0, true, e
	}
	if sourceRef == "" {
		sourceRef = f.SourceRefID
	}
	_, input, e := s.input(ctx, p, sourceRef, false)
	if e != nil {
		return 0, true, e
	}
	if input.Kind != "table" || len(input.Rows) < 1 || len(input.Rows) > 1000 {
		return 0, true, featureErr("governed_embedding_limit", "governed embedding supports at most 1000 rows per explicitly budgeted task")
	}
	return 2 + (len(input.Rows)+31)/32, true, nil
}
func (s *Service) runGovernedFeatureJob(ctx context.Context, record *d.Record, state d.GovernedExecution) error {
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	r, e := s.featureJobs()
	if e != nil {
		return e
	}
	operations, e := s.governedRepo()
	if e != nil {
		return e
	}
	key := "governed-job:" + record.ID
	j, e := r.FindFeatureJobByKey(ctx, p.TenantID, p.UserID, key)
	if e != nil {
		return e
	}
	current, e := operations.UpdateGoverned(ctx, p, record.ID, func(op *d.Record, _ *d.GovernedExecution, _ d.Repository) error {
		if op.Status == "queued" {
			op.Status = "running"
		}
		return nil
	})
	if e != nil {
		return e
	}
	if current.Status == "completed" || current.Status == "cancelled" || current.Status == "failed" {
		return nil
	}
	if current.Status == "cancel_requested" {
		if j != nil && (j.Status == "queued" || j.Status == "running") {
			_ = r.CancelFeatureJob(ctx, p.TenantID, p.UserID, j.ID, j.Version)
			_ = s.RunFeatureJob(ctx, p.TenantID, p.UserID, j.ID)
		}
		return s.finishGovernedFeatureJob(ctx, record, j, nil)
	}
	end := record.CreatedAt.Add(time.Duration(state.Invocation.TimeoutSeconds) * time.Second)
	if state.Invocation.Deadline.Before(end) {
		end = state.Invocation.Deadline
	}
	work, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	if j == nil {
		if e = s.ValidateGovernedFeature(work, p, state.Invocation.FeatureID, state.Invocation.FeatureVersion, state.Invocation.InputRef); e != nil {
			return s.finishGovernedFeatureJob(ctx, record, nil, e)
		}
		required, _, err := s.FeatureExecutionRequirementsForSource(work, p, state.Invocation.FeatureID, state.Invocation.FeatureVersion, state.Invocation.InputRef.String())
		if err != nil {
			return s.finishGovernedFeatureJob(ctx, record, nil, err)
		}
		if state.Invocation.CallsReserved < required {
			return s.finishGovernedFeatureJob(ctx, record, nil, d.ErrInvalid)
		}
		j, e = s.createFeatureJob(work, p, d.FeatureJobRequest{FeatureID: state.Invocation.FeatureID, FeatureVersion: state.Invocation.FeatureVersion, SourceRefID: state.Invocation.InputRef.String(), Mode: "full", PartitionSize: 32}, key, record.ID, &end)
		if e != nil {
			return s.finishGovernedFeatureJob(ctx, record, nil, e)
		}
	}
	_, e = operations.UpdateGoverned(ctx, p, record.ID, func(_ *d.Record, st *d.GovernedExecution, _ d.Repository) error { st.FeatureJobID = j.ID; return nil })
	if e != nil {
		return e
	}
	e = s.RunFeatureJob(work, p.TenantID, p.UserID, j.ID)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	final, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer c()
	j, lookup := r.GetFeatureJob(final, p.TenantID, p.UserID, j.ID)
	if lookup != nil {
		return lookup
	}
	if j.Status == "queued" || j.Status == "running" || j.Status == "cancel_requested" {
		return e
	}
	return s.finishGovernedFeatureJob(final, record, j, e)
}
func (s *Service) finishGovernedFeatureJob(ctx context.Context, record *d.Record, known *d.FeatureJob, cause error) error {
	operations, e := s.governedRepo()
	if e != nil {
		return e
	}
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	final, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer c()
	_, e = operations.UpdateGoverned(final, p, record.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
		jobs, ok := repo.(d.FeatureJobRepository)
		if !ok {
			return d.ErrInvalid
		}
		j, e := jobs.FindFeatureJobByKey(final, p.TenantID, p.UserID, "governed-job:"+op.ID)
		if e != nil {
			return e
		}
		if known != nil && j == nil {
			return d.ErrConflict
		}
		if op.Status == "completed" || op.Status == "failed" || op.Status == "cancelled" {
			return nil
		}
		if j != nil && (j.Status == "running" || j.Status == "queued" || j.Status == "cancel_requested") {
			return nil
		}
		status, reason := "failed", "feature_job_failed"
		st.OutputRefs = []l.RefID{}
		if j != nil {
			st.FeatureJobID = j.ID
			for _, part := range j.Partitions {
				if part.Status == "committed" {
					id, e := l.ParseRefID(part.OutputRefID)
					if e != nil {
						return e
					}
					st.OutputRefs = append(st.OutputRefs, id)
				}
			}
			switch j.Status {
			case "completed":
				status, reason = "completed", ""
			case "cancelled":
				status, reason = "cancelled", ""
			case "reconcile_required":
				reason = "model_call_reconciliation_required"
			}
		}
		if (j == nil || j.Status != "completed") && op.Status == "cancel_requested" {
			status, reason = "cancelled", ""
		}
		if errors.Is(cause, context.DeadlineExceeded) {
			reason = "deadline_exceeded"
		}
		task, e := repo.Get(final, p.TenantID, st.DomainKind, st.DomainID)
		if e != nil {
			return e
		}
		if task.UserID != p.UserID || task.ParentID != op.ID {
			return d.ErrConflict
		}
		task.Status = status
		if e = repo.Update(final, task, task.Version); e != nil {
			return e
		}
		op.Status = status
		st.Error = reason
		return nil
	})
	return e
}
func (s *Service) isEmbeddingFeature(ctx context.Context, p l.Principal, id string, version int) (bool, error) {
	f, e := s.featureVersion(ctx, p, id, version)
	if e != nil {
		return false, e
	}
	var def d.FeatureDefinition
	if json.Unmarshal(f.Definition, &def) != nil {
		return false, d.ErrInvalid
	}
	return def.Kind == "embedding", nil
}
