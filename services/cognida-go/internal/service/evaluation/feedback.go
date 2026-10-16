package evaluation

import (
	a "cognida/internal/model/agent"
	d "cognida/internal/model/evaluation"
	f "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strconv"
	"strings"
	"time"
)

func (s *Service) FeedbackFreeze(ctx context.Context, p l.Principal, id string, raw json.RawMessage) (json.RawMessage, int, error) {
	if s.snapshots == nil || s.evalQueue == nil {
		return nil, 0, f.ErrConflict
	}
	var c EvaluationTaskConfig
	if json.Unmarshal(raw, &c) != nil || c.Type.Normalize() != EvaluationTypeRAG || c.KnowledgeBaseID == "" || c.AgentID != "" {
		return nil, 0, f.ErrInvalid
	}
	if err := validateFeedbackConfig(c); err != nil {
		return nil, 0, err
	}
	snapshot, e := s.snapshots.Freeze(ctx, p, &c, id)
	if e != nil {
		return nil, 0, e
	}
	if len(snapshot.QAPairs) > 100 {
		return nil, 0, f.ErrInvalid
	}
	out, e := json.Marshal(snapshot)
	return out, len(snapshot.QAPairs), e
}
func validateFeedbackConfig(c EvaluationTaskConfig) error {
	// Keep the fixed comparison's paid work bounded and explicit. Strategy variants
	// are tested in their own workflow, never mixed into a knowledge comparison.
	for key := range c.Config {
		if key != "top_k" && key != "graders" && key != "applied_labels" {
			return f.ErrInvalid
		}
	}
	if raw, exists := c.Config["top_k"]; exists {
		top, ok := raw.(float64)
		if !ok || top < 1 || top > 20 || top != float64(int(top)) {
			return f.ErrInvalid
		}
	}
	if graders, ok := c.Config["graders"]; ok {
		items, ok := graders.([]any)
		if !ok || len(items) == 0 || len(items) > 2 {
			return f.ErrInvalid
		}
		for _, v := range items {
			switch v {
			case "rouge", "bleu", "retrieval", "semantic_similarity", "llm_judge":
			default:
				return f.ErrInvalid
			}
		}
	}
	if raw, exists := c.Config["applied_labels"]; exists {
		labels, ok := raw.([]any)
		if !ok || len(labels) > 30 {
			return f.ErrInvalid
		}
		for _, raw := range labels {
			label, ok := raw.(string)
			if !ok || strings.TrimSpace(label) == "" || len(label) > 256 {
				return f.ErrInvalid
			}
		}
	}
	return nil
}
func (s *Service) FeedbackValidate(ctx context.Context, p l.Principal, raw json.RawMessage) error {
	if s.snapshots == nil {
		return f.ErrConflict
	}
	var snap d.EvaluationSnapshot
	if json.Unmarshal(raw, &snap) != nil || snap.FeedbackView != nil || snap.Strategy != nil {
		return f.ErrInvalid
	}
	_, e := s.snapshots.ValidateRead(ctx, p, &snap)
	return e
}
func (s *Service) FeedbackStart(ctx context.Context, p l.Principal, key string, raw json.RawMessage, view f.View) (string, error) {
	if s.snapshots == nil || s.snapshots.FeedbackView == nil || s.evalQueue == nil {
		return "", f.ErrConflict
	}
	var snap d.EvaluationSnapshot
	if json.Unmarshal(raw, &snap) != nil || snap.FeedbackView != nil || snap.Strategy != nil {
		return "", f.ErrInvalid
	}
	snap.FeedbackView = &view
	ctx = a.WithUserID(a.WithTenantID(ctx, p.TenantID), p.UserID)
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("feedback-eval/%d/%d/%s", p.TenantID, p.UserID, key))).String()
	if old, e := s.taskRepo.FindByID(ctx, id); e == nil && old != nil {
		if old.TenantID != p.TenantID || old.UserID != p.UserID || f.Digest(json.RawMessage(old.Snapshot)) != f.Digest(snap) {
			return "", f.ErrConflict
		}
		if e = s.authorizeTaskRead(ctx, old); e != nil {
			return "", e
		}
		if old.Status == d.TaskStatusPending {
			return id, s.evalQueue.Enqueue(ctx, id)
		}
		return id, nil
	} else if e != nil && !errors.Is(e, d.ErrTaskNotFound) {
		return "", e
	}
	if !time.Now().Before(view.Deadline) {
		return "", f.ErrBudget
	}
	config, e := s.snapshots.Validate(ctx, p, &snap)
	if e != nil {
		return "", e
	}
	if config.Type.Normalize() != EvaluationTypeRAG || config.KnowledgeBaseID != view.KnowledgeBaseID {
		return "", f.ErrInvalid
	}
	task := d.NewEvaluationTask(id, p.TenantID, p.UserID, config.DatasetID, d.EvaluationType(config.Type), len(snap.QAPairs))
	task.KnowledgeBaseID = config.KnowledgeBaseID
	task.ModelID = config.ModelID
	task.Config, _ = json.Marshal(config.Config)
	task.Snapshot, _ = json.Marshal(snap)
	if e = s.taskRepo.Create(ctx, task); e != nil {
		old, read := s.taskRepo.FindByID(ctx, id)
		if read == nil && old != nil && f.Digest(json.RawMessage(old.Snapshot)) == f.Digest(snap) {
			if old.Status == d.TaskStatusPending {
				return id, s.evalQueue.Enqueue(ctx, id)
			}
			return id, nil
		}
		return "", e
	}
	// Enqueue failure leaves a pending receipt for the worker startup scanner.
	// It never deletes and recreates an identity with an unknown queue outcome.
	if e = s.evalQueue.Enqueue(ctx, id); e != nil {
		return id, e
	}
	return id, nil
}
func (s *Service) FeedbackCompare(ctx context.Context, p l.Principal, baseID, nextID string, raw json.RawMessage, baseView, nextView f.View, gate f.Gate) (*Comparison, error) {
	if baseID == "" || nextID == "" || baseID == nextID {
		return nil, f.ErrConflict
	}
	var fixed d.EvaluationSnapshot
	if json.Unmarshal(raw, &fixed) != nil {
		return nil, f.ErrInvalid
	}
	sets := [][]*d.EvaluationResult{}
	for i, id := range []string{baseID, nextID} {
		task, e := s.taskRepo.FindByID(ctx, id)
		if e != nil {
			return nil, e
		}
		if task == nil || task.TenantID != p.TenantID || task.UserID != p.UserID || task.DeletedAt != nil {
			return nil, f.ErrForbidden
		}
		if task.Status != d.TaskStatusCompleted {
			return nil, fmt.Errorf("%w: evaluation %s is %s", f.ErrConflict, id, task.Status)
		}
		ctx = a.WithUserID(a.WithTenantID(ctx, p.TenantID), p.UserID)
		if e = s.authorizeTaskRead(ctx, task); e != nil {
			return nil, e
		}
		var snap d.EvaluationSnapshot
		if json.Unmarshal(task.Snapshot, &snap) != nil || snap.FeedbackView == nil {
			return nil, f.ErrGate
		}
		want := baseView
		if i == 1 {
			want = nextView
		}
		// Compare the persisted JSON contract: omitempty round-trips an empty
		// labels slice as nil, which does not change the authorized view.
		if f.Digest(*snap.FeedbackView) != f.Digest(want) {
			return nil, f.ErrGate
		}
		snap.FeedbackView = nil
		if f.Digest(snap) != f.Digest(fixed) {
			return nil, fmt.Errorf("%w: dataset, model, prompt, graders or configuration changed", f.ErrGate)
		}
		rows, e := s.resultRepo.FindByTaskID(ctx, id)
		if e != nil {
			return nil, e
		}
		if len(rows) != len(snap.QAPairs) {
			return nil, f.ErrGate
		}
		expected := map[string]bool{}
		for _, pair := range snap.QAPairs {
			expected[resultKey(&d.EvaluationResult{Question: pair.Question, ReferenceAnswer: pair.ReferenceAnswer})] = true
		}
		for _, row := range rows {
			if !expected[resultKey(row)] || row.Scores["feedback_view_applied"] != 1 {
				return nil, f.ErrGate
			}
			delete(expected, resultKey(row))
		}
		if len(expected) > 0 {
			return nil, f.ErrGate
		}
		sets = append(sets, rows)
	}
	return compareResults(ComparisonRequest{BaselineID: baseID, CandidateID: nextID, Metric: gate.Metric, HigherIsBetter: gate.HigherIsBetter, MinPairs: gate.MinPairs, MaxRegression: gate.MaxRegression}, sets[0], sets[1])
}
func (s *Service) FeedbackEvidence(ctx context.Context, p l.Principal, taskID, resultID, question string) error {
	task, e := s.taskRepo.FindByID(ctx, taskID)
	if e != nil {
		return e
	}
	if task == nil || task.TenantID != p.TenantID || task.UserID != p.UserID || task.DeletedAt != nil {
		return f.ErrForbidden
	}
	ctx = a.WithUserID(a.WithTenantID(ctx, p.TenantID), p.UserID)
	if e = s.authorizeTaskRead(ctx, task); e != nil {
		return e
	}
	rows, e := s.resultRepo.FindByTaskID(ctx, taskID)
	if e != nil {
		return e
	}
	for _, row := range rows {
		if strconv.FormatInt(row.ID, 10) == resultID && row.Question == question {
			return nil
		}
	}
	return f.ErrForbidden
}

type feedbackEvaluationClaimer interface {
	ClaimFeedbackEvaluation(context.Context, string) (bool, error)
}
