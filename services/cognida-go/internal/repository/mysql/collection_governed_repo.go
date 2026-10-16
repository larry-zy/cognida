package mysql

import (
	d "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func collectionGovernedID(p l.Principal, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("collection-governed/%d/%d/%s", p.TenantID, p.UserID, key))).String()
}
func (r *CollectionRepository) FindGoverned(ctx context.Context, p l.Principal, key string) (*d.Run, error) {
	if p.Validate() != nil || len(key) != 64 {
		return nil, tc.ErrInvalid
	}
	var row CollectionRunModel
	id := collectionGovernedID(p, key)
	err := r.getDB(ctx).Table("collection_runs AS runs").Select("runs.*").Joins("JOIN collection_tasks AS tasks ON tasks.id=runs.task_id AND tasks.tenant_id=runs.tenant_id").Where("runs.id=? AND runs.tenant_id=? AND tasks.user_id=? AND tasks.mode=?", id, p.TenantID, p.UserID, d.SnapshotMode).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := row.ToDomain()
	if out.Governed == nil || out.Governed.OperationKey != key {
		return nil, tc.ErrConflict
	}
	return out, nil
}

// Task, explicit URL, run and cancellation tombstone commit under the same key.
func (r *CollectionRepository) SubmitGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation, cancel bool) (out *d.Run, err error) {
	if p.Validate() != nil {
		return nil, tc.ErrForbidden
	}
	if err = in.Validate(); err != nil {
		return
	}
	id := collectionGovernedID(p, in.OperationKey)
	err = r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := offlineNow(tx)
		if e != nil {
			return e
		}
		now = now.Truncate(time.Millisecond)
		task := CollectionTaskModel{ID: id, TenantID: p.TenantID, UserID: p.UserID, Mode: d.SnapshotMode, IdempotencyKey: "governed:" + id, Status: string(d.TaskStatusRunning), MaxSources: 1, MaxResponseBytes: in.MaxResponseBytes, CreatedAt: now, UpdatedAt: now}
		if cancel {
			task.Status = string(d.TaskStatusCancelled)
		}
		inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&task)
		if inserted.Error != nil {
			return inserted.Error
		}
		var stored CollectionTaskModel
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).First(&stored).Error; e != nil {
			return e
		}
		if stored.Mode != d.SnapshotMode {
			return tc.ErrConflict
		}
		if inserted.RowsAffected == 1 {
			src := CollectionSourceModel{ID: id, TaskID: id, URL: in.URL, CreatedAt: now}
			if e := tx.Create(&src).Error; e != nil {
				return e
			}
			run := d.Run{CandidateOnly: true, ID: id, TaskID: id, TenantID: p.TenantID, Governed: &in, Status: d.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
			if cancel {
				run.Status = d.RunStatusCancelled
				run.CompletedAt = &now
			}
			if e := tx.Create(fromCollectionRun(&run)).Error; e != nil {
				return e
			}
		}
		var row CollectionRunModel
		if e := tx.Where("id=? AND task_id=? AND tenant_id=?", id, id, p.TenantID).First(&row).Error; e != nil {
			return e
		}
		out = row.ToDomain()
		if out.Governed == nil || !d.SameGoverned(*out.Governed, in) {
			return tc.ErrConflict
		}
		if cancel && !out.Status.IsTerminal() {
			if e := tx.Model(&stored).Updates(map[string]any{"status": d.TaskStatusCancelled, "updated_at": now}).Error; e != nil {
				return e
			}
			if e := tx.Model(&row).Updates(map[string]any{"status": d.RunStatusCancelled, "completed_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
			out.Status = d.RunStatusCancelled
			out.CompletedAt = &now
			out.UpdatedAt = now
		}
		return nil
	})
	return
}
func (r *CollectionRepository) FinishGoverned(ctx context.Context, p l.Principal, id string, status d.RunStatus, refs []l.RefID, message string) (out *d.Run, err error) {
	if p.Validate() != nil || !status.IsTerminal() || len(refs) > 2 {
		return nil, tc.ErrInvalid
	}
	err = r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		var task CollectionTaskModel
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND mode=?", id, p.TenantID, p.UserID, d.SnapshotMode).First(&task).Error; e != nil {
			return e
		}
		var row CollectionRunModel
		if e := tx.Where("id=? AND task_id=? AND tenant_id=?", id, id, p.TenantID).First(&row).Error; e != nil {
			return e
		}
		out = row.ToDomain()
		if out.Governed == nil {
			return tc.ErrConflict
		}
		if out.Status.IsTerminal() {
			return nil
		}
		if task.Status == string(d.TaskStatusCancelled) {
			status = d.RunStatusCancelled
		}
		now, e := offlineNow(tx)
		if e != nil {
			return e
		}
		now = now.Truncate(time.Millisecond)
		deadline := out.Governed.Deadline
		local := out.StartedAt.Add(time.Duration(out.Governed.TimeoutSeconds) * time.Second)
		if local.Before(deadline) {
			deadline = local
		}
		if status == d.RunStatusCompleted && !now.Before(deadline) {
			status = d.RunStatusFailed
			message = "deadline_exceeded"
			refs = nil
		}
		if status != d.RunStatusCompleted {
			refs = nil
		}
		raw, _ := json.Marshal(refs)
		if e := tx.Model(&row).Updates(map[string]any{"status": status, "output_refs": raw, "error_message": message, "completed_at": now, "updated_at": now}).Error; e != nil {
			return e
		}
		if e := tx.Model(&task).Updates(map[string]any{"status": status, "error_message": message, "updated_at": now}).Error; e != nil {
			return e
		}
		out.Status = status
		out.OutputRefs = refs
		out.ErrorMessage = message
		out.CompletedAt = &now
		out.UpdatedAt = now
		return nil
	})
	return
}

var _ d.GovernedRepository = (*CollectionRepository)(nil)
