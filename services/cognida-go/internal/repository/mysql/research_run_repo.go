package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ResearchRunRepository struct{ db *gorm.DB }

func NewResearchRunRepository(db *gorm.DB) *ResearchRunRepository { return &ResearchRunRepository{db} }

var _ r.Repository = (*ResearchRunRepository)(nil)

func researchScope(db *gorm.DB, s d.Scope) *gorm.DB {
	return db.Where("tenant_id = ? AND user_id = ?", s.TenantID, s.UserID)
}
func researchErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return e
}
func researchNow(tx *gorm.DB) (time.Time, error) {
	var us int64
	e := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6))*1000000 AS SIGNED)").Scan(&us).Error
	return time.UnixMicro(us).UTC(), e
}
func researchSame(a, b json.RawMessage) bool {
	x, e := d.CanonicalJSON(a, '{', 64<<10)
	y, f := d.CanonicalJSON(b, '{', 64<<10)
	return e == nil && f == nil && string(x) == string(y)
}
func (rpo *ResearchRunRepository) Submit(ctx context.Context, s d.Scope, in r.Submission, cancel bool) (*r.Run, error) {
	if s.Validate() != nil || in.Validate() != nil {
		return nil, d.ErrInvalid
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return nil, e
	}
	var result *r.Run
	e = rpo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := researchNow(tx)
		if e != nil {
			return e
		}
		status := "queued"
		if cancel {
			status = "cancelled"
		}
		row := r.Run{Scope: s, ID: uuid.NewString(), OperationKey: in.OperationKey, PayloadHash: in.PayloadHash, Submission: raw, Status: status, Version: 1, OutputRefs: json.RawMessage(`[]`), CreatedAt: now, UpdatedAt: now}
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return e
		}
		var stored r.Run
		if e = researchScope(tx, s).Clauses(clause.Locking{Strength: "UPDATE"}).Where("operation_key = ?", in.OperationKey).Take(&stored).Error; e != nil {
			return researchErr(e)
		}
		if stored.PayloadHash != in.PayloadHash || !researchSame(stored.Submission, raw) {
			return d.ErrConflict
		}
		if cancel && !stored.Terminal() {
			if e = researchCancelRow(tx, &stored, now); e != nil {
				return e
			}
		}
		result = &stored
		return nil
	})
	return result, e
}
func (rpo *ResearchRunRepository) Get(ctx context.Context, s d.Scope, id string) (*r.Run, error) {
	if s.Validate() != nil || !d.ValidText(id, 36) {
		return nil, d.ErrInvalid
	}
	var row r.Run
	e := researchScope(rpo.db.WithContext(ctx), s).Where("id = ?", id).Take(&row).Error
	return &row, researchErr(e)
}
func (rpo *ResearchRunRepository) FindOutput(ctx context.Context, s d.Scope, ref l.RefID) (*r.Run, error) {
	if s.Validate() != nil || ref.Validate() != nil {
		return nil, d.ErrInvalid
	}
	raw, _ := json.Marshal(ref)
	var row r.Run
	e := researchScope(rpo.db.WithContext(ctx), s).Where("status IN ? AND JSON_CONTAINS(output_refs, ?)", []string{"succeeded", "partial"}, string(raw)).Take(&row).Error
	return &row, researchErr(e)
}
func (rpo *ResearchRunRepository) Lookup(ctx context.Context, s d.Scope, key, hash string) (*r.Run, error) {
	if s.Validate() != nil || len(key) != 64 || len(hash) != 64 {
		return nil, d.ErrInvalid
	}
	var row r.Run
	e := researchScope(rpo.db.WithContext(ctx), s).Where("operation_key = ?", key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if row.PayloadHash != hash {
		return nil, d.ErrConflict
	}
	return &row, nil
}
func researchSave(tx *gorm.DB, row *r.Run, now time.Time) error {
	row.Version++
	row.UpdatedAt = now
	return researchScope(tx, row.Scope).Where("id = ?", row.ID).Select("status", "stage", "progress", "version", "owner", "lease_until_us", "report", "output_refs", "error", "updated_at").Updates(row).Error
}
func researchCancelRow(tx *gorm.DB, row *r.Run, now time.Time) error {
	if row.Terminal() || row.Status == "cancel_requested" {
		return nil
	}
	if row.Status == "queued" {
		row.Status = "cancelled"
	} else {
		row.Status = "cancel_requested"
	}
	return researchSave(tx, row, now)
}
func (rpo *ResearchRunRepository) mutate(ctx context.Context, s d.Scope, id string, fn func(*gorm.DB, *r.Run, time.Time) error) (*r.Run, error) {
	if s.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row r.Run
	e := rpo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := researchScope(tx, s).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&row).Error; e != nil {
			return researchErr(e)
		}
		now, e := researchNow(tx)
		if e != nil {
			return e
		}
		return fn(tx, &row, now)
	})
	return &row, e
}
func (rpo *ResearchRunRepository) Cancel(ctx context.Context, s d.Scope, id string) (*r.Run, error) {
	return rpo.mutate(ctx, s, id, researchCancelRow)
}
func (rpo *ResearchRunRepository) Pending(ctx context.Context, limit int) ([]r.Run, error) {
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	var rows []r.Run
	e := rpo.db.WithContext(ctx).Where("status = ? OR (status IN ? AND lease_until_us <= CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(6))*1000000 AS SIGNED))", "queued", []string{"running", "cancel_requested"}).Order("created_at ASC, id ASC").Limit(limit).Find(&rows).Error
	return rows, e
}
func researchSubmission(row *r.Run) (r.Submission, error) {
	var in r.Submission
	e := json.Unmarshal(row.Submission, &in)
	if e == nil {
		e = in.Validate()
	}
	return in, e
}
func (rpo *ResearchRunRepository) Claim(ctx context.Context, s d.Scope, id, owner string, ttl time.Duration) (*r.Run, error) {
	if !d.ValidText(owner, 128) || ttl < time.Second || ttl > time.Minute {
		return nil, d.ErrInvalid
	}
	return rpo.mutate(ctx, s, id, func(tx *gorm.DB, row *r.Run, now time.Time) error {
		if row.Terminal() {
			return d.ErrConflict
		}
		if row.Status != "queued" {
			if row.LeaseUntilUS > now.UnixMicro() {
				return d.ErrConflict
			}
			if row.Status == "cancel_requested" {
				row.Status = "cancelled"
			} else {
				row.Status = "failed"
			}
			row.Error = "research_interrupted_unconfirmed: model calls are not automatically replayed"
			row.Owner = ""
			row.LeaseUntilUS = 0
			return researchSave(tx, row, now)
		}
		in, e := researchSubmission(row)
		if e != nil {
			return e
		}
		if !in.Deadline.After(now) {
			row.Status = "timed_out"
			row.Error = "research deadline exceeded"
		} else {
			row.Status = "running"
			row.Owner = owner
			row.LeaseUntilUS = now.Add(ttl).UnixMicro()
		}
		return researchSave(tx, row, now)
	})
}
func (rpo *ResearchRunRepository) Renew(ctx context.Context, s d.Scope, id, owner string, ttl time.Duration) (*r.Run, error) {
	if ttl < time.Second || ttl > time.Minute {
		return nil, d.ErrInvalid
	}
	return rpo.mutate(ctx, s, id, func(tx *gorm.DB, row *r.Run, now time.Time) error {
		if row.Owner != owner || row.LeaseUntilUS <= now.UnixMicro() || row.Terminal() {
			return d.ErrConflict
		}
		row.LeaseUntilUS = now.Add(ttl).UnixMicro()
		return researchScope(tx, s).Model(&r.Run{}).Where("id = ?", id).Update("lease_until_us", row.LeaseUntilUS).Error
	})
}
func (rpo *ResearchRunRepository) Complete(ctx context.Context, s d.Scope, id, owner string, out r.Completion) (*r.Run, error) {
	if out.Status != "succeeded" && out.Status != "partial" && out.Status != "failed" && out.Status != "cancelled" && out.Status != "timed_out" {
		return nil, d.ErrInvalid
	}
	if len(out.Error) > 2000 || len(out.Report) > 1<<20 || len(out.OutputRefs) > 4096 {
		return nil, d.ErrInvalid
	}
	if (out.Status == "succeeded" || out.Status == "partial") && (!json.Valid(out.Report) || !json.Valid(out.OutputRefs)) {
		return nil, d.ErrInvalid
	}
	return rpo.mutate(ctx, s, id, func(tx *gorm.DB, row *r.Run, now time.Time) error {
		if row.Terminal() || row.Owner != owner || row.LeaseUntilUS <= now.UnixMicro() {
			return d.ErrConflict
		}
		in, e := researchSubmission(row)
		if e != nil {
			return e
		}
		if row.Status == "cancel_requested" {
			out = r.Completion{Status: "cancelled", Report: row.Report}
		} else if !in.Deadline.After(now) {
			out = r.Completion{Status: "timed_out", Report: row.Report, Error: "research deadline exceeded"}
		}
		row.Status = out.Status
		row.Error = out.Error
		row.Owner = ""
		row.LeaseUntilUS = 0
		if len(out.Report) > 0 && json.Valid(out.Report) {
			row.Report = out.Report
		}
		row.Stage = out.Status
		row.OutputRefs = json.RawMessage(`[]`)
		if out.Status == "succeeded" || out.Status == "partial" {
			row.Progress = 1
			row.Report = out.Report
			row.OutputRefs = out.OutputRefs
		}
		return researchSave(tx, row, now)
	})
}

// Checkpoint persists only pre-cancellation evidence under the current lease.
func (rpo *ResearchRunRepository) Checkpoint(ctx context.Context, s d.Scope, id, owner, stage string, progress float64, report json.RawMessage) (*r.Run, error) {
	if len(stage) > 64 || progress < 0 || progress > 1 || len(report) > 1<<20 || !json.Valid(report) {
		return nil, d.ErrInvalid
	}
	return rpo.mutate(ctx, s, id, func(tx *gorm.DB, row *r.Run, now time.Time) error {
		if row.Status != "running" || row.Owner != owner || row.LeaseUntilUS <= now.UnixMicro() {
			return d.ErrConflict
		}
		if progress < row.Progress {
			return d.ErrConflict
		}
		row.Stage = stage
		row.Progress = progress
		row.Report = report
		return researchSave(tx, row, now)
	})
}
