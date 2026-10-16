package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"unicode/utf8"
)

func ValidateLabelStandard(in d.LabelStandardInput) error {
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > 128 || strings.TrimSpace(in.Guidelines) == "" || utf8.RuneCountInString(in.Guidelines) > 12000 {
		return d.ErrInvalid
	}
	c := in.Constraints
	if c.MaxLabels < 0 || c.MaxLabels > 100 || c.MaxEntities < 0 || c.MaxEntities > 1000 || c.MaxRelations < 0 || c.MaxRelations > 1000 {
		return d.ErrInvalid
	}
	switch in.TaskType {
	case "classification":
		if c.MaxLabels < 1 || c.MaxEntities != 0 || c.MaxRelations != 0 {
			return d.ErrInvalid
		}
	case "named_entity":
		if c.MaxLabels != 0 || c.MaxEntities < 1 || c.MaxRelations != 0 {
			return d.ErrInvalid
		}
	case "relation":
		if c.MaxLabels != 0 || c.MaxEntities < 1 || c.MaxRelations < 1 {
			return d.ErrInvalid
		}
	case "tagging":
		if len(in.Labels) > 30 || c.MaxLabels < 1 || c.MaxLabels > 30 || c.MaxRelations != 0 {
			return d.ErrInvalid
		}
	case "qa":
		if len(in.Labels) != 0 || c.MaxLabels != 0 || c.MaxEntities != 0 || c.MaxRelations != 0 || c.AllowEmpty {
			return d.ErrInvalid
		}
		return nil
	default:
		return d.ErrInvalid
	}
	if err := ValidateLabelSchema(in.LabelSchema()); err != nil {
		return err
	}
	for _, label := range in.Labels {
		if strings.TrimSpace(label.Definition) == "" || utf8.RuneCountInString(label.Definition) > 2000 || len(label.Examples) > 10 {
			return d.ErrInvalid
		}
		for _, items := range [][]string{label.IncludeCriteria, label.ExcludeCriteria, label.ExcludeKeywords} {
			if len(items) > 50 {
				return d.ErrInvalid
			}
			seen := map[string]bool{}
			for _, item := range items {
				if strings.TrimSpace(item) == "" || utf8.RuneCountInString(item) > 1000 || seen[item] {
					return d.ErrInvalid
				}
				seen[item] = true
			}
		}
		for _, ex := range label.Examples {
			if (ex.Decision != "include" && ex.Decision != "exclude") || strings.TrimSpace(ex.Text) == "" || utf8.RuneCountInString(ex.Text) > 4000 || strings.TrimSpace(ex.Explanation) == "" || utf8.RuneCountInString(ex.Explanation) > 1000 {
				return d.ErrInvalid
			}
		}
	}
	raw, _ := json.Marshal(in)
	if len(raw) > 256<<10 {
		return d.ErrInvalid
	}
	return nil
}
func (s *Service) standardRecord(ctx context.Context, p l.Principal, kind, id string) (*d.Record, error) {
	if err := s.authorize(ctx, p, "read"); err != nil {
		return nil, err
	}
	r, err := s.Repo.Get(ctx, p.TenantID, kind, id)
	if err != nil {
		return nil, err
	}
	if r.UserID != p.UserID {
		return nil, d.ErrNotFound
	}
	return r, nil
}
func (s *Service) CreateLabelStandard(ctx context.Context, p l.Principal, in d.LabelStandardInput, key string) (*d.Record, error) {
	if err := ValidateLabelStandard(in); err != nil {
		return nil, err
	}
	if !validFeatureKey(key) {
		return nil, d.ErrInvalid
	}
	r, err := s.create(ctx, p, d.LabelStandardKind, "", "draft", key, d.LabelStandardDraft{LabelStandardInput: in}, nil)
	if err != nil {
		return nil, err
	}
	var previous d.LabelStandardDraft
	if json.Unmarshal(r.Payload, &previous) != nil {
		return nil, d.ErrInvalid
	}
	if !d.JSONEqual(mustLabelJSON(in), mustLabelJSON(previous.LabelStandardInput)) {
		return nil, d.ErrConflict
	}
	return r, nil
}
func (s *Service) UpdateLabelStandard(ctx context.Context, p l.Principal, id string, in d.LabelStandardInput, expected int) (*d.Record, error) {
	if err := ValidateLabelStandard(in); err != nil {
		return nil, err
	}
	r, err := s.standardRecord(ctx, p, d.LabelStandardKind, id)
	if err != nil {
		return nil, err
	}
	var old d.LabelStandardDraft
	if json.Unmarshal(r.Payload, &old) != nil {
		return nil, d.ErrInvalid
	}
	old.LabelStandardInput = in
	if err = s.update(ctx, r, old, "draft", expected); err != nil {
		return nil, err
	}
	return r, nil
}
func (s *Service) GetLabelStandardVersion(ctx context.Context, p l.Principal, id string) (*d.LabelStandardVersion, error) {
	if id == "" || len(id) > 64 {
		return nil, d.ErrInvalid
	}
	r, err := s.standardRecord(ctx, p, d.LabelStandardVersionKind, id)
	if err != nil {
		return nil, err
	}
	var v d.LabelStandardVersion
	if r.Status != "published" || json.Unmarshal(r.Payload, &v) != nil || ValidateLabelStandard(v.LabelStandardInput) != nil || v.ContentDigest != d.LabelContentDigest(string(mustLabelJSON(v.LabelStandardInput))) {
		return nil, d.ErrInvalid
	}
	v.Meta = r.Meta
	return &v, nil
}
func (s *Service) PublishLabelStandard(ctx context.Context, p l.Principal, id string, expected int, key string) (*d.Record, error) {
	if err := s.authorize(ctx, p, "admin"); err != nil {
		return nil, err
	}
	if expected < 1 || !validFeatureKey(key) {
		return nil, d.ErrInvalid
	}
	versionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("standard:%d:%d:%s:%d", p.TenantID, p.UserID, id, expected))).String()
	var result *d.Record
	err := s.Repo.Transaction(ctx, func(tx d.Repository) error {
		boundKey := fmt.Sprintf("%d:%s", p.UserID, key)
		if existing, e := tx.Get(ctx, p.TenantID, d.LabelStandardVersionKind, versionID); e == nil {
			if existing.UserID != p.UserID || existing.IdempotencyKey != boundKey {
				return d.ErrConflict
			}
			result = existing
			return nil
		} else if !errors.Is(e, d.ErrNotFound) {
			return e
		}
		head, e := tx.Get(ctx, p.TenantID, d.LabelStandardKind, id)
		if e != nil {
			return e
		}
		if head.UserID != p.UserID {
			return d.ErrNotFound
		}
		if head.Version != expected {
			return d.ErrConflict
		}
		var v d.LabelStandardDraft
		if json.Unmarshal(head.Payload, &v) != nil {
			return d.ErrInvalid
		}
		if e = ValidateLabelStandard(v.LabelStandardInput); e != nil {
			return e
		}
		copy := d.LabelStandardVersion{LabelStandardInput: v.LabelStandardInput, StandardID: id, StandardRevision: expected, ContentDigest: d.LabelContentDigest(string(mustLabelJSON(v.LabelStandardInput)))}
		created, e := tx.Create(ctx, &d.Record{Meta: d.Meta{ID: versionID, TenantID: p.TenantID, UserID: p.UserID}, Kind: d.LabelStandardVersionKind, ParentID: id, Status: "published", IdempotencyKey: boundKey, Payload: mustLabelJSON(copy)})
		if e != nil {
			return e
		}
		if created.ID != versionID || !d.JSONEqual(created.Payload, mustLabelJSON(copy)) {
			return d.ErrConflict
		}
		v.LatestVersionID = created.ID
		head.Payload = mustLabelJSON(v)
		if e = tx.Update(ctx, head, expected); e != nil {
			return e
		}
		result = created
		return nil
	})
	return result, err
}
func (s *Service) BindAnnotationStandard(ctx context.Context, p l.Principal, in d.AnnotationTask) (d.AnnotationTask, error) {
	v, err := s.GetLabelStandardVersion(ctx, p, in.StandardVersionID)
	if err != nil {
		return in, err
	}
	if in.Type != v.TaskType || in.Type == "tagging" {
		return in, d.ErrInvalid
	}
	labels := v.LabelSchema()
	if len(in.Labels) > 0 && !d.JSONEqual(mustLabelJSON(in.Labels), mustLabelJSON(labels)) {
		return in, d.ErrInvalid
	}
	if _, _, err = s.preparedInput(ctx, p, in.SourceRefID); err != nil {
		return in, err
	}
	in.Labels = labels
	in.StandardSnapshot = &v.LabelStandardInput
	in.StandardDigest = v.ContentDigest
	return in, nil
}
func (s *Service) BindTaggingStandard(ctx context.Context, p l.Principal, in d.TaggingTask) (d.TaggingTask, error) {
	v, err := s.GetLabelStandardVersion(ctx, p, in.StandardVersionID)
	if err != nil {
		return in, err
	}
	if v.TaskType != "tagging" {
		return in, d.ErrInvalid
	}
	labels := v.LabelSchema()
	if len(in.Config.TagSchema) > 0 {
		return in, d.ErrInvalid
	}
	if len(in.Config.LabelSchema) > 0 && !d.JSONEqual(mustLabelJSON(in.Config.LabelSchema), mustLabelJSON(labels)) {
		return in, d.ErrInvalid
	}
	if _, _, err = s.preparedInput(ctx, p, in.SourceRefID); err != nil {
		return in, err
	}
	in.Config.LabelSchema = labels
	in.StandardSnapshot = &v.LabelStandardInput
	in.StandardDigest = v.ContentDigest
	return in, nil
}
func (s *Service) preparedInput(ctx context.Context, p l.Principal, id string) (l.DataAssetRef, l.Input, error) {
	if s.Assets == nil {
		return l.DataAssetRef{}, l.Input{}, d.ErrForbidden
	}
	r, input, err := s.input(ctx, p, id, false)
	if err != nil {
		return r, input, err
	}
	if !l.IsPreparedAsset(r) {
		return r, input, fmt.Errorf("%w: source must be a prepared asset", d.ErrInvalid)
	}
	return r, input, nil
}

