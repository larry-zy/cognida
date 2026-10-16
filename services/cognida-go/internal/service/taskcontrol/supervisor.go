package taskcontrol

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
)

var (
	ErrPlannerUnavailable = errors.New("supervisor planner unavailable")
	ErrPlannerOutput      = errors.New("supervisor returned an invalid proposal")
)

type SupervisorQuestion struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}
type SupervisorAnswer struct {
	QuestionID string `json:"question_id"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}
type SupervisorSource struct {
	Ref  l.RefID `json:"ref"`
	Kind l.Kind  `json:"kind"`
}
type PlanningInput struct {
	Goal         string             `json:"goal"`
	Sources      []SupervisorSource `json:"sources"`
	Budget       Budget             `json:"budget"`
	Capabilities []Capability       `json:"capabilities"`
	Answers      []SupervisorAnswer `json:"answers"`
}
type PlannerDecision struct {
	Kind        string               `json:"kind"`
	Explanation string               `json:"explanation"`
	Questions   []SupervisorQuestion `json:"questions,omitempty"`
	Plan        *PlanSpec            `json:"plan,omitempty"`
}

// Planner receives only authorized reference metadata and a closed planning
// vocabulary. It has no dispatcher, approval, shell or business-tool interface.
type Planner interface {
	Propose(context.Context, PlanningInput) (PlannerDecision, error)
}
type Supervisor struct {
	tasks   *Service
	planner Planner
}

func NewSupervisor(tasks *Service, planner Planner) (*Supervisor, error) {
	if tasks == nil || planner == nil {
		return nil, d.ErrInvalid
	}
	return &Supervisor{tasks: tasks, planner: planner}, nil
}

type AssessmentInput struct {
	Goal      string    `json:"goal"`
	InputRefs []l.RefID `json:"input_refs"`
	Budget    *Budget   `json:"budget,omitempty"`
}
type Assessment struct {
	Ready            bool                 `json:"ready"`
	Questions        []SupervisorQuestion `json:"questions"`
	ExecutionEnabled bool                 `json:"execution_enabled"`
}
type ProposeInput struct {
	ExpectedVersion int64              `json:"expected_version"`
	Answers         []SupervisorAnswer `json:"answers"`
}
type SupervisorProposal struct {
	TaskID            string             `json:"task_id"`
	TaskVersion       int64              `json:"task_version"`
	Decision          PlannerDecision    `json:"decision"`
	Validation        *Validation        `json:"validation,omitempty"`
	BudgetAdjustments []BudgetAdjustment `json:"budget_adjustments,omitempty"`
	ExecutionEnabled  bool               `json:"execution_enabled"`
	PlanningCalls     int                `json:"planning_calls"`
	Saved             bool               `json:"saved"`
}

func (s *Supervisor) sources(ctx context.Context, p d.Scope, refs []l.RefID) ([]SupervisorSource, error) {
	if len(refs) > 64 {
		return nil, d.ErrInvalid
	}
	items := make([]SupervisorSource, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Validate() != nil || seen[ref.String()] {
			return nil, d.ErrInvalid
		}
		seen[ref.String()] = true
		kind, e := s.tasks.validator.resolve(ctx, p, ref)
		if e != nil {
			return nil, d.ErrForbidden
		}
		items = append(items, SupervisorSource{Ref: ref, Kind: kind})
	}
	return items, nil
}

// Assess asks for required planning inputs without spending a model call.
// Ready means enough input for proposing, not approval or permission to execute.
func (s *Supervisor) Assess(ctx context.Context, p d.Scope, in AssessmentInput) (*Assessment, error) {
	if e := s.tasks.authorized(ctx, p); e != nil {
		return nil, e
	}
	in.Goal = strings.TrimSpace(in.Goal)
	if !utf8.ValidString(in.Goal) || utf8.RuneCountInString(in.Goal) > 4000 || strings.ContainsRune(in.Goal, '\x00') {
		return nil, d.ErrInvalid
	}
	result := &Assessment{Questions: []SupervisorQuestion{}}
	if in.Goal == "" {
		result.Questions = append(result.Questions, SupervisorQuestion{ID: "goal", Question: "希望对这些数据完成什么工作，最终需要什么产物？"})
	}
	if len(in.InputRefs) == 0 {
		result.Questions = append(result.Questions, SupervisorQuestion{ID: "inputs", Question: "请选择本次任务的输入资产版本。"})
	}
	if _, e := s.sources(ctx, p, in.InputRefs); e != nil {
		return nil, e
	}
	if in.Budget == nil {
		result.Questions = append(result.Questions, SupervisorQuestion{ID: "budget", Question: "请设置任务节点数、并发、调用、重试、成本与时间上限。"})
	} else if !validBudget(*in.Budget) {
		return nil, d.ErrInvalid
	}
	result.Ready = len(result.Questions) == 0
	return result, nil
}

func validAnswers(answers []SupervisorAnswer) bool {
	if len(answers) > 12 {
		return false
	}
	seen := map[string]bool{}
	for _, answer := range answers {
		if !identifier.MatchString(answer.QuestionID) || seen[answer.QuestionID] || !plannerText(answer.Question, 1000) || !plannerText(answer.Answer, 4000) {
			return false
		}
		seen[answer.QuestionID] = true
	}
	return true
}

// Propose is read-only: a candidate is saved only through the existing versioned
// AppendPlan command after review. Repeating it may perform another bounded LLM
// call, but cannot create jobs, grant approval, or mutate the task.
func (s *Supervisor) Propose(ctx context.Context, p d.Scope, id string, in ProposeInput) (*SupervisorProposal, error) {
	if in.ExpectedVersion < 1 || !validAnswers(in.Answers) {
		return nil, d.ErrInvalid
	}
	task, e := s.tasks.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if task.Version != in.ExpectedVersion || (task.Status != d.StatusDraft && task.Status != d.StatusReady) {
		return nil, d.ErrConflict
	}
	children, e := s.tasks.repo.ListChildren(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if len(children) > 0 {
		return nil, d.ErrConflict
	}
	refs, e := decodeStored[[]l.RefID](task.InputRefs)
	if e != nil || len(refs) == 0 {
		return nil, d.ErrInvalid
	}
	budget, e := decodeStored[Budget](task.Budget)
	if e != nil || !validBudget(budget) {
		return nil, d.ErrInvalid
	}
	sources, e := s.sources(ctx, p, refs)
	if e != nil {
		return nil, e
	}
	// Avoid advertising capabilities that the current principal cannot use.
	caps := []Capability{}
	for _, capability := range s.tasks.validator.Capabilities() {
		allowed := true
		for _, permission := range capability.Permissions {
			if s.tasks.validator.authorize(ctx, p, permission) != nil {
				allowed = false
				break
			}
		}
		if allowed {
			caps = append(caps, capability)
		}
	}
	if len(caps) == 0 {
		return nil, d.ErrForbidden
	}
	decision, e := s.planner.Propose(ctx, PlanningInput{Goal: task.Goal, Sources: sources, Budget: budget, Capabilities: caps, Answers: in.Answers})
	if e != nil {
		return nil, e
	}
	if e = validatePlannerDecision(decision); e != nil {
		return nil, ErrPlannerOutput
	}
	if decision.Kind == "clarify" {
		for _, question := range decision.Questions {
			for _, answer := range in.Answers {
				if question.ID == answer.QuestionID {
					return nil, ErrPlannerOutput
				}
			}
		}
	}
	// Recheck after the model call: even clarification text is not an access grant.
	current, e := s.tasks.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if current.Version != task.Version || current.Status != task.Status {
		return nil, d.ErrConflict
	}
	if _, e = s.sources(ctx, p, refs); e != nil {
		return nil, e
	}
	result := &SupervisorProposal{TaskID: id, TaskVersion: task.Version, Decision: decision, PlanningCalls: 1}
	if decision.Kind == "proposal" {
		if !withinBudget(decision.Plan.Budget, budget) {
			return nil, &ValidationError{[]Issue{{Field: "budget", Code: "task_budget_exceeded", Message: "proposal cannot expand the task budget"}}}
		}
		allowed := map[string]bool{}
		for _, ref := range refs {
			allowed[ref.String()] = true
		}
		for _, node := range decision.Plan.Nodes {
			for _, input := range node.Inputs {
				if input.Asset != nil && !allowed[input.Asset.String()] {
					return nil, &ValidationError{[]Issue{{NodeID: node.ID, Field: "inputs", Code: "task_input_scope", Message: "proposal source must belong to the task input snapshot"}}}
				}
			}
			// The model must not select a capability omitted by authorization.
			found := false
			for _, capability := range caps {
				if capability.ID == node.Capability {
					found = true
					break
				}
			}
			if !found {
				return nil, d.ErrForbidden
			}
		}
		compiled, adjustments, compileErr := compileSupervisorBudget(*decision.Plan, budget)
		if compileErr != nil {
			return nil, compileErr
		}
		result.Validation, e = s.tasks.Validate(ctx, p, compiled)
		if e != nil {
			return nil, e
		}
		result.Decision.Plan = &compiled
		result.BudgetAdjustments = adjustments
	}
	return result, nil
}

type SupervisorChildSummary struct {
	ID             string        `json:"id"`
	NodeID         string        `json:"node_id"`
	PlanVersion    int64         `json:"plan_version"`
	Attempt        int           `json:"attempt"`
	Domain         string        `json:"domain"`
	DomainID       string        `json:"domain_id,omitempty"`
	DispatchStatus d.ChildStatus `json:"dispatch_status"`
	DomainStatus   string        `json:"domain_status"`
	DomainKind     string        `json:"domain_kind,omitempty"`
	ReceiptPresent bool          `json:"receipt_present"`
}
type SupervisorSummary struct {
	TaskID             string                   `json:"task_id"`
	TaskVersion        int64                    `json:"task_version"`
	Status             d.Status                 `json:"status"`
	CurrentPlanVersion int64                    `json:"current_plan_version"`
	PlannedNodes       int                      `json:"planned_nodes"`
	Children           []SupervisorChildSummary `json:"children"`
	ResultRefs         []l.RefID                `json:"result_refs"`
	Warnings           []string                 `json:"warnings"`
	ExecutionEnabled   bool                     `json:"execution_enabled"`
	Terminal           bool                     `json:"terminal"`
	CanCancel          bool                     `json:"can_cancel"`
	Nodes              []RuntimeNodeSummary     `json:"nodes"`
	Usage              RuntimeUsage             `json:"usage"`
}

// Summarize uses persisted facts, not model narration. Domain execution remains
// unknown until its authoritative read adapter is connected in the next phase.
func (s *Supervisor) Summarize(ctx context.Context, p d.Scope, id string) (*SupervisorSummary, error) {
	task, e := s.tasks.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	result := &SupervisorSummary{TaskID: id, TaskVersion: task.Version, Status: task.Status, CurrentPlanVersion: task.CurrentPlanVersion, Children: []SupervisorChildSummary{}, ResultRefs: []l.RefID{}, Warnings: []string{"当前仅提供规划；领域执行状态尚未接入。"}}
	if task.CurrentPlanVersion > 0 {
		plan, e := s.tasks.GetPlan(ctx, p, id, task.CurrentPlanVersion)
		if e != nil {
			return nil, e
		}
		snapshot, e := decodeStored[PlanSpec](plan.Snapshot)
		if e != nil || len(snapshot.Nodes) > 32 {
			return nil, d.ErrInvalid
		}
		result.PlannedNodes = len(snapshot.Nodes)
	}
	children, e := s.tasks.repo.ListChildren(ctx, p, id)
	if e != nil {
		return nil, e
	}
	for _, child := range children {
		result.Children = append(result.Children, SupervisorChildSummary{ID: child.ID, NodeID: child.NodeID, PlanVersion: child.PlanVersion, Attempt: child.Attempt, Domain: child.Domain, DomainID: child.DomainID, DispatchStatus: child.Status, DomainStatus: "unknown", ReceiptPresent: len(child.Receipt) > 0 && string(child.Receipt) != "null"})
	}
	if len(children) > 0 {
		result.Warnings = append(result.Warnings, "子任务绑定或提交回执不表示领域任务已完成。")
	}
	refs, e := decodeStored[[]l.RefID](task.ResultRefs)
	if e != nil {
		return nil, d.ErrInvalid
	}
	if len(refs) > 64 {
		return nil, d.ErrInvalid
	}
	for _, ref := range refs {
		if ref.Validate() != nil {
			return nil, d.ErrInvalid
		}
		if _, e = s.tasks.validator.resolve(ctx, p, ref); e != nil {
			result.Warnings = append(result.Warnings, "部分结果引用当前不可访问，已隐藏。")
			continue
		}
		result.ResultRefs = append(result.ResultRefs, ref)
	}
	current, e := s.tasks.Get(ctx, p, id)
	if e != nil {
		return nil, e
	}
	if current.Version != task.Version {
		return nil, d.ErrConflict
	}
	return result, nil
}
