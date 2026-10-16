package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const labelCallKind = "label_model_call"

type LabelSuggestionRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	Guidelines      string `json:"guidelines"`
}
type labelCall struct {
	Guidelines     string          `json:"guidelines"`
	LabelSchema    []d.Label       `json:"label_schema"`
	SourceRefID    string          `json:"source_ref_id"`
	SourceDigest   string          `json:"source_digest"`
	Digest         string          `json:"digest"`
	Model          string          `json:"model"`
	Candidate      *LabelCandidate `json:"candidate,omitempty"`
	AppliedVersion int             `json:"applied_version,omitempty"`
}

func (s *Service) WithLabelSuggester(v LabelSuggester) *Service { s.labelSuggester = v; return s }
func (s *Service) SuggestLabel(ctx context.Context, p l.Principal, kind, taskID, itemID string, in LabelSuggestionRequest, key string) (*d.Record, error) {
	if kind != "annotation" && kind != "tagging" || !validFeatureKey(key) || in.ExpectedVersion < 1 || len([]rune(in.Guidelines)) > 4000 {
		return nil, d.ErrInvalid
	}
	task, e := s.Get(ctx, p, kind+"_task", taskID)
	if e != nil {
		return nil, e
	}
	itemKind := "annotation_item"
	if kind == "tagging" {
		itemKind = "tagging_result"
	}
	item, e := s.Get(ctx, p, itemKind, itemID)
	if e != nil {
		return nil, e
	}
	if item.ParentID != taskID {
		return nil, d.ErrNotFound
	}
	var provenance d.Provenance
	var review, source string
	modelIn := LabelSuggestionInput{Guidelines: in.Guidelines}
	if kind == "annotation" {
		var t d.AnnotationTask
		var v d.AnnotationItem
		if json.Unmarshal(task.Payload, &t) != nil || json.Unmarshal(item.Payload, &v) != nil {
			return nil, d.ErrInvalid
		}
		provenance = v.Provenance
		review = v.ReviewStatus
		source = t.SourceRefID
		modelIn.Kind = t.Type
		modelIn.Labels = t.Labels
		modelIn.Content = v.Content
		if t.Type == "qa" {
			var original d.QAPair
			if json.Unmarshal([]byte(v.Content), &original) != nil {
				return nil, d.ErrInvalid
			}
			modelIn.QA = &original
		}
	} else {
		var t d.TaggingTask
		var v d.TaggingResult
		if json.Unmarshal(task.Payload, &t) != nil || json.Unmarshal(item.Payload, &v) != nil {
			return nil, d.ErrInvalid
		}
		provenance = v.Provenance
		review = v.ReviewStatus
		source = t.SourceRefID
		modelIn.Kind = "tagging"
		modelIn.Content = v.Content
		modelIn.Labels, e = labelSchema(t.Config)
		if e != nil {
			return nil, e
		}
	}
	if s.labelSuggester == nil || strings.TrimSpace(s.labelSuggester.ModelID()) == "" {
		return nil, ErrLabelModelUnavailable
	}
	if len([]rune(modelIn.Content)) > 20000 {
		return nil, fmt.Errorf("%w: model suggestion supports at most 20000 characters per item", d.ErrInvalid)
	}
	ref, _, e := s.input(ctx, p, source, false)
	if e != nil {
		return nil, e
	}
	bound, _ := json.Marshal(struct {
		Task, Item, Source, Model string
		Revision                  int
		Input                     LabelSuggestionInput
	}{taskID, itemID, ref.Provenance.Digest, s.labelSuggester.ModelID(), in.ExpectedVersion, modelIn})
	call := labelCall{Guidelines: in.Guidelines, LabelSchema: modelIn.Labels, SourceRefID: source, SourceDigest: ref.Provenance.Digest, Digest: d.LabelContentDigest(string(bound)), Model: s.labelSuggester.ModelID()}
	raw, _ := json.Marshal(call)
	// Reservation precedes the paid call. A crashed/ambiguous call is never automatically replayed.
	op, e := s.Repo.Create(ctx, &d.Record{Meta: d.Meta{TenantID: p.TenantID, UserID: p.UserID}, Kind: labelCallKind, ParentID: itemID, Status: "reserved", IdempotencyKey: fmt.Sprintf("%d:%s", p.UserID, key), Payload: raw})
	if e != nil {
		return nil, e
	}
	var stored labelCall
	if op.UserID != p.UserID || op.ParentID != itemID || json.Unmarshal(op.Payload, &stored) != nil || stored.Digest != call.Digest {
		return nil, d.ErrConflict
	}
	call = stored
	if op.Status == "applied" {
		if item.Version != call.AppliedVersion || provenance.ModelCallID != op.ID {
			return nil, d.ErrConflict
		}
		return item, nil
	}
	if provenance.Source == "human" || review == "approved" || review == "rejected" || item.Version != in.ExpectedVersion {
		return nil, d.ErrConflict
	}
	if op.Status == "running" {
		return nil, ErrLabelModelUnknown
	}
	if op.Status == "failed" {
		return nil, ErrLabelModelUnavailable
	}
	if op.Status == "reserved" {
		op.Status = "running"
		if e = s.Repo.Update(ctx, op, op.Version); e != nil {
			return nil, e
		}
		candidate, err := s.labelSuggester.Suggest(ctx, modelIn)
		if err == nil {
			err = validateLabelCandidate(modelIn, candidate)
		}
		final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err != nil {
			op.Status = "failed"
			_ = s.Repo.Update(final, op, op.Version)
			return nil, err
		}
		call.Candidate = &candidate
		op.Payload, _ = json.Marshal(call)
		op.Status = "completed"
		if e = s.Repo.Update(final, op, op.Version); e != nil {
			return nil, e
		}
	}
	if call.Candidate == nil {
		return nil, d.ErrConflict
	}
	// Reauthorize after inference, then atomically CAS the pending item and durable application receipt.
	if _, _, e = s.input(ctx, p, source, false); e != nil {
		return nil, e
	}
	result := call.Candidate
	prov := d.Provenance{Source: "model", Method: "label-suggestion-v1", Model: call.Model, ModelCallID: op.ID, SchemeDigest: d.LabelContentDigest(string(mustLabelJSON(modelIn.Labels))), GuidelinesDigest: d.LabelContentDigest(in.Guidelines), DecisionStatus: result.DecisionStatus, Evidence: []string{result.Rationale}, EvidenceSpans: result.Evidence}
	var payload any
	if kind == "annotation" {
		var v d.AnnotationItem
		_ = json.Unmarshal(item.Payload, &v)
		v.Annotation = *result.Annotation
		v.Provenance = prov
		v.ReviewStatus = "pending"
		v.ReviewedBy = 0
		v.ReviewComment = ""
		payload = v
	} else {
		var v d.TaggingResult
		_ = json.Unmarshal(item.Payload, &v)
		v.Tags = result.Tags
		v.Category = result.Category
		v.Provenance = prov
		v.TagConfidence = nil
		v.CategoryConfidence = nil
		v.QualityScore = nil
		v.ReviewStatus = "pending"
		v.ReviewedBy = 0
		v.ReviewComment = ""
		payload = v
	}
	item.Payload, _ = json.Marshal(payload)
	item.Status = "pending"
	e = s.Repo.Transaction(ctx, func(tx d.Repository) error {
		if err := tx.Update(ctx, item, in.ExpectedVersion); err != nil {
			return err
		}
		call.AppliedVersion = item.Version
		op.Status = "applied"
		op.Payload, _ = json.Marshal(call)
		return tx.Update(ctx, op, op.Version)
	})
	if e != nil {
		return nil, e
	}
	return item, nil
}
func mustLabelJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
