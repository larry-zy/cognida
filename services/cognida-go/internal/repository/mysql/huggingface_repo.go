package mysql

import (
	d "cognida/internal/model/huggingface"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type huggingFaceRow struct {
	ID                                string
	TenantID, UserID                  int64
	OperationKey, RequestHash, Status string
	DeadlineUS                        int64
	Body                              json.RawMessage
}

func (huggingFaceRow) TableName() string { return "huggingface_imports" }

type HuggingFaceRepository struct{ db *gorm.DB }

func NewHuggingFaceRepository(db *gorm.DB) *HuggingFaceRepository { return &HuggingFaceRepository{db} }
func hfNow(tx *gorm.DB) (time.Time, error) {
	var us int64
	e := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&us).Error
	return time.UnixMicro(us).UTC(), e
}
func hfReceipt(row huggingFaceRow) (*d.Import, error) {
	var out d.Import
	if e := json.Unmarshal(row.Body, &out); e != nil {
		return nil, e
	}
	out.TenantID = row.TenantID
	out.UserID = row.UserID
	out.Status = row.Status
	return &out, nil
}
func (r *HuggingFaceRepository) Reserve(ctx context.Context, p l.Principal, key, hash string, request d.Request) (*d.Import, bool, error) {
	if p.Validate() != nil {
		return nil, false, d.ErrForbidden
	}
	var result *d.Import
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := hfNow(tx)
		if e != nil {
			return e
		}
		value := d.Import{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, Request: request, RequestHash: hash, Status: "running", OutputRefs: []l.RefID{}, Deadline: now.Add(120 * time.Second), CreatedAt: now, UpdatedAt: now}
		body, e := json.Marshal(value)
		if e != nil {
			return e
		}
		row := huggingFaceRow{ID: value.ID, TenantID: p.TenantID, UserID: p.UserID, OperationKey: key, RequestHash: hash, Status: value.Status, DeadlineUS: value.Deadline.UnixMicro(), Body: body}
		insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if insert.Error != nil {
			return insert.Error
		}
		created = insert.RowsAffected == 1
		var stored huggingFaceRow
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND operation_key=?", p.TenantID, p.UserID, key).First(&stored).Error; e != nil {
			return e
		}
		if stored.RequestHash != hash {
			return d.ErrConflict
		}
		result, e = hfReceipt(stored)
		if e != nil {
			return e
		}
		return expireHF(tx, &stored, result, now)
	})
	return result, created, err
}
func expireHF(tx *gorm.DB, row *huggingFaceRow, value *d.Import, now time.Time) error {
	if row.Status == "running" && row.DeadlineUS <= now.UnixMicro() {
		value.Status = "failed"
		value.ErrorCode = "deadline_exceeded"
		value.OutputRefs = []l.RefID{}
		value.UpdatedAt = now
		body, e := json.Marshal(value)
		if e != nil {
			return e
		}
		return tx.Model(row).Updates(map[string]any{"status": value.Status, "body": body}).Error
	}
	return nil
}
func (r *HuggingFaceRepository) Get(ctx context.Context, p l.Principal, id string) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var result *d.Import
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row huggingFaceRow
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).First(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return d.ErrNotFound
		}
		if e != nil {
			return e
		}
		result, e = hfReceipt(row)
		if e != nil {
			return e
		}
		now, e := hfNow(tx)
		if e != nil {
			return e
		}
		return expireHF(tx, &row, result, now)
	})
	return result, err
}
func (r *HuggingFaceRepository) Finish(ctx context.Context, p l.Principal, id string, manifest *d.Manifest, refs []l.RefID, code string) (*d.Import, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var result *d.Import
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row huggingFaceRow
		e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).First(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return d.ErrNotFound
		}
		if e != nil {
			return e
		}
		result, e = hfReceipt(row)
		if e != nil {
			return e
		}
		now, e := hfNow(tx)
		if e != nil {
			return e
		}
		if e = expireHF(tx, &row, result, now); e != nil {
			return e
		}
		if result.Status != "running" {
			return nil
		}
		result.Status = "failed"
		result.ErrorCode = code
		result.Manifest = manifest
		result.OutputRefs = []l.RefID{}
		result.UpdatedAt = now
		if code == "" {
			if manifest == nil || len(refs) != 1 {
				return d.ErrInvalid
			}
			result.Status = "succeeded"
			result.OutputRefs = refs
		}
		body, e := json.Marshal(result)
		if e != nil {
			return e
		}
		return tx.Model(&row).Updates(map[string]any{"status": result.Status, "body": body}).Error
	})
	return result, err
}
