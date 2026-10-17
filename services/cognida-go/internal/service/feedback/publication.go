package feedback

import (
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	control "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"time"
)

func (s *Service) CreateCandidate(ctx context.Context, p l.Principal, id string, expected int, in d.CandidateInput, key string) (*d.Publication, error) {
	c, e := s.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if !validKey(key) || in.SourceRef.Validate() != nil || in.RoundID == "" {
		return nil, d.ErrInvalid
	}
	pubID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("feedback-publication:"+id+":"+key)).String()
	if old, err := s.Repo.GetPublication(ctx, p, pubID); err == nil {
		if old.CaseID != id || d.Digest(old.CandidateInput) != d.Digest(in) {
			return nil, d.ErrConflict
		}
		_, v, _, e := s.candidate(ctx, p, pubID)
		return v, e
	} else if !errors.Is(err, d.ErrNotFound) {
		return nil, err
	}
	if c.Version != expected {
		return nil, d.ErrConflict
	}
	if e = mutable(c); e != nil {
		return nil, e
	}
	var taskID string
	for _, r := range c.Rounds {
		if r.ID == in.RoundID {
			taskID = r.TaskID
		}
	}
	if taskID == "" || s.Tasks == nil || s.Runtime == nil {
		return nil, d.ErrInvalid
	}
	task, e := s.Tasks.Get(ctx, scope(p), taskID)
	if e != nil {
		return nil, e
	}
	if task.Status != tc.StatusCompleted {
		return nil, d.ErrConflict
	}
	// Replanning is a separate authorization flow. A fixed feedback round may
	// only consume the exact plan reserved in its original bounded budget.
	executed, e := s.Tasks.GetPlan(ctx, scope(p), taskID, task.ExecutionPlanVersion)
	if e != nil {
		return nil, e
	}
	var reserved control.PlanSpec
	for _, r := range c.Rounds {
		if r.ID == in.RoundID {
			if json.Unmarshal(r.Plan, &reserved) != nil {
				return nil, d.ErrInvalid
			}
		}
	}
	if !sameRepairPlan(executed.Snapshot, reserved) {
		return nil, d.ErrConflict
	}

	summary := &control.SupervisorSummary{TaskID: task.ID, TaskVersion: task.Version}
	if e = s.Runtime.DecorateSummary(ctx, scope(p), summary); e != nil {
		return nil, e
	}
	found := false
	for _, r := range summary.ResultRefs {
		if r == in.SourceRef {
			found = true
		}
	}
	if !found {
		return nil, d.ErrForbidden
	}
	_, raw, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, in.SourceRef)
	if e != nil {
		return nil, e
	}
	var manifest struct {
		TaskID   string `json:"task_id"`
		TaskKind string `json:"task_kind"`
	}
	if in.LabelResultID != "" {
		if json.Unmarshal(raw, &manifest) != nil || manifest.TaskKind != "tagging_task" || manifest.TaskID == "" {
			return nil, d.ErrForbidden
		}
	}
	var data l.Input
	// A labeling report carries a source reference rather than publishable prose.
	if in.LabelResultID != "" {
		_, raw, e = s.Assets.Resolve(ctx, p.TenantID, p.UserID, c.BaselineRef)
		if e != nil {
			return nil, e
		}
	}
	if json.Unmarshal(raw, &data) != nil || data.Kind != "text" || len(data.Text) > 16000 {
		return nil, fmt.Errorf("%w: candidate knowledge must be text, at most 16000 bytes", d.ErrInvalid)
	}
	labels := []string{}
	if in.LabelResultID != "" {
		labels, e = s.Projection.Labels(ctx, p, in.LabelResultID, in.LabelReviewVersion, c.BaselineRef, manifest.TaskID)
		if e != nil {
			return nil, e
		}
	} else if in.LabelReviewVersion != 0 {
		return nil, d.ErrInvalid
	}
	publication := &d.Publication{ID: pubID, CaseID: id, TenantID: p.TenantID, UserID: p.UserID, CandidateInput: in, KnowledgeID: CandidateKnowledgeID(p.TenantID, c.KnowledgeBaseID, pubID), ContentDigest: d.Digest(data.Text), Labels: labels, Status: "candidate"}
	return s.Repo.CreatePublication(ctx, publication, key, d.Digest(in))
}
func CandidateKnowledgeID(tenant int64, kb, pub string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("feedback/%d/%s/%s", tenant, kb, pub))).String()
}
func (s *Service) candidate(ctx context.Context, p l.Principal, id string) (*d.Case, *d.Publication, string, error) {
	v, e := s.Repo.GetPublication(ctx, p, id)
	if e != nil {
		return nil, nil, "", e
	}
	c, e := s.Get(ctx, p, v.CaseID)
	if e != nil {
		return nil, nil, "", e
	}
	_, raw, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, v.SourceRef)
	if e != nil {
		return nil, nil, "", e
	}
	if v.LabelResultID != "" {
		var manifest struct {
			TaskID   string `json:"task_id"`
			TaskKind string `json:"task_kind"`
		}
		if json.Unmarshal(raw, &manifest) != nil || manifest.TaskKind != "tagging_task" || manifest.TaskID == "" {
			return nil, nil, "", d.ErrForbidden
		}
		labels, e := s.Projection.Labels(ctx, p, v.LabelResultID, v.LabelReviewVersion, c.BaselineRef, manifest.TaskID)
		if e != nil || d.Digest(labels) != d.Digest(v.Labels) {
			return nil, nil, "", d.ErrForbidden
		}
		_, raw, e = s.Assets.Resolve(ctx, p.TenantID, p.UserID, c.BaselineRef)
		if e != nil {
			return nil, nil, "", e
		}
	}
	var data l.Input
	if json.Unmarshal(raw, &data) != nil || data.Kind != "text" || d.Digest(data.Text) != v.ContentDigest {
		return nil, nil, "", d.ErrForbidden
	}
	return c, v, data.Text, nil
}

