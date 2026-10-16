package mysql

import (
	"bytes"
	ds "cognida/internal/model/datasource"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/queryjob"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type queryJobRow struct {
	ID                                string
	TenantID, UserID                  int64
	OperationKey, RequestHash, Status string
	Terminal                          bool
	Version, Epoch                    int64
	LeaseUntil                        *time.Time
	Body                              json.RawMessage
	CreatedAt, UpdatedAt              time.Time
}

func (queryJobRow) TableName() string { return "query_jobs" }

type queryJobRepository struct{ db *gorm.DB }

func NewQueryJobRepository(db *gorm.DB) d.Repository { return &queryJobRepository{db} }
func queryScope(db *gorm.DB, p l.Principal) *gorm.DB {
	return db.Where("tenant_id=? AND user_id=?", p.TenantID, p.UserID)
}
func queryErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	if errors.Is(e, gorm.ErrDuplicatedKey) {
		return d.ErrConflict
	}
	return e
}
func queryRecord(row queryJobRow) (*d.Record, error) {
	var v d.Record
	dec := json.NewDecoder(bytes.NewReader(row.Body))
	dec.UseNumber()
	if dec.Decode(&v) != nil {
		return nil, d.ErrConflict
	}
	v.Job.ID = row.ID
	v.Job.TenantID = row.TenantID
	v.Job.UserID = row.UserID
	v.Job.Version = row.Version
	v.Job.Status = row.Status
	v.Epoch = row.Epoch
	return &v, nil
}
func queryNow(db *gorm.DB) (time.Time, error) {
	var micros int64
	e := db.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&micros).Error
	return time.UnixMicro(micros).UTC(), e
}
func (r *queryJobRepository) Create(ctx context.Context, p l.Principal, in d.Input, engine ds.Type, version, key string) (*d.Record, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if in.Validate() != nil || !d.ValidKey(key) || version == "" {
		return nil, d.ErrInvalid
	}
	hash := d.Digest(struct {
		Input   d.Input
		Engine  ds.Type
		Version string
	}{in, engine, version})
	var out *d.Record
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := queryNow(tx)
		if e != nil {
			return e
		}
		rec := d.Record{Job: d.Job{Input: in, Engine: engine, SourceVersion: version, Status: "queued", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Duration(in.TimeoutSeconds) * time.Second)}, Phase: "queued"}
		body, _ := json.Marshal(rec)
		row := queryJobRow{ID: uuid.NewString(), TenantID: p.TenantID, UserID: p.UserID, OperationKey: key, RequestHash: hash, Status: "queued", Version: 1, Body: body, CreatedAt: now, UpdatedAt: now}
		if e = tx.Model(&queryJobRow{}).Clauses(clause.OnConflict{DoNothing: true}).Create(map[string]any{
			"id": row.ID, "tenant_id": p.TenantID, "user_id": p.UserID, "operation_key": key, "request_hash": hash, "status": "queued", "terminal": false, "version": 1, "epoch": 0, "body": body, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)"),
		}).Error; e != nil {
			return e
		}
		row = queryJobRow{}
		if e = queryScope(tx, p).Where("operation_key=?", key).First(&row).Error; e != nil {
			return e
		}
		if row.RequestHash != hash {
			return d.ErrConflict
		}
		out, e = queryRecord(row)
		return e
	})
	return out, queryErr(e)
}
func (r *queryJobRepository) Get(ctx context.Context, p l.Principal, id string) (*d.Record, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	var row queryJobRow
	if e := queryScope(r.db.WithContext(ctx), p).Where("id=?", id).First(&row).Error; e != nil {
		return nil, queryErr(e)
	}
	return queryRecord(row)
}
func (r *queryJobRepository) List(ctx context.Context, p l.Principal, cursor string, limit int) ([]d.Job, error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	var rows []queryJobRow
	q := queryScope(r.db.WithContext(ctx), p)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	if e := q.Order("id").Limit(limit).Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []d.Job{}
	for _, row := range rows {
		v, e := queryRecord(row)
		if e != nil {
			return nil, e
		}
		out = append(out, v.Job)
	}
	return out, nil
}
func (r *queryJobRepository) Runnable(ctx context.Context, limit int) ([]d.Item, error) {
	if limit < 1 || limit > 100 {
		return nil, d.ErrInvalid
	}
	var rows []queryJobRow
	e := r.db.WithContext(ctx).Select("id,tenant_id,user_id").Where("terminal=false AND (lease_until IS NULL OR lease_until<UTC_TIMESTAMP(6))").Order("updated_at").Limit(limit).Find(&rows).Error
	out := []d.Item{}
	for _, v := range rows {
		out = append(out, d.Item{ID: v.ID, Scope: l.Principal{TenantID: v.TenantID, UserID: v.UserID}})
	}
	return out, e
}
func (r *queryJobRepository) Claim(ctx context.Context, p l.Principal, id string) (out *d.Record, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row queryJobRow
		if e := queryScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", id).First(&row).Error; e != nil {
			return e
		}
		var leased bool
		e := tx.Raw("SELECT COALESCE(lease_until > UTC_TIMESTAMP(6), FALSE) FROM query_jobs WHERE id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Scan(&leased).Error
		if e != nil {
			return e
		}
		if row.Terminal || leased {
			return d.ErrConflict
		}
		row.Epoch++
		if e = tx.Model(&row).Updates(map[string]any{"epoch": row.Epoch, "lease_until": gorm.Expr("DATE_ADD(UTC_TIMESTAMP(6), INTERVAL 60 SECOND)")}).Error; e != nil {
			return e
		}
		out, e = queryRecord(row)
		return e
	})
	return out, queryErr(err)
}
func (r *queryJobRepository) Change(ctx context.Context, p l.Principal, id string, epoch int64, fn func(*d.Record, time.Time) error) (out *d.Record, err error) {
	if p.Validate() != nil {
		return nil, d.ErrForbidden
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row queryJobRow
		if e := queryScope(tx.Clauses(clause.Locking{Strength: "UPDATE"}), p).Where("id=?", id).First(&row).Error; e != nil {
			return e
		}
		now, e := queryNow(tx)
		if e != nil {
			return e
		}
		if epoch != 0 {
			var leased bool
			if e = tx.Raw("SELECT COALESCE(lease_until > UTC_TIMESTAMP(6), FALSE) FROM query_jobs WHERE id=? AND tenant_id=? AND user_id=?", id, p.TenantID, p.UserID).Scan(&leased).Error; e != nil {
				return e
			}
			if row.Epoch != epoch || !leased {
				return d.ErrConflict
			}
		}
		v, e := queryRecord(row)
		if e != nil {
			return e
		}
		if e = fn(v, now); e != nil {
			return e
		}
		v.Job.Version = row.Version + 1
		v.Job.UpdatedAt = now
		raw, e := json.Marshal(v)
		if e != nil {
			return e
		}
		if e = tx.Model(&row).Updates(map[string]any{"version": v.Job.Version, "status": v.Job.Status, "terminal": v.Job.Terminal(), "body": raw, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error; e != nil {
			return e
		}
		out = v
		return nil
	})
	return out, queryErr(err)
}
func (r *queryJobRepository) Release(ctx context.Context, p l.Principal, id string, epoch int64) error {
	if p.Validate() != nil {
		return d.ErrForbidden
	}
	return queryScope(r.db.WithContext(ctx).Model(&queryJobRow{}), p).Where("id=? AND epoch=?", id, epoch).Update("lease_until", nil).Error
}
