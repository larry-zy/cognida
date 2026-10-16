package mysql

import (
	d "cognida/internal/model/governance"
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type driftRow struct {
	ID                            string `gorm:"primaryKey"`
	TenantID, UserID              int64
	Kind, Status                  string
	Version                       int
	IdempotencyKey, RequestDigest string
	Body                          []byte
	NextRunAt                     *time.Time
	CreatedAt, UpdatedAt          time.Time
}

func (driftRow) TableName() string { return "drift_records" }

type driftEventRow struct {
	ID, RecordID     string
	TenantID, UserID int64
	Action           string
	ActorID          int64
	Version          int
	CreatedAt        time.Time
}

func (driftEventRow) TableName() string { return "drift_events" }
func driftView(r driftRow) d.DriftRecord {
	return d.DriftRecord{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, Kind: r.Kind, Status: r.Status, Version: r.Version, Body: r.Body, NextRunAt: r.NextRunAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func driftAudit(tx *gorm.DB, r *d.DriftRecord, action string) error {
	return tx.Create(&driftEventRow{ID: uuid.NewString(), RecordID: r.ID, TenantID: r.TenantID, UserID: r.UserID, ActorID: r.UserID, Action: action, Version: r.Version, CreatedAt: r.UpdatedAt}).Error
}
func (r *governanceRepository) CreateDrift(ctx context.Context, in *d.DriftRecord, key string) (out *d.DriftRecord, err error) {
	if in.TenantID <= 0 || in.UserID <= 0 || len(key) < 1 || len(key) > 100 {
		return nil, d.ErrInvalid
	}
	digest := hashFeatureRequest(in.Body)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		row := driftRow{ID: uuid.NewString(), TenantID: in.TenantID, UserID: in.UserID, Kind: in.Kind, Status: in.Status, Version: 1, IdempotencyKey: key, RequestDigest: digest, Body: in.Body, NextRunAt: in.NextRunAt, CreatedAt: now, UpdatedAt: now}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		created := res.RowsAffected == 1
		var old driftRow
		if e = tx.Where("tenant_id=? AND user_id=? AND kind=? AND idempotency_key=?", in.TenantID, in.UserID, in.Kind, key).Take(&old).Error; e != nil {
			return e
		}
		if old.RequestDigest != digest {
			return d.ErrConflict
		}
		view := driftView(old)
		out = &view
		if created {
			return driftAudit(tx, out, "created")
		}
		return nil
	})
	return
}
func (r *governanceRepository) GetDrift(ctx context.Context, t, u int64, id string) (*d.DriftRecord, error) {
	var row driftRow
	e := r.db.WithContext(ctx).Where("id=? AND tenant_id=? AND user_id=?", id, t, u).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	v := driftView(row)
	return &v, nil
}
func (r *governanceRepository) ListDrift(ctx context.Context, t, u int64, kind, cursor string) ([]d.DriftRecord, string, error) {
	q := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND kind=?", t, u, kind)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []driftRow
	if e := q.Order("id").Limit(51).Find(&rows).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = rows[49].ID
	}
	out := []d.DriftRecord{}
	for _, r := range rows {
		out = append(out, driftView(r))
	}
	return out, next, nil
}
func (r *governanceRepository) UpdateDrift(ctx context.Context, in *d.DriftRecord, version int, action string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		res := tx.Model(&driftRow{}).Where("id=? AND tenant_id=? AND user_id=? AND version=?", in.ID, in.TenantID, in.UserID, version).Updates(map[string]any{"body": []byte(in.Body), "status": in.Status, "version": version + 1, "next_run_at": in.NextRunAt, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return d.ErrConflict
		}
		in.Version = version + 1
		in.UpdatedAt = now
		return driftAudit(tx, in, action)
	})
}
func (r *governanceRepository) ClaimDriftMonitors(ctx context.Context, limit int) (out []d.DriftRecord, err error) {
	if limit < 1 || limit > 20 {
		limit = 10
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		var rows []driftRow
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("kind='monitor' AND status='active' AND next_run_at<=?", now).Order("next_run_at").Limit(limit).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			next := now.Add(2 * time.Minute)
			res := tx.Model(&driftRow{}).Where("id=? AND version=?", row.ID, row.Version).Updates(map[string]any{"next_run_at": next, "version": row.Version + 1, "updated_at": now})
			if res.Error != nil {
				return res.Error
			}
			row.Version++
			row.UpdatedAt = now
			row.NextRunAt = &next
			v := driftView(row)
			if e = driftAudit(tx, &v, "scheduled"); e != nil {
				return e
			}
			out = append(out, v)
		}
		return nil
	})
	return
}
func (r *governanceRepository) DriftEvents(ctx context.Context, t, u int64, id string) ([]d.DriftEvent, error) {
	var rows []driftEventRow
	if e := r.db.WithContext(ctx).Where("record_id=? AND tenant_id=? AND user_id=?", id, t, u).Order("version").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.DriftEvent{}
	for _, v := range rows {
		out = append(out, d.DriftEvent{ID: v.ID, RecordID: v.RecordID, Action: v.Action, ActorID: v.ActorID, Version: v.Version, CreatedAt: v.CreatedAt})
	}
	return out, nil
}

var _ d.DriftRepository = (*governanceRepository)(nil)
