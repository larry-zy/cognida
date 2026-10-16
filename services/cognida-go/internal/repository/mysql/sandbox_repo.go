package mysql

import (
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/sandbox"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type SandboxRepository struct{ db *gorm.DB }

func NewSandboxRepository(db *gorm.DB) *SandboxRepository { return &SandboxRepository{db} }
func sandboxScope(db *gorm.DB, p l.Principal) *gorm.DB {
	return db.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
}
func sandboxError(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrForbidden
	}
	return e
}
func (r *SandboxRepository) Create(ctx context.Context, in *d.Record) (*d.Record, error) {
	if in.Principal().Validate() != nil {
		return nil, d.ErrForbidden
	}
	var out d.Record
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(in).Error; e != nil {
			return e
		}
		if e := sandboxScope(tx, in.Principal()).Where("idempotency_key=?", in.IdempotencyKey).Take(&out).Error; e != nil {
			return sandboxError(e)
		}
		if out.ID != in.ID || out.RequestSHA256 != in.RequestSHA256 || sandboxGovernedHash(out.Submission) != sandboxGovernedHash(in.Submission) {
			return d.ErrConflict
		}
		return nil
	})
	return &out, e
}
func (r *SandboxRepository) Get(ctx context.Context, p l.Principal, id string) (*d.Record, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var v d.Record
	e := sandboxScope(r.db.WithContext(ctx), p).Where("id=?", id).Take(&v).Error
	return &v, sandboxError(e)
}
func (r *SandboxRepository) List(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Record, string, error) {
	if p.Validate() != nil || limit < 1 || limit > 100 {
		return nil, "", d.ErrInvalid
	}
	q := sandboxScope(r.db.WithContext(ctx), p)
	if cursor != "" {
		if _, e := uuid.Parse(cursor); e != nil {
			return nil, "", d.ErrInvalid
		}
		q = q.Where("id>?", cursor)
	}
	var rows []d.Record
	e := q.Order("id").Limit(limit + 1).Find(&rows).Error
	next := ""
	if len(rows) > limit {
		next = rows[limit-1].ID
		rows = rows[:limit]
	}
	return rows, next, e
}
func (r *SandboxRepository) RequestCancel(ctx context.Context, p l.Principal, id string) (*d.Record, error) {
	var out d.Record
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := sandboxScope(tx, p).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&out).Error; e != nil {
			return sandboxError(e)
		}
		if d.Terminal(out.State) {
			return nil
		}
		out.CancelRequested = true
		out.UpdatedAt = time.Now().UTC()
		out.Version++
		return sandboxScope(tx.Model(&d.Record{}), p).Where("id=?", id).Updates(map[string]any{"cancel_requested": true, "updated_at": out.UpdatedAt, "version": out.Version, "lease_until": nil}).Error
	})
	return &out, e
}
func (r *SandboxRepository) Claim(ctx context.Context, lease time.Duration) (*d.Record, error) {
	var out d.Record
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var micros int64
		if e := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&micros).Error; e != nil {
			return e
		}
		now := time.UnixMicro(micros).UTC()
		e := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("state IN ? AND (lease_until IS NULL OR lease_until<?)", []d.State{d.StateQueued, d.StateRunning, d.StateCancelRequested, d.StateReconcileRequired}, now).Order("updated_at,id").Take(&out).Error
		if e != nil {
			return e
		}
		until := now.Add(lease)
		out.LeaseToken = uuid.NewString()
		out.LeaseUntil = &until
		out.Version++
		out.UpdatedAt = now
		return tx.Model(&d.Record{}).Where("id=? AND tenant_id=?", out.ID, out.TenantID).Updates(map[string]any{"lease_token": out.LeaseToken, "lease_until": until, "version": out.Version, "updated_at": now}).Error
	})
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &out, e
}
func (r *SandboxRepository) Save(ctx context.Context, in *d.Record) error {
	// Database time and the current lease fence all publication, including a
	// worker that completed after cancellation or its deadline.
	conflict := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current d.Record
		if e := sandboxScope(tx, in.Principal()).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", in.ID).Take(&current).Error; e != nil {
			return sandboxError(e)
		}
		if current.Version != in.Version || current.LeaseToken != in.LeaseToken || in.LeaseToken == "" {
			return d.ErrConflict
		}
		var micros int64
		if e := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&micros).Error; e != nil {
			return e
		}
		now := time.UnixMicro(micros).UTC()
		publish := sandboxExecutionAdvance(in)
		if current.LeaseUntil == nil || !current.LeaseUntil.After(now) || (publish && (current.CancelRequested || !current.Deadline.After(now))) {
			conflict = true
			// Release this still-owned claim so a fresh worker promptly stops the
			// container. No output becomes readable on this path.
			return tx.Model(&d.Record{}).Where("id=? AND version=?", in.ID, in.Version).Updates(map[string]any{"lease_until": nil, "version": in.Version + 1}).Error
		}
		in.UpdatedAt = now
		result := tx.Model(&d.Record{}).Where("id=? AND version=?", in.ID, in.Version).Updates(map[string]any{"state": in.State, "phase": in.Phase, "exec_id": in.ExecID, "finish_state": in.FinishState, "finish_error": in.FinishError, "manifest": in.Manifest, "output_refs": in.OutputRefs, "error_code": in.ErrorCode, "updated_at": now, "version": in.Version + 1, "lease_until": nil})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return d.ErrConflict
		}
		return nil
	})
	if err != nil {
		return err
	}
	if conflict {
		return d.ErrConflict
	}
	in.Version++
	return nil
}

// A confirmed non-success cleanup may retain the last execution phase for
// diagnosis. That historical phase must not turn cancellation/timeout/failure
// into a new launch or publication. The runner clears outputs only after the
// container removal succeeds; failed cleanup remains cleanup_pending instead.
// Version/token/lease fencing in Save still applies to every transition.
func sandboxExecutionAdvance(in *d.Record) bool {
	if (in.State == d.StateCancelled || in.State == d.StateTimedOut || in.State == d.StateFailed) && len(in.Manifest) == 0 && len(in.OutputRefs) == 0 {
		return false
	}
	return in.State == d.StateSucceeded || in.Phase == "collected" || in.Phase == "launch_committed"
}

func sandboxGovernedHash(raw json.RawMessage) string {
	var v struct {
		Governed *struct {
			CommandHash string `json:"command_hash"`
		} `json:"governed"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "invalid"
	}
	if v.Governed == nil {
		return ""
	}
	return v.Governed.CommandHash
}
func (r *SandboxRepository) FindByKey(ctx context.Context, p l.Principal, key string) (*d.Record, error) {
	if p.Validate() != nil || key == "" {
		return nil, d.ErrInvalid
	}
	var row d.Record
	e := sandboxScope(r.db.WithContext(ctx), p).Where("idempotency_key=?", key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return &row, nil
}
