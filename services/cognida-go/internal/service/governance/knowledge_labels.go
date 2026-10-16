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
	record, task, result, target, e := s.knowledgeLabelTarget(ctx, p, in.ResultID, in.ExpectedVersion, in.PublicationRefID)
	if e != nil {
		return nil, e
	}
	if target.KnowledgeID != in.KnowledgeID {
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
	return s.knowledgeLabels.ApplyApproved(ctx, p, d.PreparedKnowledgeLabels{ApplyKnowledgeLabels: in, SourceRefID: task.SourceRefID, Content: result.Content, SchemeDigest: d.LabelContentDigest(string(scheme)), ReviewDigest: d.LabelContentDigest(string(record.Payload)), IdempotencyKey: key, Labels: normalized, PublicationContentDigest: target.PublicationContentDigest, PublicationChunkDigest: target.PublicationChunkDigest})
}

func (s *Service) KnowledgeLabelTarget(ctx context.Context, p l.Principal, resultID string, expected int) (*d.KnowledgeLabelTarget, error) {
	_, _, _, target, err := s.knowledgeLabelTarget(ctx, p, resultID, expected, "")
	return target, err
}

// KnowledgeLabelPublicationTarget resolves only an explicitly selected published
// asset. An empty publication keeps the original knowledge-ancestor contract.
func (s *Service) KnowledgeLabelPublicationTarget(ctx context.Context, p l.Principal, resultID string, expected int, publication string) (*d.KnowledgeLabelTarget, error) {
	_, _, _, target, err := s.knowledgeLabelTarget(ctx, p, resultID, expected, publication)
	return target, err
}

type knowledgePublicationResolver interface {
	ResolveKnowledgePublication(context.Context, l.Principal, l.RefID, l.RefID, string) (l.KnowledgePublication, error)
}

func (s *Service) knowledgeLabelTarget(ctx context.Context, p l.Principal, resultID string, expected int, publication string) (*d.Record, d.TaggingTask, d.TaggingResult, *d.KnowledgeLabelTarget, error) {
	var task d.TaggingTask
	var result d.TaggingResult
	fail := func(err error) (*d.Record, d.TaggingTask, d.TaggingResult, *d.KnowledgeLabelTarget, error) {
		return nil, task, result, nil, err
	}
	if resultID == "" || expected < 1 {
		return fail(d.ErrInvalid)
	}
	record, err := s.Get(ctx, p, "tagging_result", resultID)
	if err != nil {
		return fail(err)
	}
	if record.Version != expected || record.Status != "approved" || json.Unmarshal(record.Payload, &result) != nil || result.ReviewStatus != "approved" || result.ReviewedBy <= 0 {
		return fail(d.ErrConflict)
	}
	taskRecord, err := s.Get(ctx, p, "tagging_task", record.ParentID)
	if err != nil {
		return fail(err)
	}
	if json.Unmarshal(taskRecord.Payload, &task) != nil {
		return fail(d.ErrInvalid)
	}
	var knowledgeID string
	var publicationProof l.KnowledgePublication
	if publication == "" {
		knowledgeID, err = s.knowledgeLabelSource(ctx, p, task.SourceRefID, result.Content)
	} else {
		publishedRef, parseErr := l.ParseRefID(publication)
		sourceRef, sourceErr := l.ParseRefID(task.SourceRefID)
		if parseErr != nil || sourceErr != nil {
			return fail(d.ErrInvalid)
		}
		resolver, ok := s.Assets.(knowledgePublicationResolver)
		if !ok {
			return fail(d.ErrForbidden)
		}
		publicationProof, err = resolver.ResolveKnowledgePublication(ctx, p, publishedRef, sourceRef, result.Content)
		err = recordSourceError(err)
		knowledgeID = publicationProof.KnowledgeID
	}
	if err != nil {
		return fail(err)
	}
	target := &d.KnowledgeLabelTarget{ResultID: resultID, ReviewVersion: expected, KnowledgeID: knowledgeID, SourceRefID: task.SourceRefID, ContentDigest: d.LabelContentDigest(result.Content), PublicationRefID: publication, PublicationContentDigest: publicationProof.ContentDigest, PublicationChunkDigest: publicationProof.ChunkDigest}
	return record, task, result, target, nil
}

// Resolve every parent through the authorized asset service. A prepared or
// cleaned snapshot can label its single original document only when its final
// text remains byte-for-byte identical. Matching text alone cannot manufacture
// a provenance edge to an unrelated knowledge document.
func (s *Service) knowledgeLabelSource(ctx context.Context, p l.Principal, sourceRef, content string) (string, error) {
	visiting, visited := map[string]bool{}, map[string]bool{}
	knowledgeID := ""
	digest := d.LabelContentDigest(content)
	var walk func(string, int) error
	walk = func(id string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 16 || visiting[id] || len(visited) >= 64 && !visited[id] {
			return d.ErrForbidden
		}
		if visited[id] {
			return nil
		}
		visiting[id], visited[id] = true, true
		defer delete(visiting, id)
		refID, err := l.ParseRefID(id)
		if err != nil || s.Assets == nil {
			return d.ErrForbidden
		}
		ref, raw, err := s.Assets.Resolve(ctx, p.TenantID, p.UserID, refID)
		if err != nil {
			return recordSourceError(err)
		}
		// Ancillary report parents still require authorization and traversal, but
		// their report payload is not a text/table Input. Decode only the source
		// being labelled and the actual knowledge snapshots used for equality.
		var input l.Input
		if depth == 0 || ref.Provenance.SourceType == "knowledge" {
			if json.Unmarshal(raw, &input) != nil || input.Kind != "text" || input.Validate() != nil {
				return d.ErrForbidden
			}
		}
		if depth == 0 && (input.Kind != "text" || d.LabelContentDigest(input.Text) != digest) {
			return d.ErrForbidden
		}
		if ref.Provenance.SourceType == "knowledge" {
			if ref.Provenance.SourceID == "" || input.Kind != "text" || d.LabelContentDigest(input.Text) != digest || knowledgeID != "" && knowledgeID != ref.Provenance.SourceID {
				return d.ErrForbidden
			}
			knowledgeID = ref.Provenance.SourceID
			return nil
		}
		if len(ref.Parents) > 16 {
			return d.ErrForbidden
		}
		for _, parent := range ref.Parents {
			if err := walk(parent.String(), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(sourceRef, 0); err != nil {
		return "", err
	}
	if knowledgeID == "" {
		return "", d.ErrForbidden
	}
	return knowledgeID, nil
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
