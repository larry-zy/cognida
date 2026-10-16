package governance

import (
	"cognida/internal/infrastructure/job"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

func (s *Service) CreateTagging(ctx context.Context, p l.Principal, in d.TaggingTask, key string) (*d.Record, error) {
	var bindErr error
	in, bindErr = s.BindTaggingStandard(ctx, p, in)
	if bindErr != nil {
		return nil, bindErr
	}
	if in.Name == "" || len(in.Name) > 128 || len(in.Config.TagSchema) > 30 || len(in.Config.LabelSchema) > 30 {
		return nil, d.ErrInvalid
	}
	if _, err := labelSchema(in.Config); err != nil {
		return nil, err
	}
	_, input, err := s.input(ctx, p, in.SourceRefID, false)
	if err != nil {
		return nil, err
	}
	if _, err = texts(input); err != nil {
		return nil, err
	}
	in.Meta = d.Meta{}
	in.Status = "queued"
	in.Error = ""
	record, err := s.create(ctx, p, "tagging_task", "", "queued", key, in, nil)
	if err != nil {
		return nil, err
	}
	var previous d.TaggingTask
	if json.Unmarshal(record.Payload, &previous) != nil {
		return nil, d.ErrInvalid
	}
	a, b := in, previous
	a.Meta = d.Meta{}
	b.Meta = d.Meta{}
	a.Status = ""
	b.Status = ""
	a.Error = ""
	b.Error = ""
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if !d.JSONEqual(x, y) {
		return nil, d.ErrConflict
	}
	if record.Status == "completed" {
		return record, nil
	}
	j, err := job.New(record.ID, "governance", jobScope{p.TenantID, p.UserID, "tagging_task", record.ID})
	if err != nil {
		return nil, err
	}
	if err = s.Queue.Enqueue(ctx, j); err != nil {
		return nil, err
	}
	return record, nil
}
func (s *Service) runTagging(ctx context.Context, record *d.Record) error {
	var task d.TaggingTask
	if err := json.Unmarshal(record.Payload, &task); err != nil {
		return err
	}
	p := l.Principal{TenantID: record.TenantID, UserID: record.UserID}
	_, input, err := s.input(ctx, p, task.SourceRefID, false)
	if err != nil {
		return err
	}
	items, err := texts(input)
	if err != nil {
		return err
	}
	if s.Compute == nil {
		return fmt.Errorf("enrichment service unavailable")
	}
	features := []string{"keywords", "summary", "sensitivity"}
	if task.Config.EnableTags {
		features = append(features, "tags")
	}
	if task.Config.EnableSentiment {
		features = append(features, "sentiment")
	}
	if task.Config.EnableExtraction {
		features = append(features, "entities")
	}
	if task.Config.EnableClassification {
		features = append(features, "category")
	}
	scheme, e := labelSchema(task.Config)
	if e != nil {
		return e
	}
	labels := []map[string]any{}
	for _, label := range scheme {
		keywords := label.Keywords
		if len(keywords) == 0 {
			keywords = []string{label.Name}
		}
		labels = append(labels, map[string]any{"name": label.Name, "keywords": keywords})
	}
	// Completed item records are durable checkpoints, including any later human
	// edits. A timed-out batch resumes without recomputing completed items.
	existing := map[string]bool{}
	cursor := ""
	for {
		page, e := s.Repo.List(ctx, p.TenantID, d.Filter{Kind: "tagging_result", ParentID: record.ID, Cursor: cursor, Limit: 100})
		if e != nil {
			return e
		}
		for _, item := range page.Items {
			existing[item.IdempotencyKey] = true
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	for i, text := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if existing[fmt.Sprintf("%d:%s:%d", p.UserID, record.ID, i)] {
			continue
		}
		result, e := s.Compute.Call(ctx, "enrich", l.Input{Kind: "text", Text: text}, map[string]any{"method": "rules", "features": features, "labels": labels}, record.ID)
		if e != nil {
			return e
		}
		flat := map[string]any{}
		for key, rawValue := range result.Result {
			if field, ok := rawValue.(map[string]any); ok {
				flat[key] = field["value"]
			}
		}
		raw, e := json.Marshal(flat)
		if e != nil {
			return e
		}
		var out d.TaggingResult
		if e = json.Unmarshal(raw, &out); e != nil {
			return e
		}
		out.TaskID = record.ID
		out.Content = text
		out.Provenance = d.Provenance{Source: "rules", Method: result.Method, Evidence: []string{}}
		out.ReviewStatus = "pending"
		applyTagRules(&out, scheme, task.Config)
		if validateStandardTagging(task, out, false) != nil {
			out.Tags = []string{}
			out.Category = ""
			out.Entities = json.RawMessage("[]")
			out.Provenance.DecisionStatus = "uncertain"
			out.Provenance.Evidence = append(out.Provenance.Evidence, "standard constraints require manual resolution")
		}
		// Store the complete evidence-bearing compute envelope alongside the typed
		// UI fields, rather than invent a numeric quality or confidence score.
		data := map[string]any{}
		raw, _ = json.Marshal(out)
		_ = json.Unmarshal(raw, &data)
		data["evidence"] = result.Result
		if _, e = s.create(ctx, p, "tagging_result", record.ID, "pending", fmt.Sprintf("%s:%d", record.ID, i), data, nil); e != nil {
			return e
		}
	}
	task.Status = "completed"
	return s.update(ctx, record, task, "completed", record.Version)
}

func confidenceValue(value any) (float64, bool) {
	var number float64
	switch v := value.(type) {
	case float64:
		number = v
	case json.Number:
		var err error
		number, err = v.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return number, number >= 0 && number <= 1
}
func (s *Service) UpdateTagging(ctx context.Context, p l.Principal, taskID, id string, in d.TaggingResult, expected int) (*d.Record, error) {
	record, err := s.Get(ctx, p, "tagging_result", id)
	if err != nil {
		return nil, err
	}
	if record.ParentID != taskID {
		return nil, d.ErrNotFound
	}
	if len(in.Tags) > 100 || len(in.Keywords) > 100 || len(in.Summary) > 4000 || len(in.Category) > 128 || len(in.Entities) > 256<<10 {
		return nil, d.ErrInvalid
	}
	for _, values := range [][]string{in.Tags, in.Keywords} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 128 {
				return nil, d.ErrInvalid
			}
		}
	}
	switch in.Sentiment {
	case "", "positive", "negative", "neutral", "unknown":
	default:
		return nil, d.ErrInvalid
	}
	if len(in.Entities) > 0 {
		var entities []map[string]any
		if json.Unmarshal(in.Entities, &entities) != nil || len(entities) > 1000 {
			return nil, d.ErrInvalid
		}
	}
	var old d.TaggingResult
	if err = json.Unmarshal(record.Payload, &old); err != nil {
		return nil, err
	}
	in.Meta = d.Meta{}
	in.TaskID = taskID
	in.Content = old.Content
	parent, err := s.Get(ctx, p, "tagging_task", taskID)
	if err != nil {
		return nil, err
	}
	var task d.TaggingTask
	if json.Unmarshal(parent.Payload, &task) != nil {
		return nil, d.ErrInvalid
	}
	if err = validateStandardTagging(task, in, false); err != nil {
		return nil, err
	}
	in.QualityScore = nil
	in.CategoryConfidence = nil
	in.TagConfidence = nil
	in.Provenance = d.Provenance{Source: "human", Method: "manual", DecisionStatus: "manual", Evidence: []string{}}
	in.ReviewStatus = "pending"
	in.ReviewComment = ""
	in.ReviewedBy = 0
	if err = s.update(ctx, record, in, "pending", expected); err != nil {
		return nil, err
	}
	return record, nil
}

// ReviewTagging only changes review metadata. In particular the original
// evidence envelope, source provenance and confidence survive approval.
func (s *Service) ReviewTagging(ctx context.Context, p l.Principal, id, decision, comment string, expected int) (*d.Record, error) {
	if err := s.authorize(ctx, p, "admin"); err != nil {
		return nil, err
	}
	if (decision != "approved" && decision != "rejected") || len(comment) > 2000 {
		return nil, d.ErrInvalid
	}
	record, err := s.Get(ctx, p, "tagging_result", id)
	if err != nil {
		return nil, err
	}
	if decision == "approved" {
		parent, e := s.Get(ctx, p, "tagging_task", record.ParentID)
		if e != nil {
			return nil, e
		}
		var task d.TaggingTask
		var item d.TaggingResult
		if json.Unmarshal(parent.Payload, &task) != nil || json.Unmarshal(record.Payload, &item) != nil {
			return nil, d.ErrInvalid
		}
		if e = validateStandardTagging(task, item, true); e != nil {
			return nil, e
		}
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(record.Payload, &fields); err != nil {
		return nil, err
	}
	fields["review_status"], _ = json.Marshal(decision)
	fields["review_comment"], _ = json.Marshal(comment)
	fields["reviewed_by"], _ = json.Marshal(p.UserID)
	if err = s.update(ctx, record, fields, decision, expected); err != nil {
		return nil, err
	}
	return record, nil
}
