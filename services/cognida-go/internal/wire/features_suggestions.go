package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	exp "cognida/internal/model/agent/experience"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/governance"
	"cognida/internal/service/knowledge"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SuggestionHooks validates source ownership and applies approved changes to
// existing bounded contexts. It never creates a second knowledge/term store.
func NewSuggestionHooks(db *gorm.DB, processor knowledge.DocumentProcessorService) (governance.SourceValidator, governance.ApplySuggestion, governance.SuggestionDiscovery) {
	validate := func(ctx context.Context, p l.Principal, source d.SuggestionSource) error {
		var count int64
		var q *gorm.DB
		switch source.Type {
		case "uncovered_term":
			q = db.WithContext(ctx).Table("agent_semantic_coverage_logs").Where("tenant_id=? AND id=? AND uncovered<>''", p.TenantID, source.ID)
		case "conversation_insight":
			q = db.WithContext(ctx).Table("messages AS m").Joins("JOIN sessions AS s ON s.id=m.session_id").Where("s.tenant_id=? AND m.id=? AND s.deleted_at IS NULL AND m.deleted_at IS NULL AND m.role='assistant' AND m.is_completed=1", p.TenantID, source.ID)
			var owner int64
			result := q.Select("s.user_id").Scan(&owner)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return d.ErrNotFound
			}
			if owner != p.UserID && featureAuthorizer(db)(ctx, p, "admin") != nil {
				return d.ErrForbidden
			}
			return nil
		case "evaluation_feedback":
			q = db.WithContext(ctx).Table("evaluation_qa_results AS r").Joins("JOIN evaluation_tasks AS t ON t.id=r.task_id").Where("t.tenant_id=? AND r.id=? AND t.deleted_at IS NULL", p.TenantID, source.ID)
		default:
			return d.ErrInvalid
		}
		if e := q.Count(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			return d.ErrNotFound
		}
		return nil
	}
	apply := func(ctx context.Context, p l.Principal, suggestion d.Suggestion) (string, error) {
		if suggestion.ID == "" || suggestion.TenantID != p.TenantID {
			return "", d.ErrInvalid
		}
		switch suggestion.Kind {
		case "term":
			var in struct {
				TargetID string   `json:"target_id"`
				Synonyms []string `json:"synonyms"`
			}
			if e := json.Unmarshal(suggestion.Proposal, &in); e != nil {
				return "", e
			}
			e := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var metric mysql.SemanticMetricModel
				if e := tx.Table("agent_semantic_metrics AS m").Select("m.*").Joins("JOIN agent_semantic_models AS s ON s.id=m.model_id").Where("s.tenant_id=? AND s.deleted_at IS NULL AND m.id=?", p.TenantID, in.TargetID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&metric).Error; e != nil {
					if errors.Is(e, gorm.ErrRecordNotFound) {
						return d.ErrNotFound
					}
					return e
				}
				var synonyms []string
				if len(metric.Synonyms) > 0 {
					if e := json.Unmarshal(metric.Synonyms, &synonyms); e != nil {
						return e
					}
				}
				seen := map[string]bool{}
				for _, word := range synonyms {
					seen[word] = true
				}
				changed := false
				for _, word := range in.Synonyms {
					word = strings.TrimSpace(word)
					if !seen[word] {
						synonyms = append(synonyms, word)
						seen[word] = true
						changed = true
					}
				}
				if !changed {
					return nil
				}
				body, e := json.Marshal(synonyms)
				if e != nil {
					return e
				}
				if e = tx.Model(&mysql.SemanticMetricModel{}).Where("id=?", metric.ID).Updates(map[string]any{"synonyms": string(body), "updated_at": time.Now().UTC()}).Error; e != nil {
					return e
				}
				return tx.Model(&mysql.SemanticModelModel{}).Where("id=? AND tenant_id=?", metric.ModelID, p.TenantID).Updates(map[string]any{"version": gorm.Expr("version+1"), "updated_at": time.Now().UTC()}).Error
			})
			return "semantic_metric:" + in.TargetID, e
		case "knowledge":
			if processor == nil {
				return "", fmt.Errorf("document processor unavailable")
			}
			var in struct {
				KnowledgeBaseID string `json:"knowledge_base_id"`
				Title           string `json:"title"`
				Content         string `json:"content"`
			}
			if e := json.Unmarshal(suggestion.Proposal, &in); e != nil {
				return "", e
			}
			result, e := processor.ProcessDocument(ctx, p.TenantID, p.UserID, &knowledge.ProcessDocumentRequest{IdempotencyKey: "suggestion:" + suggestion.ID, KnowledgeBaseID: in.KnowledgeBaseID, Title: in.Title, FileName: "suggestion-" + suggestion.ID + ".txt", FileType: "txt", Content: []byte(in.Content)})
			if e != nil {
				return "", e
			}
			if result == nil || result.ParseStatus != "completed" || result.KnowledgeID == "" {
				return "", fmt.Errorf("suggestion document did not complete processing")
			}
			return "knowledge:" + result.KnowledgeID, nil
		case "experience":
			var in struct {
				AgentType string `json:"agent_type"`
				Question  string `json:"question"`
				Answer    string `json:"answer"`
			}
			if e := json.Unmarshal(suggestion.Proposal, &in); e != nil {
				return "", e
			}
			sessionID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("reviewed-experience/%d/%s", p.TenantID, suggestion.ID))).String()
			entry := &exp.Experience{SessionID: sessionID, TenantID: p.TenantID, UserID: p.UserID, AgentType: in.AgentType, Title: suggestion.Title, Problem: in.Question, Solution: in.Answer, Tags: []string{"reviewed-suggestion", "source:" + suggestion.Source.Type + ":" + suggestion.Source.ID}, Tools: []string{}, Confidence: 0, Status: exp.StatusDone}
			if e := mysql.NewExperienceRepository(db).Save(ctx, entry); e != nil {
				return "", e
			}
			var saved mysql.AgentExperienceModel
			if e := db.WithContext(ctx).Where("tenant_id=? AND session_id=?", p.TenantID, sessionID).Take(&saved).Error; e != nil {
				return "", e
			}
			return "experience:" + strconv.FormatInt(saved.ID, 10), nil
		default:
			return "", d.ErrInvalid
		}
	}
	discover := func(ctx context.Context, p l.Principal, kinds []string, limit int) ([]d.Suggestion, error) {
		out := []d.Suggestion{}
		for _, kind := range kinds {
			remaining := limit - len(out)
			if remaining <= 0 {
				break
			}
			switch kind {
			case "uncovered_term":
				var logs []mysql.SemanticCoverageLogModel
				if e := db.WithContext(ctx).Where("tenant_id=? AND uncovered<>''", p.TenantID).Order("id DESC").Limit(remaining).Find(&logs).Error; e != nil {
					return nil, e
				}
				for _, entry := range logs {
					if len(out) >= limit {
						break
					}
					var metrics []mysql.SemanticMetricModel
					if e := db.WithContext(ctx).Table("agent_semantic_metrics AS m").Select("m.*").Joins("JOIN agent_semantic_models AS s ON s.id=m.model_id").Where("s.tenant_id=? AND s.name=? AND s.deleted_at IS NULL", p.TenantID, entry.Model).Order("m.id").Limit(100).Find(&metrics).Error; e != nil {
						return nil, e
					}
					for _, word := range strings.Split(entry.Uncovered, ",") {
						word = strings.TrimSpace(word)
						if word == "" || len(word) > 200 {
							continue
						}
						for _, metric := range metrics { // Propose only a lexical match; never guess an unrelated metric.
							if !strings.Contains(strings.ToLower(metric.Name+" "+metric.Description), strings.ToLower(word)) && !strings.Contains(strings.ToLower(word), strings.ToLower(metric.Name)) {
								continue
							}
							proposal, _ := json.Marshal(map[string]any{"target_type": "semantic_metric", "target_id": metric.ID, "synonyms": []string{word}})
							out = append(out, d.Suggestion{Source: d.SuggestionSource{Type: kind, ID: strconv.FormatInt(entry.ID, 10)}, Kind: "term", Title: "Review uncovered term: " + word, Content: "A real coverage failure has a lexical match to metric " + metric.Name + ". A domain owner must verify the semantic equivalence before approval.", EvidenceRefIDs: []string{}, Proposal: proposal})
							break
						}
						if len(out) >= limit {
							break
						}
					}
				}
			case "conversation_insight":
				var rows []struct {
					ID, SessionID, Content, AgentType string
					CreatedAt                         time.Time
				}
				if e := db.WithContext(ctx).Table("messages AS m").Select("m.id,m.session_id,m.content,m.created_at,s.agent_type").Joins("JOIN sessions AS s ON s.id=m.session_id").Where("s.tenant_id=? AND s.user_id=? AND s.deleted_at IS NULL AND m.deleted_at IS NULL AND m.role='assistant' AND m.is_completed=1 AND m.content<>''", p.TenantID, p.UserID).Order("m.created_at DESC").Limit(remaining).Scan(&rows).Error; e != nil {
					return nil, e
				}
				for _, row := range rows {
					var question mysql.MessageModel
					if e := db.WithContext(ctx).Where("session_id=? AND role='user' AND deleted_at IS NULL AND created_at<=?", row.SessionID, row.CreatedAt).Order("created_at DESC").Take(&question).Error; e != nil {
						if errors.Is(e, gorm.ErrRecordNotFound) {
							continue
						}
						return nil, e
					}
					if len(question.Content) > 20000 || len(row.Content) > 200000 {
						continue
					}
					agentType := row.AgentType
					if agentType == "" {
						agentType = "data_analysis"
					}
					proposal, _ := json.Marshal(map[string]any{"agent_type": agentType, "question": question.Content, "answer": row.Content})
					out = append(out, d.Suggestion{Source: d.SuggestionSource{Type: kind, ID: row.ID}, Kind: "experience", Title: "Review reusable conversation answer", Content: "Candidate from a completed conversation. The answer is unverified until an administrator reviews it.", EvidenceRefIDs: []string{}, Proposal: proposal})
				}
			case "evaluation_feedback":
				var rows []mysql.EvaluationResultModel
				if e := db.WithContext(ctx).Table("evaluation_qa_results AS r").Select("r.*").Joins("JOIN evaluation_tasks AS t ON t.id=r.task_id").Where("t.tenant_id=? AND t.deleted_at IS NULL AND r.reference_answer<>'' AND (r.success=0 OR r.llm_score<0.7 OR r.rouge_l<0.5)", p.TenantID).Order("r.id DESC").Limit(remaining).Scan(&rows).Error; e != nil {
					return nil, e
				}
				for _, row := range rows {
					if len(row.Question) > 20000 || len(row.ReferenceAnswer) > 200000 {
						continue
					}
					proposal, _ := json.Marshal(map[string]any{"agent_type": "data_analysis", "question": row.Question, "answer": row.ReferenceAnswer})
					out = append(out, d.Suggestion{Source: d.SuggestionSource{Type: kind, ID: strconv.FormatInt(row.ID, 10)}, Kind: "experience", Title: "Review evaluation reference answer", Content: "A real evaluation failure supplies the labelled reference answer; adoption still requires administrator review.", EvidenceRefIDs: []string{}, Proposal: proposal})
				}
			default:
				return nil, d.ErrInvalid
			}
		}
		return out, nil
	}
	return validate, apply, discover
}
