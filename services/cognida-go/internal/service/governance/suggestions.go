package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
)

type SuggestionDiscovery func(context.Context, l.Principal, []string, int) ([]d.Suggestion, error)

func normalizeSource(kind string) string {
	switch kind {
	case "coverage":
		return "uncovered_term"
	case "conversation":
		return "conversation_insight"
	case "evaluation":
		return "evaluation_feedback"
	}
	return kind
}
func validateProposal(in d.Suggestion) error {
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 256 || len(in.Content) > 200000 || len(in.EvidenceRefIDs) > 32 || !json.Valid(in.Proposal) || len(in.Proposal) > 256<<10 {
		return d.ErrInvalid
	}
	var obj map[string]json.RawMessage
	if e := json.Unmarshal(in.Proposal, &obj); e != nil {
		return d.ErrInvalid
	}
	switch in.Kind {
	case "term":
		var v struct {
			TargetType string   `json:"target_type"`
			TargetID   string   `json:"target_id"`
			Synonyms   []string `json:"synonyms"`
		}
		if json.Unmarshal(in.Proposal, &v) != nil || v.TargetType != "semantic_metric" || v.TargetID == "" || len(v.Synonyms) == 0 || len(v.Synonyms) > 50 {
			return d.ErrInvalid
		}
		for _, word := range v.Synonyms {
			if strings.TrimSpace(word) == "" || len(word) > 200 {
				return d.ErrInvalid
			}
		}
	case "knowledge":
		var v struct {
			KnowledgeBaseID string `json:"knowledge_base_id"`
			Title           string `json:"title"`
			Content         string `json:"content"`
		}
		if json.Unmarshal(in.Proposal, &v) != nil || v.KnowledgeBaseID == "" || strings.TrimSpace(v.Title) == "" || len(v.Title) > 255 || strings.TrimSpace(v.Content) == "" || len(v.Content) > 200000 {
			return d.ErrInvalid
		}
	case "experience":
		var v struct {
			AgentType string `json:"agent_type"`
			Question  string `json:"question"`
			Answer    string `json:"answer"`
		}
		if json.Unmarshal(in.Proposal, &v) != nil || v.AgentType == "" || len(v.AgentType) > 50 || strings.TrimSpace(v.Question) == "" || strings.TrimSpace(v.Answer) == "" || len(v.Question) > 20000 || len(v.Answer) > 200000 {
			return d.ErrInvalid
		}
	default:
		return d.ErrInvalid
	}
	return nil
}
func (s *Service) CreateSuggestion(ctx context.Context, p l.Principal, in d.Suggestion, key string) (*d.Record, error) {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return nil, e
	}
	if strings.TrimSpace(key) == "" || len(key) > 100 {
		return nil, d.ErrInvalid
	}
	in.Source.Type = normalizeSource(in.Source.Type)
	if in.Source.ID == "" || len(in.Source.ID) > 128 {
		return nil, d.ErrInvalid
	}
	switch in.Source.Type {
	case "uncovered_term", "conversation_insight", "evaluation_feedback":
	default:
		return nil, d.ErrInvalid
	}
	if e := validateProposal(in); e != nil {
		return nil, e
	}
	if s.ValidateSource == nil {
		return nil, fmt.Errorf("suggestion source validator unavailable")
	}
	if e := s.ValidateSource(ctx, p, in.Source); e != nil {
		return nil, e
	}
	for _, raw := range in.EvidenceRefIDs {
		id, e := l.ParseRefID(raw)
		if e != nil {
			return nil, d.ErrInvalid
		}
		if s.Assets == nil {
			return nil, d.ErrInvalid
		}
		if _, _, e = s.Assets.Resolve(ctx, p.TenantID, p.UserID, id); e != nil {
			return nil, e
		}
	}
	in.Meta = d.Meta{}
	in.Status = "pending"
	in.TargetRefID = ""
	in.Error = ""
	in.ReviewedBy = 0
	in.ReviewComment = ""
	record, e := s.create(ctx, p, "suggestion", "", "pending", key, in, nil)
	if e != nil {
		return nil, e
	}
	var saved d.Suggestion
	if e = json.Unmarshal(record.Payload, &saved); e != nil {
		return nil, e
	}
	if saved.Source != in.Source || saved.Kind != in.Kind || saved.Title != in.Title || saved.Content != in.Content || !equalJSON(saved.Proposal, in.Proposal) || strings.Join(saved.EvidenceRefIDs, "\x00") != strings.Join(in.EvidenceRefIDs, "\x00") {
		return nil, d.ErrConflict
	}
	return record, nil
}
func equalJSON(a, b json.RawMessage) bool { return d.JSONEqual(a, b) }
func (s *Service) ReviewSuggestion(ctx context.Context, p l.Principal, id, decision, comment string, expectedVersion int) (*d.Record, error) {
	if e := s.authorize(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if len(comment) > 4000 {
		return nil, d.ErrInvalid
	}
	switch decision {
	case "approve", "accepted", "approved":
		decision = "approve"
	case "reject", "rejected":
		decision = "reject"
	default:
		return nil, d.ErrInvalid
	}
	record, e := s.Repo.Get(ctx, p.TenantID, "suggestion", id)
	if e != nil {
		return nil, e
	}
	if record.Status == "applied" && decision == "approve" || record.Status == "rejected" && decision == "reject" {
		return record, nil
	}
	if record.Version != expectedVersion {
		return nil, d.ErrConflict
	}
	if record.Status != "pending" && record.Status != "failed" && record.Status != "applying" {
		return nil, d.ErrConflict
	}
	var in d.Suggestion
	if e = json.Unmarshal(record.Payload, &in); e != nil {
		return nil, e
	}
	in.Meta = record.Meta
	in.ReviewedBy = p.UserID
	in.ReviewComment = comment
	in.Error = ""
	if decision == "reject" {
		if record.Status == "applying" {
			return nil, d.ErrConflict
		}
		in.Status = "rejected"
		if e = s.update(ctx, record, in, "rejected", expectedVersion); e != nil {
			return nil, e
		}
		return record, nil
	}
	if e = validateProposal(in); e != nil {
		return nil, e
	}
	if s.ValidateSource == nil || s.Apply == nil {
		return nil, fmt.Errorf("suggestion application adapter unavailable")
	}
	if e = s.ValidateSource(ctx, p, in.Source); e != nil {
		return nil, e
	}
	for _, raw := range in.EvidenceRefIDs {
		id, e := l.ParseRefID(raw)
		if e != nil {
			return nil, d.ErrInvalid
		}
		if _, _, e = s.Assets.Resolve(ctx, p.TenantID, p.UserID, id); e != nil {
			return nil, e
		}
	}
	// Claim before effects. Every concrete adapter must also use this immutable
	// suggestion ID as its idempotency key, allowing recovery after process loss.
	in.Status = "applying"
	if e = s.update(ctx, record, in, "applying", expectedVersion); e != nil {
		return nil, e
	}
	in.Meta = record.Meta
	target, applyErr := s.Apply(ctx, p, in)
	if applyErr != nil {
		in.Status = "failed"
		in.Error = applyErr.Error()
		if e = s.update(ctx, record, in, "failed", record.Version); e != nil {
			return nil, e
		}
		return record, applyErr
	}
	if target == "" {
		in.Status = "failed"
		in.Error = "application produced no durable target"
		if e = s.update(ctx, record, in, "failed", record.Version); e != nil {
			return nil, e
		}
		return record, fmt.Errorf("%s", in.Error)
	}
	in.Status = "applied"
	in.TargetRefID = target
	if e = s.update(ctx, record, in, "applied", record.Version); e != nil {
		return nil, e
	}
	return record, nil
}
func (s *Service) DiscoverSuggestions(ctx context.Context, p l.Principal, types []string, limit int) ([]*d.Record, error) {
	if e := s.authorize(ctx, p, "read"); e != nil {
		return nil, e
	}
	if s.DiscoverSource == nil {
		return nil, fmt.Errorf("suggestion discovery unavailable")
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	if len(types) == 0 {
		types = []string{"uncovered_term", "conversation_insight", "evaluation_feedback"}
	}
	if len(types) > 3 {
		return nil, d.ErrInvalid
	}
	for i, kind := range types {
		types[i] = normalizeSource(kind)
		if types[i] != "uncovered_term" && types[i] != "conversation_insight" && types[i] != "evaluation_feedback" {
			return nil, d.ErrInvalid
		}
	}
	suggestions, e := s.DiscoverSource(ctx, p, types, limit)
	if e != nil {
		return nil, e
	}
	if len(suggestions) > limit {
		return nil, d.ErrInvalid
	}
	out := make([]*d.Record, 0, len(suggestions))
	for _, candidate := range suggestions {
		digest := sha256.Sum256(append([]byte(candidate.Source.Type+":"+candidate.Source.ID+":"+candidate.Kind+":"), candidate.Proposal...))
		key := "discover:" + hex.EncodeToString(digest[:])
		record, e := s.CreateSuggestion(ctx, p, candidate, key)
		if e != nil {
			return nil, e
		}
		out = append(out, record)
	}
	return out, nil
}
