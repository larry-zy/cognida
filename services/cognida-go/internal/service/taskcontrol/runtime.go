package taskcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/taskcontrol"
	"github.com/google/uuid"
)

type Runtime struct {
	tasks          *Service
	repo           d.RuntimeRepository
	adapters       map[string]ExecutionAdapter
	enabled        bool
	validateInputs func(context.Context, d.Scope, []l.RefID) error
	orchestration  func(context.Context, d.Scope, string, PlanSpec) (int, error)
}

func NewRuntime(tasks *Service, repo d.RuntimeRepository, adapters map[string]ExecutionAdapter, enabled bool, sources ...func(context.Context, d.Scope, []l.RefID) error) (*Runtime, error) {
	if tasks == nil || repo == nil || len(adapters) == 0 {
		return nil, d.ErrInvalid
	}
	r := &Runtime{tasks: tasks, repo: repo, adapters: map[string]ExecutionAdapter{}, enabled: enabled}
	if len(sources) > 1 || (len(sources) == 1 && sources[0] == nil) {
		return nil, d.ErrInvalid
	}
	if len(sources) == 1 {
		r.validateInputs = sources[0]
	}
	for id, adapter := range adapters {
		capability, ok := tasks.validator.capabilities[id]
		if !ok || adapter == nil || capability.RequiresApproval {
			return nil, d.ErrInvalid
		}
		support := adapter.Support()
		if support.Profile == "" || support.BaseCalls < 1 || support.CallsPerInput < 0 || support.MaxTimeoutSeconds < 1 {
			return nil, d.ErrInvalid
		}
		capability.Execution = &support
		tasks.validator.capabilities[id] = capability
		r.adapters[id] = adapter
	}
	return r, nil
}

// WithOrchestration installs a trusted persisted strategy resolver. It cannot
// raise the approved plan concurrency or change node payloads and budgets.
func (r *Runtime) WithOrchestration(resolve func(context.Context, d.Scope, string, PlanSpec) (int, error)) *Runtime {
	r.orchestration = resolve
	return r
}
func (r *Runtime) orchestrationLimit(ctx context.Context, p d.Scope, id string, plan PlanSpec) (int, error) {
	if r.orchestration == nil {
		return plan.Budget.MaxConcurrency, nil
	}
	limit, err := r.orchestration(ctx, p, id, plan)
	if err != nil {
		return 0, err
	}
	if limit < 1 || limit > plan.Budget.MaxConcurrency {
		return 0, d.ErrInvalid
	}
	return limit, nil
}