// Project claims before any external call. A replay only reconciles persisted
// document/index receipts; it never blindly repeats an uncertain embedding call.
func (s *Service) Project(ctx context.Context, p l.Principal, id string, expected int) (*d.Publication, error) {
	c, v, content, e := s.candidate(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Status == "ready" || v.Status == "validating" || v.Status == "validated" || v.Status == "approved" {
		return v, nil
	}
	if v.Status == "projecting" || v.Status == "reconcile_required" {
		receipt, e := s.Projection.Lookup(ctx, p, c, v)
		if e != nil {
			return nil, e
		}
		if len(receipt) == 0 {
			return v, nil
		}
		v.Projection = receipt
		v.Status = "ready"
		v.Error = ""
		e = s.Repo.SavePublication(ctx, v, v.Version, d.Event{ActorID: p.UserID, Action: "projection_reconciled", Detail: "从真实文档与索引回执恢复，未重放未知外部调用"})
		return v, e
	}
	if v.Version != expected || v.Status != "candidate" {
		return nil, d.ErrConflict
	}
	if e = mutable(c); e != nil {
		return nil, e
	}
	if e = s.reserve(ctx, p, c, "projection:"+v.ID, 34); e != nil {
		return nil, e
	}
	v.Status = "projecting"
	if e = s.Repo.SavePublication(ctx, v, expected, d.Event{ActorID: p.UserID, Action: "projection_started", Detail: "候选隔离索引开始，正式读取仍使用旧活动版本"}); e != nil {
		return nil, e
	}
	receipt, err := s.Projection.Prepare(ctx, p, c, v, content)
	if err != nil {
		v.Status = "reconcile_required"
		v.Error = "投影结果待核对；不会自动重放模型调用"
	} else if len(receipt) == 0 {
		err = d.ErrConflict
		v.Status = "reconcile_required"
		v.Error = "缺少真实投影回执"
	} else {
		v.Status = "ready"
		v.Projection = receipt
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	saveErr := s.Repo.SavePublication(saveCtx, v, v.Version, d.Event{ActorID: p.UserID, Action: "projection_observed", Detail: v.Status})
	if saveErr != nil {
		return nil, saveErr
	}
	return v, err
}
func baselineView(c *d.Case) d.View {
	return d.View{CaseID: c.ID, PublicationID: "baseline", KnowledgeBaseID: c.KnowledgeBaseID, KnowledgeID: c.BaselineKnowledgeID, Digest: c.BaselineDigest, Labels: c.BaselineLabels, Deadline: c.Deadline}
}
func candidateView(c *d.Case, v *d.Publication) d.View {
	return d.View{CaseID: c.ID, PublicationID: v.ID, KnowledgeBaseID: c.KnowledgeBaseID, KnowledgeID: v.KnowledgeID, Digest: v.ContentDigest, Labels: v.Labels, Deadline: c.Deadline}
}
func (s *Service) ValidateCandidate(ctx context.Context, p l.Principal, id string, expected int) (*d.Publication, error) {
	c, v, _, e := s.candidate(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Status == "validated" || v.Status == "approved" {
		return v, nil
	}
	if v.Status != "ready" && v.Status != "validating" {
		return nil, d.ErrConflict
	}
	if v.Status == "ready" {
		if v.Version != expected {
			return nil, d.ErrConflict
		}
		if e = mutable(c); e != nil {
			return nil, e
		}
		var frozen struct {
			QAPairs []json.RawMessage `json:"qa_pairs"`
		}
		if json.Unmarshal(c.EvaluationSnapshot, &frozen) != nil || len(frozen.QAPairs) == 0 {
			return nil, d.ErrInvalid
		}
		calls := len(frozen.QAPairs) * 8
		if e = s.reserve(ctx, p, c, "validation:"+v.ID, calls); e != nil {
			return nil, e
		}
		v.Status = "validating"
		if e = s.Repo.SavePublication(ctx, v, expected, d.Event{ActorID: p.UserID, Action: "validation_started", Detail: "固定基线与隔离候选验证开始"}); e != nil {
			return nil, e
		}
	}
	v.BaselineEvaluationID, e = s.Evaluations.FeedbackStart(ctx, p, "feedback-base:"+v.ID, c.EvaluationSnapshot, baselineView(c))
	if e != nil {
		return nil, e
	}
	v.CandidateEvaluationID, e = s.Evaluations.FeedbackStart(ctx, p, "feedback-next:"+v.ID, c.EvaluationSnapshot, candidateView(c, v))
	if e != nil {
		return nil, e
	}
	if e = s.Repo.SavePublication(ctx, v, v.Version, d.Event{ActorID: p.UserID, Action: "validation_bound", Detail: "真实基线与候选评测任务已绑定"}); e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) Compare(ctx context.Context, p l.Principal, id string) (*d.Publication, error) {
	c, v, _, e := s.candidate(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Status != "validating" && v.Status != "validated" && v.Status != "approved" && v.Status != "rejected" {
		return nil, d.ErrConflict
	}
	comparison, e := s.Evaluations.FeedbackCompare(ctx, p, v.BaselineEvaluationID, v.CandidateEvaluationID, c.EvaluationSnapshot, baselineView(c), candidateView(c, v), c.Gate)
	if e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(comparison)
	digest := d.Digest(struct {
		Gate       d.Gate
		Comparison json.RawMessage
		View       d.View
	}{c.Gate, raw, candidateView(c, v)})
	if v.Status == "approved" {
		if v.ValidationDigest != digest {
			return nil, d.ErrGate
		}
		return v, nil
	}
	v.Comparison = raw
	v.ValidationDigest = digest
	v.Status = "rejected"
	v.Error = "固定门槛未通过：" + comparison.Gate
	if comparison.Gate == "eligible_for_review" && comparison.Improvement >= c.Gate.MinImprovement {
		v.Status = "validated"
		v.Error = ""
	}
	if e = s.Repo.SavePublication(ctx, v, v.Version, d.Event{ActorID: p.UserID, Action: "validation_compared", Detail: v.Status}); e != nil {
		return nil, e
	}
	return v, nil
}
func (s *Service) Approve(ctx context.Context, p l.Principal, id string, expected int, comment string) (*d.Publication, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	if len(comment) == 0 || len(comment) > 2000 {
		return nil, d.ErrInvalid
	}
	_, v, _, e := s.candidate(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Status == "approved" {
		return v, nil
	}
	if v.Version != expected || v.Status != "validated" {
		return nil, d.ErrGate
	}
	// Re-read real metrics on approval; a saved UI result does not grant publication.
	v, e = s.Compare(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Status != "validated" {
		return nil, d.ErrGate
	}
	v.Status = "approved"
	v.ApprovedBy = p.UserID
	v.ReviewComment = comment
	e = s.Repo.SavePublication(ctx, v, v.Version, d.Event{ActorID: p.UserID, Action: "approved", Detail: comment})
	return v, e
}
func (s *Service) Activate(ctx context.Context, p l.Principal, id string, expectedCase, expectedPublication int) (*d.Case, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	c, v, _, e := s.candidate(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if v.Status != "approved" || v.Version != expectedPublication {
		return nil, d.ErrGate
	}
	if _, e = s.Compare(ctx, p, id); e != nil {
		return nil, e
	}
	receipt, e := s.Projection.Lookup(ctx, p, c, v)
	if e != nil || len(receipt) == 0 || d.Digest(receipt) != d.Digest(v.Projection) {
		return nil, d.ErrGate
	}
	if e = s.Repo.Commit(ctx, p, c.ID, v.ID, expectedCase, v.Version, v.ValidationDigest, "activate"); e != nil {
		return nil, e
	}
	return s.Get(ctx, p, c.ID)
}
func (s *Service) Withdraw(ctx context.Context, p l.Principal, id string, expected int) (*d.Case, error) {
	// Owner/admin can close the read gate after source revocation; never read its body.
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	_, e := s.Repo.GetCase(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.Repo.Commit(ctx, p, id, "", expected, 0, "", "withdraw"); e != nil {
		return nil, e
	}
	c, e := s.Repo.GetCase(ctx, p, id)
	if e != nil {
		return nil, e
	}
	c.Create = d.Create{}
	c.EvaluationSnapshot = nil
	c.Rounds = nil
	c.Corrections = nil
	c.Reservations = nil
	c.BaselineKnowledgeID = ""
	c.BaselineDigest = ""
	return c, nil
}
func (s *Service) Rollback(ctx context.Context, p l.Principal, id, pubID string, expected int) (*d.Case, error) {
	if e := s.auth(ctx, p, "admin"); e != nil {
		return nil, e
	}
	c, e := s.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if pubID != "baseline" {
		v, e := s.Repo.GetPublication(ctx, p, pubID)
		if e != nil {
			return nil, e
		}
		if v.CaseID != id {
			return nil, d.ErrForbidden
		}
		return s.Activate(ctx, p, pubID, expected, v.Version)
	}
	if e = s.Projection.Validate(ctx, p, c.KnowledgeBaseID, c.BaselineKnowledgeID, c.BaselineDigest); e != nil {
		return nil, e
	}
	if e = s.Repo.Commit(ctx, p, id, "", expected, 0, "", "rollback_baseline"); e != nil {
		return nil, e
	}
	return s.Get(ctx, p, id)
}

// CheckView revalidates the exact immutable projection on every evaluation read.
func (s *Service) CheckView(ctx context.Context, p l.Principal, view d.View) error {
	c, e := s.Get(ctx, p, view.CaseID)
	if e != nil {
		return e
	}
	if c.Status == "invalidated" || c.Status == "withdrawn" {
		return d.ErrForbidden
	}
	if view.PublicationID == "baseline" {
		if d.Digest(view) != d.Digest(baselineView(c)) {
			return d.ErrForbidden
		}
		return nil
	}
	_, v, _, e := s.candidate(ctx, p, view.PublicationID)
	if e != nil {
		return e
	}
	if v.CaseID != c.ID || d.Digest(view) != d.Digest(candidateView(c, v)) || len(v.Projection) == 0 {
		return d.ErrForbidden
	}
	receipt, e := s.Projection.Lookup(ctx, p, c, v)
	if e != nil || len(receipt) == 0 || d.Digest(receipt) != d.Digest(v.Projection) {
		return d.ErrForbidden
	}
	return nil
}

// ReconcileSources does no paid work. Read gates run this check synchronously;
// asynchronous cleanup removes obsolete projections after the gate is closed.
func (s *Service) ReconcileSources(ctx context.Context, cursor string) (string, error) {
	rows, next, e := s.Repo.ScanCases(ctx, cursor, 30)
	if e != nil {
		return "", e
	}
	for _, c := range rows {
		p := l.Principal{TenantID: c.TenantID, UserID: c.UserID}
		sourceErr := s.refs(ctx, p, &c)
		if sourceErr != nil && !permanentSourceError(sourceErr) {
			return "", sourceErr
		}
		if sourceErr != nil && c.Status != "invalidated" {
			c.Status = "invalidated"
			c.Error = "来源或验证样本已失效，历史输出不可消费"
			if e = s.Repo.SaveCase(ctx, &c, c.Version, d.Event{Action: "source_invalidated", ActorID: p.UserID, Detail: c.Error}); e != nil {
				if errors.Is(e, d.ErrConflict) {
					continue
				}
				return "", e
			}
		}
		pubs, e := s.Repo.ListPublications(ctx, p, c.ID)
		if e != nil {
			return "", e
		}
		for _, v := range pubs {
			if v.Status == "cleaned" {
				continue
			}
			if sourceErr == nil && c.Status != "invalidated" {
				_, _, _, candidateErr := s.candidate(ctx, p, v.ID)
				if candidateErr == nil {
					continue
				}
				if !permanentSourceError(candidateErr) {
					return "", candidateErr
				}
			}
			err := s.Projection.Remove(ctx, p, c.KnowledgeBaseID, v.KnowledgeID)
			if err != nil {
				v.Status = "cleanup_failed"
				v.Error = "派生索引清理失败，可重试；读取已阻断"
			} else {
				v.Status = "cleaned"
				v.Error = ""
			}
			if e = s.Repo.SavePublication(ctx, &v, v.Version, d.Event{ActorID: p.UserID, Action: v.Status, Detail: v.Error}); e != nil && !errors.Is(e, d.ErrConflict) {
				return "", e
			}
		}
	}
	return next, nil
}

func sameRepairPlan(actual json.RawMessage, reserved control.PlanSpec) bool {
	var plan control.PlanSpec
	return json.Unmarshal(actual, &plan) == nil && d.Digest(plan) == d.Digest(reserved)
}
