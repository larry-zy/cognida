package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

func (s *Service) WithKnowledgeLabels(repo d.KnowledgeLabelRepository) *Service {
	s.knowledgeLabels = repo
	return s
}
func (s *Service) ApplyKnowledgeLabels(ctx context.Context, p l.Principal, in d.ApplyKnowledgeLabels, key string) (*d.KnowledgeLabelApplication, error) {
	if s.knowledgeLabels == nil || in.ExpectedVersion < 1 || in.KnowledgeID == "" || key == "" || len(key) > 100 {
		return nil, d.ErrInvalid
	}
	if e := s.authorize(ctx, p, "admin"); e != nil {
		return nil, e
	}
	record, e := s.Get(ctx, p, "tagging_result", in.ResultID)
	if e != nil {
		return nil, e
	}
	if record.Version != in.ExpectedVersion || record.Status != "approved" {
		return nil, d.ErrConflict
	}
	var result d.TaggingResult
	if json.Unmarshal(record.Payload, &result) != nil || result.ReviewStatus != "approved" || result.ReviewedBy <= 0 {
		return nil, d.ErrConflict
	}
	taskRecord, e := s.Get(ctx, p, "tagging_task", record.ParentID)
	if e != nil {
		return nil, e
	}
	var task d.TaggingTask
	if json.Unmarshal(taskRecord.Payload, &task) != nil {
		return nil, d.ErrInvalid
	}
	ref, input, e := s.input(ctx, p, task.SourceRefID, false)
	if e != nil {
		return nil, e
	}
	// The target must be the actual knowledge source captured by the import. A
	// hand-written asset or an unrelated document cannot be relabelled by ID.
	if ref.Provenance.SourceType != "knowledge" || ref.Provenance.SourceID != in.KnowledgeID || input.Kind != "text" || d.LabelContentDigest(input.Text) != d.LabelContentDigest(result.Content) {
		return nil, d.ErrForbidden
	}
	labels := append([]string{}, result.Tags...)
	if result.Category != "" {
		labels = append(labels, result.Category)
	}
	unique := map[string]bool{}
	normalized := []string{}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" || utf8.RuneCountInString(label) > 128 {
			return nil, d.ErrInvalid
		}
		if !unique[label] {
			unique[label] = true
			normalized = append(normalized, label)
		}
	}
	if len(normalized) > 100 {
		return nil, d.ErrInvalid
	}
	sort.Strings(normalized)
	scheme, _ := json.Marshal(task.Config)
	return s.knowledgeLabels.ApplyApproved(ctx, p, d.PreparedKnowledgeLabels{ApplyKnowledgeLabels: in, SourceRefID: task.SourceRefID, Content: result.Content, SchemeDigest: d.LabelContentDigest(string(scheme)), ReviewDigest: d.LabelContentDigest(string(record.Payload)), IdempotencyKey: key, Labels: normalized})
}
func (s *Service) KnowledgeLabels(ctx context.Context, p l.Principal, id string) ([]string, error) {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return nil, e
	}
	if s.knowledgeLabels == nil {
		return nil, d.ErrInvalid
	}
	return s.knowledgeLabels.Labels(ctx, p, id)
}
