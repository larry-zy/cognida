package feedback

import (
	ev "cognida/internal/model/evaluation"
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	"cognida/internal/service/evaluation"
	control "cognida/internal/service/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"math"
	"strings"
	"time"
)

type Assets interface {
	Resolve(context.Context, int64, int64, l.RefID) (l.DataAssetRef, json.RawMessage, error)
}
type Projection interface {
	CurrentLabels(context.Context, l.Principal, string) ([]string, error)
	Visible(context.Context, l.Principal, string, string, bool) (bool, error)
	// Baseline must be a real knowledge-backed input, not a caller supplied document ID.
	Baseline(context.Context, l.Principal, l.DataAssetRef, string) (string, string, error)
	Validate(context.Context, l.Principal, string, string, string) error
	Prepare(context.Context, l.Principal, *d.Case, *d.Publication, string) (json.RawMessage, error)
	Lookup(context.Context, l.Principal, *d.Case, *d.Publication) (json.RawMessage, error)
	Remove(context.Context, l.Principal, string, string) error
	Labels(context.Context, l.Principal, string, int, l.RefID, string) ([]string, error)
}
type Evaluations interface {
	FeedbackFreeze(context.Context, l.Principal, string, json.RawMessage) (json.RawMessage, int, error)
	FeedbackValidate(context.Context, l.Principal, json.RawMessage) error
	FeedbackStart(context.Context, l.Principal, string, json.RawMessage, d.View) (string, error)
	FeedbackCompare(context.Context, l.Principal, string, string, json.RawMessage, d.View, d.View, d.Gate) (*evaluation.Comparison, error)
	FeedbackEvidence(context.Context, l.Principal, string, string, string) error
}
type Service struct {
	Repo        d.Repository
	Assets      Assets
	Projection  Projection
	Evaluations Evaluations
	Tasks       *control.Service
	Runtime     *control.Runtime
	Authorize   func(context.Context, l.Principal, string) error
}

