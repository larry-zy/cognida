package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	d "cognida/internal/model/taskcontrol"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ d.RuntimeRepository = (*TaskControlRepository)(nil)

type taskControlLeaseRow struct {
	d.Scope      `gorm:"embedded"`
	TaskID       string
	Owner        string
	Epoch        int64
	LeaseUntilUs int64
	NextPollUs   int64
}

func (taskControlLeaseRow) TableName() string { return "task_control_leases" }
func (l taskControlLeaseRow) public() *d.TaskLease {
	return &d.TaskLease{Scope: l.Scope, TaskID: l.TaskID, Owner: l.Owner, Epoch: l.Epoch, Until: time.UnixMicro(l.LeaseUntilUs).UTC(), NextPollAt: time.UnixMicro(l.NextPollUs).UTC()}
}
func tcClock(tx *gorm.DB) (int64, error) {
	var now int64
	err := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&now).Error
	return now, err
}
func tcLease(tx *gorm.DB, scope d.Scope, id string) (*taskControlLeaseRow, error) {
	var row taskControlLeaseRow
	err := tcScope(tx, scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("task_id = ?", id).Take(&row).Error
	return &row, tcError(err)
}
func tcCheckFence(tx *gorm.DB, scope d.Scope, task *taskControlTaskRow, f d.ExecutionFence) error {
	if task.StartedAt == nil {
		return nil
	} // First-phase records have never entered execution.
	if f.TaskID != task.ID || f.Epoch < 1 || len(f.Owner) > 128 || !tcToken.MatchString(f.Owner) {
		return d.ErrConflict
	}
	lease, err := tcLease(tx, scope, task.ID)
	if err != nil {
		return err
	}
	now, err := tcClock(tx)
	if err != nil {
		return err
	}
	if lease.Owner != f.Owner || lease.Epoch != f.Epoch || lease.LeaseUntilUs <= now {
		return d.ErrConflict
	}
	return nil
}
func tcExecutionOpen(tx *gorm.DB, scope d.Scope, task *taskControlTaskRow) error {
	if task.StartedAt == nil {
		return nil
	}
	// Compare instants decoded with the configured driver timezone, rather than
	// SQL wall-clock DATETIME against UTC_TIMESTAMP (production uses loc=Local).
	micros, err := tcClock(tx)
	if err != nil {
		return err
	}
	if task.DeadlineAt == nil || !task.DeadlineAt.After(time.UnixMicro(micros)) || task.ExecutionPlanVersion != task.CurrentPlanVersion {
		return d.ErrConflict
	}
	return nil
}

type tcRuntimeBudget struct {
	MaxNodes       int   `json:"max_nodes"`
	MaxConcurrency int   `json:"max_concurrency"`
	MaxAttempts    int   `json:"max_attempts"`
	MaxCalls       int   `json:"max_calls"`
	MaxCostMicros  int64 `json:"max_cost_micros"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}
type tcRuntimeNode struct {
	ID                      string `json:"id"`
	MaxAttempts             int    `json:"max_attempts"`
	MaxCallsPerAttempt      int    `json:"max_calls_per_attempt"`
	MaxCostMicrosPerAttempt int64  `json:"max_cost_micros_per_attempt"`
}
type tcRuntimePlan struct {
	Budget tcRuntimeBudget `json:"budget"`
	Nodes  []tcRuntimeNode `json:"nodes"`
}

func (b tcRuntimeBudget) valid() bool {
	return b.MaxNodes >= 1 && b.MaxNodes <= 32 && b.MaxConcurrency >= 1 && b.MaxConcurrency <= 8 && b.MaxConcurrency <= b.MaxNodes && b.MaxAttempts >= 1 && b.MaxAttempts <= 160 && b.MaxCalls >= 1 && b.MaxCalls <= 4096 && b.MaxCostMicros >= 0 && b.MaxCostMicros <= 1_000_000_000 && b.TimeoutSeconds >= 1 && b.TimeoutSeconds <= 86400
}
func tcReadRuntimePlan(tx *gorm.DB, scope d.Scope, id string, version int64) (*tcRuntimePlan, string, error) {
	var row taskControlPlanRow
	if err := tcScope(tx, scope).Where("task_id = ? AND version = ?", id, version).Take(&row).Error; err != nil {
		return nil, "", tcError(err)
	}
	var p tcRuntimePlan
	if json.Unmarshal(row.Snapshot, &p) != nil || !p.Budget.valid() || len(p.Nodes) < 1 || len(p.Nodes) > p.Budget.MaxNodes {
		return nil, "", d.ErrInvalid
	}
	seen := map[string]bool{}
	for _, n := range p.Nodes {
		if len(n.ID) > 128 || !tcToken.MatchString(n.ID) || seen[n.ID] || n.MaxAttempts < 1 || n.MaxAttempts > 5 || n.MaxCallsPerAttempt < 1 || n.MaxCallsPerAttempt > 4096 || n.MaxCostMicrosPerAttempt < 0 || n.MaxCostMicrosPerAttempt > 1_000_000_000 {
			return nil, "", d.ErrInvalid
		}
		seen[n.ID] = true
	}
	return &p, row.ContentHash, nil
}
func (r *TaskControlRepository) BeginExecution(ctx context.Context, scope d.Scope, id string, expected int64, in d.StartExecutionInput) (*d.Task, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 || in.PlanVersion < 1 || in.TimeoutSeconds < 1 || in.TimeoutSeconds > 86400 || !d.ValidText(in.IdempotencyKey, 256) {
		return nil, d.ErrInvalid
	}
	var out d.Task
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		plan, hash, err := tcReadRuntimePlan(tx, scope, id, in.PlanVersion)
		if err != nil {
			return err
		}
		request, _ := json.Marshal([]any{in.PlanVersion, hash, in.TimeoutSeconds})
		key, digest := d.Digest([]byte(in.IdempotencyKey)), d.Digest(request)
		if task.StartedAt != nil {
			if task.ExecutionKey != key || task.ExecutionHash != digest {
				return d.ErrConflict
			}
			out = task.Task
			return nil
		}
		if task.Version != expected || task.CurrentPlanVersion != in.PlanVersion || task.CancelRequestedAt != nil || (task.Status != d.StatusDraft && task.Status != d.StatusReady) {
			return d.ErrConflict
		}
		var budget tcRuntimeBudget
		if json.Unmarshal(task.Budget, &budget) != nil || !budget.valid() || in.TimeoutSeconds > budget.TimeoutSeconds || in.TimeoutSeconds > plan.Budget.TimeoutSeconds {
			return d.ErrInvalid
		}
		if plan.Budget.MaxNodes > budget.MaxNodes || plan.Budget.MaxConcurrency > budget.MaxConcurrency || plan.Budget.MaxAttempts > budget.MaxAttempts || plan.Budget.MaxCalls > budget.MaxCalls || plan.Budget.MaxCostMicros > budget.MaxCostMicros {
			return d.ErrInvalid
		}
		var count int64
		if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return d.ErrConflict
		}
		micros, err := tcClock(tx)
		if err != nil {
			return err
		}
		now := time.UnixMicro(micros).UTC()
		if err := tcUpdateTask(tx, scope, task, map[string]any{"status": d.StatusRunning, "execution_plan_version": in.PlanVersion, "execution_key": key, "execution_hash": digest, "started_at": now, "deadline_at": now.Add(time.Duration(in.TimeoutSeconds) * time.Second), "reason": ""}); err != nil {
			return err
		}
		if err := tx.Create(&taskControlLeaseRow{Scope: scope, TaskID: id, NextPollUs: micros}).Error; err != nil {
			return err
		}
		stored, err := tcTask(tx, scope, id, false)
		if err != nil {
			return err
		}
		out = stored.Task
		return nil
	})
	return &out, err
}
func tcValidTTL(ttl time.Duration) bool { return ttl >= 100*time.Millisecond && ttl <= 5*time.Minute }
func (r *TaskControlRepository) ClaimTask(ctx context.Context, scope d.Scope, id, owner string, ttl time.Duration) (*d.TaskLease, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if !tcValidTTL(ttl) || len(owner) > 128 || !tcToken.MatchString(owner) {
		return nil, d.ErrInvalid
	}
	var out *d.TaskLease
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.StartedAt == nil || task.Status.Terminal() {
			return d.ErrConflict
		}
		lease, err := tcLease(tx, scope, id)
		if err != nil {
			return err
		}
		now, err := tcClock(tx)
		if err != nil {
			return err
		}
		if lease.LeaseUntilUs > now || lease.NextPollUs > now {
			return d.ErrConflict
		}
		lease.Owner = owner
		lease.Epoch++
		lease.LeaseUntilUs = now + ttl.Microseconds()
		if err := tcScope(tx, scope).Model(&taskControlLeaseRow{}).Where("task_id = ?", id).Updates(map[string]any{"owner": owner, "epoch": lease.Epoch, "lease_until_us": lease.LeaseUntilUs}).Error; err != nil {
			return err
		}
		out = lease.public()
		return nil
	})
	return out, err
}
func (r *TaskControlRepository) RenewTaskLease(ctx context.Context, scope d.Scope, id string, f d.ExecutionFence, ttl time.Duration) (*d.TaskLease, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if !tcValidTTL(ttl) {
		return nil, d.ErrInvalid
	}
	var out *d.TaskLease
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.StartedAt == nil || task.Status.Terminal() {
			return d.ErrConflict
		}
		if err := tcCheckFence(tx, scope, task, f); err != nil {
			return err
		}
		lease, err := tcLease(tx, scope, id)
		if err != nil {
			return err
		}
		now, err := tcClock(tx)
		if err != nil {
			return err
		}
		lease.LeaseUntilUs = now + ttl.Microseconds()
		if err := tcScope(tx.Model(&taskControlLeaseRow{}), scope).Where("task_id = ?", id).Update("lease_until_us", lease.LeaseUntilUs).Error; err != nil {
			return err
		}
		out = lease.public()
		return nil
	})
	return out, err
}
func (r *TaskControlRepository) ReleaseTaskLease(ctx context.Context, scope d.Scope, id string, f d.ExecutionFence, next time.Time) error {
	if err := tcCheck(scope, id); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.StartedAt == nil {
			return d.ErrConflict
		}
		if err := tcCheckFence(tx, scope, task, f); err != nil {
			return err
		}
		now, err := tcClock(tx)
		if err != nil {
			return err
		}
		nextUs := next.UnixMicro()
		if next.IsZero() || nextUs < now {
			nextUs = now
		}
		if nextUs > now+time.Hour.Microseconds() {
			return d.ErrInvalid
		}
		return tcScope(tx.Model(&taskControlLeaseRow{}), scope).Where("task_id = ?", id).Updates(map[string]any{"owner": "", "lease_until_us": now, "next_poll_us": nextUs}).Error
	})
}
func (r *TaskControlRepository) ListRunnable(ctx context.Context, filter d.ListFilter) (*d.RunnableTaskPage, error) {
	after, limit, err := tcPage(filter)
	if err != nil {
		return nil, err
	}
	// This explicit global scope is confined to the worker's locator-only scan.
	q := r.db.WithContext(ctx).Scopes(GlobalScope()).Table("task_control_tasks AS t").Select("t.id,t.tenant_id,t.user_id,t.version").Joins("JOIN task_control_leases AS l ON l.tenant_id=t.tenant_id AND l.user_id=t.user_id AND l.task_id=t.id").Where("t.started_at IS NOT NULL AND t.status IN ?", []d.Status{d.StatusRunning, d.StatusAwaitingApproval, d.StatusCancelRequested, d.StatusReconcileRequired}).Where("l.lease_until_us <= CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED) AND l.next_poll_us <= CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)")
	if after != "" {
		q = q.Where("t.id > ?", after)
	}
	out := &d.RunnableTaskPage{Items: []d.RunnableTask{}}
	if err := q.Order("t.id ASC").Limit(limit + 1).Scan(&out.Items).Error; err != nil {
		return nil, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = tcCursor(out.Items[limit-1].ID)
	}
	return out, nil
}
func tcDomainTerminal(status string) bool {
	switch status {
	case d.DomainSucceeded, d.DomainFailed, d.DomainTimedOut, d.DomainCancelled, d.DomainQuarantined:
		return true
	}
	return false
}
func tcDomainValid(status string) bool {
	return tcDomainTerminal(status) || status == d.DomainUnknown || status == d.DomainQueued || status == d.DomainRunning || status == d.DomainAwaitingApproval
}
func (r *TaskControlRepository) ObserveChild(ctx context.Context, scope d.Scope, id string, expected int64, attempt int, in d.ChildObservation) (*d.Child, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 || attempt < 1 || attempt > 5 || !tcDomainValid(in.DomainStatus) || !d.ValidText(in.DomainID, 255) || !utf8.ValidString(in.Error) || len(in.Error) > 2000 || in.ObservedAt.IsZero() {
		return nil, d.ErrInvalid
	}
	refs := in.OutputRefs
	if len(refs) == 0 {
		refs = json.RawMessage(`[]`)
	}
	refs, err := d.CanonicalJSON(refs, '[', d.MaxReferencesBytes)
	if err != nil {
		return nil, err
	}
	var entries []json.RawMessage
	if json.Unmarshal(refs, &entries) != nil || len(entries) > 160 {
		return nil, d.ErrInvalid
	}
	observed := in.ObservedAt.UTC().Truncate(time.Microsecond)
	var out d.Child
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, child, _, err := tcLockedDispatch(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.StartedAt == nil || task.Status.Terminal() {
			return d.ErrConflict
		}
		if err := tcCheckFence(tx, scope, task, in.Fence); err != nil {
			return err
		}
		if child.Status != d.ChildBound || child.DomainID != in.DomainID || child.Attempt != attempt {
			return d.ErrConflict
		}
		if child.ObservedAt != nil {
			if observed.Before(*child.ObservedAt) {
				return d.ErrConflict
			}
			if observed.Equal(*child.ObservedAt) {
				old := child.OutputRefs
				if len(old) == 0 {
					old = json.RawMessage(`[]`)
				}
				old, _ = d.CanonicalJSON(old, '[', d.MaxReferencesBytes)
				if child.DomainStatus != in.DomainStatus || string(old) != string(refs) || child.Error != in.Error {
					return d.ErrConflict
				}
				out = child.Child
				return nil
			}
		}
		if child.Version != expected || (tcDomainTerminal(child.DomainStatus) && child.DomainStatus != in.DomainStatus) {
			return d.ErrConflict
		}
		if err := tcUpdateChild(tx, scope, child, map[string]any{"domain_status": in.DomainStatus, "output_refs": string(refs), "error": in.Error, "observed_at": observed}); err != nil {
			return err
		}
		if err := tcUpdateTask(tx, scope, task, map[string]any{}); err != nil {
			return err
		}
		if err := tcScope(tx, scope).Where("id = ?", id).Take(child).Error; err != nil {
			return err
		}
		out = child.Child
		return nil
	})
	return &out, err
}
func tcCancelPending(tx *gorm.DB, scope d.Scope, id string, now time.Time) error {
	if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND status = ?", id, d.ChildPending).Updates(map[string]any{"status": d.ChildCancelled, "version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
		return err
	}
	return tcScope(tx.Model(&taskControlOutboxRow{}), scope).Where("task_id = ? AND status = ?", id, d.OutboxPending).Updates(map[string]any{"status": d.OutboxCancelled, "version": gorm.Expr("version + 1"), "updated_at": now}).Error
}
func (r *TaskControlRepository) FinalizeTask(ctx context.Context, scope d.Scope, id string, expected int64, in d.TaskUpdate) (*d.Task, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 || (!in.Status.Terminal() && in.Status != d.StatusCancelRequested && in.Status != d.StatusRunning) || len(in.Reason) > 2000 || !utf8.ValidString(in.Reason) {
		return nil, d.ErrInvalid
	}
	refs := in.ResultRefs
	if len(refs) == 0 {
		refs = json.RawMessage(`[]`)
	}
	refs, err := d.CanonicalJSON(refs, '[', d.MaxReferencesBytes)
	if err != nil {
		return nil, err
	}
	var resultEntries []json.RawMessage
	if json.Unmarshal(refs, &resultEntries) != nil || len(resultEntries) > 160 {
		return nil, d.ErrInvalid
	}
	var out d.Task
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.StartedAt == nil || task.Version != expected || !d.CanTransition(task.Status, in.Status) {
			return d.ErrConflict
		}
		if err := tcCheckFence(tx, scope, task, in.Fence); err != nil {
			return err
		}
		now := time.Now().UTC()
		values := map[string]any{"status": in.Status, "reason": in.Reason}
		if in.Status == d.StatusRunning {
			if task.Status != d.StatusReconcileRequired || task.CancelRequestedAt != nil {
				return d.ErrConflict
			}
			var unknown int64
			if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND status IN ?", id, []d.ChildStatus{d.ChildDispatching, d.ChildReconcileRequired}).Count(&unknown).Error; err != nil {
				return err
			}
			if unknown > 0 {
				return d.ErrConflict
			}
			if err := tcScope(tx.Model(&taskControlOutboxRow{}), scope).Where("task_id = ? AND status IN ?", id, []d.OutboxStatus{d.OutboxDispatching, d.OutboxReconcileRequired}).Count(&unknown).Error; err != nil {
				return err
			}
			if unknown > 0 {
				return d.ErrConflict
			}
			values["reason"] = ""
		} else if in.Status == d.StatusCancelRequested {
			if in.Reason == "" {
				return d.ErrInvalid
			}
			if err := tcCancelPending(tx, scope, id, now); err != nil {
				return err
			}
			if task.CancelRequestedAt == nil {
				values["cancel_requested_at"] = now
			} else {
				values["reason"] = task.Reason
			}
		} else {
			var children []taskControlChildRow
			if err := tcScope(tx, scope).Where("task_id = ?", id).Limit(161).Find(&children).Error; err != nil {
				return err
			}
			if len(children) > 160 {
				return d.ErrConflict
			}
			latest := map[string]d.Child{}
			allowedRefs := map[string]bool{}
			successes := 0
			for _, c := range children {
				if c.Status != d.ChildCancelled && (c.Status != d.ChildBound || !tcDomainTerminal(c.DomainStatus)) {
					return d.ErrConflict
				}
				if c.DomainStatus == d.DomainSucceeded {
					successes++
					var entries []json.RawMessage
					if json.Unmarshal(c.OutputRefs, &entries) == nil {
						for _, entry := range entries {
							var v any
							dec := json.NewDecoder(bytes.NewReader(entry))
							dec.UseNumber()
							if dec.Decode(&v) == nil {
								b, _ := json.Marshal(v)
								allowedRefs[string(b)] = true
							}
						}
					}
				}
				if old, ok := latest[c.NodeID]; !ok || old.Attempt < c.Attempt {
					latest[c.NodeID] = c.Child
				}
			}
			for _, entry := range resultEntries {
				var v any
				dec := json.NewDecoder(bytes.NewReader(entry))
				dec.UseNumber()
				if dec.Decode(&v) != nil {
					return d.ErrInvalid
				}
				b, _ := json.Marshal(v)
				if !allowedRefs[string(b)] {
					return d.ErrConflict
				}
			}
			switch in.Status {
			case d.StatusCompleted:
				if task.CancelRequestedAt != nil {
					return d.ErrConflict
				}
				plan, _, err := tcReadRuntimePlan(tx, scope, id, task.ExecutionPlanVersion)
				if err != nil {
					return err
				}
				for _, n := range plan.Nodes {
					if latest[n.ID].DomainStatus != d.DomainSucceeded {
						return d.ErrConflict
					}
				}
			case d.StatusPartiallyCompleted:
				if successes == 0 {
					return d.ErrConflict
				}
			case d.StatusCancelled:
				if task.CancelRequestedAt == nil || successes > 0 {
					return d.ErrConflict
				}
			case d.StatusFailed, d.StatusTimedOut:
				if successes > 0 {
					return d.ErrConflict
				}
			}
			values["result_refs"] = string(refs)
			values["finished_at"] = now
		}
		if err := tcUpdateTask(tx, scope, task, values); err != nil {
			return err
		}
		stored, err := tcTask(tx, scope, id, false)
		if err != nil {
			return err
		}
		out = stored.Task
		return nil
	})
	return &out, err
}

// A reservation is derived from the immutable plan, never reported as actual use.
func tcReserveDispatch(tx *gorm.DB, scope d.Scope, task *taskControlTaskRow, in d.DispatchInput) (int, int64, error) {
	if task.StartedAt == nil {
		return 0, 0, nil
	}
	if in.PlanVersion != task.ExecutionPlanVersion || in.Attempt > 5 {
		return 0, 0, d.ErrConflict
	}
	plan, _, err := tcReadRuntimePlan(tx, scope, task.ID, in.PlanVersion)
	if err != nil {
		return 0, 0, err
	}
	var node *tcRuntimeNode
	for i := range plan.Nodes {
		if plan.Nodes[i].ID == in.NodeID {
			node = &plan.Nodes[i]
		}
	}
	if node == nil || in.Attempt > node.MaxAttempts {
		return 0, 0, d.ErrConflict
	}
	var children []taskControlChildRow
	if err := tcScope(tx, scope).Where("task_id = ?", task.ID).Limit(161).Find(&children).Error; err != nil {
		return 0, 0, err
	}
	if len(children) >= 160 || len(children) >= plan.Budget.MaxAttempts {
		return 0, 0, d.ErrConflict
	}
	calls, cost, active := node.MaxCallsPerAttempt, node.MaxCostMicrosPerAttempt, 0
	var previous *taskControlChildRow
	for i := range children {
		c := &children[i]
		calls += c.CallsReserved
		cost += c.CostMicrosReserved
		if c.Status != d.ChildCancelled && (c.Status != d.ChildBound || !tcDomainTerminal(c.DomainStatus)) {
			active++
		}
		if c.NodeID == in.NodeID && (previous == nil || previous.Attempt < c.Attempt) {
			previous = c
		}
	}
	if active >= plan.Budget.MaxConcurrency || calls > plan.Budget.MaxCalls || cost > plan.Budget.MaxCostMicros {
		return 0, 0, d.ErrConflict
	}
	if previous == nil {
		if in.Attempt != 1 {
			return 0, 0, d.ErrConflict
		}
	} else if in.Attempt != previous.Attempt+1 || previous.Status != d.ChildBound || (previous.DomainStatus != d.DomainFailed && previous.DomainStatus != d.DomainTimedOut) {
		return 0, 0, d.ErrConflict
	}
	return node.MaxCallsPerAttempt, node.MaxCostMicrosPerAttempt, nil
}