type ExecutionBlocker struct {
	NodeID  string `json:"node_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ExecutionReadiness struct {
	TaskID           string             `json:"task_id"`
	TaskVersion      int64              `json:"task_version"`
	PlanVersion      int64              `json:"plan_version"`
	ExecutionEnabled bool               `json:"execution_enabled"`
	Allowed          bool               `json:"allowed"`
	Blockers         []ExecutionBlocker `json:"blockers"`
}
type StartInput struct {
	ExpectedVersion int64 `json:"expected_version"`
	PlanVersion     int64 `json:"plan_version"`
}
type CancelInput struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (r *Runtime) plan(ctx context.Context, p d.Scope, task *d.Task) (PlanSpec, error) {
	version := task.CurrentPlanVersion
	if task.ExecutionPlanVersion > 0 {
		version = task.ExecutionPlanVersion
	}
	stored, err := r.repo.GetPlan(ctx, p, task.ID, version)
	if err != nil {
		return PlanSpec{}, err
	}
	return decodeStored[PlanSpec](stored.Snapshot)
}

func (r *Runtime) Readiness(ctx context.Context, p d.Scope, id string) (*ExecutionReadiness, error) {
	task, err := r.tasks.Get(ctx, p, id)
	if err != nil {
		return nil, err
	}
	result := &ExecutionReadiness{TaskID: id, TaskVersion: task.Version, PlanVersion: task.CurrentPlanVersion, ExecutionEnabled: r.enabled, Blockers: []ExecutionBlocker{}}
	block := func(node, code, message string) {
		result.Blockers = append(result.Blockers, ExecutionBlocker{node, code, message})
	}
	if !r.enabled {
		block("", "runtime_disabled", "任务执行入口未启用。")
	}
	if task.StartedAt != nil || (task.Status != d.StatusDraft && task.Status != d.StatusReady) {
		block("", "task_state", "任务已启动或当前状态不允许启动。")
	}
	if task.CurrentPlanVersion == 0 {
		block("", "plan_required", "请先保存计划版本。")
	} else {
		plan, err := r.plan(ctx, p, task)
		if err != nil {
			return nil, err
		}
		if _, err = r.tasks.Validate(ctx, p, plan); err != nil {
			block("", "preflight_failed", "计划或输入的当前授权未通过预检。")
		}
		refs, err := decodeStored[[]l.RefID](task.InputRefs)
		if err != nil {
			return nil, err
		}
		outputs := 0
		for _, node := range plan.Nodes {
			outputs += len(node.OutputKinds)
			adapter, ok := r.adapters[node.Capability]
			if !ok {
				block(node.ID, "adapter_unavailable", "此能力尚未接通受控执行。 ")
				continue
			}
			if err := adapter.Validate(ctx, p, node); err != nil {
				block(node.ID, "preset_constraints", "节点不符合固定预置操作或调用／时间预算要求。 ")
				continue
			}
			if err := r.validateRootInputs(ctx, adapter, p, refs); err != nil {
				block(node.ID, "source_not_supported", "输入不可访问，或来源链尚未支持执行；仅支持可信手工输入、已接入的受控来源及其派生数据。")
			}
		}
		if outputs > 64 {
			block("", "output_limit", "本次执行最多声明 64 个输出类型。")
		}
	}
	current, err := r.tasks.Get(ctx, p, id)
	if err != nil {
		return nil, err
	}
	if current.Version != task.Version {
		return nil, d.ErrConflict
	}
	result.Allowed = len(result.Blockers) == 0
	return result, nil
}

func (r *Runtime) Start(ctx context.Context, p d.Scope, id string, in StartInput, key string) (*d.Task, error) {
	if in.ExpectedVersion < 1 || in.PlanVersion < 1 || !d.ValidText(key, 100) {
		return nil, d.ErrInvalid
	}
	task, err := r.tasks.Get(ctx, p, id)
	if err != nil {
		return nil, err
	}
	if task.CurrentPlanVersion != in.PlanVersion {
		return nil, d.ErrConflict
	}
	plan, err := r.plan(ctx, p, task)
	if err != nil {
		return nil, err
	}
	// Replaying an accepted start only reads that execution. The repository
	// checks its original key and plan, and never resets its deadline or state.
	if task.StartedAt == nil {
		if task.Version != in.ExpectedVersion {
			return nil, d.ErrConflict
		}
		ready, err := r.Readiness(ctx, p, id)
		if err != nil {
			return nil, err
		}
		if !ready.Allowed {
			issues := []Issue{}
			for _, b := range ready.Blockers {
				issues = append(issues, Issue{NodeID: b.NodeID, Field: "execution", Code: b.Code, Message: b.Message})
			}
			return nil, &ValidationError{Issues: issues}
		}
	}
	if _, err = r.orchestrationLimit(ctx, p, id, plan); err != nil {
		return nil, err
	}
	return r.repo.BeginExecution(ctx, p, id, in.ExpectedVersion, d.StartExecutionInput{PlanVersion: in.PlanVersion, IdempotencyKey: key, TimeoutSeconds: plan.Budget.TimeoutSeconds})
}

func (r *Runtime) Cancel(ctx context.Context, p d.Scope, id string, in CancelInput) (*d.Task, error) {
	if in.ExpectedVersion < 1 {
		return nil, d.ErrInvalid
	}
	if _, err := r.tasks.Get(ctx, p, id); err != nil {
		return nil, err
	}
	return r.repo.RequestCancel(ctx, p, id, in.ExpectedVersion)
}

func domainTerminal(status string) bool {
	switch status {
	case d.DomainSucceeded, d.DomainFailed, d.DomainTimedOut, d.DomainCancelled, d.DomainQuarantined:
		return true
	}
	return false
}
func childTerminal(child d.Child) bool {
	return child.Status == d.ChildCancelled || (child.Status == d.ChildBound && domainTerminal(child.DomainStatus))
}
func latestChildren(children []d.Child, version int64) map[string]d.Child {
	out := map[string]d.Child{}
	for _, child := range children {
		if child.PlanVersion <= version && child.Attempt > out[child.NodeID].Attempt {
			out[child.NodeID] = child
		}
	}
	return out
}
func nodeByID(plan PlanSpec, id string) (Node, bool) {
	for _, node := range plan.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

// Tick scans durable executions only. Merely reading a task never dispatches it.
func (r *Runtime) Tick(ctx context.Context) error {
	cursor := ""
	var failures []error
	for {
		page, err := r.repo.ListRunnable(ctx, d.ListFilter{Cursor: cursor, Limit: 50})
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := r.Reconcile(ctx, item.Scope, item.ID); err != nil && !errors.Is(err, d.ErrConflict) {
				failures = append(failures, fmt.Errorf("task reconciliation failed: %w", err))
			}
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return errors.Join(failures...)
}

// Reconcile leases one parent. The processing deadline is strictly shorter
// than its database lease; every mutation also checks the lease fence in SQL.
func (r *Runtime) Reconcile(parent context.Context, p d.Scope, id string) error {
	lease, err := r.repo.ClaimTask(parent, p, id, uuid.NewString(), 40*time.Second)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	fence := lease.Fence()
	defer func() {
		release, stop := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
		defer stop()
		_ = r.repo.ReleaseTaskLease(release, p, id, fence, time.Now().UTC().Add(2*time.Second))
	}()
	task, err := r.repo.GetTask(ctx, p, id)
	if err != nil {
		return err
	}
	if task.Status.Terminal() {
		return nil
	}
	plan, err := r.plan(ctx, p, task)
	if err != nil {
		return err
	}
	if task.StartedAt == nil || task.DeadlineAt == nil || task.ExecutionPlanVersion != task.CurrentPlanVersion {
		return d.ErrConflict
	}
	concurrencyLimit := plan.Budget.MaxConcurrency
	if task.CancelRequestedAt == nil {
		reason := ""
		if !time.Now().Before(*task.DeadlineAt) {
			reason = "budget_timeout"
		} else if _, e := r.tasks.Validate(ctx, p, plan); e != nil {
			reason = "authorization_or_plan_invalid"
		} else if e := r.validateSources(ctx, p, task, plan); e != nil {
			reason = "source_authorization_revoked"
		}
		if reason == "" {
			var strategyErr error
			concurrencyLimit, strategyErr = r.orchestrationLimit(ctx, p, id, plan)
			if strategyErr != nil {
				reason = "orchestration_policy_unavailable"
			}
		}
		if reason != "" {
			task, err = r.stop(ctx, p, task, fence, reason)
			if err != nil {
				return err
			}
		}
	}
	children, err := r.repo.ListChildren(ctx, p, id)
	if err != nil {
		return err
	}
	events, err := r.outbox(ctx, p, id)
	if err != nil {
		return err
	}
	for _, child := range children {
		if child.PlanVersion > task.ExecutionPlanVersion || childTerminal(child) {
			continue
		}
		event, ok := events[child.ID]
		if !ok {
			return d.ErrConflict
		}
		if err = r.syncChild(ctx, p, id, plan, child, event, fence); err != nil {
			if errors.Is(err, d.ErrForbidden) || errors.Is(err, d.ErrInvalid) {
				current, e := r.repo.GetTask(ctx, p, id)
				if e != nil {
					return e
				}
				if current.CancelRequestedAt == nil {
					_, e = r.stop(ctx, p, current, fence, "authorization_or_output_invalid")
				}
				if e != nil {
					return e
				}
				continue
			}
			return err
		}
	}
	task, err = r.repo.GetTask(ctx, p, id)
	if err != nil {
		return err
	}
	children, err = r.repo.ListChildren(ctx, p, id)
	if err != nil {
		return err
	}
	latest := latestChildren(children, task.ExecutionPlanVersion)
	if task.CancelRequestedAt != nil {
		return r.finishStopped(ctx, p, task, children, fence)
	}
	if task.Status == d.StatusReconcileRequired {
		task, err = r.repo.FinalizeTask(ctx, p, id, task.Version, d.TaskUpdate{Status: d.StatusRunning, Fence: fence})
		if err != nil {
			return err
		}
	}
	allSucceeded := true
	active := 0
	for _, child := range children {
		if !childTerminal(child) {
			active++
		}
	}
	for _, node := range plan.Nodes {
		child, exists := latest[node.ID]
		if exists && child.DomainStatus == d.DomainSucceeded {
			continue
		}
		allSucceeded = false
		if exists && childTerminal(child) && !(child.DomainStatus == d.DomainFailed || child.DomainStatus == d.DomainTimedOut) {
			_, err = r.stop(ctx, p, task, fence, "node_stopped:"+node.ID)
			return err
		}
		if exists && (child.DomainStatus == d.DomainFailed || child.DomainStatus == d.DomainTimedOut) && child.Attempt >= node.MaxAttempts {
			_, err = r.stop(ctx, p, task, fence, "node_failed:"+node.ID)
			return err
		}
	}
	if allSucceeded {
		return r.finalize(ctx, p, task, children, fence, d.StatusCompleted, "")
	}
	for _, node := range plan.Nodes {
		if active >= concurrencyLimit {
			break
		}
		child, exists := latest[node.ID]
		if exists && (!childTerminal(child) || child.DomainStatus == d.DomainSucceeded) {
			continue
		}
		ready := true
		for _, dependency := range node.DependsOn {
			if latest[dependency].DomainStatus != d.DomainSucceeded {
				ready = false
			}
		}
		if !ready {
			continue
		}
		adapter, ok := r.adapters[node.Capability]
		if !ok {
			_, err = r.stop(ctx, p, task, fence, "adapter_unavailable")
			return err
		}
		if err = adapter.Validate(ctx, p, node); err != nil {
			_, err = r.stop(ctx, p, task, fence, "preset_invalid")
			return err
		}
		refs, e := r.resolveBindings(ctx, p, node, latest)
		if e != nil {
			_, err = r.stop(ctx, p, task, fence, "input_binding_invalid")
			return err
		}
		if err = adapter.ValidateInputs(ctx, p, refs); err != nil {
			_, err = r.stop(ctx, p, task, fence, "source_authorization_revoked")
			return err
		}
		current, e := r.repo.GetTask(ctx, p, id)
		if e != nil {
			return e
		}
		if current.CancelRequestedAt != nil {
			return nil
		}
		if !time.Now().Before(*current.DeadlineAt) {
			_, err = r.stop(ctx, p, current, fence, "budget_timeout")
			return err
		}
		cmd := ExecutionCommand{Scope: p, TaskID: id, PlanVersion: task.ExecutionPlanVersion, Node: node, Attempt: child.Attempt + 1, InputRefs: refs, Deadline: *task.DeadlineAt, RequestID: task.RequestID}
		raw, e := json.Marshal(cmd)
		if e != nil {
			return e
		}
		_, err = r.repo.EnqueueChild(ctx, p, id, current.Version, d.DispatchInput{PlanVersion: cmd.PlanVersion, NodeID: node.ID, Attempt: cmd.Attempt, Domain: r.tasks.validator.capabilities[node.Capability].Domain, Payload: raw, Fence: fence})
		if err != nil {
			return err
		}
		active++
	}
	return nil
}

func validateRootInputs(ctx context.Context, adapter ExecutionAdapter, p d.Scope, refs []l.RefID) error {
	return validateInputBatches(ctx, adapter.ValidateInputs, p, refs)
}
func (r *Runtime) validateRootInputs(ctx context.Context, adapter ExecutionAdapter, p d.Scope, refs []l.RefID) error {
	if r.validateInputs != nil {
		return validateInputBatches(ctx, r.validateInputs, p, refs)
	}
	return validateRootInputs(ctx, adapter, p, refs)
}
func validateInputBatches(ctx context.Context, validate func(context.Context, d.Scope, []l.RefID) error, p d.Scope, refs []l.RefID) error {
	if len(refs) > 64 {
		return d.ErrInvalid
	}
	// A task may contain 64 roots even though each domain command takes 16.
	for start := 0; start < len(refs); start += 16 {
		end := start + 16
		if end > len(refs) {
			end = len(refs)
		}
		if err := validate(ctx, p, refs[start:end]); err != nil {
			return err
		}
	}
	return nil
}
func (r *Runtime) validateSources(ctx context.Context, p d.Scope, task *d.Task, plan PlanSpec) error {
	refs, err := decodeStored[[]l.RefID](task.InputRefs)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, node := range plan.Nodes {
		// Domain parameters may carry an explicit source (URL or KB scope), so
		// revalidate them as well as asset roots on every reconciliation.
		adapter := r.adapters[node.Capability]
		if adapter == nil {
			return d.ErrInvalid
		}
		if err := adapter.Validate(ctx, p, node); err != nil {
			return err
		}
		if seen[node.Capability] {
			continue
		}
		seen[node.Capability] = true
		if err := r.validateRootInputs(ctx, adapter, p, refs); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) outbox(ctx context.Context, p d.Scope, id string) (map[string]d.Outbox, error) {
	result := map[string]d.Outbox{}
	cursor := ""
	for {
		page, err := r.repo.ListOutbox(ctx, p, d.OutboxFilter{TaskID: id, ListFilter: d.ListFilter{Cursor: cursor, Limit: 100}})
		if err != nil {
			return nil, err
		}
		for _, event := range page.Items {
			result[event.ChildID] = event
		}
		if page.NextCursor == "" {
			return result, nil
		}
		cursor = page.NextCursor
	}
}

func (r *Runtime) command(p d.Scope, taskID string, plan PlanSpec, child d.Child, event d.Outbox) (ExecutionCommand, error) {
	cmd, err := decodeStored[ExecutionCommand](event.Payload)
	if err != nil {
		return cmd, err
	}
	canonical, err := d.CanonicalJSON(event.Payload, '{', d.MaxSnapshotBytes)
	if err != nil || d.Digest(canonical) != event.PayloadHash {
		return cmd, d.ErrInvalid
	}
	node, ok := nodeByID(plan, child.NodeID)
	a, _ := json.Marshal(node)
	b, _ := json.Marshal(cmd.Node)
	if !ok || string(a) != string(b) || cmd.TaskID != taskID || cmd.PlanVersion != child.PlanVersion || cmd.Attempt != child.Attempt || event.OperationKey != child.OperationKey {
		return cmd, d.ErrInvalid
	}
	cmd.Scope = p
	cmd.OperationKey = event.OperationKey
	cmd.PayloadHash = event.PayloadHash
	return cmd, nil
}

func (r *Runtime) syncChild(ctx context.Context, p d.Scope, id string, plan PlanSpec, child d.Child, event d.Outbox, fence d.ExecutionFence) error {
	cmd, err := r.command(p, id, plan, child, event)
	if err != nil {
		return err
	}
	adapter, ok := r.adapters[cmd.Node.Capability]
	if !ok {
		return d.ErrInvalid
	}
	task, err := r.repo.GetTask(ctx, p, id)
	if err != nil {
		return err
	}
	stopping := task.CancelRequestedAt != nil
	if event.Status == d.OutboxPending {
		if stopping {
			return nil
		}
		claimed, e := r.repo.ClaimOutboxWithLease(ctx, p, event.ID, event.Version, fence)
		if e != nil {
			return e
		}
		event = *claimed
	}
	if child.Status != d.ChildBound {
		handle, e := adapter.Lookup(ctx, cmd)
		if e == nil && handle == nil {
			// A cancellation tombstone fences Submit calls from an old worker.
			if stopping {
				handle, e = adapter.CancelSubmission(ctx, cmd)
			} else {
				current, readErr := r.repo.GetTask(ctx, p, id)
				if readErr != nil {
					return readErr
				}
				if current.CancelRequestedAt != nil {
					handle, e = adapter.CancelSubmission(ctx, cmd)
					stopping = true
				} else {
					handle, e = adapter.Submit(ctx, cmd)
				}
			}
		}
		if e != nil || handle == nil {
			_, markErr := r.repo.MarkDispatchUnknownWithLease(ctx, p, event.ID, event.Version, "领域提交结果待核对", fence)
			if markErr != nil {
				return markErr
			}
			if e != nil {
				return e
			}
			return d.ErrConflict
		}
		fresh, e := r.repo.GetChild(ctx, p, child.ID)
		if e != nil {
			return e
		}
		bound, e := r.repo.BindChild(ctx, p, child.ID, fresh.Version, child.Attempt, d.Binding{DomainID: handle.DomainID, Receipt: handle.Receipt, Fence: fence})
		if e != nil {
			return e
		}
		child = *bound
	}
	current, err := r.repo.GetTask(ctx, p, id)
	if err != nil {
		return err
	}
	stopping = stopping || current.CancelRequestedAt != nil
	handle := ExecutionHandle{DomainID: child.DomainID, Receipt: child.Receipt}
	var observation *DomainObservation
	if stopping {
		observation, err = adapter.Cancel(ctx, cmd, handle)
	} else {
		observation, err = adapter.Inspect(ctx, cmd, handle)
	}
	if err != nil {
		return err
	}
	if observation == nil {
		return d.ErrInvalid
	}
	refs := []l.RefID{}
	if !stopping {
		refs, err = r.checkedOutputs(ctx, p, cmd.Node, observation)
		if err != nil {
			return err
		}
	} else {
		// Cancellation may still report previously committed results. Keep only
		// currently authorized references, never revive access while stopping.
		for _, ref := range observation.OutputRefs {
			if _, e := r.tasks.validator.resolve(ctx, p, ref); e == nil {
				refs = append(refs, ref)
			}
		}
	}
	raw, _ := json.Marshal(refs)
	_, err = r.repo.ObserveChild(ctx, p, child.ID, child.Version, child.Attempt, d.ChildObservation{DomainID: child.DomainID, DomainStatus: observation.Status, OutputRefs: raw, Error: observation.Error, ObservedAt: observation.ObservedAt, Fence: fence})
	return err
}

func (r *Runtime) resolveBindings(ctx context.Context, p d.Scope, node Node, latest map[string]d.Child) ([]l.RefID, error) {
	refs := []l.RefID{}
	for _, input := range node.Inputs {
		if input.Asset != nil {
			refs = append(refs, *input.Asset)
			continue
		}
		child, ok := latest[input.FromNode]
		if !ok || child.DomainStatus != d.DomainSucceeded {
			return nil, d.ErrConflict
		}
		outputs, err := decodeStored[[]l.RefID](child.OutputRefs)
		if err != nil {
			return nil, err
		}
		matches := []l.RefID{}
		for _, ref := range outputs {
			kind, e := r.tasks.validator.resolve(ctx, p, ref)
			if e != nil {
				return nil, e
			}
			if kind == input.Kind {
				matches = append(matches, ref)
			}
		}
		if len(matches) != 1 {
			return nil, d.ErrInvalid
		}
		refs = append(refs, matches[0])
	}
	return refs, nil
}
func (r *Runtime) checkedOutputs(ctx context.Context, p d.Scope, node Node, observation *DomainObservation) ([]l.RefID, error) {
	refs := []l.RefID{}
	kinds := map[l.Kind]bool{}
	seen := map[string]bool{}
	if len(observation.OutputRefs) > 64 {
		return nil, d.ErrInvalid
	}
	for _, ref := range observation.OutputRefs {
		kind, err := r.tasks.validator.resolve(ctx, p, ref)
		if err != nil {
			return nil, d.ErrForbidden
		}
		if contains(node.OutputKinds, kind) && !seen[ref.String()] {
			refs = append(refs, ref)
			kinds[kind] = true
			seen[ref.String()] = true
		}
	}
	if observation.Status == d.DomainSucceeded {
		for _, kind := range node.OutputKinds {
			if !kinds[kind] {
				return nil, d.ErrInvalid
			}
		}
	}
	return refs, nil
}
func (r *Runtime) stop(ctx context.Context, p d.Scope, task *d.Task, fence d.ExecutionFence, reason string) (*d.Task, error) {
	return r.repo.FinalizeTask(ctx, p, task.ID, task.Version, d.TaskUpdate{Status: d.StatusCancelRequested, Reason: reason, Fence: fence})
}
func (r *Runtime) finishStopped(ctx context.Context, p d.Scope, task *d.Task, children []d.Child, fence d.ExecutionFence) error {
	for _, child := range children {
		if !childTerminal(child) {
			return nil
		}
	}
	state := d.StatusCancelled
	if task.Reason == "budget_timeout" {
		state = d.StatusTimedOut
	} else if task.Reason != "" && task.Reason != "user_cancelled" {
		state = d.StatusFailed
	}
	for _, child := range children {
		if child.DomainStatus == d.DomainSucceeded {
			state = d.StatusPartiallyCompleted
			break
		}
	}
	return r.finalize(ctx, p, task, children, fence, state, task.Reason)
}
func (r *Runtime) finalize(ctx context.Context, p d.Scope, task *d.Task, children []d.Child, fence d.ExecutionFence, state d.Status, reason string) error {
	refs := []l.RefID{}
	seen := map[string]bool{}
	plan, err := r.plan(ctx, p, task)
	if err != nil {
		return err
	}
	for _, child := range latestChildren(children, task.ExecutionPlanVersion) {
		if child.DomainStatus != d.DomainSucceeded {
			continue
		}
		outputs, err := decodeStored[[]l.RefID](child.OutputRefs)
		if len(child.OutputRefs) == 0 {
			continue
		}
		if err != nil {
			return err
		}
		node, ok := nodeByID(plan, child.NodeID)
		adapter := r.adapters[node.Capability]
		if !ok || adapter == nil {
			continue
		}
		for _, ref := range outputs {
			if !seen[ref.String()] {
				if e := authorizeExecutionOutputs(ctx, p, adapter, []l.RefID{ref}); e == nil {
					if _, e := r.tasks.validator.resolve(ctx, p, ref); e != nil {
						continue
					}
					refs = append(refs, ref)
					seen[ref.String()] = true
				}
			}
		}
	}
	if len(refs) > 64 {
		return d.ErrInvalid
	}
	raw, _ := json.Marshal(refs)
	_, err = r.repo.FinalizeTask(ctx, p, task.ID, task.Version, d.TaskUpdate{Status: state, Reason: reason, ResultRefs: raw, Fence: fence})
	return err
}