func NewService(repo d.Repository, assets Assets, projection Projection, eval Evaluations, tasks *control.Service, runtime *control.Runtime, authorize func(context.Context, l.Principal, string) error) (*Service, error) {
	if repo == nil || assets == nil || projection == nil || eval == nil || authorize == nil {
		return nil, d.ErrInvalid
	}
	return &Service{repo, assets, projection, eval, tasks, runtime, authorize}, nil
}
func (s *Service) auth(ctx context.Context, p l.Principal, permission string) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	if e := s.Authorize(ctx, p, permission); e != nil {
		return d.ErrForbidden
	}
	return nil
}
func validKey(key string) bool {
	return len(key) > 0 && len(key) <= 100 && strings.TrimSpace(key) == key
}
func validCreate(in d.Create) bool {
	if strings.TrimSpace(in.Title) == "" || len(in.Title) > 256 || len(in.Evidence.Question) == 0 || len(in.Evidence.Question) > 16000 || len(in.Evidence.Problem) == 0 || len(in.Evidence.Problem) > 16000 || in.BaselineRef.Validate() != nil || in.Evidence.OutputRef.Validate() != nil || len(in.Evidence.SourceRefs) == 0 || len(in.Evidence.SourceRefs) > 16 || in.DatasetVersionID == "" || len(in.EvaluationConfig) > 32000 || !json.Valid(in.EvaluationConfig) {
		return false
	}
	switch in.Evidence.Stage {
	case "collection", "cleaning", "labeling", "retrieval", "research":
	default:
		return false
	}
	if (in.Evidence.EvaluationTaskID == "") != (in.Evidence.EvaluationResultID == "") {
		return false
	}
	if in.Gate.MinPairs < 2 || in.Gate.MinPairs > 100 || in.Gate.MaxRegression < 0 || in.Gate.MinImprovement <= 0 || math.IsNaN(in.Gate.MaxRegression) || math.IsInf(in.Gate.MaxRegression, 0) || math.IsNaN(in.Gate.MinImprovement) || math.IsInf(in.Gate.MinImprovement, 0) {
		return false
	}
	switch in.Gate.Metric {
	case "rouge_l", "bleu_4", "semantic_similarity", "llm_score", "recall", "precision", "success_rate":
	default:
		return false
	}
	return in.Budget.MaxRounds >= 1 && in.Budget.MaxRounds <= 3 && in.Budget.MaxCalls >= 1 && in.Budget.MaxCalls <= 2000 && in.Budget.MaxCostMicros >= 0 && in.Budget.TimeoutSeconds >= 60 && in.Budget.TimeoutSeconds <= 86400
}
func (s *Service) refs(ctx context.Context, p l.Principal, c *d.Case) error {
	seen := map[string]bool{}
	refs := append([]l.RefID{c.BaselineRef, c.Evidence.OutputRef}, c.Evidence.SourceRefs...)
	for _, id := range refs {
		if id.Validate() != nil {
			return d.ErrInvalid
		}
		if seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		if _, _, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, id); e != nil {
			return e
		}
	}
	if e := s.Projection.Validate(ctx, p, c.KnowledgeBaseID, c.BaselineKnowledgeID, c.BaselineDigest); e != nil {
		return e
	}
	labels, e := s.Projection.CurrentLabels(ctx, p, c.BaselineKnowledgeID)
	if e != nil {
		return e
	}
	if d.Digest(labels) != d.Digest(c.BaselineLabels) {
		return d.ErrForbidden
	}
	if c.Evidence.EvaluationTaskID != "" {
		if e := s.Evaluations.FeedbackEvidence(ctx, p, c.Evidence.EvaluationTaskID, c.Evidence.EvaluationResultID, c.Evidence.Question); e != nil {
			return e
		}
	}
	return s.Evaluations.FeedbackValidate(ctx, p, c.EvaluationSnapshot)
}
func (s *Service) Create(ctx context.Context, p l.Principal, in d.Create, key string) (*d.Case, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	if !validKey(key) || !validCreate(in) {
		return nil, d.ErrInvalid
	}
	ref, _, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, in.BaselineRef)
	if e != nil {
		return nil, e
	}
	knowledgeID, digest, e := s.Projection.Baseline(ctx, p, ref, in.KnowledgeBaseID)
	if e != nil {
		return nil, e
	}
	// A candidate belongs to its original publication ledger forever. Reusing it
	// as another baseline would shadow that ledger's revocation gate.
	if _, parent, e := s.Repo.FindKnowledge(ctx, p.TenantID, knowledgeID); e != nil {
		return nil, e
	} else if parent != nil {
		return nil, fmt.Errorf("%w: continue repair in the original feedback case", d.ErrInvalid)
	}
	baselineLabels, e := s.Projection.CurrentLabels(ctx, p, knowledgeID)
	if e != nil {
		return nil, e
	}
	frozen, count, e := s.Evaluations.FeedbackFreeze(ctx, p, in.DatasetVersionID, in.EvaluationConfig)
	if e != nil {
		return nil, e
	}
	var config evaluation.EvaluationTaskConfig
	if json.Unmarshal(in.EvaluationConfig, &config) != nil || config.KnowledgeBaseID != in.KnowledgeBaseID {
		return nil, d.ErrInvalid
	}
	if count < in.Gate.MinPairs || count > 100 {
		return nil, fmt.Errorf("%w: fixed dataset must contain %d–100 evaluation samples", d.ErrInvalid, in.Gate.MinPairs)
	}
	// A pair reserves generation, retrieval embedding and grader calls per sample.
	if count*8+34 > in.Budget.MaxCalls {
		return nil, d.ErrBudget
	}
	c := &d.Case{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Status: "open", Create: in, BaselineKnowledgeID: knowledgeID, BaselineDigest: digest, BaselineLabels: baselineLabels, EvaluationSnapshot: frozen, Deadline: time.Now().UTC().Add(time.Duration(in.Budget.TimeoutSeconds) * time.Second), Rounds: []d.Round{}}
	if e = s.refs(ctx, p, c); e != nil {
		return nil, e
	}
	return s.Repo.CreateCase(ctx, c, key, d.Digest(in))
}
func (s *Service) Get(ctx context.Context, p l.Principal, id string) (*d.Case, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, e
	}
	c, e := s.Repo.GetCase(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if e = s.refs(ctx, p, c); e != nil {
		return nil, e
	}
	return c, nil
}
func (s *Service) List(ctx context.Context, p l.Principal, cursor string) ([]d.Case, string, error) {
	if e := s.auth(ctx, p, "read"); e != nil {
		return nil, "", e
	}
	rows, next, e := s.Repo.ListCases(ctx, p, cursor, 30)
	if e != nil {
		return nil, "", e
	}
	for i := range rows {
		if s.refs(ctx, p, &rows[i]) != nil {
			rows[i].Create = d.Create{}
			rows[i].EvaluationSnapshot = nil
			rows[i].Rounds = nil
			rows[i].Corrections = nil
			rows[i].Reservations = nil
			rows[i].BaselineKnowledgeID = ""
			rows[i].BaselineDigest = ""
			rows[i].Status = "invalidated"
			rows[i].Error = "来源或评测样本已不可用，正文与历史证据访问已阻断"
		}
	}
	return rows, next, nil
}
func (s *Service) Publications(ctx context.Context, p l.Principal, id string) ([]d.Publication, error) {
	if _, e := s.Get(ctx, p, id); e != nil {
		return nil, e
	}
	rows, e := s.Repo.ListPublications(ctx, p, id)
	if e != nil {
		return nil, e
	}
	for i := range rows {
		if _, _, e = s.Assets.Resolve(ctx, p.TenantID, p.UserID, rows[i].SourceRef); e != nil {
			rows[i].Comparison = nil
			rows[i].Projection = nil
			rows[i].Labels = nil
			rows[i].Status = "invalidated"
			rows[i].Error = "候选来源不可用"
		}
	}
	return rows, nil
}
func (s *Service) Events(ctx context.Context, p l.Principal, id string) ([]d.Event, error) {
	if _, e := s.Get(ctx, p, id); e != nil {
		return nil, e
	}
	return s.Repo.Events(ctx, p, id)
}
func scope(p l.Principal) tc.Scope { return tc.Scope{TenantID: p.TenantID, UserID: p.UserID} }
func mutable(c *d.Case) error {
	if c.Status == "invalidated" || c.Status == "withdrawn" {
		return d.ErrConflict
	}
	if !time.Now().Before(c.Deadline) {
		return d.ErrBudget
	}
	return nil
}

