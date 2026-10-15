package taskcontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
)

type Service struct {
	repo      d.Repository
	validator *Validator
}

func NewService(repo d.Repository, validator *Validator) (*Service, error) {
	if repo == nil || validator == nil {
		return nil, d.ErrInvalid
	}
	return &Service{repo: repo, validator: validator}, nil
}

type DraftInput struct {
	Goal      string    `json:"goal"`
	InputRefs []l.RefID `json:"input_refs"`
	Budget    Budget    `json:"budget"`
}
type AppendPlanInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	Plan            PlanSpec `json:"plan"`
}
type PlanResult struct {
	Plan       *d.Plan     `json:"plan"`
	Validation *Validation `json:"validation"`
}

func (s *Service) authorized(ctx context.Context, p d.Scope) error {
	if e := p.Validate(); e != nil {
		return e
	}
	if e := s.validator.authorize(ctx, p, "read"); e != nil {
		return d.ErrForbidden
	}
	return nil
}
func (s *Service) Capabilities(ctx context.Context, p d.Scope) ([]Capability, error) {
	if e := s.authorized(ctx, p); e != nil {
		return nil, e
	}
	return s.validator.Capabilities(), nil
}
func (s *Service) Validate(ctx context.Context, p d.Scope, plan PlanSpec) (*Validation, error) {
	return s.validator.Validate(ctx, p, plan)
}

func (s *Service) CreateDraft(ctx context.Context, p d.Scope, in DraftInput, key, requestID string) (*d.Task, error) {
	if e := s.authorized(ctx, p); e != nil {
		return nil, e
	}
	in.Goal = strings.TrimSpace(in.Goal)
	if !utf8.ValidString(in.Goal) || utf8.RuneCountInString(in.Goal) < 1 || utf8.RuneCountInString(in.Goal) > 4000 || !validBudget(in.Budget) || len(in.InputRefs) < 1 || len(in.InputRefs) > 64 || strings.TrimSpace(key) == "" || len(key) > 100 {
		return nil, d.ErrInvalid
	}
	seen := map[string]bool{}
	for _, ref := range in.InputRefs {
		if e := ref.Validate(); e != nil || seen[ref.String()] {
			return nil, d.ErrInvalid
		}
		seen[ref.String()] = true
		if _, e := s.validator.resolve(ctx, p, ref); e != nil {
			return nil, d.ErrForbidden
		}
	}
	refs, e := json.Marshal(in.InputRefs)
	if e != nil {
		return nil, e
	}
	budget, e := json.Marshal(in.Budget)
	if e != nil {
		return nil, e
	}
	// Resolve again even on idempotent replay: a saved draft is not lasting access.
	return s.repo.CreateTask(ctx, p, d.CreateTaskInput{Goal: in.Goal, InputRefs: refs, Budget: budget, RequestID: requestID, IdempotencyKey: key})
}
func (s *Service) Get(ctx context.Context, p d.Scope, id string) (*d.Task, error) {
	if e := s.authorized(ctx, p); e != nil {
		return nil, e
	}
	return s.repo.GetTask(ctx, p, id)
}
func (s *Service) List(ctx context.Context, p d.Scope, f d.ListFilter) (*d.TaskPage, error) {
	if e := s.authorized(ctx, p); e != nil {
		return nil, e
	}
	return s.repo.ListTasks(ctx, p, f)
}
func (s *Service) GetPlan(ctx context.Context, p d.Scope, id string, version int64) (*d.Plan, error) {
	if e := s.authorized(ctx, p); e != nil {
		return nil, e
	}
	return s.repo.GetPlan(ctx, p, id, version)
}

func withinBudget(candidate, ceiling Budget) bool {
	return candidate.MaxNodes <= ceiling.MaxNodes && candidate.MaxConcurrency <= ceiling.MaxConcurrency && candidate.MaxAttempts <= ceiling.MaxAttempts && candidate.MaxCalls <= ceiling.MaxCalls && candidate.MaxCostMicros <= ceiling.MaxCostMicros && candidate.TimeoutSeconds <= ceiling.TimeoutSeconds
}

func (s *Service) AppendPlan(ctx context.Context, p d.Scope, id string, in AppendPlanInput) (*PlanResult, error) {
	task, e := s.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if task.Version != in.ExpectedVersion || (task.Status != d.StatusDraft && task.Status != d.StatusReady) {
		return nil, d.ErrConflict
	}
	budget, e := decodeStored[Budget](task.Budget)
	if e != nil {
		return nil, fmt.Errorf("invalid stored task budget")
	}
	if !withinBudget(in.Plan.Budget, budget) {
		return nil, &ValidationError{[]Issue{{Field: "budget", Code: "task_budget_exceeded", Message: "plan cannot expand the task's authorized budget"}}}
	}
	validation, e := s.validator.Validate(ctx, p, in.Plan)
	if e != nil {
		return nil, e
	}
	roots, e := decodeStored[[]l.RefID](task.InputRefs)
	if e != nil {
		return nil, fmt.Errorf("invalid stored input references")
	}
	allowed := map[string]bool{}
	for _, ref := range roots {
		allowed[ref.String()] = true
	}
	for _, ref := range validation.InputRefs {
		if !allowed[ref.String()] {
			return nil, &ValidationError{[]Issue{{Field: "inputs", Code: "task_input_scope", Message: "plan source must belong to the task input snapshot"}}}
		}
	}
	raw, e := json.Marshal(in.Plan)
	if e != nil {
		return nil, e
	}
	plan, e := s.repo.AppendPlan(ctx, p, id, in.ExpectedVersion, raw)
	if e != nil {
		return nil, e
	}
	return &PlanResult{Plan: plan, Validation: validation}, nil
}

// LifecycleCapabilities wraps existing specialist contracts. It declares a
// planning vocabulary only; adapters and runtime approval enforcement are separate.
func LifecycleCapabilities(contracts map[string]l.AgentContract) []Capability {
	items := make([]Capability, 0, len(contracts))
	for _, c := range contracts {
		permissions := append([]string(nil), c.RequiredScopes...)
		write := c.RiskClass == "write" || c.RiskClass == "etl"
		if write && !contains(permissions, "write") {
			permissions = append(permissions, "write")
		}
		capability := Capability{ID: "lifecycle." + c.ID, Version: 1, Domain: "lifecycle", InputKinds: c.InputKinds, OutputKinds: c.OutputKinds, Tools: c.ToolAllowlist, Permissions: permissions, RequiresApproval: write}
		// Mirror dataInput's ordered selection in the existing lifecycle adapters.
		// A report can accompany cleaning, but cannot replace its data input.
		for _, kind := range c.InputKinds {
			if kind == l.RawAsset || kind == l.CleanedAsset || kind == l.EnrichedAsset || kind == l.PublishedAsset {
				capability.RequiredAnyInputKinds = append(capability.RequiredAnyInputKinds, kind)
			}
		}
		if c.ID == "data-evaluation" {
			capability.RequiredAnyInputKinds = nil
		}
		if c.ID == "data-quality" {
			capability.FirstInputPassthroughKinds = append([]l.Kind(nil), capability.RequiredAnyInputKinds...)
		}
		items = append(items, capability)
	}
	return items
}
