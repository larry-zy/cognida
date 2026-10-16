package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Service) CreateAnnotation(ctx context.Context, p l.Principal, in d.AnnotationTask, key string) (*d.Record, error) {
	var bindErr error
	in, bindErr = s.BindAnnotationStandard(ctx, p, in)
	if bindErr != nil {
		return nil, bindErr
	}
	if err := validateAnnotationTask(in); err != nil {
		return nil, err
	}
	_, input, err := s.input(ctx, p, in.SourceRefID, false)
	if err != nil {
		return nil, err
	}
	if _, err = annotationSources(input, in); err != nil {
		return nil, err
	}
	in.Meta = d.Meta{}
	in.Status = "building"
	record, err := s.create(ctx, p, "annotation_task", "", "building", key, in, nil)
	if err != nil {
		return nil, err
	}
	var previous d.AnnotationTask
	if json.Unmarshal(record.Payload, &previous) != nil {
		return nil, d.ErrInvalid
	}
	if !sameAnnotationDefinition(in, previous) {
		return nil, d.ErrConflict
	}
	if record.Status == "ready" {
		return record, nil
	}
	if err = json.Unmarshal(record.Payload, &in); err != nil {
		return nil, err
	}
	_, input, err = s.input(ctx, p, in.SourceRefID, false)
	if err != nil {
		return nil, err
	}
	contents, err := annotationSources(input, in)
	if err != nil {
		return nil, err
	}
	for i, source := range contents {
		item := d.AnnotationItem{TaskID: record.ID, Content: source.Content, Annotation: d.Annotation{Labels: []string{}, Entities: []d.Entity{}, Relations: []d.Relation{}}, Provenance: d.Provenance{Source: "unlabeled", DecisionStatus: "uncertain", Evidence: []string{}}, ReviewStatus: "pending"}
		if _, err = s.create(ctx, p, "annotation_item", record.ID, "pending", fmt.Sprintf("%s:%d", record.ID, i), item, nil); err != nil {
			return nil, err
		}
	}
	in.ItemCount = len(contents)
	in.Status = "ready"
	if err = s.update(ctx, record, in, "ready", record.Version); err != nil {
		return nil, err
	}
	return record, nil
}
func validateAnnotation(task d.AnnotationTask, content string, a d.Annotation) error {
	if err := validateStandardAnnotation(task.StandardSnapshot, content, a, false); err != nil {
		return err
	}
	if a.EvaluationProfile != nil && task.Type != "qa" {
		return d.ErrInvalid
	}
	if a.EvaluationProfile != nil {
		p := a.EvaluationProfile
		if len(p.SourceRefs) > 16 || len(p.RelevantChunkIDs) > 100 || len(p.ExpectedTools) > 100 || len(p.ExpectedSteps) > 100 || len(p.GoldSQL) > 20000 {
			return d.ErrInvalid
		}
		switch p.Kind {
		case "qa", "rag", "agent", "sql":
		default:
			return d.ErrInvalid
		}
	}
	if task.Type == "qa" {
		if a.QA == nil || strings.TrimSpace(a.QA.Question) == "" || strings.TrimSpace(a.QA.ReferenceAnswer) == "" || len([]rune(a.QA.Question)) > 20000 || len([]rune(a.QA.ReferenceAnswer)) > 100000 || len(a.Labels)+len(a.Entities)+len(a.Relations) > 0 {
			return d.ErrInvalid
		}
		return nil
	}
	if a.QA != nil {
		return d.ErrInvalid
	}

	labels := map[string]bool{}
	for _, v := range task.Labels {
		labels[v.ID] = true
	}
	if len(a.Labels) > 100 || len(a.Entities) > 1000 || len(a.Relations) > 1000 {
		return d.ErrInvalid
	}
	seenLabels := map[string]bool{}
	for _, label := range a.Labels {
		if seenLabels[label] {
			return d.ErrInvalid
		}
		seenLabels[label] = true
		if !labels[label] {
			return fmt.Errorf("%w: unknown label", d.ErrInvalid)
		}
	}
	chars := []rune(content)
	entities := map[string]bool{}
	for _, e := range a.Entities {
		if e.ID == "" || entities[e.ID] || e.Start < 0 || e.End <= e.Start || e.End > len(chars) || e.Text != string(chars[e.Start:e.End]) {
			return fmt.Errorf("%w: invalid entity span", d.ErrInvalid)
		}
		if task.Type == "named_entity" && !labels[e.Label] {
			return fmt.Errorf("%w: unknown entity label", d.ErrInvalid)
		}
		entities[e.ID] = true
	}
	for _, r := range a.Relations {
		if !entities[r.From] || !entities[r.To] || !labels[r.Label] {
			return fmt.Errorf("%w: invalid relation", d.ErrInvalid)
		}
	}
	if task.Type == "named_entity" && (len(a.Labels) > 0 || len(a.Relations) > 0) {
		return d.ErrInvalid
	}
	if task.Type == "relation" && len(a.Labels) > 0 {
		return d.ErrInvalid
	}
	if task.Type == "classification" && (len(a.Entities) > 0 || len(a.Relations) > 0) {
		return d.ErrInvalid
	}
	return nil
}
func (s *Service) UpdateAnnotation(ctx context.Context, p l.Principal, taskID, itemID string, a d.Annotation, expected int) (*d.Record, error) {
	taskRecord, err := s.Get(ctx, p, "annotation_task", taskID)
	if err != nil {
		return nil, err
	}
	itemRecord, err := s.Get(ctx, p, "annotation_item", itemID)
	if err != nil {
		return nil, err
	}
	if itemRecord.ParentID != taskID {
		return nil, d.ErrNotFound
	}
	var task d.AnnotationTask
	var item d.AnnotationItem
	if err = json.Unmarshal(taskRecord.Payload, &task); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(itemRecord.Payload, &item); err != nil {
		return nil, err
	}
	if err = validateAnnotation(task, item.Content, a); err != nil {
		return nil, err
	}
	item.Annotation = a
	item.Provenance = d.Provenance{Source: "human", Method: "manual", DecisionStatus: "manual", Evidence: []string{}}
	item.ReviewStatus = "pending"
	item.ReviewedBy = 0
	item.ReviewComment = ""
	if err = s.update(ctx, itemRecord, item, "pending", expected); err != nil {
		return nil, err
	}
	return itemRecord, nil
}
func (s *Service) ReviewAnnotation(ctx context.Context, p l.Principal, taskID, itemID, decision, comment string, expected int) (*d.Record, error) {
	if err := s.authorize(ctx, p, "admin"); err != nil {
		return nil, err
	}
	if decision != "approved" && decision != "rejected" {
		return nil, d.ErrInvalid
	}
	if len(comment) > 2000 {
		return nil, d.ErrInvalid
	}
	record, err := s.Get(ctx, p, "annotation_item", itemID)
	if err != nil {
		return nil, err
	}
	if record.ParentID != taskID {
		return nil, d.ErrNotFound
	}
	var item d.AnnotationItem
	if err = json.Unmarshal(record.Payload, &item); err != nil {
		return nil, err
	}
	if decision == "approved" {
		taskRecord, e := s.Get(ctx, p, "annotation_task", taskID)
		if e != nil {
			return nil, e
		}
		var task d.AnnotationTask
		if json.Unmarshal(taskRecord.Payload, &task) != nil {
			return nil, d.ErrInvalid
		}
		if e = validateAnnotation(task, item.Content, item.Annotation); e != nil {
			return nil, e
		}
		if e = validateStandardAnnotation(task.StandardSnapshot, item.Content, item.Annotation, true); e != nil {
			return nil, e
		}
	}
	item.ReviewStatus = decision
	item.ReviewedBy = p.UserID
	item.ReviewComment = comment
	if err = s.update(ctx, record, item, decision, expected); err != nil {
		return nil, err
	}
	return record, nil
}
func (s *Service) Preannotate(ctx context.Context, p l.Principal, id string) error {
	taskRecord, err := s.Get(ctx, p, "annotation_task", id)
	if err != nil {
		return err
	}
	var task d.AnnotationTask
	if json.Unmarshal(taskRecord.Payload, &task) != nil {
		return d.ErrInvalid
	}
	if task.Type == "relation" {
		return fmt.Errorf("%w: relation preannotation is not supported; annotate manually", d.ErrInvalid)
	}
	_, input, err := s.input(ctx, p, task.SourceRefID, false)
	if err != nil {
		return err
	}
	sources, err := annotationSources(input, task)
	if err != nil {
		return err
	}
	byContent := map[string]annotationSource{}
	for _, source := range sources {
		byContent[source.Content] = source
	}
	cursor := ""
	for {
		page, err := s.List(ctx, p, d.Filter{Kind: "annotation_item", ParentID: id, Limit: 100, Cursor: cursor})
		if err != nil {
			return err
		}
		for _, record := range page.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			var item d.AnnotationItem
			if json.Unmarshal(record.Payload, &item) != nil {
				return d.ErrInvalid
			}
			if item.Provenance.Source == "human" || item.ReviewStatus == "approved" || item.ReviewStatus == "rejected" {
				continue
			}
			source, ok := byContent[item.Content]
			if !ok {
				return d.ErrConflict
			}
			annotation, provenance, err := candidateFor(task, source)
			if err != nil {
				return err
			}
			if err = validateAnnotation(task, item.Content, annotation); err != nil {
				return err
			}
			item.Annotation = annotation
			item.Provenance = provenance
			item.ReviewStatus = "pending"
			item.ReviewedBy = 0
			item.ReviewComment = ""
			if err = s.update(ctx, &record, item, "pending", record.Version); err != nil {
				return err
			}
		}
		if !page.HasMore {
			return nil
		}
		cursor = page.NextCursor
	}
}

func sameAnnotationDefinition(a, b d.AnnotationTask) bool {
	a.Meta = d.Meta{}
	b.Meta = d.Meta{}
	a.Status = ""
	b.Status = ""
	a.ItemCount = 0
	b.ItemCount = 0
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return d.JSONEqual(x, y)
}