func (s *Service) Rework(ctx context.Context, p l.Principal, id string, expected int, plan control.PlanSpec, key, requestID string) (*d.Case, error) {
	c, e := s.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if !validKey(key) || s.Tasks == nil || s.Runtime == nil {
		return nil, d.ErrInvalid
	}
	digest := d.Digest(plan)
	index := -1
	for i, r := range c.Rounds {
		if r.Key == key {
			if r.Digest != digest {
				return nil, d.ErrConflict
			}
			index = i
			break
		}
	}
	if index < 0 {
		if c.Version != expected {
			return nil, d.ErrConflict
		}
		if e = mutable(c); e != nil {
			return nil, e
		}
		if len(c.Rounds) >= c.Budget.MaxRounds || plan.Budget.MaxCalls > c.Budget.MaxCalls-c.ReservedCalls || plan.Budget.MaxCostMicros > c.Budget.MaxCostMicros-c.ReservedCostMicros {
			return nil, d.ErrBudget
		}
		// Feedback only repairs existing assets. It never authorizes publication, paid research or scripts.
		allowed := map[string]bool{"lifecycle.data-quality": true, "lifecycle.data-cleaning": true, "lifecycle.data-enrichment": true, "lifecycle.data-evaluation": true, "governance.classification": true, "governance.named_entity": true, "governance.tagging": true}
		roots := map[string]bool{c.BaselineRef.String(): true, c.Evidence.OutputRef.String(): true}
		for _, correction := range c.Corrections {
			roots[correction.Ref.String()] = true
		}
		for _, ref := range c.Evidence.SourceRefs {
			roots[ref.String()] = true
		}
		for _, n := range plan.Nodes {
			if !allowed[n.Capability] {
				return nil, d.ErrInvalid
			}
			for _, input := range n.Inputs {
				if input.Asset != nil && !roots[input.Asset.String()] {
					return nil, d.ErrForbidden
				}
			}
		}
		if _, e = s.Tasks.Validate(ctx, scope(p), plan); e != nil {
			return nil, e
		}
		if time.Duration(plan.Budget.TimeoutSeconds)*time.Second > time.Until(c.Deadline) {
			return nil, d.ErrBudget
		}
		raw, _ := json.Marshal(plan)
		c.Rounds = append(c.Rounds, d.Round{ID: uuid.NewString(), Key: key, Digest: digest, Plan: raw, Status: "reserved", Calls: plan.Budget.MaxCalls, CostMicros: plan.Budget.MaxCostMicros, CreatedAt: time.Now().UTC()})
		c.ReservedCalls += plan.Budget.MaxCalls
		c.ReservedCostMicros += plan.Budget.MaxCostMicros
		if e = s.Repo.SaveCase(ctx, c, expected, d.Event{ActorID: p.UserID, Action: "rework_reserved", Detail: "返工次数和总预算已预留，不因失败或重试返还"}); e != nil {
			return nil, e
		}
		index = len(c.Rounds) - 1
	}
	round := &c.Rounds[index]
	roots := []l.RefID{}
	seen := map[string]bool{}
	for _, n := range plan.Nodes {
		for _, in := range n.Inputs {
			if in.Asset != nil && !seen[in.Asset.String()] {
				roots = append(roots, *in.Asset)
				seen[in.Asset.String()] = true
			}
		}
	}
	task, e := s.Tasks.CreateDraft(ctx, scope(p), control.DraftInput{Goal: "反馈返工：" + c.Title, InputRefs: roots, Budget: plan.Budget}, "feedback:"+round.ID, requestID)
	if e != nil {
		return nil, e
	}
	if task.CurrentPlanVersion == 0 {
		if _, e = s.Tasks.AppendPlan(ctx, scope(p), task.ID, control.AppendPlanInput{ExpectedVersion: task.Version, Plan: plan}); e != nil {
			return nil, e
		}
		task, e = s.Tasks.Get(ctx, scope(p), task.ID)
		if e != nil {
			return nil, e
		}
	}
	// A persisted plan remains a draft until Runtime.Start performs its current
	// capability/source checks. Saving a plan is not an execution transition.
	if task.Status == tc.StatusDraft || task.Status == tc.StatusReady {
		task, e = s.Runtime.Start(ctx, scope(p), task.ID, control.StartInput{ExpectedVersion: task.Version, PlanVersion: task.CurrentPlanVersion}, "feedback-start:"+round.ID)
		if e != nil {
			return nil, e
		}
	}
	round.TaskID = task.ID
	round.Status = string(task.Status)
	if e = s.Repo.SaveCase(ctx, c, c.Version, d.Event{ActorID: p.UserID, Action: "rework_bound", Detail: "已绑定真实统一任务 " + task.ID}); e != nil {
		return nil, e
	}
	return c, nil
}