// Prose definitions are guidance; the bounded rules below are deterministic.
// Draft uncertainty may be stored, but approval must honor AllowEmpty.
func validateStandardAnnotation(standard *d.LabelStandardInput, content string, a d.Annotation, approval bool) error {
	if standard == nil {
		return nil
	}
	c := standard.Constraints
	if len(a.Labels) > c.MaxLabels || len(a.Entities) > c.MaxEntities || len(a.Relations) > c.MaxRelations {
		return fmt.Errorf("%w: standard cardinality exceeded", d.ErrInvalid)
	}
	if approval && !c.AllowEmpty && a.QA == nil && len(a.Labels)+len(a.Entities)+len(a.Relations) == 0 {
		return fmt.Errorf("%w: standard requires an annotation", d.ErrInvalid)
	}
	chosen := map[string]bool{}
	for _, id := range a.Labels {
		if chosen[id] {
			return d.ErrInvalid
		}
		chosen[id] = true
	}
	for _, e := range a.Entities {
		chosen[e.Label] = true
	}
	for _, r := range a.Relations {
		chosen[r.Label] = true
	}
	for _, label := range standard.Labels {
		if chosen[label.ID] {
			for _, term := range label.ExcludeKeywords {
				if len(literalEvidence(content, term, label.ID, "excluded", true)) > 0 {
					return fmt.Errorf("%w: label exclusion rule matched", d.ErrInvalid)
				}
			}
		}
	}
	return nil
}
func validateStandardTagging(task d.TaggingTask, v d.TaggingResult, approval bool) error {
	if task.StandardSnapshot == nil {
		return nil
	}
	known := map[string]string{}
	for _, label := range task.StandardSnapshot.Labels {
		known[label.Name] = label.ID
	}
	a := d.Annotation{}
	seen := map[string]bool{}
	for _, tag := range v.Tags {
		if known[tag] == "" || seen[tag] {
			return d.ErrInvalid
		}
		seen[tag] = true
		a.Labels = append(a.Labels, known[tag])
	}
	if v.Category != "" {
		if known[v.Category] == "" {
			return d.ErrInvalid
		}
		if !seen[v.Category] {
			a.Labels = append(a.Labels, known[v.Category])
		}
	}
	if len(v.Entities) > 0 && string(v.Entities) != "null" {
		if json.Unmarshal(v.Entities, &a.Entities) != nil {
			return d.ErrInvalid
		}
		if validateAnnotation(d.AnnotationTask{Type: "named_entity", Labels: task.StandardSnapshot.LabelSchema()}, v.Content, d.Annotation{Entities: a.Entities}) != nil {
			return d.ErrInvalid
		}
	}
	return validateStandardAnnotation(task.StandardSnapshot, v.Content, a, approval)
}

