package mysql

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	d "cognida/internal/model/taskcontrol"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskControlRepository struct{ db *gorm.DB }

func NewTaskControlRepository(db *gorm.DB) *TaskControlRepository {
	return &TaskControlRepository{db: db}
}

var _ d.Repository = (*TaskControlRepository)(nil)

type taskControlTaskRow struct {
	d.Task                      `gorm:"embedded"`
	IdempotencyKey, RequestHash string
}

func (taskControlTaskRow) TableName() string { return "task_control_tasks" }

type taskControlPlanRow struct {
	d.Plan  `gorm:"embedded"`
	d.Scope `gorm:"embedded"`
}

func (taskControlPlanRow) TableName() string { return "task_control_plans" }

type taskControlChildRow struct {
	d.Child     `gorm:"embedded"`
	RequestHash string
}

func (taskControlChildRow) TableName() string { return "task_control_children" }

type taskControlOutboxRow struct {
	d.Outbox `gorm:"embedded"`
}

func (taskControlOutboxRow) TableName() string { return "task_control_outbox" }

func tcError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return err
}
func tcScope(db *gorm.DB, scope d.Scope) *gorm.DB {
	return db.Where("tenant_id = ? AND user_id = ?", scope.TenantID, scope.UserID)
}
func tcID(id string) bool { parsed, err := uuid.Parse(id); return err == nil && parsed.String() == id }
func tcCheck(scope d.Scope, id string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if !tcID(id) {
		return d.ErrInvalid
	}
	return nil
}
func tcTask(db *gorm.DB, scope d.Scope, id string, lock bool) (*taskControlTaskRow, error) {
	q := tcScope(db, scope)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row taskControlTaskRow
	if err := q.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, tcError(err)
	}
	return &row, nil
}
func tcUpdateTask(db *gorm.DB, scope d.Scope, task *taskControlTaskRow, values map[string]any) error {
	values["version"] = gorm.Expr("version + 1")
	values["updated_at"] = time.Now().UTC()
	result := tcScope(db.Model(&taskControlTaskRow{}), scope).Where("id = ? AND version = ?", task.ID, task.Version).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
func (r *TaskControlRepository) CreateTask(ctx context.Context, scope d.Scope, in d.CreateTaskInput) (*d.Task, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	in, hash, err := d.NormalizeCreate(in)
	if err != nil {
		return nil, err
	}
	key := d.Digest([]byte(in.IdempotencyKey))
	now := time.Now().UTC()
	row := taskControlTaskRow{Task: d.Task{Scope: scope, ID: uuid.NewString(), Goal: in.Goal, InputRefs: in.InputRefs, Budget: in.Budget,
		ResultRefs: json.RawMessage(`[]`), RequestID: in.RequestID, Status: d.StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}, IdempotencyKey: key, RequestHash: hash}
	db := r.db.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, err
	}
	var stored taskControlTaskRow
	if err := tcScope(db, scope).Where("idempotency_key = ?", key).Take(&stored).Error; err != nil {
		return nil, tcError(err)
	}
	if stored.RequestHash != hash {
		return nil, d.ErrConflict
	}
	return &stored.Task, nil
}
func (r *TaskControlRepository) GetTask(ctx context.Context, scope d.Scope, id string) (*d.Task, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	row, err := tcTask(r.db.WithContext(ctx), scope, id, false)
	if err != nil {
		return nil, err
	}
	return &row.Task, nil
}
func tcPage(filter d.ListFilter) (string, int, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return "", 0, d.ErrInvalid
	}
	if filter.Cursor == "" {
		return "", limit, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(filter.Cursor)
	if err != nil || !tcID(string(raw)) {
		return "", 0, d.ErrInvalid
	}
	return string(raw), limit, nil
}
func tcCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
func (r *TaskControlRepository) ListTasks(ctx context.Context, scope d.Scope, filter d.ListFilter) (*d.TaskPage, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	after, limit, err := tcPage(filter)
	if err != nil {
		return nil, err
	}
	q := tcScope(r.db.WithContext(ctx), scope)
	if after != "" {
		q = q.Where("id > ?", after)
	}
	var rows []taskControlTaskRow
	if err := q.Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	page := &d.TaskPage{Items: []d.Task{}}
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextCursor = tcCursor(rows[len(rows)-1].ID)
	}
	for _, row := range rows {
		page.Items = append(page.Items, row.Task)
	}
	return page, nil
}
func (r *TaskControlRepository) AppendPlan(ctx context.Context, scope d.Scope, id string, expected int64, snapshot json.RawMessage) (*d.Plan, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 {
		return nil, d.ErrInvalid
	}
	body, err := d.CanonicalJSON(snapshot, '{', d.MaxSnapshotBytes)
	if err != nil {
		return nil, err
	}
	var out d.Plan
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.Version != expected || task.CancelRequestedAt != nil || (task.Status != d.StatusDraft && task.Status != d.StatusReady) {
			return d.ErrConflict
		}
		var count int64
		if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		// This foundation only permits plans before any dispatch preparation. Replacing
		// future nodes of an already executing plan requires the later controller.
		if count != 0 {
			return d.ErrConflict
		}
		out = d.Plan{TaskID: id, Version: task.CurrentPlanVersion + 1, Snapshot: body, ContentHash: d.Digest(body), CreatedAt: time.Now().UTC()}
		row := taskControlPlanRow{Plan: out, Scope: scope}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// A new snapshot cannot inherit the readiness/approval of the old plan.
		return tcUpdateTask(tx, scope, task, map[string]any{"current_plan_version": out.Version, "status": d.StatusDraft})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
func (r *TaskControlRepository) GetPlan(ctx context.Context, scope d.Scope, id string, version int64) (*d.Plan, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if version < 1 {
		return nil, d.ErrInvalid
	}
	var row taskControlPlanRow
	if err := tcScope(r.db.WithContext(ctx), scope).Where("task_id = ? AND version = ?", id, version).Take(&row).Error; err != nil {
		return nil, tcError(err)
	}
	return &row.Plan, nil
}

var tcToken = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]*$`)

func tcDispatch(scope d.Scope, taskID string, in d.DispatchInput) (json.RawMessage, string, string, error) {
	if in.PlanVersion < 1 || in.Attempt < 1 || in.Attempt > 100 || len(in.NodeID) > 128 || !tcToken.MatchString(in.NodeID) || len(in.Domain) > 64 || !tcToken.MatchString(in.Domain) {
		return nil, "", "", d.ErrInvalid
	}
	payload, err := d.CanonicalJSON(in.Payload, '{', d.MaxSnapshotBytes)
	if err != nil {
		return nil, "", "", err
	}
	identity, _ := json.Marshal([]any{scope.TenantID, scope.UserID, taskID, in.PlanVersion, in.NodeID, in.Attempt})
	request, _ := json.Marshal([]any{in.Domain, json.RawMessage(payload)})
	return payload, d.Digest(identity), d.Digest(request), nil
}
func (r *TaskControlRepository) EnqueueChild(ctx context.Context, scope d.Scope, id string, expected int64, in d.DispatchInput) (*d.Child, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 {
		return nil, d.ErrInvalid
	}
	payload, key, hash, err := tcDispatch(scope, id, in)
	if err != nil {
		return nil, err
	}
	var out d.Child
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		var existing taskControlChildRow
		err = tcScope(tx, scope).Where("operation_key = ?", key).Take(&existing).Error
		if err == nil {
			if existing.RequestHash != hash {
				return d.ErrConflict
			}
			out = existing.Child
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if task.Version != expected || task.CurrentPlanVersion != in.PlanVersion || task.CancelRequestedAt != nil || (task.Status != d.StatusReady && task.Status != d.StatusRunning) {
			return d.ErrConflict
		}
		// The service owns capability/permission/budget validation. Persistence
		// still fences the binding to a node in this exact immutable snapshot.
		var plan taskControlPlanRow
		if err := tcScope(tx, scope).Where("task_id = ? AND version = ?", id, in.PlanVersion).Take(&plan).Error; err != nil {
			return tcError(err)
		}
		var shape struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(plan.Snapshot, &shape); err != nil {
			return d.ErrInvalid
		}
		matches := 0
		for _, node := range shape.Nodes {
			if node.ID == in.NodeID {
				matches++
			}
		}
		if matches != 1 {
			return d.ErrInvalid
		}
		var unknown int64
		if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND node_id = ? AND status IN ?", id, in.NodeID, []d.ChildStatus{d.ChildPending, d.ChildDispatching, d.ChildReconcileRequired}).Count(&unknown).Error; err != nil {
			return err
		}
		if unknown > 0 {
			return d.ErrConflict
		}
		now := time.Now().UTC()
		out = d.Child{Scope: scope, ID: uuid.NewString(), TaskID: id, PlanVersion: in.PlanVersion, NodeID: in.NodeID, Attempt: in.Attempt, Domain: in.Domain, OperationKey: key, Status: d.ChildPending, Version: 1, CreatedAt: now, UpdatedAt: now}
		row := taskControlChildRow{Child: out, RequestHash: hash}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		event := taskControlOutboxRow{Outbox: d.Outbox{Scope: scope, ID: uuid.NewString(), TaskID: id, ChildID: out.ID, OperationKey: key, Payload: payload, PayloadHash: d.Digest(payload), Status: d.OutboxPending, Version: 1, CreatedAt: now, UpdatedAt: now}}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return tcUpdateTask(tx, scope, task, map[string]any{})
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
func (r *TaskControlRepository) GetChild(ctx context.Context, scope d.Scope, id string) (*d.Child, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	var row taskControlChildRow
	if err := tcScope(r.db.WithContext(ctx), scope).Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, tcError(err)
	}
	return &row.Child, nil
}
func (r *TaskControlRepository) ListChildren(ctx context.Context, scope d.Scope, taskID string) ([]d.Child, error) {
	if _, err := r.GetTask(ctx, scope, taskID); err != nil {
		return nil, err
	}
	var rows []taskControlChildRow
	if err := tcScope(r.db.WithContext(ctx), scope).Where("task_id = ?", taskID).Order("plan_version ASC,node_id ASC,attempt ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]d.Child, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Child)
	}
	return out, nil
}
func (r *TaskControlRepository) ListOutbox(ctx context.Context, scope d.Scope, filter d.OutboxFilter) (*d.OutboxPage, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return nil, d.ErrInvalid
	}
	after, limit, err := tcPage(filter.ListFilter)
	if err != nil {
		return nil, err
	}
	q := tcScope(r.db.WithContext(ctx), scope)
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if after != "" {
		q = q.Where("id > ?", after)
	}
	var rows []taskControlOutboxRow
	if err := q.Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	page := &d.OutboxPage{Items: []d.Outbox{}}
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextCursor = tcCursor(rows[len(rows)-1].ID)
	}
	for _, row := range rows {
		page.Items = append(page.Items, row.Outbox)
	}
	return page, nil
}

// All compound operations lock parent, child, then outbox. This serializes
// cancellation against claim without holding a database transaction across I/O.
func tcLockedDispatch(tx *gorm.DB, scope d.Scope, id string, byChild bool) (*taskControlTaskRow, *taskControlChildRow, *taskControlOutboxRow, error) {
	var event taskControlOutboxRow
	column := "id"
	if byChild {
		column = "child_id"
	}
	if err := tcScope(tx, scope).Where(column+" = ?", id).Take(&event).Error; err != nil {
		return nil, nil, nil, tcError(err)
	}
	task, err := tcTask(tx, scope, event.TaskID, true)
	if err != nil {
		return nil, nil, nil, err
	}
	var child taskControlChildRow
	if err := tcScope(tx, scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND task_id = ?", event.ChildID, event.TaskID).Take(&child).Error; err != nil {
		return nil, nil, nil, tcError(err)
	}
	if err := tcScope(tx, scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", event.ID).Take(&event).Error; err != nil {
		return nil, nil, nil, tcError(err)
	}
	if child.OperationKey != event.OperationKey {
		return nil, nil, nil, d.ErrConflict
	}
	return task, &child, &event, nil
}
func tcUpdateChild(tx *gorm.DB, scope d.Scope, child *taskControlChildRow, values map[string]any) error {
	values["version"] = gorm.Expr("version + 1")
	values["updated_at"] = time.Now().UTC()
	result := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("id = ? AND version = ?", child.ID, child.Version).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
func tcUpdateOutbox(tx *gorm.DB, scope d.Scope, event *taskControlOutboxRow, values map[string]any) error {
	values["version"] = gorm.Expr("version + 1")
	values["updated_at"] = time.Now().UTC()
	result := tcScope(tx.Model(&taskControlOutboxRow{}), scope).Where("id = ? AND version = ?", event.ID, event.Version).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
func (r *TaskControlRepository) ClaimOutbox(ctx context.Context, scope d.Scope, id string, expected int64) (*d.Outbox, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 {
		return nil, d.ErrInvalid
	}
	var out d.Outbox
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, child, event, err := tcLockedDispatch(tx, scope, id, false)
		if err != nil {
			return err
		}
		if event.Version != expected || event.Status != d.OutboxPending || child.Status != d.ChildPending || task.CancelRequestedAt != nil || task.CurrentPlanVersion != child.PlanVersion || (task.Status != d.StatusReady && task.Status != d.StatusRunning) {
			return d.ErrConflict
		}
		if err := tcUpdateChild(tx, scope, child, map[string]any{"status": d.ChildDispatching}); err != nil {
			return err
		}
		if err := tcUpdateOutbox(tx, scope, event, map[string]any{"status": d.OutboxDispatching}); err != nil {
			return err
		}
		if err := tcUpdateTask(tx, scope, task, map[string]any{}); err != nil {
			return err
		}
		if err := tcScope(tx, scope).Where("id = ?", id).Take(event).Error; err != nil {
			return err
		}
		out = event.Outbox
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
func (r *TaskControlRepository) MarkDispatchUnknown(ctx context.Context, scope d.Scope, id string, expected int64, reason string) (*d.Outbox, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 || !d.ValidText(reason, 2000) {
		return nil, d.ErrInvalid
	}
	var out d.Outbox
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, child, event, err := tcLockedDispatch(tx, scope, id, false)
		if err != nil {
			return err
		}
		if event.Version != expected || (event.Status != d.OutboxDispatching && event.Status != d.OutboxReconcileRequired) || (child.Status != d.ChildDispatching && child.Status != d.ChildReconcileRequired) {
			return d.ErrConflict
		}
		if err := tcUpdateChild(tx, scope, child, map[string]any{"status": d.ChildReconcileRequired}); err != nil {
			return err
		}
		if err := tcUpdateOutbox(tx, scope, event, map[string]any{"status": d.OutboxReconcileRequired, "reason": reason}); err != nil {
			return err
		}
		state := d.StatusReconcileRequired
		if task.CancelRequestedAt != nil {
			state = d.StatusCancelRequested
		}
		if err := tcUpdateTask(tx, scope, task, map[string]any{"status": state, "reason": reason}); err != nil {
			return err
		}
		if err := tcScope(tx, scope).Where("id = ?", id).Take(event).Error; err != nil {
			return err
		}
		out = event.Outbox
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
func (r *TaskControlRepository) BindChild(ctx context.Context, scope d.Scope, id string, expected int64, attempt int, binding d.Binding) (*d.Child, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 || attempt < 1 || !d.ValidText(binding.DomainID, 255) {
		return nil, d.ErrInvalid
	}
	receipt, err := d.CanonicalJSON(binding.Receipt, '{', d.MaxReceiptBytes)
	if err != nil {
		return nil, err
	}
	var out d.Child
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, child, event, err := tcLockedDispatch(tx, scope, id, true)
		if err != nil {
			return err
		}
		if child.Attempt != attempt {
			return d.ErrConflict
		}
		if child.Status == d.ChildBound {
			old, err := d.CanonicalJSON(child.Receipt, '{', d.MaxReceiptBytes)
			if err != nil {
				return err
			}
			if child.DomainID != binding.DomainID || d.Digest(old) != d.Digest(receipt) {
				return d.ErrConflict
			}
			out = child.Child
			return nil
		}
		if child.Version != expected || (child.Status != d.ChildDispatching && child.Status != d.ChildReconcileRequired) || (event.Status != d.OutboxDispatching && event.Status != d.OutboxReconcileRequired) {
			return d.ErrConflict
		}
		if err := tcUpdateChild(tx, scope, child, map[string]any{"status": d.ChildBound, "domain_id": binding.DomainID, "receipt": string(receipt)}); err != nil {
			return err
		}
		if err := tcUpdateOutbox(tx, scope, event, map[string]any{"status": d.OutboxDelivered}); err != nil {
			return err
		}
		// A real receipt is retained even after cancellation. It never automatically
		// changes the parent into success or starts a dependent node.
		if err := tcUpdateTask(tx, scope, task, map[string]any{}); err != nil {
			return err
		}
		if err := tcScope(tx, scope).Where("id = ?", id).Take(child).Error; err != nil {
			return err
		}
		out = child.Child
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
func (r *TaskControlRepository) RequestCancel(ctx context.Context, scope d.Scope, id string, expected int64) (*d.Task, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 {
		return nil, d.ErrInvalid
	}
	var out d.Task
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.CancelRequestedAt != nil {
			out = task.Task
			return nil
		}
		if task.Version != expected || task.Status.Terminal() {
			return d.ErrConflict
		}
		now := time.Now().UTC()
		if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND status = ?", id, d.ChildPending).Updates(map[string]any{"status": d.ChildCancelled, "version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tcScope(tx.Model(&taskControlOutboxRow{}), scope).Where("task_id = ? AND status = ?", id, d.OutboxPending).Updates(map[string]any{"status": d.OutboxCancelled, "version": gorm.Expr("version + 1"), "updated_at": now}).Error; err != nil {
			return err
		}
		var count int64
		if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND status <> ?", id, d.ChildCancelled).Count(&count).Error; err != nil {
			return err
		}
		state := d.StatusCancelled
		if count > 0 {
			state = d.StatusCancelRequested
		}
		if err := tcUpdateTask(tx, scope, task, map[string]any{"status": state, "cancel_requested_at": now}); err != nil {
			return err
		}
		stored, err := tcTask(tx, scope, id, false)
		if err != nil {
			return err
		}
		out = stored.Task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
func (r *TaskControlRepository) UpdateTaskStatus(ctx context.Context, scope d.Scope, id string, expected int64, in d.TaskUpdate) (*d.Task, error) {
	if err := tcCheck(scope, id); err != nil {
		return nil, err
	}
	if expected < 1 || !in.Status.Valid() || len(in.Reason) > 2000 {
		return nil, d.ErrInvalid
	}
	// Cancellation and unknown submission have dedicated transactional operations.
	if in.Status == d.StatusCancelRequested || in.Status == d.StatusReconcileRequired || in.Status == d.StatusCancelled {
		return nil, d.ErrInvalid
	}
	var refs json.RawMessage
	var err error
	if len(in.ResultRefs) > 0 {
		refs, err = d.CanonicalJSON(in.ResultRefs, '[', d.MaxReferencesBytes)
		if err != nil {
			return nil, err
		}
	}
	var out d.Task
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		task, err := tcTask(tx, scope, id, true)
		if err != nil {
			return err
		}
		if task.Version != expected || !d.CanTransition(task.Status, in.Status) {
			return d.ErrConflict
		}
		if task.CancelRequestedAt != nil && in.Status != d.StatusPartiallyCompleted && in.Status != d.StatusFailed {
			return d.ErrConflict
		}
		if (in.Status == d.StatusReady || in.Status == d.StatusRunning || in.Status == d.StatusCompleted) && task.CurrentPlanVersion == 0 {
			return d.ErrConflict
		}
		var unresolved int64
		if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND status IN ?", id, []d.ChildStatus{d.ChildPending, d.ChildDispatching, d.ChildReconcileRequired}).Count(&unresolved).Error; err != nil {
			return err
		}
		if unresolved > 0 && (in.Status.Terminal() || in.Status == d.StatusDraft || task.Status == d.StatusReconcileRequired) {
			return d.ErrConflict
		}
		if in.Status == d.StatusCompleted || in.Status == d.StatusPartiallyCompleted {
			var bound int64
			if err := tcScope(tx.Model(&taskControlChildRow{}), scope).Where("task_id = ? AND status = ?", id, d.ChildBound).Count(&bound).Error; err != nil {
				return err
			}
			if bound == 0 {
				return d.ErrConflict
			}
		}
		values := map[string]any{"status": in.Status, "reason": in.Reason}
		if refs != nil {
			values["result_refs"] = string(refs)
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
	if err != nil {
		return nil, err
	}
	return &out, nil
}