// Correct records an explicit human proposal as an immutable derived input. It
// cannot be published directly: a real repair task and the fixed gate follow.
func (s *Service) Correct(ctx context.Context, p l.Principal, id string, expected int, text, key string) (*d.Case, error) {
	c, e := s.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if !validKey(key) || strings.TrimSpace(text) == "" || len(text) > 16000 {
		return nil, d.ErrInvalid
	}
	digest := d.Digest(text)
	for _, v := range c.Corrections {
		if v.Key == key {
			if v.Digest != digest {
				return nil, d.ErrConflict
			}
			return c, nil
		}
	}
	if c.Version != expected {
		return nil, d.ErrConflict
	}
	if e = mutable(c); e != nil {
		return nil, e
	}
	if len(c.Corrections) >= c.Budget.MaxRounds {
		return nil, d.ErrBudget
	}
	writer, ok := s.Assets.(interface {
		ImportDerived(context.Context, l.Principal, l.Kind, string, []l.RefID, json.RawMessage, l.PolicySnapshot) (l.DataAssetRef, error)
	})
	if !ok {
		return nil, d.ErrConflict
	}
	base, _, e := s.Assets.Resolve(ctx, p.TenantID, p.UserID, c.BaselineRef)
	if e != nil {
		return nil, e
	}
	policy := base.Policy
	policy.Approved = false
	raw, _ := json.Marshal(l.Input{Kind: "text", Text: text})
	ref, e := writer.ImportDerived(ctx, p, l.RawAsset, "feedback-correction:"+c.ID+":"+d.Digest(key), []l.RefID{c.BaselineRef}, raw, policy)
	if e != nil {
		return nil, e
	}
	c.Corrections = append(c.Corrections, d.Correction{Ref: ref.RefID, Digest: digest, Key: key})
	if e = s.Repo.SaveCase(ctx, c, expected, d.Event{ActorID: p.UserID, Action: "human_correction", Detail: "已保存人工修正草稿；尚未运行、验证或发布"}); e != nil {
		return nil, e
	}
	return c, nil
}

func (s *Service) reserve(ctx context.Context, p l.Principal, c *d.Case, key string, calls int) error {
	if value, ok := c.Reservations[key]; ok {
		if value != calls {
			return d.ErrConflict
		}
		return nil
	}
	if calls < 1 || calls > c.Budget.MaxCalls-c.ReservedCalls {
		return d.ErrBudget
	}
	if c.Reservations == nil {
		c.Reservations = map[string]int{}
	}
	c.Reservations[key] = calls
	c.ReservedCalls += calls
	return s.Repo.SaveCase(ctx, c, c.Version, d.Event{ActorID: p.UserID, Action: "calls_reserved", Detail: fmt.Sprintf("%s：预留 %d 次调用；重试不重复预留", key, calls)})
}

// Only confirmed loss of a source can permanently invalidate and clean projections.
// Transport/database failures still fail reads closed, but never become destructive evidence.
func permanentSourceError(err error) bool {
	for _, target := range []error{d.ErrForbidden, d.ErrNotFound, l.ErrForbidden, l.ErrNotFound, l.ErrExpired, ev.ErrVersionForbidden, ev.ErrVersionNotFound, ev.ErrTaskNotFound, ev.ErrDatasetNotFound} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
