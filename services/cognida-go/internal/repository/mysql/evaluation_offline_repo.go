package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	d "cognida/internal/model/evaluation"
	l "cognida/internal/model/lifecycle"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type offlineRow struct {
	ID           string `gorm:"column:id;primaryKey"`
	TenantID     int64  `gorm:"column:tenant_id"`
	UserID       int64  `gorm:"column:user_id"`
	OperationKey string `gorm:"column:operation_key"`
	PayloadHash  string `gorm:"column:payload_hash"`
	Status       string `gorm:"column:status"`
	Version      int64  `gorm:"column:version"`
	Epoch        int64  `gorm:"column:epoch"`
	LeaseUntilUS int64  `gorm:"column:lease_until_us"`
	DeadlineUS   int64  `gorm:"column:deadline_us"`
	Body         []byte `gorm:"column:body;type:json"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (offlineRow) TableName() string { return "evaluation_offline_runs" }

type offlineRepository struct{ db *gorm.DB }

func NewOfflineEvaluationRepository(db *gorm.DB) d.OfflineRepository { return &offlineRepository{db} }
func offlineNow(tx *gorm.DB) (time.Time, error) {
	var us int64
	e := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&us).Error
	return time.UnixMicro(us).UTC(), e
}
func offlineEncode(r *d.OfflineRun) (offlineRow, error) {
	body, e := json.Marshal(r)
	lease := int64(0)
	if !r.LeaseUntil.IsZero() {
		lease = r.LeaseUntil.UnixMicro()
	}
	return offlineRow{ID: r.ID, TenantID: r.Scope.TenantID, UserID: r.Scope.UserID, OperationKey: r.Request.OperationKey, PayloadHash: r.Request.PayloadHash, Status: r.Status, Version: r.Version, Epoch: r.Epoch, LeaseUntilUS: lease, DeadlineUS: r.Request.Deadline.UnixMicro(), Body: body, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, e
}
func offlineDecode(row offlineRow) (*d.OfflineRun, error) {
	var r d.OfflineRun
	if json.Unmarshal(row.Body, &r) != nil || r.ID != row.ID || r.Scope.TenantID != row.TenantID || r.Scope.UserID != row.UserID || r.Request.OperationKey != row.OperationKey || r.Request.PayloadHash != row.PayloadHash || r.Status != row.Status || r.Version != row.Version || r.Epoch != row.Epoch {
		return nil, d.ErrOfflineConflict
	}
	return &r, nil
}
func offlineFind(tx *gorm.DB, p d.OfflineScope, field, value string, lock bool) (*d.OfflineRun, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	q := tx.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
	if field == "id" {
		q = q.Where("id=?", value)
	} else {
		q = q.Where("operation_key=?", value)
	}
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row offlineRow
	e := q.Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrTaskNotFound
	}
	if e != nil {
		return nil, e
	}
	return offlineDecode(row)
}
func offlineSame(a, b d.OfflineRequest) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func offlineSave(tx *gorm.DB, r *d.OfflineRun, now time.Time) error {
	previous := r.Version
	r.Version++
	r.UpdatedAt = now
	row, e := offlineEncode(r)
	if e != nil {
		return e
	}
	res := tx.Model(&offlineRow{}).Where("id=? AND tenant_id=? AND user_id=? AND version=?", r.ID, r.Scope.TenantID, r.Scope.UserID, previous).Select("*").Updates(&row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return d.ErrOfflineConflict
	}
	return offlineProject(tx, r)
}
func offlineProject(tx *gorm.DB, r *d.OfflineRun) error {
	status := r.Status
	switch status {
	case d.OfflineQueued:
		status = "pending"
	case d.OfflineSucceeded:
		status = "completed"
	}
	metrics, _ := json.Marshal(map[string]any{"scores": d.OfflineAverages(r.Scores)})
	res := tx.Model(&EvaluationTaskModel{}).Where("id=? AND tenant_id=? AND user_id=? AND type=?", r.ID, r.Scope.TenantID, r.Scope.UserID, string(d.OfflineType)).Updates(map[string]any{"status": status, "error_message": r.Error, "total_count": len(r.Items), "success_count": len(r.Scores), "failure_count": 0, "metrics": metrics, "updated_at": r.UpdatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return d.ErrOfflineConflict
	}
	return nil
}
func (r *offlineRepository) Lookup(ctx context.Context, p d.OfflineScope, key string) (*d.OfflineRun, error) {
	return offlineFind(r.db.WithContext(ctx), p, "key", key, false)
}
func (r *offlineRepository) Get(ctx context.Context, p d.OfflineScope, id string) (*d.OfflineRun, error) {
	return offlineFind(r.db.WithContext(ctx), p, "id", id, false)
}
func (r *offlineRepository) Submit(ctx context.Context, p d.OfflineScope, req d.OfflineRequest, items []d.OfflineItem, hash string) (*d.OfflineRun, error) {
	if e := d.ValidateOfflineSnapshot(items, hash); e != nil {
		return nil, e
	}
	return r.create(ctx, p, req, items, hash, false)
}
func (r *offlineRepository) CancelSubmission(ctx context.Context, p d.OfflineScope, req d.OfflineRequest) (*d.OfflineRun, error) {
	return r.create(ctx, p, req, nil, "", true)
}
func (r *offlineRepository) create(ctx context.Context, p d.OfflineScope, req d.OfflineRequest, items []d.OfflineItem, hash string, cancel bool) (out *d.OfflineRun, err error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	if e := req.Validate(); e != nil {
		return nil, e
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := offlineNow(tx)
		if e != nil {
			return e
		}
		status := d.OfflineQueued
		if cancel {
			status = d.OfflineCancelled
		}
		candidate := &d.OfflineRun{ID: "eval-" + uuid.NewString(), Scope: p, Request: req, Items: items, SnapshotHash: hash, Status: status, Version: 1, CreatedAt: now, UpdatedAt: now}
		row, e := offlineEncode(candidate)
		if e != nil {
			return e
		}
		inserted := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if inserted.Error != nil {
			return inserted.Error
		}
		out, e = offlineFind(tx, p, "key", req.OperationKey, true)
		if e != nil {
			return e
		}
		if !offlineSame(out.Request, req) {
			return d.ErrOfflineConflict
		}
		if out.ID == candidate.ID {
			config, _ := json.Marshal(map[string]any{"profile": d.OfflineProfile, "mode": "offline_existing_answers", "input_ref": req.InputRef, "columns": req.Columns, "snapshot_sha256": hash})
			main := &EvaluationTaskModel{ID: out.ID, TenantID: p.TenantID, UserID: p.UserID, DatasetID: "asset:" + req.InputRef.AssetID, Type: string(d.OfflineType), Config: config, Status: "pending", TotalCount: len(items), CreatedAt: now, UpdatedAt: now}
			if cancel {
				main.Status = d.OfflineCancelled
			}
			if e = tx.Create(main).Error; e != nil {
				return e
			}
		}
		if !cancel && out.Status != d.OfflineCancelled && out.SnapshotHash != hash {
			return d.ErrOfflineConflict
		}
		if cancel && !out.Terminal() {
			out.Status = d.OfflineCancelled
			out.Epoch++
			out.LeaseUntil = time.Time{}
			return offlineSave(tx, out, now)
		}
		return nil
	})
	return
}
func (r *offlineRepository) mutate(ctx context.Context, p d.OfflineScope, id string, fn func(*gorm.DB, *d.OfflineRun, time.Time) error) (out *d.OfflineRun, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		out, e = offlineFind(tx, p, "id", id, true)
		if e != nil {
			return e
		}
		now, e := offlineNow(tx)
		if e != nil {
			return e
		}
		return fn(tx, out, now)
	})
	return
}
func validLease(ttl time.Duration) bool { return ttl >= time.Second && ttl <= time.Minute }
func offlineOwned(r *d.OfflineRun, epoch int64, now time.Time) bool {
	return r.Status == d.OfflineRunning && r.Epoch == epoch && r.LeaseUntil.After(now)
}
func (r *offlineRepository) Claim(ctx context.Context, p d.OfflineScope, id string, ttl time.Duration) (*d.OfflineRun, error) {
	if !validLease(ttl) {
		return nil, d.ErrOfflineInvalid
	}
	return r.mutate(ctx, p, id, func(tx *gorm.DB, v *d.OfflineRun, now time.Time) error {
		if v.Terminal() {
			return nil
		}
		if v.LeaseUntil.After(now) {
			return d.ErrOfflineConflict
		}
		if v.ExecutionDeadline.IsZero() {
			v.ExecutionDeadline = now.Add(time.Duration(v.Request.TimeoutSeconds) * time.Second)
			if v.Request.Deadline.Before(v.ExecutionDeadline) {
				v.ExecutionDeadline = v.Request.Deadline
			}
		}
		if !v.ExecutionDeadline.After(now) {
			v.Status = d.OfflineTimedOut
			v.Error = "offline evaluation deadline exceeded"
			return offlineSave(tx, v, now)
		}
		v.Status = d.OfflineRunning
		v.Epoch++
		v.LeaseUntil = now.Add(ttl)
		return offlineSave(tx, v, now)
	})
}
func (r *offlineRepository) Renew(ctx context.Context, p d.OfflineScope, id string, epoch int64, ttl time.Duration) error {
	if !validLease(ttl) {
		return d.ErrOfflineInvalid
	}
	_, e := r.mutate(ctx, p, id, func(tx *gorm.DB, v *d.OfflineRun, now time.Time) error {
		if !offlineOwned(v, epoch, now) || !v.ExecutionDeadline.After(now) {
			return d.ErrOfflineConflict
		}
		v.LeaseUntil = now.Add(ttl)
		return offlineSave(tx, v, now)
	})
	return e
}
func (r *offlineRepository) Checkpoint(ctx context.Context, p d.OfflineScope, id string, epoch int64, next int, scores []d.OfflineScore) (*d.OfflineRun, error) {
	return r.mutate(ctx, p, id, func(tx *gorm.DB, v *d.OfflineRun, now time.Time) error {
		if !offlineOwned(v, epoch, now) || !v.ExecutionDeadline.After(now) || v.NextIndex != next {
			return d.ErrOfflineConflict
		}
		if len(scores) < 1 || len(scores) > d.OfflineBatchSize || next+len(scores) > len(v.Items) {
			return d.ErrOfflineInvalid
		}
		for i, s := range scores {
			if s.Index != next+i || s.Validate() != nil {
				return d.ErrOfflineInvalid
			}
			item := v.Items[s.Index]
			record := FromDomainEvaluationResult(&d.EvaluationResult{TaskID: v.ID, Question: item.Question, ReferenceAnswer: item.ReferenceAnswer, GeneratedAnswer: item.GeneratedAnswer, Success: true, Scores: map[string]float64{"exact_match": s.ExactMatch, "contains_match": s.ContainsMatch}, CreatedAt: now})
			if e := tx.Create(record).Error; e != nil {
				return e
			}
		}
		v.Scores = append(v.Scores, scores...)
		v.NextIndex += len(scores)
		return offlineSave(tx, v, now)
	})
}
func (r *offlineRepository) Finish(ctx context.Context, p d.OfflineScope, id string, epoch int64, status, message string, output *l.RefID) (*d.OfflineRun, error) {
	if status != d.OfflineSucceeded && status != d.OfflineFailed && status != d.OfflineTimedOut {
		return nil, d.ErrOfflineInvalid
	}
	return r.mutate(ctx, p, id, func(tx *gorm.DB, v *d.OfflineRun, now time.Time) error {
		if !offlineOwned(v, epoch, now) {
			return d.ErrOfflineConflict
		}
		if status == d.OfflineSucceeded {
			if !v.ExecutionDeadline.After(now) || v.NextIndex != len(v.Items) || len(v.Items) == 0 || output == nil || output.Validate() != nil {
				return d.ErrOfflineConflict
			}
			v.OutputRef = output
		} else {
			if output != nil {
				return d.ErrOfflineInvalid
			}
			if len(message) > 256 {
				return d.ErrOfflineInvalid
			}
		}
		v.Status = status
		v.Error = message
		v.LeaseUntil = time.Time{}
		return offlineSave(tx, v, now)
	})
}
func (r *offlineRepository) Release(ctx context.Context, p d.OfflineScope, id string, epoch int64) error {
	_, e := r.mutate(ctx, p, id, func(tx *gorm.DB, v *d.OfflineRun, now time.Time) error {
		if v.Status != d.OfflineRunning || v.Epoch != epoch {
			return d.ErrOfflineConflict
		}
		v.LeaseUntil = time.Time{}
		return offlineSave(tx, v, now)
	})
	return e
}
func (r *offlineRepository) Pending(ctx context.Context, limit int) ([]d.OfflineRun, error) {
	if limit < 1 || limit > 1000 {
		return nil, d.ErrOfflineInvalid
	}
	tx := r.db.WithContext(ctx)
	now, e := offlineNow(tx)
	if e != nil {
		return nil, e
	}
	var rows []offlineRow
	e = tx.Where("status IN ? AND lease_until_us<=?", []string{d.OfflineQueued, d.OfflineRunning}, now.UnixMicro()).Order("id ASC").Limit(limit).Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := make([]d.OfflineRun, 0, len(rows))
	for _, row := range rows {
		v, e := offlineDecode(row)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, nil
}
