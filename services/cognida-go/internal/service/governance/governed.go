package governance

import (
	d "cognida/internal/model/governance"
	"cognida/internal/model/job"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type GovernedInputValidator func(context.Context, l.Principal, []l.RefID) error

func (s *Service) SetGovernedInputValidator(fn GovernedInputValidator) { s.governedInputs = fn }
func (s *Service) governedRepo() (d.GovernedRepository, error) {
	r, ok := s.Repo.(d.GovernedRepository)
	if !ok {
		return nil, d.ErrInvalid
	}
	return r, nil
}
func (s *Service) LookupGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation) (*d.Record, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if e := in.Validate(); e != nil {
		return nil, e
	}
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	record, e := r.FindGoverned(ctx, p, in.OperationKey)
	if e != nil || record == nil {
		return record, e
	}
	var state d.GovernedExecution
	if json.Unmarshal(record.Payload, &state) != nil || !d.SameGoverned(state.Invocation, in) {
		return nil, d.ErrConflict
	}
	return record, nil
}
func (s *Service) ValidateGovernedInput(ctx context.Context, p l.Principal, id l.RefID) error {
	if s.governedInputs == nil {
		return d.ErrForbidden
	}
	if e := s.governedInputs(ctx, p, []l.RefID{id}); e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return d.ErrForbidden
	}
	_, input, e := s.input(ctx, p, id.String(), false)
	if e != nil {
		return e
	}
	contents, e := texts(input)
	if e != nil || len(contents) < 1 || len(contents) > 32 {
		return d.ErrInvalid
	}
	return nil
}
func (s *Service) SubmitGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation, cancel bool) (*d.Record, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if e := in.Validate(); e != nil {
		return nil, e
	}
	r, e := s.governedRepo()
	if e != nil {
		return nil, e
	}
	if !cancel {
		if s.Queue == nil || s.Assets == nil {
			return nil, d.ErrInvalid
		}
		if !in.Deadline.After(time.Now()) {
			return nil, context.DeadlineExceeded
		}
		if e = s.validateGovernedInvocationSource(ctx, p, in); e != nil {
			return nil, e
		}
	}
	record, e := r.SubmitGoverned(ctx, p, in, cancel)
	if e != nil {
		return nil, e
	}
	if !cancel && in.Profile != d.FeatureProfile && in.Profile != d.SFTProfile && (record.Status == "queued" || record.Status == "running") {
		record, e = s.bindGovernedStandard(ctx, p, record, in)
		if e != nil {
			return nil, e
		}
	}
	if !cancel && (record.Status == "queued" || record.Status == "running" || record.Status == "cancel_requested") {
		j, e := job.New(record.ID, "governance", jobScope{p.TenantID, p.UserID, d.GovernedKind, record.ID})
		if e != nil {
			return nil, e
		}
		if e = s.Queue.Enqueue(ctx, j); e != nil {
			return nil, e
		}
	}
	return record, nil
}
func (s *Service) runGoverned(ctx context.Context, record *d.Record) error {
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	var state d.GovernedExecution
	if json.Unmarshal(record.Payload, &state) != nil || state.Invocation.Validate() != nil {
		return d.ErrInvalid
	}
	if record.Status == "completed" || record.Status == "failed" || record.Status == "cancelled" {
		return nil
	}
	in := state.Invocation
	if in.Profile == d.SFTProfile {
		return s.runGovernedSFT(ctx, record, state)
	}
	if in.Profile == d.FeatureProfile {
		return s.runGovernedFeature(ctx, record, state)
	}
	var bindErr error
	record, bindErr = s.bindGovernedStandard(ctx, p, record, in)
	if bindErr != nil {
		return bindErr
	}
	if record.Status == "cancelled" || record.Status == "completed" || record.Status == "failed" {
		return nil
	}
	if json.Unmarshal(record.Payload, &state) != nil {
		return d.ErrInvalid
	}
	r, e := s.governedRepo()
	if e != nil {
		return e
	}
	end := record.CreatedAt.Add(time.Duration(in.TimeoutSeconds) * time.Second)
	if in.Deadline.Before(end) {
		end = in.Deadline
	}
	work, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	finishFailure := func(reason string) error {
		if errors.Is(work.Err(), context.DeadlineExceeded) {
			reason = "deadline_exceeded"
		}
		final, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_, e := r.UpdateGoverned(final, p, record.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
			if op.Status == "cancelled" || op.Status == "completed" || op.Status == "failed" {
				return nil
			}
			op.Status = "failed"
			st.Error = reason
			task, e := repo.Get(final, p.TenantID, st.DomainKind, st.DomainID)
			if e != nil {
				return e
			}
			task.Status = "failed"
			return repo.Update(final, task, task.Version)
		})
		return e
	}
	check := func() error {
		if e := work.Err(); e != nil {
			return e
		}
		current, e := r.FindGoverned(work, p, in.OperationKey)
		if e != nil {
			return e
		}
		if current == nil {
			return d.ErrNotFound
		}
		if current.Status == "cancelled" || current.Status == "completed" || current.Status == "failed" {
			return context.Canceled
		}
		return s.validateGovernedInvocationSource(work, p, in)
	}
	if e = check(); e != nil {
		if errors.Is(e, context.Canceled) {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return finishFailure("source_unavailable_or_deadline")
	}
	source, input, e := s.input(work, p, in.InputRef.String(), false)
	if e != nil {
		return finishFailure("source_unavailable")
	}
	taskDefinition := d.AnnotationTask{Type: in.AnnotationType(), Labels: state.StandardSnapshot.LabelSchema(), QAMapping: in.QAMapping, StandardSnapshot: state.StandardSnapshot}
	var contents []annotationSource
	if in.Profile == d.TaggingProfile {
		taskDefinition.Type = "classification"
	}
	contents, e = annotationSources(input, taskDefinition)
	if e != nil || len(contents) > 32 {
		return finishFailure("input_limit")
	}
	results := []map[string]any{}
	for i, content := range contents {
		if e = check(); e != nil {
			if errors.Is(e, context.Canceled) {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return finishFailure("source_unavailable_or_deadline")
		}
		itemKey := fmt.Sprintf("%s:item:%d", record.ID, i)
		// A stable per-item ID allows restart to retain human edits and skip completed compute.
		itemID := governedItemID(record.ID, i)
		kind := "tagging_result"
		if state.DomainKind == "annotation_task" {
			kind = "annotation_item"
		}
		existing, e := s.Repo.Get(work, p.TenantID, kind, itemID)
		if e != nil && !errors.Is(e, d.ErrNotFound) {
			return e
		}
		if existing != nil && (existing.UserID != p.UserID || existing.ParentID != state.DomainID) {
			return d.ErrConflict
		}
		if existing == nil {
			candidate, e := s.governedCandidate(work, state, content)
			if e != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return finishFailure("candidate_generation_failed")
			}
			if e = check(); e != nil {
				if errors.Is(e, context.Canceled) {
					return nil
				}
				return finishFailure("source_unavailable_or_deadline")
			}
			raw, e := json.Marshal(candidate)
			if e != nil {
				return e
			}
			_, e = r.UpdateGoverned(work, p, record.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
				if op.Status == "cancelled" || op.Status == "completed" || op.Status == "failed" {
					return context.Canceled
				}
				op.Status = "running"
				_, e := repo.Create(work, &d.Record{Meta: d.Meta{ID: itemID, TenantID: p.TenantID, UserID: p.UserID}, Kind: kind, ParentID: st.DomainID, Status: "pending", IdempotencyKey: itemKey, Payload: raw})
				return e
			})
			if errors.Is(e, context.Canceled) {
				return nil
			}
			if e != nil {
				return e
			}
		}
		results = append(results, map[string]any{"id": itemID, "kind": kind, "review_status_at_generation": "pending"})
	}
	if e = check(); e != nil {
		if errors.Is(e, context.Canceled) {
			return nil
		}
		return finishFailure("source_unavailable_or_deadline")
	}
	manifest := map[string]any{"schema_version": 1, "domain": "governance", "profile": in.Profile, "task_id": state.DomainID, "task_kind": state.DomainKind, "source_ref_id": in.InputRef.String(), "candidates": results, "review_required": true, "knowledge_labels_applied": false, "dataset_published": false}
	raw, _ := json.Marshal(manifest)
	policy := source.Policy
	policy.Approved = false
	output, e := s.Assets.ImportDerived(work, p, l.Result, "governance-governed:"+record.ID, []l.RefID{in.InputRef}, raw, policy)
	if e != nil {
		return e
	}
	_, e = r.UpdateGoverned(work, p, record.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
		if op.Status == "cancelled" || op.Status == "completed" || op.Status == "failed" {
			return nil
		}
		task, e := repo.Get(work, p.TenantID, st.DomainKind, st.DomainID)
		if e != nil {
			return e
		}
		var fields map[string]any
		if json.Unmarshal(task.Payload, &fields) != nil {
			return d.ErrInvalid
		}
		fields["status"] = "ready"
		fields["item_count"] = len(contents)
		task.Payload, _ = json.Marshal(fields)
		task.Status = "ready"
		if e = repo.Update(work, task, task.Version); e != nil {
			return e
		}
		op.Status = "completed"
		st.OutputRefs = []l.RefID{output.RefID}
		return nil
	})
	return e
}
func (s *Service) governedCandidate(ctx context.Context, state d.GovernedExecution, source annotationSource) (any, error) {
	in := state.Invocation
	if in.AnnotationType() != "" {
		task := d.AnnotationTask{Type: in.AnnotationType(), Labels: in.Schema(), QAMapping: in.QAMapping, StandardSnapshot: state.StandardSnapshot}
		if state.StandardSnapshot != nil {
			task.Labels = state.StandardSnapshot.LabelSchema()
		}
		annotation, p, e := candidateFor(task, source)
		if e != nil {
			return nil, e
		}
		return d.AnnotationItem{TaskID: state.DomainID, Content: source.Content, Annotation: annotation, Provenance: p, ReviewStatus: "pending"}, nil
	}
	out := d.TaggingResult{TaskID: state.DomainID, Content: source.Content, ReviewStatus: "pending", Keywords: []string{}}
	labels := in.Schema()
	if state.StandardSnapshot != nil {
		labels = state.StandardSnapshot.LabelSchema()
	}
	applyTagRules(&out, labels, d.TaggingConfig{EnableTags: true, EnableClassification: true, LabelSchema: labels})
	if validateStandardTagging(d.TaggingTask{StandardSnapshot: state.StandardSnapshot}, out, false) != nil {
		out.Tags = []string{}
		out.Category = ""
		out.Provenance.DecisionStatus = "uncertain"
		out.Provenance.Evidence = append(out.Provenance.Evidence, "standard constraints require manual resolution")
	}
	return out, nil
}
func (s *Service) validateGovernedInvocationSource(ctx context.Context, p l.Principal, in d.GovernedInvocation) error {
	if in.Profile == d.SFTProfile {
		return s.ValidateGovernedSFT(ctx, p, *in.SFT, in.InputRef)
	}
	if in.Profile == d.FeatureProfile {
		if err := s.ValidateGovernedFeature(ctx, p, in.FeatureID, in.FeatureVersion, in.InputRef); err != nil {
			return err
		}
		required, _, err := s.FeatureExecutionRequirementsForSource(ctx, p, in.FeatureID, in.FeatureVersion, in.InputRef.String())
		if err != nil {
			return err
		}
		if in.CallsReserved < required {
			return d.ErrInvalid
		}
		return nil
	}
	if e := s.ValidateGovernedInput(ctx, p, in.InputRef); e != nil {
		return e
	}
	if in.Profile == d.TaggingProfile {
		_, e := s.BindTaggingStandard(ctx, p, d.TaggingTask{StandardVersionID: in.StandardVersionID, SourceRefID: in.InputRef.String(), Config: d.TaggingConfig{EnableTags: true, EnableClassification: true, LabelSchema: in.Schema()}})
		if e != nil {
			return e
		}
	} else {
		_, e := s.BindAnnotationStandard(ctx, p, d.AnnotationTask{StandardVersionID: in.StandardVersionID, SourceRefID: in.InputRef.String(), Type: in.AnnotationType(), Labels: in.Schema(), QAMapping: in.QAMapping})
		if e != nil {
			return e
		}
	}
	if in.Profile == d.QAProfile {
		_, input, e := s.input(ctx, p, in.InputRef.String(), false)
		if e != nil {
			return e
		}
		_, e = annotationSources(input, d.AnnotationTask{Type: "qa", QAMapping: in.QAMapping})
		return e
	}
	return nil
}
func (s *Service) ValidateGovernedOrigin(ctx context.Context, p l.Principal, ref l.DataAssetRef) error {
	prefix := "governance-governed:"
	if p.Validate() != nil || ref.TenantID != p.TenantID || !strings.HasPrefix(ref.ProducerID, prefix) || ref.Kind != l.Result {
		return l.ErrForbidden
	}
	record, e := s.Repo.Get(ctx, p.TenantID, d.GovernedKind, strings.TrimPrefix(ref.ProducerID, prefix))
	if e != nil || record.UserID != p.UserID || record.Status != "completed" {
		return l.ErrForbidden
	}
	var state d.GovernedExecution
	if json.Unmarshal(record.Payload, &state) != nil || len(state.OutputRefs) != 1 || state.OutputRefs[0] != ref.RefID || len(ref.Parents) != 1 || ref.Parents[0] != state.Invocation.InputRef {
		return l.ErrForbidden
	}
	task, e := s.Repo.Get(ctx, p.TenantID, state.DomainKind, state.DomainID)
	if state.Invocation.Profile == d.SFTProfile {
		if e != nil || task.UserID != p.UserID || task.Kind != d.SFTDatasetKind || task.ParentID != record.ID {
			return l.ErrForbidden
		}
		return s.authorizeSFT(ctx, p, task)
	}
	if e != nil || task.UserID != p.UserID || task.ParentID != record.ID || task.Status != "ready" {
		return l.ErrForbidden
	}
	return nil
}

func governedItemID(id string, index int) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s/item/%d", id, index))).String()
}