// Bind under the operation fence before any candidate is committed. The command
// hash remains the caller's immutable ID binding; the derived snapshot is trusted.
func (s *Service) bindGovernedStandard(ctx context.Context, p l.Principal, record *d.Record, in d.GovernedInvocation) (*d.Record, error) {
	v, err := s.GetLabelStandardVersion(ctx, p, in.StandardVersionID)
	if err != nil {
		return nil, err
	}
	r, err := s.governedRepo()
	if err != nil {
		return nil, err
	}
	return r.UpdateGoverned(ctx, p, record.ID, func(op *d.Record, st *d.GovernedExecution, repo d.Repository) error {
		if op.Status != "queued" && op.Status != "running" {
			return nil
		}
		if st.StandardSnapshot != nil {
			if st.StandardDigest != v.ContentDigest {
				return d.ErrConflict
			}
			return nil
		}
		task, err := repo.Get(ctx, p.TenantID, st.DomainKind, st.DomainID)
		if err != nil {
			return err
		}
		if st.DomainKind == "annotation_task" {
			var t d.AnnotationTask
			if json.Unmarshal(task.Payload, &t) != nil {
				return d.ErrInvalid
			}
			t.StandardSnapshot = &v.LabelStandardInput
			t.StandardDigest = v.ContentDigest
			t.StandardVersionID = v.ID
			t.Labels = v.LabelSchema()
			task.Payload = mustLabelJSON(t)
		} else {
			var t d.TaggingTask
			if json.Unmarshal(task.Payload, &t) != nil {
				return d.ErrInvalid
			}
			t.StandardSnapshot = &v.LabelStandardInput
			t.StandardDigest = v.ContentDigest
			t.StandardVersionID = v.ID
			t.Config.LabelSchema = v.LabelSchema()
			task.Payload = mustLabelJSON(t)
		}
		if err = repo.Update(ctx, task, task.Version); err != nil {
			return err
		}
		st.StandardSnapshot = &v.LabelStandardInput
		st.StandardDigest = v.ContentDigest
		return nil
	})
}
