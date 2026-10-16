package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	d "cognida/internal/model/taskcontrol"
	"gorm.io/gorm"
)

type taskControlReplanRow struct {
	d.ReplanReceipt `gorm:"embedded"`
}

func (taskControlReplanRow) TableName() string { return "task_control_replans" }

var _ d.ReplanRepository = (*TaskControlRepository)(nil)

func (r *TaskControlRepository) LookupReplan(ctx context.Context, p d.Scope, id, key, hash string) (*d.Plan, error) {
	if tcCheck(p, id) != nil || len(key) != 64 || len(hash) != 64 {
		return nil, d.ErrInvalid
	}
	var row taskControlReplanRow
	e := tcScope(r.db.WithContext(ctx), p).Where("task_id=? AND operation_key=?", id, key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if row.RequestHash != hash {
		return nil, d.ErrConflict
	}
	return r.GetPlan(ctx, p, id, row.PlanVersion)
}
func tcPlanNodes(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var plan struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if json.Unmarshal(raw, &plan) != nil || len(plan.Nodes) < 1 || len(plan.Nodes) > 32 {
		return nil, d.ErrInvalid
	}
	out := map[string]json.RawMessage{}
	for _, raw := range plan.Nodes {
		var node struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &node) != nil || node.ID == "" {
			return nil, d.ErrInvalid
		}
		if _, exists := out[node.ID]; exists {
			return nil, d.ErrInvalid
		}
		canonical, e := d.CanonicalJSON(raw, '{', d.MaxSnapshotBytes)
		if e != nil {
			return nil, e
		}
		out[node.ID] = canonical
	}
	return out, nil
}
func tcFrozenChild(tx *gorm.DB, p d.Scope, task *taskControlTaskRow, child *taskControlChildRow) error {
	if task.StartedAt == nil {
		if child.PlanVersion == task.CurrentPlanVersion {
			return nil
		}
		return d.ErrConflict
	}
	if child.PlanVersion == task.ExecutionPlanVersion {
		return nil
	}
	if child.PlanVersion > task.ExecutionPlanVersion {
		return d.ErrConflict
	}
	var old, current taskControlPlanRow
	if e := tcScope(tx, p).Where("task_id=? AND version=?", task.ID, child.PlanVersion).Take(&old).Error; e != nil {
		return tcError(e)
	}
	if e := tcScope(tx, p).Where("task_id=? AND version=?", task.ID, task.ExecutionPlanVersion).Take(&current).Error; e != nil {
		return tcError(e)
	}
	a, e := tcPlanNodes(old.Snapshot)
	if e != nil {
		return e
	}
	b, e := tcPlanNodes(current.Snapshot)
	if e != nil {
		return e
	}
	if len(a[child.NodeID]) == 0 || string(a[child.NodeID]) != string(b[child.NodeID]) {
		return d.ErrConflict
	}
	return nil
}
func (r *TaskControlRepository) ReplanExecution(ctx context.Context, p d.Scope, id string, expected int64, in d.ReplanCommand) (out *d.Plan, err error) {
	if tcCheck(p, id) != nil || expected < 1 || len(in.IdempotencyKey) != 64 || len(in.RequestHash) != 64 || !d.ValidText(in.Reason, 2000) {
		return nil, d.ErrInvalid
	}
	body, e := d.CanonicalJSON(in.Snapshot, '{', d.MaxSnapshotBytes)
	if e != nil {
		return nil, e
	}
	var candidate tcRuntimePlan
	if json.Unmarshal(body, &candidate) != nil || !candidate.Budget.valid() || len(candidate.Nodes) > candidate.Budget.MaxNodes {
		return nil, d.ErrInvalid
	}
	newNodes, e := tcPlanNodes(body)
	if e != nil {
		return nil, e
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, e := tcTask(tx, p, id, true)
		if e != nil {
			return e
		}
		var prior taskControlReplanRow
		e = tcScope(tx, p).Where("task_id=? AND operation_key=?", id, in.IdempotencyKey).Take(&prior).Error
		if e == nil {
			if prior.RequestHash != in.RequestHash {
				return d.ErrConflict
			}
			var plan taskControlPlanRow
			if e = tcScope(tx, p).Where("task_id=? AND version=?", id, prior.PlanVersion).Take(&plan).Error; e != nil {
				return tcError(e)
			}
			out = &plan.Plan
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if task.Version != expected || task.StartedAt == nil || task.CancelRequestedAt != nil || (task.Status != d.StatusRunning && task.Status != d.StatusReconcileRequired) || task.ExecutionPlanVersion != task.CurrentPlanVersion {
			return d.ErrConflict
		}
		if e = tcExecutionOpen(tx, p, task); e != nil {
			return e
		}
		var old taskControlPlanRow
		if e = tcScope(tx, p).Where("task_id=? AND version=?", id, task.ExecutionPlanVersion).Take(&old).Error; e != nil {
			return tcError(e)
		}
		var priorPlan tcRuntimePlan
		if json.Unmarshal(old.Snapshot, &priorPlan) != nil || candidate.Budget != priorPlan.Budget {
			return d.ErrInvalid
		}
		oldNodes, e := tcPlanNodes(old.Snapshot)
		if e != nil {
			return e
		}
		var children []taskControlChildRow
		if e = tcScope(tx, p).Where("task_id=?", id).Find(&children).Error; e != nil {
			return e
		}
		for _, child := range children {
			if len(oldNodes[child.NodeID]) == 0 || string(oldNodes[child.NodeID]) != string(newNodes[child.NodeID]) {
				return d.ErrConflict
			}
		}
		if d.Digest(body) == old.ContentHash {
			return d.ErrConflict
		}
		lease, e := tcLease(tx, p, id)
		if e != nil {
			return e
		}
		micros, e := tcClock(tx)
		if e != nil {
			return e
		}
		now := time.UnixMicro(micros).UTC()
		plan := d.Plan{TaskID: id, Version: task.CurrentPlanVersion + 1, Snapshot: body, ContentHash: d.Digest(body), CreatedAt: now}
		if e = tx.Create(&taskControlPlanRow{Plan: plan, Scope: p}).Error; e != nil {
			return e
		}
		receipt := taskControlReplanRow{d.ReplanReceipt{Scope: p, TaskID: id, OperationKey: in.IdempotencyKey, RequestHash: in.RequestHash, FromVersion: task.ExecutionPlanVersion, PlanVersion: plan.Version, Reason: in.Reason, CreatedAt: now}}
		if e = tx.Create(&receipt).Error; e != nil {
			return e
		}
		// An old worker may already be inside an idempotent domain Submit. Its child
		// stays bound to the same operation, but every subsequent SQL mutation is fenced.
		if e = tcScope(tx.Model(&taskControlLeaseRow{}), p).Where("task_id=?", id).Updates(map[string]any{"owner": "", "epoch": lease.Epoch + 1, "lease_until_us": 0, "next_poll_us": 0}).Error; e != nil {
			return e
		}
		if e = tcUpdateTask(tx, p, task, map[string]any{"current_plan_version": plan.Version, "execution_plan_version": plan.Version, "status": d.StatusRunning}); e != nil {
			return e
		}
		out = &plan
		return nil
	})
	return
}
