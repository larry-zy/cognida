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
	if in.Name == "" || len(in.Name) > 128 || len(in.Labels) == 0 || len(in.Labels) > 100 {
		return nil, d.ErrInvalid
	}
	switch in.Type {
	case "classification", "named_entity", "relation":
	default:
		return nil, d.ErrInvalid
	}
	labels := map[string]bool{}
	for _, label := range in.Labels {
		if label.ID == "" || label.Name == "" || labels[label.ID] || len(label.Name) > 128 {
			return nil, d.ErrInvalid
		}
		labels[label.ID] = true
	}
	_, input, err := s.input(ctx, p, in.SourceRefID, false)
	if err != nil {
		return nil, err
	}
	if _, err = texts(input); err != nil {
		return nil, err
	}
	in.Meta = d.Meta{}
	in.Status = "building"
	record, err := s.create(ctx, p, "annotation_task", "", "building", key, in, nil)
	if err != nil {
		return nil, err
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
	contents, err := texts(input)
	if err != nil {
		return nil, err
	}
	for i, text := range contents {
		item := d.AnnotationItem{TaskID: record.ID, Content: text, Annotation: d.Annotation{Labels: []string{}, Entities: []d.Entity{}, Relations: []d.Relation{}}, Provenance: d.Provenance{Source: "unlabeled", Evidence: []string{}}, ReviewStatus: "pending"}
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
	labels := map[string]bool{}
	for _, v := range task.Labels {
		labels[v.ID] = true
	}
	if len(a.Labels) > 100 || len(a.Entities) > 1000 || len(a.Relations) > 1000 {
		return d.ErrInvalid
	}
	for _, label := range a.Labels {
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
	item.Provenance = d.Provenance{Source: "human", Method: "manual", Evidence: []string{}}
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
	if err = json.Unmarshal(taskRecord.Payload, &task); err != nil {
		return err
	}
	if task.Type != "classification" {
		return fmt.Errorf("%w: rule preannotation supports classification; use manual spans for entity/relation tasks", d.ErrInvalid)
	}
	cursor := ""
	for {
		page, err := s.List(ctx, p, d.Filter{Kind: "annotation_item", ParentID: id, Limit: 100, Cursor: cursor})
		if err != nil {
			return err
		}
		for _, record := range page.Items {
			var item d.AnnotationItem
			if err = json.Unmarshal(record.Payload, &item); err != nil {
				return err
			}
			if item.Provenance.Source == "human" || item.ReviewStatus == "approved" {
				continue
			}
			matches := []string{}
			evidence := []string{}
			for _, label := range task.Labels {
				if strings.Contains(strings.ToLower(item.Content), strings.ToLower(label.Name)) {
					matches = append(matches, label.ID)
					evidence = append(evidence, label.Name)
				}
			}
			confidence := 0.5
			if len(matches) == 0 {
				confidence = 0
			}
			item.Annotation.Labels = matches
			item.Provenance = d.Provenance{Source: "rules", Method: "literal_label_match", Confidence: &confidence, Evidence: evidence}
			item.ReviewStatus = "pending"
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
