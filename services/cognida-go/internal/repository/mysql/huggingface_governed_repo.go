package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func hfHash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func saveHF(tx *gorm.DB, row *huggingFaceRow, value *d.Import) error {
	body, e := json.Marshal(value)
	if e != nil {
		return e
	}
	return tx.Model(row).Updates(map[string]any{"status": value.Status, "body": body, "claim_token": row.ClaimToken}).Error
}
func (r *HuggingFaceRepository) ReserveGoverned(ctx context.Context, p l.Principal, in d.GovernedInvocation, cancel bool) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if in.Validate() != nil {
		return nil, d.ErrInvalid
	}
	var out *d.Import
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := hfNow(tx)
		if e != nil {
			return e
		}
		deadline := now.Add(time.Duration(in.TimeoutSeconds) * time.Second)
		if in.Deadline.Before(deadline) {
			deadline = in.Deadline
		}
		value := d.Import{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Request: in.Request, RequestHash: hfHash(in.Request), Status: "queued", OutputRefs: []l.RefID{}, Deadline: deadline, CreatedAt: now, UpdatedAt: now, Governed: &in}
		if cancel {
			value.Status, value.ErrorCode = "cancelled", "cancelled"
		}
		body, e := json.Marshal(value)
		if e != nil {
			return e
		}
		row := huggingFaceRow{ID: value.ID, TenantID: p.TenantID, UserID: p.UserID, OperationKey: hfHash([]string{"governed", in.OperationKey}), RequestHash: value.RequestHash, Status: value.Status, DeadlineUS: deadline.UnixMicro(), Body: body, GovernedKey: &in.OperationKey, GovernedHash: &in.PayloadHash}
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return e
		}
		var stored huggingFaceRow
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND governed_key=?", p.TenantID, p.UserID, in.OperationKey).First(&stored).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return d.ErrConflict
			}
			return e
		}
		out, e = hfReceipt(stored)
		if e != nil {
			return e
		}
		if stored.GovernedHash == nil || *stored.GovernedHash != in.PayloadHash || stored.RequestHash != value.RequestHash || out.Governed == nil || hfHash(out.Governed) != hfHash(in) {
			return d.ErrConflict
		}
		if e = expireHF(tx, &stored, out, now); e != nil {
			return e
		}
		if cancel && (out.Status == "queued" || out.Status == "running") {
			out.Status = "cancelled"
			if out.StartedAt != nil {
				out.Status = "cancel_requested"
			}
			out.ErrorCode = "cancelled"
			out.OutputRefs = []l.RefID{}
			out.UpdatedAt = now
			return saveHF(tx, &stored, out)
		}
		return nil
	})
	return out, err
}
func (r *HuggingFaceRepository) LookupGoverned(ctx context.Context, p l.Principal, key, hash string) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row huggingFaceRow
	e := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND governed_key=?", p.TenantID, p.UserID, key).First(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if row.GovernedHash == nil || *row.GovernedHash != hash {
		return nil, d.ErrConflict
	}
	return r.Get(ctx, p, row.ID)
}
func (r *HuggingFaceRepository) ClaimGoverned(ctx context.Context, p l.Principal, id string) (*d.Import, bool, error) {
	if p.Validate() != nil {
		return nil, false, d.ErrForbidden
	}
	var out *d.Import
	claimed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row huggingFaceRow
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND governed_key IS NOT NULL", id, p.TenantID, p.UserID).First(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return d.ErrNotFound
		}
		if e != nil {
			return e
		}
		out, e = hfReceipt(row)
		if e != nil {
			return e
		}
		now, e := hfNow(tx)
		if e != nil {
			return e
		}
		if e = expireHF(tx, &row, out, now); e != nil {
			return e
		}
		if out.Status != "queued" {
			return nil
		}
		if out.Governed == nil || out.Governed.Validate() != nil || row.GovernedHash == nil || *row.GovernedKey != out.Governed.OperationKey || *row.GovernedHash != out.Governed.PayloadHash || out.RequestHash != hfHash(out.Request) || hfHash(out.Request) != hfHash(out.Governed.Request) || row.ClaimToken != nil || out.StartedAt != nil {
			return d.ErrConflict
		}
		token := uuid.NewString()
		row.ClaimToken = &token
		out.ClaimToken = token
		out.StartedAt = &now
		out.UpdatedAt = now
		out.Status = "running"
		if e = saveHF(tx, &row, out); e != nil {
			return e
		}
		claimed = true
		return nil
	})
	return out, claimed, err
}
func (r *HuggingFaceRepository) FinishGoverned(ctx context.Context, p l.Principal, id, token string, manifest *d.Manifest, refs []l.RefID, code string) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var out *d.Import
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row huggingFaceRow
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND governed_key IS NOT NULL", id, p.TenantID, p.UserID).First(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return d.ErrNotFound
		}
		if e != nil {
			return e
		}
		if row.ClaimToken == nil || token == "" || *row.ClaimToken != token {
			return d.ErrConflict
		}
		out, e = hfReceipt(row)
		if e != nil {
			return e
		}
		now, e := hfNow(tx)
		if e != nil {
			return e
		}
		if e = expireHF(tx, &row, out, now); e != nil {
			return e
		}
		if out.Status != "running" && out.Status != "cancel_requested" {
			return nil
		}
		cancelled := out.Status == "cancel_requested"
		out.Status = "failed"
		out.ErrorCode = code
		out.OutputRefs = []l.RefID{}
		out.Manifest = manifest
		out.UpdatedAt = now
		if cancelled || code == "cancelled" {
			out.Status = "cancelled"
			out.ErrorCode = "cancelled"
		} else if code == "deadline_exceeded" {
			out.Status = "timed_out"
		} else if code == "" {
			if manifest == nil || manifest.RequestHash != out.RequestHash || len(refs) != 1 || refs[0].Validate() != nil {
				return d.ErrInvalid
			}
			out.Status = "succeeded"
			out.OutputRefs = refs
		}
		return saveHF(tx, &row, out)
	})
	return out, err
}
func (r *HuggingFaceRepository) PendingGoverned(ctx context.Context, limit int) ([]d.Import, error) {
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	var rows []huggingFaceRow
	e := r.db.WithContext(ctx).Where("governed_key IS NOT NULL AND status IN ?", []string{"queued", "running", "cancel_requested"}).Order("deadline_us,id").Limit(limit).Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := make([]d.Import, 0, len(rows))
	for _, row := range rows {
		value, e := hfReceipt(row)
		if e != nil {
			return nil, e
		}
		out = append(out, *value)
	}
	return out, nil
}

var _ d.GovernedRepository = (*HuggingFaceRepository)(nil)
